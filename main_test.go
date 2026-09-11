package main

import (
	"reflect"
	"strconv"
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

	// "cpu.temp", "cpu.clock0", and "gpu.temp" should be parsed since they have
	// numeric types. "cpu.brand" should NOT be parsed because it is Unknown.
	if _, exists := minValues["cpu.brand"]; exists {
		t.Errorf("Expected cpu.brand to be ignored, but it was parsed")
	}

	// With a single sample, min and max are both the sampled value.
	expected := map[string]string{
		"cpu.temp":   "50.00",
		"cpu.clock0": "3800.00",
		"gpu.temp":   "60.00",
	}

	if !reflect.DeepEqual(minValues, expected) {
		t.Errorf("minValues = %v; expected %v", minValues, expected)
	}
	if !reflect.DeepEqual(maxValues, expected) {
		t.Errorf("maxValues = %v; expected %v", maxValues, expected)
	}
}

// TestUpdateHistoryAndCalculateMinMaxTracksExtremes verifies that min and max
// diverge once more than one sample has been recorded, and that they persist
// across calls rather than tracking only the latest reading.
func TestUpdateHistoryAndCalculateMinMaxTracksExtremes(t *testing.T) {
	history := make(map[string][]float64)
	samples := []string{"80C", "50C", "70C"}

	for _, sample := range samples {
		updateHistoryAndCalculateMinMax(history, map[string]shared.HardwareValue{
			"cpu.temp": {Value: sample, Unit: shared.Celsius, Type: shared.Temperature},
		})
	}

	minValues, maxValues := updateHistoryAndCalculateMinMax(history, map[string]shared.HardwareValue{
		"cpu.temp": {Value: "65C", Unit: shared.Celsius, Type: shared.Temperature},
	})

	if got, want := minValues["cpu.temp"], "50.00"; got != want {
		t.Errorf("minValues[cpu.temp] = %q; want %q", got, want)
	}
	if got, want := maxValues["cpu.temp"], "80.00"; got != want {
		t.Errorf("maxValues[cpu.temp] = %q; want %q", got, want)
	}
}

// TestUpdateHistoryKeepsBoundedWindow guards the memory cap: history must not
// grow without bound as the UI polls every few seconds.
func TestUpdateHistoryKeepsBoundedWindow(t *testing.T) {
	history := make(map[string][]float64)

	for i := 0; i < 150; i++ {
		updateHistoryAndCalculateMinMax(history, map[string]shared.HardwareValue{
			"cpu.temp": {Value: float64(i), Unit: shared.Celsius, Type: shared.Temperature},
		})
	}

	if got := len(history["cpu.temp"]); got != 100 {
		t.Errorf("history window = %d samples; want 100", got)
	}
}

// cpuValues builds a value map from key names, which is all the template
// generation looks at.
func cpuValues(keys ...string) map[string]shared.HardwareValue {
	values := make(map[string]shared.HardwareValue, len(keys))
	for _, key := range keys {
		values[key] = shared.NewHardwareValue(float64(1), shared.UnknownUnit, shared.Unknown)
	}
	return values
}

func labelsOf(templates []shared.KeyTemplate) []string {
	labels := make([]string, 0, len(templates))
	for _, t := range templates {
		labels = append(labels, t.Label)
	}
	return labels
}

func keysOfTemplates(templates []shared.KeyTemplate) []string {
	keys := make([]string, 0, len(templates))
	for _, t := range templates {
		keys = append(keys, t.Key)
	}
	return keys
}

// TestCPUKeyTemplatesIncludesEveryCore is the core of the change: a CPU that
// reports 16 clocks must produce 16 clock rows, not the first four.
func TestCPUKeyTemplatesIncludesEveryCore(t *testing.T) {
	var keys []string
	for i := 0; i < 16; i++ {
		keys = append(keys, "cpu.clock"+strconv.Itoa(i))
	}

	templates := cpuKeyTemplates(cpuValues(keys...))
	labels := labelsOf(templates)

	for i := 0; i < 16; i++ {
		want := "CPU Clock " + strconv.Itoa(i)
		if !containsString(labels, want) {
			t.Errorf("expected label %q to be present in %v", want, labels)
		}
	}
}

// TestCPUKeyTemplatesOrder verifies the display order: identity fields, then
// clocks, then package temperatures, then Tccd sensors.
func TestCPUKeyTemplatesOrder(t *testing.T) {
	templates := cpuKeyTemplates(cpuValues(
		"cpu.vendorID", "cpu.brand", "cpu.model",
		"cpu.clock0", "cpu.clock1", "cpu.clock2", "cpu.clock3",
		"cpu.temp", "cpu.pch",
		"cpu.coreTemp0", "cpu.coreTemp1",
		"tccd0", "tccd1",
	))

	want := []string{
		"cpu.vendorID", "cpu.brand", "cpu.model",
		"cpu.clock0", "cpu.clock1", "cpu.clock2", "cpu.clock3",
		"cpu.temp", "cpu.pch",
		"cpu.coreTemp0", "cpu.coreTemp1",
		"tccd0", "tccd1",
	}

	if got := keysOfTemplates(templates); !reflect.DeepEqual(got, want) {
		t.Errorf("template keys = %v; want %v", got, want)
	}
}

// TestCPUKeyTemplatesIncludesCoreTemperatures guards the per-core temperature
// rows: they are collected per physical core and must survive the template
// step, otherwise the frontend never receives them.
func TestCPUKeyTemplatesIncludesCoreTemperatures(t *testing.T) {
	templates := cpuKeyTemplates(cpuValues("cpu.coreTemp0", "cpu.coreTemp1", "cpu.coreTemp2"))

	labels := labelsOf(templates)
	for i := 0; i < 3; i++ {
		want := "Core " + strconv.Itoa(i) + " Temperature"
		if !containsString(labels, want) {
			t.Errorf("expected label %q to be present in %v", want, labels)
		}
	}
}

// TestCPUKeyTemplatesSortsNumerically guards against lexicographic ordering,
// which would place core 10 between core 1 and core 2.
func TestCPUKeyTemplatesSortsNumerically(t *testing.T) {
	templates := cpuKeyTemplates(cpuValues(
		"cpu.clock10", "cpu.clock2", "cpu.clock0", "cpu.clock1",
	))

	want := []string{
		"cpu.vendorID", "cpu.brand", "cpu.model",
		"cpu.clock0", "cpu.clock1", "cpu.clock2", "cpu.clock10",
		"cpu.temp", "cpu.pch",
	}
	if got := keysOfTemplates(templates); !reflect.DeepEqual(got, want) {
		t.Errorf("template keys = %v; want %v", got, want)
	}
}

// TestCPUKeyTemplatesIgnoresUnrelatedKeys ensures the prefix matching does not
// pick up non-numeric or partially matching keys.
func TestCPUKeyTemplatesIgnoresUnrelatedKeys(t *testing.T) {
	templates := cpuKeyTemplates(cpuValues(
		"cpu.clock", "cpu.clockX", "cpu.clocks", "tccd", "tccdZ",
	))

	for _, tpl := range templates {
		if len(tpl.Key) > 8 && (tpl.Key[:9] == "cpu.clock" || tpl.Key[:4] == "tccd") {
			t.Errorf("unexpected generated template %+v", tpl)
		}
	}
}

func TestCPUKeyTemplatesTccdLabels(t *testing.T) {
	templates := cpuKeyTemplates(cpuValues("tccd0", "tccd1"))
	labels := labelsOf(templates)

	if !containsString(labels, "Tccd 0 Temperature") || !containsString(labels, "Tccd 1 Temperature") {
		t.Errorf("expected Tccd temperature labels, got %v", labels)
	}
}

func TestCPUKeyTemplatesNoClocksOnNonX86(t *testing.T) {
	templates := cpuKeyTemplates(cpuValues("cpu.vendorID", "cpu.temp"))

	want := []string{"cpu.vendorID", "cpu.brand", "cpu.model", "cpu.temp", "cpu.pch"}
	if got := keysOfTemplates(templates); !reflect.DeepEqual(got, want) {
		t.Errorf("template keys = %v; want %v", got, want)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestUpdateHistorySkipsEmptyValues ensures a sensor that momentarily reports
// no value does not poison the min/max history with a zero reading.
func TestUpdateHistorySkipsEmptyValues(t *testing.T) {
	history := make(map[string][]float64)

	minValues, maxValues := updateHistoryAndCalculateMinMax(history, map[string]shared.HardwareValue{
		"gpu.usage": {Value: "", Unit: shared.Percent, Type: shared.Usage},
	})

	if _, exists := minValues["gpu.usage"]; exists {
		t.Errorf("expected empty value to be skipped, got min %v", minValues["gpu.usage"])
	}
	if _, exists := maxValues["gpu.usage"]; exists {
		t.Errorf("expected empty value to be skipped, got max %v", maxValues["gpu.usage"])
	}
	if len(history["gpu.usage"]) != 0 {
		t.Errorf("expected no history recorded for empty value, got %v", history["gpu.usage"])
	}
}
