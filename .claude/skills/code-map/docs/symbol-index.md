# Symbol index (generated)

Generated from the source by `make map`; `go test ./...` fails when this file
and the tree disagree, so an edit here is lost on the next run. Line numbers
for any symbol in this project come from this file, or from a search.

Document sections are deliberately absent -- they move with every prose edit:
`grep -n '^## ' README.md CODE_REVIEW.md`. Tests are listed in `test-index.md`.

### amd/amd.go — 293 lines
- funcs: probeAMD 27, getGPUName 44, resolveAMDGPUName 52, cleanGPUName 88, disambiguateGPUName 135, collectGPUDataForCard 161, GetAllInformations 225, parseTemperatureValue 251, parseMemoryValue 270, parseGPUUsage 282
- types: gpuVariant 117
- consts: amdVendorID 18
- vars: sysDir 15, revSuffixRe 74, vendorPrefixes 78, chipsetVariants 125

### battery/battery.go — 373 lines
- funcs: batteryInfo.reported 71, GetInformations 78, powerSupplies 117, isSystemBattery 138, externalPowerState 149, deliversMains 180, readBattery 186, evaluateHealth 285, readText 325, readNumber 334, readBool 347, addReading 365
- types: supply 39, batteryInfo 49
- consts: supplyDir 28, healthWarnPercent 34, healthBadPercent 35
- vars: sysDir 24

### cpuinfo/cpu.go — 391 lines
- funcs: isCPUHwmonChip 45, GetInformations 53, numberedTemperatures 90, temperatureSensorIndex 113, cpuHwmonChipsInOrder 138, cpuTemperatures 162, setHottestPackageTemp 234, coreNumberFromLabel 245, milliCelsius 259, firstFieldAfterColon 269, parseCpuInfo 282
- types: hwmonSensor 81, cpuHwmonChip 127
- consts: maxTccdSensors 27
- vars: cpuVendorIDKey 15, cpuBrandKey 16, cpuModelKey 17, sysDir 22, cpuHwmonChips 31

### intel/intel.go — 326 lines
- funcs: probeIntel 22, getPCIDeviceID 36, getGPUName 49, resolveIntelGPUName 61, parseIntelGPUName 129, firstLine 172, looksLikeDeviceName 191, getIntelDeviceModel 201, cleanPCIDeviceName 215, getDriverVersion 229, resolveIntelDriverVersion 233, collectGPUDataForCard 247, GetAllInformations 292, addTemperature 318
- consts: intelVendorID 18, hexLetters 183
- vars: sysDir 15

### intel/pci_ids.go — 214 lines
- vars: intelGraphicsModels 23, intelCodenameOnlyModels 210

### main.go — 618 lines
- funcs: cpuKeyTemplates 79, indexedCPUTemplates 95, numericSuffixes 109, boardKeyTemplates 130, labelledBoardTemplates 150, diskKeyTemplates 214, newHistoryStore 283, historyStore.updateAndCalculate 287, NewApp 309, App.startup 314, App.shutdown 319, App.GetData 325, gpuViews 422, deviceView 439, updateHistoryAndCalculateMinMax 481, calculateMinMax 521, formatValue 545, withUnit 556, withUnits 573, main 581
- types: DeviceView 43, Data 54, historyStore 278, App 304
- vars: assets 32, appIcon 37, gpuTemplates 167, diskTemplates 193, memTemplates 224, batteryTemplates 251, cpuHistory 295, gpuHistory 296, diskHistory 297, boardHistory 298, memoryHistory 299, batteryHistory 300

### mainboard/mainboard.go — 322 lines
- funcs: GetInformations 69, isBoardZone 103, addThermalZones 117, isBoardMonitorChip 161, addBoardMonitorChips 173, numberedSensors 220, sensorIndex 243, addTemperature 262, addFan 278, milliCelsius 293, sortByTrailingNumber 303, trailingNumber 312
- types: boardSensor 211
- consts: dmiDir 30, maxBoardTemps 36, maxBoardFans 37, tempKeyPrefix 45, tempLabelKeyPrefix 46, fanKeyPrefix 47, fanLabelKeyPrefix 48
- vars: sysDir 27, dmiFields 53, zoneTypeRe 96, boardMonitorChipPrefixes 143, trailingNumberRe 309

### memory/memory.go — 400 lines
- funcs: GetInformations 49, availableMemory 110, addSize 123, addPercent 129, readMeminfo 140, addPressure 167, pressureKey 192, psiValue 203, addZram 235, readZram 298, readZramDevice 324, readZramAlgorithm 351, readNumbers 369, allDigits 390
- types: zramDevice 221
- consts: bytesPerGiB 32, bytesPerMiB 37
- vars: meminfoPath 25, psiPath 26, zramPath 27

### nvidia/nvidia.go — 182 lines
- funcs: queryGPUs 48, parseGPUOutput 61, parseFloatField 75, parseCSVFields 94, parseGPURecord 105, GetAllInformations 158
- consts: gpuIndexColumn 40
- vars: gpuQueryFields 22

### shared/cache.go — 46 lines
- funcs: CachedProbe 23, ResetProbeCache 42
- vars: probeCacheMu 11, probeCache 12

### shared/debug.go — 31 lines
- funcs: SetDebug 15, DebugEnabled 20, Debugf 26
- vars: debug 12

### shared/drm.go — 48 lines
- funcs: DRMCardDevicePath 16, ProbeDRMCards 23, ResolvePCIDevice 41
- consts: maxDRMCards 12

### shared/hardware.go — 212 lines
- funcs: HardwareUnit.Suffix 66, HardwareData.HasTelemetry 154, NewHardwareValue 167, HardwareValue.GetNumericValue 176, HardwareValue.IsNumericType 210
- types: HardwareUnit 11, HardwareValue 102, HardwareValueType 109, DeviceStatus 126, HardwareData 143
- consts: UnknownUnit 14, Celsius 15, MHz 16, Percent 17, Watt 18, Megabyte 19, RPM 20, Gigabyte 24, Terabyte 28, Hour 30, Gibibyte 36, WattHour 42, AmpereHour 43, Volt 45, Ratio 50, Unknown 112, Temperature 113, Frequency 114, Usage 115, Power 116, Memory 117, StatusGood 136, StatusWarn 137, StatusBad 138, StatusUnknown 139

### shared/sysutil.go — 84 lines
- funcs: RunShell 14, RunCommand 26, FindFirstSubpath 38, ReadSysFile 50, ParseFrequencyMHz 60, ParsePower 75

### shared/templates.go — 169 lines
- types: KeyTemplate 4
- vars: CPUVendorID 11, CPUBrand 12, CPUModel 13, CPUTemp 18, CPUPCHTemp 19, BoardVendor 24, BoardModel 25, BoardVersion 26, BoardSerial 27, BoardBIOSVersion 28, BoardBIOSDate 29, DiskModel 42, DiskSerial 43, DiskFirmware 44, DiskMedium 45, DiskSize 46, DiskFSTotal 47, DiskFSUsed 48, DiskFSFree 49, DiskSmartLife 56, DiskSmartWritten 57, DiskSmartPowerOn 58, DiskSmartBadSectors 59, DiskSmartSpare 60, DiskSmartMediaErr 61, DiskSmartSelftest 62, DiskSmartWarnings 63, MemTotal 72, MemUsed 73, MemAvailable 74, MemUsage 75, SwapTotal 77, SwapUsed 78, SwapUsage 79, ZramAlgorithm 94, ZramDevices 95, ZramSize 96, ZramUsage 97, ZramMemory 98, ZramRatio 99, MemPressureSome 105, MemPressureFull 106, BatteryStatus 120, BatteryCharge 121, BatteryHealth 122, BatteryEnergyNow 124, BatteryEnergyFull 125, BatteryEnergyDesign 126, BatteryChargeNow 128, BatteryChargeFull 129, BatteryChargeDesign 130, BatteryTimeEmpty 135, BatteryTimeFull 136, BatteryPower 138, BatteryVoltage 139, BatteryDesignVoltage 140, BatteryCycles 141, BatteryTechnology 142, BatteryModel 143, BatteryManufacturer 144, BatterySerial 145, BatteryAC 148, GPUName 153, GPUDriverVersion 154, GPUTemp 157, GPUHotspotTemp 158, VRAMTemp 159, GPUFrequency 160, VRAMLrequency 161, VRAMTotal 162, VRAMUsed 163, VRAMVendor 164, GPUUsage 165, GPUPowerMax 166, GPUPowerAvg 167, FanSpeed 168

### storage/smart.go — 983 lines
- funcs: fetchSmart 131, refreshSmartEntry 159, smartInfo.withCachedAttributeReadings 195, smartInfo.clearAttributeReadings 218, readSmartProperties 231, driveObjectPath 280, interfaceProperties 305, smartUpdate 316, ataSmartInfo 324, nvmeSmartInfo 354, readSmartAttributes 406, writtenFromAttributes 500, lifeFromAttributes 526, applyNvmeAttributes 541, noWakeup 563, isLifeAttribute 567, lifeUsedFrom 584, evaluateHealth 621, describeHealthy 735, max0 760, nvmeWarningText 769, selftestFinding 790, selftestText 805, variantBool 825, variantString 834, variantStringSlice 843, variantFloat 852, variantIntOr 857, variantInt64 867, variantUint64Or 875, variantByte 885, variantUint64 895, variantNumber 905, addSmart 936
- types: smartInfo 66, smartCacheEntry 119, ataAttribute 376, finding 611
- consts: udisksService 32, ataInterface 33, nvmeInterface 34, smartPropsTTL 41, smartAttrsTTL 45, smartCommandTimeout 49, lifeWarnPercent 110, lifeBadPercent 111, defaultSpareThreshold 114, ataTotalLbasWrittenID 453, ataUnitUnknown 464, ataUnitNone 465, ataUnitMseconds 466, ataUnitSectors 467, ataUnitMkelvin 468, ataUnitSmallPct 469, ataUnitPercent 470, ataUnitMB 471, ataMBUnitSectors 481, severityNote 606, severityWarn 607, severityBad 608
- vars: smartCacheMu 55, smartCache 56, smartProvider 62, lifeAttributes 396

### storage/statfs_linux.go — 27 lines
- vars: filesystemUsage 14

### storage/statfs_other.go — 10 lines
- vars: filesystemUsage 8

### storage/storage.go — 563 lines
- funcs: GetInformations 74, physicalDrives 120, hasVirtualPrefix 142, mediumOf 156, driveSizeGB 172, addFilesystemUsage 188, bytesToGB 209, bytesToTB 217, filesystemUsageByDrive 236, resolveDevice 286, driveNameFor 297, isWholeDisk 315, partitionParent 327, allDigits 348, unescapeMountField 362, addTemperatures 391, findDriveHwmonDir 428, numberedSensors 461, tempSensorIndex 484, addTemperature 498, milliCelsius 512, getUDisksConn 534
- types: drive 65, fsUsage 222, driveSensor 451
- consts: maxDiskTemps 45, tempKeyPrefix 51, tempLabelKeyPrefix 52, udisksTimeout 532
- vars: sysDir 40, mountsPath 41, virtualDevicePrefixes 61, udisksConnMu 523, udisksConn 524, udisksConnErr 525, udisksConnErrTime 526

### website/app.js

- declarations: num 6, frac 12, temperatureLimit 24, betterHigher 33, gauge 43, keyOf 97, labelsOf 103, makeEl 109, reconcile 122, cardHead 136, statusPill 157, setStatusPill 163, updateStatus 182, buildCardShell 195, buildMetricRow 214, setBar 232, updateMetricRow 248, updateMetrics 262, isClockKey 274, isCoreTempKey 278, trailingIndex 282, tempSeverity 288, coreSection 294, coreRange 321, clockPeak 335, tempLayout 352, tempForClock 365, buildCoreGrid 381, setValueLine 504, setRange 524, cellsByKey 537, updateClocks 545, updateTemps 590, render 620, POLL_MS 841, getData 843, timer 854, tick 856, startPolling 870, stopPolling 876, updatePauseBadge 882
