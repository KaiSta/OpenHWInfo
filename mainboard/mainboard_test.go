package mainboard

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// writeSysFixture creates the given files under a temporary sysfs root and
// points the collector at it for the duration of the test.
func writeSysFixture(t *testing.T, files map[string]string) {
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

// boardFixture describes a machine with the three sensor sources at once: two
// board-level ACPI zones (plus two component zones that must be ignored), a
// Super I/O chip, and a CPU sensor chip that must not be read as board data.
func boardFixture() map[string]string {
	files := map[string]string{
		"class/dmi/id/board_vendor":  "ASUSTeK COMPUTER INC.",
		"class/dmi/id/board_name":    "PRIME B450M-A",
		"class/dmi/id/board_version": "Rev X.0x",
		"class/dmi/id/bios_version":  "2409",
		"class/dmi/id/bios_date":     "07/10/2020",

		// Zone 2 must be reported before zone 10 numerically; glob alone would
		// order "thermal_zone10" first.
		"class/thermal/thermal_zone2/type":  "pch_skylake",
		"class/thermal/thermal_zone2/temp":  "52000",
		"class/thermal/thermal_zone10/type": "acpitz",
		"class/thermal/thermal_zone10/temp": "47000",
		// Component zones: the CPU package and the wireless card.
		"class/thermal/thermal_zone1/type": "x86_pkg_temp",
		"class/thermal/thermal_zone1/temp": "48000",
		"class/thermal/thermal_zone3/type": "iwlwifi_1",
		"class/thermal/thermal_zone3/temp": "41000",

		// The Super I/O chip on the board.
		"class/hwmon/hwmon0/name":        "nct6798",
		"class/hwmon/hwmon0/temp1_label": "SYSTIN",
		"class/hwmon/hwmon0/temp1_input": "41000",
		"class/hwmon/hwmon0/temp2_label": "CPUTIN",
		"class/hwmon/hwmon0/temp2_input": "44000",
		"class/hwmon/hwmon0/temp3_input": "39000", // no label: falls back to the chip name
		"class/hwmon/hwmon0/temp4_input": "not a number",
		"class/hwmon/hwmon0/fan1_label":  "Chassis Fan",
		"class/hwmon/hwmon0/fan1_input":  "1200",
		"class/hwmon/hwmon0/fan2_input":  "900",
		"class/hwmon/hwmon0/fan3_input":  "n/a",
		"class/hwmon/hwmon0/fan4_input":  "",

		// The CPU's own sensors, which belong to the CPU card.
		"class/hwmon/hwmon1/name":        "coretemp",
		"class/hwmon/hwmon1/temp1_input": "45000",
		// A GPU's sensor chip, which belongs to the GPU card.
		"class/hwmon/hwmon2/name":        "amdgpu",
		"class/hwmon/hwmon2/temp1_input": "61000",
	}
	return files
}

func TestGetInformationsReadsDMIIdentity(t *testing.T) {
	writeSysFixture(t, boardFixture())

	data := GetInformations()

	if data.Name != "PRIME B450M-A" {
		t.Errorf("Name = %q; want %q", data.Name, "PRIME B450M-A")
	}

	want := map[string]string{
		shared.BoardVendor.Key:      "ASUSTeK COMPUTER INC.",
		shared.BoardModel.Key:       "PRIME B450M-A",
		shared.BoardVersion.Key:     "Rev X.0x",
		shared.BoardBIOSVersion.Key: "2409",
		shared.BoardBIOSDate.Key:    "07/10/2020",
	}
	for key, expected := range want {
		value, ok := data.Values[key]
		if !ok {
			t.Errorf("expected %q to be collected", key)
			continue
		}
		if value.Value != expected {
			t.Errorf("%s = %v; want %q", key, value.Value, expected)
		}
	}
}

// TestGetInformationsSkipsMissingDMIFields pins the difference between a field
// the firmware leaves blank (or that is root-only, like board_serial) and one
// that carries a value: an unknown field must be absent rather than empty.
func TestGetInformationsSkipsMissingDMIFields(t *testing.T) {
	writeSysFixture(t, map[string]string{
		"class/dmi/id/board_vendor": "LENOVO",
		"class/dmi/id/board_name":   "Lenovo Y50-70",
		"class/dmi/id/board_serial": "",
	})

	data := GetInformations()

	if _, ok := data.Values[shared.BoardSerial.Key]; ok {
		t.Error("an empty board_serial must not produce a row")
	}
	if _, ok := data.Values[shared.BoardModel.Key]; !ok {
		t.Error("expected board_name to be collected")
	}
}

func TestGetInformationsWithoutSMIBIOS(t *testing.T) {
	writeSysFixture(t, map[string]string{})

	data := GetInformations()

	if data.Name != "Mainboard" {
		t.Errorf("Name = %q; want the %q fallback", data.Name, "Mainboard")
	}
	if len(data.Values) != 0 {
		t.Errorf("expected no values on a system without SMBIOS, got %v", data.Values)
	}
}

// TestAddThermalZonesSelectsBoardSensors covers both halves of the zone rule:
// the board's own zones are reported, and the CPU/wireless zones are not
// attributed to the board.
func TestAddThermalZonesSelectsBoardSensors(t *testing.T) {
	writeSysFixture(t, boardFixture())

	data := GetInformations()

	// Numeric ordering: zone 2 (pch) before zone 10 (acpitz).
	assertTemperature(t, data, "board.temp0", 52, "pch_skylake Temperature")
	assertTemperature(t, data, "board.temp1", 47, "acpitz Temperature")

	for key, value := range data.Values {
		if !strings.Contains(key, "Label") {
			continue
		}
		label := fmt.Sprint(value.Value)
		if strings.Contains(label, "x86_pkg_temp") || strings.Contains(label, "iwlwifi") {
			t.Errorf("component zone %q was reported as board data", label)
		}
	}
}

// TestAddBoardMonitorChipsReadsSensorsAndFans checks the Super I/O path: sensor
// labels are used when the driver provides them, the chip name is the fallback,
// and only the monitoring chip is read.
func TestAddBoardMonitorChipsReadsSensorsAndFans(t *testing.T) {
	writeSysFixture(t, boardFixture())

	data := GetInformations()

	assertTemperature(t, data, "board.temp2", 41, "SYSTIN Temperature")
	assertTemperature(t, data, "board.temp3", 44, "CPUTIN Temperature")
	assertTemperature(t, data, "board.temp4", 39, "nct6798 Temperature 3")

	// temp4_input is unparsable and the CPU/GPU chips are not board sensors, so
	// no sixth temperature row may exist (45 C and 61 C must not appear).
	if value, ok := data.Values["board.temp5"]; ok {
		t.Errorf("board.temp5 = %v; no further sensor may be reported here", value.Value)
	}

	assertFan(t, data, "board.fan0", 1200, "Chassis Fan Speed")
	assertFan(t, data, "board.fan1", 900, "Fan 2 Speed")

	if _, ok := data.Values["board.fan2"]; ok {
		t.Error("a fan that does not report a number must not become a row")
	}
}

// TestFanIsEligibleForMinMaxTracking documents why fans carry the Frequency
// value type: it is what makes the min/max history track them.
func TestFanIsEligibleForMinMaxTracking(t *testing.T) {
	writeSysFixture(t, boardFixture())

	data := GetInformations()

	fan, ok := data.Values["board.fan0"]
	if !ok {
		t.Fatal("expected a fan to be collected")
	}
	if !fan.IsNumericType() {
		t.Error("a fan speed must be a numeric type so its min/max are tracked")
	}
	if fan.Unit != shared.RPM {
		t.Errorf("fan unit = %v; want RPM", fan.Unit)
	}
}

func TestTemperatureLabelKeysAreCollectedForEveryRow(t *testing.T) {
	writeSysFixture(t, boardFixture())

	data := GetInformations()

	for index := 0; index < 5; index++ {
		if _, ok := data.Values["board.temp"+strconv.Itoa(index)]; !ok {
			t.Fatalf("expected board.temp%d to be collected", index)
		}
		if _, ok := data.Values["board.tempLabel"+strconv.Itoa(index)]; !ok {
			t.Errorf("board.temp%d has no label; main.go would fall back to a generic name", index)
		}
	}
	for index := 0; index < 2; index++ {
		if _, ok := data.Values["board.fanLabel"+strconv.Itoa(index)]; !ok {
			t.Errorf("board.fan%d has no label", index)
		}
	}
}

func TestFanCountIsCapped(t *testing.T) {
	files := map[string]string{"class/hwmon/hwmon0/name": "it8688"}
	for i := 1; i <= maxBoardFans+2; i++ {
		files["class/hwmon/hwmon0/fan"+strconv.Itoa(i)+"_input"] = "1000"
	}
	writeSysFixture(t, files)

	data := GetInformations()

	count := 0
	for index := 0; index < maxBoardFans+2; index++ {
		if _, ok := data.Values["board.fan"+strconv.Itoa(index)]; ok {
			count++
		}
	}
	if count != maxBoardFans {
		t.Errorf("collected %d fan rows; want the cap of %d", count, maxBoardFans)
	}
}

func TestSensorIndex(t *testing.T) {
	cases := []struct {
		fileName string
		kind     string
		want     int
		ok       bool
	}{
		{"temp1_input", "temp", 1, true},
		{"temp10_input", "temp", 10, true},
		{"fan12_input", "fan", 12, true},
		{"temp_input", "temp", 0, false},
		{"tempX_input", "temp", 0, false},
		{"temp1_label", "temp", 0, false},
		{"name", "temp", 0, false},
	}
	for _, c := range cases {
		got, ok := sensorIndex(c.fileName, c.kind)
		if got != c.want || ok != c.ok {
			t.Errorf("sensorIndex(%q, %q) = %d, %v; want %d, %v", c.fileName, c.kind, got, ok, c.want, c.ok)
		}
	}
}

func TestIsBoardZone(t *testing.T) {
	cases := map[string]bool{
		"acpitz":          true,
		"pch_skylake":     true,
		"pch_cannonlake":  true,
		"TZ00":            true,
		"TZ01":            true,
		"x86_pkg_temp":    false,
		"iwlwifi_1":       false,
		"amdgpu":          false,
		"nvme":            false,
		"INT3400 Thermal": false,
	}
	for zoneType, want := range cases {
		if got := isBoardZone(zoneType); got != want {
			t.Errorf("isBoardZone(%q) = %v; want %v", zoneType, got, want)
		}
	}
}

func TestIsBoardMonitorChip(t *testing.T) {
	cases := map[string]bool{
		"nct6798":         true,
		"nct6687":         true,
		"it8688":          true,
		"f71869a":         true,
		"w83627ehf":       true,
		"asus":            true,
		"gigabyte":        true,
		"coretemp":        false,
		"k10temp":         false,
		"zenpower":        false,
		"amdgpu":          false,
		"nouveau":         false,
		"nvme":            false,
		"iwlwifi_1":       false,
		"acpitz":          false,
		"BAT1":            false,
		"thinkpad":        false,
		"dell_smm":        false,
		"intel_rapl":      false,
		"hidpp_battery_0": false,
	}
	for name, want := range cases {
		if got := isBoardMonitorChip(name); got != want {
			t.Errorf("isBoardMonitorChip(%q) = %v; want %v", name, got, want)
		}
	}
}

func TestMilliCelsius(t *testing.T) {
	cases := []struct {
		raw  string
		want float64
		ok   bool
	}{
		{"47000", 47, true},
		{" 2500 \n", 2.5, true},
		{"-5000", -5, true},
		{"0", 0, true},
		{"n/a", 0, false},
		{"", 0, false},
		{"47.5", 0, false},
	}
	for _, c := range cases {
		got, ok := milliCelsius(c.raw)
		if ok != c.ok || (ok && math.Abs(got-c.want) > 1e-9) {
			t.Errorf("milliCelsius(%q) = %v, %v; want %v, %v", c.raw, got, ok, c.want, c.ok)
		}
	}
}

func TestTrailingNumberOrdersZonesNumerically(t *testing.T) {
	paths := []string{"x/thermal_zone10", "x/thermal_zone2", "x/thermal_zone1"}
	sortByTrailingNumber(paths)

	want := []string{"x/thermal_zone1", "x/thermal_zone2", "x/thermal_zone10"}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("sorted to %v; want %v", paths, want)
		}
	}
}

func assertTemperature(t *testing.T, data shared.HardwareData, key string, wantCelsius float64, wantLabel string) {
	t.Helper()

	value, ok := data.Values[key]
	if !ok {
		t.Errorf("expected %s to be collected", key)
		return
	}
	gotCelsius, ok := value.Value.(float64)
	if !ok {
		t.Errorf("%s = %v (%T); want a float64 reading", key, value.Value, value.Value)
		return
	}
	if math.Abs(gotCelsius-wantCelsius) > 1e-9 {
		t.Errorf("%s = %v; want %v", key, gotCelsius, wantCelsius)
	}
	if value.Type != shared.Temperature || value.Unit != shared.Celsius {
		t.Errorf("%s type/unit = %v/%v; want Temperature/Celsius", key, value.Type, value.Unit)
	}

	label, ok := data.Values[labelKeyFor(key)]
	if !ok {
		t.Errorf("%s has no label key", key)
		return
	}
	if label.Value != wantLabel {
		t.Errorf("%s label = %v; want %q", key, label.Value, wantLabel)
	}
}

func assertFan(t *testing.T, data shared.HardwareData, key string, wantRPM float64, wantLabel string) {
	t.Helper()

	value, ok := data.Values[key]
	if !ok {
		t.Errorf("expected %s to be collected", key)
		return
	}
	if gotRPM, ok := value.Value.(float64); !ok || math.Abs(gotRPM-wantRPM) > 1e-9 {
		t.Errorf("%s = %v; want %v RPM", key, value.Value, wantRPM)
	}

	label, ok := data.Values[labelKeyFor(key)]
	if !ok {
		t.Errorf("%s has no label key", key)
		return
	}
	if label.Value != wantLabel {
		t.Errorf("%s label = %v; want %q", key, label.Value, wantLabel)
	}
}

// labelKeyFor turns "board.temp3" into "board.tempLabel3".
func labelKeyFor(key string) string {
	index := strings.LastIndexFunc(key, func(r rune) bool { return r < '0' || r > '9' })
	if index < 0 {
		return key
	}
	return key[:index+1] + "Label" + key[index+1:]
}
