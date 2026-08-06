package nvidia

import (
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

func Available() bool {
	return probeNvidia()
}

func probeNvidia() bool {
	cmd := exec.Command("sh", "-c", "lspci | grep -i nvidia")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		log.Println("No NVIDIA cards found")
		return false
	}
	return true
}

func getGPUName() string {
	cmd := exec.Command("sh", "-c", "lspci | grep -i nvidia")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing lspci command: %v\n", err)
		return "NVIDIA GPU"
	}
	gpuName := strings.TrimSpace(string(output))
	return gpuName
}

func getGPUName2() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=name --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "NVIDIA GPU"
	}
	gpuName := strings.TrimSpace(string(output))
	return gpuName
}

func getDriverVersion() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-driverversion --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "NVIDIA Driver"
	}
	driverVersion := strings.TrimSpace(string(output))
	return driverVersion
}

func getGPUTemperature() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	temp := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(temp); err == nil {
		return float64(val)
	}
	return -1
}

func getGPUHotspotTemperature() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=temperature.memory --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	temp := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(temp); err == nil {
		return float64(val)
	}
	return -1
}

func getVRAMTemperature() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=temperature.memory --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	temp := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(temp); err == nil {
		return float64(val)
	}
	return -1
}

func getGPUFrequency() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=clocks.sm --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	freq := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(freq); err == nil {
		return float64(val)
	}
	return -1
}

func getVRAMFrequency() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=clocks.mem --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	freq := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(freq); err == nil {
		return float64(val)
	}
	return -1
}

func getVRAMTotal() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=memory.total --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	vramTotal := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(vramTotal); err == nil {
		return float64(val)
	}
	return -1
}

func getVRAMUsed() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=memory.used --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	vramUsed := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(vramUsed); err == nil {
		return float64(val)
	}
	return -1
}

func getVRAMVendor() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=vbios.version --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return ""
	}
	vramVendor := strings.TrimSpace(string(output))
	return vramVendor
}

func getGPUUsage() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	usage := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(usage); err == nil {
		return float64(val)
	}
	return -1
}

func getGPUPowerMax() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=power.max --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	power := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(power); err == nil {
		return float64(val)
	}
	return -1
}

func getGPUPowerAvg() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=power.draw --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	power := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(power); err == nil {
		return float64(val)
	}
	return -1
}

func getFanSpeed() float64 {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=fan.speed --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return -1
	}
	fanSpeed := strings.TrimSpace(string(output))

	// Try to convert to numeric value
	if val, err := strconv.Atoi(fanSpeed); err == nil {
		return float64(val)
	}
	return -1
}

func GetInformations() shared.HardwareData {
	data := make(map[string]shared.HardwareValue)

	if !probeNvidia() {
		log.Println("Returning default info due to no NVIDIA cards found")
		return shared.HardwareData{Name: "NVIDIA GPU", Values: data, IsGPU: true}
	}

	gpuName := getGPUName2()
	data[shared.GPUName.Key] = shared.NewHardwareValue(gpuName, shared.UnknownUnit, shared.Unknown)

	driverVersion := getDriverVersion()
	data[shared.GPUDriverVersion.Key] = shared.NewHardwareValue(driverVersion, shared.UnknownUnit, shared.Unknown)

	data[shared.GPUTemp.Key] = shared.NewHardwareValue(getGPUTemperature(), shared.Celsius, shared.Temperature)
	data[shared.GPUHotspotTemp.Key] = shared.NewHardwareValue(getGPUHotspotTemperature(), shared.Celsius, shared.Temperature)
	data[shared.VRAMTemp.Key] = shared.NewHardwareValue(getVRAMTemperature(), shared.Celsius, shared.Temperature)

	data[shared.GPUFrequency.Key] = shared.NewHardwareValue(getGPUFrequency(), shared.MHz, shared.Frequency)
	data[shared.VRAMLrequency.Key] = shared.NewHardwareValue(getVRAMFrequency(), shared.MHz, shared.Frequency)

	data[shared.VRAMTotal.Key] = shared.NewHardwareValue(getVRAMTotal(), shared.Megabyte, shared.Memory)
	data[shared.VRAMUsed.Key] = shared.NewHardwareValue(getVRAMUsed(), shared.Megabyte, shared.Memory)
	data[shared.VRAMVendor.Key] = shared.NewHardwareValue(getVRAMVendor(), shared.UnknownUnit, shared.Unknown)

	data[shared.GPUUsage.Key] = shared.NewHardwareValue(getGPUUsage(), shared.Percent, shared.Usage)
	data[shared.GPUPowerMax.Key] = shared.NewHardwareValue(getGPUPowerMax(), shared.Watt, shared.Power)
	data[shared.GPUPowerAvg.Key] = shared.NewHardwareValue(getGPUPowerAvg(), shared.Watt, shared.Power)
	data[shared.FanSpeed.Key] = shared.NewHardwareValue(getFanSpeed(), shared.RPM, shared.Unknown)

	return shared.HardwareData{Name: gpuName, Values: data, IsGPU: true}
}
