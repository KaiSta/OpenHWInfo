# OpenHWInfo — Linux hardware monitor for CPU, GPU, memory, storage and battery

<img src="icons/icon-512.png" width="112" align="right" alt="OpenHWInfo icon — a Linux hardware monitor for CPU, GPU, memory, storage and battery">

OpenHWInfo is a native **Linux hardware monitor** that shows what your machine is doing right now: CPU temperature and per-core clocks, GPU temperature and usage, VRAM, fan speeds, SSD and HDD health, mainboard sensors, RAM and swap, and laptop battery wear — all in one window, refreshed every five seconds.

It reads the kernel's own interfaces directly — `/proc`, `/sys`, hwmon and UDisks2 over D-Bus — instead of parsing the output of `lm-sensors`, `smartctl` or `nvme-cli`. There is nothing to install alongside it, nothing to configure, and nothing to run as root. The Go backend and the web frontend are embedded in one self-contained executable.

## Highlights

* **Everything in one window** — CPU, GPU (NVIDIA, AMD, Intel and Lisuan), drives, mainboard, memory and battery, each on its own card.
* **No root, no helper tools, no configuration.** No `lm-sensors`, `smartctl`, `nvme-cli` or background service of its own; the readings come from the kernel and from the UDisks2 service your desktop already runs.
* **It never shows a zero for a reading it could not take.** A missing sensor produces no row rather than a fake `0`, which is the rule the whole app is built around.
* **Health verdicts, not just numbers** — `SMART: Good` / `Warning` / `Failing` per drive and `Health: Good` / `Worn` / `Poor` per battery, with the figures behind the verdict in the tooltip.
* **Quiet on the machine it monitors.** Identities are resolved once, readings are cached, and SMART is read passively so a sleeping drive is not spun up.
* **No network listener and no telemetry.** Every reading comes from the machine it runs on.

## Features

### CPU

* Vendor, brand and model name, and one clock per logical processor.
* CPU (package) temperature, chipset/PCH temperature, per-core temperatures, and AMD per-CCD (TCCD) temperatures where the processor reports them.
* Per-core temperatures are matched to the thread that runs on that core, so both threads of an SMT core show that core's reading.
* A compact per-core grid: current clock, a bar scaled against the highest clock reached, and the observed min–max range.

### GPU

* **NVIDIA** through `nvidia-smi`: core and memory temperature, VRAM used and total, power draw and power limit, clocks, fan speed, utilisation and driver version.
* **AMD** through the `amdgpu` sysfs interface: temperature and hotspot/junction temperature, VRAM used and total, GPU clock, average power draw and cap, fan speed and GPU utilisation.
* **Intel** through the `i915` sysfs interface: the card is named from a PCI ID table, with driver version, temperature, hotspot, clock, power draw and fan speed where the driver publishes them.
* **Lisuan** 7G100 series cards: name from a PCI ID table, driver version, temperature and any hotspot/VRAM sensors the driver labels as such, clock, power draw, power cap and fan speed.
* VRAM rows are omitted on integrated graphics, where memory is shared with the system; a sensor the card does not expose produces no row, and a card with no readable sensors is not drawn at all.

### Storage and drive health

* One card per drive: model, serial number, firmware revision, medium (`SSD`, `SSD (NVMe)`, `HDD`), capacity in decimal GB, temperature, and filesystem used/free space.
* **SMART health badge** per drive, read from UDisks2 — no root and no `smartctl` needed.
* Life used, power-on time, bad sectors, self-test result, spare capacity (NVMe), media errors (NVMe) and lifetime data written in TBW.
* Reads are passive: the attribute table is queried at most once a minute and with UDisks2's `nowakeup` option, so a sleeping drive answers with an error instead of being spun up.

### Mainboard

* Board vendor, model, version and serial number, plus BIOS version and date from DMI/SMBIOS.
* Board temperatures from the ACPI thermal zones (`acpitz`, chipset zones).
* Temperature and fan-header readings from the board's Super I/O chips (Nuvoton, ITE, Fintek, Winbond) and ASUS/Gigabyte WMI sensors, labelled with the driver's own names such as `SYSTIN` or `Chassis Fan Speed`.

### Memory

* RAM total, used and available in binary GiB, with usage percentage.
* Swap total, used and usage on machines that have swap configured.
* Memory pressure (`some` and `full`) from the kernel's PSI accounting.
* zram details: compressor, uncompressed size, usage, the RAM the compressed store actually costs, compression ratio and device count.

### Battery

* One card per battery: status, charge percentage, energy in Wh or charge in Ah, health, power draw, voltage, charge cycles, time to empty or full, AC adapter state, and model/manufacturer/serial/technology.
* **Health badge** (`Good` / `Worn` / `Poor`) computed from what the pack holds when full against its design capacity, with the figures in the tooltip.
* Multiple packs are each reported separately; an empty battery bay and the battery inside a wireless mouse or headset are not reported.

### The window itself

* Refreshes every five seconds; min and max columns keep the observed range over the last 100 samples (about eight minutes).
* Every value carries its unit next to it, so a figure and its label cannot disagree.
* Colour-coded temperature bars, health badges with reasons, and no cards for hardware the machine does not have.

## Installation

### Debian and Ubuntu (recommended)

Download the package for your architecture from the [Releases page](https://github.com/KaiSta/OpenHWInfo/releases) — `amd64` for Intel and AMD machines, `arm64` for ARM — and install it:

```bash
sudo apt install ./openhwinfo_0.1.0_amd64.deb
```

`apt` is used rather than `dpkg -i` so the GTK 3 and WebKitGTK 4.1 runtime packages are pulled in automatically. Afterwards, start **OpenHWInfo** from your application menu, or run `openhwinfo` in a terminal.

### Build from source

Prerequisites on Debian/Ubuntu: `sudo apt install libwebkit2gtk-4.1-dev build-essential`

```bash
make run        # builds + runs the native app
make build      # just builds → ./OpenHWInfo
make dev        # development mode with hot-reload (serves frontend from disk)
make test       # run all tests
make package    # build a .deb installer into dist/
make install    # desktop entry + icons for the current user
make clean      # remove binary and dist/
```

The `production` tag is required for Wails to create the native window; `webkit2_41` selects WebKitGTK 4.1 (on an older distribution, install `libwebkit2gtk-4.0-dev` and drop that tag). The frontend in `website/` is embedded into the binary, so a single self-contained executable is produced.

> **Note:** running `go build .` without tags fails with "Wails applications will not build without the correct build tags". Use `make build`, or add `-tags=production,webkit2_41`.

## Requirements

* Linux with GTK 3 and WebKitGTK 4.1 — `libwebkit2gtk-4.1-0` on Debian and Ubuntu, which the `.deb` installs for you.
* No root and no extra sensor packages: `lm-sensors`, `smartctl` and `nvme-cli` are not needed.
* For SATA drive temperatures, the kernel's `drivetemp` module has to be loaded; NVMe drives report their own. Without it the temperature row is absent, not zero.
* For SMART readings, UDisks2 (`udisksd`) has to be running — the service a desktop installation starts anyway.

## Usage

Start **OpenHWInfo** from the application menu or run `openhwinfo`. Each card is one piece of hardware; each row is one reading, with its unit and its observed min/max range.

`openhwinfo -debug` (or `./OpenHWInfo -debug` from a build) writes what each collector found on every poll — the DRM cards, drives and sensor chips, and why a device was skipped. Normal runs print nothing, so a genuine warning stays visible when it appears.

### Reading the output

Each metric is a key/value pair with a unit and a value type. Min and max track the observed range over the last 100 samples (~8 minutes at the 5 s refresh).

Per-core clocks render as a compact grid rather than one table row per core, so a 32-thread CPU stays readable. Each tile shows the current clock, a bar scaled against the highest clock any core has reached (so boost headroom is visible), and the observed min–max range. When core temperatures can be attributed to a thread, they appear in the same tile; otherwise they render in their own block. See [CPU readings](#cpu-readings).

Cards, rows and tiles are reconciled in place rather than rebuilt, which is what keeps the bar transitions and the scroll position stable while the numbers change.

## Where the readings come from

| Card | Source |
|------|--------|
| CPU | `/proc/cpuinfo`, and the processor's own hwmon chips (`coretemp`, `k10temp`, `zenpower`, `cpu_thermal`) |
| GPU — NVIDIA | `nvidia-smi --query-gpu=... --format=csv,noheader` |
| GPU — AMD | `/sys/class/drm/cardN/device` (vendor `0x1002`) plus hwmon, named through `lspci` |
| GPU — Intel | `/sys/class/drm/cardN/device` (vendor `0x8086`) plus hwmon, named from a PCI ID table with an `lspci`/`glxinfo` fallback |
| GPU — Lisuan | `/sys/class/drm/cardN/device` (vendor `0x4c54`) plus hwmon and the bound driver module |
| Storage | `/sys/block` and `/sys/class/block`, `/proc/mounts` + `statfs`, the drive's `nvme`/`drivetemp` hwmon chip, and UDisks2 over D-Bus for SMART |
| Mainboard | `/sys/class/dmi/id`, `/sys/class/thermal/thermal_zone*`, and the board's Super I/O hwmon chips |
| Memory | `/proc/meminfo`, `/proc/pressure/memory`, `/sys/block/zramN` |
| Battery | `/sys/class/power_supply` |

## CPU readings

`/proc/cpuinfo`

* Vendor ID, brand and model name
* One clock per logical processor
* The physical core each logical processor runs on, used to attribute per-core temperatures correctly on SMT systems

The processor's own hwmon chips (`/sys/class/hwmon/hwmon*`)

* CPU temperature — `cpu.temp`, from Intel `coretemp`'s `Package id N` or AMD
  `k10temp`'s `Tctl`
* PCH (chipset) temperature, from the PCH chip's first sensor
* Per-core temperatures — `cpu.coreTempN`, from Intel `coretemp`'s `Core N` rows
* TCCD (per-CCD) temperatures where reported — `tccdN`, from AMD `k10temp`

Only the chips that belong to the processor are read, and the readings come
straight from their `tempN_input`/`tempN_label` attributes. lm-sensors does not
have to be installed, and nothing is forked on each poll. A driver that supplies
no labels at all (older `k10temp`, ARM's `cpu_thermal`) is read as a single
package reading, and a chip whose readings cannot be parsed produces no row
rather than a zero.

A core's row is keyed by the number in its **own** label (`Core 7` →
`cpu.coreTemp7`), which is the same numbering `/proc/cpuinfo` reports as
`core id` and therefore the one a thread's temperature is looked up with. The
tile for a thread shows its core's reading, so on an SMT CPU both threads of a
core display that core's temperature. A label that names no number produces no
row rather than an invented one.

When a core number cannot identify a single core, per-core rows are left out
entirely and only the package temperature is shown: that happens on a machine
with more than one processor package (each package numbers its cores from 0
again, and sysfs does not say which `coretemp` chip monitors which package) and
on a machine whose processor sensors are spread over more than one chip. The
package figure is then the **hottest** package's reading. A mislabelled core
would be worse than an absent row, so nothing is guessed.

## GPU readings

### NVIDIA

`nvidia-smi --query-gpu=... --format=csv,noheader`

One call per refresh covers every card, since `--query-gpu` already reports all of them (the card `index` is part of the same query). Reported metrics:

* GPU name and driver version
* Temperature (core and memory)
* Current and max power draw
* RAM current/max
* Utilization, clocks and fan speed

A card is only reported when `nvidia-smi` can actually query it. A GPU that is visible in `lspci` but has no proprietary driver installed is omitted rather than rendered as an empty card, because every metric query would fail. Fields the driver reports as `[N/A]` (a missing fan sensor, for instance) are omitted too, rather than shown as a zero.

https://nvidia.custhelp.com/app/answers/detail/a_id/3751/~/useful-nvidia-smi-queries
https://developer.download.nvidia.com/compute/DCGM/docs/nvidia-smi-367.38.pdf

### AMD

`lspci` + `/sys/class/drm/cardN/device` (PCI vendor ID `0x1002`) + hwmon

* Name (PCI-address-scoped `lspci` query, disambiguated when several identical cards are present). A card's name cannot change while the app runs, so it is resolved once instead of forking `lspci` on every poll.
* Temperature, hotspot/junction temperature where available (`temp1_input` / `temp2_input` / `temp3_input`)
* VRAM used/total/vendor (`mem_info_vram_*`)
* GPU clock (`freq1_input` / `freq2_input`)
* Average power draw and power cap (`power1_average`, `power1_cap_max`)
* Fan speed (`fan1_input`) and GPU usage (`gpu_busy_percent`)

### Intel

DRM sysfs `/sys/class/drm/cardN/device` (PCI vendor ID `0x8086`) + hwmon

* Name — the PCI device ID from sysfs is mapped to Intel's marketing name using
  a table generated from the [pci.ids](https://pci-ids.ucw.cz/) database
  (`intel/pci_ids.go`), so no `lspci`/`glxinfo` call is needed to name a card.
  Alchemist (`0x5690`–`0x56BF`), Battlemage (`0xE20B`/`0xE20C`) and Meteor Lake's
  integrated Arc Graphics (`0x7D55`) are covered, as are the integrated
  generations from Sandy Bridge onward. If the ID is unknown the name comes from
  a PCI-address-scoped `lspci` query, then `glxinfo`, then `Intel Graphics`.
* Driver version (`/sys/module/i915/version`, with a `modinfo` fallback). Read once per process, like the GPU name.
* Temperature (`temp1_input`), hotspot temp if available (`temp2_input`)
* GPU clock (`freq0_input` / `freq1_input`)
* Average power draw (`power1_average`)
* Fan speed (`fan1_input`) if available

For **integrated** graphics, VRAM metrics are not reported because they share
system memory. Discrete Arc cards are detected and named, but their dedicated
VRAM is not read yet — see the Arc item under *Remaining opportunities* in
`CODE_REVIEW.md` for the current gaps.

### Lisuan

DRM sysfs `/sys/class/drm/cardN/device` (PCI vendor ID `0x4c54`, the vendor Lisuan Technology registered for the 7G100 series) + the card's hwmon directory + its bound driver module

* Name — from the PCI device ID (`0x5000`–`0x5006`) through a table built out of the PCI ID database (`lisuan/pci_ids.go`). There is deliberately no `lspci` fallback, unlike the AMD and Intel collectors: the vendor database most distributions ship has no entry for `0x4c54` yet, so a query can only hand the identifiers back, and an identifier is not a card name. A device ID the table does not know is reported as `Lisuan GPU` rather than guessed at.
* Driver version — read from the module the card is actually bound to, discovered through the card's own `driver` and `module` sysfs links rather than a hardcoded module name, since the Lisuan driver's name is not documented publicly.
* Temperature — the card's own sensor, with the hotspot and video-memory rows filled from whichever sensors the driver *labels* as such (`hotspot`, `junction`, `mem`, `vram`, `hbm`). Nothing standard fixes a driver's sensor order, so an unlabelled sensor is reported as the card's own reading rather than moved into a row that would claim the number means something else.
* GPU clock (`freq1_input`, or `freq0_input` on a driver publishing one channel), average power draw and power cap, and fan speed.

A sensor the driver does not publish produces no row, and a card whose driver publishes nothing but its identity is not drawn at all — the `-debug` log is what says so. **This collector has not been checked against a real Lisuan card**, and cannot be: no machine here has one, and the driver's sysfs surface is not publicly documented. It therefore reads only what the kernel itself defines — the DRM card node, the hwmon interface, and the module's `version` — and derives nothing from another vendor's interface. VRAM size, VRAM in use and GPU utilisation are absent for exactly that reason: those attribute names belong to amdgpu's interface, and reading them here would be a guess presented as support. See the Lisuan item under *Remaining opportunities* in `CODE_REVIEW.md` for what would close the gap.

### Common behaviour

A sensor file that does not exist produces **no metric**, rather than a zero reading — a card with no hotspot sensor or no power cap simply omits those rows. Similarly, a card that exposes no readable sensors at all (for example an integrated GPU whose hwmon directory is absent) is not rendered as an empty card.

A machine that reports no GPU whatsoever — a virtual machine, a headless host, or a card whose driver exposes nothing — gets no GPU card either, exactly as the memory and mainboard cards are withheld when the machine reports nothing for them. The card is left out rather than drawn as a title over an empty table, because that table would state nothing the machine said. `./OpenHWInfo -debug` is what tells the two cases apart: it logs whether the collectors found a card and why one was skipped, since a card that is simply absent leaves no other trace.

## Storage readings

Sysfs (`/sys/block`, `/sys/class/block`) — one card per drive, no root required

* Model, serial number and firmware revision (NVMe reports `firmware_rev`, SATA/SCSI reports `rev`)
* Medium: `SSD`, `SSD (NVMe)` or `HDD`, taken from the kernel's rotational flag. A drive whose flag cannot be read is reported as `Drive` rather than assumed to be an SSD.
* Capacity in decimal GB — how drives are sold, so a 512 GB SSD reads 512.11, not the 476.9 a binary unit would give
* Temperatures from the drive's own hwmon device: an NVMe drive's `nvme` chip (the composite sensor plus one row per sensor, using the labels the driver provides) or a SATA drive's `drivetemp` chip. A SATA drive's temperature needs the kernel's `drivetemp` module loaded; without it the row is simply absent rather than shown as a zero.
* Filesystem Total / Used / Free, aggregated over the filesystems mounted from that drive (`/proc/mounts` plus `statfs`). Only mounted filesystems are counted, so these are labelled as filesystem figures rather than drive figures, and a drive that is not mounted shows none of them.

A mount whose device cannot be resolved to a whole disk in sysfs — an LVM or LUKS volume, for instance — contributes no usage rows at all: reporting one drive's space against another would be worse than reporting nothing. Virtual devices (`loop`, `zram`, `dm-*`, `md*`) and optical drives are never reported as drives.

### Drive health (SMART)

Every drive card carries a **health badge** beside its title — `SMART: Good`, `SMART: Warning` or `SMART: Failing` — with the reason in its tooltip. A drive that supports SMART but has not been read yet is shown as `SMART: Unknown` rather than assumed healthy, and a drive with no SMART at all gets no badge, so the badge is never a guess.

The readings come from UDisks2 over D-Bus: udisksd runs privileged and already talks to the drive, so **no root and no external tool is needed** — `smartctl` fails with `Permission denied` for a normal user and `nvme-cli` may not be installed. Both command sets are understood, `org.freedesktop.UDisks2.Drive.Ata` and `org.freedesktop.UDisks2.NVMe.Controller`, and their readings are shown under the same row names:

* **Life Used** — endurance consumed. An NVMe controller reports this itself; on an ATA drive it is derived from the drive's wear attribute (normalized to 100 when new and to the drive's own fail threshold at end of life, so the fraction is measured against that threshold).
* **Power-On Time**, **Bad Sectors** (ATA: pending plus reallocated), **SMART Self-Test**
* **Spare Capacity** and **Media Errors** — NVMe only, since a SATA drive has neither
* **Data Written** — the drive's lifetime writes, in the unit total writes are quoted in (`TBW`). An NVMe controller reports `total_data_written` in bytes; an ATA drive's `Total_LBAs_Written` (attribute 241) is read as the 512-byte sectors the ATA standard defines it as — including when the platform's SMART library reports the counter under its own `MB` unit, which is the same counter with a 32 MiB-unit assumption already applied and is converted back rather than trusted. Some models (SanDisk and WD families are the documented ones) genuinely count that attribute in 32 MiB units, which nothing in the reading reveals, so those drives are **under-reported** — see the note below the list.

Each row's unit is rendered from the reading itself rather than written into the row's label, so a figure and the label beside it cannot disagree about what is being measured; the rows below are named by the quantity they show, and the unit appears next to the value. A row appears only when the drive reports it, so an HDD shows no wear row, a SATA drive shows no spare-capacity row, and a drive whose SMART table carries no total-written attribute (the review machine's own Samsung SSD is one) shows no `Data Written` row. The badge and its reason rest on the drive's own verdict first — its failing flag, the count of attributes below their threshold, and the NVMe critical warnings — and only then on the app's reading of figures the drive has not yet faulted (endurance past 80% or 95%, spare capacity below its threshold, bad sectors or media errors above zero, a failed self-test). A value that could not be read is never rendered as a zero.

One caveat belongs with those rows rather than in a footnote: attribute 241's *unit* is the vendor's, and the two conventions in use differ by a factor of 65536. The app always takes the sector convention the ATA standard specifies — the SMART library's own `MB` figure is built from that same counter with the 32 MiB-unit assumption already applied, so it is divided back out rather than read as megabytes of data — but a drive that genuinely counts in 32 MiB units cannot be told apart from a slow, lightly used one, and so is under-reported rather than reported too large. A drive that reports no counter gets no row, because a reported zero and an absent one are indistinguishable and `0.00 TB` would be the app's claim rather than the drive's.

Readings are passive: the attribute table is read at most once a minute and with UDisks2's `nowakeup` option, so a sleeping drive answers with an error rather than being spun up. If udisksd is not running, or reports that a drive has no SMART, the card simply has no badge and no SMART rows.

## Mainboard readings

DMI (`/sys/class/dmi/id`, i.e. the SMBIOS strings the firmware publishes)

* Vendor, model and version of the board, and its serial number when the
  firmware exposes one — `board_serial` is root-only on many systems, and a
  blank field produces **no row** rather than an empty one
* BIOS version and BIOS date

The board's own sensors come from two places.

ACPI thermal zones (`/sys/class/thermal/thermal_zone*`)

* The generic `acpitz` zone and the chipset zones (`pch_skylake`, `TZ00`, …).
  Component zones — CPU package, GPU, wireless card, disks — are deliberately
  *not* reported here, because those readings belong to the component's own card.

Mainboard monitoring chips (`/sys/class/hwmon/hwmon*`)

* The Super I/O chips soldered onto the board — Nuvoton `nct67xx`, ITE `it87xx`,
  Fintek `f718xx`, Winbond `w836xx` — plus ASUS/Gigabyte WMI board sensors.
  Temperatures (`tempN_input`) and fan headers (`fanN_input`) are read, and each
  sensor's own label is used when the driver provides one (`tempN_label`,
  `fanN_label`), so rows read `SYSTIN Temperature` or `Chassis Fan Speed` rather
  than `Sensor 3`.
* CPU (`coretemp`, `k10temp`), GPU, storage, wireless and battery hwmon chips are
  skipped: their readings are reported by the CPU and GPU cards instead.

Like the per-core CPU rows, the board's temperature and fan rows are generated
from what the board actually reports (`board.tempN`, `board.fanN`), so a board
with a dozen fan headers shows all of them. A board that reports nothing at all
(no SMBIOS, no board sensors) produces no card.

## Memory readings

`/proc/meminfo` (RAM and swap), `/proc/pressure/memory` (the kernel's own pressure accounting) and `/sys/block/zramN` (the compressed store swap usually lives on) — one card for the machine

* **Memory Total**, **Used** and **Available**, in binary GiB — a machine sold as 16 GB reports exactly `16.00 GiB`. `Used` is `Total − Available`, the same figure `free` prints: `MemAvailable` is the kernel's estimate of what a new workload could claim without swapping, whereas `MemFree` ignores the reclaimable cache and would report a healthy machine as nearly full.
* **Memory Usage** — the used share of total.
* **Swap Total**, **Used** and **Usage** — only on a machine that has swap configured. A system with no swap reports no swap rows rather than three zeroes.
* **Memory Pressure (some)** and **(full)** — the share of the last ten seconds during which tasks were stalled waiting for memory. `some` counts the windows in which at least one task stalled, `full` only those in which every task did, so `full` is the stronger signal that a machine is actually struggling. Both are absent on a kernel built without PSI.
* **Zram Algorithm**, **Size**, **Usage**, **Memory** and **Ratio** — on a machine that swaps to a compressed RAM block device, and only when at least one device is configured. These rows describe the store itself rather than what is inside it: the swap rows above say how much the machine has swapped out, and these say what that costs.
  * **Zram Size** is the device's uncompressed capacity in binary GiB — what it can hold before it is full. Note that this is the *uncompressed* figure, which is why it can be larger than the RAM the machine has.
  * **Zram Usage** is the share of that capacity currently holding data.
  * **Zram Memory** is what the stored data actually costs in RAM. It is larger than the data's compressed size, because the kernel's figure includes allocator fragmentation and per-page metadata, and it is quoted in MiB because an idle device costs a few hundred kilobytes — in GiB every idle machine would read `0.00`.
  * **Zram Ratio** compares the data stored against its compressed size, written `4.00:1`. It appears only once there is data on both sides of it, and with almost nothing stored it is dominated by the few pages that are there — a freshly configured device reads tens to one — so **Zram Memory** is the figure to read on an idle machine.
  * **Zram Algorithm** names the compressor in use, and only when every device uses the same one; naming one of several would be a claim about devices that do not use it. **Zram Devices** appears only when the kernel has more than one configured, because the other rows are then sums rather than one device's own figures.
  * A device the module pre-created but nobody configured is skipped: it has no size, holds nothing and costs nothing. If any configured device's counters cannot be read, the counter rows are withheld rather than reported as a sum over the devices that did answer.

`zswap` — the kernel's own compressed cache in front of a swap device — is a different mechanism and is not read. `zram` devices are also never reported as drives: they hold no filesystem identity of their own.

`Used` and `Available` are derived from one reading, so they are only shown together: a machine whose availability cannot be read shows `Total` on its own, because a guess rendered as `0.00 GiB used` would be the app's claim rather than the kernel's. Kernels before 3.14 do not report `MemAvailable`; the figure is then derived the way those kernels' tooling derived it, from free memory plus the reclaimable caches.

## Battery readings

`/sys/class/power_supply` — one card per battery, no root required, on a laptop

* **Status** — the kernel's own word: `Charging`, `Discharging`, `Full`, `Not charging` or `Unknown`.
* **Charge** — the percentage the firmware reports, drawn with a bar that runs the opposite way from a load bar: a nearly empty pack is the alarming end, so it turns red rather than green.
* **Energy Now**, **Energy Full** and **Energy Design** in Wh, or **Charge Now**, **Charge Full** and **Charge Design** in Ah. Most firmware reports energy and some reports charge; only the family the battery publishes appears, and the unit on the reading says which it is. The two are never converted into each other, because that needs the pack voltage and would give a different figure from the one the battery reports.
* **Health** — what the battery holds when full against what it was designed to hold.
* **Power Draw** in W, **Voltage** and **Design Voltage**, **Charge Cycles**, and **Time to Empty** or **Time to Full**.
* **AC Adapter** — `Online` or `Offline`, read from the supply that actually delivers the mains: a `Mains` adapter or one of the USB charger types, since a USB-C supply is how most current laptops charge. A UPS is deliberately not counted — it reports itself online while it runs the machine from its own battery — and an adapter whose state cannot be read produces no row rather than an `Offline` claim.
* Identity: model, manufacturer, serial number and battery technology.

Each card carries a health badge beside its title — `Health: Good`, `Health: Worn` or `Health: Poor` — with the figures behind the verdict in its tooltip: the percentages, the Wh or Ah they came from, and the charge-cycle count. The thresholds are 80% and 50% of the design capacity. A battery whose firmware publishes no design capacity gets **no badge**, because there would be nothing to compare it against and an invented verdict would be worse than none.

A laptop can have **more than one battery** — an internal pack plus a removable one, or one in an ultrabay — and each gets its own card, titled with the supply name the kernel lists (`BAT0`, `BAT1`), with its own capacity, its own wear and its own min/max history. A battery bay with the pack removed (`present` is 0) is not reported, and neither is a peripheral's battery: a wireless mouse or headset reports one too, under scope `Device`. A supply whose type cannot be read is skipped rather than assumed to be a battery, so a mains adapter can never end up on a battery card.

Nothing is derived that the firmware did not state. A reading that cannot be taken produces no row rather than a zero, a battery that publishes almost nothing shows almost nothing, and time to empty or full is only estimated where the arithmetic is exact — energy over power, while the battery is actually discharging or charging. A charge-family battery gets no estimate, because Ah cannot be divided by watts without the voltage; a full battery on the mains gets none either, since it moves no power in either direction.

## Design rules

Three rules explain most of the behaviour above, and they are the standard a change to a collector is held to.

1. **An absent reading is no row, not a zero.** A missing hotspot sensor, an unreadable serial and an empty battery bay all produce no row. A `0` would be the app's claim about the machine, which is the one thing a monitor must not make up.
2. **The unit travels on the reading.** Every value carries its own unit, rendered from the reading itself, so a figure and the label beside it cannot disagree — `GB` and `GiB` are deliberately different units and are never merged.
3. **No forks on the refresh path, no root, no optional tools.** Names and versions are resolved once per process, slow readings are cached, and every reading comes from `/proc`, `/sys` or UDisks2. `shared.RunCommand` (no shell) is preferred when an argument comes from sysfs.

## Development

OpenHWInfo is a Go backend of one collector per vendor or subsystem, plus a dependency-free frontend (`website/`) embedded into the binary with `//go:embed all:website`. There is no HTTP server, no port and no template loading at request time.

```bash
make build      # go vet ./..., then the production binary → ./OpenHWInfo
make test       # go test ./... (fixtures only: no hardware, no webview headers needed)
make lint       # go vet ./...
make map        # regenerate the agent-facing code map (make test checks it is current)
make hooks      # install the repo's git hooks into this clone (one-time)
make package    # .deb into dist/ (needs dpkg-deb)
make install    # desktop entry + hicolor icons into ~/.local/share
make clean      # remove binary and dist/
```

Collectors are fixture-testable: file-system roots and external paths are package variables, so a test points them at a `t.TempDir()` tree and the suite passes on a machine with no GPU, no battery and no drives. `./OpenHWInfo -debug` traces what each collector found and why a device was skipped.

`./OpenHWInfo`, `dist/` and `build/` are build output and are ignored by git, so `make build` is free to overwrite the binary. A one-off package can override the version and maintainer without touching `VERSION`: `make package VERSION=1.0.0 MAINTAINER="Your Name <you@example.org>"`.

### Releases

The version lives in [`VERSION`](VERSION) — the one file a release bumps, read by `make package` as well as by CI. A release is a `v` tag that matches that file, so `v0.1.0` releases what `VERSION` calls `0.1.0`, and a tag that disagrees with it fails before anything is packaged:

```bash
echo 0.2.0 > VERSION
git commit -m "Release 0.2.0" VERSION
git tag v0.2.0
git push origin main v0.2.0
```

That tag is the only trigger. `.github/workflows/release.yml` runs `make package` on an amd64 runner and on an arm64 runner, then attaches `dist/openhwinfo_<version>_amd64.deb` and `..._arm64.deb` to the GitHub release — a branch push or a manual run builds nothing there, so every released `.deb` was built from the tagged commit. The release notes are the release commit's own message, minus the attribution trailers, so what someone reads on the release page is what the commit says; a release commit with no body falls back to GitHub's generated list. The packages are built on a machine of their own architecture rather than cross-compiled, because Wails links the webview through cgo. The control file's `Maintainer` is the Makefile's default unless a repository variable named `MAINTAINER` is set.

`AGENTS.md` describes how an agent works in this repository and `CODE_REVIEW.md` records the design decisions, the test coverage and the known limitations.

## License

OpenHWInfo is licensed under the [Apache License 2.0](LICENSE). It may be used, modified and redistributed, including commercially, as long as the license text and the attribution notices travel with it.
