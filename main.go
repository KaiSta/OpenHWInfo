package main

import (
	"fmt"
	"html/template"
	"net/http"

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
func updateHistoryAndCalculateMinMax(history map[string][]float64, currentValues map[string]shared.HardwareValue) (map[string]string, map[string]string) {
	minValues := make(map[string]string)
	maxValues := make(map[string]string)

	// Update history with current values and calculate min/max
	for key, value := range currentValues {
		if value.Value == "" {
			continue
		}

		// Check if this value is numeric type that can be used for min/max calculation
		if !value.IsNumericType() {
			continue
		}

		// Extract the numeric value directly from the HardwareValue
		num, err := value.GetNumericValue()
		if err != nil {
			// Log error but continue processing other values
			fmt.Printf("Error parsing numeric value for %s: %v\n", key, err)
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
		cpuData := cpuinfo.GetInformations()
		var gpuData shared.HardwareData
		if nvidia.Available() {
			gpuData = nvidia.GetInformations()
		} else {
			gpuData = amd.GetInformations()
		}

		// Update history and calculate min/max values
		cpuMinValues, cpuMaxValues := updateHistoryAndCalculateMinMax(cpuHistory, cpuData.Values)
		gpuMinValues, gpuMaxValues := updateHistoryAndCalculateMinMax(gpuHistory, gpuData.Values)

		// Filter CPU keys to only show those with available values
		var filteredCpuKeys []shared.KeyTemplate
		for _, key := range cpuTemplates {
			if value, exists := cpuData.Values[key.Key]; exists && value.Value != "" {
				filteredCpuKeys = append(filteredCpuKeys, key)
			}
		}

		// Filter GPU keys to only show those with available values
		var filteredGpuKeys []shared.KeyTemplate
		for _, key := range gpuTemplates {
			if value, exists := gpuData.Values[key.Key]; exists && value.Value != "" {
				filteredGpuKeys = append(filteredGpuKeys, key)
			}
		}

		// Create a map of string values for display purposes
		cpuStringValues := make(map[string]string)
		for key, value := range cpuData.Values {
			// Convert interface{} to string properly
			if valStr, ok := value.Value.(string); ok {
				cpuStringValues[key] = valStr
			} else if valFloat, ok := value.Value.(float64); ok {
				cpuStringValues[key] = fmt.Sprintf("%.2f", valFloat)
			} else {
				cpuStringValues[key] = fmt.Sprintf("%v", value.Value)
			}
		}

		gpuStringValues := make(map[string]string)
		for key, value := range gpuData.Values {
			// Convert interface{} to string properly
			if valStr, ok := value.Value.(string); ok {
				gpuStringValues[key] = valStr
			} else if valFloat, ok := value.Value.(float64); ok {
				gpuStringValues[key] = fmt.Sprintf("%.2f", valFloat)
			} else {
				gpuStringValues[key] = fmt.Sprintf("%v", value.Value)
			}
		}

		data := Data{
			CpuKeys:      filteredCpuKeys,
			GpuKeys:      filteredGpuKeys,
			CpuValues:    cpuStringValues,
			GpuValues:    gpuStringValues,
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
