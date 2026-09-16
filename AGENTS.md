# AGENTS.md — working in OpenHWInfo

This file is written for AI agents (and for humans driving one). It states what
the project is, how it is built, tested and run, the conventions the code holds
itself to, and the rules a commit and a documentation change have to satisfy.
Read it before the first edit.

The other two documents are not optional:

| File | Answers |
|------|---------|
| `README.md` | What the app shows a user, row by row, and what it deliberately does not show. |
| `CODE_REVIEW.md` | How the code got here: architecture, issues already fixed, test coverage, remaining opportunities. |

`AGENTS.md` is the process. `README.md` is the user-facing contract.
`CODE_REVIEW.md` is the maintenance record. All three must stay true at once.

## What this project is

OpenHWInfo is a **native Linux desktop hardware monitor**: CPU, memory, GPU,
storage, mainboard and battery. It is a single self-contained executable — a Go
backend, a Wails v2 webview window, and a dependency-free frontend that is
embedded into the binary with `//go:embed all:website`.

- Module: `github.com/KaiSta/OpenHWInfo` (`go 1.25.0` in `go.mod`).
- Direct dependencies: **exactly two** — `github.com/wailsapp/wails/v2` (window
  and asset server) and `github.com/godbus/dbus/v5` (SMART over UDisks2). Every
  other entry in `go.mod` is an indirect dependency of those two. Adding a direct
  dependency needs a stated reason in the commit body.
- Platform: Linux. `storage/statfs_linux.go` carries a `//go:build linux` guard
  and `statfs_other.go` its `!linux` counterpart; the rest is portable Go.
- No HTTP server, no port, no root, no `lm-sensors`, no `smartctl`, no
  `nvme-cli`. Readings come from `/proc`, `/sys` and D-Bus through `udisksd`.
- The rule the whole app is built around: **a reading that could not be taken
  produces no row, never a zero.** See "Non-negotiable rules" below.

## Repository map

```
main.go            Wails entry point, App/GetData, DeviceView assembly, min/max
                   history, formatValue/withUnit(s), embedded assets + icon
main_test.go       View assembly, history and unit-rendering tests
amd/               AMD GPU collector (DRM sysfs + lspci), amd_test.go
intel/             Intel GPU collector (i915 sysfs + lspci/glxinfo), pci_ids.go
nvidia/            Nvidia GPU collector (nvidia-smi CSV)
cpuinfo/           /proc/cpuinfo + the processor's own hwmon chips
mainboard/         DMI identity + ACPI zones + Super I/O (nct67xx, it87xx, …)
memory/            /proc/meminfo, PSI pressure, zram sysfs
storage/           /sys/block + /sys/class/block, /proc/mounts, statfs, smart.go
battery/           /sys/class/power_supply (one card per battery)
shared/            shared types + helpers: hardware.go (HardwareValue/HardwareUnit/
                   DeviceStatus), templates.go (key templates), sysutil.go
                   (RunShell/RunCommand/ReadSysFile/FindFirstSubpath/Parse*),
                   drm.go (ProbeDRMCards/DRMCardDevicePath/ResolvePCIDevice),
                   cache.go (CachedProbe/ResetProbeCache), debug.go (Debugf)
website/           index.html shell, app.js renderer, styles.css, resources/
icons/             icon.svg plus the PNG sizes install/package ship as hicolor
openhwinfo.desktop Desktop entry template (@EXEC@ is substituted by the Makefile)
wails.json         Wails project metadata (frontend dir, output filename)
Makefile           build / run / dev / test / lint / install / package / clean
.github/workflows/ci.yml   gofmt gate, go vet, go test -race, production build
```

There is no `docs/` directory and no generated documentation. The documents are
`README.md`, `CODE_REVIEW.md`, `AGENTS.md` and the Go doc comments.

## Build, run and test — use the Makefile

`make build` is the supported build path. A bare `go build .` fails by design
("Wails applications will not build without the correct build tags"), because the
`production` tag is what makes Wails link the real webview instead of a no-op
stub.

| Command | What it does |
|---------|--------------|
| `make build` | `go vet ./...` then `go build -tags=production,webkit2_41 -o OpenHWInfo .` |
| `make run` | `make build`, then runs `./OpenHWInfo` |
| `make dev` | `go run -tags=dev,webkit2_41 .` — development mode, frontend served from disk (the Makefile's hot-reload path) |
| `make test` | `go test ./...` |
| `make lint` | `go vet ./...` |
| `make install` | Builds, then installs the desktop entry and hicolor icons into `~/.local/share` (no root). Idempotent. |
| `make package` | Builds a `.deb` into `dist/` (binary + desktop entry + icons). Needs `dpkg-deb`, so it is Debian/Ubuntu-only. |
| `make clean` | Removes `./OpenHWInfo` and `dist/`. |

Build tags, in full:

- `production` — the real app. Without it Wails compiles a no-op stub.
- `webkit2_41` — link WebKitGTK 4.1. Drop it on a distro that still ships 4.0
  and install `libwebkit2gtk-4.0-dev` instead.
- `dev` — used only by `make dev`; serves the frontend from `website/` on disk
  instead of from the embedded copy, so frontend edits show on reload.

Before handing work back, reproduce what CI gates (`.github/workflows/ci.yml`):

```bash
gofmt -l .                          # must print nothing: formatting is gated, not fixed
go vet ./...
go test ./... -count=1 -race        # -race matters: collectors cache state across polls
go build -tags=production,webkit2_41 -o /tmp/OpenHWInfo .
```

Prerequisites for a local build: `sudo apt install libwebkit2gtk-4.1-dev
build-essential` (plus `libgtk-3-dev` on a minimal image). Tests do not need the
webview headers: the hardware collectors are exercised against fixtures by
pointing their `sysDir`/path variables at a `t.TempDir()` tree, so `go test ./...`
runs on a machine with no GPU, no battery and no drives.

`./OpenHWInfo -debug` turns on the collectors' per-poll tracing
(`shared.SetDebug`, `shared.Debugf`), which logs which DRM cards, drives and
sensor chips were found and why a device was skipped. Normal runs print nothing
but the window, so that a genuine warning is visible when it appears.

Artifacts: the built `./OpenHWInfo`, `dist/` and `build/` are gitignored and must
never be committed. `make package` writes
`dist/openhwinfo_<version>_<arch>.deb`; override `VERSION` and `MAINTAINER` for a
release (`make package VERSION=1.0.0 MAINTAINER="You <you@example.org>"`).

## Architecture and data flow

One poll is one call chain; keep new code on it rather than inventing a second
path.

1. `website/app.js` polls `window.go.main.App.GetData()` every 5 seconds. The
   Wails binding exposes `App`'s exported methods; `startup`/`shutdown` are the
   lifecycle hooks, and `main()` wires the window options, the embedded assets
   and the Linux `ProgramName: "openhwinfo"` that ties the window to the desktop
   entry.
2. `App.GetData` in `main.go` calls each collector exactly once per poll:
   `cpuinfo.GetInformations()`, then `nvidia`/`intel`/`amd`
   `GetAllInformations()`, then `storage`, `mainboard`, `memory`, `battery`.
   A vendor that is absent returns `nil`/an empty slice rather than being probed
   twice for availability.
3. Collectors return `shared.HardwareData`: a `Name`, a
   `map[string]shared.HardwareValue` of readings, and an optional
   `*shared.DeviceStatus` verdict (drives and batteries carry one; GPUs and the
   board do not).
4. `main.go` turns each `HardwareData` into a `DeviceView` via `deviceView()`:
   it keeps only the templates that carry a value, formats each value to display
   text (`formatValue` + `withUnit`), and updates that device's own
   `historyStore`. Per-card prefixes (`gpu0.`, `disk1.`, `battery0.`) keep two
   cards' ranges apart; the board and memory are single devices and use no
   prefix. History holds the last 100 samples and is mutex-guarded because Wails
   does not promise that `GetData` is never called concurrently.
5. Row order is never map iteration order. Numeric families (`cpu.clockN`,
   `cpu.coreTempN`, `tccdN`, `board.tempN`, `board.fanN`, `disk.tempN`) get their
   templates generated from the collected values in ascending numeric order by
   `main.go`'s `numericSuffixes` / `indexedCPUTemplates` / `labelledBoardTemplates`
   helpers. Fixed rows come from `shared/templates.go`.
6. `website/app.js` renders `Data` into cards with no framework and no build
   step. Its `gauge()` decides bar direction and colour class
   (`good`/`warn`/`bad`) per key, and `betterHigher()` covers the readings that
   are worse when lower (battery charge, a drive's spare pool). Verdict badges
   reuse the same palette through `DeviceStatus.Level`
   (`good`/`warn`/`bad`/`unknown`).

Two frontend conventions matter when editing it: the file is one ES module loaded
directly by the webview (no bundler, no npm, no `node_modules` in the build
path), and a unit is never appended by the frontend — it comes from the reading.

## Non-negotiable rules

These are the invariants this codebase was built around. A change that breaks one
of them is a regression even when the tests still pass.

1. **An absent reading is no row, not a zero.** `shared.ReadSysFile` returns
   `(value, ok)` and callers must use `ok`. A missing hotspot sensor, an
   unreadable serial and an empty battery bay all produce no row. Never fabricate
   a `0`, an `Offline`, or an `Unknown` claim for data that was not read.
2. **The unit travels on the reading.** Units are `shared.HardwareUnit` values
   rendered by `HardwareUnit.Suffix()`; a row's label names the quantity only.
   Never spell a unit into a `KeyTemplate` label — that gives a row two places to
   disagree with its number, which is the exact bug this codebase removed. A new
   quantity gets a new unit constant, with a comment saying why it is distinct
   (see `Ratio`, `WattHour`, `AmpereHour`, `Terabyte`, `Gibibyte`, and why `GB`
   and `GiB` must not be merged).
3. **Never spend a fork on the poll path.** Identity that cannot change while the
   process runs (GPU marketing name, driver version) is resolved once through
   `shared.CachedProbe`; slow readings such as the SMART attribute table are
   cached with a TTL and taken passively. A sysfs or `/proc` read is cheap and
   preferred over shelling out, and `shared.RunCommand` (no shell) is preferred
   over `shared.RunShell` whenever an argument derives from sysfs.
4. **No root, no optional external tool, no new dependency** for a reading that
   `/proc`, `/sys` or UDisks2 can supply. The app must work on a machine that has
   none of `lm-sensors`, `smartctl` or `nvme-cli` installed.
5. **Keep collectors fixture-testable.** File-system roots and external paths
   (`sysDir`, `meminfoPath`, `mountsPath`, `zramPath`, …) are package variables so
   a test can point them at a `t.TempDir()` tree. A new collector follows the same
   shape, or its parsing is split into a pure function that takes the raw text.
6. **Ordering and caps are explicit.** Anything that could grow without bound —
   per-core rows, CCD sensors, drive sensors, DRM cards — is bounded by a named
   constant with a comment explaining the bound.
7. **Do not log by default.** New tracing goes through `shared.Debugf`, which is
   silent unless `-debug` is passed.

## Coding and testing conventions

Go:

- Standard `gofmt` formatting, tabs, no exception. CI fails on a `gofmt -l .`
  diff, so format before committing.
- Comments here explain **why**, not what: a comment earns its place by
  recording the tradeoff, the rejected alternative, or the hardware quirk
  behind the line. Compare `shared.CachedProbe` (why the cache stores empty
  results) or `HardwareUnit` (why `Gibibyte` and `Gigabyte` are separate).
  Every exported identifier carries a doc comment, and every package carries
  one of its own: a `// Package <name>` block naming the data source and the
  caps. `mainboard`, `storage`, `memory` and `battery` show that shape;
  `amd`, `intel`, `nvidia`, `cpuinfo`, `shared`, the root `main` package and
  `main.Data` predate the rule and still lack one. The backlog is listed under
  `Remaining opportunities` in `CODE_REVIEW.md`, so a new package copies the
  four that have a comment rather than the six that do not.
- A comment spanning a decision should name what happens in the awkward case
  ("a drive whose counter cannot be read gets no row, because a figure quietly
  too low is worse than no figure").
- Errors are swallowed deliberately in most collectors: a missing sensor is not
  an error, it is a row that does not exist. Log through `shared.Debugf` when the
  reason helps diagnosis, and return the partial `HardwareData`.
- Keep the packages flat and one-purpose (`amd`, `intel`, `nvidia`, …). A helper
  used by two collectors belongs in `shared`, not copied into both — duplicate
  helpers were removed once already (see `CODE_REVIEW.md`, "Issues addressed").

Tests:

- Tests live beside the code as `package <name>` (internal tests, so unexported
  parsers and the fixture variables are reachable) and there is no test
  framework: plain `testing` with `t.Fatalf`/`t.Errorf` and helpers marked
  `t.Helper()`.
- Hardware is never required. A collector test builds a fixture tree in
  `t.TempDir()`, writes the sysfs/proc files it wants, points the package's path
  variable at it, and restores the original in `t.Cleanup`. Follow the existing
  `writeHWMonFixture` (`cpuinfo`), `writeSysFixture` (`mainboard`, `storage`) and
  `writeFixture` (`memory`) pattern rather than inventing a second one. Call
  `shared.ResetProbeCache()` in tests that rewrite a fixture in place, because
  identity probes are memoized for the process lifetime.
- Both directions are asserted: a value that should appear, with its unit, and
  the absent case producing **no key** rather than `""` or `0`. Regression tests
  are named for the failure they pin, and `CODE_REVIEW.md` lists their names when
  the fix is recorded.
- Test names read as sentences and name the behaviour, not the function:
  `TestCPUIntelCoretempTemperatures`, `TestParseCpuInfoMalformedDoesNotPanic`,
  `TestZramUnconfiguredDevicesAreSkipped`.
- Run the suite with `-race`: several collectors cache state between polls.
- Frontend changes are checked in the browser by rendering `Data` payloads by
  hand (see the verification paragraphs in the commit history), because the
  project has no JS test runner.

## Commit rules

The history is the maintenance record, so a commit has to say what changed and
why. Read `git --no-pager log -5` before writing one: the body is long, plain
prose, and it is the norm rather than an exception. One commit per logical
change; never mix an unrelated cleanup into a fix.

A commit message has this shape:

```
<area>: imperative subject, ≤ 72 chars, no trailing period

Blank line. 2–6 paragraphs of body text wrapped at ~80 columns. Say what the
old behaviour was, why it was wrong or what it cost, and what the new behaviour
is. Name the rejected alternative when one was considered. Mention the
user-visible consequence (a row that appears, a unit that changes, a badge that
is withheld) because that is what the README has to match. Explain anything
deliberately not done.

Then a verification paragraph: the exact commands run and what they reported,
e.g. "gofmt, go vet, the test suite in 10 packages under -race and make build all
pass", plus what was checked on real hardware and what was only fixture-tested.

🤖 Generated with <tool>
Co-Authored-By: <tool> <noreply@example.com>
```

Rules:

1. **Subject line: imperative mood, capitalised, no period, ≤ 72 characters.**
   "Report the machine's zram store on the memory card" — not "Reporting…",
   not "Added…".
2. **An `area:` prefix is optional** and only for a change confined to one
   package, e.g. `intel: resolve specific GPU model names from PCI device IDs`.
   A cross-cutting change gets no prefix.
3. **A behaviour change must not be a one-line commit.** If a reading, a row, a
   unit, a verdict or a threshold changed, the body explains the old and new
   behaviour and why. Documentation-only or formatting-only changes may be
   brief, but say so.
4. **Include the verification paragraph** with the commands actually run. Never
   claim a check that was not run; say what could not be verified (a battery
   family or a drive model the review host does not have) instead of implying
   coverage.
5. **Documentation changes belong in the same commit as the code.** Reviewers
   (human or agent) must be able to read one commit and find both the change and
   the doc that describes it.
6. **No WIP, "fix up", "missing stuff" or "various improvements" subjects.**
   Those commit subjects are in the early history and are not the standard.
   Squash a fix back into the commit it belongs to before pushing, or describe
   precisely what it fixes.
7. **Say what was deliberately not done**, including known limitations, and add
   such an item to "Remaining opportunities" in `CODE_REVIEW.md` when it is worth
   tracking.
8. Keep the AI-attribution trailers when the commit was AI-assisted — they are
   already part of the project's history.
9. Never commit build artifacts: `./OpenHWInfo`, `dist/`, `build/`,
   `website/node_modules`, logs. `.gitignore` already covers them; do not
   `git add -f`.

## Documentation is part of the change

Documentation drift is treated as a bug in this project, not as a follow-up. A
change is not finished until the documents that describe it are true again, and
they are updated **in the same commit** as the code. Which document moves:

| If you changed… | Update |
|-----------------|--------|
| A row, card, unit, threshold, badge or refresh behaviour a user can see | `README.md` — the relevant `## … infos` section, and `## Reading the output` if the min/max or table contract moved |
| A collector's data source, what it reads, or what it deliberately ignores | `README.md` (the section for that device) and the package doc comment |
| A package, its responsibility, its public entry point, or a build/packaging step | `## Architecture (as built)` in `CODE_REVIEW.md`, and `## What this project is` / `## Repository map` here |
| The build, run, test or packaging workflow (Makefile target, tag, CI step) | `README.md` `## Build & Run`, this file's Makefile section, and `## Repo hygiene` in `CODE_REVIEW.md` if it is about the tree |
| Added or removed tests | The `## Test coverage` table in `CODE_REVIEW.md`, with the new cases named |
| Fixed something listed as a known limitation | Delete the item from `## Remaining opportunities` and record the fix under `## Issues addressed`, naming the regression tests |
| Found a new deliberate limitation (hardware you cannot verify, a convention that cannot be disambiguated) | Add it to `## Remaining opportunities`, saying what would be needed to fix it and why it cannot be verified here |

Rules for documentation:

1. **Never state something the code does not do.** If a row is withheld in some
   case, the README says so and says why. If a figure is a sum over devices, the
   README says it is a sum. The existing prose is precise about this — match it.
2. **Write the *why*, in the project's voice.** The README explains what the
   kernel reports, what the app does with it, and why the alternative was
   rejected (converting Wh to Ah, guessing a device's identity, reporting zero
   for an absent sensor). New paragraphs carry the same weight of reasoning; a
   bare list of field names is not enough.
3. **Keep the README's structure.** Device sections live under `## … infos`, in
   the order the cards appear, and describe rows in the order the card shows
   them. New rows are added where they render, not appended at the end.
4. **Update the coverage table, do not duplicate it.** `CODE_REVIEW.md`'s table
   summarises what the tests cover in prose; new test cases extend the relevant
   cell instead of adding a second table.
5. **Do not create new top-level `.md` files without a reason.** Four documents
   are enough: a change is documented where it belongs, not in a new file. If a
   document becomes wrong, fix it rather than adding a note elsewhere.
6. **Package and function doc comments count as documentation.** They are the
   only place a future reader sees the data source and the caps; a new collector
   without a package comment is incomplete.
7. **Never leave a stale claim behind.** If you cannot update a document in the
   same change, that is a signal the change is too large — split it.

## Definition of done

Before reporting work as finished, all of the following must hold:

- [ ] `gofmt -l .` prints nothing.
- [ ] `go vet ./...` is clean.
- [ ] `go test ./... -count=1 -race` passes, with new tests covering both the
      reading that should appear and the absent case that must produce no row.
- [ ] `go build -tags=production,webkit2_41 -o /tmp/OpenHWInfo .` (or `make build`)
      succeeds.
- [ ] `README.md` and `CODE_REVIEW.md` are updated per the table above, and
      `AGENTS.md` is updated if the workflow itself changed.
- [ ] The commit message follows the commit rules, including the verification
      paragraph and what was verified on real hardware versus fixtures.
- [ ] No build artifact, log or editor file is staged (`git status` is clean of
      everything but the source and doc changes).
