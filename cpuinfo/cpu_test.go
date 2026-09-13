package cpuinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// numeric converts a parsed sensor value the same way GetInformations does, so
// the tests assert what the UI would ultimately display.
func numeric(t *testing.T, temps map[string]interface{}, key string) float64 {
	t.Helper()
	raw, ok := temps[key]
	if !ok {
		t.Fatalf("key %q missing from parsed temperatures (got keys %v)", key, keysOf(temps))
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

// writeHWMonFixture creates the given files under a temporary sysfs root and
// points the collector at it for the duration of the test.
func writeHWMonFixture(t *testing.T, files map[string]string) {
	t.Helper()

	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", full, err)
		}
	}

	original := sysDir
	sysDir = root
	t.Cleanup(func() { sysDir = original })
}

func TestCPUIntelCoretempTemperatures(t *testing.T) {
	writeHWMonFixture(t, map[string]string{
		"class/hwmon/hwmon0/name":        "coretemp",
		"class/hwmon/hwmon0/temp1_label": "Package id 0",
		"class/hwmon/hwmon0/temp1_input": "45000",
		"class/hwmon/hwmon0/temp2_label": "Core 0",
		"class/hwmon/hwmon0/temp2_input": "43000",
		"class/hwmon/hwmon0/temp3_label": "Core 1",
		"class/hwmon/hwmon0/temp3_input": "44000",

		// Other components' chips expose temperatures too, but they belong to
		// their own cards and must not leak onto the CPU card.
		"class/hwmon/hwmon1/name":        "amdgpu",
		"class/hwmon/hwmon1/temp1_label": "edge",
		"class/hwmon/hwmon1/temp1_input": "61000",
		"class/hwmon/hwmon2/name":        "nvme",
		"class/hwmon/hwmon2/temp1_input": "38000",
	})

	temps := cpuTemperatures()

	checks := map[string]float64{
		// coretemp's "Package id 0" IS the CPU temperature and must bind to the
		// cpu.temp key the frontend template looks up; the per-core sensors get
		// canonical cpu.coreTempN keys in sensor-number order.
		"cpu.temp":      45,
		"cpu.coreTemp0": 43,
		"cpu.coreTemp1": 44,
	}
	for key, want := range checks {
		if got := numeric(t, temps, key); got != want {
			t.Errorf("%s = %v; want %v", key, got, want)
		}
	}

	if _, exists := temps["cpu.pch"]; exists {
		t.Errorf("unexpected cpu.pch entry on a machine with no PCH chip: %v", temps["cpu.pch"])
	}
	for key := range temps {
		if key != "cpu.temp" && key != "cpu.coreTemp0" && key != "cpu.coreTemp1" {
			t.Errorf("unexpected key %q in %v", key, temps)
		}
	}
}

func TestCPUAMDK10tempTemperatures(t *testing.T) {
	writeHWMonFixture(t, map[string]string{
		"class/hwmon/hwmon0/name":        "k10temp",
		"class/hwmon/hwmon0/temp1_label": "Tctl",
		"class/hwmon/hwmon0/temp1_input": "65000",
		"class/hwmon/hwmon0/temp3_label": "Tccd1",
		"class/hwmon/hwmon0/temp3_input": "60000",
	})

	temps := cpuTemperatures()

	checks := map[string]float64{
		"cpu.temp": 65,
		"tccd0":    60,
	}
	for key, want := range checks {
		if got := numeric(t, temps, key); got != want {
			t.Errorf("%s = %v; want %v", key, got, want)
		}
	}

	// The CCD rows are numbered in sensor order, not after their label's number:
	// the first Tccd sensor is tccd0, so tccd1 and tccd3 must be absent.
	for _, absent := range []string{"tccd1", "tccd3"} {
		if _, exists := temps[absent]; exists {
			t.Errorf("unexpected %s entry: %v", absent, temps[absent])
		}
	}
}

// TestCPUTccdSensorsAreCapped verifies the sensor count is bounded so that an
// unusually chatty chip cannot produce unbounded keys.
func TestCPUTccdSensorsAreCapped(t *testing.T) {
	files := map[string]string{"class/hwmon/hwmon0/name": "k10temp"}
	for i := 1; i <= 8; i++ {
		files[tempLabelPath(0, i)] = "Tccd" + itoa(i)
		files[tempInputPath(0, i)] = "60000"
	}
	writeHWMonFixture(t, files)

	temps := cpuTemperatures()

	for i := 0; i < maxTccdSensors; i++ {
		if _, ok := temps["tccd"+itoa(i)]; !ok {
			t.Errorf("expected tccd%d to be present, got keys %v", i, keysOf(temps))
		}
	}
	if _, ok := temps["tccd"+itoa(maxTccdSensors)]; ok {
		t.Errorf("expected CCD sensors beyond the cap to be ignored")
	}
}

func TestCPUPCHTemperature(t *testing.T) {
	writeHWMonFixture(t, map[string]string{
		"class/hwmon/hwmon0/name":        "pch_cannonlake",
		"class/hwmon/hwmon0/temp1_input": "48000",
		"class/hwmon/hwmon1/name":        "coretemp",
		"class/hwmon/hwmon1/temp1_label": "Package id 0",
		"class/hwmon/hwmon1/temp1_input": "47000",
	})

	temps := cpuTemperatures()

	if got := numeric(t, temps, "cpu.pch"); got != 48 {
		t.Errorf("cpu.pch = %v; want 48", got)
	}
	if got := numeric(t, temps, "cpu.temp"); got != 47 {
		t.Errorf("cpu.temp = %v; want 47", got)
	}
}

// TestCPUTemperaturesFollowSensorNumberOrder pins the ordering: glob returns
// temp10 before temp2, so without the numeric sort the per-core ordinals would
// silently swap on a machine with more than nine sensors.
func TestCPUTemperaturesFollowSensorNumberOrder(t *testing.T) {
	writeHWMonFixture(t, map[string]string{
		"class/hwmon/hwmon0/name":         "coretemp",
		"class/hwmon/hwmon0/temp2_label":  "Core 1",
		"class/hwmon/hwmon0/temp2_input":  "44100",
		"class/hwmon/hwmon0/temp10_label": "Core 2",
		"class/hwmon/hwmon0/temp10_input": "45100",
	})

	temps := cpuTemperatures()

	if got := numeric(t, temps, "cpu.coreTemp0"); got != 44.1 {
		t.Errorf("cpu.coreTemp0 = %v; want 44.1 (temp2 must be read before temp10)", got)
	}
	if got := numeric(t, temps, "cpu.coreTemp1"); got != 45.1 {
		t.Errorf("cpu.coreTemp1 = %v; want 45.1", got)
	}
}

// TestCPUUnlabelledFirstSensorBindsCPUTemp covers the drivers that report no
// labels at all (older k10temp, ARM's cpu_thermal): their first sensor is the
// package reading.
func TestCPUUnlabelledFirstSensorBindsCPUTemp(t *testing.T) {
	writeHWMonFixture(t, map[string]string{
		"class/hwmon/hwmon0/name":        "k10temp",
		"class/hwmon/hwmon0/temp1_input": "55000",
	})

	temps := cpuTemperatures()

	if got := numeric(t, temps, "cpu.temp"); got != 55 {
		t.Errorf("cpu.temp = %v; want 55", got)
	}
}

// TestCPUUnreadableSensorsAreSkipped verifies a sensor file that is not a number
// produces no row rather than a bogus zero reading.
func TestCPUUnreadableSensorsAreSkipped(t *testing.T) {
	writeHWMonFixture(t, map[string]string{
		"class/hwmon/hwmon0/name":        "coretemp",
		"class/hwmon/hwmon0/temp1_label": "Package id 0",
		"class/hwmon/hwmon0/temp1_input": "not a number",
		"class/hwmon/hwmon0/temp2_label": "Core 0",
		"class/hwmon/hwmon0/temp2_input": "",
	})

	temps := cpuTemperatures()

	if len(temps) != 0 {
		t.Errorf("expected unreadable sensors to produce no rows, got %v", temps)
	}
}

func TestCPUNoHwmonChips(t *testing.T) {
	writeHWMonFixture(t, map[string]string{
		"class/hwmon/hwmon0/name":        "nct6798",
		"class/hwmon/hwmon0/temp2_label": "CPUTIN",
		"class/hwmon/hwmon0/temp2_input": "44000",
	})

	if temps := cpuTemperatures(); len(temps) != 0 {
		t.Errorf("expected a board chip's CPUTIN label not to become the CPU temperature, got %v", temps)
	}
}

// TestGetInformationsKeepsIdentityWithoutCPUHwmonChip is the regression test for
// the behaviour this collector used to have: when lm-sensors was not installed,
// the ENTIRE CPU card disappeared. Identity and clocks come from /proc/cpuinfo
// and must survive a machine that exposes no CPU temperature sensor.
func TestGetInformationsKeepsIdentityWithoutCPUHwmonChip(t *testing.T) {
	writeHWMonFixture(t, map[string]string{
		"class/hwmon/hwmon0/name":        "amdgpu",
		"class/hwmon/hwmon0/temp1_input": "61000",
	})

	data := GetInformations()

	for _, key := range []string{cpuVendorIDKey, cpuBrandKey, cpuModelKey} {
		if _, ok := data.Values[key]; !ok {
			t.Errorf("expected %s to be reported without any CPU hwmon chip, got keys %v", key, keysOfHardware(data.Values))
		}
	}
	if _, ok := data.Values["cpu.temp"]; ok {
		t.Errorf("unexpected cpu.temp without a CPU hwmon chip")
	}
}

func keysOfHardware(m map[string]shared.HardwareValue) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestMilliCelsius(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   float64
		wantOK bool
	}{
		{"positive", "45000", 45, true},
		{"fractional", "48850", 48.85, true},
		{"negative", "-5000", -5, true},
		{"zero", "0", 0, true},
		{"padded", " 43000\n", 43, true},
		{"not a number", "N/A", 0, false},
		{"empty", "", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := milliCelsius(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("milliCelsius(%q) ok = %v; want %v", tc.in, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("milliCelsius(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestTemperatureSensorIndex(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"temp1_input", 1, true},
		{"temp10_input", 10, true},
		{"temp_input", 0, false},
		{"tempN_input", 0, false},
		{"fan1_input", 0, false},
		{"temp1_label", 0, false},
	}

	for _, tc := range cases {
		got, ok := temperatureSensorIndex(tc.in)
		if ok != tc.wantOK {
			t.Fatalf("temperatureSensorIndex(%q) ok = %v; want %v", tc.in, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Errorf("temperatureSensorIndex(%q) = %d; want %d", tc.in, got, tc.want)
		}
	}
}

// helpers used to build fixture paths and names
func itoa(i int) string { return strconv.Itoa(i) }

func tempInputPath(chip, index int) string {
	return "class/hwmon/hwmon" + itoa(chip) + "/temp" + itoa(index) + "_input"
}

func tempLabelPath(chip, index int) string {
	return "class/hwmon/hwmon" + itoa(chip) + "/temp" + itoa(index) + "_label"
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
