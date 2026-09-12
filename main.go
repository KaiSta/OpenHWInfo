package main

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/KaiSta/OpenHWInfo/amd"
	"github.com/KaiSta/OpenHWInfo/cpuinfo"
	"github.com/KaiSta/OpenHWInfo/intel"
	"github.com/KaiSta/OpenHWInfo/mainboard"
	"github.com/KaiSta/OpenHWInfo/nvidia"
	"github.com/KaiSta/OpenHWInfo/shared"
	"github.com/KaiSta/OpenHWInfo/storage"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	assetserver "github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// assets embeds the frontend (website/) so production builds are fully
// self-contained and do not depend on files next to the binary.
//
//go:embed all:website
var assets embed.FS

// appIcon is the window/panel icon, embedded so it ships inside the binary.
//
//go:embed icons/icon-512.png
var appIcon []byte

// DeviceView holds the data for one card in the frontend: a GPU, a drive or the
// mainboard. They share a shape because they share a presentation — a name for
// the card title, and a metric table whose rows depend on the device, so the
// frontend renders all three with the same code.
type DeviceView struct {
	Name      string
	Keys      []shared.KeyTemplate
	Values    map[string]string
	MinValues map[string]string
	MaxValues map[string]string
}

type Data struct {
	CpuKeys      []shared.KeyTemplate
	GpuViews     []DeviceView
	DiskViews    []DeviceView
	CpuValues    map[string]string
	CpuMinValues map[string]string
	CpuMaxValues map[string]string
	// Board is nil when the board reports nothing at all (no SMBIOS and no board
	// sensors), which the frontend renders as no card.
	Board *DeviceView
}

// cpuKeyTemplates assembles the CPU rows in display order: identity fields, one
// row per core, package temperatures, then per-CCD temperatures. The clock and
// Tccd rows are generated from the collected values so that no core or sensor
// is hidden behind a fixed-size template list — a 16-core CPU shows 16 clock
// rows rather than the first four.
func cpuKeyTemplates(values map[string]shared.HardwareValue) []shared.KeyTemplate {
	templates := []shared.KeyTemplate{
		shared.CPUVendorID,
		shared.CPUBrand,
		shared.CPUModel,
	}
	templates = append(templates, indexedCPUTemplates(values, "cpu.clock", "CPU Clock %d")...)
	templates = append(templates, shared.CPUTemp, shared.CPUPCHTemp)
	templates = append(templates, indexedCPUTemplates(values, "cpu.coreTemp", "Core %d Temperature")...)
	templates = append(templates, indexedCPUTemplates(values, "tccd", "Tccd %d Temperature")...)
	return templates
}

// indexedCPUTemplates returns one template per collected key of the form
// prefix + <number>, ordered by that number. labelFormat receives the index,
// e.g. "CPU Clock %d" yields "CPU Clock 3".
func indexedCPUTemplates(values map[string]shared.HardwareValue, prefix, labelFormat string) []shared.KeyTemplate {
	var templates []shared.KeyTemplate
	for _, index := range numericSuffixes(values, prefix) {
		templates = append(templates, shared.KeyTemplate{
			Label: fmt.Sprintf(labelFormat, index),
			Key:   prefix + strconv.Itoa(index),
		})
	}
	return templates
}

// numericSuffixes collects the numeric suffix of every value key shaped like
// prefix + <digits>, sorted ascending, so display order follows core/sensor
// order rather than Go's map iteration order.
func numericSuffixes(values map[string]shared.HardwareValue, prefix string) []int {
	var indices []int
	for key := range values {
		suffix, ok := strings.CutPrefix(key, prefix)
		if !ok || suffix == "" {
			continue
		}
		index, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}
		indices = append(indices, index)
	}
	sort.Ints(indices)
	return indices
}

// boardKeyTemplates assembles the mainboard rows in display order: the identity
// and firmware fields, then one row per temperature and one per fan. Both sensor
// families are generated from the collected values, because how many there are
// depends on the board's ACPI zones and its Super I/O chip.
func boardKeyTemplates(values map[string]shared.HardwareValue) []shared.KeyTemplate {
	templates := []shared.KeyTemplate{
		shared.BoardVendor,
		shared.BoardModel,
		shared.BoardVersion,
		shared.BoardSerial,
		shared.BoardBIOSVersion,
		shared.BoardBIOSDate,
	}
	templates = append(templates, labelledBoardTemplates(values, "board.temp", "board.tempLabel", "Temperature %d")...)
	templates = append(templates, labelledBoardTemplates(values, "board.fan", "board.fanLabel", "Fan %d Speed")...)
	return templates
}

// labelledBoardTemplates returns one template per collected key of the form
// keyPrefix + <number>, ordered by that number. The row label is the sensor's
// own name, which the collector stores under labelPrefix + <number> because only
// it knows what the number means ("SYSTIN", "acpitz", "Chassis Fan",
// "Composite" for an NVMe drive). Those label keys are data rather than metrics:
// no template names them, so they never reach the table.
func labelledBoardTemplates(values map[string]shared.HardwareValue, keyPrefix, labelPrefix, fallbackFormat string) []shared.KeyTemplate {
	var templates []shared.KeyTemplate
	for _, index := range numericSuffixes(values, keyPrefix) {
		label := fmt.Sprintf(fallbackFormat, index)
		if stored, ok := values[labelPrefix+strconv.Itoa(index)]; ok {
			if name, ok := stored.Value.(string); ok && name != "" {
				label = name
			}
		}
		templates = append(templates, shared.KeyTemplate{
			Label: label,
			Key:   keyPrefix + strconv.Itoa(index),
		})
	}
	return templates
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

// diskTemplates are the fixed rows of a drive card. The per-drive temperature
// rows ("disk.tempN") are generated from the collected values, because an NVMe
// drive reports a composite sensor plus one per temperature sensor.
var diskTemplates = []shared.KeyTemplate{
	shared.DiskModel,
	shared.DiskSerial,
	shared.DiskFirmware,
	shared.DiskMedium,
	shared.DiskSize,
	shared.DiskFSTotal,
	shared.DiskFSUsed,
	shared.DiskFSFree,
}

// diskKeyTemplates assembles a drive card's rows: its identity and size, how
// full its filesystems are, then its temperature sensors.
func diskKeyTemplates(values map[string]shared.HardwareValue) []shared.KeyTemplate {
	templates := append([]shared.KeyTemplate{}, diskTemplates...)
	return append(templates, labelledBoardTemplates(values, "disk.temp", "disk.tempLabel", "Drive Temperature %d")...)
}

// Global maps to track historical values for min/max calculations
var cpuHistory = make(map[string][]float64)
var gpuHistory = make(map[string][]float64)
var diskHistory = make(map[string][]float64)
var boardHistory = make(map[string][]float64)

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

	// Collect GPU data from every vendor. Each vendor probes once and returns nil
	// when it has nothing to report, so there is no separate availability check
	// (which would run lspci or scan sysfs a second time on every poll).
	var allGPUs []shared.HardwareData
	allGPUs = append(allGPUs, nvidia.GetAllInformations()...)
	allGPUs = append(allGPUs, intel.GetAllInformations()...)
	allGPUs = append(allGPUs, amd.GetAllInformations()...)

	// If no GPU was found, add a placeholder
	if len(allGPUs) == 0 {
		allGPUs = append(allGPUs, shared.HardwareData{Name: "No GPU", Values: make(map[string]shared.HardwareValue)})
	}

	// Build per-GPU view data. The history is scoped per card so that GPU 0's
	// range never mixes with GPU 1's.
	var gpuViews []DeviceView
	for gpuIdx, gpuData := range allGPUs {
		gpuViews = append(gpuViews, deviceView(gpuData, gpuTemplates, gpuHistory, fmt.Sprintf("gpu%d.", gpuIdx)))
	}

	// Each drive is its own card as well, and keeps its own history for the same
	// reason: two drives have no shared range.
	var diskViews []DeviceView
	for diskIdx, diskData := range storage.GetInformations() {
		diskViews = append(diskViews, deviceView(diskData, diskKeyTemplates(diskData.Values), diskHistory, fmt.Sprintf("disk%d.", diskIdx)))
	}

	// The mainboard is a single device, so its history needs no prefix.
	var board *DeviceView
	if boardData := mainboard.GetInformations(); len(boardData.Values) > 0 {
		view := deviceView(boardData, boardKeyTemplates(boardData.Values), boardHistory, "")
		if len(view.Keys) > 0 {
			board = &view
		}
	}

	// Filter CPU keys to only show those with available values
	var filteredCpuKeys []shared.KeyTemplate
	for _, key := range cpuKeyTemplates(cpuData.Values) {
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
		DiskViews:    diskViews,
		CpuValues:    cpuStringValues,
		CpuMinValues: cpuMinValues,
		CpuMaxValues: cpuMaxValues,
		Board:        board,
	}
}

// deviceView turns one device's collected values into the view the frontend
// renders: the templates that carry a value, the values as display strings, and
// the min/max ranges from that device's own value history. historyPrefix scopes
// the history to the device ("gpu0.", "disk1."), so two cards of the same kind
// never share a range; it is empty for the mainboard, of which there is one.
func deviceView(data shared.HardwareData, templates []shared.KeyTemplate, history map[string][]float64, historyPrefix string) DeviceView {
	view := DeviceView{Name: data.Name}

	// Keep only the rows that carry a value: a device does not have to report
	// every field (a drive with no serial, a card with no hotspot sensor), and an
	// absent reading must not become an empty row.
	for _, key := range templates {
		if value, exists := data.Values[key.Key]; exists && value.Value != "" {
			view.Keys = append(view.Keys, key)
		}
	}

	view.Values = make(map[string]string, len(data.Values))
	for key, value := range data.Values {
		view.Values[key] = formatValue(value)
	}

	// Only numeric values are tracked, and only under this device's prefix, so
	// that the ranges the frontend displays are the device's own.
	scoped := make(map[string]shared.HardwareValue, len(data.Values))
	for key, value := range data.Values {
		if value.IsNumericType() {
			scoped[historyPrefix+key] = value
		}
	}
	minValues, maxValues := updateHistoryAndCalculateMinMax(history, scoped)

	view.MinValues = make(map[string]string, len(minValues))
	for key, value := range minValues {
		view.MinValues[strings.TrimPrefix(key, historyPrefix)] = value
	}
	view.MaxValues = make(map[string]string, len(maxValues))
	for key, value := range maxValues {
		view.MaxValues[strings.TrimPrefix(key, historyPrefix)] = value
	}

	return view
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
		Linux: &linux.Options{
			Icon: appIcon,
			// Deterministic WM_CLASS so Cinnamon/GNOME can match this window to
			// the openhwinfo.desktop entry (which carries the panel/taskbar icon).
			ProgramName: "openhwinfo",
			// Keep Wails' default GPU policy; setting Linux options otherwise
			// changes the default — see wails#2977.
			WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
	})
	if err != nil {
		println("error:", err.Error())
	}
}
