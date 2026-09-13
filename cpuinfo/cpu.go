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
	name, cpus := parseCpuInfo(lines)
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
	for k, v := range cpuTemperatures() {
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

// cpuTemperatures maps the CPU's hwmon sensors onto the keys the UI looks up:
// Intel's coretemp ("Package id N" plus one "Core N" per physical core), AMD's
// k10temp ("Tctl" plus one "TccdN" per CCD) and the chipset PCH sensor. Chips
// are visited in a stable order so the per-core ordinals do not depend on the
// order the kernel happened to enumerate hwmon devices in.
func cpuTemperatures() map[string]interface{} {
	temps := make(map[string]interface{})

	chips, _ := filepath.Glob(filepath.Join(sysDir, "class/hwmon/hwmon*"))
	sort.Strings(chips)

	var coreCount, tccdCount int

	for _, chip := range chips {
		chipName, ok := shared.ReadSysFile(filepath.Join(chip, "name"))
		if !ok || !isCPUHwmonChip(chipName) {
			continue
		}

		sensors := numberedTemperatures(chip)

		// The PCH reports a single chipset temperature, the first its chip
		// exposes; the frontend shows it as its own row rather than a core.
		if strings.HasPrefix(chipName, "pch") {
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
			case strings.HasPrefix(sensor.label, "Package"):
				temps["cpu.temp"] = value
			case strings.HasPrefix(sensor.label, "Core"):
				temps[fmt.Sprintf("cpu.coreTemp%d", coreCount)] = value
				coreCount++
			case sensor.label == "Tctl":
				temps["cpu.temp"] = value
			case strings.HasPrefix(sensor.label, "Tccd"):
				if tccdCount < maxTccdSensors {
					temps[fmt.Sprintf("tccd%d", tccdCount)] = value
					tccdCount++
				}
			case sensor.label == "" && sensor.index == 1:
				// Chips that provide no labels at all (older k10temp, ARM's
				// cpu_thermal) report the package reading as their first sensor.
				if _, taken := temps["cpu.temp"]; !taken {
					temps["cpu.temp"] = value
				}
			}
		}
	}

	return temps
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

func parseCpuInfo(lines []string) (string, map[string]interface{}) {
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

	flush := func() {
		if !blockHasClock {
			return
		}
		index := strconv.Itoa(clockIndex)
		cpus["cpu.clock"+index] = blockMHz
		// Expose which physical core this thread runs on, so the UI can show the
		// core's temperature next to each thread instead of guessing that thread
		// N belongs to core N (false whenever SMT is enabled).
		if blockCoreID != "" {
			cpus["cpu.coreOf"+index] = blockCoreID
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

	// Extract brand and model from name.
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

	return name, cpus
}
