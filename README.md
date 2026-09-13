# OpenHWInfo

A native desktop hardware monitor (CPU + GPU) built with [Wails](https://wails.io). The Go backend collects live sensor data and the embedded webview renders it, refreshing every 5 seconds.

## Build & Run

### Linux (WebKitGTK 4.1)

Prerequisites: `sudo apt install libwebkit2gtk-4.1-dev build-essential`

```bash
make run        # builds + runs the native app
make build      # just builds → ./OpenHWInfo
make dev        # development mode with hot-reload (serves frontend from disk)
make test       # run all tests
make package    # build a .deb installer into dist/ (also: make install)
make clean      # remove binary and dist/
```

The `production` tag is required for Wails to create the native window; `webkit2_41` selects WebKitGTK 4.1 (use `libwebkit2gtk-4.0-dev` and drop this tag on older distros). The frontend in `website/` is embedded into the binary, so a single self-contained executable is produced.

> **Note:** Running `go build .` without tags will fail with "Wails applications will not build without the correct build tags." Always use `make build` or add `-tags=production,webkit2_41`.

The build output (`./OpenHWInfo`) is **not tracked by git** — it is rebuilt by `make build` and ignored via `.gitignore`. `make package` writes `dist/openhwinfo_<version>_<arch>.deb`; override `VERSION` and `MAINTAINER` for a release build:

```bash
make package VERSION=1.0.0 MAINTAINER="Your Name <you@example.org>"
```

## Reading the output

Each metric is a key/value pair with a unit and a value type. Min and max columns track the observed range over the last 100 samples (~8 minutes at the 5 s refresh).

Per-core clocks render as a compact grid rather than one table row per core, so a 32-thread CPU stays readable. Each tile shows the current clock, a bar scaled against the highest clock any core has reached (so boost headroom is visible), and the observed min–max range. When the core temperatures can be attributed to a thread, they appear in the same tile; otherwise they render in their own block. See the CPU section below.

## CPU infos

`/proc/cpuinfo`

* Vendor ID, brand and model name
* One clock per logical processor
* The physical core each logical processor runs on, used to attribute per-core temperatures correctly on SMT systems

lm-sensors `sensors`

* CPU temperature — `cpu.temp`, from Intel `Package id N` or AMD `Tctl`
* PCH temp = chipset
* Per-core temperatures — `cpu.coreTempN`
* TCCD (per-CCD) temperatures where reported

## Mainboard infos

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

## GPU infos

### Nvidia

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

* Name (PCI-address-scoped `lspci` query, disambiguated when several identical cards are present)
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
* Driver version (`/sys/module/i915/version`)
* Temperature (`temp1_input`), hotspot temp if available (`temp2_input`)
* GPU clock (`freq0_input` / `freq1_input`)
* Average power draw (`power1_average`)
* Fan speed (`fan1_input`) if available

For **integrated** graphics, VRAM metrics are not reported because they share
system memory. Discrete Arc cards are detected and named, but their dedicated
VRAM is not read yet — see the Arc item under *Remaining opportunities* in
`CODE_REVIEW.md` for the current gaps.

### Common behaviour

A sensor file that does not exist produces **no metric**, rather than a zero reading — a card with no hotspot sensor or no power cap simply omits those rows. Similarly, a card that exposes no readable sensors at all (for example an integrated GPU whose hwmon directory is absent) is not rendered as an empty card.

## Storage infos

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

* **Life Used (%)** — endurance consumed. An NVMe controller reports this itself; on an ATA drive it is derived from the drive's wear attribute (normalized to 100 when new and to the drive's own fail threshold at end of life, so the fraction is measured against that threshold).
* **Power-On Time (h)**, **Bad Sectors** (ATA: pending plus reallocated), **SMART Self-Test**
* **Spare Capacity (%)** and **Media Errors** — NVMe only, since a SATA drive has neither
* **Data Written (TB)** — the drive's lifetime writes, in the unit total writes are quoted in (`TBW`). An NVMe controller reports `total_data_written` in bytes; an ATA drive's `Total_LBAs_Written` (attribute 241) is read as the 512-byte sectors the standard defines it as, or in megabytes when the platform's SMART library reports that unit for it. Some models (SanDisk and WD families are the documented ones) count that attribute in 32 MiB units instead, which nothing in the reading reveals, so those drives are **under-reported** — see the note below the list.

A row appears only when the drive reports it, so an HDD shows no wear row, a SATA drive shows no spare-capacity row, and a drive whose SMART table carries no total-written attribute (the review machine's own Samsung SSD is one) shows no `Data Written` row. The badge and its reason rest on the drive's own verdict first — its failing flag, the count of attributes below their threshold, and the NVMe critical warnings — and only then on the app's reading of figures the drive has not yet faulted (endurance past 80% or 95%, spare capacity below its threshold, bad sectors or media errors above zero, a failed self-test). A value that could not be read is never rendered as a zero.

One caveat belongs with those rows rather than in a footnote: attribute 241's *unit* is the vendor's, and the two conventions in use differ by a factor of 65536. The app reads the sector convention the ATA standard specifies and honors a different byte unit when the SMART library reports one, but a drive counting in 32 MiB units cannot be told apart from a slow, lightly used one, so its figure is too small rather than too large. A drive that reports no counter gets no row, because a reported zero and an absent one are indistinguishable and `0.00 TB` would be the app's claim rather than the drive's.

Readings are passive: the attribute table is read at most once a minute and with UDisks2's `nowakeup` option, so a sleeping drive answers with an error rather than being spun up. If udisksd is not running, or reports that a drive has no SMART, the card simply has no badge and no SMART rows.
