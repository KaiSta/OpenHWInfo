package intel

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

var sysDir = "/sys"

// intelVendorID is the PCI vendor ID for Intel devices (0x8086).
const intelVendorID = "0x8086"

// Available reports whether an Intel graphics card is present on the system.
func Available() bool {
	cards, _ := probeIntel()
	return len(cards) > 0
}

// probeIntel scans the DRM cards and returns the indices of all Intel cards,
// identified by their PCI vendor ID.
func probeIntel() ([]int, error) {
	var indices []int
	for i := 0; i < 16; i++ {
		devicePath := filepath.Join(sysDir, fmt.Sprintf("class/drm/card%d/device", i))
		if _, err := os.Stat(devicePath); err != nil {
			continue
		}
		if vendor, ok := readSysFile(filepath.Join(devicePath, "vendor")); ok && vendor == intelVendorID {
			log.Printf("Found Intel card at index %d\n", i)
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		log.Println("No Intel cards found")
	}
	return indices, nil
}

// resolvePCIDevice reads the symlink at class/drm/cardN/device and returns
// the PCI address (e.g. "0000:00:02.0"). Empty string on error.
func resolvePCIDevice(cardIndex int) string {
	link, err := os.Readlink(filepath.Join(sysDir, fmt.Sprintf("class/drm/card%d/device", cardIndex)))
	if err != nil {
		return ""
	}
	parts := strings.Split(link, "/")
	return parts[len(parts)-1]
}

// getGPUName returns a human readable name for the Intel GPU at the given DRM
// index, preferring a PCI-address-scoped lspci query so that multi-GPU systems
// get the correct name per card. Falls back to glxinfo which provides specific
// model names like "Intel(R) UHD Graphics 770" or "Intel(R) HD Graphics 4600",
// then falls through multiple lspci patterns to extract specific Intel GPU identifiers.
// Handles integrated graphics (i915 driver, no separate DRM device path).
func getGPUName(cardIndex int) string {
	// Method 1: Direct PCI address query - most reliable for multi-GPU systems
	if pciAddr := resolvePCIDevice(cardIndex); pciAddr != "" {
		line := strings.TrimSpace(runShell("lspci -s " + pciAddr))
		if line != "" && strings.Contains(strings.ToLower(line), "intel") {
			return parseIntelGPUName(line)
		}
	}

	// Method 2: glxinfo provides the most specific model name (e.g. "Intel(R) UHD Graphics 770" or "Intel(R) HD Graphics 4600")
	line := runShell("glxinfo | grep 'OpenGL renderer'")
	if line != "" {
		parts := strings.Split(line, ":")
		if len(parts) > 1 {
			renderer := strings.TrimSpace(parts[1])
			return parseIntelGPUName(renderer)
		}
	}

	// Method 3: Check if i915 driver is loaded (indicates Intel GPU present)
	// This handles integrated graphics that don't have a separate DRM device path
	if _, err := os.Stat("/sys/module/i915"); err == nil {
		// Try lspci for VGA devices with Intel
		line = runShell("lspci -nnk | grep -i 'vga' | grep -i intel")
		if line != "" {
			return parseIntelGPUName(line)
		}

		// Fallback: check glxinfo again (handles both discrete and integrated GPUs)
		line = runShell("glxinfo 2>/dev/null | grep 'OpenGL renderer' || lspci -nnk | grep -i intel")
		if line != "" {
			return parseIntelGPUName(line)
		}

		// Last resort: generic Intel GPU detection
		name := firstNonEmpty(
			runShell("lspci | grep -i 'intel'"),
			runShell("glxinfo 2>/dev/null | grep renderer"),
		)
		if name != "" {
			return parseIntelGPUName(name)
		}

		return "Intel UHD Graphics" // Generic fallback for integrated GPUs without specific info
	}

	// Method 4: Generic lspci intel grep as last resort (handles some edge cases)
	name := firstNonEmpty(
		runShell("lspci | grep -i 'intel'"),
		runShell("glxinfo | grep -i renderer"),
	)
	if name != "" {
		return parseIntelGPUName(name)
	}

	return "Intel GPU"
}

// parseIntelGPUName extracts and formats the Intel GPU model from an lspci output line.
// It handles various output formats including:
//   - "Intel Corporation Device 9a49 (rev xx)" -> "Intel Arc A380"
//   - "Intel(R) UHD Graphics 630" -> "Intel UHD Graphics 630"
//   - "Intel(R) Iris Xe Graphics G7" -> "Intel Iris Xe Graphics G7"
//   - "Intel(R) UHD Graphics X590" -> "Intel UHD Graphics X590"
func parseIntelGPUName(line string) string {
	line = strings.TrimSpace(line)

	// Check for Intel branded graphics (most specific)
	if matched, _ := regexp.MatchString(`(Intel|R)\s+[A-Z][a-z]+\s+Graphics\s+[A-Za-z0-9]+`, line); matched {
		return strings.ReplaceAll(strings.TrimSpace(line), "Intel(R)", "Intel")
	}

	// Check for Intel Corporation Device XXXX format (needs mapping)
	if matched, _ := regexp.MatchString(`intel corporation device ([a-f0-9]{4})`, strings.ToLower(line)); matched {
		deviceID := extractHexMatch(line, `([a-f0-9]{4})`)
		model := getIntelDeviceModel(deviceID)
		return model + " (PCI: " + cleanPCIDeviceName(strings.TrimSpace(line)) + ")"
	}

	// Check for Intel Corporation Graphics Device XXXX format
	if matched, _ := regexp.MatchString(`intel corporation graphics device ([a-f0-9]{4})`, strings.ToLower(line)); matched {
		deviceID := extractHexMatch(line, `([a-f0-9]{4})`)
		model := getIntelDeviceModel(deviceID)
		return model + " (PCI: " + cleanPCIDeviceName(strings.TrimSpace(line)) + ")"
	}

	return cleanPCIDeviceName(line)
}

// extractHexMatch extracts a hex match from a line using a regex pattern.
func extractHexMatch(line, pattern string) string {
	matches := regexp.MustCompile(pattern).FindStringSubmatch(line)
	if len(matches) >= 2 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// getIntelDeviceModel maps Intel PCI device IDs to their human-readable names.
func getIntelDeviceModel(deviceID string) string {
	// Use a slice-based lookup since some device IDs can map to multiple models.
	// The first match wins (order matters).
	mappings := []struct {
		id      string
		model   string
		priority int // lower = higher priority, so these come first
	}{
		{"9a49", "Intel Arc A380", 1},
		{"7d2b", "Intel Arc A580", 2},
		{"61ed", "Intel Arc A770", 3},
		{"61e1", "Intel Arc A750", 4},
		{"61ae", "Intel Arc A310", 5},
		{"9bc4", "Intel Arc B580", 6},
		{"9bc2", "Intel Arc B570", 7},
		{"9bc6", "Intel Arc B550", 8},
		{"7cfd", "Intel Arc A310M (mobile)", 9},
		{"4bba", "Intel Arc A580V (Arc Max)", 10}, // Arc Max variant
		{"7daa", "Intel Arc A380M (mobile)", 11},
		// These are less common variants of the same PCI ID:
		{"4bba", "Intel UHD Graphics X590", 20},     // appears after Arc Max, lower priority
		{"7d2b", "Intel UHD Graphics A360V", 21},   // appears after Arc A580, lower priority
		// Legacy Iris Xe devices:
		{"63e0", "Intel Iris Xe Graphics G4 (Tiger Lake)", 22},
		{"63c8", "Intel Iris Xe Graphics G5 (Alder Lake-P)", 23},
		{"9a26", "Intel Iris Xe Graphics G7 (Tiger Lake-HX)", 24},
	}

	for _, m := range mappings {
		if strings.ToUpper(deviceID) == m.id {
			return m.model
		}
	}
	return ""
}

// cleanPCIDeviceName strips vendor/device prefixes from a PCI name to extract the model.
func cleanPCIDeviceName(name string) string {
	name = strings.TrimSpace(strings.TrimSuffix(name, "(rev xx)"))
	name = strings.TrimSpace(strings.TrimSuffix(name, " (rev xx)"))
	parts := strings.Split(name, "(")
	if len(parts) > 1 {
		return parts[0]
	}
	return name
}

// parseGPUNameFromLspci is a legacy alias for backward compatibility.
func parseGPUNameFromLspci(line string) string {
	line = strings.TrimSpace(line)
	if matched, _ := regexp.MatchString(`(intel|r)\s+[a-z]+\s+graphics\s+`, line); matched {
		return cleanPCIDeviceName(line)
	}

	// Handle "Intel Corporation Device XXXX" format
	if strings.Contains(strings.ToLower(line), "intel corporation device") {
		deviceID := extractHexMatch(line, `([a-f0-9]{4})`)
		model := getIntelDeviceModel(deviceID)
		return model + " (PCI: " + cleanPCIDeviceName(line) + ")"
	}

	return cleanPCIDeviceName(line)
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

// collectGPUDataForCard collects the hardware metrics available for a single
// Intel GPU at the given card path. Integrated graphics do not expose discrete
// VRAM, fan speed or busy percent, so those keys are only set when the
// underlying sysfs file is present.
func collectGPUDataForCard(cardPath string, cardIndex int) shared.HardwareData {
	data := make(map[string]shared.HardwareValue)
	gpuName := getGPUName(cardIndex)
	data[shared.GPUName.Key] = shared.NewHardwareValue(gpuName, shared.UnknownUnit, shared.Unknown)
	data[shared.GPUDriverVersion.Key] = shared.NewHardwareValue(getDriverVersion(), shared.UnknownUnit, shared.Unknown)

	cardPath = filepath.Join(cardPath, "hwmon")
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

// GetInformations collects the hardware metrics available for the first Intel GPU.
func GetInformations() shared.HardwareData {
	all := GetAllInformations()
	if len(all) == 0 {
		return shared.HardwareData{Name: "Intel GPU", Values: make(map[string]shared.HardwareValue), IsGPU: true}
	}
	return all[0]
}

// GetAllInformations returns the information for every Intel card on the system.
func GetAllInformations() []shared.HardwareData {
	cards, err := probeIntel()
	if err != nil || len(cards) == 0 {
		log.Println("Returning default info due to no Intel cards found")
		return []shared.HardwareData{
			{Name: "Intel GPU", Values: make(map[string]shared.HardwareValue), IsGPU: true},
		}
	}

	var result []shared.HardwareData
	for _, cardIndex := range cards {
		cardPath := filepath.Join(sysDir, fmt.Sprintf("class/drm/card%d/device", cardIndex))
		log.Printf("Collecting information for Intel card %d\n", cardIndex)
		result = append(result, collectGPUDataForCard(cardPath, cardIndex))
	}
	return result
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
