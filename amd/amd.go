package amd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

func Available() bool {
	found, _ := probeAMD()
	return found
}

func probeAMD() (bool, int) {
	for i := 0; i < 4; i++ {
		if _, err := os.Stat(fmt.Sprintf("/sys/class/drm/card%d/device", i)); err == nil {
			log.Printf("Found AMD card at index %d\n", i)
			return true, i
		}
	}
	log.Println("No AMD cards found")
	return false, -1
}

func getGPUName() string {
	cmd := exec.Command("sh", "-c", "lspci | grep -i 'vga\\|3d'")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing lspci command: %v\n", err)
		return "AMD GPU"
	}
	gpuName := strings.TrimSpace(string(output))

	// Split into parts and find the manufacturer name
	parts := strings.Fields(gpuName)
	for i, part := range parts {
		if part == "Advanced" && len(parts) > i+1 && parts[i+1] == "Micro" {
			// Return everything from "Advanced Micro Devices..." onwards
			return strings.Join(parts[i:], " ")
		}
	}
	return gpuName
}

func getGPUName2() string {
	cmd := exec.Command("sh", "-c", "glxinfo | grep 'OpenGL renderer'")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing glxinfo command: %v\n", err)
		return "AMD GPU"
	}
	gpuName := strings.TrimSpace(string(output))
	// Extract just the renderer name
	parts := strings.Split(gpuName, ":")
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return gpuName
}

func GetInformations() shared.HardwareData {
	data := make(map[string]shared.HardwareValue)
	gpuName := getGPUName2()
	data[shared.GPUName.Key] = shared.NewHardwareValue(gpuName, shared.UnknownUnit, shared.Unknown)

	found, cardIndex := probeAMD()
	if !found {
		log.Println("Returning default info due to no AMD cards found")
		return shared.HardwareData{Name: gpuName, Values: data, IsGPU: true}
	}

	cardPath := fmt.Sprintf("/sys/class/drm/card%d/device/hwmon", cardIndex)
	hwmonPath := findFirstSubpath(cardPath, "hwmon*")
	if hwmonPath == "" {
		log.Println("No hwmon path found")
		return shared.HardwareData{Name: gpuName, Values: data, IsGPU: true}
	}

	parseTemperatureValue(&data, hwmonPath, "temp1_input", shared.GPUTemp.Key)
	parseTemperatureValue(&data, hwmonPath, "temp2_input", shared.GPUHotspotTemp.Key)
	parseTemperatureValue(&data, hwmonPath, "temp3_input", shared.VRAMTemp.Key)

	// Read GPU frequency (freq1_input) and convert to MHz
	gpuFreq := readSysValue(filepath.Join(hwmonPath, "freq1_input"))
	if gpuFreq != "" {
		data[shared.GPUFrequency.Key] = shared.NewHardwareValue(parseFrequencyValue(gpuFreq), shared.MHz, shared.Frequency)
	}

	// Read memory frequency (freq2_input) and convert to MHz
	memFreq := readSysValue(filepath.Join(hwmonPath, "freq2_input"))
	if memFreq != "" {
		data[shared.VRAMLrequency.Key] = shared.NewHardwareValue(parseFrequencyValue(memFreq), shared.MHz, shared.Frequency)
	}

	// Append hwmon to the card path
	devicePath := filepath.Join(hwmonPath, "device")

	data[shared.VRAMTotal.Key] = shared.NewHardwareValue(parseMemoryValue(filepath.Join(devicePath, "mem_info_vram_total")), shared.Megabyte, shared.Memory)
	data[shared.VRAMUsed.Key] = shared.NewHardwareValue(parseMemoryValue(filepath.Join(devicePath, "mem_info_vram_used")), shared.Megabyte, shared.Memory)
	data[shared.VRAMVendor.Key] = shared.NewHardwareValue(readSysValue(filepath.Join(devicePath, "mem_info_vram_vendor")), shared.UnknownUnit, shared.Unknown)

	data[shared.GPUUsage.Key] = shared.NewHardwareValue(parseGPUUsage(readSysValue(filepath.Join(devicePath, "gpu_busy_percent"))), shared.Percent, shared.Usage)
	data[shared.GPUPowerMax.Key] = shared.NewHardwareValue(parsePowerValue(readSysValue(filepath.Join(hwmonPath, "power1_cap_max")), 1000000), shared.Watt, shared.Power)
	data[shared.GPUPowerAvg.Key] = shared.NewHardwareValue(parsePowerValue(readSysValue(filepath.Join(hwmonPath, "power1_average")), 1000000), shared.Watt, shared.Power)
	data[shared.FanSpeed.Key] = shared.NewHardwareValue(readSysValue(filepath.Join(hwmonPath, "fan1_input")), shared.RPM, shared.Frequency)

	return shared.HardwareData{Name: gpuName, Values: data, IsGPU: true}
}

func findFirstSubpath(basePath string, pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(basePath, pattern))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func readSysValue(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Error reading %s: %v\n", path, err)
		return "0"
	}
	return strings.TrimSpace(string(content))
}

func parseTemperatureValue(data *map[string]shared.HardwareValue, hwmonPath string, fileName string, dataKey ...string) {
	if val, err := strconv.Atoi(readSysValue(filepath.Join(hwmonPath, fileName))); err == nil {
		key := shared.GPUTemp.Key
		if len(dataKey) > 0 {
			key = dataKey[0]
		}
		(*data)[key] = shared.NewHardwareValue(float64(val/1000), shared.Celsius, shared.Temperature)
	}
}

func parseMemoryValue(path string) float64 {
	if val, err := strconv.Atoi(readSysValue(path)); err == nil {
		return float64(val / 1024 / 1024) // Convert KB to MB
	}
	return 0
}

func parsePowerValue(raw string, divisor int) float64 {
	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || val < 0 {
		log.Printf("Error parsing power value: %v\n", err)
		return 0
	}
	return float64(val / divisor)
}

func parseFrequencyValue(value string) float64 {
	if value == "" {
		return 0
	}

	freq, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("Error parsing frequency value: %v", err)
		return 0
	}

	// Convert Hz to MHz
	mhz := freq / 1000000
	return mhz
}

func parseGPUUsage(value string) float64 {
	if value == "" {
		return -1
	}

	// Try to convert to numeric value
	if val, err := strconv.Atoi(value); err == nil {
		return float64(val)
	}

	return -1
}
