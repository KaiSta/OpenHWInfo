package cpuinfo

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KaiSta/OpenHWInfo/shared"
)

var (
	cpuVendorIDKey = shared.CPUVendorID.Key
	cpuBrandKey    = shared.CPUBrand.Key
	cpuModelKey    = shared.CPUModel.Key

	sensorsProbeMu   sync.Mutex
	sensorsProbeOK   bool
	sensorsProbeDone bool
	sensorsProbeTime time.Time
	sensorsProbeTTL  = 60 * time.Second
)

func hasSensors() bool {
	sensorsProbeMu.Lock()
	defer sensorsProbeMu.Unlock()
	if sensorsProbeDone && time.Since(sensorsProbeTime) < sensorsProbeTTL {
		return sensorsProbeOK
	}
	_, err := exec.LookPath("sensors")
	ok := err == nil
	sensorsProbeOK = ok
	sensorsProbeDone = true
	sensorsProbeTime = time.Now()
	if !ok {
		log.Println("CPU probe failed, please install lm-sensors package")
	}
	return ok
}

func GetInformations() shared.HardwareData {
	if !hasSensors() {
		return shared.HardwareData{Name: "", Values: make(map[string]shared.HardwareValue)}
	}

	cpuinfos, _ := os.ReadFile("/proc/cpuinfo")
	lines := strings.Split(string(cpuinfos), "\n")
	name, cpus := parseCpuInfo(lines)
	cpus["cpu.model"] = name

	cmd := exec.Command("sensors")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Printf("Error reading sensor data: %v\n", err)
		hwData := shared.HardwareData{Name: name, Values: make(map[string]shared.HardwareValue)}
		for k, v := range cpus {
			hwData.Values[k] = shared.NewHardwareValue(v, shared.UnknownUnit, shared.Unknown)
		}
		return hwData
	}

	temps := parseSensorOutput(strings.Split(out.String(), "\n"))

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

	// Convert temps map to HardwareValue format with numeric values
	for k, v := range temps {
		hwData.Values[k] = shared.NewHardwareValue(v, shared.Celsius, shared.Temperature)
	}

	return hwData
}

func parseSensorOutput(lines []string) map[string]interface{} {
	var temps = make(map[string]interface{})
	var tccdCount = 0
	var coreCount = 0

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "CPU"):
			// Some chipsets expose a plainly named CPU sensor.
			setTemperature(temps, "cpu.temp", firstFieldAfterColon(line))
		case strings.Contains(line, "Core") || strings.Contains(line, "Package"):
			label := labelBeforeColon(line)
			if label == "" {
				continue
			}
			// coretemp reports the chip-wide reading as "Package id N" — the CPU
			// temperature the frontend labels as such — plus one reading per
			// physical core. Both get canonical keys; deriving them from the
			// sensor label is what previously left these readings unbound.
			if strings.HasPrefix(label, "Package") {
				setTemperature(temps, "cpu.temp", firstFieldAfterColon(line))
			} else {
				setTemperature(temps, fmt.Sprintf("cpu.coreTemp%d", coreCount), firstFieldAfterColon(line))
				coreCount++
			}
		case strings.HasPrefix(line, "Tctl"):
			setTemperature(temps, "cpu.temp", firstFieldAfterColon(line))
		case strings.HasPrefix(line, "pch"):
			if i+2 < len(lines) {
				setTemperature(temps, "cpu.pch", firstFieldAfterColon(lines[i+2]))
			}
		case strings.Contains(line, "Tccd") && tccdCount < 6:
			raw := firstFieldAfterColon(line)
			if raw == "" {
				continue
			}
			setTemperature(temps, fmt.Sprintf("tccd%d", tccdCount), raw)
			tccdCount++
		}
	}
	return temps
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

// labelBeforeColon returns the trimmed sensor label preceding the first ":" in
// s, or "" when the line contains no colon.
func labelBeforeColon(s string) string {
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(s[:idx])
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

// temperatureRe matches the first number in an lm-sensors temperature field.
var temperatureRe = regexp.MustCompile(`[-+]?[0-9]+(?:\.[0-9]+)?`)

// parseCelsius recovers the reading from a sensor field such as "  +48.0°C" or
// "+48.0°C  (high = +84.0°C, crit = +100.0°C)". Only the first number is taken,
// so it works whether or not the field carries threshold annotations.
func parseCelsius(raw string) (float64, bool) {
	match := temperatureRe.FindString(raw)
	if match == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// setTemperature stores a sensor reading under key. It prefers a clean float so
// the UI formats it like every other metric, falling back to the trimmed raw
// text when no number can be recovered.
func setTemperature(temps map[string]interface{}, key, raw string) {
	if raw == "" {
		return
	}
	if value, ok := parseCelsius(raw); ok {
		temps[key] = value
		return
	}
	temps[key] = strings.TrimSpace(raw)
}
