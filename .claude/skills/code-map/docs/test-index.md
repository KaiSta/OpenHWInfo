# Test index (generated)

Generated from the source by `make map`; `go test ./...` fails when this file
and the tree disagree. Fixture helpers are the functions below `tests:`; the
patterns behind them are described in `testing.md`.

### amd/amd_test.go — 290 lines
- tests: TestGetInformationsWithAMDCard 12, TestParseTemperatureValueKeepsFraction 138, TestParseMemoryValueKeepsFraction 155, TestCleanGPUName 170, TestDisambiguateGPUName 197, TestGetAllInformationsMultipleCards 219
- helpers: assertNumeric 121

### battery/battery_test.go — 608 lines
- tests: TestEnergyBatteryCard 128, TestChargeBatteryUsesChargeUnits 198, TestTwoBatteriesGetTheirOwnCards 243, TestTimeToEmptyOnlyWhileDischarging 275, TestWornBatteryIsWarnedAndWeakBatteryFails 318, TestNoDesignCapacityMeansNoVerdict 362, TestAbsentFieldsProduceNoRows 379, TestUnreadableValuesProduceNoRows 403, TestSuppliesWithoutReadingsAreNotBatteries 435, TestEmptyBatteryBayIsSkipped 448, TestDeviceScopedBatteryIsSkipped 460, TestUnknownScopeIsAccepted 477, TestACAdapterState 487, TestDesktopReportsNothing 553, TestSupplyWithoutTypeIsIgnored 565, TestReadBool 575
- helpers: writeSupplies 14, energyBattery 37, cardNamed 64, numericOf 76, textOf 89, unitOf 102, keysOf 111, assertAbsent 119

### cpuinfo/cpu_test.go — 676 lines
- tests: TestCPUIntelCoretempTemperatures 58, TestCPUAMDK10tempTemperatures 103, TestCPUTccdSensorsAreCapped 135, TestCPUPCHTemperature 155, TestCPUCoreKeysComeFromSensorLabels 183, TestCPUCoreLabelWithoutNumberYieldsNoRow 212, TestCPUTemperaturesOmitPerCoreRowsWhenNumberingIsAmbiguous 238, TestCPUTccdKeysFollowSensorNumberOrder 288, TestCoreNumberFromLabel 309, TestCPUUnlabelledFirstSensorBindsCPUTemp 342, TestCPUUnreadableSensorsAreSkipped 357, TestCPUNoHwmonChips 373, TestGetInformationsKeepsIdentityWithoutCPUHwmonChip 389, TestMilliCelsius 415, TestTemperatureSensorIndex 444, TestParseCpuInfo 480, TestParseCpuInfoMapsThreadsToPhysicalCores 516, TestParseCpuInfoMultiplePackagesOmitTopology 570, TestParseCpuInfoWithoutCoreIDOmitsTopology 598, TestParseCpuInfoUnparsableClockKeepsString 618, TestParseCpuInfoMalformedDoesNotPanic 637, TestFirstFieldAfterColon 659
- helpers: numeric 15, keysOf 29, writeHWMonFixture 39, keysOfHardware 407, itoa 470, tempInputPath 472, tempLabelPath 476

### intel/intel_test.go — 485 lines
- tests: TestGetInformationsWithIntelCard 14, TestGetGPUNameAlwaysNamesTheCard 132, TestGetAllInformationsMultipleCards 163, TestParseIntelGPUName 226, TestCleanPCIDeviceName 287, TestGetIntelDeviceModel 307, TestGetIntelDeviceModelIntegratedIsNotArc 323, TestGetIntelDeviceModelArcIDs 348, TestGetIntelDeviceModelNonGraphicsIDs 374, TestGetIntelDeviceModelAcceptsSysfsForm 384, TestIntelGraphicsModelsIntegrity 396, TestDriverVersionIsCachedPerSysfsRoot 435, TestIntelCodenameOnlyModelsIntegrity 473

### lisuan/lisuan_test.go — 394 lines
- tests: TestGetInformationsReadsTheCardsSensors 86, TestGetInformationsOmitsSensorsTheDriverDoesNotPublish 124, TestHotspotAndMemoryTemperaturesComeFromTheDriverLabels 153, TestTemperatureSensorsPastTheCapAreNotRead 184, TestZeroFanReadingIsReported 208, TestCardWithoutReadingsIsSkipped 225, TestOtherVendorsAreNotCollected 239, TestUnknownDeviceIDIsNamedWithoutClaimingAModel 254, TestDriverVersionComesFromTheCardsOwnModule 280, TestUnboundCardReportsNoDriverVersion 310, TestTwoCardsGetTwoEntries 322, TestLisuanModelsDescribeEveryPublishedDevice 345, TestClassifyTemperatureLabel 376
- helpers: writeFixture 15, useSysDir 34, cardFixture 46, numeric 59, assertAbsent 73

### main_test.go — 994 lines
- tests: TestUpdateHistoryAndCalculateMinMax 15, TestUpdateHistoryAndCalculateMinMaxTracksExtremes 50, TestUpdateHistoryKeepsBoundedWindow 74, TestCPUKeyTemplatesIncludesEveryCore 116, TestCPUKeyTemplatesOrder 135, TestCPUKeyTemplatesIncludesCoreTemperatures 160, TestCPUKeyTemplatesSortsNumerically 174, TestCPUKeyTemplatesIgnoresUnrelatedKeys 191, TestCPUKeyTemplatesTccdLabels 203, TestCPUKeyTemplatesNoClocksOnNonX86 212, TestCPUKeyTemplatesMatchCollectorKeyContract 227, TestBoardKeyTemplatesOrder 280, TestBoardKeyTemplatesUseSensorNames 310, TestBoardKeyTemplatesFallbackLabels 331, TestBoardKeyTemplatesMatchCollectorKeyContract 350, TestBoardHasNoRowsWhenItReportsNothing 385, TestBoardViewKeepsOnlyRowsWithValues 399, TestDiskKeyTemplatesOrder 435, TestDiskKeyTemplatesUseSensorLabels 471, TestDiskKeyTemplatesMatchCollectorKeyContract 493, TestMemTemplatesMatchCollectorKeyContract 551, TestMemTemplatesOrder 602, TestMemoryViewKeepsOnlyReportedRows 624, TestDeviceViewCarriesTheHealthStatus 648, TestDeviceViewScopesHistoryPerDevice 675, TestGPUViewsSkipCardsWithNoReadings 708, TestGPUViewsKeepPerCardRangesWhenACardAppears 734, TestFormatValueAppendsTheUnit 768, TestMinMaxCarryTheSameUnitAsTheReading 800, TestBatteryTemplatesOrder 825, TestBatteryTemplatesMatchCollectorKeyContract 852, TestBatteryViewKeepsOnlyReportedRows 903, TestTwoBatteryViewsKeepSeparateHistories 936, TestUpdateHistorySkipsEmptyValues 978
- helpers: cpuValues 90, labelsOf 98, keysOfTemplates 106, boardValue 268, boardLabel 274, boardViewFor 393, diskSize 959, diskTemp 963, containsString 967

### mainboard/mainboard_test.go — 418 lines
- tests: TestGetInformationsReadsDMIIdentity 83, TestGetInformationsSkipsMissingDMIFields 114, TestGetInformationsWithoutSMIBIOS 131, TestAddThermalZonesSelectsBoardSensors 147, TestAddBoardMonitorChipsReadsSensorsAndFans 170, TestFanIsEligibleForMinMaxTracking 195, TestTemperatureLabelKeysAreCollectedForEveryRow 212, TestFanCountIsCapped 232, TestSensorIndex 252, TestIsBoardZone 275, TestIsBoardMonitorChip 295, TestMilliCelsius 325, TestTrailingNumberOrdersZonesNumerically 347
- helpers: writeSysFixture 17, boardFixture 39, assertTemperature 359, assertFan 389, labelKeyFor 412

### memory/memory_test.go — 645 lines
- consts: sixteenGiB 89, pressureBoth 98, zram8GiB 435, zramDense 436, zramAlgoList 437
- tests: TestGetInformationsReportsMemoryInGiB 135, TestGetInformationsUnitsMatchTheRows 164, TestGetInformationsFormatsSizesWithTheirUnit 190, TestGetInformationsReadsMemoryPressure 205, TestGetInformationsWithoutKernelPSI 222, TestGetInformationsOmitsSwapWhenNotConfigured 240, TestGetInformationsOmitsSwapWhenItsFreeCountIsMissing 262, TestGetInformationsFallsBackWithoutMemAvailable 283, TestGetInformationsOmitsUnreportableUsage 308, TestGetInformationsSkipsUnparsableFields 326, TestGetInformationsWithoutMeminfo 346, TestMeminfoParsing 359, TestPressureMalformedLinesAreIgnored 391, TestZramReportsTheCompressedStore 443, TestZramUnitsMatchTheRows 475, TestZramUnconfiguredDevicesAreSkipped 504, TestZramAggregatesEveryConfiguredDevice 523, TestZramNamesAnAlgorithmOnlyWhenTheDevicesShareOne 550, TestZramAlgorithmWithoutBracketsIsNotGuessed 570, TestZramEmptyStoreHasNoRatio 584, TestZramWithoutUsableCountersOmitsTheAggregates 608, TestZramAbsentOnAMachineWithoutIt 635
- helpers: writeFixture 18, writeZramDevice 50, writeMissingFixture 73, row 103, keysOf 112, number 122

### nvidia/nvidia_test.go — 228 lines
- tests: TestGPUQueryFieldsOrder 14, TestParseGPUOutputSplitsRows 45, TestParseFloatField 62, TestParseCSVFields 98, TestParseCSVFieldsSimpleLine 108, TestParseCSVFieldsMalformedFallsBack 118, TestFieldMappingMatchesQueryColumns 128, TestParseGPURecordOmitsUnavailableFields 182, TestParseGPURecordCommaNameKeepsColumnsAligned 197, TestParseGPURecordTruncatedRow 219

### shared/cache_test.go — 76 lines
- tests: TestCachedProbeComputesOnce 5, TestCachedProbeKeysAreIndependent 28, TestCachedProbeCachesEmptyResult 45, TestResetProbeCacheDropsEntries 63

### shared/debug_test.go — 42 lines
- tests: TestDebugfIsSilentUntilEnabled 10, TestDebugEnabled 31

### shared/hardware_test.go — 58 lines
- tests: TestUnitSuffixNamesEveryUnit 8, TestUnitValuesStayAppendOnly 48

### shared/sysutil_test.go — 194 lines
- tests: TestParseFrequencyMHz 9, TestParsePower 31, TestReadSysFileReportsAbsence 64, TestFindFirstSubpath 96, TestProbeDRMCardsMatchesVendorNotPosition 125, TestProbeDRMCardsSkipsUnreadableVendor 145, TestProbeDRMCardsEmptySysDir 159, TestResolvePCIDevice 165, TestDRMCardDevicePath 190
- helpers: writeCard 114

### storage/smart_test.go — 1097 lines
- tests: TestEvaluateHealthVerdicts 59, TestEvaluateHealthExplainsAGoodVerdict 249, TestEvaluateHealthDoesNotBorrowATAVocabularyForNVMe 269, TestLifeUsedFrom 281, TestLifeFromAttributes 325, TestLifeFromAttributesIgnoresUnknownTables 345, TestLifeAttributeNamesPinTheAllowlist 365, TestWrittenFromAttributes 391, TestWrittenFromAttributesIgnoresOtherCounters 465, TestAtaSmartInfoReadsProperties 476, TestAtaSmartInfoKeepsAbsentBadSectorsAbsent 520, TestAtaSmartInfoUnsupportedDrive 534, TestAtaSmartInfoNeverReadIsNotAHealthyDrive 548, TestNvmeSmartInfoReadsProperties 563, TestNvmeZeroTemperatureIsUnknown 590, TestApplyNvmeAttributes 600, TestApplyNvmeAttributesLeavesAbsentFieldsUnread 633, TestApplyNvmeAttributesDefaultsTheSpareThreshold 643, TestVariantNumberAcceptsEveryWireType 657, TestVariantUint64AndByteRejectOutOfRangeValues 692, TestRefreshSmartEntryKeepsAttributeReadingsBetweenPropertyRefreshes 716, TestRefreshSmartEntryReplacesAttributeReadingsOnSchedule 765, TestRefreshSmartEntryDropsTheCacheWhenTheDriveStopsAnswering 798, TestWithCachedAttributeReadingsInheritsNothingFromAnotherDrive 822, TestClearAttributeReadingsDropsOnlyTheAttributeReadings 856, TestAddSmartEmitsTheExpectedRows 886, TestAddSmartReportsALifetimeWriteCountInTerabytes 947, TestAddSmartOmitsRowsTheDriveDoesNotReport 974, TestAddSmartReportsUnknownWithoutRows 1005, TestAddSmartWithoutSupportGetsNoVerdict 1020, TestAddSmartRowsAllHaveTemplates 1034, TestAddSmartWithAnUnknownDriveIsSilent 1088
- helpers: healthySATA 19, healthyNVMe 38, addSmartFrom 1078

### storage/storage_test.go — 673 lines
- consts: mountsFixture 64
- tests: TestGetInformationsReadsDriveIdentity 191, TestDriveCapacityUsesDecimalGigabytes 230, TestGetInformationsSkipsNonDrives 246, TestGetInformationsWithoutDrives 269, TestNVMeTemperaturesUseTheirSensorLabels 280, TestDrivetempIsAcceptedButOtherChipsAreNot 311, TestFilesystemUsageIsAggregatedPerDrive 339, TestFilesystemUsageIsAbsentWhenTheDriveIsNotMounted 386, TestFilesystemUsageIsAbsentWhenTheMountCannotBeRead 403, TestFilesystemUsageIsAbsentWhenTheMountTableIsMissing 420, TestDriveNameForMapsPartitionsToTheirDisk 436, TestPartitionParent 455, TestUnescapeMountField 479, TestTempSensorIndex 492, TestMilliCelsius 512, TestNVMeFlatHwmonPathIsRecognized 525, TestSATATempViaUDisksFallback 564, TestUDisksFallbackIsOnlyUsedWhenHwmonIsMissing 586, TestUDisksEmptyDoesNotFabricateATemperature 601, TestNVMETempViaUDisksWhenHwmonMissing 619, TestUDisksFallbackIsPerDrive 640
- helpers: sysfsFixture 15, writeSysFixture 75, writeMountsFixture 94, stubUDisks 112, smartTemp 127, stubFilesystemUsage 134, driveByModel 151, textValue 163, numericValue 177, tempLabelKeyFor 667
