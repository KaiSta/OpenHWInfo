package nvidia

import (
	"encoding/csv"
	"log"
	"strconv"
	"strings"
	"unicode"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// gpuQueryFields lists the metrics queried per GPU, in the same order as the
// CSV columns returned by nvidia-smi. Every entry must be a valid field name
// for --query-gpu: if even one is invalid, nvidia-smi exits with an error and
// produces no output at all.
//
// "index" is appended last so that the metric columns keep the positions the
// parser below reads them from, while still letting one query describe every
// GPU on the system. Without it a second nvidia-smi call is needed just to
// discover the card indices.
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
	"index",
}

// gpuIndexColumn is the position of the "index" field in gpuQueryFields. It must
// stay in step with the slice above; TestGPUQueryFieldsOrder asserts that.
const gpuIndexColumn = 12

// queryGPUs returns one parsed CSV record per NVIDIA GPU, using a single
// nvidia-smi invocation for the whole system. An empty result means nvidia-smi
// could not be queried at all (no driver, no cards). This is also what decides
// whether the vendor is "available": the card being visible in lspci is not
// enough, because without the proprietary driver nvidia-smi is absent, every
// metric query fails, and the card would render with no readings at all.
func queryGPUs() [][]string {
	query := strings.Join(gpuQueryFields, ",")
	output := shared.RunCommand("nvidia-smi", "--query-gpu="+query, "--format=csv,noheader")
	if strings.TrimSpace(output) == "" {
		shared.Debugf("No output from nvidia-smi --query-gpu (driver installed?)")
		return nil
	}
	return parseGPUOutput(output)
}

// parseGPUOutput splits nvidia-smi's multi-line CSV output into one field slice
// per GPU. Splitting on lines is safe: a GPU name can contain a comma but not a
// newline.
func parseGPUOutput(output string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rows = append(rows, parseCSVFields(line))
	}
	return rows
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

// parseGPURecord builds one GPU's metrics from a single nvidia-smi CSV record.
// It takes the fields rather than running the query itself so that the column
// mapping is testable against a synthetic row.
func parseGPURecord(fields []string) shared.HardwareData {
	data := make(map[string]shared.HardwareValue)
	name := "NVIDIA GPU"

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

	return shared.HardwareData{Name: name, Values: data}
}

// GetAllInformations returns the information for every NVIDIA GPU on the system.
// It returns nil when nvidia-smi cannot be queried, so callers can treat an empty
// result as "vendor unavailable" without probing a second time. Cards that expose
// no readings are omitted rather than reported as empty entries.
//
// One nvidia-smi call covers every GPU: nvidia-smi already reports all of them,
// so querying each by index would fork it twice per tick (once to discover the
// indices, once per card) for the same data.
func GetAllInformations() []shared.HardwareData {
	rows := queryGPUs()
	if len(rows) == 0 {
		return nil
	}
	shared.Debugf("nvidia-smi reported %d GPU(s)", len(rows))

	var result []shared.HardwareData
	for rowIndex, fields := range rows {
		// Fall back to the row position when the index column is unusable.
		index := rowIndex
		if len(fields) > gpuIndexColumn {
			if parsed, err := strconv.Atoi(strings.TrimSpace(fields[gpuIndexColumn])); err == nil {
				index = parsed
			}
		}
		data := parseGPURecord(fields)
		if !data.HasTelemetry() {
			log.Printf("Skipping NVIDIA GPU %d: no readable metrics\n", index)
			continue
		}
		result = append(result, data)
	}
	return result
}
