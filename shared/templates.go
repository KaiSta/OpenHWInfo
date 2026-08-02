package shared

// KeyTemplate defines the structure for hardware information templates
type KeyTemplate struct {
	Label string
	Key   string
}

// CPU template variables
var (
	CPUVendorID = KeyTemplate{"CPU Vendor ID", "cpu.vendorID"}
	CPUBrand    = KeyTemplate{"CPU Brand", "cpu.brand"}
	CPUModel    = KeyTemplate{"CPU Model", "cpu.model"}

	// Additional CPU templates
	CPUClock0  = KeyTemplate{"CPU Clock", "cpu.clock0"}
	CPUClock1  = KeyTemplate{"CPU Clock 1", "cpu.clock1"}
	CPUClock2  = KeyTemplate{"CPU Clock 2", "cpu.clock2"}
	CPUClock3  = KeyTemplate{"CPU Clock 3", "cpu.clock3"}
	CPUTemp    = KeyTemplate{"CPU Temperature", "cpu.temp"}
	CPUPCHTemp = KeyTemplate{"CPU PCH Temperature", "cpu.pch"}

	// Tccd temperature templates (for arbitrary number of Tccd sensors)
	TCcd0 = KeyTemplate{"Tccd 0 Temperature", "tccd0"}
	TCcd1 = KeyTemplate{"Tccd 1 Temperature", "tccd1"}
	TCcd2 = KeyTemplate{"Tccd 2 Temperature", "tccd2"}
	TCcd3 = KeyTemplate{"Tccd 3 Temperature", "tccd3"}
	TCcd4 = KeyTemplate{"Tccd 4 Temperature", "tccd4"}
	TCcd5 = KeyTemplate{"Tccd 5 Temperature", "tccd5"}
)

// GPU template variables
var (
	GPUName          = KeyTemplate{"GPU Name", "name"}
	GPUDriverVersion = KeyTemplate{"Driver Version", "driverVersion"}

	// Additional GPU templates
	GPUTemp        = KeyTemplate{"GPU Temperature", "gpu.temp"}
	GPUHotspotTemp = KeyTemplate{"GPU Hotspot Temperature", "gpu.hotspot_temp"}
	VRAMTemp       = KeyTemplate{"VRAM Temperature", "vram.temp"}
	GPUFrequency   = KeyTemplate{"GPU Clock", "gpu.frequency"}
	VRAMLrequency  = KeyTemplate{"VRAM Frequency", "vram.frequency"}
	VRAMTotal      = KeyTemplate{"VRAM Total", "vram.total"}
	VRAMUsed       = KeyTemplate{"VRAM Used", "vram.used"}
	VRAMVendor     = KeyTemplate{"VRAM Vendor", "vram.vendor"}
	GPUUsage       = KeyTemplate{"GPU Usage", "gpu.usage"}
	GPUPowerMax    = KeyTemplate{"GPU Power Max", "gpu.power.max"}
	GPUPowerAvg    = KeyTemplate{"GPU Power Avg", "gpu.power.avg"}
	FanSpeed       = KeyTemplate{"Fan Speed", "fan.speed"}
)
