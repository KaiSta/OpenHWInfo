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

func GetInformations() (string, map[string]string) {
	probeCmd := exec.Command("which", "sensors")
	var probeOut bytes.Buffer
	probeCmd.Stdout = &probeOut
	err := probeCmd.Run()
	if probeOut.String() == "" {
		log.Println("CPU probe failed, please install lm-sensors package")
		return "", nil
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
		return name, cpus
	}

	temps := parseSensorOutput(strings.Split(out.String(), "\n"))
	return name, mergeMaps(cpus, temps)
}

func parseCpuInfo(lines []string) (string, map[string]string) {
	var name string
	var vendorID string
	var brand string
	var model string
	var cpus = make(map[string]string)
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			name = strings.TrimSpace(strings.Split(line, ":")[1])
		} else if strings.HasPrefix(line, "vendor_id") {
			vendorID = strings.TrimSpace(strings.Split(line, ":")[1])
		} else if strings.HasPrefix(line, "cpu MHz") {
			cpus["cpu.clock"+strconv.Itoa(len(cpus))] = strings.TrimSpace(strings.Split(line, ":")[1]) + " MHz"
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

func parseSensorOutput(lines []string) map[string]string {
	var temps = make(map[string]string)
	var tccdCount = 0

	for i, line := range lines {
		if strings.HasPrefix(line, "CPU") {
			temps["cpu.temp"] = strings.TrimLeft(strings.Split(line, ":")[1], " +")
		} else if strings.Contains(line, "Core") || strings.Contains(line, "Package") {
			temps["cpu."+strings.Split(line, ":")[0]] = extractTemp(line)
		} else if strings.HasPrefix(line, "Tctl") {
			temps["cpu.temp"] = strings.TrimLeft(strings.Split(line, ":")[1], " +")
		} else if strings.HasPrefix(line, "pch") && i+2 < len(lines) {
			temps["cpu.pch"] = strings.TrimLeft(strings.Split(lines[i+2], ":")[1], " +")
		} else if strings.Contains(line, "Tccd") {
			if tccdCount < 6 {
				sensorName := fmt.Sprintf("tccd%d", tccdCount)
				temps[sensorName] = extractTemp(line)
				tccdCount++
			}
		}
	}
	return temps
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
