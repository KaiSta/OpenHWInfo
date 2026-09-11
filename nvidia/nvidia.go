package nvidia

import (
	"encoding/csv"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"unicode"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// gpuQueryFields lists the metrics queried per GPU, in the same order as the
// CSV columns returned by nvidia-smi. Every entry must be a valid field name
// for --query-gpu: if even one is invalid, nvidia-smi exits with an error and
// produces no output at all.
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
	lines := shared.RunShell("nvidia-smi --query-gpu=index --format=csv,noheader")
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
	list := shared.RunShell("nvidia-smi -L")
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

// parseFloatField extracts the leading numeric value from a nvidia-smi CSV
// field, which may be followed by a unit (e.g. "10240 MiB", "45 %"). It
// reports false when no number is available ("[N/A]", empty string).
func parseFloatField(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	end := len(s)
	for i, c := range s {
		if !unicode.IsDigit(c) && c != '.' && c != '-' {
			end = i
			break
		}
	}
	val, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

// parseCSVFields splits one line of nvidia-smi CSV output into fields. A real
// CSV parser is used so that values containing commas (e.g. some GPU names)
// do not shift the column alignment.
func parseCSVFields(line string) []string {
	records, err := csv.NewReader(strings.NewReader(line)).Read()
	if err != nil {
		return strings.Split(line, ",")
	}
	return records
}

// getInformationsForGPU collects all metrics for a single GPU index using one
// nvidia-smi invocation.
func getInformationsForGPU(index int) shared.HardwareData {
	data := make(map[string]shared.HardwareValue)
	name := "NVIDIA GPU"

	query := strings.Join(gpuQueryFields, ",")
	output := shared.RunShell("nvidia-smi -i " + strconv.Itoa(index) + " --query-gpu=" + query + " --format=csv,noheader")
	fields := parseCSVFields(output)

	field := func(i int) string {
		if i < len(fields) {
			return strings.TrimSpace(fields[i])
		}
		return ""
	}

	if field(0) != "" {
		name = field(0)
	}
	data[shared.GPUName.Key] = shared.NewHardwareValue(name, shared.UnknownUnit, shared.Unknown)

	driverVersion := field(1)
	if driverVersion == "" {
		driverVersion = "NVIDIA Driver"
	}
	data[shared.GPUDriverVersion.Key] = shared.NewHardwareValue(driverVersion, shared.UnknownUnit, shared.Unknown)

	// Only metrics the driver actually reports are added; values reported as
	// "[N/A]" (or a failed nvidia-smi call) are omitted so they do not show
	// up as bogus -1 readings.
	setNumeric := func(key string, i int, unit shared.HardwareUnit, valueType shared.HardwareValueType) {
		if val, ok := parseFloatField(field(i)); ok {
			data[key] = shared.NewHardwareValue(val, unit, valueType)
		}
	}

	setNumeric(shared.GPUTemp.Key, 2, shared.Celsius, shared.Temperature)
	setNumeric(shared.VRAMTemp.Key, 3, shared.Celsius, shared.Temperature)
	setNumeric(shared.GPUFrequency.Key, 4, shared.MHz, shared.Frequency)
	setNumeric(shared.VRAMLrequency.Key, 5, shared.MHz, shared.Frequency)
	setNumeric(shared.VRAMTotal.Key, 6, shared.Megabyte, shared.Memory)
	setNumeric(shared.VRAMUsed.Key, 7, shared.Megabyte, shared.Memory)
	setNumeric(shared.GPUUsage.Key, 8, shared.Percent, shared.Usage)
	setNumeric(shared.GPUPowerMax.Key, 9, shared.Watt, shared.Power)
	setNumeric(shared.GPUPowerAvg.Key, 10, shared.Watt, shared.Power)
	setNumeric(shared.FanSpeed.Key, 11, shared.RPM, shared.Unknown)

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
