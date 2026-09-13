package main

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/KaiSta/OpenHWInfo/battery"
	"github.com/KaiSta/OpenHWInfo/memory"
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

// TestCPUKeyTemplatesMatchCollectorKeyContract encodes what the frontend does
// with each key family the collector emits. Two real bugs lived here: a metric
// the frontend renders (cpu.temp) was collected under a key no template named,
// so it silently never displayed, and the topology keys (cpu.coreOfN) must
// travel in the values map — the per-core grid reads them to attribute SMT
// siblings — but must never become table rows.
func TestCPUKeyTemplatesMatchCollectorKeyContract(t *testing.T) {
	values := make(map[string]shared.HardwareValue)
	for _, key := range []string{"cpu.vendorID", "cpu.brand", "cpu.temp", "cpu.pch"} {
		values[key] = shared.NewHardwareValue("x", shared.UnknownUnit, shared.Unknown)
	}
	for i := 0; i < 8; i++ {
		values[fmt.Sprintf("cpu.clock%d", i)] = shared.NewHardwareValue(float64(3400+i), shared.MHz, shared.Frequency)
		values[fmt.Sprintf("cpu.coreOf%d", i)] = shared.NewHardwareValue(float64(i%4), shared.UnknownUnit, shared.Unknown)
	}
	for i := 0; i < 4; i++ {
		values[fmt.Sprintf("cpu.coreTemp%d", i)] = shared.NewHardwareValue(float64(45+i), shared.Celsius, shared.Temperature)
	}

	templates := cpuKeyTemplates(values)

	present := make(map[string]bool, len(templates))
	for _, tpl := range templates {
		present[tpl.Key] = true
		if strings.Contains(tpl.Key, "coreOf") {
			t.Errorf("topology key %q became a table row; the frontend treats it as data, not a metric", tpl.Key)
		}
	}

	// Every collected clock and core sensor must be reachable by the frontend.
	for key := range values {
		if !strings.HasPrefix(key, "cpu.clock") && !strings.HasPrefix(key, "cpu.coreTemp") {
			continue
		}
		if !present[key] {
			t.Errorf("collected key %q has no template and would never be displayed", key)
		}
	}

	for _, key := range []string{"cpu.temp", "cpu.pch"} {
		if !present[key] {
			t.Errorf("%q has no template and would never be displayed", key)
		}
	}
}

// boardValue builds a numeric board reading, i.e. a temperature or a fan.
func boardValue() shared.HardwareValue {
	return shared.NewHardwareValue(float64(42), shared.Celsius, shared.Temperature)
}

// boardLabel builds the hidden label key value the collector stores alongside a
// sensor row.
func boardLabel(text string) shared.HardwareValue {
	return shared.NewHardwareValue(text, shared.UnknownUnit, shared.Unknown)
}

// TestBoardKeyTemplatesOrder verifies the display order: identity and firmware
// fields first, then the board's temperatures, then its fans.
func TestBoardKeyTemplatesOrder(t *testing.T) {
	values := map[string]shared.HardwareValue{
		"board.vendor":       boardLabel("ASUSTeK COMPUTER INC."),
		"board.model":        boardLabel("PRIME B450M-A"),
		"board.version":      boardLabel("Rev X.0x"),
		"board.serial":       boardLabel("1234567890"),
		"board.bios.version": boardLabel("2409"),
		"board.bios.date":    boardLabel("07/10/2020"),
		"board.temp0":        boardValue(),
		"board.temp1":        boardValue(),
		"board.temp2":        boardValue(),
		"board.temp10":       boardValue(),
		"board.fan0":         boardValue(),
		"board.fan1":         boardValue(),
	}

	want := []string{
		"board.vendor", "board.model", "board.version", "board.serial",
		"board.bios.version", "board.bios.date",
		"board.temp0", "board.temp1", "board.temp2", "board.temp10",
		"board.fan0", "board.fan1",
	}
	if got := keysOfTemplates(boardKeyTemplates(values)); !reflect.DeepEqual(got, want) {
		t.Errorf("template keys = %v; want %v", got, want)
	}
}

// TestBoardKeyTemplatesUseSensorNames is the reason the collector keeps the
// sensor's own name: a row labelled "SYSTIN Temperature" is readable, and the
// same row labelled "Temperature 0" is not.
func TestBoardKeyTemplatesUseSensorNames(t *testing.T) {
	values := map[string]shared.HardwareValue{
		"board.temp0":      boardValue(),
		"board.tempLabel0": boardLabel("SYSTIN Temperature"),
		"board.temp1":      boardValue(),
		"board.tempLabel1": boardLabel("pch_skylake Temperature"),
		"board.fan0":       boardValue(),
		"board.fanLabel0":  boardLabel("Chassis Fan Speed"),
	}

	labels := labelsOf(boardKeyTemplates(values))

	for _, want := range []string{"SYSTIN Temperature", "pch_skylake Temperature", "Chassis Fan Speed"} {
		if !containsString(labels, want) {
			t.Errorf("expected label %q to be present in %v", want, labels)
		}
	}
}

// TestBoardKeyTemplatesFallbackLabels covers a sensor whose driver provides no
// label attribute: the row must still be nameable.
func TestBoardKeyTemplatesFallbackLabels(t *testing.T) {
	values := map[string]shared.HardwareValue{
		"board.temp3": boardValue(),
		"board.fan2":  boardValue(),
	}

	labels := labelsOf(boardKeyTemplates(values))

	if !containsString(labels, "Temperature 3") {
		t.Errorf("expected a generic temperature label, got %v", labels)
	}
	if !containsString(labels, "Fan 2 Speed") {
		t.Errorf("expected a generic fan label, got %v", labels)
	}
}

// TestBoardKeyTemplatesMatchCollectorKeyContract mirrors the CPU contract test:
// every collected sensor must reach the frontend, and the label keys the
// collector stores for its own rows must never become rows themselves.
func TestBoardKeyTemplatesMatchCollectorKeyContract(t *testing.T) {
	values := map[string]shared.HardwareValue{
		"board.vendor":     boardLabel("LENOVO"),
		"board.model":      boardLabel("Lenovo Y50-70"),
		"board.temp0":      boardValue(),
		"board.tempLabel0": boardLabel("acpitz Temperature"),
		"board.fan0":       boardValue(),
		"board.fanLabel0":  boardLabel("Chassis Fan Speed"),
	}

	templates := boardKeyTemplates(values)

	present := make(map[string]bool, len(templates))
	for _, tpl := range templates {
		present[tpl.Key] = true
		if strings.Contains(tpl.Key, "Label") {
			t.Errorf("label key %q became a table row; it names a row rather than reporting a metric", tpl.Key)
		}
	}

	for key := range values {
		if strings.HasPrefix(key, "board.tempLabel") || strings.HasPrefix(key, "board.fanLabel") {
			continue
		}
		if !strings.HasPrefix(key, "board.temp") && !strings.HasPrefix(key, "board.fan") {
			continue
		}
		if !present[key] {
			t.Errorf("collected key %q has no template and would never be displayed", key)
		}
	}
}

// TestBoardHasNoRowsWhenItReportsNothing keeps an empty mainboard card off the
// screen on systems that expose no board data at all.
func TestBoardHasNoRowsWhenItReportsNothing(t *testing.T) {
	data := shared.HardwareData{Name: "Mainboard", Values: map[string]shared.HardwareValue{}}
	if view := boardViewFor(data); len(view.Keys) != 0 {
		t.Errorf("expected no rows for an empty board, got %+v", view.Keys)
	}
}

// boardViewFor builds the mainboard view the way GetData does.
func boardViewFor(data shared.HardwareData) DeviceView {
	return deviceView(data, boardKeyTemplates(data.Values), newHistoryStore(), "")
}

// TestBoardViewKeepsOnlyRowsWithValues covers the partial board: a machine whose
// firmware leaves fields blank must show the rows it does have.
func TestBoardViewKeepsOnlyRowsWithValues(t *testing.T) {
	view := boardViewFor(shared.HardwareData{
		Name: "Lenovo Y50-70",
		Values: map[string]shared.HardwareValue{
			"board.vendor":     boardLabel("LENOVO"),
			"board.model":      boardLabel("Lenovo Y50-70"),
			"board.serial":     boardLabel(""),
			"board.temp0":      boardValue(),
			"board.tempLabel0": boardLabel("acpitz Temperature"),
		},
	})
	if len(view.Keys) == 0 {
		t.Fatal("expected rows for a board that reports values")
	}
	if view.Name != "Lenovo Y50-70" {
		t.Errorf("Name = %q; want the DMI board name", view.Name)
	}
	if got := keysOfTemplates(view.Keys); containsString(got, "board.serial") {
		t.Errorf("an empty serial must not produce a row, got %v", got)
	}
	if !containsString(keysOfTemplates(view.Keys), "board.temp0") {
		t.Errorf("expected the temperature row, got %v", keysOfTemplates(view.Keys))
	}

	// Temperatures are numeric, so the card gets min/max ranges for them; the
	// identity strings must not be handed to the history at all.
	if _, ok := view.MinValues["board.temp0"]; !ok {
		t.Error("expected a min range for the temperature row")
	}
	if _, ok := view.MinValues["board.vendor"]; ok {
		t.Error("an identity string must not enter the min/max history")
	}
}

// TestDiskKeyTemplatesOrder verifies a drive card's row order: identity and
// medium, then capacity, then the filesystem figures, then temperatures.
func TestDiskKeyTemplatesOrder(t *testing.T) {
	values := map[string]shared.HardwareValue{
		"disk.model":      boardLabel("Samsung SSD 990 PRO 2TB"),
		"disk.serial":     boardLabel("S7KHNJ0W512345"),
		"disk.firmware":   boardLabel("4B2QJXD7"),
		"disk.medium":     boardLabel("SSD (NVMe)"),
		"disk.size":       diskSize(2000),
		"disk.fs.total":   diskSize(1800),
		"disk.fs.used":    diskSize(900),
		"disk.fs.free":    diskSize(900),
		"disk.smart.life": boardValue(),
		"disk.temp0":      boardValue(),
		"disk.tempLabel0": boardLabel("Composite Temperature"),
		"disk.temp1":      boardValue(),
		"disk.tempLabel1": boardLabel("Sensor 1 Temperature"),
	}

	// Identity, size and filesystem first, then the SMART rows, then the
	// temperature sensors the drive reports. The SMART rows keep their listed
	// order because they are the fixed part of the card.
	want := []string{
		"disk.model", "disk.serial", "disk.firmware", "disk.medium", "disk.size",
		"disk.fs.total", "disk.fs.used", "disk.fs.free",
		"disk.smart.life", "disk.smart.written", "disk.smart.hours",
		"disk.smart.badSectors", "disk.smart.spare", "disk.smart.mediaErrors",
		"disk.smart.selftest", "disk.smart.warnings",
		"disk.temp0", "disk.temp1",
	}
	if got := keysOfTemplates(diskKeyTemplates(values)); !reflect.DeepEqual(got, want) {
		t.Errorf("template keys = %v; want %v", got, want)
	}
}

// TestDiskKeyTemplatesUseSensorLabels is why the collector stores each sensor's
// own name: "Composite Temperature" identifies the reading, "Drive Temperature 0"
// does not.
func TestDiskKeyTemplatesUseSensorLabels(t *testing.T) {
	values := map[string]shared.HardwareValue{
		"disk.temp0":      boardValue(),
		"disk.tempLabel0": boardLabel("Composite Temperature"),
		"disk.temp1":      boardValue(),
	}

	labels := labelsOf(diskKeyTemplates(values))

	if !containsString(labels, "Composite Temperature") {
		t.Errorf("expected the sensor label, got %v", labels)
	}
	if !containsString(labels, "Drive Temperature 1") {
		t.Errorf("expected a generic label for the unlabelled sensor, got %v", labels)
	}
}

// TestDiskKeyTemplatesMatchCollectorKeyContract mirrors the CPU and board
// contract tests: every collected reading must reach the frontend, and the
// label keys must never become rows themselves. The SMART rows are collected by
// the storage package under keys it does not own, so this is the only place the
// two halves of that contract meet.
func TestDiskKeyTemplatesMatchCollectorKeyContract(t *testing.T) {
	values := map[string]shared.HardwareValue{
		"disk.model":             boardLabel("SAMSUNG MZ7TE512"),
		"disk.medium":            boardLabel("SSD"),
		"disk.size":              diskSize(512),
		"disk.temp0":             boardValue(),
		"disk.tempLabel0":        boardLabel("Composite Temperature"),
		"disk.smart.life":        boardValue(),
		"disk.smart.written":     boardValue(),
		"disk.smart.hours":       boardValue(),
		"disk.smart.badSectors":  boardValue(),
		"disk.smart.spare":       boardValue(),
		"disk.smart.mediaErrors": boardValue(),
		"disk.smart.selftest":    boardLabel("Success"),
		"disk.smart.warnings":    boardLabel("spare"),
	}

	templates := diskKeyTemplates(values)

	present := make(map[string]bool, len(templates))
	for _, tpl := range templates {
		present[tpl.Key] = true
		if strings.Contains(tpl.Key, "Label") {
			t.Errorf("label key %q became a table row; it names a row rather than reporting a metric", tpl.Key)
		}
	}

	for key := range values {
		if strings.HasPrefix(key, "disk.tempLabel") {
			continue
		}
		if !present[key] {
			t.Errorf("collected key %q has no template and would never be displayed", key)
		}
	}

	// A drive that reports no SMART gets no SMART rows: the templates exist, but
	// the view keeps only the rows that carry a value, so an HDD with no
	// readable attributes does not show empty wear and spare rows.
	plain := map[string]shared.HardwareValue{
		"disk.model":  boardLabel("WDC WD40EFRX-68N"),
		"disk.medium": boardLabel("HDD"),
		"disk.size":   diskSize(4000),
	}
	view := deviceView(shared.HardwareData{Name: "WDC WD40EFRX-68N", Values: plain},
		diskKeyTemplates(plain), newHistoryStore(), "disk0.")
	for _, row := range view.Keys {
		if strings.HasPrefix(row.Key, "disk.smart.") {
			t.Errorf("row %q was rendered for a drive that reports no SMART", row.Key)
		}
	}
}

// TestMemTemplatesMatchCollectorKeyContract mirrors the other contract tests for
// the memory card: every key the collector writes must be named by a template,
// or the reading would be collected and never displayed. The template list is
// the fixed one in memTemplates, so this is where the collector's keys and the
// card's rows have to agree.
func TestMemTemplatesMatchCollectorKeyContract(t *testing.T) {
	present := make(map[string]bool, len(memTemplates))
	for _, tpl := range memTemplates {
		if present[tpl.Key] {
			t.Errorf("key %q has two templates", tpl.Key)
		}
		present[tpl.Key] = true
	}

	// The keys the collector reports, taken from the collector itself. A machine
	// without swap or without PSI simply reports fewer of them.
	data := memory.GetInformations()
	if len(data.Values) == 0 {
		t.Skipf("no memory data on this host (%s unreadable)", "/proc/meminfo")
	}
	for key := range data.Values {
		if !present[key] {
			t.Errorf("collected key %q has no template and would never be displayed", key)
		}
	}

	// And the reverse: every template the collector can fill is one it knows how
	// to fill, so a template named here is not a row that can never appear.
	known := map[string]bool{
		shared.MemTotal.Key:        true,
		shared.MemUsed.Key:         true,
		shared.MemAvailable.Key:    true,
		shared.MemUsage.Key:        true,
		shared.SwapTotal.Key:       true,
		shared.SwapUsed.Key:        true,
		shared.SwapUsage.Key:       true,
		shared.MemPressureSome.Key: true,
		shared.MemPressureFull.Key: true,
	}
	for _, tpl := range memTemplates {
		if !known[tpl.Key] {
			t.Errorf("template %q names no key the memory collector writes", tpl.Key)
		}
	}
}

// TestMemTemplatesOrder pins the reading order of the memory card: the RAM the
// machine holds, how much of it is in use, what is left, then swap and finally
// the kernel's pressure-stall averages.
func TestMemTemplatesOrder(t *testing.T) {
	got := keysOfTemplates(memTemplates)
	want := []string{
		"mem.total", "mem.used", "mem.available", "mem.usage",
		"swap.total", "swap.used", "swap.usage",
		"mem.pressure.some", "mem.pressure.full",
	}

	if len(got) != len(want) {
		t.Fatalf("memTemplates has %d rows; want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q; want %q (full order %v)", i, got[i], want[i], got)
		}
	}
}

// TestMemoryViewKeepsOnlyReportedRows covers the swapless machine: the swap
// templates exist, but a machine that reports no swap shows no swap rows rather
// than three zeroes.
func TestMemoryViewKeepsOnlyReportedRows(t *testing.T) {
	values := map[string]shared.HardwareValue{
		shared.MemTotal.Key:     shared.NewHardwareValue(16.0, shared.Gibibyte, shared.Memory),
		shared.MemUsed.Key:      shared.NewHardwareValue(12.0, shared.Gibibyte, shared.Memory),
		shared.MemAvailable.Key: shared.NewHardwareValue(4.0, shared.Gibibyte, shared.Memory),
		shared.MemUsage.Key:     shared.NewHardwareValue(75.0, shared.Percent, shared.Usage),
	}

	view := deviceView(shared.HardwareData{Name: "Memory", Values: values},
		memTemplates, newHistoryStore(), "")

	if len(view.Keys) != 4 {
		t.Fatalf("expected 4 rows for a swapless machine, got %v", keysOfTemplates(view.Keys))
	}
	for _, row := range view.Keys {
		if strings.HasPrefix(row.Key, "swap.") || strings.HasPrefix(row.Key, "mem.pressure.") {
			t.Errorf("row %q was rendered without a reading", row.Key)
		}
	}
}

// TestDeviceViewCarriesTheHealthStatus pins that a drive's verdict survives the
// trip from the collector to the view the frontend renders, and that a device
// without one (a GPU, say) does not invent a badge.
func TestDeviceViewCarriesTheHealthStatus(t *testing.T) {
	verdict := &shared.DeviceStatus{Level: shared.StatusWarn, Text: "SMART: Warning", Detail: "SMART reports: 8 bad sector(s)."}

	disk := deviceView(shared.HardwareData{
		Name:   "SAMSUNG MZ7TE512",
		Values: map[string]shared.HardwareValue{"disk.model": boardLabel("SAMSUNG MZ7TE512")},
		Status: verdict,
	}, diskKeyTemplates(nil), newHistoryStore(), "disk0.")

	if disk.Status != verdict {
		t.Errorf("drive status = %+v; want %+v", disk.Status, verdict)
	}

	gpu := deviceView(shared.HardwareData{
		Name:   "NVIDIA GeForce GTX 860M",
		Values: map[string]shared.HardwareValue{"driverVersion": boardLabel("580")},
	}, gpuTemplates, newHistoryStore(), "gpu0.")

	if gpu.Status != nil {
		t.Errorf("gpu status = %+v; want none", gpu.Status)
	}
}

// TestDeviceViewScopesHistoryPerDevice guards the reason each card has a history
// prefix: two drives (or two GPUs) reporting the same metric key must not share
// one min/max range. The keys the frontend reads are unprefixed, so the prefix
// must not leak into the view either.
func TestDeviceViewScopesHistoryPerDevice(t *testing.T) {
	first := newHistoryStore()
	second := newHistoryStore()

	cold := deviceView(shared.HardwareData{
		Name:   "drive 0",
		Values: map[string]shared.HardwareValue{"disk.temp0": diskTemp(30)},
	}, diskKeyTemplates(map[string]shared.HardwareValue{"disk.temp0": diskTemp(30)}), first, "disk0.")

	warm := deviceView(shared.HardwareData{
		Name:   "drive 1",
		Values: map[string]shared.HardwareValue{"disk.temp0": diskTemp(70)},
	}, diskKeyTemplates(map[string]shared.HardwareValue{"disk.temp0": diskTemp(70)}), second, "disk1.")

	// The unit travels with the figure, so the range states what it measures.
	if got := cold.MaxValues["disk.temp0"]; got != "30.00 °C" {
		t.Errorf("drive 0 max = %q; want its own reading", got)
	}
	if got := warm.MaxValues["disk.temp0"]; got != "70.00 °C" {
		t.Errorf("drive 1 max = %q; want its own reading", got)
	}
	for key := range cold.MinValues {
		if strings.Contains(key, ".disk") || strings.HasPrefix(key, "disk0") {
			t.Errorf("history prefix leaked into the view under key %q", key)
		}
	}
}

// TestFormatValueAppendsTheUnit covers why the unit belongs to the reading: a
// numeric value states what it measures, so a row cannot be labelled in one unit
// while carrying a figure in another.
func TestFormatValueAppendsTheUnit(t *testing.T) {
	cases := []struct {
		name  string
		value shared.HardwareValue
		want  string
	}{
		{"terabytes", shared.NewHardwareValue(14.95, shared.Terabyte, shared.Memory), "14.95 TB"},
		{"percent", shared.NewHardwareValue(4.2, shared.Percent, shared.Usage), "4.20 %"},
		{"celsius", shared.NewHardwareValue(38.625, shared.Celsius, shared.Temperature), "38.62 °C"},
		{"mebibytes", shared.NewHardwareValue(24564.0, shared.Megabyte, shared.Memory), "24564.00 MiB"},
		{"hours", shared.NewHardwareValue(120.0, shared.Hour, shared.Usage), "120.00 h"},
		// A unit the app does not describe renders bare rather than guessed at.
		{"unknown unit", shared.NewHardwareValue(120.0, shared.UnknownUnit, shared.Usage), "120.00"},
		// Text values come from fallback paths that embed their own unit.
		{"text is passed through", shared.NewHardwareValue("N/A MHz", shared.MHz, shared.Frequency), "N/A MHz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatValue(tc.value); got != tc.want {
				t.Errorf("formatValue = %q; want %q", got, tc.want)
			}
		})
	}
}

// TestMinMaxCarryTheSameUnitAsTheReading keeps a row's three numeric columns
// consistent. The history stores bare numbers, so the unit has to be attached
// while the reading it describes is still in hand.
func TestMinMaxCarryTheSameUnitAsTheReading(t *testing.T) {
	values := map[string]shared.HardwareValue{
		"disk.smart.written": shared.NewHardwareValue(14.95, shared.Terabyte, shared.Memory),
	}
	view := deviceView(shared.HardwareData{Name: "drive", Values: values},
		diskKeyTemplates(values), newHistoryStore(), "disk0.")

	columns := map[string]string{
		"current": view.Values["disk.smart.written"],
		"min":     view.MinValues["disk.smart.written"],
		"max":     view.MaxValues["disk.smart.written"],
	}
	for name, column := range columns {
		if column == "" {
			t.Fatalf("%s column is missing", name)
		}
		if !strings.HasSuffix(column, " TB") {
			t.Errorf("%s column = %q; want it to carry the reading's unit (TB)", name, column)
		}
	}
}

// TestBatteryTemplatesOrder pins the reading order of a battery card: what the
// battery is doing, how full it is, what it holds, how worn it is, and then what
// its firmware says about itself.
func TestBatteryTemplatesOrder(t *testing.T) {
	got := keysOfTemplates(batteryTemplates)
	want := []string{
		"battery.status", "battery.capacity",
		"battery.energy.now", "battery.energy.full", "battery.energy.design",
		"battery.charge.now", "battery.charge.full", "battery.charge.design",
		"battery.health", "battery.power",
		"battery.time.empty", "battery.time.full",
		"battery.voltage", "battery.voltage.design", "battery.cycles",
		"battery.model", "battery.manufacturer", "battery.serial",
		"battery.technology", "battery.ac",
	}

	if len(got) != len(want) {
		t.Fatalf("batteryTemplates has %d rows; want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q; want %q (full order %v)", i, got[i], want[i], got)
		}
	}
}

// TestBatteryTemplatesMatchCollectorKeyContract mirrors the other contract tests
// for the battery collector: every row must be one the collector can fill, and
// every reading the collector produces must have a row. The second half runs
// against the machine's own battery, so it skips on a desktop.
func TestBatteryTemplatesMatchCollectorKeyContract(t *testing.T) {
	known := map[string]bool{
		shared.BatteryStatus.Key:        true,
		shared.BatteryCharge.Key:        true,
		shared.BatteryEnergyNow.Key:     true,
		shared.BatteryEnergyFull.Key:    true,
		shared.BatteryEnergyDesign.Key:  true,
		shared.BatteryChargeNow.Key:     true,
		shared.BatteryChargeFull.Key:    true,
		shared.BatteryChargeDesign.Key:  true,
		shared.BatteryHealth.Key:        true,
		shared.BatteryPower.Key:         true,
		shared.BatteryTimeEmpty.Key:     true,
		shared.BatteryTimeFull.Key:      true,
		shared.BatteryVoltage.Key:       true,
		shared.BatteryDesignVoltage.Key: true,
		shared.BatteryCycles.Key:        true,
		shared.BatteryModel.Key:         true,
		shared.BatteryManufacturer.Key:  true,
		shared.BatterySerial.Key:        true,
		shared.BatteryTechnology.Key:    true,
		shared.BatteryAC.Key:            true,
	}

	present := make(map[string]bool, len(batteryTemplates))
	for _, tpl := range batteryTemplates {
		if present[tpl.Key] {
			t.Errorf("key %q has two templates", tpl.Key)
		}
		present[tpl.Key] = true
		if !known[tpl.Key] {
			t.Errorf("template %q names no key the battery collector writes", tpl.Key)
		}
	}

	cards := battery.GetInformations()
	if len(cards) == 0 {
		t.Skip("no battery on this host")
	}
	for _, card := range cards {
		for key := range card.Values {
			if !present[key] {
				t.Errorf("battery %s reports %q, which has no template and would never be displayed", card.Name, key)
			}
		}
	}
}

// TestBatteryViewKeepsOnlyReportedRows covers the capacity families: a battery
// publishes energy (Wh) or charge (Ah), never both, so the other family's rows
// are templates without a reading behind them.
func TestBatteryViewKeepsOnlyReportedRows(t *testing.T) {
	values := map[string]shared.HardwareValue{
		shared.BatteryStatus.Key:    boardLabel("Discharging"),
		shared.BatteryCharge.Key:    shared.NewHardwareValue(64.0, shared.Percent, shared.Usage),
		shared.BatteryEnergyNow.Key: shared.NewHardwareValue(30.0, shared.WattHour, shared.Memory),
		shared.BatteryAC.Key:        boardLabel("Offline"),
	}

	view := deviceView(shared.HardwareData{Name: "BAT1", Values: values},
		batteryTemplates, newHistoryStore(), "battery0.")

	if got := keysOfTemplates(view.Keys); !reflect.DeepEqual(got, []string{
		"battery.status", "battery.capacity", "battery.energy.now", "battery.ac",
	}) {
		t.Errorf("rows = %v; want only the reported ones in display order", got)
	}
	for _, row := range view.Keys {
		if strings.HasPrefix(row.Key, "battery.charge.") {
			t.Errorf("row %q was rendered without a reading", row.Key)
		}
	}
	// The unit travels with the figure, so the row states what it measures.
	if got := view.Values[shared.BatteryEnergyNow.Key]; got != "30.00 Wh" {
		t.Errorf("energy now = %q; want %q", got, "30.00 Wh")
	}
	if got := view.Values[shared.BatteryCharge.Key]; got != "64.00 %" {
		t.Errorf("charge = %q; want %q", got, "64.00 %")
	}
}

// TestTwoBatteryViewsKeepSeparateHistories is the multi-battery case the collector
// exists for: a laptop's second battery has its own capacity and its own wear, so
// the two cards must not share a min/max range.
func TestTwoBatteryViewsKeepSeparateHistories(t *testing.T) {
	first := newHistoryStore()
	second := newHistoryStore()

	spare := deviceView(shared.HardwareData{
		Name:   "BAT0",
		Values: map[string]shared.HardwareValue{shared.BatteryCharge.Key: shared.NewHardwareValue(12.0, shared.Percent, shared.Usage)},
	}, batteryTemplates, first, "battery0.")

	charged := deviceView(shared.HardwareData{
		Name:   "BAT1",
		Values: map[string]shared.HardwareValue{shared.BatteryCharge.Key: shared.NewHardwareValue(97.0, shared.Percent, shared.Usage)},
	}, batteryTemplates, second, "battery1.")

	if got := spare.MaxValues[shared.BatteryCharge.Key]; got != "12.00 %" {
		t.Errorf("BAT0 max = %q; want its own reading", got)
	}
	if got := charged.MaxValues[shared.BatteryCharge.Key]; got != "97.00 %" {
		t.Errorf("BAT1 max = %q; want its own reading", got)
	}
}

// diskSize and diskTemp build drive readings with the units the collector uses.
func diskSize(gb float64) shared.HardwareValue {
	return shared.NewHardwareValue(gb, shared.Gigabyte, shared.Memory)
}

func diskTemp(celsius float64) shared.HardwareValue {
	return shared.NewHardwareValue(celsius, shared.Celsius, shared.Temperature)
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
