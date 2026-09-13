package intel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

func TestGetInformationsWithIntelCard(t *testing.T) {
	// Create a temporary directory to mock /sys
	tempDir := t.TempDir()

	// Backup original sysDir and restore after test
	originalSysDir := sysDir
	sysDir = tempDir
	defer func() {
		sysDir = originalSysDir
	}()

	// Intel code looks for hwmon at card0/device/hwmon/hwmon0, then device at hwmon0/device
	cardIndex := 0
	cardDir := filepath.Join(tempDir, "class/drm", fmt.Sprintf("card%d", cardIndex))
	cardDevicePath := filepath.Join(cardDir, "device")
	hwmonDir := filepath.Join(cardDevicePath, "hwmon", "hwmon0")

	// Create directories
	err := os.MkdirAll(hwmonDir, 0755)
	if err != nil {
		t.Fatalf("failed to create mock directories: %v", err)
	}

	// Write mock sysfs files with realistic values
	files := map[string]string{
		filepath.Join(cardDevicePath, "vendor"):       "0x8086",     // Intel PCI vendor ID
		filepath.Join(cardDevicePath, "device"):       "0x3E9B",     // Coffee Lake UHD Graphics 630
		filepath.Join(hwmonDir, "temp1_input"):        "42500",      // 42.5 C
		filepath.Join(hwmonDir, "temp2_input"):        "51000",      // 51 C
		filepath.Join(hwmonDir, "freq0_input"):        "1350000000", // 1350 MHz (1.35 GHz in Hz)
		filepath.Join(hwmonDir, "power1_average"):     "179500000",  // 179.5 W with divisor 1000000
		filepath.Join(tempDir, "module/i915/version"): "6.8.0-45-generic",
	}

	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create parent directory for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write mock file %s: %v", path, err)
		}
	}

	// Get informations
	gpus := GetAllInformations()
	if len(gpus) != 1 {
		t.Fatalf("expected exactly 1 Intel GPU, got %d", len(gpus))
	}
	info := gpus[0]

	// The name must come from the mocked device ID, not from whatever GPU this
	// test host happens to have: with the ID present the naming path never needs
	// lspci or glxinfo, which keeps the assertion deterministic.
	nameVal, ok := info.Values[shared.GPUName.Key]
	if !ok {
		t.Errorf("expected %s to be present in hardware values", shared.GPUName.Key)
	} else if nameVal.Value != "Intel UHD Graphics 630" {
		t.Errorf("GPU name = %v; want %q", nameVal.Value, "Intel UHD Graphics 630")
	}
	if _, ok := info.Values[shared.GPUDriverVersion.Key]; !ok {
		t.Errorf("expected %s to be present in hardware values", shared.GPUDriverVersion.Key)
	}

	// Verify GPU temperature is parsed correctly (millidegrees -> degrees)
	tempVal, ok := info.Values[shared.GPUTemp.Key]
	if !ok {
		t.Errorf("expected %s to be present in hardware values", shared.GPUTemp.Key)
	} else {
		tempNum, err := tempVal.GetNumericValue()
		if err != nil {
			t.Errorf("failed to get numeric value for GPU temperature: %v", err)
		}
		// 42.5, not 42: the millidegree conversion is done in floating point, so
		// the fraction survives an integer division that used to drop it.
		if tempNum != 42.5 {
			t.Errorf("expected GPU temperature to be 42.5, got %f", tempNum)
		}
	}

	// Verify GPU frequency is parsed correctly (Hz -> MHz) and >= 0
	freqVal, ok := info.Values[shared.GPUFrequency.Key]
	if !ok {
		t.Errorf("expected %s to be present in hardware values", shared.GPUFrequency.Key)
	} else {
		freqNum, err := freqVal.GetNumericValue()
		if err != nil {
			t.Errorf("failed to get numeric value for GPU frequency: %v", err)
		}
		if freqNum != 1350 {
			t.Errorf("expected GPU frequency to be 1350 MHz, got %f", freqNum)
		}
	}

	// Verify average power is parsed correctly (microwatts -> watts) and >= 0
	powerVal, ok := info.Values[shared.GPUPowerAvg.Key]
	if !ok {
		t.Errorf("expected %s to be present in hardware values", shared.GPUPowerAvg.Key)
	} else {
		powerNum, err := powerVal.GetNumericValue()
		if err != nil {
			t.Errorf("failed to get numeric value for GPU power: %v", err)
		}
		if powerNum != 179.5 {
			t.Errorf("expected GPU power to be 179.5 W, got %f", powerNum)
		}
	}

	// VRAM metrics are not exposed by i915 and must not be set to bogus values
	for _, key := range []string{shared.VRAMTotal.Key, shared.VRAMUsed.Key, shared.VRAMVendor.Key} {
		if _, exists := info.Values[key]; exists {
			t.Errorf("did not expect %s to be present in hardware values for Intel", key)
		}
	}
}

// TestGetGPUNameAlwaysNamesTheCard guards the case where every naming source
// fails: the function used to return whatever the last branch produced, which
// became an empty GPU name once identifier-only text was rejected.
func TestGetGPUNameAlwaysNamesTheCard(t *testing.T) {
	tempDir := t.TempDir()

	originalSysDir := sysDir
	sysDir = tempDir
	defer func() {
		sysDir = originalSysDir
	}()

	// A card whose device ID is not in the table, so the ID lookup cannot name
	// it. Whatever the host's lspci/glxinfo report, the result must be usable.
	cardDevicePath := filepath.Join(tempDir, "class/drm/card0/device")
	if err := os.MkdirAll(cardDevicePath, 0755); err != nil {
		t.Fatalf("failed to create mock card: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cardDevicePath, "vendor"), []byte("0x8086"), 0644); err != nil {
		t.Fatalf("failed to write vendor file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cardDevicePath, "device"), []byte("0xbEEf"), 0644); err != nil {
		t.Fatalf("failed to write device file: %v", err)
	}

	name := getGPUName(0)
	if name == "" {
		t.Fatal("getGPUName returned an empty name")
	}
	if strings.HasPrefix(name, "0") || strings.Contains(name, "8086") {
		t.Errorf("getGPUName = %q; looks like raw PCI identifiers rather than a name", name)
	}
}

func TestGetAllInformationsMultipleCards(t *testing.T) {
	// Create a temporary directory to mock /sys
	tempDir := t.TempDir()

	// Backup original sysDir and restore after test
	originalSysDir := sysDir
	sysDir = tempDir
	defer func() {
		sysDir = originalSysDir
	}()

	// Intel code looks for hwmon at cardX/device/hwmon/hwmonY, then device at hwmonY/device
	for cardIndex := 0; cardIndex < 2; cardIndex++ {
		cardDir := filepath.Join(tempDir, "class/drm", fmt.Sprintf("card%d", cardIndex))
		cardDevicePath := filepath.Join(cardDir, "device")
		hwmonDir := filepath.Join(cardDevicePath, "hwmon", fmt.Sprintf("hwmon%d", cardIndex))

		err := os.MkdirAll(hwmonDir, 0755)
		if err != nil {
			t.Fatalf("failed to create mock directories for card %d: %v", cardIndex, err)
		}

		files := map[string]string{
			filepath.Join(cardDevicePath, "vendor"): "0x8086",
			filepath.Join(hwmonDir, "temp1_input"):  fmt.Sprintf("%d000", 40+cardIndex*10),
			filepath.Join(hwmonDir, "freq0_input"):  fmt.Sprintf("%d000000000", 13+cardIndex),
		}

		for path, content := range files {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatalf("failed to create parent directory for %s: %v", path, err)
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("failed to write mock file %s: %v", path, err)
			}
		}
	}

	// GetAllInformations should return both cards
	gpus := GetAllInformations()
	if len(gpus) != 2 {
		t.Errorf("expected 2 GPUs, got %d", len(gpus))
	}

	// Verify each GPU has temperature data
	for i, gpu := range gpus {
		tempVal, ok := gpu.Values[shared.GPUTemp.Key]
		if !ok {
			t.Errorf("expected %s to be present for GPU %d", shared.GPUTemp.Key, i)
			continue
		}
		tempNum, err := tempVal.GetNumericValue()
		if err != nil {
			t.Errorf("failed to get numeric value for GPU %d temperature: %v", i, err)
			continue
		}
		expectedTemp := float64(40 + i*10)
		if tempNum != expectedTemp {
			t.Errorf("expected GPU %d temperature to be %f, got %f", i, expectedTemp, tempNum)
		}
	}
}

func TestParseIntelGPUName(t *testing.T) {
	testCases := []struct {
		line      string
		deviceIDs []string
		want      string
	}{
		// Real-world line from a Haswell laptop (lspci -s <addr> -n): the PCI ID
		// in the [8086:xxxx] marker must map to the specific model name.
		{
			line: "00:02.0 VGA compatible controller [0300]: Intel Corporation 4th Gen Core Processor Integrated Graphics Controller [8086:0416] (rev 06)",
			want: "Intel HD Graphics 4600",
		},
		// Bare ID format produced by "lspci -s <addr> -n" on this machine.
		{line: "00:02.0 0300: 8086:0416 (rev 06)", want: "Intel HD Graphics 4600"},
		// Same line without -n (no ID in the output): the sysfs-provided ID is used.
		{
			line:      "00:02.0 VGA compatible controller: Intel Corporation 4th Gen Core Processor Integrated Graphics Controller (rev 06)",
			deviceIDs: []string{"0416"},
			want:      "Intel HD Graphics 4600",
		},
		// glxinfo renderer strings are already branded.
		{line: "Intel(R) UHD Graphics 630", want: "Intel UHD Graphics 630"},
		{line: "Intel(R) Iris(R) Xe Graphics [4th Gen Intel(R) Core Processor]", want: "Intel Iris Xe Graphics [4th Gen Intel Core Processor]"},
		// An integrated GPU whose ID the old table called a discrete Arc card.
		{
			line: "01:00.0 VGA compatible controller [0300]: Intel Corporation TigerLake-LP GT2 [Iris Xe Graphics] [8086:9a49] (rev 02)",
			want: "Intel Iris Xe Graphics",
		},
		// Discrete Arc GPU identified by PCI ID.
		{
			line: "03:00.0 VGA compatible controller [0300]: Intel Corporation DG2 [Arc A770] [8086:56a0] (rev 08)",
			want: "Intel Arc A770",
		},
		// Unknown device ID falls back to the cleaned-up lspci name.
		{
			line: "01:00.0 VGA compatible controller [0300]: Intel Corporation Test GPU [8086:beef] (rev 01)",
			want: "Intel Corporation Test GPU",
		},
		// The numeric lspci form carries no product name. Returning it would
		// name the card "00:02.0 0300: 8086:beef", so nothing is returned and
		// the caller moves on to a query that does carry a name.
		{line: "00:02.0 0300: 8086:beef (rev 01)", want: ""},
		// Bare identifiers are equally nameless.
		{line: "8086:beef", want: ""},
		{line: "00:02.0 0300: 8086:0416 (rev 06)", want: "Intel HD Graphics 4600"},
		// A named lspci line for an ID the table does not know still yields a
		// usable name, which is what the caller falls back to.
		{
			line: "00:02.0 VGA compatible controller: Intel Corporation Foo Bar Graphics Adapter (rev 05)",
			want: "Intel Corporation Foo Bar Graphics Adapter",
		},
	}

	for _, tc := range testCases {
		got := parseIntelGPUName(tc.line, tc.deviceIDs...)
		if got != tc.want {
			t.Errorf("parseIntelGPUName(%q, %v) = %q, want %q", tc.line, tc.deviceIDs, got, tc.want)
		}
	}
}

func TestCleanPCIDeviceName(t *testing.T) {
	testCases := []struct{ in, want string }{
		{
			in:   "00:02.0 VGA compatible controller [0300]: Intel Corporation 4th Gen Core Processor Integrated Graphics Controller [8086:0416] (rev 06)",
			want: "Intel Corporation 4th Gen Core Processor Integrated Graphics Controller",
		},
		{
			in:   "00:02.0 VGA compatible controller: Intel Corporation Foo Bar (rev 05)",
			want: "Intel Corporation Foo Bar",
		},
		{in: "Intel Corporation Plain Name", want: "Intel Corporation Plain Name"},
	}

	for _, tc := range testCases {
		if got := cleanPCIDeviceName(tc.in); got != tc.want {
			t.Errorf("cleanPCIDeviceName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGetIntelDeviceModel(t *testing.T) {
	if model := getIntelDeviceModel("0416"); model != "Intel HD Graphics 4600" {
		t.Errorf("getIntelDeviceModel(0416) = %q, want Intel HD Graphics 4600", model)
	}
	if model := getIntelDeviceModel("unknown-id-here"); model != "" {
		t.Errorf("getIntelDeviceModel(unknown) = %q, want empty string", model)
	}
	if model := getIntelDeviceModel(""); model != "" {
		t.Errorf("getIntelDeviceModel(empty) = %q, want empty string", model)
	}
}

// TestGetIntelDeviceModelIntegratedIsNotArc is the regression test for the
// table's worst failure: three device IDs belonging to integrated GPUs were
// labelled as discrete Arc cards, so a Tiger Lake or Comet Lake laptop reported
// hardware it does not have.
func TestGetIntelDeviceModelIntegratedIsNotArc(t *testing.T) {
	tests := []struct {
		deviceID string
		hardware string
		want     string
	}{
		{"9A49", "TigerLake-LP GT2 [Iris Xe Graphics]", "Intel Iris Xe Graphics"},
		{"9BC4", "CometLake-H GT2 [UHD Graphics]", "Intel UHD Graphics"},
		{"9BC6", "Comet Lake-S GT2 [UHD Graphics P630]", "Intel UHD Graphics P630"},
	}

	for _, tc := range tests {
		got := getIntelDeviceModel(tc.deviceID)
		if got != tc.want {
			t.Errorf("getIntelDeviceModel(%s) = %q; want %q", tc.deviceID, got, tc.want)
		}
		if strings.Contains(got, "Arc") {
			t.Errorf("getIntelDeviceModel(%s) names an Arc card for %s", tc.deviceID, tc.hardware)
		}
	}
}

// TestGetIntelDeviceModelArcIDs covers the real discrete Arc IDs, which the
// previous table lacked entirely (its "Arc" entries were either integrated IDs
// or IDs absent from pci.ids).
func TestGetIntelDeviceModelArcIDs(t *testing.T) {
	tests := map[string]string{
		"5690": "Intel Arc A770M",
		"5693": "Intel Arc A370M",
		"56A0": "Intel Arc A770",
		"56A1": "Intel Arc A750",
		"56A2": "Intel Arc A580",
		"56A5": "Intel Arc A380",
		"56A6": "Intel Arc A310",
		"56B3": "Intel Arc Pro A60",
		"56BE": "Intel Arc Graphics A750E",
		"E20B": "Intel Arc B580",
		"E20C": "Intel Arc B570",
		"7D55": "Intel Arc Graphics", // Meteor Lake's integrated Arc graphics
	}

	for deviceID, want := range tests {
		if got := getIntelDeviceModel(deviceID); got != want {
			t.Errorf("getIntelDeviceModel(%s) = %q; want %q", deviceID, got, want)
		}
	}
}

// TestGetIntelDeviceModelNonGraphicsIDs ensures IDs that share the Intel vendor
// ID but are not GPUs (host bridges, DRAM and PCIe controllers) are not named as
// graphics devices. The previous table mapped several of these to GPU names.
func TestGetIntelDeviceModelNonGraphicsIDs(t *testing.T) {
	for _, deviceID := range []string{"9A26", "5914", "0C09", "0C08", "0152"} {
		if model := getIntelDeviceModel(deviceID); model != "" {
			t.Errorf("getIntelDeviceModel(%s) = %q; want empty (not a graphics device)", deviceID, model)
		}
	}
}

// TestGetIntelDeviceModelAcceptsSysfsForm pins that both the sysfs form
// ("0x0416") and the lspci form ("0416") resolve, since the two callers differ.
func TestGetIntelDeviceModelAcceptsSysfsForm(t *testing.T) {
	for _, in := range []string{"0416", "0x0416", "0X0416", " 0416 "} {
		if got := getIntelDeviceModel(in); got != "Intel HD Graphics 4600" {
			t.Errorf("getIntelDeviceModel(%q) = %q; want Intel HD Graphics 4600", in, got)
		}
	}
}

// TestIntelGraphicsModelsIntegrity enforces the table's own rules. The failure
// that prompted this test was a table typed from memory: it named Arc cards that
// did not exist and mislabelled integrated GPUs, so the shape of every entry is
// checked here rather than trusted.
func TestIntelGraphicsModelsIntegrity(t *testing.T) {
	if len(intelGraphicsModels) < 100 {
		t.Fatalf("intelGraphicsModels has only %d entries; the generated table was truncated", len(intelGraphicsModels))
	}

	for deviceID, model := range intelGraphicsModels {
		if len(deviceID) != 4 || deviceID != strings.ToUpper(deviceID) {
			t.Errorf("device ID %q is not a 4-character uppercase hex string", deviceID)
		}
		if _, err := strconv.ParseUint(deviceID, 16, 16); err != nil {
			t.Errorf("device ID %q is not hex: %v", deviceID, err)
		}
		if !strings.HasPrefix(model, "Intel ") || len(model) <= len("Intel ") {
			t.Errorf("model %q for %s is not a branded, non-empty name", model, deviceID)
		}
		if strings.Contains(model, "(R)") {
			t.Errorf("model %q for %s still carries a trademark mark", model, deviceID)
		}
		// Only Alchemist (DG2, 0x5690-0x56BF), Battlemage (0xE2xx) and Meteor
		// Lake's integrated Arc (0x7D55) may be called Arc. This is the check
		// that would have caught the three mislabelled integrated GPUs.
		if !strings.Contains(model, "Arc") {
			continue
		}
		id, _ := strconv.ParseUint(deviceID, 16, 16)
		isDG2 := id >= 0x5690 && id <= 0x56BF
		isBattlemage := id >= 0xE200 && id <= 0xE2FF
		isMeteorLake := deviceID == "7D55"
		if !isDG2 && !isBattlemage && !isMeteorLake {
			t.Errorf("device ID %s is outside the discrete Arc ranges but is named %q", deviceID, model)
		}
	}
}

// TestDriverVersionIsCachedPerSysfsRoot documents the memoization: the driver
// version is read once per sysfs root, not on every poll, and a fixture that is
// rewritten in place only takes effect after the cache is dropped. Both halves
// matter — caching without the sysfs root in the key would leak one fixture
// tree's version into another's.
func TestDriverVersionIsCachedPerSysfsRoot(t *testing.T) {
	tempDir := t.TempDir()

	originalSysDir := sysDir
	sysDir = tempDir
	defer func() {
		sysDir = originalSysDir
	}()
	shared.ResetProbeCache()
	defer shared.ResetProbeCache()

	versionPath := filepath.Join(tempDir, "module/i915/version")
	if err := os.MkdirAll(filepath.Dir(versionPath), 0755); err != nil {
		t.Fatalf("failed to create module directory: %v", err)
	}
	if err := os.WriteFile(versionPath, []byte("6.8.0-45-generic"), 0644); err != nil {
		t.Fatalf("failed to write version file: %v", err)
	}

	if got := getDriverVersion(); got != "6.8.0-45-generic" {
		t.Fatalf("getDriverVersion = %q; want %q", got, "6.8.0-45-generic")
	}

	if err := os.WriteFile(versionPath, []byte("6.9.0-12-generic"), 0644); err != nil {
		t.Fatalf("failed to rewrite version file: %v", err)
	}
	if got := getDriverVersion(); got != "6.8.0-45-generic" {
		t.Errorf("getDriverVersion = %q after a rewrite; want the cached %q", got, "6.8.0-45-generic")
	}

	shared.ResetProbeCache()
	if got := getDriverVersion(); got != "6.9.0-12-generic" {
		t.Errorf("getDriverVersion = %q after dropping the cache; want %q", got, "6.9.0-12-generic")
	}
}

// TestIntelCodenameOnlyModelsIntegrity checks the hand-maintained map that
// covers parts pci.ids describes only by codename.
func TestIntelCodenameOnlyModelsIntegrity(t *testing.T) {
	for deviceID, model := range intelCodenameOnlyModels {
		if len(deviceID) != 4 || deviceID != strings.ToUpper(deviceID) {
			t.Errorf("device ID %q is not a 4-character uppercase hex string", deviceID)
		}
		if !strings.HasPrefix(model, "Intel ") {
			t.Errorf("model %q for %s is not a branded name", model, deviceID)
		}
		if _, duplicated := intelGraphicsModels[deviceID]; duplicated {
			t.Errorf("device ID %s is in both tables; remove the hand-maintained copy", deviceID)
		}
	}
}
