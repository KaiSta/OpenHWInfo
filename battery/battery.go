// Package battery collects information about a laptop's batteries: what they
// hold, how much of it is left, how worn they are, and whether the machine is
// running from the mains.
//
// Every reading comes from /sys/class/power_supply, the kernel's power supply
// class, so no external tool is needed and nothing is forked. One card is
// produced per battery rather than one for the machine, because a laptop can
// have more than one — a fixed internal pack plus a removable one, or a second
// battery in an ultrabay — and each has its own capacity, wear and charge.
package battery

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// sysDir is the sysfs mount point. It is a variable so tests can point the
// collector at a fixture tree, exactly as the other collectors do.
var sysDir = "/sys"

// supplyDir is where the kernel lists the machine's power supplies: its
// batteries, its mains adapters, and any peripheral that reports a battery.
const supplyDir = "class/power_supply"

// Health thresholds, as a percentage of the capacity a battery was designed to
// hold. A lithium pack below about 80% no longer holds a useful charge for its
// weight, and one below half is at the end of its service life.
const (
	healthWarnPercent = 80
	healthBadPercent  = 50
)

// supply is one power supply device.
type supply struct {
	name  string // "BAT1", "ACAD"
	path  string
	kind  string // "Battery", "Mains", "USB", "UPS", …
	scope string // "System" or "Device"; empty when the kernel does not say
}

// batteryInfo is what one battery reports about itself, normalized so that the
// verdict does not care which quantity its firmware publishes: some batteries
// report energy (Wh), others charge (Ah).
type batteryInfo struct {
	name string
	// unit is the quantity this battery reports its capacity in, so a sentence
	// about it quotes the same unit as its rows.
	unit shared.HardwareUnit

	now, full, design          float64
	hasNow, hasFull, hasDesign bool
	holdsEnergy                bool
	health                     float64
	hasHealth                  bool
	status                     string
	hasStatus, hasCapacity     bool
	cycles                     string
	hasCycles                  bool
	power                      float64
	hasPower                   bool
}

// reported reports whether the battery described itself at all. A supply that
// exposes nothing but its type and the AC adapter state would be an empty card,
// so it is left out instead.
func (info batteryInfo) reported() bool {
	return info.hasStatus || info.hasCapacity || info.hasNow || info.hasFull || info.hasDesign || info.hasCycles
}

// GetInformations returns one HardwareData per battery, ordered by supply name so
// each battery keeps its card position between polls. It returns nil on a machine
// with no battery to report, which is every desktop.
func GetInformations() []shared.HardwareData {
	supplies := powerSupplies()
	ac, acKnown := externalPowerState(supplies)

	var result []shared.HardwareData
	for _, s := range supplies {
		if !isSystemBattery(s) {
			continue
		}
		// An empty battery bay describes no battery: `present` is 0 and every
		// reading below it is absent, so a card would be a heading with nothing
		// under it. A kernel that reports no `present` attribute is taken at its
		// word, since its absence is not a claim that the bay is empty.
		if present, ok := readBool(s.path, "present"); ok && !present {
			shared.Debugf("Skipping empty battery bay %s", s.name)
			continue
		}

		values := make(map[string]shared.HardwareValue)
		info := readBattery(s, values)
		if !info.reported() {
			shared.Debugf("Skipping battery %s: nothing readable", s.name)
			continue
		}
		if acKnown {
			values[shared.BatteryAC.Key] = shared.NewHardwareValue(ac, shared.UnknownUnit, shared.Unknown)
		}

		shared.Debugf("Found battery %s (%s)", s.name, info.status)
		result = append(result, shared.HardwareData{Name: s.name, Values: values, Status: evaluateHealth(info)})
	}

	return result
}

// powerSupplies lists the machine's power supply devices. A device whose type
// cannot be read is left out: without it there is no way to tell a battery from
// an adapter, and guessing that it is a battery would put the mains on a battery
// card.
func powerSupplies() []supply {
	dirs, _ := filepath.Glob(filepath.Join(sysDir, supplyDir, "*"))
	sort.Strings(dirs)

	var supplies []supply
	for _, dir := range dirs {
		kind, ok := readText(dir, "type")
		if !ok {
			continue
		}
		scope, _ := readText(dir, "scope")
		supplies = append(supplies, supply{name: filepath.Base(dir), path: dir, kind: kind, scope: scope})
	}
	return supplies
}

// isSystemBattery reports whether a supply is a battery that powers the machine.
// Its type is not enough on its own: a wireless mouse, a keyboard or a headset
// reports a battery too, and the kernel marks those with scope "Device". A kernel
// that reports no scope at all is taken at its word, because discarding every
// battery on it would be worse than accepting a peripheral's.
func isSystemBattery(s supply) bool {
	if s.kind != "Battery" {
		return false
	}
	return s.scope == "" || s.scope == "System"
}

// externalPowerState reports whether an external supply is feeding the machine,
// and whether that could be determined at all. The result is the text a battery
// card shows, so "Offline" means a readable adapter reported itself as unplugged
// rather than a machine that has no adapter information.
func externalPowerState(supplies []supply) (string, bool) {
	known, online := false, false
	for _, s := range supplies {
		if !deliversMains(s.kind) {
			continue
		}
		plugged, ok := readBool(s.path, "online")
		if !ok {
			continue
		}
		known = true
		if plugged {
			online = true
		}
	}

	switch {
	case !known:
		return "", false
	case online:
		return "Online", true
	default:
		return "Offline", true
	}
}

// deliversMains reports whether a supply type feeds the machine's power input:
// an ordinary mains adapter or one of the USB charger types (a USB-C supply is
// how most current laptops charge). A UPS is deliberately not counted — it
// reports itself online while it runs the machine from its own battery, so
// calling it the mains would report an adapter that is not plugged in.
func deliversMains(kind string) bool {
	return kind == "Mains" || strings.HasPrefix(kind, "USB")
}

// readBattery records every reading one battery reports, and returns what the
// verdict needs to know about it.
func readBattery(s supply, values map[string]shared.HardwareValue) batteryInfo {
	info := batteryInfo{name: s.name}

	if status, ok := readText(s.path, "status"); ok {
		info.status, info.hasStatus = status, true
		values[shared.BatteryStatus.Key] = shared.NewHardwareValue(status, shared.UnknownUnit, shared.Unknown)
	}

	// The kernel computes the percentage from whichever capacity attributes the
	// firmware provides, so it is reported as-is rather than re-derived here.
	if capacity, ok := readNumber(s.path, "capacity"); ok {
		info.hasCapacity = true
		values[shared.BatteryCharge.Key] = shared.NewHardwareValue(capacity, shared.Percent, shared.Usage)
	}

	// Whichever capacity family the firmware publishes. A battery reports one or
	// the other, so the first family that yields a reading is what the card is
	// built from; the other family's rows stay absent rather than being converted
	// (which would need the pack voltage, and would be a different number from
	// the one the firmware reports).
	info.unit = shared.WattHour
	info.now, info.hasNow = addReading(values, s.path, "energy_now", shared.BatteryEnergyNow.Key, shared.WattHour, shared.Memory)
	info.full, info.hasFull = addReading(values, s.path, "energy_full", shared.BatteryEnergyFull.Key, shared.WattHour, shared.Memory)
	// A design figure is what the battery was built to hold, so it is a spec
	// rather than a measurement and is left out of the min/max history.
	info.design, info.hasDesign = addReading(values, s.path, "energy_full_design", shared.BatteryEnergyDesign.Key, shared.WattHour, shared.Unknown)
	info.holdsEnergy = info.hasNow || info.hasFull || info.hasDesign

	if !info.holdsEnergy {
		info.unit = shared.AmpereHour
		info.now, info.hasNow = addReading(values, s.path, "charge_now", shared.BatteryChargeNow.Key, shared.AmpereHour, shared.Memory)
		info.full, info.hasFull = addReading(values, s.path, "charge_full", shared.BatteryChargeFull.Key, shared.AmpereHour, shared.Memory)
		info.design, info.hasDesign = addReading(values, s.path, "charge_full_design", shared.BatteryChargeDesign.Key, shared.AmpereHour, shared.Unknown)
	}

	// Health is how much a battery holds when full against what it was designed
	// to hold, which is the figure that says whether it is worn out. Both numbers
	// are needed: with only one of them there is nothing to compare, and this is
	// what the card's badge rests on.
	if info.hasFull && info.hasDesign && info.design > 0 {
		info.health, info.hasHealth = info.full/info.design*100, true
		values[shared.BatteryHealth.Key] = shared.NewHardwareValue(info.health, shared.Percent, shared.Usage)
	}

	if power, ok := readNumber(s.path, "power_now"); ok {
		// ParsePower is fed only a value that parsed as a number, so a battery
		// reporting power where the file holds no reading does not become one.
		info.power, info.hasPower = shared.ParsePower(strconv.FormatFloat(power, 'f', -1, 64), 1000000), true
		values[shared.BatteryPower.Key] = shared.NewHardwareValue(info.power, shared.Watt, shared.Power)
	}

	// Energy over power is a duration, and both have to be readable for it to
	// mean anything. The charge family is measured in Ah, which cannot be divided
	// by watts without the pack voltage, so such a battery gets no estimate
	// rather than a wrong one. A full battery on the mains moves no power in
	// either direction, so it has no time to report.
	if info.holdsEnergy && info.hasNow && info.hasPower && info.power > 0 {
		hours := info.now / info.power
		switch info.status {
		case "Discharging":
			values[shared.BatteryTimeEmpty.Key] = shared.NewHardwareValue(hours, shared.Hour, shared.Unknown)
		case "Charging":
			values[shared.BatteryTimeFull.Key] = shared.NewHardwareValue(hours, shared.Hour, shared.Unknown)
		}
	}

	addReading(values, s.path, "voltage_now", shared.BatteryVoltage.Key, shared.Volt, shared.Power)
	// The design voltage is a spec like the design capacity, not a measurement.
	addReading(values, s.path, "voltage_min_design", shared.BatteryDesignVoltage.Key, shared.Volt, shared.Unknown)

	// Cycle count is a count rather than a measurement, so it is carried as the
	// text the firmware wrote: it stays "0" instead of becoming "0.00", and a
	// min/max range for it would say nothing.
	if cycles, ok := readText(s.path, "cycle_count"); ok {
		if _, err := strconv.ParseFloat(cycles, 64); err == nil {
			info.cycles, info.hasCycles = cycles, true
			values[shared.BatteryCycles.Key] = shared.NewHardwareValue(cycles, shared.UnknownUnit, shared.Unknown)
		}
	}

	for _, field := range []struct{ file, key string }{
		{"technology", shared.BatteryTechnology.Key},
		{"model_name", shared.BatteryModel.Key},
		{"manufacturer", shared.BatteryManufacturer.Key},
		{"serial_number", shared.BatterySerial.Key},
	} {
		if value, ok := readText(s.path, field.file); ok {
			values[field.key] = shared.NewHardwareValue(value, shared.UnknownUnit, shared.Unknown)
		}
	}

	return info
}

// evaluateHealth turns what a battery reports into the verdict shown on its
// card. It rests on the design capacity comparison and nothing else: a battery
// whose firmware publishes no design capacity gets no badge at all, because an
// invented verdict would be worse than none, and a battery whose capacity has
// fallen simply has not been judged by anything else the kernel exposes.
func evaluateHealth(info batteryInfo) *shared.DeviceStatus {
	if !info.hasHealth {
		return nil
	}

	level, reason := shared.StatusGood, ""
	switch {
	case info.health < healthBadPercent:
		level = shared.StatusBad
		reason = fmt.Sprintf("below the %d%% threshold", healthBadPercent)
	case info.health < healthWarnPercent:
		level = shared.StatusWarn
		reason = fmt.Sprintf("below the %d%% threshold", healthWarnPercent)
	}

	// The figures behind the verdict, in the unit this battery reports. They
	// travel with the sentence either way, because a warning that does not say
	// what it compared is as unaccountable as a "Good" badge that does not.
	holds := fmt.Sprintf("it holds %.0f%% of the capacity it was designed for (%.2f of %.2f %s)",
		info.health, info.full, info.design, info.unit.Suffix())
	if info.hasCycles {
		holds += fmt.Sprintf(" over %s charge cycle(s)", info.cycles)
	}

	text := map[string]string{
		shared.StatusGood: "Health: Good",
		shared.StatusWarn: "Health: Worn",
		shared.StatusBad:  "Health: Poor",
	}[level]

	detail := "No problems: " + holds + "."
	if reason != "" {
		detail = "Health reports: " + holds + ", " + reason + "."
	}

	return &shared.DeviceStatus{Level: level, Text: text, Detail: detail}
}

// readText reads a power supply attribute, reporting false when it is absent or
// empty so that "no reading" stays distinguishable from a reading of zero.
func readText(dir, file string) (string, bool) {
	value, ok := shared.ReadSysFile(filepath.Join(dir, file))
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

// readNumber reads a numeric power supply attribute.
func readNumber(dir, file string) (float64, bool) {
	raw, ok := readText(dir, file)
	if !ok {
		return 0, false
	}
	number, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// readBool reads a power supply's 0/1 attribute, such as `present` or `online`.
func readBool(dir, file string) (bool, bool) {
	raw, ok := readText(dir, file)
	if !ok {
		return false, false
	}
	switch raw {
	case "1":
		return true, true
	case "0":
		return false, true
	}
	return false, false
}

// addReading records a micro-scaled power supply attribute — µWh, µAh or µV — as
// the unit its own name carries, keeping the fraction: the kernel reports these
// in millionths, and dividing in integer arithmetic would truncate a reading
// towards zero, which is visible as a whole-Watt or whole-degree step.
func addReading(values map[string]shared.HardwareValue, dir, file, key string, unit shared.HardwareUnit, valueType shared.HardwareValueType) (float64, bool) {
	micro, ok := readNumber(dir, file)
	if !ok {
		return 0, false
	}
	value := micro / 1e6
	values[key] = shared.NewHardwareValue(value, unit, valueType)
	return value, true
}
