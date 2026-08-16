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
