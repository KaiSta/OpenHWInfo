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

	// Zram is the compressed block device the kernel builds in RAM, which on a
	// laptop is usually what swap lives on. These rows describe the compressed
	// store itself, not what is inside it: how much data the machine has swapped
	// out is already reported by the swap rows above, and the difference between
	// the two is the point of zram — a machine can have gigabytes swapped and
	// only a few hundred megabytes of RAM spent on it.
	//
	// Zram Memory is the RAM the compressed data actually occupies, which is not
	// the same figure as the data's compressed size: the kernel's number includes
	// allocator fragmentation and per-page metadata. Zram Ratio compares the data
	// stored against its compressed size, and Zram Devices is listed only when the
	// kernel has more than one device configured, since the other rows are then
	// sums rather than one device's own figures.
	ZramAlgorithm = KeyTemplate{"Zram Algorithm", "zram.algorithm"}
	ZramDevices   = KeyTemplate{"Zram Devices", "zram.devices"}
	ZramSize      = KeyTemplate{"Zram Size", "zram.size"}
	ZramUsage     = KeyTemplate{"Zram Usage", "zram.usage"}
	ZramMemory    = KeyTemplate{"Zram Memory", "zram.memory"}
	ZramRatio     = KeyTemplate{"Zram Ratio", "zram.ratio"}

	// Pressure is the kernel's own stall accounting: the share of the last 10
	// seconds during which tasks were stalled waiting for memory. "some" counts
	// any stalled task, "full" only windows in which every task was stalled, so
	// "full" is the stronger signal of a machine that is actually struggling.
	MemPressureSome = KeyTemplate{"Memory Pressure (some)", "mem.pressure.some"}
	MemPressureFull = KeyTemplate{"Memory Pressure (full)", "mem.pressure.full"}
)

// Battery template variables. A battery card is one card per battery, so these
// keys repeat across cards the way a drive's do; each card carries its own
// values and its own history, and a row is listed only by the battery that
// reports it.
//
// The amount of energy a battery holds arrives in one of two quantities
// depending on its firmware — energy (Wh) or charge (Ah) — so both families have
// rows and only the one the battery reports is ever filled. The unit on the
// reading then says which it is, instead of a single "Capacity" row quietly
// changing meaning between machines.
var (
	BatteryStatus = KeyTemplate{"Status", "battery.status"}
	BatteryCharge = KeyTemplate{"Charge", "battery.capacity"}
	BatteryHealth = KeyTemplate{"Health", "battery.health"}

	BatteryEnergyNow    = KeyTemplate{"Energy Now", "battery.energy.now"}
	BatteryEnergyFull   = KeyTemplate{"Energy Full", "battery.energy.full"}
	BatteryEnergyDesign = KeyTemplate{"Energy Design", "battery.energy.design"}

	BatteryChargeNow    = KeyTemplate{"Charge Now", "battery.charge.now"}
	BatteryChargeFull   = KeyTemplate{"Charge Full", "battery.charge.full"}
	BatteryChargeDesign = KeyTemplate{"Charge Design", "battery.charge.design"}

	// Time to empty or full is derived from the energy the battery holds and the
	// power it is moving, so it is only reported while the battery is actually
	// discharging or charging — a full battery on the mains is neither.
	BatteryTimeEmpty = KeyTemplate{"Time to Empty", "battery.time.empty"}
	BatteryTimeFull  = KeyTemplate{"Time to Full", "battery.time.full"}

	BatteryPower         = KeyTemplate{"Power Draw", "battery.power"}
	BatteryVoltage       = KeyTemplate{"Voltage", "battery.voltage"}
	BatteryDesignVoltage = KeyTemplate{"Design Voltage", "battery.voltage.design"}
	BatteryCycles        = KeyTemplate{"Charge Cycles", "battery.cycles"}
	BatteryTechnology    = KeyTemplate{"Technology", "battery.technology"}
	BatteryModel         = KeyTemplate{"Model", "battery.model"}
	BatteryManufacturer  = KeyTemplate{"Manufacturer", "battery.manufacturer"}
	BatterySerial        = KeyTemplate{"Serial Number", "battery.serial"}
	// BatteryAC is the state of the external supply, which is what makes a
	// battery that is neither charging nor discharging readable.
	BatteryAC = KeyTemplate{"AC Adapter", "battery.ac"}
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
