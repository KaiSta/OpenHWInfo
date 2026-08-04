package main

import (
	"reflect"
	"testing"
)

func TestHasUnit(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"45.0 °C", true},
		{"45C", true},
		{"1200.00 MHz", true},
		{"1500 Mhz", true},
		{"150 W", true},
		{"8192 MB", true},
		{"12%", true},
		{"1200.00", false},
		{"Intel Core", false},
		{"", false},
	}

	for _, test := range tests {
		actual := hasUnit(test.input)
		if actual != test.expected {
			t.Errorf("hasUnit(%q) = %v; expected %v", test.input, actual, test.expected)
		}
	}
}

func TestUpdateHistoryAndCalculateMinMax(t *testing.T) {
	history := make(map[string][]float64)
	currentValues := map[string]string{
		"cpu.temp":   "50C",
		"cpu.clock0": "3800.00 MHz",
		"cpu.brand":  "Intel Core",
		"gpu.temp":   "60 °C",
	}

	minValues, maxValues := updateHistoryAndCalculateMinMax(history, currentValues)

	// "cpu.temp", "cpu.clock0", and "gpu.temp" should be parsed since they have units.
	// "cpu.brand" should NOT be parsed because it has no unit.
	if _, exists := minValues["cpu.brand"]; exists {
		t.Errorf("Expected cpu.brand to be ignored, but it was parsed")
	}

	expectedMin := map[string]string{
		"cpu.temp":   "50.00",
		"cpu.clock0": "3800.00",
		"gpu.temp":   "60.00",
	}

	if !reflect.DeepEqual(minValues, expectedMin) {
		t.Errorf("minValues = %v; expected %v", minValues, expectedMin)
	}

	if !reflect.DeepEqual(maxValues, expectedMin) {
		t.Errorf("maxValues = %v; expected %v", maxValues, expectedMin)
	}
}
