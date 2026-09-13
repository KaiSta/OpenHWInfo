package shared

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// HardwareUnit defines the unit types for hardware values
type HardwareUnit int

const (
	UnknownUnit HardwareUnit = iota
	Celsius
	MHz
	Percent
	Watt
	Megabyte
	RPM
	// Gigabyte is appended rather than grouped with Megabyte so the existing
	// unit values keep their numbers. Drive capacities and filesystem sizes are
	// reported in GB because that is how drives are sold and labelled.
	Gigabyte
	// Terabyte is the unit of the lifetime-written counter a drive's SMART data
	// carries: total writes are quoted in TB (the "TBW" of an endurance rating),
	// not in the gigabytes capacities are sold in.
	Terabyte
	// Hour is the unit of the SMART power-on counter.
	Hour
	// Gibibyte is the binary counterpart of Gigabyte, used for memory. RAM and
	// swap are quoted in GiB — a 16 GiB machine holds exactly 16 GiB — while a
	// drive's capacity is quoted in the decimal GB it is sold in. Rendering both
	// as "GB" would make the two disagree by 7% on every machine, so the units
	// stay distinct.
	Gibibyte
	// WattHour and AmpereHour are the two ways a battery's firmware reports how
	// much it holds: most laptops publish energy (Wh), some publish charge (Ah)
	// instead, and the kernel exposes whichever it gets. They are separate units
	// because a value must be read with the quantity it actually is — the unit on
	// the reading is what says which of the two the firmware is quoting.
	WattHour
	AmpereHour
	// Volt is a battery's voltage.
	Volt
)

// Suffix is the unit text shown beside a value, and the only place a unit
// becomes display text. Deriving it from the unit rather than spelling it into a
// row label is what stops a value and its label from disagreeing about what is
// being measured -- the failure behind a lifetime-write count that was labelled
// in terabytes while the number was something else entirely.
//
// Megabyte is binary despite the name: every collector that uses it either
// divides by 1024*1024 (amd.parseMemoryValue) or receives MiB from its source
// (nvidia-smi reports memory.total in MiB), so it is rendered as MiB.
func (u HardwareUnit) Suffix() string {
	switch u {
	case Celsius:
		return "°C"
	case MHz:
		return "MHz"
	case Percent:
		return "%"
	case Watt:
		return "W"
	case Megabyte:
		return "MiB"
	case Gigabyte:
		return "GB"
	case Terabyte:
		return "TB"
	case RPM:
		return "RPM"
	case Hour:
		return "h"
	case Gibibyte:
		return "GiB"
	case WattHour:
		return "Wh"
	case AmpereHour:
		return "Ah"
	case Volt:
		return "V"
	}
	// UnknownUnit carries no text, and an unrecognised unit is not guessed at.
	return ""
}

// HardwareValue represents a hardware metric with its value and metadata
type HardwareValue struct {
	Value interface{} // Can be string, float64, etc.
	Unit  HardwareUnit
	Type  HardwareValueType
}

// HardwareValueType defines the type of hardware value for min/max calculation purposes
type HardwareValueType int

const (
	Unknown HardwareValueType = iota
	Temperature
	Frequency
	Usage
	Power
	Memory
)

// DeviceStatus is a device's own verdict about itself, shown as a badge on its
// card. Level is one of the values the frontend already styles for metric bars
// ("good", "warn", "bad") plus "unknown", so a verdict reuses the palette
// rather than introducing a second one. Detail explains the verdict and is
// surfaced as the badge's tooltip, because a warning a user cannot account for
// is worse than no warning.
type DeviceStatus struct {
	Level  string
	Text   string
	Detail string
}

// Status levels. "unknown" means the device supports a health protocol but no
// reading of it is available yet — different from having no verdict at all,
// which is a nil Status.
const (
	StatusGood    = "good"
	StatusWarn    = "warn"
	StatusBad     = "bad"
	StatusUnknown = "unknown"
)

// HardwareData represents a collection of hardware values
type HardwareData struct {
	Name   string
	Values map[string]HardwareValue
	// Status is the device's health verdict, or nil for devices that do not
	// report one (GPUs, the mainboard).
	Status *DeviceStatus
}

// HasTelemetry reports whether the data carries any reading beyond its identity
// fields (name and driver version). Devices that expose no sensors are filtered
// out with this so they are not rendered as empty entries.
func (hd HardwareData) HasTelemetry() bool {
	for key, value := range hd.Values {
		if key == GPUName.Key || key == GPUDriverVersion.Key {
			continue
		}
		if value.Value != nil && value.Value != "" {
			return true
		}
	}
	return false
}

// NewHardwareValue creates a new HardwareValue with proper type conversion
func NewHardwareValue(value interface{}, unit HardwareUnit, valueType HardwareValueType) HardwareValue {
	return HardwareValue{
		Value: value,
		Unit:  unit,
		Type:  valueType,
	}
}

// GetNumericValue extracts the numeric part from a HardwareValue's Value field
func (hv *HardwareValue) GetNumericValue() (float64, error) {
	// If Value is already a float64, return it directly
	if val, ok := hv.Value.(float64); ok {
		return val, nil
	}

	// If Value is a string, parse it
	if valStr, ok := hv.Value.(string); ok {
		// Remove common units and symbols to get just the number
		value := strings.TrimSpace(valStr)

		// Remove common unit suffixes that may be present in the string
		// Handle cases like "50C", "3800.00 MHz", "60 °C"
		// Remove any non-numeric characters except decimal points and minus signs
		var numericStr strings.Builder
		for _, char := range value {
			if unicode.IsDigit(char) || char == '.' || char == '-' {
				numericStr.WriteRune(char)
			}
		}

		// Try to parse the numeric value
		num, err := strconv.ParseFloat(numericStr.String(), 64)
		if err != nil {
			return 0, err
		}

		return num, nil
	}

	return 0, fmt.Errorf("cannot convert to numeric value")
}

// IsNumericType returns true if this HardwareValue represents a numeric type that can be used for min/max calculations
func (hv *HardwareValue) IsNumericType() bool {
	return hv.Type == Temperature || hv.Type == Frequency || hv.Type == Memory || hv.Type == Usage || hv.Type == Power
}
