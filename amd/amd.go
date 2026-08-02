package amd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func probeAMD() (bool, int) {
	// check /sys/class/drm/cardX for X in 0..3
	for i := 0; i < 4; i++ {
		cardPath := fmt.Sprintf("/sys/class/drm/card%d/device", i)
		if _, err := os.Stat(cardPath); err == nil {
			log.Println("Found AMD card at", cardPath)
			return true, i
		}
	}
	log.Println("No AMD cards found")
	return false, -1
}

// GetInformations gathers metrics for AMD GPUs.
// It searches the sysfs path to find temperature and power data regardless of hwmon indexing.
func GetInformations() (string, map[string]string) {
	log.Println("amd get infos")

	data := make(map[string]string)
	gpuName := "AMD GPU"

	found, cardIndex := probeAMD()
	if !found {
		log.Println("No AMD cards found, returning default info")
		return gpuName, data
	}

	// Find the primary card (usually card0)
	cardPath := fmt.Sprintf("/sys/class/drm/card%d/device", cardIndex)
	if _, err := os.Stat(cardPath); os.IsNotExist(err) {
		log.Println("Could not find card0, returning default info")
		return gpuName, data
	}

	cardPath += "/hwmon" // Append hwmon to the card path
	// Helper to find the first existing hwmon path under the device /sys/class/drm/card1/device/hwmon/hwmon3/
	hwmonPath := findFirstSubpath(cardPath, "hwmon*")
	if hwmonPath != "" {
		// Temperature is usually in 'temp1_input' (value in millidegrees)
		tempRaw := readSysValue(filepath.Join(hwmonPath, "temp1_input"))
		if val, err := strconv.Atoi(tempRaw); err == nil {
			data["gpu.temp"] = fmt.Sprintf("%d", val/1000)
		} else {
			data["gpu.temp"] = tempRaw
		}

		// GPU hotspot temperature (temp2_input)
		hotspotTempRaw := readSysValue(filepath.Join(hwmonPath, "temp2_input"))
		if val, err := strconv.Atoi(hotspotTempRaw); err == nil {
			data["gpu.hotspot_temp"] = fmt.Sprintf("%d", val/1000)
		} else {
			data["gpu.hotspot_temp"] = hotspotTempRaw
		}

		// VRAM temperature (temp3_input)
		vramTempRaw := readSysValue(filepath.Join(hwmonPath, "temp3_input"))
		if val, err := strconv.Atoi(vramTempRaw); err == nil {
			data["vram_temp"] = fmt.Sprintf("%d", val/1000)
		} else {
			data["vram_temp"] = vramTempRaw
		}
	}

	//kai@Kai-Gaming:~/repos/OpenHWInfo$ cat /sys/class/drm/card1/device/hwmon/hwmon3/temp1_input
	// 45000 gpu
	// kai@Kai-Gaming:~/repos/OpenHWInfo$ cat /sys/class/drm/card1/device/hwmon/hwmon3/temp2_input
	// 49000 gpu hotspot
	// kai@Kai-Gaming:~/repos/OpenHWInfo$ cat /sys/class/drm/card1/device/hwmon/hwmon3/temp3_input
	// 64000 vram

	// Memory and Power info are often in the device directory or via drm info
	// Memory and Power info are often in the device directory or via drm info
	// Note: Some metrics might require specific kernel drivers (amdgpu) /sys/class/drm/card1/device/hwmon/hwmon3/device/mem_info_vram_used
	devicePath := hwmonPath + "/device"

	parseMB := func(fileName string) string {
		raw := readSysValue(filepath.Join(devicePath, fileName))
		if val, err := strconv.Atoi(raw); err == nil {
			return fmt.Sprintf("%d", val/1024/1024) // Assuming input is in KB
		}
		return raw
	}

	data["memory.total"] = parseMB("mem_info_vram_total")                                   // varies by kernel
	data["memory.used"] = parseMB("mem_info_vram_used")                                     // varies by kernel
	data["memory.vendor"] = readSysValue(filepath.Join(devicePath, "mem_info_vram_vendor")) // vendor ID
	// data["gpu.power_draw"] = readSysValue(filepath.Join(hwmonPath, "power1_now"))           // variation of power draw

	// Fallback/Generic fields to maintain compatibility with your mapping
	// data["memory.used"] = "N/A"
	data["gpu.usage"] = readSysValue(filepath.Join(devicePath, "gpu_busy_percent")) // varies by kernel
	rawMaxPower := readSysValue(filepath.Join(hwmonPath, "power1_cap_max"))         // varies by kernel
	if val, err := strconv.Atoi(rawMaxPower); err == nil {
		data["gpu.power.max"] = fmt.Sprintf("%d W", val/1000000)
	} else {
		// Handle parsing error, log or default to 0
		log.Printf("Error parsing gpu.power.max: %v", err)
		data["gpu.power.max"] = "0"
	}
	rawPowerAvg := readSysValue(filepath.Join(hwmonPath, "power1_average")) // varies by kernel
	if val, err := strconv.Atoi(rawPowerAvg); err == nil {
		data["gpu.power.avg"] = fmt.Sprintf("%d W", val/1000000)
	} else {
		log.Printf("Error parsing gpu.power.avg: %v", err)
		data["gpu.power.avg"] = "0"
	}
	data["gpu.power.limit"] = "N/A"
	data["fan.speed"] = "N/A"

	return gpuName, data
}

// findFirstSubpath finds the first path matching a glob pattern (like hwmon*)
func findFirstSubpath(basePath string, pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(basePath, pattern))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// readSysValue reads a file and returns its trimmed content
func readSysValue(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading", path, ":", err)
		return "0"
	}
	return strings.TrimSpace(string(content))
}
