---
name: git-workflow
description: How a change is staged, gated, committed and documented in OpenHWInfo — the commit checklist that includes refreshing the generated code map, the hooks, what may never be staged, the commit-message contract and the table of which document moves with which change. Use when committing or staging changes, writing a commit message, installing the git hooks, refreshing the code map, or updating README.md, CODE_REVIEW.md or AGENTS.md.
---

# Committing a change in OpenHWInfo

The history is the maintenance record, so a commit says what changed **and why**.
Read `git --no-pager log -5` before writing one: the body is long, plain prose, and
that is the norm rather than an exception. One commit per logical change — never mix
an unrelated cleanup into a fix — and commit only when the user asked for it.

## Before you commit

1. **Refresh the code map.** `.claude/skills/code-map/docs/symbol-index.md` and
   `docs/test-index.md` carry the line numbers every agent navigating this
   repository reads. They are generated from the source, so every symbol you added,
   renamed, removed or moved between files makes them wrong — and a commit that
   leaves them wrong hands the next reader a lie. Run

   ```bash
   make map      # rewrite both indexes from the tree
   ```

   and stage the result in the same commit. Two things fail while the indexes and
   the tree disagree: the guard test `TestAgentDocsIndexesAreCurrent`, which
   `go test ./...` and CI both run, and the `pre-commit` hook for anyone who
   installed it. A commit that changes symbols but not the map is incomplete.

   Only the generated indexes hold numbers. The hand-written half — the code-map
   `SKILL.md`, `docs/testing.md`, this skill, `AGENTS.md` — cites symbols and files
   and never line numbers, so it changes only when the *structure* moves: a new
   package, entry point, cap constant or fixture helper.

2. **Update the documents the change invalidates** — the table below. Documentation
   belongs in the same commit as the code it describes, and drift here is treated as
   a bug rather than a follow-up.

3. **Run the gates**: `gofmt -l .` printing nothing, `go vet ./...`,
   `go test ./... -count=1 -race`, `make build`. Report what you actually ran.

4. **Stage only what you changed.** The files the change touched, never
   `git add -A`, and never a hunk another author is editing in a shared checkout —
   if ownership is ambiguous, leave it unstaged and say so. Never stage
   `./OpenHWInfo`, `dist/`, `build/`, logs, editor files or `.freebuff/`; they are
   gitignored, and `git add -f` is not the way around that.

5. **Write the message** in the shape below.

## The hook

`.githooks/` holds the same rules for whoever wants them enforced locally;
`make hooks` points `core.hooksPath` at it, once per clone. `pre-commit` runs the
two gates a commit would make worse for the next author — `gofmt -l .` and the
code-map guard — and `commit-msg` checks the mechanical half of the subject rules
(non-empty, ≤ 72 characters, no trailing punctuation, no
`WIP`/`fixup`/`various improvements`), noting a missing body without blocking it,
since a documentation-only commit may legitimately be brief.

Both hooks **gate rather than fix** — the same stance CI takes on formatting, a
silent rewrite is a decision taken from the author — so a failure prints the command
that repairs it. They are fast and local; CI (`.github/workflows/ci.yml`) remains
the authority, running the same checks plus `-race`, `go vet` and the production
build, so nothing depends on a hook being installed.

## Message shape

```
<area>: imperative subject, ≤ 72 chars, no trailing period

Blank line. 2–6 paragraphs wrapped at ~80 columns: what the old behaviour was, why
it was wrong or what it cost, what the new behaviour is, the rejected alternative if
one was considered, and the user-visible consequence (a row that appears, a unit
that changes, a badge that is withheld) because the README has to match it. Then a
verification paragraph naming the exact commands run and what they reported, and
what was checked on real hardware versus only fixtures.

🤖 Generated with <tool>
Co-Authored-By: <tool> <noreply@example.com>
```

1. Imperative, capitalised subject, no period, ≤ 72 characters.
2. An `area:` prefix only for a change confined to one package
   (`intel: resolve specific GPU model names from PCI device IDs`).
3. A behaviour change is never a one-line commit: a changed reading, row, unit,
   verdict or threshold gets a body explaining old and new. Documentation-only and
   formatting-only commits may be brief, but say so.
4. Include the verification paragraph with the commands actually run. Never claim a
   check that was not run; say what could not be verified (a battery family or a
   drive model this host does not have) instead of implying coverage.
5. No WIP, "fix up", "missing stuff" or "various improvements" subjects — those
   exist in the early history and are not the standard.
6. Say what was deliberately not done, and add it to `## Remaining opportunities` in
   `CODE_REVIEW.md` when it is worth tracking.
7. Keep the AI-attribution trailers on AI-assisted commits.
8. The **release commit** — the one the `v*` tag points at, which bumps
   `./VERSION` — is the exception to rules 3 and 4. It is read by users deciding
   whether to upgrade rather than by the next maintainer, so its body is short and
   lists the user-visible changes as bullets in the words `README.md` uses. The
   release workflow publishes that body as the GitHub release notes, so it is
   Markdown. `AGENTS.md` carries the full wording.

## Which document moves

| If you changed… | Update |
|-----------------|--------|
| Any symbol: added, renamed, removed, or moved between files | `make map`, and stage `.claude/skills/code-map/docs/` |
| A row, card, unit, threshold, badge or refresh behaviour a user can see | `README.md` — that device's `## … readings` section, and `Reading the output` if the min/max or table contract moved |
| A collector's data source, what it reads, or what it deliberately ignores | `README.md` (that device's section) and the package doc comment |
| A new package, entry point, cap constant or fixture helper | The hand-written half of the `code-map` skill: its file table, the poll chain, `docs/testing.md` |
| A package's responsibility or a build step | `## Architecture (as built)` in `CODE_REVIEW.md`, "What this project is" in `AGENTS.md`, and the file table in `code-map` |
| The build, run, test or packaging workflow (Makefile target, tag, CI step, hook) | `README.md` `## Build & Run`, the gates section of `AGENTS.md`, and `## Repo hygiene` in `CODE_REVIEW.md` |
| Added or removed tests | The `## Test coverage` table in `CODE_REVIEW.md`, with the new cases named |
| Fixed something listed as a known limitation | Delete it from `## Remaining opportunities`, record the fix under `## Issues addressed`, naming the regression tests |
| A new deliberate limitation (hardware you cannot verify, a convention that cannot be disambiguated) | Add it to `## Remaining opportunities`, saying what would fix it and why it cannot be verified here |

## How to write the prose

- Never state something the code does not do. If a row is withheld in some case, the
  README says so and says why; if a figure is a sum over devices, it says it is a sum.
- Write the *why* in the project's voice: what the kernel reports, what the app does
  with it, and why the obvious alternative was rejected (converting Wh to Ah,
  guessing a device's identity, reporting zero for an absent sensor). A bare list of
  field names is not enough.
- Keep the README's structure: device sections in card order, rows in render order,
  new rows where they render rather than appended.
- Extend the coverage table in `CODE_REVIEW.md`; do not add a second one.
- Do not add new top-level `.md` files — four documents are enough, and process
  detail belongs in a skill. Package and function doc comments count as
  documentation.
- Never write a line number into a hand-written document; cite the symbol and its
  file, and let the generated indexes carry the numbers.
- Never leave a stale claim behind. If you cannot update a document in the same
  change, the change is too large — split it.
