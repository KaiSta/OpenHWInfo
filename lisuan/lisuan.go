// Package lisuan collects information about Lisuan Technology's graphics cards.
// The 7G100 series carries PCI vendor ID 0x4C54 (devices 0x5000–0x5006), which is
// the only thing that identifies one of these cards on a Linux system: the driver
// is out of tree, the PCI database does not know the vendor yet, and no other
// component in the userland stack names the card either.
//
// Everything reported here comes from the kernel's own interfaces — the DRM card
// node for identity, the card's hwmon directory for temperatures, clocks, fans and
// power, and the module behind the card for its version. The driver's own sensor
// labels decide what a temperature means, because nothing standard fixes the
// sensor order, and a file the driver does not publish yields no row rather than a
// zero. A card whose driver publishes no reading at all is omitted instead of
// drawn empty (shared.HardwareData.HasTelemetry), which -debug explains.
//
// Bounds: maxTemps caps how many temperature sensors are read, and the naming
// table is bounded by lisuanModels. VRAM size, VRAM in use and GPU utilisation are
// deliberately not read: those attribute names belong to amdgpu's interface, and
// assuming another vendor's driver implements them would be a guess wearing the
// clothes of a reading.
package lisuan

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// sysDir is the sysfs mount point. It is a variable so tests can point the
// collector at a fixture tree, as the other collectors do.
var sysDir = "/sys"

// lisuanVendorID is the PCI vendor ID of Lisuan Technology's graphics cards.
const lisuanVendorID = "0x4c54"

// maxTemps bounds the temperature sensors read from one card's hwmon directory.
// A card exposing more than this is logged rather than read on: the bound keeps a
// malformed sysfs from turning into an unbounded poll, and no known card comes
// close to it.
const maxTemps = 8

// probeLisuan returns the DRM card indices whose PCI vendor is Lisuan's. The
// driver has to register a DRM card to drive a display, so this is the vendor's
// presence on the machine.
func probeLisuan() ([]int, error) {
	indices := shared.ProbeDRMCards(sysDir, lisuanVendorID)
	for _, index := range indices {
		shared.Debugf("Found Lisuan card at index %d", index)
	}
	if len(indices) == 0 {
		shared.Debugf("No Lisuan cards found")
	}
	return indices, nil
}

// getPCIDeviceID reads the PCI device ID (e.g. "5000") of the card at the given
// DRM index from sysfs, uppercased to match lisuanModels.
func getPCIDeviceID(cardIndex int) string {
	id, ok := shared.ReadSysFile(filepath.Join(shared.DRMCardDevicePath(sysDir, cardIndex), "device"))
	if !ok {
		return ""
	}
	return strings.ToUpper(strings.TrimPrefix(id, "0x"))
}

// getGPUName returns the card's name. A card's name cannot change while the
// process runs, so it is resolved once rather than on every poll; the cache key
// carries the sysfs root and the card identity so two cards — and two tests'
// fixture trees — never share one name.
func getGPUName(cardIndex int) string {
	deviceID := getPCIDeviceID(cardIndex)
	key := "lisuan.gpuName|" + sysDir + "|" + strconv.Itoa(cardIndex) + "|" + deviceID
	return shared.CachedProbe(key, func() string { return resolveLisuanGPUName(deviceID) })
}

// resolveLisuanGPUName names a card from its PCI device ID, falling back to a
// name that claims nothing.
//
// There is no lspci fallback here, unlike the Intel and AMD collectors. On every
// PCI database that exists today a query for this vendor returns the identifiers
// back — the vendor is not in the shipped database — and a query whose only
// possible answers are "4c54:5000" and "Device 4c54:5000" is a fork that can only
// produce a worse name than the fallback. When the database catches up, the
// reliable path is still this table: add the device ID there.
func resolveLisuanGPUName(deviceID string) string {
	if name, ok := lisuanModels[deviceID]; ok && name != "" {
		return name
	}
	// The card is known to be Lisuan (probeLisuan matched the vendor) but its
	// device ID is not in the table. Naming the product would be a guess, so the
	// card is named after the vendor that made it and no more.
	return "Lisuan GPU"
}

// getDriverVersion returns the version of the module driving the card, resolved
// once per card for the same reason the name is.
func getDriverVersion(cardIndex int) string {
	key := "lisuan.driverVersion|" + sysDir + "|" + strconv.Itoa(cardIndex)
	return shared.CachedProbe(key, func() string { return resolveDriverVersion(cardIndex) })
}

// resolveDriverVersion reads the <version> attribute of the module bound to the
// card. The Lisuan driver's module name is not documented anywhere this project
// could verify, so it is discovered rather than hardcoded: the card's driver
// symlink names the bound driver, and that driver's module symlink names the
// module. An unbound card, or a module publishing no version, produces no row.
func resolveDriverVersion(cardIndex int) string {
	module := driverModuleName(cardIndex)
	if module == "" {
		return ""
	}
	version, ok := shared.ReadSysFile(filepath.Join(sysDir, "module", module, "version"))
	if !ok {
		shared.Debugf("Lisuan card %d is driven by module %q, which publishes no version", cardIndex, module)
		return ""
	}
	return version
}

// driverModuleName follows the card's driver symlink to the module implementing
// it, e.g. <device> -driver-> /sys/bus/pci/drivers/<driver> -module-> /sys/module/<module>.
func driverModuleName(cardIndex int) string {
	devicePath := shared.DRMCardDevicePath(sysDir, cardIndex)
	for _, link := range []string{
		filepath.Join(devicePath, "driver", "module"),
		filepath.Join(devicePath, "driver"),
	} {
		target, err := os.Readlink(link)
		if err != nil {
			continue
		}
		if name := path.Base(target); name != "" && name != "." && name != "/" {
			return name
		}
	}
	return ""
}

// temperatureKind is what a driver's own sensor label says a sensor measures.
type temperatureKind int

const (
	// temperatureCard is a sensor that is neither of the two below: the card's own
	// reading, which is what an unlabelled or edge-labelled sensor is.
	temperatureCard temperatureKind = iota
	temperatureHotspot
	temperatureMemory
)

// classifyTemperatureLabel reads the driver's word for a sensor. The lists are
// deliberately short: a label this package does not recognise is treated as the
// card's own temperature, which is the reading every card has, rather than being
// forced into a row that would claim the number means something else.
func classifyTemperatureLabel(label string) temperatureKind {
	lowered := strings.ToLower(label)
	for _, word := range []string{"hotspot", "hot spot", "junction"} {
		if strings.Contains(lowered, word) {
			return temperatureHotspot
		}
	}
	for _, word := range []string{"mem", "vram", "hbm"} {
		if strings.Contains(lowered, word) {
			return temperatureMemory
		}
	}
	return temperatureCard
}

// collectGPUDataForCard collects the readings the card's driver publishes at the
// given DRM device path.
func collectGPUDataForCard(cardPath string, cardIndex int) shared.HardwareData {
	data := make(map[string]shared.HardwareValue)
	gpuName := getGPUName(cardIndex)
	data[shared.GPUName.Key] = shared.NewHardwareValue(gpuName, shared.UnknownUnit, shared.Unknown)
	if version := getDriverVersion(cardIndex); version != "" {
		data[shared.GPUDriverVersion.Key] = shared.NewHardwareValue(version, shared.UnknownUnit, shared.Unknown)
	}

	hwmonPath := shared.FindFirstSubpath(filepath.Join(cardPath, "hwmon"), "hwmon*")
	if hwmonPath == "" {
		shared.Debugf("No hwmon path found for Lisuan card %d", cardIndex)
		return shared.HardwareData{Name: gpuName, Values: data}
	}

	addTemperatures(&data, hwmonPath, cardIndex)

	// The card's clock: channel 1 is the graphics clock on the drivers that
	// publish two channels, so it is preferred, and a single-channel driver
	// reports through channel 0.
	for _, file := range []string{"freq1_input", "freq0_input"} {
		raw, ok := shared.ReadSysFile(filepath.Join(hwmonPath, file))
		if !ok {
			continue
		}
		data[shared.GPUFrequency.Key] = shared.NewHardwareValue(shared.ParseFrequencyMHz(raw), shared.MHz, shared.Frequency)
		break
	}

	if raw, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "power1_average")); ok {
		data[shared.GPUPowerAvg.Key] = shared.NewHardwareValue(shared.ParsePower(raw, 1000000), shared.Watt, shared.Power)
	}
	if raw, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "power1_cap_max")); ok {
		data[shared.GPUPowerMax.Key] = shared.NewHardwareValue(shared.ParsePower(raw, 1000000), shared.Watt, shared.Power)
	}

	if raw, ok := shared.ReadSysFile(filepath.Join(hwmonPath, "fan1_input")); ok {
		if rpm, err := strconv.Atoi(raw); err == nil {
			// A published zero is reported as a reading. The AMD collector
			// withholds a zero fan, because some amdgpu cards report one for a
			// fan they do not have; here the driver published a fan sensor, and a
			// fan that has stopped is exactly the figure a monitor exists to show.
			data[shared.FanSpeed.Key] = shared.NewHardwareValue(float64(rpm), shared.RPM, shared.Frequency)
		}
	}

	return shared.HardwareData{Name: gpuName, Values: data}
}

// addTemperatures records the card's temperature sensors, each under the row its
// own label says it belongs to: the first card sensor becomes the GPU temperature,
// a hotspot or junction sensor the hotspot temperature, and a memory sensor the
// video-memory temperature. A sensor whose file exists but cannot be parsed
// produces no row, and sensors past maxTemps are logged rather than read.
func addTemperatures(data *map[string]shared.HardwareValue, hwmonPath string, cardIndex int) {
	for index := 1; index <= maxTemps; index++ {
		raw, ok := shared.ReadSysFile(filepath.Join(hwmonPath, fmt.Sprintf("temp%d_input", index)))
		if !ok {
			continue
		}
		celsius, ok := milliCelsius(raw)
		if !ok {
			continue
		}
		label, _ := shared.ReadSysFile(filepath.Join(hwmonPath, fmt.Sprintf("temp%d_label", index)))

		key := shared.GPUTemp.Key
		switch classifyTemperatureLabel(label) {
		case temperatureHotspot:
			key = shared.GPUHotspotTemp.Key
		case temperatureMemory:
			key = shared.VRAMTemp.Key
		}
		// The first sensor of each kind wins: a driver reporting several card
		// sensors has one reading for the card, not a row per sensor.
		if _, exists := (*data)[key]; exists {
			continue
		}
		(*data)[key] = shared.NewHardwareValue(celsius, shared.Celsius, shared.Temperature)
	}

	if sensors := countTemperatureSensors(hwmonPath); sensors > maxTemps {
		shared.Debugf("Lisuan card %d publishes %d temperature sensors; the first %d are read",
			cardIndex, sensors, maxTemps)
	}
}

// countTemperatureSensors reports how many temperature sensors the hwmon
// directory holds, so the cap can say what it left out rather than dropping it
// silently.
func countTemperatureSensors(hwmonPath string) int {
	matches, err := filepath.Glob(filepath.Join(hwmonPath, "temp*_input"))
	if err != nil {
		return 0
	}
	return len(matches)
}

// milliCelsius converts a hwmon millidegree-Celsius reading to degrees, keeping
// the fraction: the division is floating point, so a sensor at 45.5 °C does not
// read as 45, and a value that cannot be parsed reports false rather than 0.
func milliCelsius(raw string) (float64, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return float64(value) / 1000, true
}

// GetAllInformations returns the readings for every Lisuan card on the machine.
// It returns nil when no card is present, so a caller can treat an empty result as
// "vendor unavailable" without probing again. A card whose driver publishes no
// reading beyond its identity is omitted rather than reported as an empty card.
func GetAllInformations() []shared.HardwareData {
	cards, err := probeLisuan()
	if err != nil || len(cards) == 0 {
		shared.Debugf("No Lisuan cards to collect information for")
		return nil
	}

	var result []shared.HardwareData
	for _, cardIndex := range cards {
		cardPath := shared.DRMCardDevicePath(sysDir, cardIndex)
		shared.Debugf("Collecting information for Lisuan card %d", cardIndex)
		data := collectGPUDataForCard(cardPath, cardIndex)
		if !data.HasTelemetry() {
			shared.Debugf("Skipping Lisuan card %d: no readable sensors", cardIndex)
			continue
		}
		result = append(result, data)
	}
	return result
}
