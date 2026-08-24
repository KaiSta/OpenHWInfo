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

// getPCIDeviceID reads the PCI device ID (e.g. "0416") of the Intel GPU at the
// given DRM index directly from sysfs. This is more reliable than parsing lspci
// output because it does not depend on the pci.ids database being installed.
func getPCIDeviceID(cardIndex int) string {
	id, ok := readSysFile(filepath.Join(sysDir, fmt.Sprintf("class/drm/card%d/device", cardIndex), "device"))
	if !ok {
		return ""
	}
	return strings.ToUpper(strings.TrimPrefix(id, "0x"))
}

// getGPUName returns a human readable name for the Intel GPU at the given DRM
// index, preferring a PCI-address-scoped lspci query so that multi-GPU systems
// get the correct name per card. The PCI device ID is read from sysfs (and/or
// parsed from the "[8086:xxxx]" marker in the lspci line) and mapped to a
// specific model name like "Intel HD Graphics 4600". Falls back to glxinfo,
// which provides renderer strings like "Intel(R) UHD Graphics 770", then to a
// cleaned-up lspci device name. Handles integrated graphics (i915 driver).
func getGPUName(cardIndex int) string {
	deviceID := getPCIDeviceID(cardIndex)

	// Method 1: Direct PCI address query - most reliable for multi-GPU systems.
	// "-n" includes the 8086:xxxx device ID so it can be mapped to a model name.
	if pciAddr := resolvePCIDevice(cardIndex); pciAddr != "" {
		line := strings.TrimSpace(runShell("lspci -s " + pciAddr + " -n"))
		// The card index was verified as Intel (vendor 0x8086) by probeIntel, and
		// "-n" output may contain only IDs without the word "Intel", so match on
		// the vendor ID instead of a name.
		if line != "" && regexp.MustCompile(`(?i)\b8086:[0-9a-f]{4}\b`).MatchString(line) {
			return parseIntelGPUName(line, deviceID)
		}
	}

	// Method 2: glxinfo provides the most specific model name (e.g. "Intel(R) UHD Graphics 770").
	// Only trust it when it actually mentions Intel so hybrid systems do not end up
	// naming a discrete GPU (e.g. NVIDIA) after an integrated card.
	line := runShell("glxinfo | grep 'OpenGL renderer'")
	if line != "" {
		parts := strings.Split(line, ":")
		if len(parts) > 1 && strings.Contains(strings.ToLower(parts[1]), "intel") {
			return parseIntelGPUName(strings.TrimSpace(parts[1]), deviceID)
		}
	}

	// Method 3: Check if i915 driver is loaded (indicates Intel GPU present).
	// This handles integrated graphics that don't have a separate DRM device path.
	if _, err := os.Stat("/sys/module/i915"); err == nil {
		// Try lspci for VGA devices with Intel ("-n" exposes the [8086:xxxx] ID).
		line = runShell("lspci -nnk | grep -i 'vga' | grep -i intel")
		if line != "" {
			return parseIntelGPUName(line, deviceID)
		}

		// Fallback: check glxinfo again (handles both discrete and integrated GPUs).
		line = runShell("glxinfo 2>/dev/null | grep 'OpenGL renderer' || lspci -nnk | grep -i intel")
		if line != "" {
			return parseIntelGPUName(line, deviceID)
		}

		// Last resort: generic Intel GPU detection.
		name := firstNonEmpty(
			runShell("lspci | grep -i 'intel'"),
			runShell("glxinfo 2>/dev/null | grep renderer"),
		)
		if name != "" {
			return parseIntelGPUName(name, deviceID)
		}

		return "Intel UHD Graphics" // Generic fallback for integrated GPUs without specific info
	}

	// Method 4: Generic lspci intel grep as last resort (handles some edge cases).
	name := firstNonEmpty(
		runShell("lspci | grep -i 'intel'"),
		runShell("glxinfo | grep -i renderer"),
	)
	if name != "" {
		return parseIntelGPUName(name, deviceID)
	}

	return "Intel GPU"
}

// parseIntelGPUName extracts a human readable Intel GPU model from an lspci
// output line or OpenGL renderer string. PCI device IDs (provided explicitly,
// e.g. from sysfs, or embedded in the line as "[8086:xxxx]") are mapped to
// specific marketing names first; branded renderer strings like "Intel(R) UHD
// Graphics 630" are normalized next; otherwise the cleaned-up lspci device
// name is returned. Examples:
//   - "…4th Gen Core Processor Integrated Graphics Controller [8086:0416] (rev 06)" -> "Intel HD Graphics 4600"
//   - "Intel(R) UHD Graphics 630" -> "Intel UHD Graphics 630"
func parseIntelGPUName(line string, deviceIDs ...string) string {
	line = strings.TrimSpace(line)

	// Collect candidate PCI device IDs: the explicit ones plus any 8086:xxxx
	// marker found in the line itself (with or without "[...]" brackets).
	var ids []string
	for _, id := range deviceIDs {
		if id != "" {
			ids = append(ids, strings.ToUpper(id))
		}
	}
	if m := regexp.MustCompile(`(?i)\b8086:([0-9a-f]{4})\b`).FindStringSubmatch(line); len(m) >= 2 {
		ids = append(ids, strings.ToUpper(m[1]))
	}

	for _, id := range ids {
		if model := getIntelDeviceModel(id); model != "" {
			return model
		}
	}

	// Branded renderer/device names (e.g. "Intel(R) UHD Graphics 630",
	// "Intel Iris Xe Graphics"): normalize the "(R)" trademark marks.
	cleaned := cleanPCIDeviceName(line)
	lower := strings.ToLower(cleaned)
	if strings.Contains(lower, "intel") && strings.Contains(lower, "graphics") {
		return strings.TrimSpace(strings.ReplaceAll(cleaned, "(R)", ""))
	}

	return cleaned
}

// getIntelDeviceModel maps Intel PCI device IDs to their human-readable model names.
func getIntelDeviceModel(deviceID string) string {
	deviceID = strings.ToUpper(strings.TrimSpace(deviceID))
	if deviceID == "" {
		return ""
	}
	mappings := map[string]string{
		// Discrete Arc GPUs:
		"9A49": "Intel Arc A380",
		"7D2B": "Intel Arc A580",
		"61ED": "Intel Arc A770",
		"61E1": "Intel Arc A750",
		"61AE": "Intel Arc A310",
		"9BC4": "Intel Arc B580",
		"9BC2": "Intel Arc B570",
		"9BC6": "Intel Arc B550",
		"7CFD": "Intel Arc A310M (mobile)",
		"7DAA": "Intel Arc A380M (mobile)",
		// Iris Xe:
		"63E0": "Intel Iris Xe Graphics G4 (Tiger Lake)",
		"63C8": "Intel Iris Xe Graphics G5 (Alder Lake-P)",
		"9A26": "Intel Iris Xe Graphics G7 (Tiger Lake-HX)",
		// Kaby Lake / Skylake UHD:
		"591C": "Intel UHD Graphics 630",
		"591B": "Intel UHD Graphics P630",
		"5912": "Intel UHD Graphics 615",
		"5916": "Intel UHD Graphics 617",
		"5915": "Intel UHD Graphics 612",
		"5914": "Intel UHD Graphics P600",
		// Haswell HD:
		"0C0B": "Intel HD Graphics 510",
		"0C0A": "Intel HD Graphics 530",
		"0C09": "Intel HD Graphics 520",
		"0C08": "Intel HD Graphics 515",
		"0C07": "Intel HD Graphics 5500",
		"0416": "Intel HD Graphics 4600",
		"0412": "Intel HD Graphics 4600",
		"0411": "Intel HD Graphics 4400",
		// Ivy Bridge:
		"0166": "Intel HD Graphics 4000",
		"0162": "Intel HD Graphics 4000",
		"0152": "Intel HD Graphics 4000",
		"0156": "Intel Iris",
		// Sandy Bridge / Atom:
		"0134": "Intel HD Graphics (Sandy Bridge)",
		"012E": "Intel HD Graphics",
	}
	return mappings[deviceID]
}

// cleanPCIDeviceName strips the PCI address/class prefix, vendor/device ID and
// revision suffix from an lspci output line so only the device name remains.
func cleanPCIDeviceName(name string) string {
	name = strings.TrimSpace(name)
	// Strip trailing markers like "(rev 06)" and "[8086:0416]".
	name = regexp.MustCompile(`\s*\((?:rev|subsystem)\s+[0-9a-fA-F]+\)$`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`\s*\[[0-9a-f]{4}:[0-9a-f]{4}\]$`).ReplaceAllString(name, "")
	// Strip the leading "XX:YY.Z VGA compatible controller [NNNN]:" prefix.
	name = regexp.MustCompile(`^[^ ]+\s+VGA compatible controller(?:\s*\[\d{4}\])?:?\s*`).ReplaceAllString(name, "")
	return strings.TrimSpace(name)
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
