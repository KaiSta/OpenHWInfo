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

	// Additional CPU templates. The per-core clock rows ("cpu.clockN") and the
	// per-CCD temperature rows ("tccdN") have no fixed templates: their count
	// depends on the CPU, so main.go generates them from the collected values.
	CPUTemp    = KeyTemplate{"CPU Temperature", "cpu.temp"}
	CPUPCHTemp = KeyTemplate{"CPU PCH Temperature", "cpu.pch"}
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
