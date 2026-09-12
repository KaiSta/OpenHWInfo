package amd

import (
	"log"
	"math"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

var sysDir = "/sys"

// amdVendorID is the PCI vendor ID for AMD devices (0x1002).
const amdVendorID = "0x1002"

// probeAMD scans the DRM cards and returns the indices of all AMD cards,
// identified by their PCI vendor ID.
//
// There is deliberately no lspci fallback that guesses an index. If a card's
// vendor file is unreadable then its hwmon directory is unreachable too, so a
// guessed index can only ever read some other card's sensors while labelling
// them AMD. An unreachable card yields no card, which is the honest answer.
func probeAMD() ([]int, error) {
	indices := shared.ProbeDRMCards(sysDir, amdVendorID)
	for _, i := range indices {
		log.Printf("Found AMD card at index %d\n", i)
	}
	if len(indices) == 0 {
		log.Println("No AMD cards found")
	}
	return indices, nil
}

// getGPUName returns a human-readable chipset/product label for the card at the
// given DRM index, preferring a PCI-address-scoped lspci query so that multi-GPU
// systems get the correct name per card. The raw lspci line is reduced to just
// the device portion (see cleanGPUName).
func getGPUName(cardIndex int) string {
	if pciAddr := shared.ResolvePCIDevice(sysDir, cardIndex); pciAddr != "" {
		if line := strings.TrimSpace(shared.RunShell("lspci -s " + pciAddr)); line != "" {
			return cleanGPUName(line)
		}
	}
	// Fallback: list all VGA/3D cards and pick by index.
	cmd := exec.Command("sh", "-c", "lspci | grep -i 'vga\\|3d'")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing lspci command: %v\n", err)
		return "AMD GPU"
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if cardIndex < len(lines) {
		return cleanGPUName(lines[cardIndex])
	}
	return "AMD GPU"
}

// revSuffixRe matches the trailing revision tag that lspci appends to device
// names, e.g. " (rev cc)".
var revSuffixRe = regexp.MustCompile(`\s*\(rev\s+[0-9a-fA-F]+\)\s*$`)

// vendorPrefixes are known AMD vendor prefixes stripped from lspci output so
// that only the chipset/product name remains.
var vendorPrefixes = []string{
	"Advanced Micro Devices, Inc. [AMD/ATI] ",
}

// cleanGPUName reduces a raw lspci line to just the chipset/product portion,
// dropping the PCI address, class label, vendor prefix and revision tag. For
// example:
//
//	0c:00.0 VGA compatible controller: Advanced Micro Devices, Inc. [AMD/ATI] Navi 31 [...] (rev cc)
//	  -> Navi 31 [Radeon RX 7900 XT/7900 XTX/7900M]
func cleanGPUName(raw string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return "AMD GPU"
	}

	// Drop the leading PCI address (first whitespace-delimited token).
	if idx := strings.IndexByte(line, ' '); idx >= 0 {
		line = line[idx+1:]
	}

	// Drop the class label up to the first ": " separator.
	if idx := strings.Index(line, ": "); idx >= 0 {
		line = line[idx+len(": "):]
	}

	// Strip a known vendor prefix if present.
	for _, prefix := range vendorPrefixes {
		line = strings.TrimPrefix(line, prefix)
	}

	// Remove the trailing "(rev xx)" tag.
	line = revSuffixRe.ReplaceAllString(line, "")

	return strings.TrimSpace(line)
}

// gpuVariant is one retail product that shares a chipset, identified by its
// total VRAM size so siblings can be told apart at runtime.
type gpuVariant struct {
	name   string
	vramMB float64 // expected total VRAM in MB; 0 means "any"
}

// chipsetVariants maps a cleaned chipset name to the retail products that use
// it, ordered by preference. It is used to turn a generic chipset label such as
// "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900M]" into a specific product name.
var chipsetVariants = map[string][]gpuVariant{
	"Navi 31 [Radeon RX 7900 XT/7900 XTX/7900M]": {
		{name: "Radeon RX 7900 XTX", vramMB: 24576}, // 24 GB
		{name: "Radeon RX 7900 XT", vramMB: 20480},  // 20 GB
	},
}

// disambiguateGPUName picks a specific retail product name for a chipset that
// maps to several products, using the measured total VRAM as the tie-breaker.
// If no confident match is found it returns the cleaned chipset name unchanged.
func disambiguateGPUName(chipsetName string, vramMB float64) string {
	variants := chipsetVariants[chipsetName]
	if len(variants) == 0 || vramMB <= 0 {
		return chipsetName
	}

	best := ""
	bestDiff := -1.0
	for _, v := range variants {
		diff := math.Abs(v.vramMB - vramMB)
		if bestDiff < 0 || diff < bestDiff {
			best, bestDiff = v.name, diff
		}
	}

	// Only trust the match if it is within a reasonable tolerance of the
	// expected VRAM size; otherwise keep the generic chipset name.
	const toleranceMB = 2048 // ~1 GB slack
	if bestDiff <= float64(toleranceMB) {
		return best
	}
	return chipsetName
}

// collectGPUDataForCard collects the hardware metrics available for a single
// AMD GPU at the given card path.
func collectGPUDataForCard(cardPath string, cardIndex int) shared.HardwareData {
	data := make(map[string]shared.HardwareValue)
	chipsetName := getGPUName(cardIndex)
	data[shared.GPUName.Key] = shared.NewHardwareValue(chipsetName, shared.UnknownUnit, shared.Unknown)

	cardPath = filepath.Join(cardPath, "hwmon")
	hwmonPath := shared.FindFirstSubpath(cardPath, "hwmon*")
	if hwmonPath == "" {
		log.Println("No hwmon path found")
		return shared.HardwareData{Name: chipsetName, Values: data}
	}

	devicePath := filepath.Join(hwmonPath, "device")

	parseTemperatureValue(&data, hwmonPath, "temp1_input", shared.GPUTemp.Key)
	parseTemperatureValue(&data, hwmonPath, "temp2_input", shared.GPUHotspotTemp.Key)
	parseTemperatureValue(&data, hwmonPath, "temp3_input", shared.VRAMTemp.Key)

	// GPU frequency (freq1_input) and convert to MHz
	if gpuFreq, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "freq1_input")); ok {
		data[shared.GPUFrequency.Key] = shared.NewHardwareValue(shared.ParseFrequencyMHz(gpuFreq), shared.MHz, shared.Frequency)
	}

	// Memory frequency (freq2_input) and convert to MHz
	if memFreq, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "freq2_input")); ok {
		data[shared.VRAMLrequency.Key] = shared.NewHardwareValue(shared.ParseFrequencyMHz(memFreq), shared.MHz, shared.Frequency)
	}

	vramTotalMB := parseMemoryValue(filepath.Join(devicePath, "mem_info_vram_total"))
	if vramTotalMB > 0 {
		data[shared.VRAMTotal.Key] = shared.NewHardwareValue(vramTotalMB, shared.Megabyte, shared.Memory)
	}
	if usedMB := parseMemoryValue(filepath.Join(devicePath, "mem_info_vram_used")); usedMB > 0 {
		data[shared.VRAMUsed.Key] = shared.NewHardwareValue(usedMB, shared.Megabyte, shared.Memory)
	}
	if vendor, ok := shared.ReadSysFile(filepath.Join(devicePath, "mem_info_vram_vendor")); ok && vendor != "" {
		data[shared.VRAMVendor.Key] = shared.NewHardwareValue(vendor, shared.UnknownUnit, shared.Unknown)
	}

	// Refine the generic chipset label into a specific product name using the
	// measured VRAM size (e.g. Navi 31 -> Radeon RX 7900 XT vs XTX).
	gpuName := disambiguateGPUName(chipsetName, vramTotalMB)
	data[shared.GPUName.Key] = shared.NewHardwareValue(gpuName, shared.UnknownUnit, shared.Unknown)

	if busy, ok := shared.ReadSysFile(filepath.Join(devicePath, "gpu_busy_percent")); ok {
		data[shared.GPUUsage.Key] = shared.NewHardwareValue(parseGPUUsage(busy), shared.Percent, shared.Usage)
	}
	if cap, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "power1_cap_max")); ok {
		data[shared.GPUPowerMax.Key] = shared.NewHardwareValue(shared.ParsePower(cap, 1000000), shared.Watt, shared.Power)
	}
	if avg, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "power1_average")); ok {
		data[shared.GPUPowerAvg.Key] = shared.NewHardwareValue(shared.ParsePower(avg, 1000000), shared.Watt, shared.Power)
	}
	if fanSpeed, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "fan1_input")); ok && fanSpeed != "0" {
		data[shared.FanSpeed.Key] = shared.NewHardwareValue(fanSpeed, shared.RPM, shared.Frequency)
	}

	return shared.HardwareData{Name: gpuName, Values: data}
}

// GetAllInformations returns the information for every AMD card on the system.
// It returns nil when no card is present, so callers can treat an empty result
// as "vendor unavailable" without probing a second time. Cards that expose no
// sensors at all are omitted rather than reported as empty entries.
func GetAllInformations() []shared.HardwareData {
	cards, err := probeAMD()
	if err != nil || len(cards) == 0 {
		log.Println("No AMD cards to collect information for")
		return nil
	}

	var result []shared.HardwareData
	for _, cardIndex := range cards {
		cardPath := shared.DRMCardDevicePath(sysDir, cardIndex)
		log.Printf("Collecting information for AMD card %d\n", cardIndex)
		data := collectGPUDataForCard(cardPath, cardIndex)
		if !data.HasTelemetry() {
			log.Printf("Skipping AMD card %d: no readable sensors", cardIndex)
			continue
		}
		result = append(result, data)
	}
	return result
}

// parseTemperatureValue records a millidegree-Celsius hwmon sensor as degrees
// Celsius. The sensor is skipped entirely when its file is absent, so that
// boards without a given sensor do not report a bogus 0 C reading.
func parseTemperatureValue(data *map[string]shared.HardwareValue, hwmonPath string, fileName string, dataKey ...string) {
	raw, ok := shared.ReadSysFile(filepath.Join(hwmonPath, fileName))
	if !ok {
		return
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return
	}
	key := shared.GPUTemp.Key
	if len(dataKey) > 0 {
		key = dataKey[0]
	}
	(*data)[key] = shared.NewHardwareValue(float64(val/1000), shared.Celsius, shared.Temperature)
}

// parseMemoryValue converts a sysfs byte count to megabytes, returning 0 when
// the file is absent or unparsable.
func parseMemoryValue(path string) float64 {
	raw, ok := shared.ReadSysFile(path)
	if !ok {
		return 0
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return float64(val / 1024 / 1024) // Convert bytes to MB
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
