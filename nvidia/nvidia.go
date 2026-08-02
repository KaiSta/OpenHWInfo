package nvidia

import (
	"log"
	"os/exec"
	"strings"

	"github.com/KaiSta/OpenHWInfo/shared"
)

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

func getGPUTemperature() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	temp := strings.TrimSpace(string(output))
	return temp + " °C"
}

func getGPUHotspotTemperature() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=temperature.memory --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	temp := strings.TrimSpace(string(output))
	return temp + " °C"
}

func getVRAMTemperature() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=temperature.memory --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	temp := strings.TrimSpace(string(output))
	return temp + " °C"
}

func getGPUFrequency() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=clocks.sm --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	freq := strings.TrimSpace(string(output))
	return freq + " MHz"
}

func getVRAMFrequency() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=clocks.mem --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	freq := strings.TrimSpace(string(output))
	return freq + " MHz"
}

func getVRAMTotal() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=memory.total --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	vramTotal := strings.TrimSpace(string(output))
	return vramTotal
}

func getVRAMUsed() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=memory.used --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	vramUsed := strings.TrimSpace(string(output))
	return vramUsed
}

func getVRAMVendor() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=vbios.version --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	vramVendor := strings.TrimSpace(string(output))
	return vramVendor
}

func getGPUUsage() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	usage := strings.TrimSpace(string(output))
	return usage
}

func getGPUPowerMax() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=power.max --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	power := strings.TrimSpace(string(output))
	return power
}

func getGPUPowerAvg() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=power.draw --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	power := strings.TrimSpace(string(output))
	return power
}

func getFanSpeed() string {
	cmd := exec.Command("sh", "-c", "nvidia-smi --query-gpu=fan.speed --format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error executing nvidia-smi command: %v\n", err)
		return "N/A"
	}
	fanSpeed := strings.TrimSpace(string(output))
	return fanSpeed
}

func GetInformations() (string, map[string]string) {
	data := make(map[string]string)

	if !probeNvidia() {
		log.Println("Returning default info due to no NVIDIA cards found")
		return "NVIDIA GPU", data
	}

	gpuName := getGPUName2()
	data[shared.GPUName.Key] = gpuName

	driverVersion := getDriverVersion()
	data[shared.GPUDriverVersion.Key] = driverVersion

	data[shared.GPUTemp.Key] = getGPUTemperature()
	data[shared.GPUHotspotTemp.Key] = getGPUHotspotTemperature()
	data[shared.VRAMTemp.Key] = getVRAMTemperature()

	data[shared.GPUFrequency.Key] = getGPUFrequency()
	data[shared.VRAMLrequency.Key] = getVRAMFrequency()

	data[shared.VRAMTotal.Key] = getVRAMTotal()
	data[shared.VRAMUsed.Key] = getVRAMUsed()
	data[shared.VRAMVendor.Key] = getVRAMVendor()

	data[shared.GPUUsage.Key] = getGPUUsage()
	data[shared.GPUPowerMax.Key] = getGPUPowerMax()
	data[shared.GPUPowerAvg.Key] = getGPUPowerAvg()
	data[shared.FanSpeed.Key] = getFanSpeed()

	return gpuName, data
}
