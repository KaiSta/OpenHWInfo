package intel

import (
	"fmt"
	"os"
	"path/filepath"
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
		filepath.Join(hwmonDir, "temp1_input"):        "42000",      // 42 C
		filepath.Join(hwmonDir, "temp2_input"):        "51000",      // 51 C
		filepath.Join(hwmonDir, "freq0_input"):        "1350000000", // 1350 MHz (1.35 GHz in Hz)
		filepath.Join(hwmonDir, "power1_average"):     "180000000",  // 180 W with divisor 1000000
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

	// Run Available() check to ensure it detects the mock card
	if !Available() {
		t.Error("expected Intel card to be available under mock sysDir")
	}

	// Get informations
	info := GetInformations()

	// GPU name and driver version must always be present
	if _, ok := info.Values[shared.GPUName.Key]; !ok {
		t.Errorf("expected %s to be present in hardware values", shared.GPUName.Key)
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
		if tempNum != 42 {
			t.Errorf("expected GPU temperature to be 42, got %f", tempNum)
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
		if powerNum != 180 {
			t.Errorf("expected GPU power to be 180 W, got %f", powerNum)
		}
	}

	// VRAM metrics are not exposed by i915 and must not be set to bogus values
	for _, key := range []string{shared.VRAMTotal.Key, shared.VRAMUsed.Key, shared.VRAMVendor.Key} {
		if _, exists := info.Values[key]; exists {
			t.Errorf("did not expect %s to be present in hardware values for Intel", key)
		}
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
		// Discrete Arc GPU identified by PCI ID.
		{
			line: "01:00.0 VGA compatible controller [0300]: Intel Corporation Device 9a49 [8086:9a49] (rev 02)",
			want: "Intel Arc A380",
		},
		// Unknown device ID falls back to the cleaned-up lspci name.
		{
			line: "01:00.0 VGA compatible controller [0300]: Intel Corporation Test GPU [8086:beef] (rev 01)",
			want: "Intel Corporation Test GPU",
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
	if model := getIntelDeviceModel("9a49"); model != "Intel Arc A380" {
		t.Errorf("getIntelDeviceModel(9a49) = %q, want Intel Arc A380", model)
	}
	if model := getIntelDeviceModel("unknown-id-here"); model != "" {
		t.Errorf("getIntelDeviceModel(unknown) = %q, want empty string", model)
	}
	if model := getIntelDeviceModel(""); model != "" {
		t.Errorf("getIntelDeviceModel(empty) = %q, want empty string", model)
	}
}
