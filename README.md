# OpenHWInfo

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
