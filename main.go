package main

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/KaiSta/OpenHWInfo/amd"
	"github.com/KaiSta/OpenHWInfo/cpuinfo"
	"github.com/KaiSta/OpenHWInfo/intel"
	"github.com/KaiSta/OpenHWInfo/nvidia"
	"github.com/KaiSta/OpenHWInfo/shared"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	assetserver "github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// assets embeds the frontend (website/) so production builds are fully
// self-contained and do not depend on files next to the binary.
//
//go:embed all:website
var assets embed.FS

// GPUView holds the data for a single GPU in the frontend.
type GPUView struct {
	Name      string
	Keys      []shared.KeyTemplate
	Values    map[string]string
	MinValues map[string]string
	MaxValues map[string]string
}

type Data struct {
	CpuKeys      []shared.KeyTemplate
	GpuViews     []GPUView
	CpuValues    map[string]string
	CpuMinValues map[string]string
	CpuMaxValues map[string]string
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

// gpuHistoryKey builds a unique history key for a metric belonging to a
// specific GPU.  The prefix ensures that min/max tracking for GPU 0 does not
// interfere with GPU 1, etc.
func gpuHistoryKey(gpuIndex int, metricKey string) string {
	return fmt.Sprintf("gpu%d.%s", gpuIndex, metricKey)
}

// Global maps to track historical values for min/max calculations
var cpuHistory = make(map[string][]float64)
var gpuHistory = make(map[string][]float64)

// App is the main application struct.
type App struct {
	ctx context.Context
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is run at app startup. It sets the ctx for later use.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown is run at app exit.
func (a *App) shutdown(_ context.Context) {
	fmt.Println("Shutting down")
}

// GetData collects current CPU and GPU data, updates the min/max history and
// returns a snapshot for the frontend to render.
func (a *App) GetData() Data {
	// Collect CPU data
	cpuData := cpuinfo.GetInformations()

	// Collect GPU data from all available sources
	var allGPUs []shared.HardwareData
	usedGPU := false

	if nvidia.Available() {
		allGPUs = nvidia.GetAllInformations()
		usedGPU = true
	}
	if intel.Available() {
		allGPUs = append(allGPUs, intel.GetAllInformations()...)
		usedGPU = true
	}
	if amd.Available() {
		allGPUs = append(allGPUs, amd.GetAllInformations()...)
		usedGPU = true
	}

	// If no GPU was found, add a placeholder
	if !usedGPU {
		allGPUs = append(allGPUs, shared.HardwareData{Name: "No GPU", Values: make(map[string]shared.HardwareValue), IsGPU: true})
	}

	// Build per-GPU view data
	var gpuViews []GPUView
	for gpuIdx, gpuData := range allGPUs {
		// Filter keys to only show those with available values
		var filteredKeys []shared.KeyTemplate
		for _, key := range gpuTemplates {
			if value, exists := gpuData.Values[key.Key]; exists && value.Value != "" {
				filteredKeys = append(filteredKeys, key)
			}
		}

		// Build string values map
		stringValues := make(map[string]string)
		for key, value := range gpuData.Values {
			stringValues[key] = formatValue(value)
		}

		// Build a subset of values that use prefixed keys for per-GPU history tracking.
		prefixedValues := make(map[string]shared.HardwareValue)
		for k, v := range gpuData.Values {
			if v.IsNumericType() {
				prefixedKey := gpuHistoryKey(gpuIdx, k)
				prefixedValues[prefixedKey] = v
			}
		}

		// Calculate min/max using prefixed history keys (so each GPU's
		// history is tracked independently), then remap them back to the
		// plain metric keys that the frontend uses for lookups.
		minValues, maxValues := updateHistoryAndCalculateMinMax(gpuHistory, prefixedValues)

		remap := func(src map[string]string) map[string]string {
			dst := make(map[string]string, len(src))
			for k, v := range src {
				if strings.HasPrefix(k, "gpu"+fmt.Sprint(gpuIdx)+".") {
					dst[strings.TrimPrefix(k, "gpu"+fmt.Sprint(gpuIdx)+".")] = v
				}
			}
			return dst
		}
		minValues = remap(minValues)
		maxValues = remap(maxValues)

		gpuViews = append(gpuViews, GPUView{
			Name:      gpuData.Name,
			Keys:      filteredKeys,
			Values:    stringValues,
			MinValues: minValues,
			MaxValues: maxValues,
		})
	}

	// Filter CPU keys to only show those with available values
	var filteredCpuKeys []shared.KeyTemplate
	for _, key := range cpuTemplates {
		if value, exists := cpuData.Values[key.Key]; exists && value.Value != "" {
			filteredCpuKeys = append(filteredCpuKeys, key)
		}
	}

	// Create a map of string values for display purposes
	cpuStringValues := make(map[string]string)
	for key, value := range cpuData.Values {
		cpuStringValues[key] = formatValue(value)
	}

	// Update history and calculate min/max values for CPU
	cpuMinValues, cpuMaxValues := updateHistoryAndCalculateMinMax(cpuHistory, cpuData.Values)

	return Data{
		CpuKeys:      filteredCpuKeys,
		GpuViews:     gpuViews,
		CpuValues:    cpuStringValues,
		CpuMinValues: cpuMinValues,
		CpuMaxValues: cpuMaxValues,
	}
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

// formatValue converts a HardwareValue to a display string.
func formatValue(value shared.HardwareValue) string {
	if valStr, ok := value.Value.(string); ok {
		return valStr
	}
	if valFloat, ok := value.Value.(float64); ok {
		return fmt.Sprintf("%.2f", valFloat)
	}
	return fmt.Sprintf("%v", value.Value)
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "OpenHWInfo — Hardware Monitor",
		Width:            1180,
		Height:           760,
		MinWidth:         560,
		MinHeight:        400,
		StartHidden:      false,
		BackgroundColour: options.NewRGB(11, 15, 23), // #0b0f17
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
	})
	if err != nil {
		println("error:", err.Error())
	}
}
