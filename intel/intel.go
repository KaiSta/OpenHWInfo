package intel

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

var sysDir = "/sys"

// intelVendorID is the PCI vendor ID for Intel devices (0x8086).
const intelVendorID = "0x8086"

// Available reports whether an Intel graphics card is present on the system.
func Available() bool {
	found, _ := probeIntel()
	return found
}

// probeIntel scans the DRM cards and returns the index of the first Intel card,
// identified by its PCI vendor ID.
func probeIntel() (bool, int) {
	for i := 0; i < 8; i++ {
		devicePath := filepath.Join(sysDir, fmt.Sprintf("class/drm/card%d/device", i))
		if _, err := os.Stat(devicePath); err != nil {
			continue
		}
		if vendor, ok := readSysFile(filepath.Join(devicePath, "vendor")); ok && vendor == intelVendorID {
			log.Printf("Found Intel card at index %d\n", i)
			return true, i
		}
	}
	log.Println("No Intel cards found")
	return false, -1
}

// getGPUName returns a human readable name for the Intel GPU, falling back to
// the OpenGL renderer string and finally a generic label.
func getGPUName() string {
	name := firstNonEmpty(
		runShell("lspci | grep -i 'intel' | grep -iE 'vga|3d'"),
		runShell("lspci | grep -i 'intel'"),
		openglRenderer(),
	)
	if name != "" {
		return name
	}
	return "Intel GPU"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func runShell(command string) string {
	output, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		log.Printf("Error executing command %q: %v\n", command, err)
		return ""
	}
	return strings.TrimSpace(string(output))
}

func openglRenderer() string {
	line := runShell("glxinfo | grep 'OpenGL renderer'")
	parts := strings.Split(line, ":")
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// getDriverVersion returns the i915 driver version (the kernel version the
// driver was built with), read from sysfs with a modinfo fallback.
func getDriverVersion() string {
	if version, ok := readSysFile(filepath.Join(sysDir, "module/i915/version")); ok && version != "" {
		return version
	}
	if version := runShell("modinfo i915 | awk '/^version:/ {print $2}'"); version != "" {
		return version
	}
	return "Intel Driver"
}

// GetInformations collects the hardware metrics available for the Intel GPU.
// Integrated graphics do not expose discrete VRAM, fan speed or busy percent,
// so those keys are only set when the underlying sysfs file is present.
func GetInformations() shared.HardwareData {
	data := make(map[string]shared.HardwareValue)
	gpuName := getGPUName()
	data[shared.GPUName.Key] = shared.NewHardwareValue(gpuName, shared.UnknownUnit, shared.Unknown)
	data[shared.GPUDriverVersion.Key] = shared.NewHardwareValue(getDriverVersion(), shared.UnknownUnit, shared.Unknown)

	found, cardIndex := probeIntel()
	if !found {
		log.Println("Returning default info due to no Intel cards found")
		return shared.HardwareData{Name: gpuName, Values: data, IsGPU: true}
	}

	cardPath := filepath.Join(sysDir, fmt.Sprintf("class/drm/card%d/device/hwmon", cardIndex))
	hwmonPath := findFirstSubpath(cardPath, "hwmon*")
	if hwmonPath == "" {
		log.Println("No hwmon path found for Intel card")
		return shared.HardwareData{Name: gpuName, Values: data, IsGPU: true}
	}

	// Temperatures (i915 reports millidegrees Celsius).
	addTemperature(&data, hwmonPath, "temp1_input", shared.GPUTemp.Key)
	addTemperature(&data, hwmonPath, "temp2_input", shared.GPUHotspotTemp.Key)

	// GPU frequency (i915 reports Hz).
	for _, file := range []string{"freq0_input", "freq1_input"} {
		if v, ok := readSysFile(filepath.Join(hwmonPath, file)); ok {
			data[shared.GPUFrequency.Key] = shared.NewHardwareValue(parseFrequencyValue(v), shared.MHz, shared.Frequency)
			break
		}
	}

	// Average power draw (i915 reports microwatts).
	if v, ok := readSysFile(filepath.Join(hwmonPath, "power1_average")); ok {
		data[shared.GPUPowerAvg.Key] = shared.NewHardwareValue(parsePowerValue(v), shared.Watt, shared.Power)
	}

	// Fan speed (rarely present on integrated graphics).
	if v, ok := readSysFile(filepath.Join(hwmonPath, "fan1_input")); ok {
		if rpm, err := strconv.Atoi(v); err == nil {
			data[shared.FanSpeed.Key] = shared.NewHardwareValue(float64(rpm), shared.RPM, shared.Frequency)
		}
	}

	return shared.HardwareData{Name: gpuName, Values: data, IsGPU: true}
}

func findFirstSubpath(basePath string, pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(basePath, pattern))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func readSysFile(path string) (string, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(content)), true
}

func addTemperature(data *map[string]shared.HardwareValue, hwmonPath string, fileName string, key string) {
	raw, ok := readSysFile(filepath.Join(hwmonPath, fileName))
	if !ok {
		return
	}
	if val, err := strconv.Atoi(raw); err == nil {
		(*data)[key] = shared.NewHardwareValue(float64(val/1000), shared.Celsius, shared.Temperature)
	}
}

func parseFrequencyValue(value string) float64 {
	freq, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("Error parsing frequency value %q: %v\n", value, err)
		return 0
	}
	return freq / 1000000 // Convert Hz to MHz
}

func parsePowerValue(raw string) float64 {
	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || val < 0 {
		return 0
	}
	return float64(val / 1000000) // Convert microwatts to watts
}
