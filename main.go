package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/amd"
	"github.com/KaiSta/OpenHWInfo/cpuinfo"
	"github.com/KaiSta/OpenHWInfo/nvidia"
	"github.com/KaiSta/OpenHWInfo/shared"
)

type Data struct {
	CpuKeys      []shared.KeyTemplate
	GpuKeys      []shared.KeyTemplate
	CpuValues    map[string]string
	GpuValues    map[string]string
	CpuMinValues map[string]string
	CpuMaxValues map[string]string
	GpuMinValues map[string]string
	GpuMaxValues map[string]string
}

var cpuTemplates = []shared.KeyTemplate{
	shared.CPUVendorID,
	shared.CPUBrand,
	shared.CPUModel,
	shared.CPUClock0,
	shared.CPUClock1,
	shared.CPUClock2,
	shared.CPUClock3,
	shared.CPUTemp,
	shared.CPUPCHTemp,
	shared.TCcd0,
	shared.TCcd1,
	shared.TCcd2,
	shared.TCcd3,
	shared.TCcd4,
	shared.TCcd5,
}

var gpuTemplates = []shared.KeyTemplate{
	shared.GPUName,
	shared.GPUDriverVersion,
	shared.GPUTemp,
	shared.GPUHotspotTemp,
	shared.VRAMTemp,
	shared.GPUFrequency,
	shared.VRAMLrequency,
	shared.VRAMTotal,
	shared.VRAMUsed,
	shared.VRAMVendor,
	shared.GPUUsage,
	shared.GPUPowerMax,
	shared.GPUPowerAvg,
	shared.FanSpeed,
}

// Global maps to track historical values for min/max calculations
var cpuHistory = make(map[string][]float64)
var gpuHistory = make(map[string][]float64)

var templateCache *template.Template

func init() {
	templateCache = template.Must(template.ParseGlob("website/*.html"))
}

// updateHistoryAndCalculateMinMax updates the history of values and calculates min/max
func updateHistoryAndCalculateMinMax(history map[string][]float64, currentValues map[string]string) (map[string]string, map[string]string) {
	minValues := make(map[string]string)
	maxValues := make(map[string]string)

	// Update history with current values and calculate min/max
	for key, value := range currentValues {
		if value == "" {
			continue
		}

		// Parse the numeric value from the string (remove units like "C", "Mhz", etc.)
		numStr := parseNumericValue(value)
		if numStr == "" {
			continue
		}

		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			continue
		}

		// Update history
		history[key] = append(history[key], num)

		// Keep only the last 100 values to prevent memory issues
		if len(history[key]) > 100 {
			history[key] = history[key][len(history[key])-100:]
		}

		// Calculate min and max
		min, max := calculateMinMax(history[key])
		minValues[key] = fmt.Sprintf("%.2f", min)
		maxValues[key] = fmt.Sprintf("%.2f", max)
	}

	return minValues, maxValues
}

// parseNumericValue extracts the numeric part from a value string
func parseNumericValue(value string) string {
	// Remove common units and symbols
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " °C", "")
	value = strings.ReplaceAll(value, "C", "")
	value = strings.ReplaceAll(value, " Mhz", "")
	value = strings.ReplaceAll(value, " MHz", "")
	value = strings.ReplaceAll(value, " W", "")
	value = strings.ReplaceAll(value, " MB", "")
	value = strings.ReplaceAll(value, "%", "")

	// Extract the first numeric part
	for i := 0; i < len(value); i++ {
		if value[i] == '.' || (value[i] >= '0' && value[i] <= '9') {
			// Find the end of the number
			j := i
			for j < len(value) && (value[j] == '.' || value[j] >= '0' && value[j] <= '9') {
				j++
			}
			return value[i:j]
		}
	}

	return ""
}

// calculateMinMax calculates min and max from a slice of float64 values
func calculateMinMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}

	min := values[0]
	max := values[0]

	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	return min, max
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, cpuElements := cpuinfo.GetInformations()
		var gpuElements map[string]string
		if nvidia.Available() {
			_, gpuElements = nvidia.GetInformations()
		} else {
			_, gpuElements = amd.GetInformations()
		}

		// Update history and calculate min/max values
		cpuMinValues, cpuMaxValues := updateHistoryAndCalculateMinMax(cpuHistory, cpuElements)
		gpuMinValues, gpuMaxValues := updateHistoryAndCalculateMinMax(gpuHistory, gpuElements)

		// Filter CPU keys to only show those with available values
		var filteredCpuKeys []shared.KeyTemplate
		for _, key := range cpuTemplates {
			if value, exists := cpuElements[key.Key]; exists && value != "" {
				filteredCpuKeys = append(filteredCpuKeys, key)
			}
		}

		// Filter GPU keys to only show those with available values
		var filteredGpuKeys []shared.KeyTemplate
		for _, key := range gpuTemplates {
			if value, exists := gpuElements[key.Key]; exists && value != "" {
				filteredGpuKeys = append(filteredGpuKeys, key)
			}
		}

		data := Data{
			CpuKeys:      filteredCpuKeys,
			GpuKeys:      filteredGpuKeys,
			CpuValues:    cpuElements,
			GpuValues:    gpuElements,
			CpuMinValues: cpuMinValues,
			CpuMaxValues: cpuMaxValues,
			GpuMinValues: gpuMinValues,
			GpuMaxValues: gpuMaxValues,
		}

		err := templateCache.ExecuteTemplate(w, "index.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	fs := http.FileServer(http.Dir("website/resources"))
	http.Handle("/resources/", http.StripPrefix("/resources/", fs))

	fmt.Println("Starting server at port 8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
