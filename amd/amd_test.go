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

	// Construct the mock path structure
	cardIndex := 0
	cardDevicePath := filepath.Join(tempDir, "class/drm", fmt.Sprintf("card%d/device", cardIndex))
	hwmonBase := filepath.Join(cardDevicePath, "hwmon")
	hwmonDir := filepath.Join(hwmonBase, "hwmon0")
	deviceDir := filepath.Join(hwmonDir, "device")

	// Create directories
	err := os.MkdirAll(deviceDir, 0755)
	if err != nil {
		t.Fatalf("failed to create mock directories: %v", err)
	}

	// Write mock sysfs files with realistic values
	files := map[string]string{
		filepath.Join(hwmonDir, "temp1_input"):           "45000",      // 45 C
		filepath.Join(hwmonDir, "temp2_input"):           "55000",      // 55 C
		filepath.Join(hwmonDir, "temp3_input"):           "65000",      // 65 C
		filepath.Join(hwmonDir, "freq1_input"):           "1200000000", // 1200 MHz (1.2 GHz in Hz)
		filepath.Join(hwmonDir, "freq2_input"):           "1000000000", // 1000 MHz (1.0 GHz in Hz)
		filepath.Join(hwmonDir, "power1_cap_max"):        "250000000",  // 250 W with divisor 1000000
		filepath.Join(hwmonDir, "power1_average"):        "150000000",  // 150 W with divisor 1000000
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

	// Run Available() check to ensure it detects the mock card
	if !Available() {
		t.Error("expected AMD card to be available under mock sysDir")
	}

	// Get informations
	info := GetInformations()

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
}
