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
)

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

// HardwareData represents a collection of hardware values
type HardwareData struct {
	Name   string
	Values map[string]HardwareValue
	IsGPU  bool
	IsCPU  bool
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
