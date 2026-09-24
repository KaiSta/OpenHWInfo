---
name: code-map
description: Navigable index of OpenHWInfo — which file owns what, the poll chain, the behaviour-to-code jump table, key namespaces and recipes for common changes. Use before reading source, when locating a symbol or line, when choosing which file to edit, or when adding a reading end to end.
---

# OpenHWInfo code map

Load this before opening a source file. `main.go`, `main_test.go`,
`website/app.js` and `CODE_REVIEW.md` are long files — read any of them as
40-line windows around a search hit, never whole.

**There are no line numbers in this file, deliberately.** A number written here
would be wrong after the next edit; the symbols and their lines live in the
generated indexes beside this file, which `go test ./...` keeps current:

- `docs/symbol-index.md` — every top-level symbol in the source and the frontend.
- `docs/test-index.md` — every `Test*` and every fixture helper, per package.
- `docs/testing.md` — the fixture and pattern guidance behind those helpers.

## Read code cheaply

1. Search first — your search tool returns matches with their current line
   numbers; trust those over anything remembered from an earlier session.
2. Then read the window — about 40 lines around the hit, never the whole file.
3. For a whole file's shape in one read, use its entry in `docs/symbol-index.md`.
4. Read a package's first ~30 lines before its code — that is where its
   `// Package` comment states the data source and the caps.
5. Read a document by section: `grep -n '^## ' README.md CODE_REVIEW.md`.

## Files at a glance

| Path | Owns |
|------|------|
| `main.go` | Wails entry point, `App.GetData`, `DeviceView`, min/max history, `formatValue`/`withUnit`, template lists |
| `main_test.go` | view assembly, template/key contracts, history window, units |
| `shared/hardware.go` | `HardwareUnit` + `Suffix()`, `HardwareValue`, `HardwareData`, `DeviceStatus` |
| `shared/templates.go` | every fixed `KeyTemplate` (label + key) |
| `shared/sysutil.go` | `RunShell`, `RunCommand`, `ReadSysFile`, `FindFirstSubpath`, `Parse*` |
| `shared/drm.go` | `ProbeDRMCards`, `ResolvePCIDevice`, `maxDRMCards` |
| `shared/cache.go` | `CachedProbe`, `ResetProbeCache` |
| `shared/debug.go` | `SetDebug`, `Debugf` |
| `cpuinfo/cpu.go` | `/proc/cpuinfo` + the CPU's own hwmon chips |
| `mainboard/mainboard.go` | DMI identity, ACPI zones, Super I/O chips |
| `memory/memory.go` | `/proc/meminfo`, PSI pressure, zram |
| `storage/storage.go` | drives, filesystems, drive temperatures |
| `storage/smart.go` | UDisks2/D-Bus SMART: fetch, cache, health verdict |
| `storage/statfs_linux.go` / `_other.go` | the only build-tagged files (`linux` / `!linux`) |
| `battery/battery.go` | `/sys/class/power_supply`, health verdict |
| `amd/amd.go`, `intel/intel.go` (+`pci_ids.go`), `nvidia/nvidia.go`, `lisuan/lisuan.go` (+`pci_ids.go`) | one GPU vendor each |
| `website/app.js` | the whole renderer: cards, gauges, core grid, polling |
| `website/styles.css` | the `good`/`warn`/`bad` palette gauges and badges share |

## The poll chain — one path, five hops

1. `website/app.js` `POLL_MS` (5 s), `getData()` → `window.go.main.App.GetData()`, driven by `tick()`.
2. `main.go` `App.GetData` calls each collector exactly once: `cpuinfo`, then `nvidia`/`intel`/`amd`/`lisuan`, then `storage`, `mainboard`, `memory`, `battery`. A machine whose vendors report no card gets no GPU card either (see `gpuViews`).
3. Collectors return `shared.HardwareData{Name, Values, Status}`; an absent vendor returns nil/empty rather than being probed twice for availability.
4. `main.go` `deviceView` keeps only the templates that carry a value, formats them (`formatValue` + `withUnit`) and folds the numeric ones into that card's own history under a prefix (`gpu0.`, `disk1.`, `battery0.`; none for board and memory).
5. `website/app.js` `render` draws the cards; `gauge` picks bar direction and colour, `betterHigher` flags the readings where lower is better.

History is the last 100 samples, mutex-guarded because Wails does not promise
`GetData` is never called concurrently. Two frontend rules: it is one ES module
loaded directly by the webview (no bundler, no npm, no `node_modules` in the build
path), and a unit is never appended there — it arrives on the reading.

## Where a behaviour is decided

| "Why does the UI do X?" | Look at |
|-------------------------|---------|
| a row is missing | `deviceView` in `main.go` drops templates without a value — fix the collector, never add a `0` |
| a row's order | `numericSuffixes`, `labelledBoardTemplates` in `main.go`; never map iteration order |
| unit text (`°C`, `GiB`, `:1`) | `HardwareUnit.Suffix()` in `shared/hardware.go` — the unit rides on the reading, never in a label |
| min/max range, 100-sample window | `updateHistoryAndCalculateMinMax`, `calculateMinMax` in `main.go`; per-card prefixes in `GetData` |
| a card's health badge | `shared.DeviceStatus`, produced by `storage/smart.go` `evaluateHealth` and `battery/battery.go` `evaluateHealth`, drawn by `statusPill`/`updateStatus` in `app.js` |
| bar colour and direction | `gauge`, `temperatureLimit`, `betterHigher` in `app.js`; palette in `styles.css` |
| CPU tile pairing clock with core temp | `tempLayout`, `buildCoreGrid` in `app.js` |
| how many cores, sensors or fans fit | the named caps: `maxTccdSensors` (cpuinfo), `maxDiskTemps` (storage), `maxDRMCards` (shared), `maxBoardTemps`/`maxBoardFans` (mainboard) |

## Key namespaces

Rows are prefixed per card, so a key says which device it belongs to and two cards
never collide: `cpu.*` (`cpu.clockN`, `cpu.coreTempN`, `cpu.temp`, `cpu.pch`,
`tccdN`), `board.*` (`board.tempN`/`board.fanN` plus the data keys
`board.tempLabelN`/`board.fanLabelN`), `disk.*` (`disk.tempN` + `disk.tempLabelN`,
`disk.smart.*`), `mem.*`/`swap.*`/`zram.*`, `battery.*`. GPU rows are unprefixed
(`name`, `driverVersion`, `gpu.*`, `vram.*`, `fan.speed`) because each GPU is its
own card. A `*LabelN` key is **data, not a metric**: no template names it, so it
never reaches the table — `main.go` reads it to name the generated row.

## Task recipes

**Add a reading to an existing card** — collect it in the package's
`GetInformations` only when the read succeeded; add a `KeyTemplate` to the right
block in `shared/templates.go` and to the matching list in `main.go`
(`gpuTemplates`, `diskTemplates`, `memTemplates`, `batteryTemplates` — CPU, board
and disk temperature rows are generated instead); reuse a unit or add one to
`HardwareUnit` with a comment saying why it is distinct; extend the package's
`Test*TemplatesMatchCollectorKeyContract` in `main_test.go`, which is what catches
a template no collector produces; update the README section.

**Add a collector package** — copy the shape of `battery` or `memory`: `// Package`
comment (source + caps), `sysDir`-style path variable, `GetInformations`, a
`NewHardwareValue` per reading, a fixture test, a template list and a call in
`GetData`, then this file's table, `docs/testing.md`, and `Architecture (as built)`
in `CODE_REVIEW.md`. A vendor whose identity is not in the shipped databases also
needs a naming table of its own (`lisuan/pci_ids.go` is the newest example).

**Add a health verdict** — return `*shared.DeviceStatus` from the collector and the
badge follows for free (`deviceView` copies it, `updateStatus` draws it). Use the
`StatusGood`/`Warn`/`Bad`/`Unknown` constants, always fill `Detail`, and withhold the
badge when the figures behind it are absent.

**Debug a card that is missing** — run `./OpenHWInfo -debug` (or
`go run -tags=dev,webkit2_41 . -debug`), which prints per poll what each collector
found and why a device was skipped. A probe that can silently skip a device should
log through `shared.Debugf`.

**After adding, renaming or removing a symbol** — run `make map` and commit the
result; `go test ./...` fails until the generated indexes match the tree.

## Reference — read only when needed

- `.claude/skills/code-map/docs/symbol-index.md` — symbols and lines, source + frontend.
- `.claude/skills/code-map/docs/test-index.md` — test functions and fixture helpers.
- `.claude/skills/code-map/docs/testing.md` — how the fixtures are written.
