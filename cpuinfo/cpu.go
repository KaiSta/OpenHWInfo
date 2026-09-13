package cpuinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

var (
	cpuVendorIDKey = shared.CPUVendorID.Key
	cpuBrandKey    = shared.CPUBrand.Key
	cpuModelKey    = shared.CPUModel.Key

	// sysDir is the sysfs mount point. It is a variable so tests can point the
	// collector at a fixture tree, exactly as the GPU, storage and mainboard
	// collectors do.
	sysDir = "/sys"
)

// maxTccdSensors bounds the per-CCD temperature rows: a Zen chip with more CCDs
// than this would otherwise produce an unbounded number of keys.
const maxTccdSensors = 6

// cpuHwmonChips are the hwmon drivers whose temperature sensors belong to the
// processor and are mapped onto the CPU card.
var cpuHwmonChips = map[string]bool{
	"coretemp":    true, // Intel
	"k10temp":     true, // AMD
	"zenpower":    true, // AMD (out-of-tree driver)
	"k8temp":      true, // older AMD
	"cpu_thermal": true, // ARM SoCs
	"cpu-thermal": true, // ARM SoCs (older naming)
}

// isCPUHwmonChip reports whether an hwmon chip's sensors belong on the CPU card.
// The chip name is required, not just a sensor label: a Super I/O board chip can
// label one of its headers "CPUTIN", so matching on labels alone would report
// that board reading as the CPU temperature. The chipset's PCH sensor is the one
// exception — it is a board chip, but the app shows it on the CPU card.
func isCPUHwmonChip(name string) bool {
	return cpuHwmonChips[name] || strings.HasPrefix(name, "pch")
}

// GetInformations assembles the CPU card: identity, clocks and core topology
// from /proc/cpuinfo, temperatures from the hwmon sysfs interface. Neither
// source needs an external program, so the card is populated whether or not
// lm-sensors happens to be installed.
func GetInformations() shared.HardwareData {
	cpuinfos, _ := os.ReadFile("/proc/cpuinfo")
	lines := strings.Split(string(cpuinfos), "\n")
	name, cpus, packages := parseCpuInfo(lines)
	cpus[cpuModelKey] = name

	hwData := shared.HardwareData{Name: name, Values: make(map[string]shared.HardwareValue)}

	// Convert cpus map to HardwareValue format
	for k, v := range cpus {
		// Check if this is a clock value and set appropriate type
		if strings.HasPrefix(k, "cpu.clock") {
			hwData.Values[k] = shared.NewHardwareValue(v, shared.MHz, shared.Frequency)
		} else {
			hwData.Values[k] = shared.NewHardwareValue(v, shared.UnknownUnit, shared.Unknown)
		}
	}

	// The temperatures are degrees Celsius and their own type, so the frontend
	// colours and sorts them like the other temperature rows.
	for k, v := range cpuTemperatures(packages) {
		hwData.Values[k] = shared.NewHardwareValue(v, shared.Celsius, shared.Temperature)
	}

	return hwData
}

// hwmonSensor is one numbered temperature group (tempN_*) of an hwmon chip.
type hwmonSensor struct {
	index int
	input string
	label string
}

// numberedTemperatures returns the readable temperature sensors of an hwmon
// chip, ordered by sensor number. The optional tempN_label attribute supplies
// the sensor's name ("Package id 0", "Core 3", "Tctl", "Tccd1", …).
func numberedTemperatures(dir string) []hwmonSensor {
	matches, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))

	var sensors []hwmonSensor
	for _, match := range matches {
		index, ok := temperatureSensorIndex(filepath.Base(match))
		if !ok || index == 0 {
			continue
		}
		input, ok := shared.ReadSysFile(match)
		if !ok || input == "" {
			continue
		}
		label, _ := shared.ReadSysFile(filepath.Join(dir, fmt.Sprintf("temp%d_label", index)))
		sensors = append(sensors, hwmonSensor{index: index, input: input, label: label})
	}

	// Glob sorts lexicographically, which would place temp10 before temp2.
	sort.Slice(sensors, func(i, j int) bool { return sensors[i].index < sensors[j].index })
	return sensors
}

// temperatureSensorIndex extracts N from "tempN_input", e.g. "temp10_input" -> 10.
func temperatureSensorIndex(fileName string) (int, bool) {
	digits, ok := strings.CutSuffix(strings.TrimPrefix(fileName, "temp"), "_input")
	if !ok || digits == "" {
		return 0, false
	}
	index, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return index, true
}

// cpuHwmonChip is one hwmon chip whose sensors belong on the CPU card, with the
// name the driver reports for it.
type cpuHwmonChip struct {
	name  string
	dir   string
	isPCH bool
}

// cpuHwmonChipsInOrder returns the CPU's hwmon chips, sorted by their sysfs
// path so the result does not depend on the order the kernel enumerated them
// in. The chips are collected before they are read because the per-core mapping
// has to know how many of them there are: a core number published by two chips
// names two different cores.
func cpuHwmonChipsInOrder() []cpuHwmonChip {
	dirs, _ := filepath.Glob(filepath.Join(sysDir, "class/hwmon/hwmon*"))
	sort.Strings(dirs)

	var chips []cpuHwmonChip
	for _, dir := range dirs {
		name, ok := shared.ReadSysFile(filepath.Join(dir, "name"))
		if !ok || !isCPUHwmonChip(name) {
			continue
		}
		chips = append(chips, cpuHwmonChip{name: name, dir: dir, isPCH: strings.HasPrefix(name, "pch")})
	}
	return chips
}

// cpuTemperatures maps the CPU's hwmon sensors onto the keys the UI looks up:
// Intel's coretemp ("Package id N" plus one "Core N" per physical core), AMD's
// k10temp ("Tctl" plus one "TccdN" per CCD) and the chipset PCH sensor.
//
// Per-core rows are keyed by the core number their own label carries, because
// that is the numbering /proc/cpuinfo reports as "core id" and therefore the one
// the frontend pairs a thread with. They are only produced when a core number
// can mean one thing (see perCore below); the package reading is reported
// either way.
func cpuTemperatures(packages int) map[string]interface{} {
	temps := make(map[string]interface{})
	chips := cpuHwmonChipsInOrder()

	// A core number is ambiguous when a machine has more than one processor
	// package (each package restarts the numbering at 0, and nothing in sysfs
	// ties a coretemp device to the package it monitors) or when its CPU sensors
	// are spread over more than one chip. In both cases the per-core and per-CCD
	// rows are left out entirely rather than published under a number that may
	// name a different core — a mislabelled core is worse than an absent row, and
	// those keys are what the frontend pairs a thread with. The package reading is
	// unaffected, so such a machine still reports its CPU temperature.
	coreChips := 0
	for _, chip := range chips {
		if !chip.isPCH {
			coreChips++
		}
	}
	perCore := packages <= 1 && coreChips <= 1

	var tccdCount int

	for _, chip := range chips {
		sensors := numberedTemperatures(chip.dir)

		// The PCH reports a single chipset temperature, the first its chip
		// exposes; the frontend shows it as its own row rather than a core.
		if chip.isPCH {
			if _, taken := temps["cpu.pch"]; !taken && len(sensors) > 0 {
				if value, ok := milliCelsius(sensors[0].input); ok {
					temps["cpu.pch"] = value
				}
			}
			continue
		}

		for _, sensor := range sensors {
			value, ok := milliCelsius(sensor.input)
			if !ok {
				continue
			}
			switch {
			case strings.HasPrefix(sensor.label, "Package"), sensor.label == "Tctl":
				// One reading per processor, so a machine with several packages
				// reports the hottest: the cooler package must not hide the other.
				setHottestPackageTemp(temps, value)
			case sensor.label == "" && sensor.index == 1:
				// Chips that provide no labels at all (older k10temp, ARM's
				// cpu_thermal) report the package reading as their first sensor.
				if _, taken := temps["cpu.temp"]; !taken {
					temps["cpu.temp"] = value
				}
			case !perCore:
				// A second package repeats this package's core numbers.
			case strings.HasPrefix(sensor.label, "Core"):
				if core, ok := coreNumberFromLabel(sensor.label); ok {
					temps[fmt.Sprintf("cpu.coreTemp%d", core)] = value
				}
			case strings.HasPrefix(sensor.label, "Tccd"):
				if tccdCount < maxTccdSensors {
					temps[fmt.Sprintf("tccd%d", tccdCount)] = value
					tccdCount++
				}
			}
		}
	}

	return temps
}

// setHottestPackageTemp records a package reading, keeping the largest seen: the
// CPU temperature of a multi-package machine is the hottest of its processors.
func setHottestPackageTemp(temps map[string]interface{}, value float64) {
	if current, ok := temps["cpu.temp"].(float64); ok && current >= value {
		return
	}
	temps["cpu.temp"] = value
}

// coreNumberFromLabel returns the core a sensor's own label names, e.g.
// "Core 7" -> 7. A label carrying no number reports false, and the sensor then
// produces no row: numbering it by the order it was read would point the UI at a
// different core than the label does.
func coreNumberFromLabel(label string) (int, bool) {
	digits, ok := strings.CutPrefix(strings.TrimSpace(label), "Core")
	if !ok {
		return 0, false
	}
	core, err := strconv.Atoi(strings.TrimSpace(digits))
	if err != nil || core < 0 {
		return 0, false
	}
	return core, true
}

// milliCelsius converts a hwmon millidegree-Celsius reading to degrees. It
// reports false for an unreadable value rather than inventing a zero reading.
func milliCelsius(raw string) (float64, bool) {
	milli, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return float64(milli) / 1000, true
}

// firstFieldAfterColon returns the trimmed text after the first ":" in s, or
// "" when the line contains no colon.
func firstFieldAfterColon(s string) string {
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(s[idx+1:])
}

// parseCpuInfo reads the processor identity, clocks and topology out of
// /proc/cpuinfo. It also reports how many physical packages the file describes,
// which is what decides whether the per-core topology keys may be published: a
// second package restarts the core numbering, so "cpu.coreOfN" would otherwise
// name a core of one package or the other.
func parseCpuInfo(lines []string) (string, map[string]interface{}, int) {
	var name string
	var vendorID string
	var brand string
	var model string
	var cpus = make(map[string]interface{})

	// /proc/cpuinfo lists one block per logical processor, and within a block
	// "cpu MHz" comes before "core id". Each block is therefore buffered and
	// flushed when the next one starts, so a thread's clock and the physical core
	// it belongs to can be emitted together.
	var clockIndex int
	var blockHasClock bool
	var blockMHz interface{}
	var blockCoreID string
	var blockPackageID string

	// Buffered until the whole file has been read: whether the topology keys can
	// be published at all is only known once every package has been seen.
	coreOf := make(map[string]string)
	packages := make(map[string]struct{})

	flush := func() {
		if blockPackageID != "" {
			packages[blockPackageID] = struct{}{}
		}
		if !blockHasClock {
			return
		}
		index := strconv.Itoa(clockIndex)
		cpus["cpu.clock"+index] = blockMHz
		// Expose which physical core this thread runs on, so the UI can show the
		// core's temperature next to each thread instead of guessing that thread
		// N belongs to core N (false whenever SMT is enabled).
		if blockCoreID != "" {
			coreOf[index] = blockCoreID
		}
		clockIndex++
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "processor"):
			flush()
			blockHasClock = false
			blockMHz = nil
			blockCoreID = ""
			blockPackageID = ""
		case strings.HasPrefix(line, "model name"):
			if v := firstFieldAfterColon(line); v != "" {
				name = v
			}
		case strings.HasPrefix(line, "vendor_id"):
			if v := firstFieldAfterColon(line); v != "" {
				vendorID = v
			}
		case strings.HasPrefix(line, "core id"):
			if v := firstFieldAfterColon(line); v != "" {
				blockCoreID = v
			}
		case strings.HasPrefix(line, "physical id"):
			if v := firstFieldAfterColon(line); v != "" {
				blockPackageID = v
			}
		case strings.HasPrefix(line, "cpu MHz"):
			if v := firstFieldAfterColon(line); v != "" {
				blockHasClock = true
				if cpuMHz, err := strconv.ParseFloat(v, 64); err == nil {
					blockMHz = cpuMHz
				} else {
					blockMHz = v + " MHz"
				}
			}
		}
	}
	flush()

	// The thread-to-core mapping is only published for a single-package machine.
	// On a machine with several packages the core numbers repeat, so this key
	// would point at a core of either package; leaving it out is what makes the
	// frontend show the core sensors on their own instead of pairing a thread
	// with whichever sensor shares its number.
	if len(packages) <= 1 {
		for index, core := range coreOf {
			cpus["cpu.coreOf"+index] = core
		}
	}
	if len(name) > 0 && strings.Contains(name, "@") {
		parts := strings.Split(name, "@")
		name = strings.TrimSpace(parts[0])
	}

	// Simple heuristic to extract brand and model.
	if len(name) > 0 {
		brandModel := strings.SplitN(name, " ", 2)
		if len(brandModel) == 2 {
			brand = brandModel[0]
			model = brandModel[1]
		} else if len(brandModel) == 1 {
			brand = name
			model = ""
		}
	}

	cpus[cpuVendorIDKey] = vendorID
	cpus[cpuBrandKey] = brand
	cpus[cpuModelKey] = model

	return name, cpus, len(packages)
}
