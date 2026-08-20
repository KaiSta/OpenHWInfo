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
make clean      # remove binary
```

The `production` tag is required for Wails to create the native window; `webkit2_41` selects WebKitGTK 4.1 (use `libwebkit2gtk-4.0-dev` and drop this tag on older distros). The frontend in `website/` is embedded into the binary, so a single self-contained executable is produced.

> **Note:** Running `go build .` without tags will fail with "Wails applications will not build without the correct build tags." Always use `make build` or add `-tags=production,webkit2_41`.

## CPU Infos

`/proc/cpuinfo`

* Name
* Core count
* Clock for each core

lm-sensors `sensors`

* CPU temperature
* PCH temp = chipset

## GPU infos

### Nvidia

`nvidia-smi`

* `-L` GPU name
* Temperature
* Current and max power draw
* RAM current/max
* Utilization

https://nvidia.custhelp.com/app/answers/detail/a_id/3751/~/useful-nvidia-smi-queries
https://developer.download.nvidia.com/compute/DCGM/docs/nvidia-smi-367.38.pdf

### AMD

### Intel

`lspci` + `/sys/class/drm/cardN/device` (PCI vendor ID `0x8086`) + hwmon (i915)

* Name
* Driver version (`/sys/module/i915/version`)
* Temperature (`temp1_input`), hotspot temp if available (`temp2_input`)
* GPU clock (`freq0_input` / `freq1_input`)
* Average power draw (`power1_average`)
* Fan speed (`fan1_input`) if available

VRAM metrics are not reported because integrated graphics share system memory.
