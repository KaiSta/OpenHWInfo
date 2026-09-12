// Package mainboard collects information about the mainboard: the identity
// strings the firmware publishes through DMI, plus the temperatures and fan
// speeds of the board's own sensors.
//
// The board's sensors come from two sources. ACPI thermal zones
// (class/thermal/thermal_zone*) describe the zones the firmware declared, of
// which the generic "acpitz" zone and the chipset "pch*" zones belong to the
// board. A Super I/O chip (nct67xx, it87xx, f718xx, w836xx and friends) is a
// monitoring chip soldered onto the board itself and reports the system and
// chipset temperatures plus the fan headers.
package mainboard

import (
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// sysDir is the sysfs mount point. It is a variable so tests can point the
// collector at a fixture tree, exactly as the GPU collectors do.
var sysDir = "/sys"

// dmiDir holds the SMBIOS/DMI strings the firmware exposes.
const dmiDir = "class/dmi/id"

// Bounds on how many board sensors are reported. A Super I/O chip tops out
// around ten temperatures and eight fans; the caps only keep a malformed sysfs
// (or a chip stuck in a loop) from producing an unbounded card.
const (
	maxBoardTemps = 32
	maxBoardFans  = 16
)

// Keys for the rows this package produces, and for the labels that go with
// them. The label keys are data, not metrics: main.go reads them to name the
// generated rows, but they have no template of their own and so never appear in
// the table.
const (
	tempKeyPrefix      = "board.temp"
	tempLabelKeyPrefix = "board.tempLabel"
	fanKeyPrefix       = "board.fan"
	fanLabelKeyPrefix  = "board.fanLabel"
)

// dmiFields maps the DMI attribute files to the keys they are published under.
// board_name is also the card's title.
var dmiFields = []struct {
	file string
	key  string
}{
	{"board_vendor", shared.BoardVendor.Key},
	{"board_name", shared.BoardModel.Key},
	{"board_version", shared.BoardVersion.Key},
	{"board_serial", shared.BoardSerial.Key},
	{"bios_version", shared.BoardBIOSVersion.Key},
	{"bios_date", shared.BoardBIOSDate.Key},
}

// GetInformations reads everything the mainboard can report: its DMI identity
// plus the temperatures and fan speeds of its sensors. Callers treat an empty
// value map as "no mainboard information" (for example on systems without
// SMBIOS or without any board sensor).
func GetInformations() shared.HardwareData {
	values := make(map[string]shared.HardwareValue)

	name := ""
	for _, field := range dmiFields {
		raw, ok := shared.ReadSysFile(filepath.Join(sysDir, dmiDir, field.file))
		if !ok || raw == "" {
			// Unreadable and blank are both "unknown": board_serial is 0400 on
			// most systems, and firmware may leave any of these fields empty.
			continue
		}
		values[field.key] = shared.NewHardwareValue(raw, shared.UnknownUnit, shared.Unknown)
		if field.file == "board_name" {
			name = raw
		}
	}
	if name == "" {
		name = "Mainboard"
	}

	addThermalZones(values)
	addBoardMonitorChips(values)

	return shared.HardwareData{Name: name, Values: values}
}

// zoneTypeRe matches the firmware-named generic ACPI zones ("TZ00", "TZ01").
var zoneTypeRe = regexp.MustCompile(`^TZ\d+$`)

// isBoardZone reports whether an ACPI thermal zone describes the board itself.
// The allowlist is deliberate: a machine also publishes zones for its CPU
// package (x86_pkg_temp), its wireless card (iwlwifi_1), its GPU and its disks,
// and those readings belong to those components. Showing them here would
// attribute a component's heat to the board.
func isBoardZone(zoneType string) bool {
	switch {
	case zoneType == "acpitz": // generic ACPI thermal zone
		return true
	case strings.HasPrefix(zoneType, "pch"): // chipset zones: pch_skylake, pch_cannonlake, ...
		return true
	case zoneTypeRe.MatchString(zoneType):
		return true
	}
	return false
}

// addThermalZones records the temperature of every board-level ACPI thermal
// zone. Sysfs reports thermal zone temperatures in millidegrees Celsius.
func addThermalZones(values map[string]shared.HardwareValue) {
	zones, _ := filepath.Glob(filepath.Join(sysDir, "class/thermal/thermal_zone*"))
	sortByTrailingNumber(zones)

	for _, zone := range zones {
		zoneType, ok := shared.ReadSysFile(filepath.Join(zone, "type"))
		if !ok || !isBoardZone(zoneType) {
			continue
		}
		raw, ok := shared.ReadSysFile(filepath.Join(zone, "temp"))
		if !ok {
			continue
		}
		celsius, ok := milliCelsius(raw)
		if !ok {
			continue
		}
		addTemperature(values, zoneType+" Temperature", celsius)
	}
}

// boardMonitorChipPrefixes are the hwmon driver names of chips that are part of
// the mainboard. It is a prefix list because most of these drivers name their
// hwmon device after the exact chip variant (nct6798, it8688e, f71869a).
// Anything absent here — coretemp/k10temp (CPU), amdgpu/nouveau (GPU), nvme, the
// wireless and battery chips, and the laptop ECs — monitors another component.
var boardMonitorChipPrefixes = []string{
	"nct6", "nct7", // Nuvoton NCT66xx/67xx/78xx
	"it8",    // ITE IT85xx/86xx/87xx
	"f718",   // Fintek F718xx
	"w836",   // Winbond W836xx
	"dme17",  // SMSC DME1737
	"sch5",   // SMSC SCH50xx/51xx/56xx
	"smsc47", // SMSC 47M1xx/47B397
	"pc8736", // National Semiconductor PC8736x
	"vt1211", "vt8231",
	"asb100", "adm102", "adm103", "adt74", "f7537",
	"emc6w", "emc21", "lm85", "lm87", "max66",
	"asus",     // ASUS WMI/EC board sensors (motherboard, VRM, chipset)
	"gigabyte", // Gigabyte WMI board sensors
}

// isBoardMonitorChip reports whether an hwmon chip name belongs to a mainboard
// monitoring chip.
func isBoardMonitorChip(name string) bool {
	for _, prefix := range boardMonitorChipPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// addBoardMonitorChips records the temperatures and fan speeds of every
// mainboard monitoring chip, using each sensor's own label ("SYSTIN", "CPUTIN",
// "Chassis Fan") when the driver provides one.
func addBoardMonitorChips(values map[string]shared.HardwareValue) {
	chips, _ := filepath.Glob(filepath.Join(sysDir, "class/hwmon/hwmon*"))
	sortByTrailingNumber(chips)

	for _, chipPath := range chips {
		chipName, ok := shared.ReadSysFile(filepath.Join(chipPath, "name"))
		if !ok || !isBoardMonitorChip(chipName) {
			continue
		}
		log.Printf("Found mainboard sensor chip %q at %s\n", chipName, chipPath)

		for _, sensor := range numberedSensors(chipPath, "temp", maxBoardTemps) {
			celsius, ok := milliCelsius(sensor.input)
			if !ok {
				continue
			}
			label := fmt.Sprintf("%s Temperature %d", chipName, sensor.index)
			if sensor.label != "" {
				label = sensor.label + " Temperature"
			}
			addTemperature(values, label, celsius)
		}

		for _, sensor := range numberedSensors(chipPath, "fan", maxBoardFans) {
			rpm, err := strconv.Atoi(strings.TrimSpace(sensor.input))
			if err != nil {
				continue
			}
			label := fmt.Sprintf("Fan %d Speed", sensor.index)
			if sensor.label != "" {
				label = sensor.label + " Speed"
			}
			addFan(values, label, float64(rpm))
		}
	}
}

// boardSensor is one numbered hwmon attribute group (tempN_* or fanN_*).
type boardSensor struct {
	index int
	input string
	label string
}

// numberedSensors returns the readable sensors of one kind ("temp" or "fan") in
// a hwmon directory, ordered by sensor number and capped at limit. The optional
// <kind>N_label attribute supplies the sensor's name.
func numberedSensors(dir, kind string, limit int) []boardSensor {
	matches, _ := filepath.Glob(filepath.Join(dir, kind+"*_input"))

	var sensors []boardSensor
	for _, match := range matches {
		index, ok := sensorIndex(filepath.Base(match), kind)
		if !ok || index == 0 || index > limit {
			continue
		}
		input, ok := shared.ReadSysFile(match)
		if !ok || input == "" {
			continue
		}
		label, _ := shared.ReadSysFile(filepath.Join(dir, fmt.Sprintf("%s%d_label", kind, index)))
		sensors = append(sensors, boardSensor{index: index, input: input, label: label})
	}

	// Glob sorts lexicographically, which would place temp10 before temp2.
	sort.Slice(sensors, func(i, j int) bool { return sensors[i].index < sensors[j].index })
	return sensors
}

// sensorIndex extracts N from "<kind>N_input", e.g. "temp10_input" -> 10.
func sensorIndex(fileName, kind string) (int, bool) {
	rest, ok := strings.CutPrefix(fileName, kind)
	if !ok {
		return 0, false
	}
	digits, ok := strings.CutSuffix(rest, "_input")
	if !ok || digits == "" {
		return 0, false
	}
	index, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return index, true
}

// addTemperature appends a board temperature under the next free board.tempN
// key, remembering its label under the matching label key so main.go can name
// the generated row.
func addTemperature(values map[string]shared.HardwareValue, label string, celsius float64) {
	for index := 0; index < maxBoardTemps; index++ {
		key := tempKeyPrefix + strconv.Itoa(index)
		if _, taken := values[key]; taken {
			continue
		}
		values[key] = shared.NewHardwareValue(celsius, shared.Celsius, shared.Temperature)
		values[tempLabelKeyPrefix+strconv.Itoa(index)] = shared.NewHardwareValue(label, shared.UnknownUnit, shared.Unknown)
		return
	}
	log.Println("Mainboard reports more temperature sensors than can be displayed")
}

// addFan appends a board fan speed under the next free board.fanN key. Fans are
// recorded in RPM with the Frequency value type, which is what the GPU fan
// metric already does and what makes the reading eligible for min/max tracking.
func addFan(values map[string]shared.HardwareValue, label string, rpm float64) {
	for index := 0; index < maxBoardFans; index++ {
		key := fanKeyPrefix + strconv.Itoa(index)
		if _, taken := values[key]; taken {
			continue
		}
		values[key] = shared.NewHardwareValue(rpm, shared.RPM, shared.Frequency)
		values[fanLabelKeyPrefix+strconv.Itoa(index)] = shared.NewHardwareValue(label, shared.UnknownUnit, shared.Unknown)
		return
	}
	log.Println("Mainboard reports more fans than can be displayed")
}

// milliCelsius converts a sysfs millidegree-Celsius reading (a thermal zone's
// temp, or a hwmon tempN_input) to degrees Celsius.
func milliCelsius(raw string) (float64, bool) {
	milli, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return float64(milli) / 1000, true
}

// sortByTrailingNumber orders sysfs paths by the number at the end of their
// name, so thermal_zone2 follows thermal_zone10 correctly.
func sortByTrailingNumber(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		return trailingNumber(paths[i]) < trailingNumber(paths[j])
	})
}

var trailingNumberRe = regexp.MustCompile(`(\d+)$`)

// trailingNumber returns the number at the end of a path's base name, or 0.
func trailingNumber(path string) int {
	match := trailingNumberRe.FindString(filepath.Base(path))
	if match == "" {
		return 0
	}
	number, err := strconv.Atoi(match)
	if err != nil {
		return 0
	}
	return number
}
