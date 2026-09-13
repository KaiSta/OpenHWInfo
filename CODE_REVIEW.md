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
  - `mainboard` — DMI (`/sys/class/dmi/id`) plus the board's ACPI thermal zones
    and its hwmon monitoring chips (`nct67xx`, `it87xx`, `f718xx`, `w836xx`, …).
  - `storage` — `/sys/block` and `/sys/class/block` for drive identity, the
    drive's own hwmon chip (`nvme`, `drivetemp`) for temperature, and
    `/proc/mounts` plus `statfs` for how full its filesystems are.
- **`shared`** — `HardwareValue`/`HardwareData` types, key templates, the
  sysfs/shell helpers used by the vendor packages, and the DRM card scan
  (`ProbeDRMCards`, `DRMCardDevicePath`, `ResolvePCIDevice`).

Each GPU vendor package exposes a single collection entry point,
`GetAllInformations()`, which returns `nil` when the vendor is unavailable and
omits cards that expose no readable sensors. `main.go` appends the three results
and never probes a vendor twice.

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

11. **Every vendor was probed twice per tick** (`main.go`). `Available()` was
    called and then `GetAllInformations()`, and both ran the probe, so NVIDIA
    forked `sh -c 'lspci ...'` twice every 5 seconds (and AMD/Intel rescanned 16
    DRM paths twice). `GetAllInformations()` is now the only entry point, with
    `nil` meaning "unavailable"; the test-only `Available()` helpers the tests
    used as a redundant pre-assertion to `len(gpus) != 1` were removed.

12. **NVIDIA cards were detected via `lspci`, not `nvidia-smi`.** A machine with
    a card but no proprietary driver produced a phantom card whose entire
    content was a name plus the literal string `"NVIDIA Driver"`, because the
    index fallback returned `[]int{0}` and every metric query then failed.
    Availability now comes from `nvidia-smi` itself (`gpuIndices`).

13. **Cards with no readable sensors rendered as empty cards.** An integrated
    Intel GPU without a hwmon directory produced a card with only *GPU Name* and
    *Driver Version* rows. `HardwareData.HasTelemetry()` (which ignores the two
    identity-only fields) now filters these out, and the omission is logged.

14. **AMD's `lspci` fallback guessed a card index.** When the DRM scan found
    nothing but `lspci` mentioned AMD, the probe appended index `0` — which on a
    mixed system is typically *another* vendor's card (an Intel iGPU on this
    host), so its sensors would be read and labelled AMD. The fallback is gone:
    if a card's vendor file is unreadable, its hwmon directory is unreachable
    too, so a guessed index can only ever mislabel. A `ProbeDRMCards` test pins
    the vendor-matching behaviour on the Intel-0 / AMD-1 layout.

15. **The frontend rebuilt its DOM on every tick.** `render()` cleared the grid
    and recreated every node, so the `.bar { transition: width 0.4s }` rule was
    dead code — each new element was painted at its final width in the same
    frame. The vestigial `data-key`/`data-value`/`data-min`/`data-max`
    attributes were the fingerprint of the intended approach. Cards, rows and
    core cells are now reconciled in place (`reconcile`, `updateMetrics`,
    `updateClocks`, `updateTemps`): a card is rebuilt only when its shape
    changes (its metric keys, or temperatures moving between the tiles and their
    own block) and otherwise its text and bar widths are rewritten, which is
    what makes the bar transitions run.

16. **Temperature thresholds were key-string based and incomplete.** `gauge()`
    tested `/temp/i`, which missed `cpu.pch` and `tccdN` entirely (those rows
    showed no bar at all) and gave every sensor a CPU-style ceiling.
    `temperatureLimit()` now maps hotspot/junction sensors to 110, VRAM/memory to
    105, and everything else to 100.

17. **The `shared` package had no tests** despite becoming the home of all the
    parsing and DRM logic. `shared/sysutil_test.go` covers frequency/power
    conversion (including negative and unparsable input), `ReadSysFile`'s
    present/absent/zero distinction, `FindFirstSubpath`, and the DRM helpers.

18. **nvidia-smi was invoked once per GPU plus once to discover the GPUs.**
    `--query-gpu` already reports every card, so `index` is now appended as the
    last query column and a single call describes the whole system: the separate
    `--query-gpu=index` probe (and its `nvidia-smi -L` counting fallback) are
    gone, and the per-GPU `-i N` queries with them. Appending the column rather
    than prepending it keeps the metric column positions — and the contract test
    that pins them — intact. Parsing is now split into `parseGPUOutput`
    (multi-line response) and `parseGPURecord` (one card), so the column mapping
    is exercised by tests instead of by a copy of itself: the previous
    `TestFieldMappingMatchesQueryColumns` re-implemented the mapping and so only
    proved the test agreed with itself.

19. **The Intel GPU naming table was wrong for real hardware.** It was typed by
    hand and labelled at least three *integrated* device IDs as discrete Arc
    cards — `9A49` (Tiger Lake's Iris Xe) came out as "Intel Arc A380", and
    `9BC4`/`9BC6` (Comet Lake UHD Graphics) as "Arc B580"/"Arc B550" — while
    containing no valid Arc ID at all (Alchemist is `0x5690`–`0x56BF`). Seven
    further entries were IDs absent from `pci.ids`, two were host bridges rather
    than GPUs, and the Kaby Lake block was shifted by one, so `591C` was named
    UHD 630 instead of UHD 615. The table now lives in `intel/pci_ids.go`,
    generated from hwdata's `pci.ids` (2024.03.31) by keeping the bracketed
    marketing name of every 8086 graphics entry: Alchemist, Meteor Lake's
    integrated Arc and — from the newer pci.ids repository — Battlemage
    (`E20B` B580, `E20C` B570). Three Haswell/Ivy IDs that `pci.ids` names only by
    codename sit in a separate map so it stays visible which entries are not
    sourced. `TestIntelGraphicsModelsIntegrity` now enforces the entry shape and
    rejects any name containing "Arc" outside the DG2/Battlemage/Meteor Lake
    ranges.

20. **An unknown PCI ID produced a garbage card name.** The naming path queried
    `lspci` in numeric form (`-n`), whose output carries no product name, so a
    card missing from the table was displayed as the raw string
    `"00:02.0 0300: 8086:0416 (rev 06)"`. The device ID is per card and already
    read from sysfs, so the ID lookup needs no shell call at all, and the
    fallback now uses the *named* `lspci` form (and rejects identifier-only text
    before returning it). `looksLikeDeviceName` deliberately does not count
    words: `beef` is four letters and still a device ID.

21. **`getGPUName` could return an empty string.** Each fallback branch returned
    `parseIntelGPUName`'s result directly; once that can return `""`, a card
    would have been named with nothing at all. The branches now collect
    candidate strings and return the first that parses, ending at
    `"Intel Graphics"` — vague, but the card is known to be Intel, and a wrong
    model name would be worse.

22. **The last-resort naming query named the card after the whole device list.**
    `lspci | grep -i 'intel'` matches every Intel device on the machine, and its
    multi-line output was passed straight through as the GPU name — a 17-line
    dump of bridges, USB controllers and audio devices. The fallback queries are
    now filtered to display devices (`vga|3d|display`) and reduced to their first
    line. Found by the test added for the empty-name case above, which asserts a
    fallback name never looks like raw PCI identifiers.

23. **The Intel test's name assertion depended on the test host.** The mocked
    card had no `device` file, so the ID lookup always missed and the name came
    from whatever `lspci`/`glxinfo` reported on the machine running the tests —
    on the review host, a real Haswell iGPU. The fixture now writes a device ID
    and asserts the exact model name.

24. **Metric table rows stayed blank for one poll after any shape change**
    (`website/index.html`). `reconcile()` returned immediately after rebuilding a
    container, and `build()` only creates the *shell* of a row — the labels and
    values are written by the updater. A rebuilt table was therefore empty until
    the *next* tick, five seconds later, which affected the first paint as well
    as every layout change. Found while adding the mainboard card, whose rows
    change count when a board gains or loses a sensor. `reconcile` now always
    runs the updater over the (re)built children, which both fills a fresh table
    on the same pass and keeps the in-place update path — and therefore the bar
    transitions — intact. It had survived the previous review because every
    manual check looked at the page after at least one poll, while the defect is
    in the *first* render; the fix was verified by forcing a shape change and
    reading the DOM in the same evaluation.

25. **The mainboard was not reported at all.** `mainboard` is a new collector:
    the board's DMI identity (vendor, model, version, serial, BIOS version/date),
    the temperatures of its ACPI zones, and the temperatures and fan speeds of
    its monitoring chips. Three decisions are worth recording.
    - The ACPI zone list is an *allowlist* (`acpitz`, `pch*`, `TZnn`). A machine
      also publishes zones for its CPU package, its GPU, its wireless card and
      its disks, and putting those on a mainboard card would attribute a
      component's heat to the board.
    - The hwmon chip list is a prefix list of the drivers that sit *on the board*
      (`nct6`/`nct7`, `it8`, `f718`, `w836`, `asus`, `gigabyte`, …) rather than
      chip families that monitor other components. `coretemp`/`k10temp`,
      `amdgpu`/`nouveau`, `nvme`, `iwlwifi` and the laptop ECs are excluded.
    - The row labels are the sensors' own names (`SYSTIN`, `acpitz`,
      `Chassis Fan`), which only the collector knows. They travel in the values
      map as `board.tempLabelN`/`board.fanLabelN` so `main.go` can name the
      generated rows, with a documented rule and a contract test that they never
      become rows themselves — the same pattern as the CPU topology keys.
    An empty field produces no row (`board_serial` is root-only on most systems),
    a sensor file that cannot be read produces no row, and a board that reports
    nothing produces no card.

26. **Drives were not reported at all.** `storage` is a new collector: it lists
    one card per physical drive with its model, serial, firmware revision,
    medium, capacity, temperature and filesystem usage. The decisions worth
    recording:
    - A drive is identified by having a `model` string, which is what separates a
      real drive from the virtual devices sysfs lists beside it (`loop`, `zram`,
      `dm-*` and `md*` report none). Optical drives *do* carry a model, so they
      are excluded by device name instead.
    - The medium comes from the kernel's rotational flag rather than a guess, and
      an unreadable flag is reported as `Drive` — claiming an unknown device is
      an SSD would be a fabrication.
    - Temperatures are read only from an `nvme` or `drivetemp` chip found beneath
      the block device, so a sensor belonging to something else cannot be shown
      as a drive temperature. The row labels are the driver's own sensor names
      (`Composite`, `Sensor 1`), carried as hidden keys exactly like the
      mainboard's, with the same contract test.
    - Filesystem usage is aggregated per drive from `/proc/mounts` and `statfs`,
      and only for drives that are actually reported: a partition is mapped to
      its disk through sysfs (the `partition` attribute) *and* the derived parent
      is verified to exist, so a mount can never be charged to the wrong drive.
      LVM/LUKS volumes resolve to a `dm-*` device that is not a drive, so their
      usage is left out rather than attributed to the disk underneath.
    - Used space is derived from free *blocks*, not from the available count, so
      the 5% most filesystems reserve for root is not reported as used: on the
      review host 301.7 GB used + 175.6 GB available + 25.6 GB reserved = 502.9 GB.

27. **Three device cards had near-identical view-building code** (`main.go`).
    Adding drives would have made it a third copy of "filter the templates that
    carry a value, stringify the values, scope the min/max history per device".
    The three share one `DeviceView` type and one `deviceView()` helper that
    takes the history prefix (`gpu0.`, `disk1.`, `""` for the single mainboard),
    which also removed the hand-written prefix/remap closure the GPU path used
    and made the prefix scoping directly testable.

28. **Drives reported no health verdict.** Wear, defect counts and the drive's
    own SMART judgement were absent (item 15 below), so a card could show a
    drive at 40 °C with 90 % of its endurance gone and look identical to a new
    one. Each drive card now carries a health badge (`SMART: Good`, `Warning`,
    `Failing` or `Unknown`) with the reason as its tooltip, plus the readings
    behind it. The design is shaped by one constraint: **the data has to be
    available to an unprivileged process.**
    - The source is UDisks2 over D-Bus. udisksd runs privileged and already
      talks to the drive, so the app needs neither root nor an external tool.
      `smartctl` (installed on the review host, v7.4) fails as `Permission
      denied` for a normal user and `nvme-cli` is not installed at all, so
      neither is a usable source for an app the user launches as themselves.
    - Both command sets are read through the interfaces that carry them:
      `Drive.Ata` (failing flag, attributes below threshold, bad sectors,
      self-test, power-on seconds, and a temperature in Kelvin) and
      `NVMe.Controller` (critical warnings, power-on hours, and the attribute
      dictionary with spare capacity, percentage used, bytes written and media
      errors). The two are normalized into one `smartInfo` so the evaluation
      does not care which answered.
    - **Life is derived only from attributes whose meaning is unambiguous.**
      Vendors reuse attribute ids and normalize them differently, so the
      attribute *name* (which libatasmart derives from the drive's own IDENTIFY
      data, and which the UDisks2 documentation calls the authoritative
      identifier) is matched against an allowlist of three: `ssd-life-left`,
      `media-wearout-indicator` and `wear-leveling-count`. Each of these counts
      down from 100 as the media wears and stops at the drive's own fail
      threshold, so the consumed fraction is measured against that threshold —
      on the review host's SSD the wear attribute reads 96 against a threshold
      of 5, reported as 4.21 % used. `percent-lifetime-remain` is deliberately
      *not* interpreted: some drives put the wear figure in its raw value and
      leave the normalized value at 100, which would report a worn-out drive as
      barely used. An allowlist test pins the set and asserts that name stays
      out.
    - **Data written comes from both command sets, in the terabytes it is quoted
      in.** NVMe reports `total_data_written` in bytes; an ATA drive's
      `Total_LBAs_Written` (attribute 241) is read as the count of 512-byte
      sectors the standard defines it as, honoring libatasmart's unit when the
      report names a different byte unit. The attribute is matched by *id*
      rather than by name, because unlike the endurance attributes its meaning is
      the same everywhere — only its unit varies (item 21).
    - The verdict puts the drive's own judgement first — its failing flag, the
      number of attributes below their threshold, its critical warnings — and
      only then the app's reading of figures the drive has not faulted:
      endurance at or above 80 % (warning) or 95 % (failing), spare capacity
      below its threshold, bad sectors or media errors above zero, a failed
      self-test. A status a future kernel adds is surfaced as a warning rather
      than dropped, and an *empty* critical-warning array is the healthy state.
    - Past events are **notes**, not warnings: the review host's SSD reports
      `SmartNumAttributesFailedInThePast: 1` (its spare-block attribute dipped
      below threshold once) while reporting nothing currently failing, so
      calling the drive unwell would be crying wolf. Notes are listed in the
      tooltip and leave the badge green.
    - **Absent is never zero.** The property reader keeps "no reading" apart
      from "0" for every field, so a drive whose bad-sector count could not be
      read shows no bad-sector row instead of a clean `0.00`, and a drive whose
      `SmartUpdated` is 0 — where UDisks2 documents every other SMART property
      as meaningless — is reported as `Unknown` with no rows at all, rather than
      healthy. A drive with no SMART support gets no badge: the badge is never a
      guess.
    - Read cost is bounded. Properties refresh on a 10 s cache (cheap, served
      from udisksd), the attribute table on a 60 s cache because it is the one
      call that commands the drive, and both use UDisks2's `nowakeup` option so
      a sleeping drive answers with an error instead of being spun up. A drive
      whose SMART has never been read is asked **once** per process, also with
      `nowakeup`.
    - `shared.DeviceStatus` carries the level (`good`/`warn`/`bad`/`unknown`,
      the vocabulary the bar styles already use), the badge text and the reason,
      so the frontend needed one pill and one in-place updater rather than any
      device-specific code.

29. **The temperature fallback and SMART were two independent D-Bus reads.**
    The SATA temperature fallback queried `SmartTemperature` on its own schedule
    with its own cache, next to the SMART read that fetches the same property as
    part of a `GetAll`. The temperature now comes out of the cached SMART
    reading, so one round trip per drive serves both and the duplicated cache,
    provider and property-marshalling code (about 110 lines) is gone. The
    behaviour is unchanged: hwmon wins when it is available, and a drive that
    reports no temperature still produces no row.

30. **"Terabytes written" was not reported for an ATA drive.** The written row
    existed for NVMe only, and in gigabytes, so a SATA SSD showed nothing at all
    and an NVMe figure read `10337.00` instead of `10.34`. The row is now
    `Data Written (TB)`, in the unit total writes are quoted in (the TBW of an
    endurance rating), and it is produced from either command set: an NVMe
    controller's `total_data_written` in bytes, or an ATA drive's attribute 241.
    The ATA count is converted with the unit libatasmart reports for it — sectors
    (the ATA definition, which is also how a bare count is read) or megabytes —
    while a unit that is not a byte count yields no row rather than a converted
    figure, and a counter the drive reports as zero yields none either: "none"
    and "nothing" are indistinguishable, and `0.00 TB` would be the app's claim
    rather than the drive's. The row is driven by the drive, so the review host's
    own SSD still shows no `Data Written` row — its SMART table (23 attributes,
    read through both `busctl` and the app) contains no attribute 241.

## Repo hygiene

- `go.mod` and `go.sum` were listed in `.gitignore` even though both are tracked.
  They are required for a reproducible build, so the entries were removed with a
  note explaining why.
- The built `./OpenHWInfo` binary was **tracked**, so every `make build` dirtied
  the tree and a 12 MB artifact churned in history. It is now untracked (the
  deletion is staged) and ignored via `/OpenHWInfo`, leaving `make build` free to
  overwrite it.
- `wails.json`'s `outputBin` points into the ignored `build/` directory, so a
  `wails build` cannot dirty the tree either. `make build` remains the supported
  build path.

## Test coverage

| Package    | Coverage |
|------------|----------|
| `cpuinfo`  | `/proc/cpuinfo` parsing, `sensors` output (Intel coretemp incl. the `Package`→`cpu.temp` mapping, AMD k10temp, TCCD cap, malformed input), Celsius parsing |
| `nvidia`   | CSV field parsing, unit stripping, `[N/A]` handling, multi-row output splitting, column-to-metric contract driven through the real parser, name-with-comma alignment, truncated rows |
| `amd`      | sysfs-mocked single/multi-card collection, GPU name cleaning and disambiguation |
| `intel`    | sysfs-mocked single/multi-card collection with an exact name assertion, PCI ID mapping (Arc incl. Battlemage, integrated GPUs must not be named Arc, non-GPU IDs resolve to nothing), table integrity rules, name cleaning, rejection of identifier-only input, and a fallback that always yields a usable name |
| `mainboard`| sysfs-mocked DMI identity (including a blank/root-only field producing no row), board-zone selection and millidegree conversion, Super I/O temperatures and fans with and without driver labels, chip-name allowlist, sensor caps, unparsable readings |
| `storage`  | sysfs-mocked drive listing (SATA SSD, NVMe, HDD, and the virtual/optical devices that must not appear), medium and capacity from the rotational flag and sector count, NVMe labels vs a drivetemp chip, rejection of a non-drive hwmon chip, partition-to-disk mapping with a mount-table fixture and a stubbed `statfs` (aggregation, duplicate mounts, an unmappable `dm-0`, an escaped space, a failing lookup, a missing mount table); SMART: every health rule as its own case (a healthy drive with exactly one reading broken), Kelvin and wire-type decoding for both interfaces, the life-attribute allowlist and its threshold scaling, the fabrication guards (absent bad-sector count, never-read drive, empty NVMe dictionary), every row's presence *and* unit, and the row→template contract |
| `shared`   | frequency/power conversion, `ReadSysFile` absence vs zero, glob lookup, DRM vendor-matched card scan, PCI address resolution |
| `main`     | min/max history, history window cap, dynamic per-core/per-CCD, per-board-row and per-drive template generation, per-device history scoping, and the collector→frontend key contract (every rendered key has a template; topology and label keys never become rows) |

The frontend has no automated tests, and this environment has no JavaScript
runtime to add one with (`node`/`npm` are absent, and the app ships a static page
with no build step), so its guarantees are documented rather than enforced. The
Go-side contract test above covers the *key* half of that boundary — the half
that produced two real bugs. The rendering itself was verified by previewing
`index.html` against stubbed `GetData` payloads covering:

- 32 threads and a single core; a CPU reporting no clocks (non-x86); missing
  clock and temperature values; absent min/max maps.
- the `paired` (topology keys present, 8 threads / 4 sensors), `index` (no
  topology, one sensor per thread) and `block` (no topology, fewer sensors than
  threads) temperature layouts, including that a missing sensor omits just that
  tile's temperature rather than shifting the others.
- in-place updating: after a second tick with changed readings, the card, row,
  bar and temperature nodes are the *same* objects (so the bar transition can
  run) while their values are rewritten; a layout change rebuilds instead — and a
  rebuilt table is populated on the same pass (item 24).
- the mainboard card: a desktop-shaped board (ACPI zones, four labelled sensors,
  two fan headers) rendering identity rows without bars, temperatures with them,
  and fans with min/max but no scale; a laptop-shaped board with one `acpitz`
  row; an identity-only board; and `Board: null` producing no card at all, then
  the card reappearing when a board is reported again.
- the drive cards: an NVMe SSD with three labelled sensors and a filesystem bar
  scaled against its own total (77 % of 1863 GB), a SATA SSD with no temperature
  and no min/max history yet, an HDD (head reads "HDD"), a drive whose mount
  could not be measured (identity and temperature rows only, no zero-size rows),
  and a machine with no drives at all (no card).
- the health badge and the SMART rows: all four levels rendered (`Good` green,
  `Warning` amber, `Failing` red, `Unknown` muted), the tooltip carrying the
  reason; an NVMe-shaped drive with every row present and correct bar
  directions (life rising into red, spare falling into red — the two percentage
  readings that are better the *lower* and the *higher* they are), a SATA-shaped
  drive with no NVMe-only rows, a drive with no verdict (no badge), and two
  drives each keeping their own badge. A verdict change is written onto the
  existing pill: after a tick that turns `Good` into `Failing`, the card, pill,
  row and bar are the **same DOM nodes** while the pill's class, text and
  tooltip and the bar's width and colour are rewritten — so the verdict updates
  without a rebuild, and the bar transition survives it.

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

3. **The device scan still runs on every poll.** `GetAllInformations()` probes
   once per tick (one `nvidia-smi` call, one `sensors` call), which is correct
   but not free, and the device set is stable for the lifetime of the process —
   a device appearing later is the only case a cache would miss. Caching the
   probes with a short TTL (or checking for new cards on a slower cadence) would
   remove the remaining forks; on the review host one `nvidia-smi` query costs
   ~15 ms, so the win is modest but real.

4. **Thresholds are still key-string based.** `temperatureLimit()` and `gauge()`
   match on key substrings and the tables are hardcoded in the frontend, so a
   sensor whose key does not match a pattern gets no scale at all. Passing the
   value type and a scale (or a limit) through from the backend would let the
   collectors own this. The drive health work added two more such branches —
   `/life/` and `/spare/`, the two percentages that are better the lower and the
   higher they are — and a third direction if a `Wear Leveling Count` row is ever
   shown as remaining life rather than consumed life.

5. **The "Auto-refresh · 5s" badge is hardcoded** in the HTML while the interval
   lives in the polling code; the two can drift.

6. **Fan speed uses `HardwareValueType` `Frequency`.** `shared.RPM` exists as a
   unit, but the AMD/Intel collectors tag fan RPM as a `Frequency` value type,
   which means it participates in min/max display intended for clocks.

7. **Min/max history is never pruned.** `history` is capped at 100 samples *per
   key*, but keys are never removed, so a machine whose metric keys change (a
   different CPU, an eGPU appearing) keeps tracking ranges for hardware that is
   no longer present. Pruning by "not in the current values" was considered and
   deliberately not implemented: a card that momentarily fails to report would
   then lose its accumulated range and start over, which is a worse outcome than
   a few hundred stale floats.

8. **Intel Arc is detected but barely measured.** Arc is found (vendor `0x8086`)
   and named correctly now, and its hwmon device exists under both the `i915` and
   `xe` drivers, so temperature, power and (Battlemage, kernel ≥ 6.16) fan speed
   would be picked up. The gaps are the discrete-specific parts: nothing reads
   Arc's dedicated VRAM (`mem_info_vram_*`), GPU usage is not read for Intel at
   all, and `temp2_input` is labelled *GPU Hotspot* unconditionally while the
   `xe` hwmon driver exposes several labelled channels (package, VRAM, memory
   controller, PCIe) — those labels (`tempN_label`) are ignored. Frequency is
   also read from i915's hwmon `freq*_input`, which the `xe` driver does not
   appear to provide. None of this can be verified without Arc hardware, but it
   is testable with sysfs fixtures the way `amd`/`intel` already are.

9. **The Intel driver-version row can never resolve** (found while auditing Arc
   support). `getDriverVersion()` reads `/sys/module/i915/version`, then
   `modinfo i915 | awk '/^version:/'`. Neither exists: the kernel's `i915`
   module sets no `MODULE_VERSION`, so the attribute is absent and `modinfo`
   reports only `srcversion`. On the review host the row therefore shows the
   literal placeholder `"Intel Driver"`, and `xe` behaves the same way. The test
   passes only because its fixture *creates* `/sys/module/i915/version`, a file
   no running kernel provides — a good example of a test that encodes an
   assumption instead of the system.

10. **The frontend is untested.** Every UI guarantee above rests on manual
   preview runs. A small DOM harness (the ones used during review were deleted so
   they cannot be embedded by `//go:embed all:website`) plus a headless runner
   would turn those checks into regressions.

11. **Per-core topology assumes `coretemp`'s `Core N` matches `/proc/cpuinfo`'s
   `core id N`.** This holds on the hosts checked, and it is the usual
   convention, but the mapping is inferred rather than read from a single source.
   If a kernel numbered them differently, a tile would show a sibling core's
   temperature rather than none.

12. **The mainboard card has never run against a Super I/O chip.** The review
   host is a laptop whose hwmon devices are `acpitz`, `coretemp`, `iwlwifi`,
   `ACAD` and `BAT1` — no board monitoring chip — so `acpitz` is the only live
   temperature. The Super I/O path (labels, fan headers, the chip allowlist) is
   covered by sysfs fixtures, which is how `amd`/`intel` are tested too, but the
   desktop case is unverified against real hardware.

13. **Board monitoring chips that are not Super I/O are only partly covered.**
   `asus` and `gigabyte` WMI sensors are matched, but laptop ECs (`thinkpad`,
   `dell_smm`, `applesmc`) are deliberately excluded: they report CPU and GPU
   readings that the CPU/GPU cards already show, so including them would
   duplicate values under a board heading. A machine whose only board sensor is
   an EC therefore gets an identity-only card.

14. **Super I/O voltages are not read.** The same chips expose `inN_input`
   (rail voltages) next to the temperatures and fan speeds. They are omitted
   because the card has no sensible scale for them and a dozen rails would
   dominate it; adding them behind a separate card (or a fixed 12 V/5 V/3.3 V
   scale) would be the way to do it.

15. **Drive health needs UDisks2.** SMART is read through udisksd, which is the
   part that makes it work without root (item 28). A system without udisks2 — a
   minimal server install, a container — therefore gets no badge and no SMART
   rows, and there is no fallback: `smartctl` returns `Permission denied` for a
   normal user, and a root-only fallback would need a password prompt on every
   poll. A helper that asks for elevation once at startup, or a small setuid
   reader, would be the way to cover those systems.

16. **A SATA drive's temperature needs the `drivetemp` module.** Without it there
   is no hwmon device under the block device at all, so the drive card shows no
   temperature — the correct outcome, but one that depends on a module the user
   may not have loaded. The review host has the module available and not loaded,
   so the drivetemp path is fixture-tested only, like the desktop Super I/O path.

17. **Filesystem usage behind LVM, LUKS or RAID is not attributed.** The mount
   resolves to `dm-0`/`md0`, which is a block device but not a drive, so the card
   shows no usage rows for it. Walking `/sys/block/dm-0/slaves` to the physical
   drive would attribute it, at the cost of handling multi-disk volumes (a RAID 5
   has no single right answer).

18. **The mainboard card has no chassis or system information.** `chassis_type`
   (desktop/notebook/rack), `sys_vendor` and `product_name` sit in the same DMI
   directory and would answer "what machine is this?" on systems where
   `board_name` is an OEM code rather than a marketing name — on the review
   laptop, `board_name` is the friendly `Lenovo Y50-70` while `product_name` is
   the machine type `20378`.

19. **The NVMe path has never run against a real NVMe drive.** The review host's
   only drive is a SATA SSD (`/dev/sda`, no `/dev/nvme*` and no `nvme-cli`), so
   the ATA path is verified live end to end — 23 attributes decoded, the wear
   attribute read as 4.21 % used, `SMART: Good` on the card — while the
   `NVMe.Controller` path is covered by unit tests that build its replies in the
   documented wire shapes (byte percentages, uint64 byte counts, Kelvin
   temperatures, the critical-warning array). The two interfaces differ in
   exactly those types, which is what the decoding tests pin, but a real
   controller could still differ in a way the tests do not anticipate.

20. **Which attribute carries endurance varies by vendor, and only three names
   are interpreted.** The allowlist covers Sandisk/WD/Kingston, Intel/Samsung
   and Samsung's wear-leveling attribute, which is the large majority of SSDs,
   but a drive using another name or another convention (Crucial/Micron's
   `percent-lifetime-remain` is the notable one, ambiguous because some drives
   put the wear figure in the raw value and leave the normalized value at 100)
   shows no `Life Used` row at all rather than a wrong one. Extending the list
   means confirming the direction of the normalized value per vendor, which
   `smartctl`'s `drivedb` files are the reference for.

21. **An ATA drive's written counter is read in the wrong unit on some models**
   (item 30). ATA defines attribute 241 as a count of 512-byte sectors, which is
   how it is read here, but several vendors — the smartmontools drive database
   names SanDisk and WD families, and its ticket tracker has drives whose output
   is annotated "(32MB Units)" — count in 32 MiB units instead. Nothing in the
   reading distinguishes the two conventions, and the error is in the invisible
   direction: the coarser convention read as sectors *under*-reports by 65536×
   (200 GB becomes 3.1 MB, i.e. `0.00 TB`), so no plausibility check against the
   drive's capacity or power-on hours can catch it — a figure that is too small
   is also always physically possible. Configuring the unit per model, the way
   `smartctl`'s `drivedb` does, is the only reliable fix and needs a table this
   project has no way to verify without the drives; the app's own host reports no
   241 at all, so even the common case could not be checked live here.
