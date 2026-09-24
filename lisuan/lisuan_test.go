package lisuan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// writeFixture builds a sysfs tree and points the collector at it. The tree is a
// map of relative path to content, so a test states exactly which files the
// driver "publishes" and nothing else can leak in from the review host.
func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}

// useSysDir points the package at a fixture tree for one test. The probe cache is
// dropped afterwards because a card's name and driver version are memoized for the
// process, and the next test must not inherit this tree's answer.
func useSysDir(t *testing.T, dir string) {
	t.Helper()

	previous := sysDir
	sysDir = dir
	t.Cleanup(func() {
		sysDir = previous
		shared.ResetProbeCache()
	})
}

// cardFixture is a LISUAN 7G100 that publishes every file the collector reads.
func cardFixture() map[string]string {
	return map[string]string{
		"class/drm/card0/device/vendor":                      "0x4c54\n",
		"class/drm/card0/device/device":                      "0x5000\n",
		"class/drm/card0/device/hwmon/hwmon0/temp1_input":    "45000\n",
		"class/drm/card0/device/hwmon/hwmon0/temp1_label":    "GPU\n",
		"class/drm/card0/device/hwmon/hwmon0/freq1_input":    "1800000000\n",
		"class/drm/card0/device/hwmon/hwmon0/power1_average": "35000000\n",
		"class/drm/card0/device/hwmon/hwmon0/power1_cap_max": "110000000\n",
		"class/drm/card0/device/hwmon/hwmon0/fan1_input":     "1500\n",
	}
}

func numeric(t *testing.T, card shared.HardwareData, key string) float64 {
	t.Helper()

	value, ok := card.Values[key]
	if !ok {
		t.Fatalf("no reading for %s; the card reported %v", key, card.Values)
	}
	number, err := value.GetNumericValue()
	if err != nil {
		t.Fatalf("%s is not numeric: %v", key, err)
	}
	return number
}

func assertAbsent(t *testing.T, card shared.HardwareData, keys ...string) {
	t.Helper()

	for _, key := range keys {
		if value, ok := card.Values[key]; ok {
			t.Errorf("%s was reported as %v; want no row", key, value.Value)
		}
	}
}

// TestGetInformationsReadsTheCardsSensors pins the whole published set: identity
// from the PCI device ID, and every reading with the unit that makes it a
// measurement rather than a bare number.
func TestGetInformationsReadsTheCardsSensors(t *testing.T) {
	useSysDir(t, writeFixture(t, cardFixture()))

	cards := GetAllInformations()
	if len(cards) != 1 {
		t.Fatalf("got %d card(s); want one", len(cards))
	}
	card := cards[0]

	if got := card.Values[shared.GPUName.Key].Value; got != "LISUAN 7G100 Series Graphics" {
		t.Errorf("name = %v; want the PCI database's name for device 5000", got)
	}
	if got := numeric(t, card, shared.GPUTemp.Key); got != 45 {
		t.Errorf("temperature = %v; want 45", got)
	}
	if got := card.Values[shared.GPUTemp.Key].Unit.Suffix(); got != "°C" {
		t.Errorf("temperature unit = %q; want the reading to say what it measures", got)
	}
	if got := numeric(t, card, shared.GPUFrequency.Key); got != 1800 {
		t.Errorf("clock = %v MHz; want 1800 (sysfs reports Hz)", got)
	}
	if got := numeric(t, card, shared.GPUPowerAvg.Key); got != 35 {
		t.Errorf("power = %v W; want 35 (sysfs reports microwatts)", got)
	}
	if got := numeric(t, card, shared.GPUPowerMax.Key); got != 110 {
		t.Errorf("power cap = %v W; want 110", got)
	}
	if got := numeric(t, card, shared.FanSpeed.Key); got != 1500 {
		t.Errorf("fan = %v RPM; want 1500", got)
	}
	if got := card.Values[shared.FanSpeed.Key].Unit.Suffix(); got != "RPM" {
		t.Errorf("fan unit = %q; want RPM", got)
	}
}

// TestGetInformationsOmitsSensorsTheDriverDoesNotPublish is the other half of the
// contract: a driver that publishes one temperature and nothing else gets one row,
// not a table of zeroes.
func TestGetInformationsOmitsSensorsTheDriverDoesNotPublish(t *testing.T) {
	useSysDir(t, writeFixture(t, map[string]string{
		"class/drm/card0/device/vendor":                   "0x4c54\n",
		"class/drm/card0/device/device":                   "0x5000\n",
		"class/drm/card0/device/hwmon/hwmon0/temp1_input": "39000\n",
	}))

	cards := GetAllInformations()
	if len(cards) != 1 {
		t.Fatalf("got %d card(s); want the one card that reported a temperature", len(cards))
	}

	assertAbsent(t, cards[0],
		shared.GPUHotspotTemp.Key,
		shared.VRAMTemp.Key,
		shared.GPUFrequency.Key,
		shared.GPUPowerAvg.Key,
		shared.GPUPowerMax.Key,
		shared.FanSpeed.Key,
	)
	if got := numeric(t, cards[0], shared.GPUTemp.Key); got != 39 {
		t.Errorf("temperature = %v; want 39", got)
	}
}

// TestHotspotAndMemoryTemperaturesComeFromTheDriverLabels pins that the mapping
// follows the driver's own words rather than the sensor number: a driver that
// lists memory first and hotspot second must not have them swapped into each
// other's rows.
func TestHotspotAndMemoryTemperaturesComeFromTheDriverLabels(t *testing.T) {
	useSysDir(t, writeFixture(t, map[string]string{
		"class/drm/card0/device/vendor":                   "0x4c54\n",
		"class/drm/card0/device/device":                   "0x5000\n",
		"class/drm/card0/device/hwmon/hwmon0/temp1_input": "60000\n",
		"class/drm/card0/device/hwmon/hwmon0/temp1_label": "Memory\n",
		"class/drm/card0/device/hwmon/hwmon0/temp2_input": "81000\n",
		"class/drm/card0/device/hwmon/hwmon0/temp2_label": "Junction\n",
		"class/drm/card0/device/hwmon/hwmon0/temp3_input": "52000\n",
	}))

	cards := GetAllInformations()
	if len(cards) != 1 {
		t.Fatalf("got %d card(s); want one", len(cards))
	}
	card := cards[0]

	if got := numeric(t, card, shared.VRAMTemp.Key); got != 60 {
		t.Errorf("video-memory temperature = %v; want the sensor labelled Memory (60)", got)
	}
	if got := numeric(t, card, shared.GPUHotspotTemp.Key); got != 81 {
		t.Errorf("hotspot temperature = %v; want the sensor labelled Junction (81)", got)
	}
	if got := numeric(t, card, shared.GPUTemp.Key); got != 52 {
		t.Errorf("card temperature = %v; want the unlabelled sensor (52)", got)
	}
}

// TestTemperatureSensorsPastTheCapAreNotRead keeps the bound honest: the ninth
// sensor is beyond maxTemps, so even a label that would earn it a row is ignored
// and the card keeps the readings it published within the bound.
func TestTemperatureSensorsPastTheCapAreNotRead(t *testing.T) {
	files := map[string]string{
		"class/drm/card0/device/vendor":                   "0x4c54\n",
		"class/drm/card0/device/device":                   "0x5000\n",
		"class/drm/card0/device/hwmon/hwmon0/temp1_input": "40000\n",
	}
	files["class/drm/card0/device/hwmon/hwmon0/temp9_input"] = "99000\n"
	files["class/drm/card0/device/hwmon/hwmon0/temp9_label"] = "Junction\n"
	useSysDir(t, writeFixture(t, files))

	cards := GetAllInformations()
	if len(cards) != 1 {
		t.Fatalf("got %d card(s); want one", len(cards))
	}

	assertAbsent(t, cards[0], shared.GPUHotspotTemp.Key)
	if got := numeric(t, cards[0], shared.GPUTemp.Key); got != 40 {
		t.Errorf("temperature = %v; want the sensor within the cap (40)", got)
	}
}

// TestZeroFanReadingIsReported pins the deliberate difference from the AMD
// collector: the driver published a fan, so a zero is a stopped fan and a reading,
// not a missing sensor.
func TestZeroFanReadingIsReported(t *testing.T) {
	files := cardFixture()
	files["class/drm/card0/device/hwmon/hwmon0/fan1_input"] = "0\n"
	useSysDir(t, writeFixture(t, files))

	cards := GetAllInformations()
	if len(cards) != 1 {
		t.Fatalf("got %d card(s); want one", len(cards))
	}
	if got := numeric(t, cards[0], shared.FanSpeed.Key); got != 0 {
		t.Errorf("fan = %v; want the published zero to be reported", got)
	}
}

// TestCardWithoutReadingsIsSkipped: identity alone is not telemetry. A card whose
// driver publishes no sensor is left out rather than drawn as a card with a name
// and nothing under it.
func TestCardWithoutReadingsIsSkipped(t *testing.T) {
	useSysDir(t, writeFixture(t, map[string]string{
		"class/drm/card0/device/vendor": "0x4c54\n",
		"class/drm/card0/device/device": "0x5000\n",
	}))

	if cards := GetAllInformations(); len(cards) != 0 {
		t.Errorf("got %d card(s) from a card with no sensors; want none", len(cards))
	}
}

// TestOtherVendorsAreNotCollected guards the probe: the vendor file decides, not
// the card's position, so a machine with an AMD card at index 0 reports no Lisuan
// card.
func TestOtherVendorsAreNotCollected(t *testing.T) {
	useSysDir(t, writeFixture(t, map[string]string{
		"class/drm/card0/device/vendor":                   "0x1002\n",
		"class/drm/card0/device/device":                   "0x744c\n",
		"class/drm/card0/device/hwmon/hwmon0/temp1_input": "45000\n",
	}))

	if cards := GetAllInformations(); len(cards) != 0 {
		t.Errorf("got %d card(s) from an AMD machine; want none", len(cards))
	}
}

// TestUnknownDeviceIDIsNamedWithoutClaimingAModel: the vendor is known from the
// probe, so the card is named after the vendor, and never after its bus address —
// the failure the Intel collector hit when an identifier string reached the UI.
func TestUnknownDeviceIDIsNamedWithoutClaimingAModel(t *testing.T) {
	useSysDir(t, writeFixture(t, map[string]string{
		"class/drm/card0/device/vendor":                   "0x4c54\n",
		"class/drm/card0/device/device":                   "0x9999\n",
		"class/drm/card0/device/hwmon/hwmon0/temp1_input": "41000\n",
	}))

	cards := GetAllInformations()
	if len(cards) != 1 {
		t.Fatalf("got %d card(s); want one", len(cards))
	}

	name, _ := cards[0].Values[shared.GPUName.Key].Value.(string)
	if name != "Lisuan GPU" {
		t.Errorf("name = %q; want the vendor fallback", name)
	}
	for _, unwanted := range []string{":", "0x", "9999", "4c54"} {
		if strings.Contains(strings.ToLower(name), strings.ToLower(unwanted)) {
			t.Errorf("name = %q; it carries an identifier rather than a product name", name)
		}
	}
}

// TestDriverVersionComesFromTheCardsOwnModule pins that the version is read from
// the module the card is actually bound to — the module name is not hardcoded,
// because the Lisuan driver's name is not documented.
func TestDriverVersionComesFromTheCardsOwnModule(t *testing.T) {
	files := cardFixture()
	files["module/lisuan_drm/version"] = "1.2.3\n"
	dir := writeFixture(t, files)
	useSysDir(t, dir)

	// sysfs links the card's driver to the bound driver and that driver's module
	// to the module directory; the fixture reproduces both links.
	driverDir := filepath.Join(dir, "bus/pci/drivers/lisuan")
	if err := os.MkdirAll(driverDir, 0755); err != nil {
		t.Fatalf("create driver dir: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "module/lisuan_drm"), filepath.Join(driverDir, "module")); err != nil {
		t.Fatalf("link module: %v", err)
	}
	if err := os.Symlink(driverDir, filepath.Join(dir, "class/drm/card0/device/driver")); err != nil {
		t.Fatalf("link driver: %v", err)
	}

	cards := GetAllInformations()
	if len(cards) != 1 {
		t.Fatalf("got %d card(s); want one", len(cards))
	}
	if got := cards[0].Values[shared.GPUDriverVersion.Key].Value; got != "1.2.3" {
		t.Errorf("driver version = %v; want the version of the module driving the card", got)
	}
}

// TestUnboundCardReportsNoDriverVersion: a card with no driver bound publishes no
// version, and a version row would be a claim about software that is not running.
func TestUnboundCardReportsNoDriverVersion(t *testing.T) {
	useSysDir(t, writeFixture(t, cardFixture()))

	cards := GetAllInformations()
	if len(cards) != 1 {
		t.Fatalf("got %d card(s); want one", len(cards))
	}
	assertAbsent(t, cards[0], shared.GPUDriverVersion.Key)
}

// TestTwoCardsGetTwoEntries: two Lisuan cards are two cards, each with its own
// readings, not one card overwritten by the other.
func TestTwoCardsGetTwoEntries(t *testing.T) {
	files := cardFixture()
	files["class/drm/card1/device/vendor"] = "0x4c54\n"
	files["class/drm/card1/device/device"] = "0x5001\n"
	files["class/drm/card1/device/hwmon/hwmon1/temp1_input"] = "77000\n"
	useSysDir(t, writeFixture(t, files))

	cards := GetAllInformations()
	if len(cards) != 2 {
		t.Fatalf("got %d card(s); want two", len(cards))
	}
	if got := numeric(t, cards[0], shared.GPUTemp.Key); got != 45 {
		t.Errorf("first card temperature = %v; want 45", got)
	}
	if got := numeric(t, cards[1], shared.GPUTemp.Key); got != 77 {
		t.Errorf("second card temperature = %v; want 77", got)
	}
}

// TestLisuanModelsDescribeEveryPublishedDevice keeps the naming table honest: it
// is the only thing naming a card, so a malformed row (a key that is not a PCI
// device ID, or a name that drops the vendor) would put a wrong name on real
// hardware.
func TestLisuanModelsDescribeEveryPublishedDevice(t *testing.T) {
	if len(lisuanModels) == 0 {
		t.Fatal("the naming table is empty; every Lisuan card would fall back to the vendor name")
	}

	for deviceID, name := range lisuanModels {
		if len(deviceID) != 4 || strings.ToUpper(deviceID) != deviceID {
			t.Errorf("device ID %q is not an uppercase 4-digit hex value", deviceID)
		}
		for _, char := range deviceID {
			if !strings.ContainsRune("0123456789ABCDEF", char) {
				t.Errorf("device ID %q contains %q, which is not hexadecimal", deviceID, char)
			}
		}
		if name == "" || !strings.HasPrefix(name, "LISUAN ") {
			t.Errorf("device %s is named %q; want the PCI database's name for the vendor", deviceID, name)
		}
	}

	// The 7G100 series is the only product the PCI database lists for this vendor,
	// and those are the devices the collector must therefore be able to name.
	for _, deviceID := range []string{"5000", "5001", "5002", "5003", "5004", "5005", "5006"} {
		if _, ok := lisuanModels[deviceID]; !ok {
			t.Errorf("device %s is not in the table; the PCI database lists it", deviceID)
		}
	}
}

// TestClassifyTemperatureLabel is the mapping the rows rest on, so its edges are
// pinned: an unknown word is the card's own temperature rather than a guess at a
// row, and the words are matched regardless of case or surrounding text.
func TestClassifyTemperatureLabel(t *testing.T) {
	cases := map[string]temperatureKind{
		"":                      temperatureCard,
		"GPU":                   temperatureCard,
		"edge":                  temperatureCard,
		"Hotspot":               temperatureHotspot,
		"hot spot":              temperatureHotspot,
		"GPU Junction Temp":     temperatureHotspot,
		"Memory":                temperatureMemory,
		"VRAM":                  temperatureMemory,
		"HBM2":                  temperatureMemory,
		"fan controller sensor": temperatureCard,
	}
	for label, want := range cases {
		if got := classifyTemperatureLabel(label); got != want {
			t.Errorf("classifyTemperatureLabel(%q) = %v; want %v", label, got, want)
		}
	}
}
