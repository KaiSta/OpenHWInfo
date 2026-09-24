# AGENTS.md — working in OpenHWInfo

The process for any agent editing this repository. It is loaded on every request,
so it holds only what applies to most edits; the detail lives in the skills below.

| You need | Load |
|----------|------|
| Where anything lives, before reading source | `code-map` skill — `.claude/skills/code-map/SKILL.md` (`/skill:code-map` in Codebuff, `/code-map` in Cline) |
| To commit, stage, install the hooks, or change `README.md` / `CODE_REVIEW.md` | `git-workflow` skill |
| What a user sees, row by row | `README.md`, by section |
| How the code got here, test coverage, backlog | `CODE_REVIEW.md`, by section |

Skills live in `.claude/skills/<name>/SKILL.md` with `name` (matching the folder)
and `description` frontmatter, and deep detail in that skill's `docs/`. This one
path is read by Codebuff, Cline and Claude Code, so an instruction lives there once
rather than once per tool. Read a skill instead of exploring: `main.go` (599
lines), `main_test.go` (932) and `website/app.js` (908) are meant to be read as
40-line windows around a search hit, never whole.

## What this project is

A native Linux hardware monitor — CPU, memory, GPU, storage, mainboard, battery —
as one self-contained executable: Go backend, Wails v2 webview, dependency-free
frontend embedded with `//go:embed all:website`.

- Module `github.com/KaiSta/OpenHWInfo`, Go 1.25. Direct dependencies are exactly
  two (`wailsapp/wails/v2`, `godbus/dbus/v5`); a third needs a reason in the commit
  body.
- Readings come from `/proc`, `/sys` and D-Bus (`udisksd`). No HTTP server, no
  root, no `lm-sensors`, `smartctl` or `nvme-cli`. Only
  `storage/statfs_{linux,other}.go` is build-tagged; the rest is portable Go.
- The rule the app is built around: **a reading that could not be taken produces no
  row, never a zero.**

## Gates — run before handing work back

`make build` (never a bare `go build .`, which fails by design: the `production`
tag is what links the real webview instead of a stub), tags `production` +
`webkit2_41` + `dev` for hot reload. Prerequisite:
`sudo apt install libwebkit2gtk-4.1-dev build-essential`.

```
make build    # go vet ./..., then go build -tags=production,webkit2_41 -o OpenHWInfo .
make run      # build, then run the binary
make dev      # go run -tags=dev,webkit2_41 . — frontend served from disk
make test     # go test ./...
make lint     # go vet ./...
make install  # desktop entry + hicolor icons into ~/.local/share (no root, idempotent)
make package  # .deb into dist/ (needs dpkg-deb); VERSION= and MAINTAINER= override
make map      # regenerate the code-map indexes under .claude/ (see below)
make hooks    # install .githooks/ for this clone: pre-commit gates gofmt + the code map
make clean    # remove ./OpenHWInfo and dist/
```

Reproduce what CI gates (`.github/workflows/ci.yml`):

```bash
gofmt -l .                          # must print nothing: formatting is gated, not fixed
go vet ./...
go test ./... -count=1 -race        # -race matters: collectors cache state across polls
go build -tags=production,webkit2_41 -o /tmp/OpenHWInfo .
```

Tests need no webview headers and no hardware — collectors are driven from fixture
trees, so the suite passes on a machine with no GPU, no battery and no drives.
`./OpenHWInfo -debug` traces what each collector found and why a device was
skipped. Never commit `./OpenHWInfo`, `dist/`, `build/` or logs; stage your own
files, never `git add -A`.

## Non-negotiable rules

Breaking one of these is a regression even when the tests still pass.

1. **An absent reading is no row, not a zero.** `shared.ReadSysFile` returns
   `(value, ok)` and callers must use `ok`. A missing hotspot sensor, an unreadable
   serial and an empty battery bay all produce no row. Never fabricate a `0`, an
   `Offline`, or an `Unknown` claim for data that was not read.
2. **The unit travels on the reading.** Units are `shared.HardwareUnit` values
   rendered by `HardwareUnit.Suffix()`; a row's label names the quantity only, so
   the two cannot disagree. A new quantity gets a new unit constant with a comment
   saying why it is distinct (see `Ratio`, `WattHour`, `Terabyte`, `Gibibyte`, and
   why `GB` and `GiB` must not be merged).
3. **Never spend a fork on the poll path.** A name or version that cannot change
   while the process runs is resolved once through `shared.CachedProbe`; slow
   readings such as the SMART attribute table are cached with a TTL and taken
   passively. A sysfs or `/proc` read beats shelling out, and `shared.RunCommand`
   (no shell) beats `shared.RunShell` when an argument derives from sysfs.
4. **No root, no optional external tool, no new dependency** for a reading that
   `/proc`, `/sys` or UDisks2 can supply.
5. **Keep collectors fixture-testable.** File-system roots and external paths are
   package variables so a test can point them at a `t.TempDir()` tree; a new
   collector follows that shape or splits its parsing into a pure function over the
   raw text.
6. **Ordering and caps are explicit.** Anything that could grow without bound —
   per-core rows, CCD sensors, drive sensors, DRM cards, board fans — is bounded by
   a named constant with a comment explaining the bound, and display order is never
   map iteration order.
7. **Do not log by default.** New tracing goes through `shared.Debugf`, silent
   unless `-debug` is passed.

## Conventions

Go: `gofmt`, tabs, no exception — CI fails on a diff. Comments record **why** (the
tradeoff, the rejected alternative, the hardware quirk); every exported identifier
and every package carries a doc comment naming the data source and the caps — copy
`mainboard`, `storage`, `memory` or `battery`, the four that have one, not the older
packages that do not. Errors are swallowed deliberately in collectors: a missing
sensor is a row that does not exist, not a failure, so log the reason through
`shared.Debugf` and return the partial data. Packages stay one-purpose, and a helper
two collectors need belongs in `shared` rather than copied into both.

Tests: beside the code as internal `package <name>`, plain `testing` with
`t.Helper()` helpers and no framework. A test never needs hardware — it builds a
fixture tree in `t.TempDir()`, points the package's path variable at it and restores
the original in `t.Cleanup`; `code-map`'s `docs/testing.md` names the helper and the
variable per package. Call `shared.ResetProbeCache()` when a fixture is rewritten in
place, assert both directions (the value with its unit, and the absent case
producing **no key** rather than `""` or `0`), name tests as sentences about the
behaviour, and name a regression test for the failure it pins. The frontend has no
runner: verify a change by rendering a `Data` payload in the webview with `make dev`.

Committing: the `git-workflow` skill carries the staging rules, the commit hook,
the message contract and the table of which document moves with which change —
load it before staging. Documentation belongs in the same commit as the code, and a
changed symbol is a documentation change: run `make map`, which the pre-commit hook
and the test suite both enforce.

## Done

- [ ] `gofmt -l .` prints nothing, `go vet ./...` is clean.
- [ ] `go test ./... -count=1 -race` passes, with the new test covering both the
      reading that should appear and the absent case that must produce no row. This
      includes the code-map guard, so a symbol added, renamed or removed needs
      `make map` and the regenerated indexes in the same commit.
- [ ] `make build` succeeds.
- [ ] `README.md`, `CODE_REVIEW.md` and the skills are updated per the table in
      `git-workflow`, and the generated code-map indexes are staged.
- [ ] The commit message follows `git-workflow`, including what was verified on
      real hardware versus fixtures.
- [ ] `git status` shows only the source and doc changes.
