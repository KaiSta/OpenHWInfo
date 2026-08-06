package main

import (
	"reflect"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

func TestUpdateHistoryAndCalculateMinMax(t *testing.T) {
	history := make(map[string][]float64)
	currentValues := map[string]shared.HardwareValue{
		"cpu.temp":   {Value: "50C", Unit: shared.Celsius, Type: shared.Temperature},
		"cpu.clock0": {Value: "3800.00 MHz", Unit: shared.MHz, Type: shared.Frequency},
		"cpu.brand":  {Value: "Intel Core", Unit: shared.UnknownUnit, Type: shared.Unknown},
		"gpu.temp":   {Value: "60 °C", Unit: shared.Celsius, Type: shared.Temperature},
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
