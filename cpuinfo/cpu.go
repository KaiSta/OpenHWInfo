package cpuinfo

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

var (
	cpuVendorIDKey = shared.CPUVendorID.Key
	cpuBrandKey    = shared.CPUBrand.Key
	cpuModelKey    = shared.CPUModel.Key
)

func GetInformations() shared.HardwareData {
	probeCmd := exec.Command("which", "sensors")
	var probeOut bytes.Buffer
	probeCmd.Stdout = &probeOut
	err := probeCmd.Run()
	if probeOut.String() == "" {
		log.Println("CPU probe failed, please install lm-sensors package")
		return shared.HardwareData{Name: "", Values: make(map[string]shared.HardwareValue), IsCPU: true}
	}

	cpuinfos, _ := os.ReadFile("/proc/cpuinfo")
	lines := strings.Split(string(cpuinfos), "\n")
	name, cpus := parseCpuInfo(lines)
	cpus["cpu.model"] = name

	cmd := exec.Command("sensors")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		log.Printf("Error reading sensor data: %v\n", err)
		hwData := shared.HardwareData{Name: name, Values: make(map[string]shared.HardwareValue), IsCPU: true}
		for k, v := range cpus {
			hwData.Values[k] = shared.NewHardwareValue(v, shared.UnknownUnit, shared.Unknown)
		}
		return hwData
	}

	temps := parseSensorOutput(strings.Split(out.String(), "\n"))

	hwData := shared.HardwareData{Name: name, Values: make(map[string]shared.HardwareValue), IsCPU: true}

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

	for i, line := range lines {
		if strings.HasPrefix(line, "CPU") {
			tempStr := strings.TrimLeft(strings.Split(line, ":")[1], " +")
			// Try to parse as numeric value
			if tempValue, err := strconv.ParseFloat(strings.TrimSuffix(tempStr, "C"), 64); err == nil {
				temps["cpu.temp"] = tempValue
			} else {
				temps["cpu.temp"] = tempStr
			}
		} else if strings.Contains(line, "Core") || strings.Contains(line, "Package") {
			tempStr := extractTemp(line)
			// Try to parse as numeric value
			if tempValue, err := strconv.ParseFloat(strings.TrimSuffix(tempStr, "C"), 64); err == nil {
				temps["cpu."+strings.Split(line, ":")[0]] = tempValue
			} else {
				temps["cpu."+strings.Split(line, ":")[0]] = tempStr
			}
		} else if strings.HasPrefix(line, "Tctl") {
			tempStr := strings.TrimLeft(strings.Split(line, ":")[1], " +")
			// Try to parse as numeric value
			if tempValue, err := strconv.ParseFloat(strings.TrimSuffix(tempStr, "C"), 64); err == nil {
				temps["cpu.temp"] = tempValue
			} else {
				temps["cpu.temp"] = tempStr
			}
		} else if strings.HasPrefix(line, "pch") && i+2 < len(lines) {
			tempStr := strings.TrimLeft(strings.Split(lines[i+2], ":")[1], " +")
			// Try to parse as numeric value
			if tempValue, err := strconv.ParseFloat(strings.TrimSuffix(tempStr, "C"), 64); err == nil {
				temps["cpu.pch"] = tempValue
			} else {
				temps["cpu.pch"] = tempStr
			}
		} else if strings.Contains(line, "Tccd") {
			if tccdCount < 6 {
				sensorName := fmt.Sprintf("tccd%d", tccdCount)
				tempStr := extractTemp(line)
				// Try to parse as numeric value
				if tempValue, err := strconv.ParseFloat(strings.TrimSuffix(tempStr, "C"), 64); err == nil {
					temps[sensorName] = tempValue
				} else {
					temps[sensorName] = tempStr
				}
				tccdCount++
			}
		}
	}
	return temps
}

func parseCpuInfo(lines []string) (string, map[string]interface{}) {
	var name string
	var vendorID string
	var brand string
	var model string
	var cpus = make(map[string]interface{})
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			name = strings.TrimSpace(strings.Split(line, ":")[1])
		} else if strings.HasPrefix(line, "vendor_id") {
			vendorID = strings.TrimSpace(strings.Split(line, ":")[1])
		} else if strings.HasPrefix(line, "cpu MHz") {
			cpuMHzStr := strings.TrimSpace(strings.Split(line, ":")[1])
			if cpuMHz, err := strconv.ParseFloat(cpuMHzStr, 64); err == nil {
				cpus["cpu.clock"+strconv.Itoa(len(cpus))] = cpuMHz
			} else {
				cpus["cpu.clock"+strconv.Itoa(len(cpus))] = cpuMHzStr + " MHz"
			}
		}
	}

	// Extract brand and model from name
	if len(name) > 0 && strings.Contains(name, "@") {
		parts := strings.Split(name, "@")
		name = strings.TrimSpace(parts[0])
	}

	// Simple heuristic to extract brand and model
	if len(name) > 0 {
		// Try to find (Ryzen|Core|Pentium|Celeron|Athlon|EPYC|Threadripper)
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

func extractTemp(line string) string {
	elements := strings.Split(line, ":")
	temp := ""
	for _, c := range elements[1] {
		if c == 'C' {
			break
		}
		temp += string(c)
	}
	return temp + "C"
}

func mergeMaps(m1, m2 map[string]string) map[string]string {
	for k, v := range m2 {
		m1[k] = v
	}
	return m1
}
