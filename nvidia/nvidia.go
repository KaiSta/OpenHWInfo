package nvidia

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
	"unicode"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// gpuQueryFields lists the metrics queried per GPU, in the same order as the
// CSV columns returned by nvidia-smi.
var gpuQueryFields = []string{
	"name",
	"driver_version",
	"temperature.gpu",
	"temperature.memory",
	"clocks.sm",
	"clocks.mem",
	"memory.total",
	"memory.used",
	"utilization.gpu",
	"power.limit",
	"power.draw",
	"fan.speed",
	"vbios.version",
}

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

// gpuIndices returns the index of every NVIDIA GPU known to nvidia-smi.
func gpuIndices() []int {
	lines := runShell("nvidia-smi --query-gpu=index --format=csv,noheader")
	var indices []int
	for _, line := range strings.Split(lines, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx, err := strconv.Atoi(line); err == nil {
			indices = append(indices, idx)
		}
	}
	if len(indices) > 0 {
		return indices
	}

	// Fall back to counting the cards reported by `nvidia-smi -L`.
	list := runShell("nvidia-smi -L")
	count := 0
	for _, line := range strings.Split(list, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	if count > 0 {
		indices = make([]int, count)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}

	return []int{0}
}

func runShell(command string) string {
	output, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		log.Printf("Error executing command %q: %v\n", command, err)
		return ""
	}
	return strings.TrimSpace(string(output))
}

// parseFloatField extracts the leading numeric value from a nvidia-smi CSV
// field, which may be followed by a unit (e.g. "10240 MiB", "45 %") or be
// unavailable ("[N/A]").
func parseFloatField(s string) float64 {
	s = strings.TrimSpace(s)
	end := len(s)
	for i, c := range s {
		if !unicode.IsDigit(c) && c != '.' && c != '-' {
			end = i
			break
		}
	}
	if val, err := strconv.ParseFloat(s[:end], 64); err == nil {
		return val
	}
	return -1
}

// getInformationsForGPU collects all metrics for a single GPU index using one
// nvidia-smi invocation.
func getInformationsForGPU(index int) shared.HardwareData {
	data := make(map[string]shared.HardwareValue)
	name := "NVIDIA GPU"

	query := strings.Join(gpuQueryFields, ",")
	output := runShell("nvidia-smi -i " + strconv.Itoa(index) + " --query-gpu=" + query + " --format=csv,noheader")
	fields := strings.Split(output, ",")

	field := func(i int) string {
		if i < len(fields) {
			return strings.TrimSpace(fields[i])
		}
		return ""
	}

	if len(fields) > 0 && field(0) != "" {
		name = field(0)
	}
	data[shared.GPUName.Key] = shared.NewHardwareValue(name, shared.UnknownUnit, shared.Unknown)

	driverVersion := field(1)
	if driverVersion == "" {
		driverVersion = "NVIDIA Driver"
	}
	data[shared.GPUDriverVersion.Key] = shared.NewHardwareValue(driverVersion, shared.UnknownUnit, shared.Unknown)

	data[shared.GPUTemp.Key] = shared.NewHardwareValue(parseFloatField(field(2)), shared.Celsius, shared.Temperature)
	data[shared.GPUHotspotTemp.Key] = shared.NewHardwareValue(parseFloatField(field(3)), shared.Celsius, shared.Temperature)
	data[shared.VRAMTemp.Key] = shared.NewHardwareValue(parseFloatField(field(3)), shared.Celsius, shared.Temperature)

	data[shared.GPUFrequency.Key] = shared.NewHardwareValue(parseFloatField(field(4)), shared.MHz, shared.Frequency)
	data[shared.VRAMLrequency.Key] = shared.NewHardwareValue(parseFloatField(field(5)), shared.MHz, shared.Frequency)

	data[shared.VRAMTotal.Key] = shared.NewHardwareValue(parseFloatField(field(6)), shared.Megabyte, shared.Memory)
	data[shared.VRAMUsed.Key] = shared.NewHardwareValue(parseFloatField(field(7)), shared.Megabyte, shared.Memory)
	data[shared.VRAMVendor.Key] = shared.NewHardwareValue(field(12), shared.UnknownUnit, shared.Unknown)

	data[shared.GPUUsage.Key] = shared.NewHardwareValue(parseFloatField(field(8)), shared.Percent, shared.Usage)
	data[shared.GPUPowerMax.Key] = shared.NewHardwareValue(parseFloatField(field(9)), shared.Watt, shared.Power)
	data[shared.GPUPowerAvg.Key] = shared.NewHardwareValue(parseFloatField(field(10)), shared.Watt, shared.Power)
	data[shared.FanSpeed.Key] = shared.NewHardwareValue(parseFloatField(field(11)), shared.RPM, shared.Unknown)

	return shared.HardwareData{Name: name, Values: data, IsGPU: true}
}

// GetInformations returns the information for the first NVIDIA GPU.
func GetInformations() shared.HardwareData {
	all := GetAllInformations()
	if len(all) == 0 {
		return shared.HardwareData{Name: "NVIDIA GPU", Values: make(map[string]shared.HardwareValue), IsGPU: true}
	}
	return all[0]
}

// GetAllInformations returns the information for every NVIDIA GPU on the system.
func GetAllInformations() []shared.HardwareData {
	if !probeNvidia() {
		log.Println("Returning default info due to no NVIDIA cards found")
		return []shared.HardwareData{
			{Name: "NVIDIA GPU", Values: make(map[string]shared.HardwareValue), IsGPU: true},
		}
	}

	var result []shared.HardwareData
	for _, index := range gpuIndices() {
		log.Printf("Collecting information for NVIDIA GPU %d\n", index)
		result = append(result, getInformationsForGPU(index))
	}
	if len(result) == 0 {
		result = append(result, shared.HardwareData{Name: "NVIDIA GPU", Values: make(map[string]shared.HardwareValue), IsGPU: true})
	}
	return result
}
