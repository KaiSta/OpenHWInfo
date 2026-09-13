package intel

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/KaiSta/OpenHWInfo/shared"
)

var sysDir = "/sys"

// intelVendorID is the PCI vendor ID for Intel devices (0x8086).
const intelVendorID = "0x8086"

// probeIntel scans the DRM cards and returns the indices of all Intel cards,
// identified by their PCI vendor ID.
func probeIntel() ([]int, error) {
	indices := shared.ProbeDRMCards(sysDir, intelVendorID)
	for _, i := range indices {
		log.Printf("Found Intel card at index %d\n", i)
	}
	if len(indices) == 0 {
		log.Println("No Intel cards found")
	}
	return indices, nil
}

// getPCIDeviceID reads the PCI device ID (e.g. "0416") of the Intel GPU at the
// given DRM index directly from sysfs. This is more reliable than parsing lspci
// output because it does not depend on the pci.ids database being installed.
func getPCIDeviceID(cardIndex int) string {
	id, ok := shared.ReadSysFile(filepath.Join(shared.DRMCardDevicePath(sysDir, cardIndex), "device"))
	if !ok {
		return ""
	}
	return strings.ToUpper(strings.TrimPrefix(id, "0x"))
}

// getGPUName returns a human readable name for the Intel GPU at the given DRM
// index. The name is fixed for the lifetime of the app, so it is resolved once
// per card rather than re-running lspci or glxinfo on every poll; the cache key
// carries the sysfs root and the card identity so two cards — and two tests'
// fixture trees — never share one name.
func getGPUName(cardIndex int) string {
	deviceID := getPCIDeviceID(cardIndex)
	pciAddr := shared.ResolvePCIDevice(sysDir, cardIndex)
	key := "intel.gpuName|" + sysDir + "|" + strconv.Itoa(cardIndex) + "|" + deviceID + "|" + pciAddr
	return shared.CachedProbe(key, func() string { return resolveIntelGPUName(deviceID, pciAddr) })
}

// resolveIntelGPUName runs the actual naming methods. The PCI device ID from
// sysfs identifies the model exactly, so that is tried first; multi-GPU systems
// are handled because the ID is per card. Falls back to glxinfo, which provides
// renderer strings like "Intel(R) UHD Graphics 770", then to a PCI-scoped lspci
// query that does carry a product name.
func resolveIntelGPUName(deviceID, pciAddr string) string {
	// Method 1: PCI device ID -> marketing name. This is the only method that
	// names the exact model, and it needs no external command.
	if model := getIntelDeviceModel(deviceID); model != "" {
		return model
	}

	// Method 2: PCI-address-scoped lspci. The query deliberately omits "-n":
	// the numeric form carries no product name ("00:02.0 0300: 8086:0416"),
	// while the named form gives "Intel Corporation 4th Gen Core Processor
	// Integrated Graphics Controller" using the pci.ids database.
	if pciAddr != "" {
		line := strings.TrimSpace(shared.RunCommand("lspci", "-s", pciAddr))
		if line != "" {
			if name := parseIntelGPUName(line, deviceID); name != "" {
				return name
			}
		}
	}

	// Methods 3+: each source yields a candidate string, and the first one that
	// parses into a usable name wins. glxinfo comes first because it reports the
	// most specific string (e.g. "Intel(R) UHD Graphics 770"); it is only trusted
	// when it mentions Intel, so hybrid systems do not end up naming a discrete
	// GPU (an NVIDIA card) after an integrated one.
	var candidates []string
	glxinfo := shared.RunShell("glxinfo | grep 'OpenGL renderer'")
	if parts := strings.Split(glxinfo, ":"); len(parts) > 1 &&
		strings.Contains(strings.ToLower(parts[1]), "intel") {
		candidates = append(candidates, strings.TrimSpace(parts[1]))
	}

	// The i915 check covers integrated graphics without a usable DRM device
	// path. It is not a requirement: Arc cards run on either i915 or xe.
	if _, err := os.Stat(filepath.Join(sysDir, "module/i915")); err == nil {
		candidates = append(candidates,
			shared.RunShell("lspci -nnk | grep -i 'vga' | grep -i intel"),
			shared.RunShell("lspci -nnk | grep -iE 'vga|3d|display' | grep -i intel"),
		)
	}

	// Each query is filtered to display devices: a bare `lspci | grep -i intel`
	// matches every Intel device on the system, and naming a GPU after the whole
	// device list is worse than admitting the name is unknown.
	candidates = append(candidates,
		shared.RunShell("lspci | grep -iE 'vga|3d|display' | grep -i intel"),
		shared.RunShell("glxinfo 2>/dev/null | grep -i renderer"),
	)

	for _, candidate := range candidates {
		if name := parseIntelGPUName(firstLine(candidate), deviceID); name != "" {
			return name
		}
	}

	// The card is known to be Intel (probeIntel matched the vendor) but nothing
	// could name it. "Intel Graphics" is vague; a wrong model name is worse.
	return "Intel Graphics"
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

	// Anything left is only a name if it reads like one. The numeric lspci
	// form ("00:02.0 0300: 8086:0416 (rev 06)") is what the collector queries,
	// and returning it unparsed would show a card name of "00:02.0 0300:
	// 8086:0416". Reporting "" instead lets getGPUName try a query that does
	// carry a name.
	if looksLikeDeviceName(cleaned) {
		return cleaned
	}
	return ""
}

// firstLine returns the first non-empty line of s, trimmed. A card name is one
// line: the fallback queries can still match more than one device, and the first
// line is the best guess among them.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// hexLetters are the letters that also occur in hex numbers, so their presence
// in a string proves nothing about it being a name.
const hexLetters = "abcdefABCDEF"

// looksLikeDeviceName reports whether text could plausibly be a product name
// rather than raw PCI identifiers. The numeric lspci form
// ("00:02.0 0300: 8086:0416") is made only of digits, separators and hex
// letters, so a single letter from outside a-f means real text was found.
// Length or word counts cannot be used here: "beef" is four letters and still
// just a device ID.
func looksLikeDeviceName(text string) bool {
	return strings.ContainsFunc(text, func(r rune) bool {
		return unicode.IsLetter(r) && !strings.ContainsRune(hexLetters, r)
	})
}

// getIntelDeviceModel maps an Intel PCI device ID to its model name, or returns
// "" when the ID is not a known graphics device. The tables live in pci_ids.go
// (data, generated from pci.ids); this function only normalises the lookup, so
// that sysfs ("0x0416") and lspci ("0416") forms both resolve.
func getIntelDeviceModel(deviceID string) string {
	deviceID = strings.ToUpper(strings.TrimSpace(deviceID))
	deviceID = strings.TrimPrefix(deviceID, "0X")
	if deviceID == "" {
		return ""
	}
	if model, ok := intelGraphicsModels[deviceID]; ok {
		return model
	}
	return intelCodenameOnlyModels[deviceID]
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

// getDriverVersion returns the i915 driver version (the kernel version the
// driver was built with), read from sysfs with a modinfo fallback. Like the GPU
// name it is fixed for the process lifetime, so the modinfo fork is paid at most
// once.
func getDriverVersion() string {
	return shared.CachedProbe("intel.driverVersion|"+sysDir, resolveIntelDriverVersion)
}

func resolveIntelDriverVersion() string {
	if version, ok := shared.ReadSysFile(filepath.Join(sysDir, "module/i915/version")); ok && version != "" {
		return version
	}
	if version := shared.RunShell("modinfo i915 | awk '/^version:/ {print $2}'"); version != "" {
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
	hwmonPath := shared.FindFirstSubpath(cardPath, "hwmon*")
	if hwmonPath == "" {
		log.Println("No hwmon path found for Intel card")
		return shared.HardwareData{Name: gpuName, Values: data}
	}

	// Temperatures (i915 reports millidegrees Celsius).
	addTemperature(&data, hwmonPath, "temp1_input", shared.GPUTemp.Key)
	addTemperature(&data, hwmonPath, "temp2_input", shared.GPUHotspotTemp.Key)

	// GPU frequency (i915 reports Hz).
	for _, file := range []string{"freq0_input", "freq1_input"} {
		if v, ok := shared.ReadSysFile(filepath.Join(hwmonPath, file)); ok {
			data[shared.GPUFrequency.Key] = shared.NewHardwareValue(shared.ParseFrequencyMHz(v), shared.MHz, shared.Frequency)
			break
		}
	}

	// Average power draw (i915 reports microwatts).
	if v, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "power1_average")); ok {
		data[shared.GPUPowerAvg.Key] = shared.NewHardwareValue(shared.ParsePower(v, 1000000), shared.Watt, shared.Power)
	}

	// Fan speed (rarely present on integrated graphics).
	if v, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "fan1_input")); ok {
		if rpm, err := strconv.Atoi(v); err == nil {
			data[shared.FanSpeed.Key] = shared.NewHardwareValue(float64(rpm), shared.RPM, shared.Frequency)
		}
	}

	return shared.HardwareData{Name: gpuName, Values: data}
}

// GetAllInformations returns the information for every Intel card on the system.
// It returns nil when no card is present, so callers can treat an empty result
// as "vendor unavailable" without probing a second time. Cards that expose no
// sensors (for example an integrated GPU whose hwmon directory is absent) are
// omitted rather than reported as empty entries.
func GetAllInformations() []shared.HardwareData {
	cards, err := probeIntel()
	if err != nil || len(cards) == 0 {
		log.Println("No Intel cards to collect information for")
		return nil
	}

	var result []shared.HardwareData
	for _, cardIndex := range cards {
		cardPath := shared.DRMCardDevicePath(sysDir, cardIndex)
		log.Printf("Collecting information for Intel card %d\n", cardIndex)
		data := collectGPUDataForCard(cardPath, cardIndex)
		if !data.HasTelemetry() {
			log.Printf("Skipping Intel card %d: no readable sensors", cardIndex)
			continue
		}
		result = append(result, data)
	}
	return result
}

// addTemperature records a millidegree-Celsius i915 hwmon sensor as degrees
// Celsius, skipping the metric entirely when the sensor file is absent.
func addTemperature(data *map[string]shared.HardwareValue, hwmonPath string, fileName string, key string) {
	raw, ok := shared.ReadSysFile(filepath.Join(hwmonPath, fileName))
	if !ok {
		return
	}
	if val, err := strconv.Atoi(raw); err == nil {
		(*data)[key] = shared.NewHardwareValue(float64(val/1000), shared.Celsius, shared.Temperature)
	}
}
