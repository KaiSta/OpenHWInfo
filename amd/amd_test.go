package amd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

func TestGetInformationsWithAMDCard(t *testing.T) {
	// Create a temporary directory to mock /sys
	tempDir := t.TempDir()

	// Backup original sysDir and restore after test
	originalSysDir := sysDir
	sysDir = tempDir
	defer func() {
		sysDir = originalSysDir
	}()

	// AMD code looks for hwmon at card0/hwmon/hwmon0, then device at hwmon0/device
	cardIndex := 0
	cardDir := filepath.Join(tempDir, "class/drm", fmt.Sprintf("card%d", cardIndex))
	cardDevicePath := filepath.Join(cardDir, "device")
	hwmonDir := filepath.Join(cardDevicePath, "hwmon", "hwmon0")
	deviceDir := filepath.Join(hwmonDir, "device")

	// Create directories and vendor file so probeAMD finds it via sysfs
	err := os.MkdirAll(cardDevicePath, 0755)
	if err != nil {
		t.Fatalf("failed to create mock directories for card %d: %v", cardIndex, err)
	}
	if err := os.WriteFile(filepath.Join(cardDevicePath, "vendor"), []byte("0x1002"), 0644); err != nil {
		t.Fatalf("failed to write vendor file for card %d: %v", cardIndex, err)
	}
	err = os.MkdirAll(deviceDir, 0755)
	if err != nil {
		t.Fatalf("failed to create mock directories: %v", err)
	}

	// Write mock sysfs files with realistic values
	files := map[string]string{
		filepath.Join(hwmonDir, "temp1_input"):           "45500",      // 45.5 C
		filepath.Join(hwmonDir, "temp2_input"):           "55000",      // 55 C
		filepath.Join(hwmonDir, "temp3_input"):           "65000",      // 65 C
		filepath.Join(hwmonDir, "freq1_input"):           "1200000000", // 1200 MHz (1.2 GHz in Hz)
		filepath.Join(hwmonDir, "freq2_input"):           "1000000000", // 1000 MHz (1.0 GHz in Hz)
		filepath.Join(hwmonDir, "power1_cap_max"):        "250000000",  // 250 W with divisor 1000000
		filepath.Join(hwmonDir, "power1_average"):        "149500000",  // 149.5 W with divisor 1000000
		filepath.Join(hwmonDir, "fan1_input"):            "1500",       // 1500 RPM
		filepath.Join(deviceDir, "mem_info_vram_total"):  "8589934592", // 8192 MB
		filepath.Join(deviceDir, "mem_info_vram_used"):   "2147483648", // 2048 MB
		filepath.Join(deviceDir, "mem_info_vram_vendor"): "Samsung",
		filepath.Join(deviceDir, "gpu_busy_percent"):     "45",
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write mock file %s: %v", path, err)
		}
	}

	// Get informations
	gpus := GetAllInformations()
	if len(gpus) != 1 {
		t.Fatalf("expected exactly 1 AMD GPU, got %d", len(gpus))
	}
	info := gpus[0]

	// 1. Verify Frequency/Clock values are >= 0 and not -1
	gpuFreqVal, ok := info.Values[shared.GPUFrequency.Key]
	if !ok {
		t.Errorf("expected %s to be present in hardware values", shared.GPUFrequency.Key)
	} else {
		freqNum, err := gpuFreqVal.GetNumericValue()
		if err != nil {
			t.Errorf("failed to get numeric value for GPU frequency: %v", err)
		}
		if freqNum < 0 {
			t.Errorf("expected GPU Frequency to be >= 0, got %f", freqNum)
		}
	}

	memFreqVal, ok := info.Values[shared.VRAMLrequency.Key]
	if !ok {
		t.Errorf("expected %s to be present in hardware values", shared.VRAMLrequency.Key)
	} else {
		freqNum, err := memFreqVal.GetNumericValue()
		if err != nil {
			t.Errorf("failed to get numeric value for VRAM frequency: %v", err)
		}
		if freqNum < 0 {
			t.Errorf("expected VRAM Frequency to be >= 0, got %f", freqNum)
		}
	}

	// 2. Verify other key values are parsed correctly and are >= 0 / not -1
	gpuUsageVal, ok := info.Values[shared.GPUUsage.Key]
	if !ok {
		t.Errorf("expected %s to be present in hardware values", shared.GPUUsage.Key)
	} else {
		usageNum, err := gpuUsageVal.GetNumericValue()
		if err != nil {
			t.Errorf("failed to get numeric value for GPU usage: %v", err)
		}
		if usageNum < 0 {
			t.Errorf("expected GPU usage to be >= 0, got %f (could be -1 on failure)", usageNum)
		}
	}

	// 3. Fractional readings must survive the conversion. Both of these were
	// truncated towards zero by an integer division before the float conversion:
	// the sensor above reads 45.5 C and the card draws 149.5 W.
	assertNumeric(t, info, shared.GPUTemp.Key, 45.5)
	assertNumeric(t, info, shared.GPUPowerAvg.Key, 149.5)
}

// assertNumeric fails when a key is absent or does not carry want.
func assertNumeric(t *testing.T, info shared.HardwareData, key string, want float64) {
	t.Helper()
	value, ok := info.Values[key]
	if !ok {
		t.Fatalf("expected %s to be present in hardware values", key)
	}
	got, err := value.GetNumericValue()
	if err != nil {
		t.Fatalf("failed to get numeric value for %s: %v", key, err)
	}
	if got != want {
		t.Errorf("%s = %v; want %v", key, got, want)
	}
}

// TestParseTemperatureValueKeepsFraction pins the conversion itself, so the
// end-to-end assertion above cannot pass by way of some other path.
func TestParseTemperatureValueKeepsFraction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "temp1_input"), []byte("45500"), 0o644); err != nil {
		t.Fatal(err)
	}

	data := map[string]shared.HardwareValue{}
	parseTemperatureValue(&data, dir, "temp1_input")

	if got := data[shared.GPUTemp.Key].Value; got != 45.5 {
		t.Errorf("temperature = %v; want 45.5", got)
	}
}

// TestParseMemoryValueKeepsFraction covers a byte count that is not a whole
// number of megabytes, which an integer division rounded down (and a count below
// one megabyte collapsed to zero).
func TestParseMemoryValueKeepsFraction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mem_info_vram_total")
	if err := os.WriteFile(path, []byte("1000000"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := parseMemoryValue(path); got != 1000000.0/(1024*1024) {
		t.Errorf("parseMemoryValue = %v; want %v", got, 1000000.0/(1024*1024))
	}
	if got := parseMemoryValue(filepath.Join(dir, "absent")); got != 0 {
		t.Errorf("parseMemoryValue of an absent file = %v; want 0", got)
	}
}

func TestCleanGPUName(t *testing.T) {
	cases := []struct {
		raw    string
		expect string
	}{
		{
			raw:    "0c:00.0 VGA compatible controller: Advanced Micro Devices, Inc. [AMD/ATI] Navi 31 [Radeon RX 7900 XT/7900 XTX/7900M] (rev cc)",
			expect: "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900M]",
		},
		{
			raw:    "01:00.0 VGA compatible controller: Advanced Micro Devices, Inc. [AMD/ATI] Radeon RX 6800 (rev c2)",
			expect: "Radeon RX 6800",
		},
		{
			raw:    "",
			expect: "AMD GPU",
		},
	}

	for _, tc := range cases {
		got := cleanGPUName(tc.raw)
		if got != tc.expect {
			t.Errorf("cleanGPUName(%q) = %q, want %q", tc.raw, got, tc.expect)
		}
	}
}

func TestDisambiguateGPUName(t *testing.T) {
	const navi31 = "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900M]"

	cases := []struct {
		chipset string
		vramMB  float64
		expect  string
	}{
		{navi31, 20480, "Radeon RX 7900 XT"},          // 20 GB -> XT
		{navi31, 24576, "Radeon RX 7900 XTX"},         // 24 GB -> XTX
		{navi31, 0, navi31},                           // no VRAM info -> keep chipset name
		{"Unknown chipset", 20480, "Unknown chipset"}, // unmapped chipset unchanged
	}

	for _, tc := range cases {
		got := disambiguateGPUName(tc.chipset, tc.vramMB)
		if got != tc.expect {
			t.Errorf("disambiguateGPUName(%q, %v) = %q, want %q", tc.chipset, tc.vramMB, got, tc.expect)
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

	// Create two AMD cards
	for cardIndex := 0; cardIndex < 2; cardIndex++ {
		// Create vendor file at card device path so probeAMD finds it via sysfs
		cardDevicePath := filepath.Join(tempDir, "class/drm", fmt.Sprintf("card%d/device", cardIndex))
		err := os.MkdirAll(cardDevicePath, 0755)
		if err != nil {
			t.Fatalf("failed to create mock directories for card %d: %v", cardIndex, err)
		}
		if err := os.WriteFile(filepath.Join(cardDevicePath, "vendor"), []byte("0x1002"), 0644); err != nil {
			t.Fatalf("failed to write vendor file for card %d: %v", cardIndex, err)
		}

		// AMD code looks for hwmon at cardX/hwmon/hwmonY, then device at hwmonY/device
		hwmonDir := filepath.Join(tempDir, "class/drm", fmt.Sprintf("card%d/device/hwmon/hwmon%d", cardIndex, cardIndex))
		deviceDir := filepath.Join(hwmonDir, "device")

		err = os.MkdirAll(deviceDir, 0755)
		if err != nil {
			t.Fatalf("failed to create mock directories for card %d: %v", cardIndex, err)
		}

		files := map[string]string{
			filepath.Join(hwmonDir, "temp1_input"):       fmt.Sprintf("%d000", 40+cardIndex*10),
			filepath.Join(hwmonDir, "fan1_input"):        fmt.Sprintf("%d00", 1500+cardIndex*100),
			filepath.Join(deviceDir, "gpu_busy_percent"): fmt.Sprintf("%d", 40+cardIndex*5),
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
