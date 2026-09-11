package cpuinfo

import (
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// numeric converts a parsed sensor value the same way GetInformations does, so
// the tests assert what the UI would ultimately display.
func numeric(t *testing.T, temps map[string]interface{}, key string) float64 {
	t.Helper()
	raw, ok := temps[key]
	if !ok {
		t.Fatalf("key %q missing from parsed sensor output (got keys %v)", key, keysOf(temps))
	}
	hv := shared.NewHardwareValue(raw, shared.Celsius, shared.Temperature)
	got, err := hv.GetNumericValue()
	if err != nil {
		t.Fatalf("key %q: cannot convert %v to numeric: %v", key, raw, err)
	}
	return got
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestParseSensorOutputIntelCoretemp(t *testing.T) {
	lines := []string{
		"coretemp-isa-0000",
		"Adapter: ISA adapter",
		"Package id 0:  +45.0°C  (high = +80.0°C, crit = +100.0°C)",
		"Core 0:        +43.0°C  (high = +80.0°C, crit = +100.0°C)",
		"Core 1:        +44.0°C  (high = +80.0°C, crit = +100.0°C)",
		"pch_cannonlake-virtual-0",
		"Adapter: Virtual device",
		"temp1:        +48.0°C",
	}

	temps := parseSensorOutput(lines)

	checks := map[string]float64{
		// coretemp's "Package id 0" IS the CPU temperature and must bind to
		// the cpu.temp key the frontend template looks up; the per-core sensors
		// get canonical cpu.coreTempN keys in the order they appear.
		"cpu.temp":      45,
		"cpu.coreTemp0": 43,
		"cpu.coreTemp1": 44,
		"cpu.pch":       48,
	}
	for key, want := range checks {
		if got := numeric(t, temps, key); got != want {
			t.Errorf("%s = %v; want %v", key, got, want)
		}
	}
}

func TestParseSensorOutputAMDK10temp(t *testing.T) {
	lines := []string{
		"k10temp-pci-00c3",
		"Adapter: PCI adapter",
		"Tctl:         +65.0°C",
		"Tccd1:        +60.0°C",
		"Tccd2:        +61.0°C",
	}

	temps := parseSensorOutput(lines)

	checks := map[string]float64{
		"cpu.temp": 65,
		"tccd0":    60,
		"tccd1":    61,
	}
	for key, want := range checks {
		if got := numeric(t, temps, key); got != want {
			t.Errorf("%s = %v; want %v", key, got, want)
		}
	}

	// The third Tccd sensor is named tccd2, not tccd3.
	if _, exists := temps["tccd3"]; exists {
		t.Errorf("unexpected tccd3 entry: %v", temps["tccd3"])
	}
}

// TestParseSensorOutputCapsTccdSensors verifies the sensor count is bounded so
// that an unusually chatty chipset cannot produce unbounded keys.
func TestParseSensorOutputCapsTccdSensors(t *testing.T) {
	var lines []string
	for i := 1; i <= 8; i++ {
		lines = append(lines, "Tccd"+string(rune('0'+i))+":        +6"+string(rune('0'+i%10))+"0.0°C")
	}

	temps := parseSensorOutput(lines)

	for i := 0; i < 6; i++ {
		key := "tccd" + string(rune('0'+i))
		if _, ok := temps[key]; !ok {
			t.Errorf("expected %s to be present", key)
		}
	}
	if _, ok := temps["tccd6"]; ok {
		t.Errorf("expected sensors beyond the sixth to be ignored")
	}
}

// TestParseSensorOutputMalformedDoesNotPanic is a regression test: lines without
// a colon, a trailing "pch" line and a bare "CPU" marker must be skipped rather
// than causing an index-out-of-range panic.
func TestParseSensorOutputMalformedDoesNotPanic(t *testing.T) {
	lines := []string{
		"CPU",
		"Core without any colon",
		"pch",
		"",
		"Tccd",
	}

	temps := parseSensorOutput(lines)

	if len(temps) != 0 {
		t.Errorf("expected no values from malformed input, got %v", temps)
	}
}

func TestParseCpuInfo(t *testing.T) {
	lines := []string{
		"processor\t: 0",
		"vendor_id\t: GenuineIntel",
		"model name\t: Intel(R) Core(TM) i7-4770 CPU @ 3.40GHz",
		"cpu MHz\t\t: 3392.148",
		"processor\t: 1",
		"cpu MHz\t\t: 3400.000",
	}

	name, cpus := parseCpuInfo(lines)

	if want := "Intel(R) Core(TM) i7-4770 CPU"; name != want {
		t.Errorf("name = %q; want %q", name, want)
	}
	if got := cpus[cpuVendorIDKey]; got != "GenuineIntel" {
		t.Errorf("vendorID = %v; want GenuineIntel", got)
	}
	if got := cpus["cpu.clock0"]; got != 3392.148 {
		t.Errorf("cpu.clock0 = %v; want 3392.148", got)
	}
	if got := cpus["cpu.clock1"]; got != 3400.0 {
		t.Errorf("cpu.clock1 = %v; want 3400.0", got)
	}
}

// TestParseCpuInfoMapsThreadsToPhysicalCores covers the SMT topology keys the UI
// uses to show each core's temperature beside every thread: on this shape, two
// threads share each of four physical cores, so thread 1 belongs to core 0 (not
// core 1).
func TestParseCpuInfoMapsThreadsToPhysicalCores(t *testing.T) {
	lines := []string{
		"processor\t: 0",
		"model name\t: Intel(R) Core(TM) i7-4710HQ CPU @ 2.50GHz",
		"cpu MHz\t\t: 2494.000",
		"physical id\t: 0",
		"core id\t\t: 0",
		"processor\t: 1",
		"cpu MHz\t\t: 2494.100",
		"physical id\t: 0",
		"core id\t\t: 0",
		"processor\t: 2",
		"cpu MHz\t\t: 2494.200",
		"physical id\t: 0",
		"core id\t\t: 1",
		"processor\t: 3",
		"cpu MHz\t\t: 2494.300",
		"physical id\t: 0",
		"core id\t\t: 1",
	}

	_, cpus := parseCpuInfo(lines)

	// "core id" trails "cpu MHz" in each block, so this only works if blocks are
	// buffered rather than parsed as a single pass.
	wantCore := map[string]string{
		"cpu.coreOf0": "0",
		"cpu.coreOf1": "0",
		"cpu.coreOf2": "1",
		"cpu.coreOf3": "1",
	}
	for key, expected := range wantCore {
		if got := cpus[key]; got != expected {
			t.Errorf("%s = %#v; want %q", key, got, expected)
		}
	}

	if got := cpus["cpu.clock1"]; got != 2494.100 {
		t.Errorf("cpu.clock1 = %#v; want 2494.1", got)
	}
	if _, exists := cpus["cpu.coreOf4"]; exists {
		t.Errorf("unexpected core mapping beyond the parsed processors")
	}
}

// TestParseCpuInfoWithoutCoreIDOmitsTopology verifies the topology keys are
// simply absent when /proc/cpuinfo does not report "core id" (e.g. ARM), rather
// than defaulting to a guess.
func TestParseCpuInfoWithoutCoreIDOmitsTopology(t *testing.T) {
	lines := []string{
		"processor\t: 0",
		"cpu MHz\t\t: 1200.000",
		"processor\t: 1",
		"cpu MHz\t\t: 1200.000",
	}

	_, cpus := parseCpuInfo(lines)

	if _, exists := cpus["cpu.coreOf0"]; exists {
		t.Errorf("expected no core mapping when core id is unavailable")
	}
	if got := cpus["cpu.clock1"]; got != 1200.0 {
		t.Errorf("cpu.clock1 = %#v; want 1200", got)
	}
}

// TestParseCpuInfoUnparsableClockKeepsString covers the fallback path used when
// /proc/cpuinfo reports a non-numeric clock.
func TestParseCpuInfoUnparsableClockKeepsString(t *testing.T) {
	lines := []string{
		"model name\t: Test CPU",
		"cpu MHz\t\t: N/A",
	}

	_, cpus := parseCpuInfo(lines)

	got, ok := cpus["cpu.clock0"]
	if !ok {
		t.Fatalf("expected cpu.clock0 to be present, got %v", cpus)
	}
	if got != "N/A MHz" {
		t.Errorf("cpu.clock0 = %v; want %q", got, "N/A MHz")
	}
}

// TestParseCpuInfoMalformedDoesNotPanic is a regression test for the bounds
// guard: lines lacking a colon must be ignored.
func TestParseCpuInfoMalformedDoesNotPanic(t *testing.T) {
	lines := []string{
		"model name",
		"vendor_id",
		"cpu MHz",
		"",
	}

	name, cpus := parseCpuInfo(lines)

	if name != "" {
		t.Errorf("name = %q; want empty", name)
	}
	if len(cpus) != 3 {
		// Only the vendor/brand/model placeholders are populated.
		t.Errorf("expected only the 3 metadata keys, got %v", cpus)
	}
}

func TestFirstFieldAfterColon(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Core 0:  +43.0°C", "+43.0°C"},
		{"Tctl: +65.0°C", "+65.0°C"},
		{"no colon here", ""},
		{"trailing:", ""},
		{"a:b:c", "b:c"},
	}

	for _, tc := range cases {
		if got := firstFieldAfterColon(tc.in); got != tc.want {
			t.Errorf("firstFieldAfterColon(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestLabelBeforeColon(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Core 0:  +43.0°C", "Core 0"},
		{"Package id 0: +45.0°C", "Package id 0"},
		{"no colon here", ""},
	}

	for _, tc := range cases {
		if got := labelBeforeColon(tc.in); got != tc.want {
			t.Errorf("labelBeforeColon(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseCelsius(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   float64
		wantOK bool
	}{
		{"padded plus", "  +48.0°C", 48, true},
		{"with thresholds", "+48.0°C  (high = +84.0°C, crit = +100.0°C)", 48, true},
		{"plain", "43.5°C", 43.5, true},
		{"negative", "-5.0°C", -5, true},
		{"integer", "+44°C", 44, true},
		{"no number", "N/A", 0, false},
		{"empty", "", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseCelsius(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parseCelsius(%q) ok = %v; want %v", tc.in, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("parseCelsius(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestSetTemperaturePrefersFloat documents that readings are stored as numbers
// (so the UI formats them) and that unparsable text is preserved rather than
// dropped.
func TestSetTemperaturePrefersFloat(t *testing.T) {
	temps := make(map[string]interface{})
	setTemperature(temps, "cpu.temp", "  +51.0°C  (high = +84.0°C)")

	value, ok := temps["cpu.temp"].(float64)
	if !ok {
		t.Fatalf("expected a float64, got %#v", temps["cpu.temp"])
	}
	if value != 51 {
		t.Errorf("cpu.temp = %v; want 51", value)
	}

	setTemperature(temps, "cpu.pch", "N/A")
	if got := temps["cpu.pch"]; got != "N/A" {
		t.Errorf("cpu.pch = %#v; want the raw text preserved", got)
	}

	setTemperature(temps, "tccd0", "")
	if _, exists := temps["tccd0"]; exists {
		t.Errorf("expected an empty reading to add no entry")
	}
}

// TestParseSensorOutputIntelPackageBindsCPUTemp is the regression test for the
// missing CPU temperature: an Intel coretemp block must yield cpu.temp.
func TestParseSensorOutputIntelPackageBindsCPUTemp(t *testing.T) {
	lines := []string{
		"coretemp-isa-0000",
		"Adapter: ISA adapter",
		"Package id 0:  +48.0°C  (high = +84.0°C, crit = +100.0°C)",
		"Core 0:        +47.0°C  (high = +84.0°C, crit = +100.0°C)",
	}

	temps := parseSensorOutput(lines)

	if _, ok := temps["cpu.temp"]; !ok {
		t.Fatalf("expected cpu.temp to be set from the Package row, got keys %v", keysOf(temps))
	}
	if got := numeric(t, temps, "cpu.temp"); got != 48 {
		t.Errorf("cpu.temp = %v; want 48", got)
	}

	// The package reading must not also be stored under a label-derived key.
	if _, exists := temps["cpu.Package id 0"]; exists {
		t.Errorf("package row leaked into a label-derived key")
	}
}

// TestParseSensorOutputCoreTemperaturesAreNumbered verifies the per-core
// sensors get sequential canonical keys regardless of the sensor label text.
func TestParseSensorOutputCoreTemperaturesAreNumbered(t *testing.T) {
	lines := []string{
		"coretemp-isa-0000",
		"Adapter: ISA adapter",
		"Package id 0:  +48.0°C",
		"Core 0:        +47.0°C",
		"Core 1:        +49.0°C",
		"Core 2:        +46.0°C",
		"Core 3:        +45.0°C",
	}

	temps := parseSensorOutput(lines)

	want := map[string]float64{
		"cpu.coreTemp0": 47,
		"cpu.coreTemp1": 49,
		"cpu.coreTemp2": 46,
		"cpu.coreTemp3": 45,
	}
	for key, expected := range want {
		if got := numeric(t, temps, key); got != expected {
			t.Errorf("%s = %v; want %v", key, got, expected)
		}
	}

	// The package row must not be counted as a core.
	if _, exists := temps["cpu.coreTemp4"]; exists {
		t.Errorf("package row was counted as a core sensor")
	}
	if got := numeric(t, temps, "cpu.temp"); got != 48 {
		t.Errorf("cpu.temp = %v; want 48", got)
	}
}
