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

// Mainboard template variables
var (
	BoardVendor      = KeyTemplate{"Mainboard Vendor", "board.vendor"}
	BoardModel       = KeyTemplate{"Mainboard Model", "board.model"}
	BoardVersion     = KeyTemplate{"Mainboard Version", "board.version"}
	BoardSerial      = KeyTemplate{"Mainboard Serial", "board.serial"}
	BoardBIOSVersion = KeyTemplate{"BIOS Version", "board.bios.version"}
	BoardBIOSDate    = KeyTemplate{"BIOS Date", "board.bios.date"}

	// As with the CPU, the sensor rows have no fixed templates: a board exposes
	// as many temperature and fan rows as its ACPI zones and Super I/O chip
	// provide, so main.go generates them from the collected values ("board.tempN"
	// and "board.fanN").
)

// Storage template variables. The per-drive temperature rows ("disk.tempN") have
// no fixed templates, for the same reason as the mainboard's: an NVMe drive
// reports a composite sensor plus one per temperature sensor, so main.go
// generates those rows from the collected values.
var (
	DiskModel    = KeyTemplate{"Model", "disk.model"}
	DiskSerial   = KeyTemplate{"Serial Number", "disk.serial"}
	DiskFirmware = KeyTemplate{"Firmware", "disk.firmware"}
	DiskMedium   = KeyTemplate{"Medium", "disk.medium"}
	DiskSize     = KeyTemplate{"Capacity", "disk.size"}
	DiskFSTotal  = KeyTemplate{"Filesystem Total", "disk.fs.total"}
	DiskFSUsed   = KeyTemplate{"Filesystem Used", "disk.fs.used"}
	DiskFSFree   = KeyTemplate{"Filesystem Free", "disk.fs.free"}

	// SMART rows. The unit is carried by the reading itself and rendered from
	// HardwareUnit.Suffix, so these labels name the quantity only: spelling a unit
	// here as well would give a row two places to disagree with its number. Each
	// row is produced only by a drive that reports it, so an HDD shows no wear row
	// and a SATA drive shows no spare-capacity row.
	DiskSmartLife       = KeyTemplate{"Life Used", "disk.smart.life"}
	DiskSmartWritten    = KeyTemplate{"Data Written", "disk.smart.written"}
	DiskSmartPowerOn    = KeyTemplate{"Power-On Time", "disk.smart.hours"}
	DiskSmartBadSectors = KeyTemplate{"Bad Sectors", "disk.smart.badSectors"}
	DiskSmartSpare      = KeyTemplate{"Spare Capacity", "disk.smart.spare"}
	DiskSmartMediaErr   = KeyTemplate{"Media Errors", "disk.smart.mediaErrors"}
	DiskSmartSelftest   = KeyTemplate{"SMART Self-Test", "disk.smart.selftest"}
	DiskSmartWarnings   = KeyTemplate{"SMART Warnings", "disk.smart.warnings"}
)

// Memory template variables. A machine reports the same rows on every poll, so
// these are fixed templates rather than generated ones. The swap rows are only
// collected when swap is configured, and the pressure rows only when the kernel
// provides them; deviceView drops the templates the collector did not fill, so a
// swapless machine shows no swap rows rather than three zeroes.
var (
	MemTotal     = KeyTemplate{"Memory Total", "mem.total"}
	MemUsed      = KeyTemplate{"Memory Used", "mem.used"}
	MemAvailable = KeyTemplate{"Memory Available", "mem.available"}
	MemUsage     = KeyTemplate{"Memory Usage", "mem.usage"}

	SwapTotal = KeyTemplate{"Swap Total", "swap.total"}
	SwapUsed  = KeyTemplate{"Swap Used", "swap.used"}
	SwapUsage = KeyTemplate{"Swap Usage", "swap.usage"}

	// Pressure is the kernel's own stall accounting: the share of the last 10
	// seconds during which tasks were stalled waiting for memory. "some" counts
	// any stalled task, "full" only windows in which every task was stalled, so
	// "full" is the stronger signal of a machine that is actually struggling.
	MemPressureSome = KeyTemplate{"Memory Pressure (some)", "mem.pressure.some"}
	MemPressureFull = KeyTemplate{"Memory Pressure (full)", "mem.pressure.full"}
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
