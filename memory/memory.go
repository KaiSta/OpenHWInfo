// Package memory reports the machine's RAM and swap usage.
//
// The readings come from /proc/meminfo, which every Linux system provides, so
// there is nothing to configure and no external program to install. Memory
// pressure, where the kernel supports it, is read from the PSI file the kernel
// maintains for its own stall accounting.
package memory

import (
	"os"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// meminfoPath and psiPath are variables so tests can point the collector at a
// fixture, exactly as the other collectors do with their own paths.
var (
	meminfoPath = "/proc/meminfo"
	psiPath     = "/proc/pressure/memory"
)

// bytesPerGiB is the divisor that turns a byte count into GiB.
const bytesPerGiB = 1024 * 1024 * 1024

// GetInformations reports the machine's memory as one card: the RAM it holds,
// how much of that is in use, what is still available, and — when the machine
// has swap and a kernel that accounts for stalls — swap usage and memory
// pressure.
//
// Only fields that could actually be read produce a row. A machine with no swap
// configured reports no swap rows, and a kernel without PSI reports no pressure
// rows, rather than three zeroes each.
func GetInformations() shared.HardwareData {
	data := shared.HardwareData{Name: "Memory", Values: make(map[string]shared.HardwareValue)}

	fields, ok := readMeminfo()
	if !ok {
		return data
	}

	total := fields["MemTotal"]
	if total <= 0 {
		shared.Debugf("%s reported no MemTotal", meminfoPath)
		return data
	}

	addSize(&data, shared.MemTotal.Key, total)

	// How much is in use is derived from how much is available, so the two are
	// only reportable together: without an availability figure the used and usage
	// rows would be a guess, and a guess rendered as "0.00 GiB used" is worse
	// than no row at all.
	if available, ok := availableMemory(fields); ok {
		used := total - available
		if used < 0 {
			used = 0
		}
		addSize(&data, shared.MemUsed.Key, used)
		addSize(&data, shared.MemAvailable.Key, available)
		addPercent(&data, shared.MemUsage.Key, used, total)
	} else {
		shared.Debugf("%s reported neither MemAvailable nor MemFree", meminfoPath)
	}

	// Swap is reported only when the machine has some: a system that swaps to a
	// file or not at all reports SwapTotal 0, and three zero rows would say less
	// than no rows do.
	if swapTotal := fields["SwapTotal"]; swapTotal > 0 {
		addSize(&data, shared.SwapTotal.Key, swapTotal)
		if swapFree, ok := fields["SwapFree"]; ok {
			swapUsed := swapTotal - swapFree
			if swapUsed < 0 {
				swapUsed = 0
			}
			addSize(&data, shared.SwapUsed.Key, swapUsed)
			addPercent(&data, shared.SwapUsage.Key, swapUsed, swapTotal)
		}
	}

	addPressure(&data)

	return data
}

// availableMemory returns how much memory a new workload could use before the
// machine starts swapping, and whether that could be determined at all.
//
// MemAvailable is the kernel's own estimate and the honest counterpart of the
// misleading MemFree, which ignores reclaimable cache. Kernels before 3.14 do
// not report it, so the approximation those kernels' tooling used is derived
// instead: free memory plus the caches that can be reclaimed.
func availableMemory(fields map[string]float64) (float64, bool) {
	if available, ok := fields["MemAvailable"]; ok {
		return available, true
	}
	if free, ok := fields["MemFree"]; ok {
		return free + fields["Buffers"] + fields["Cached"] + fields["SReclaimable"], true
	}
	return 0, false
}

// addSize records a /proc/meminfo size, which is reported in kB, as GiB. Those
// "kB" are really KiB — 1024 bytes, whatever the field says — which is why a
// machine sold as 16 GB reports 16777216 here and comes out as exactly 16 GiB.
func addSize(data *shared.HardwareData, key string, kiloBytes float64) {
	data.Values[key] = shared.NewHardwareValue(kiloBytes*1024/bytesPerGiB, shared.Gibibyte, shared.Memory)
}

// addPercent records part of whole as a percentage, for the readings that are
// meaningful as a share rather than as a size.
func addPercent(data *shared.HardwareData, key string, part, whole float64) {
	if whole <= 0 {
		return
	}
	data.Values[key] = shared.NewHardwareValue(part/whole*100, shared.Percent, shared.Usage)
}

// readMeminfo parses /proc/meminfo into its numeric fields. It reports false
// when the file cannot be read at all, so the caller can distinguish "no
// memory to report" from a machine whose memory is genuinely zero. A single
// line that does not parse is skipped rather than failing the whole file.
func readMeminfo() (map[string]float64, bool) {
	raw, err := os.ReadFile(meminfoPath)
	if err != nil {
		shared.Debugf("Could not read %s: %v", meminfoPath, err)
		return nil, false
	}

	fields := make(map[string]float64)
	for _, line := range strings.Split(string(raw), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		// Every size field carries its unit on the line ("MemTotal: 16309352 kB").
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "kB"))
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		fields[strings.TrimSpace(name)] = number
	}
	return fields, true
}

// addPressure records the kernel's pressure-stall averages for memory. The file
// is absent unless the kernel was built with PSI, in which case no pressure rows
// are reported.
func addPressure(data *shared.HardwareData) {
	raw, err := os.ReadFile(psiPath)
	if err != nil {
		shared.Debugf("No memory pressure information in %s: %v", psiPath, err)
		return
	}

	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		avg10, ok := psiValue(fields[1:], "avg10")
		if !ok {
			continue
		}
		key, known := pressureKey(fields[0])
		if !known {
			continue
		}
		data.Values[key] = shared.NewHardwareValue(avg10, shared.Percent, shared.Usage)
	}
}

// pressureKey maps a PSI line's leading word to the row it feeds.
func pressureKey(kind string) (string, bool) {
	switch kind {
	case "some":
		return shared.MemPressureSome.Key, true
	case "full":
		return shared.MemPressureFull.Key, true
	}
	return "", false
}

// psiValue returns the number in the "name=value" field, e.g. "avg10=0.34".
func psiValue(fields []string, name string) (float64, bool) {
	for _, field := range fields {
		value, ok := strings.CutPrefix(field, name+"=")
		if !ok {
			continue
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}
