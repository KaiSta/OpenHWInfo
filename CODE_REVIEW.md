# OpenHWInfo Code Review

## Architecture (as built)

OpenHWInfo is a **native desktop application**, not a web server. Wails v2 binds
the Go `App` struct to the embedded webview, the Go backend collects sensor data,
and the frontend calls the bound `App.GetData` method directly.

- **Entry point:** `main.go` — builds the Wails options, embeds the frontend with
  `//go:embed all:website`, and exposes `GetData()` to JavaScript.
- **Frontend:** `website/index.html` — a single self-contained page, rendered from
  the `Data` struct returned by `GetData`. Polls every 5 seconds via
  `window.go.main.App.GetData()`.
- **Collectors:** one package per vendor.
  - `cpuinfo` — `/proc/cpuinfo` plus lm-sensors `sensors` output.
  - `nvidia` — `nvidia-smi --query-gpu=... --format=csv`.
  - `amd` — DRM sysfs (`/sys/class/drm/cardN/device/hwmon`) plus `lspci`.
  - `intel` — DRM sysfs (i915) plus `lspci`/`glxinfo`.
- **`shared`** — `HardwareValue`/`HardwareData` types, key templates, and the
  sysfs/shell helpers shared by the vendor packages.

There is no HTTP listener, no port, and no template loading at request time. The
frontend is compiled into the binary, so a `make build` produces one
self-contained executable.

> An earlier revision of this document described the pre-Wails-v2 HTTP-server
> design (port 8080, templates read per request). That design no longer exists
> and those findings have been removed.

## Issues addressed

The following were fixed and are now covered by tests.

1. **Unchecked index access in CPU sensor parsing** (`cpuinfo/cpu.go`).
   `strings.Split(line, ":")[1]` panicked on any line lacking a colon.
   Parsing now goes through `firstFieldAfterColon` / `labelBeforeColon`, which
   return `""` for colon-less lines, and `extractTemp` bounds-checks its split.
   Regression tests: `TestParseSensorOutputMalformedDoesNotPanic`,
   `TestParseCpuInfoMalformedDoesNotPanic`.

2. **Duplicate helpers across vendor packages.** `runShell`, `findFirstSubpath`,
   `readSysFile`, `readSysValue`, `parseFrequencyValue` and `parsePowerValue`
   existed in two or three near-identical copies. They now live once in
   `shared/sysutil.go` (`RunShell`, `FindFirstSubpath`, `ReadSysFile`,
   `ReadSysValue`, `ParseFrequencyMHz`, `ParsePower`).

3. **Bogus zero readings for absent sensors** (`amd/amd.go`). Missing sysfs files
   were surfaced as `"0"` and then parsed successfully, so GPUs without a hotspot
   sensor, VRAM vendor file or power cap reported a literal `0`/`0.00`. Metrics
   are now omitted entirely when the underlying file is absent
   (`ReadSysFile`'s `ok` result).

4. **Malformed `Tccd` line produced an empty entry.** A `Tccd` line with no colon
   created `tccd0: ""`; empty extraction results are now discarded.

5. **`main_test.go` did not test max tracking.** The test asserted `maxValues`
   against the *min* expectation map, so a max/min mix-up would have passed. Min
   and max are now asserted separately, with coverage for divergent extremes and
   the 100-sample history cap.

6. **Placeholder `.deb` maintainer address.** `Makefile` now takes the control
   file's `Maintainer` from a `MAINTAINER` variable, overridable per build:
   `make package MAINTAINER="Your Name <you@example.org>"`.

7. **Only four CPU cores were ever displayed** (`main.go`). The static
   `cpuTemplates` list named `cpu.clock0`–`cpu.clock3`, so an 8-core CPU showed 4
   clock rows and a 16-core CPU still showed 4, even though the collector already
   emitted one clock per core. The clock and Tccd rows are now generated from the
   collected values by `cpuKeyTemplates`, sorted numerically so core 10 follows
   core 2. The ten now-dead `CPUClock0-3` / `TCcd0-5` constants were removed from
   `shared/templates.go` so they cannot be re-wired by mistake.

8. **CPU temperature never appeared on Intel CPUs.** The CPU temperature row
   binds to the `cpu.temp` key, but `coretemp` reports the chip-wide reading as
   `Package id N`, which the parser stored under a label-derived key
   (`cpu.Package id 0`) that no template referenced. So Intel users saw no CPU
   temperature at all, while AMD users (`Tctl`) did. `Package` rows now map to
   `cpu.temp`, and `parseCelsius` recovers the number from fields carrying
   threshold annotations (`+48.0°C  (high = +84.0°C, crit = +100.0°C)`), so
   readings are stored as floats and formatted like every other metric instead
   of being carried around as raw text such as `"        +47.0°C"`.

9. **Per-core temperatures were collected but discarded.** `coretemp` reports one
   reading per physical core, and these were stored under label-derived keys
   (`cpu.Core N`) that no template referenced — the same failure mode as the
   missing CPU temperature above. They now use canonical `cpu.coreTempN` keys,
   are generated into templates, and render in the per-core grid.

   SMT means there are more threads than physical cores (this host: 8 threads, 4
   sensors), so a thread cannot simply be given the sensor of the same number.
   This host reports thread 0,1 → core 0 and 2,3 → core 1, so index pairing would
   have shown thread 1 core 1's temperature. `parseCpuInfo` therefore buffers
   each `/proc/cpuinfo` block ("cpu MHz" precedes "core id" within a block) and
   emits `cpu.coreOfN`, giving the physical core each thread runs on. The UI
   resolves each thread's tile through that mapping, so sibling threads share
   their core's reading. Temperature cells are coloured using the same thresholds
   as the metrics table.

   Two fallbacks keep this honest rather than guessed:

   - With no topology keys (e.g. ARM `/proc/cpuinfo`), a temperature is folded
     into a clock cell only when there is exactly one sensor per thread.
   - Otherwise the sensors render in their own `Per-core temperatures` block, so
     a core's heat is never attributed to whichever thread shares its number.

10. **Clocks consumed a table row each.** With every core now reported, a
   32-thread CPU would have added 32 rows to the CPU card. `index.html` splits
   `CpuKeys` into clocks (`cpu.clockN`) and everything else: the metrics keep the
   table and the clocks render as a compact per-core grid
   (`buildCoreGrid`), each cell showing the core, its current clock, a bar and
   the observed min–max range. Bars are scaled against the highest clock any core
   has reached, so they show boost headroom rather than pinning one core at 100%.

## Test coverage

| Package    | Coverage |
|------------|----------|
| `cpuinfo`  | `/proc/cpuinfo` parsing, `sensors` output (Intel coretemp incl. the `Package`→`cpu.temp` mapping, AMD k10temp, TCCD cap, malformed input), Celsius parsing |
| `nvidia`   | CSV field parsing, unit stripping, `[N/A]` handling, column-to-metric contract |
| `amd`      | sysfs-mocked single/multi-card collection, GPU name cleaning and disambiguation |
| `intel`    | sysfs-mocked single/multi-card collection, PCI ID mapping, name cleaning |
| `main`     | min/max history, history window cap, dynamic per-core/per-CCD template generation |

The frontend has no automated tests. The per-core grids were verified by
rendering `index.html` against stubbed `GetData` payloads covering 32 threads, a
single core, a CPU reporting no clocks (non-x86), missing clock and temperature
values, absent min/max maps, and both the paired (4 threads / 4 sensors) and
unpaired (8 threads / 4 sensors) temperature layouts.

`go test ./...` runs without a GPU or lm-sensors installed; hardware collectors
are exercised by mocking `sysDir` (`amd`, `intel`) or by testing the pure parsing
layer (`cpuinfo`, `nvidia`).

## Remaining opportunities

These are known limitations rather than defects, listed roughly by value.

1. **Values are `map[string]interface{}` in `cpuinfo`.** The CPU collector builds
   untyped maps and converts to `HardwareValue` afterwards, while the GPU
   collectors build `HardwareValue` maps directly. Unifying on one representation
   would remove the conversion step and the risk of an untyped value reaching the
   frontend.

2. **CPU brand/model extraction is a crude heuristic.** `cpuinfo` splits the
   model name on the first space, so an Intel name yields brand `Intel(R)` and a
   model of `Core(TM) i7-4770 CPU`. Note that `GetInformations` overwrites
   `cpu.model` with the full name anyway.

3. **Probing runs a shell command per poll.** `Available()` / `probeNvidia()`
   shell out to `lspci` on every 5-second tick. The result is stable for the
   lifetime of the process and could be cached.

4. **Frontend thresholds are key-string based.** `gauge()` in `index.html`
   treats any value above 85 as "bad" for both temperature and usage, so VRAM and
   hotspot temperatures inherit CPU-style thresholds. Passing the metric type
   through from the backend would allow per-metric scales.

5. **The "Auto-refresh · 5s" badge is hardcoded** in the HTML while the interval
   lives in the polling code; the two can drift.

6. **Fan speed uses `HardwareValueType` `Frequency`.** `shared.RPM` exists as a
   unit, but the AMD/Intel collectors tag fan RPM as a `Frequency` value type,
   which means it participates in min/max display intended for clocks.
