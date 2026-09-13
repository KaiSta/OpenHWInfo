// Package memory reports the machine's RAM and swap usage.
//
// The readings come from /proc/meminfo, which every Linux system provides, so
// there is nothing to configure and no external program to install. Memory
// pressure, where the kernel supports it, is read from the PSI file the kernel
// maintains for its own stall accounting. The kernel's compressed RAM block
// devices (zram), which is what swap usually lives on, are read from their own
// sysfs attributes — still no external program, still no root.
package memory

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// meminfoPath, psiPath and zramPath are variables so tests can point the
// collector at a fixture, exactly as the other collectors do with their own
// paths. zramPath is the sysfs block directory, which holds one zramN entry per
// device.
var (
	meminfoPath = "/proc/meminfo"
	psiPath     = "/proc/pressure/memory"
	zramPath    = "/sys/block"
)

const (
	// bytesPerGiB is the divisor that turns a byte count into GiB.
	bytesPerGiB = 1024 * 1024 * 1024
	// bytesPerMiB is the divisor for the zram memory row. That figure is quoted
	// in MiB because a machine swapping hard spends hundreds of MiB on its
	// compressed store, while the same figure in GiB would round an idle
	// device's few hundred kilobytes down to nothing.
	bytesPerMiB = 1024 * 1024
)

// GetInformations reports the machine's memory as one card: the RAM it holds,
// how much of that is in use, what is still available, and — when the machine
// has swap and a kernel that accounts for stalls — swap usage and memory
// pressure, with the compressed store behind that swap where zram provides it.
//
// Only fields that could actually be read produce a row. A machine with no swap
// configured reports no swap rows, a kernel without PSI reports no pressure
// rows, and a machine without zram reports no zram rows, rather than zeroes for
// each.
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

	addZram(&data)

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

// zramDevice is what one zram device reports about itself. hasCounters is false
// when the device's mm_stat could not be read or did not carry the counters, so
// its figures are unknown rather than zero.
type zramDevice struct {
	size        float64 // disksize: the uncompressed capacity of the device
	hasCounters bool
	orig        float64 // orig_data_size: the uncompressed data stored in it
	compressed  float64 // compr_data_size: what that data was compressed to
	memory      float64 // mem_used_total: the RAM the store occupies
	algorithm   string  // the algorithm in use, "" when the file names none
}

// addZram reports the kernel's compressed RAM block devices, read from sysfs so
// that nothing is forked and no tool has to be installed. What it describes is
// the store itself — how large it is, how much of it holds data, what that data
// costs in RAM and how well it compressed — not what is inside it, which the
// swap rows above already state.
func addZram(data *shared.HardwareData) {
	devices := readZram()
	if len(devices) == 0 {
		return
	}

	var size, orig, compressed, memory float64
	counters := true
	algorithms := make(map[string]bool)
	for _, device := range devices {
		size += device.size
		if device.algorithm != "" {
			algorithms[device.algorithm] = true
		}
		if !device.hasCounters {
			counters = false
			continue
		}
		orig += device.orig
		compressed += device.compressed
		memory += device.memory
	}

	data.Values[shared.ZramSize.Key] = shared.NewHardwareValue(size/bytesPerGiB, shared.Gibibyte, shared.Memory)

	if !counters {
		// The counters are reported only when every device answered: a sum over
		// the devices that did would understate what the machine is spending,
		// and a reading that is quietly too low is worse than none.
		shared.Debugf("zram: a device reported no usable mm_stat; omitting the aggregate counters")
	} else {
		if size > 0 {
			data.Values[shared.ZramUsage.Key] = shared.NewHardwareValue(orig/size*100, shared.Percent, shared.Usage)
		}
		data.Values[shared.ZramMemory.Key] = shared.NewHardwareValue(memory/bytesPerMiB, shared.Megabyte, shared.Memory)
		// A ratio has data on both sides of it or it does not exist at all: an
		// empty store has no ratio, which is not a ratio of zero.
		if orig > 0 && compressed > 0 {
			data.Values[shared.ZramRatio.Key] = shared.NewHardwareValue(orig/compressed, shared.Ratio, shared.Unknown)
		}
	}

	// One algorithm can be named for the set only when the devices share one;
	// naming one of several would be a claim about devices that do not use it.
	if len(algorithms) == 1 {
		for algorithm := range algorithms {
			data.Values[shared.ZramAlgorithm.Key] = shared.NewHardwareValue(algorithm, shared.UnknownUnit, shared.Unknown)
		}
	} else if len(algorithms) > 1 {
		shared.Debugf("zram: devices use %d different algorithms; naming none of them", len(algorithms))
	}

	// The device count is worth a row because it makes the other rows readable as
	// sums, but only when there is more than one device to sum.
	if len(devices) > 1 {
		data.Values[shared.ZramDevices.Key] = shared.NewHardwareValue(strconv.Itoa(len(devices)), shared.UnknownUnit, shared.Unknown)
	}
}

// readZram lists the zram devices the kernel currently has. A device the module
// pre-created but nobody configured is not one of them: it has no size, holds
// nothing and costs nothing, and reporting it would put a 0 GiB device on the
// card of every machine that loads the module with a device count.
func readZram() []zramDevice {
	entries, err := os.ReadDir(zramPath)
	if err != nil {
		shared.Debugf("Could not list %s: %v", zramPath, err)
		return nil
	}

	var devices []zramDevice
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "zram") || !allDigits(strings.TrimPrefix(name, "zram")) {
			continue
		}
		device, ok := readZramDevice(filepath.Join(zramPath, name))
		if !ok {
			continue
		}
		devices = append(devices, device)
	}
	return devices
}

// readZramDevice reads one device's attributes. It reports false when the device
// has no size — which is how an unconfigured one looks — and when the size
// cannot be read, because a device whose capacity is unknown cannot be summed
// into the card's total.
func readZramDevice(dir string) (zramDevice, bool) {
	sizes, ok := readNumbers(filepath.Join(dir, "disksize"))
	if !ok || len(sizes) == 0 || sizes[0] <= 0 {
		return zramDevice{}, false
	}

	device := zramDevice{size: sizes[0]}
	device.algorithm = readZramAlgorithm(filepath.Join(dir, "comp_algorithm"))

	// mm_stat is one line of whitespace-separated counters in the order the
	// kernel documents: orig_data_size, compr_data_size and mem_used_total, then
	// mem_limit and mem_used_max, then the page counters. Only the first three
	// say what is stored and what it costs.
	counters, ok := readNumbers(filepath.Join(dir, "mm_stat"))
	if !ok || len(counters) < 3 {
		shared.Debugf("zram: %s reported no usable mm_stat", dir)
		return device, true
	}
	device.orig, device.compressed, device.memory = counters[0], counters[1], counters[2]
	device.hasCounters = true
	return device, true
}

// readZramAlgorithm returns the algorithm a device is using. comp_algorithm
// lists every algorithm the kernel supports with the one in use in square
// brackets ("lzo-rle lzo [lz4] zstd"), so a file without brackets says nothing
// about which is active and is reported as saying nothing.
func readZramAlgorithm(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		shared.Debugf("Could not read %s: %v", path, err)
		return ""
	}
	for _, field := range strings.Fields(string(raw)) {
		if strings.HasPrefix(field, "[") && strings.HasSuffix(field, "]") {
			return strings.Trim(field, "[]")
		}
	}
	return ""
}

// readNumbers reads whitespace-separated numbers from a sysfs file. Every field
// must parse: these files are positional — mm_stat is a sequence of counters — so
// a field that did not parse would leave the ones after it misaligned, and a
// misaligned counter is another counter's reading.
func readNumbers(path string) ([]float64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		shared.Debugf("Could not read %s: %v", path, err)
		return nil, false
	}

	var numbers []float64
	for _, field := range strings.Fields(string(raw)) {
		number, err := strconv.ParseFloat(field, 64)
		if err != nil {
			shared.Debugf("%s: %q is not a number", path, field)
			return nil, false
		}
		numbers = append(numbers, number)
	}
	return numbers, true
}

// allDigits reports whether s is one or more digits, which is what tells a zram
// device's name from anything else the block directory holds.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
