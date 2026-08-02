package amd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

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

	// // Use regex to extract the Radeon model from the full name
	// re := regexp.MustCompile(`Radeon.*?\S`)
	// match := re.FindString(gpuName)

	// if match != "" {
	// 	return match
	// }

	return gpuName
}

func GetInformations() (string, map[string]string) {
	data := make(map[string]string)
	gpuName := getGPUName()
	data["name"] = gpuName

	found, cardIndex := probeAMD()
	if !found {
		log.Println("Returning default info due to no AMD cards found")
		return gpuName, data
	}

	cardPath := fmt.Sprintf("/sys/class/drm/card%d/device/hwmon", cardIndex)
	if hwmonPath := findFirstSubpath(cardPath, "hwmon*"); hwmonPath != "" {
		parseTemperature(&data, hwmonPath, "temp1_input", "gpu.temp")
		parseTemperature(&data, hwmonPath, "temp2_input", "gpu.hotspot_temp")
		parseTemperature(&data, hwmonPath, "temp3_input", "vram.temp")
	} else {
		log.Println("No hwmon path found")
	}
	// Append hwmon to the card path
	fmt.Println("Card Path:", cardPath) // Debugging line to print the card path
	hwmonPath := findFirstSubpath(cardPath, "hwmon*")
	fmt.Println("HWMon Path:", hwmonPath) // Debugging line to print the hwmon path
	devicePath := hwmonPath + "/device"
	fmt.Println("Device Path:", devicePath) // Debugging line to print the device path

	data["vram.total"] = parseMemoryValue(filepath.Join(devicePath, "mem_info_vram_total"))
	data["vram.used"] = parseMemoryValue(filepath.Join(devicePath, "mem_info_vram_used"))
	data["vram.vendor"] = readSysValue(filepath.Join(devicePath, "mem_info_vram_vendor"))

	data["gpu.usage"] = readSysValue(filepath.Join(devicePath, "gpu_busy_percent"))
	data["gpu.power.max"] = parsePowerValue(readSysValue(filepath.Join(hwmonPath, "power1_cap_max")), 1000000)
	data["gpu.power.avg"] = parsePowerValue(readSysValue(filepath.Join(hwmonPath, "power1_average")), 1000000)
	data["fan.speed"] = readSysValue(filepath.Join(hwmonPath, "fan1_input"))

	return gpuName, data
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

func parseTemperature(data *map[string]string, hwmonPath string, fileName string, dataKey ...string) {
	if val, err := strconv.Atoi(readSysValue(filepath.Join(hwmonPath, fileName))); err == nil {
		key := "gpu.temp"
		if len(dataKey) > 0 {
			key = dataKey[0]
		}
		(*data)[key] = fmt.Sprintf("%d °C", val/1000)
	}
}

func parseMemoryValue(path string) string {
	if val, err := strconv.Atoi(readSysValue(path)); err == nil {
		return fmt.Sprintf("%d MB", val/1024/1024) // Convert KB to MB
	}
	return "N/A"
}

func parsePowerValue(raw string, divisor int) string {
	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || val < 0 {
		log.Printf("Error parsing power value: %v\n", err)
		return "N/A"
	}
	return fmt.Sprintf("%d W", val/divisor)
}
