package storage

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// sysfsFixture describes a machine with one SATA SSD, one NVMe SSD and one hard
// disk, plus the devices that must never be reported as drives.
func sysfsFixture() map[string]string {
	return map[string]string{
		// SATA SSD, with the drivetemp driver providing its temperature.
		"block/sda/device/model":                    "SAMSUNG MZ7TE512",
		"block/sda/device/serial":                   "S1NYNSAF803084",
		"block/sda/device/rev":                      "2L06",
		"block/sda/queue/rotational":                "0",
		"block/sda/size":                            "1000215216",
		"block/sda/device/hwmon/hwmon2/name":        "drivetemp",
		"block/sda/device/hwmon/hwmon2/temp1_input": "31000",
		"class/block/sda1/partition":                "1",
		"class/block/sda2/partition":                "2",

		// NVMe SSD: one hwmon chip with a composite sensor and one per sensor.
		"block/nvme0n1/device/model":                    "Samsung SSD 990 PRO 2TB",
		"block/nvme0n1/device/serial":                   "S7KHNJ0W512345",
		"block/nvme0n1/device/firmware_rev":             "4B2QJXD7",
		"block/nvme0n1/queue/rotational":                "0",
		"block/nvme0n1/size":                            "3907029168",
		"block/nvme0n1/device/hwmon/hwmon0/name":        "nvme",
		"block/nvme0n1/device/hwmon/hwmon0/temp1_input": "38000",
		"block/nvme0n1/device/hwmon/hwmon0/temp1_label": "Composite",
		"block/nvme0n1/device/hwmon/hwmon0/temp2_input": "52000",
		"block/nvme0n1/device/hwmon/hwmon0/temp2_label": "Sensor 1",
		"block/nvme0n1/device/hwmon/hwmon0/temp3_input": "48000",
		"class/block/nvme0n1p2/partition":               "2",

		// A hard disk, to prove the medium is read rather than assumed.
		"block/sdb/device/model":     "WDC WD40EFRX-68N",
		"block/sdb/device/rev":       "0A82",
		"block/sdb/queue/rotational": "1",
		"block/sdb/size":             "7814037168",
		"class/block/sdb2/partition": "2",

		// Never drives: a virtual disk, RAM, an optical drive (which does carry a
		// model, so only the device-name rule keeps it out) and a device-mapper
		// volume (which carries no model, so the identity rule keeps it out).
		"block/zram0/queue/rotational": "0",
		"block/loop0/device/model":     "Loopback",
		"block/sr0/device/model":       "HL-DT-ST DVDRAM GU71N",
		"block/sr0/queue/rotational":   "1",
		"block/dm-0/queue/rotational":  "0",
	}
}

// mountsFixture is a mount table shaped like a real one: two filesystems on one
// drive, the same device mounted twice, non-block filesystems, an unmappable
// device-mapper volume, a device that is not in sysfs, and a mount point with an
// escaped space.
const mountsFixture = `sysfs /sys sysfs rw,nosuid 0 0
/dev/sda2 / ext4 rw,relatime 0 0
/dev/sda1 /boot/efi vfat rw,relatime 0 0
/dev/sda2 /mnt/bind ext4 rw,relatime 0 0
/dev/nvme0n1p2 /home ext4 rw,relatime 0 0
tmpfs /run tmpfs rw 0 0
/dev/dm-0 /mnt/encrypted ext4 rw 0 0
/dev/sdc1 /mnt/missing ext4 rw 0 0
/dev/sdb2 /data\040disk ext4 rw 0 0
`

func writeSysFixture(t *testing.T, files map[string]string) {
	t.Helper()

	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", full, err)
		}
	}

	original := sysDir
	sysDir = root
	t.Cleanup(func() { sysDir = original })
}

func writeMountsFixture(t *testing.T, content string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mounts")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write mount table: %v", err)
	}

	original := mountsPath
	mountsPath = path
	t.Cleanup(func() { mountsPath = original })
}

// stubUDisks stubs the UDisks2 temperature lookup so tests do not reach the
// host's D-Bus: sysDir is a fixture, so the real provider would short-circuit
// anyway, but that contract is pinned here and the stub lets the fallback be
// exercised in isolation.
func stubUDisks(t *testing.T, temps map[string]float64) {
	t.Helper()

	original := udisksTemperatureProvider
	udisksTemperatureProvider = func(blockName string) (float64, bool) {
		value, ok := temps[blockName]
		return value, ok
	}
	t.Cleanup(func() { udisksTemperatureProvider = original })
}

// stubFilesystemUsage replaces the statfs call with fixed sizes per mount point
// and records which mount points were queried, so the tests can prove that an
// unmappable device is never measured.
func stubFilesystemUsage(t *testing.T, sizes map[string][3]uint64) *[]string {
	t.Helper()

	var queried []string
	original := filesystemUsage
	filesystemUsage = func(mountPoint string) (uint64, uint64, uint64, bool) {
		queried = append(queried, mountPoint)
		size, ok := sizes[mountPoint]
		if !ok {
			return 0, 0, 0, false
		}
		return size[0], size[1], size[2], true
	}
	t.Cleanup(func() { filesystemUsage = original })
	return &queried
}

func driveByModel(t *testing.T, drives []shared.HardwareData, model string) shared.HardwareData {
	t.Helper()

	for _, drive := range drives {
		if drive.Name == model {
			return drive
		}
	}
	t.Fatalf("no drive named %q in %d drives", model, len(drives))
	return shared.HardwareData{}
}

func textValue(t *testing.T, data shared.HardwareData, key string) string {
	t.Helper()

	value, ok := data.Values[key]
	if !ok {
		t.Fatalf("expected %q to be collected", key)
	}
	text, ok := value.Value.(string)
	if !ok {
		t.Fatalf("%s = %v (%T); want a string", key, value.Value, value.Value)
	}
	return text
}

func numericValue(t *testing.T, data shared.HardwareData, key string) float64 {
	t.Helper()

	value, ok := data.Values[key]
	if !ok {
		t.Fatalf("expected %q to be collected", key)
	}
	number, ok := value.Value.(float64)
	if !ok {
		t.Fatalf("%s = %v (%T); want a float64", key, value.Value, value.Value)
	}
	return number
}

func TestGetInformationsReadsDriveIdentity(t *testing.T) {
	writeSysFixture(t, sysfsFixture())
	writeMountsFixture(t, mountsFixture)
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	drives := GetInformations()
	if len(drives) != 3 {
		t.Fatalf("expected 3 drives, got %d", len(drives))
	}

	sata := driveByModel(t, drives, "SAMSUNG MZ7TE512")
	if got := textValue(t, sata, shared.DiskSerial.Key); got != "S1NYNSAF803084" {
		t.Errorf("serial = %q; want the SCSI serial", got)
	}
	if got := textValue(t, sata, shared.DiskFirmware.Key); got != "2L06" {
		t.Errorf("firmware = %q; want the SCSI rev", got)
	}
	if got := textValue(t, sata, shared.DiskMedium.Key); got != "SSD" {
		t.Errorf("medium = %q; want SSD", got)
	}

	nvme := driveByModel(t, drives, "Samsung SSD 990 PRO 2TB")
	if got := textValue(t, nvme, shared.DiskFirmware.Key); got != "4B2QJXD7" {
		t.Errorf("nvme firmware = %q; want firmware_rev", got)
	}
	if got := textValue(t, nvme, shared.DiskMedium.Key); got != "SSD (NVMe)" {
		t.Errorf("nvme medium = %q; want %q", got, "SSD (NVMe)")
	}

	hdd := driveByModel(t, drives, "WDC WD40EFRX-68N")
	if got := textValue(t, hdd, shared.DiskMedium.Key); got != "HDD" {
		t.Errorf("hdd medium = %q; want HDD", got)
	}
	if _, ok := hdd.Values[shared.DiskSerial.Key]; ok {
		t.Error("a drive that reports no serial must not get a serial row")
	}
}

func TestDriveCapacityUsesDecimalGigabytes(t *testing.T) {
	writeSysFixture(t, sysfsFixture())
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	drives := GetInformations()

	// 1000215216 sectors of 512 bytes = 512.1 GB, the capacity the drive is sold
	// as, not the 476.9 GiB a binary unit would give.
	size := numericValue(t, driveByModel(t, drives, "SAMSUNG MZ7TE512"), shared.DiskSize.Key)
	if math.Abs(size-512.11) > 0.01 {
		t.Errorf("capacity = %v; want ~512.11 GB", size)
	}
}

func TestGetInformationsSkipsNonDrives(t *testing.T) {
	writeSysFixture(t, sysfsFixture())
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	drives := GetInformations()

	for _, drive := range drives {
		switch {
		case drive.Name == "Loopback":
			t.Error("a loop device must not be reported as a drive")
		case strings.Contains(drive.Name, "DVDRAM"):
			t.Error("an optical drive must not be reported as a drive")
		case drive.Name == "":
			t.Error("a device with no model must not be reported as a drive")
		}
		if strings.Contains(drive.Name, "dm-") {
			t.Errorf("a device-mapper volume must not be reported as a drive: %q", drive.Name)
		}
	}
}

func TestGetInformationsWithoutDrives(t *testing.T) {
	writeSysFixture(t, map[string]string{"block/zram0/queue/rotational": "0"})
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	if drives := GetInformations(); drives != nil {
		t.Errorf("expected no drives, got %v", drives)
	}
}

func TestNVMeTemperaturesUseTheirSensorLabels(t *testing.T) {
	writeSysFixture(t, sysfsFixture())
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	nvme := driveByModel(t, GetInformations(), "Samsung SSD 990 PRO 2TB")

	checks := []struct {
		key   string
		value float64
		label string
	}{
		{"disk.temp0", 38, "Composite Temperature"},
		{"disk.temp1", 52, "Sensor 1 Temperature"},
		{"disk.temp2", 48, "Drive Temperature 3"}, // no label attribute
	}
	for _, c := range checks {
		if got := numericValue(t, nvme, c.key); math.Abs(got-c.value) > 1e-9 {
			t.Errorf("%s = %v; want %v", c.key, got, c.value)
		}
		label := textValue(t, nvme, tempLabelKeyFor(c.key))
		if label != c.label {
			t.Errorf("%s label = %q; want %q", c.key, label, c.label)
		}
	}
}

// TestDrivetempIsAcceptedButOtherChipsAreNot covers the SATA path: drivetemp's
// single sensor is read, and a chip belonging to something else is ignored even
// though it sits in the same place in sysfs.
func TestDrivetempIsAcceptedButOtherChipsAreNot(t *testing.T) {
	files := sysfsFixture()
	delete(files, "block/sda/device/hwmon/hwmon2/temp1_input")
	files["block/sda/device/hwmon/hwmon2/name"] = "coretemp"
	files["block/sda/device/hwmon/hwmon2/temp1_input"] = "99000"
	writeSysFixture(t, files)
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	sata := driveByModel(t, GetInformations(), "SAMSUNG MZ7TE512")
	if _, ok := sata.Values["disk.temp0"]; ok {
		t.Error("a non-drive hwmon chip must not be read as a drive temperature")
	}

	// With drivetemp in place the reading is picked up.
	files["block/sda/device/hwmon/hwmon2/name"] = "drivetemp"
	writeSysFixture(t, files)

	sata = driveByModel(t, GetInformations(), "SAMSUNG MZ7TE512")
	if got := numericValue(t, sata, "disk.temp0"); math.Abs(got-99) > 1e-9 {
		t.Errorf("disk.temp0 = %v; want 99", got)
	}
	if got := textValue(t, sata, "disk.tempLabel0"); got != "Drive Temperature 1" {
		t.Errorf("drivetemp label = %q; want a generic label", got)
	}
}

func TestFilesystemUsageIsAggregatedPerDrive(t *testing.T) {
	writeSysFixture(t, sysfsFixture())
	writeMountsFixture(t, mountsFixture)
	stubUDisks(t, nil)
	queried := stubFilesystemUsage(t, map[string][3]uint64{
		"/":          {500_000_000_000, 100_000_000_000, 400_000_000_000},
		"/boot/efi":  {535_000_000, 107_000_000, 428_000_000},
		"/home":      {1_000_000_000_000, 200_000_000_000, 800_000_000_000},
		"/data disk": {2_000_000_000_000, 500_000_000_000, 1_500_000_000_000},
	})

	drives := GetInformations()

	// Two filesystems on /dev/sda, and the same device mounted a second time —
	// the second mount must not be counted twice.
	sata := driveByModel(t, drives, "SAMSUNG MZ7TE512")
	if got := numericValue(t, sata, shared.DiskFSTotal.Key); math.Abs(got-500.535) > 0.001 {
		t.Errorf("fs total = %v; want 500.535 GB", got)
	}
	if got := numericValue(t, sata, shared.DiskFSUsed.Key); math.Abs(got-100.107) > 0.001 {
		t.Errorf("fs used = %v; want 100.107 GB", got)
	}
	if got := numericValue(t, sata, shared.DiskFSFree.Key); math.Abs(got-400.428) > 0.001 {
		t.Errorf("fs free = %v; want 400.428 GB", got)
	}

	nvme := driveByModel(t, drives, "Samsung SSD 990 PRO 2TB")
	if got := numericValue(t, nvme, shared.DiskFSTotal.Key); math.Abs(got-1000) > 0.001 {
		t.Errorf("nvme fs total = %v; want 1000 GB", got)
	}

	// The escaped mount point is looked up with the space decoded.
	hdd := driveByModel(t, drives, "WDC WD40EFRX-68N")
	if got := numericValue(t, hdd, shared.DiskFSUsed.Key); math.Abs(got-500) > 0.001 {
		t.Errorf("hdd fs used = %v; want 500 GB", got)
	}

	// Nothing that is not a reported drive may be measured: the mapped volume has
	// its own block device but is not a drive, the missing device is not in sysfs,
	// and the duplicate mount is skipped before the lookup.
	for _, mountPoint := range *queried {
		if mountPoint == "/mnt/encrypted" || mountPoint == "/mnt/missing" || mountPoint == "/mnt/bind" {
			t.Errorf("mount point %q should not have been measured (queried: %v)", mountPoint, *queried)
		}
	}
}

func TestFilesystemUsageIsAbsentWhenTheDriveIsNotMounted(t *testing.T) {
	writeSysFixture(t, sysfsFixture())
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	drives := GetInformations()
	for _, drive := range drives {
		if _, ok := drive.Values[shared.DiskFSTotal.Key]; ok {
			t.Errorf("drive %q reports filesystem usage without a mount", drive.Name)
		}
		if _, ok := drive.Values[shared.DiskFSUsed.Key]; ok {
			t.Errorf("drive %q reports used space without a mount", drive.Name)
		}
	}
}

func TestFilesystemUsageIsAbsentWhenTheMountCannotBeRead(t *testing.T) {
	writeSysFixture(t, sysfsFixture())
	writeMountsFixture(t, mountsFixture)
	// Every lookup fails, as it would for a mount point this process cannot
	// traverse. A size that cannot be read produces no row at all rather than a
	// zero-sized filesystem.
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	drives := GetInformations()
	for _, key := range []string{shared.DiskFSTotal.Key, shared.DiskFSUsed.Key, shared.DiskFSFree.Key} {
		if _, ok := driveByModel(t, drives, "SAMSUNG MZ7TE512").Values[key]; ok {
			t.Errorf("%s must not be reported when the mount point cannot be measured", key)
		}
	}
}

func TestFilesystemUsageIsAbsentWhenTheMountTableIsMissing(t *testing.T) {
	writeSysFixture(t, sysfsFixture())
	stubUDisks(t, nil)

	original := mountsPath
	mountsPath = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { mountsPath = original })

	drives := GetInformations()
	for _, key := range []string{shared.DiskFSTotal.Key, shared.DiskFSUsed.Key, shared.DiskFSFree.Key} {
		if _, ok := driveByModel(t, drives, "SAMSUNG MZ7TE512").Values[key]; ok {
			t.Errorf("%s must not be reported without a mount table", key)
		}
	}
}

func TestDriveNameForMapsPartitionsToTheirDisk(t *testing.T) {
	writeSysFixture(t, sysfsFixture())

	cases := map[string]string{"sda": "sda", // whole disk, so it maps to itself
		"sda2":        "sda",     // SATA partition
		"nvme0n1":     "nvme0n1", // whole NVMe namespace
		"nvme0n1p2":   "nvme0n1", // NVMe partition
		"dm-0":        "dm-0",    // a block device in its own right (no drive card)
		"sdd1":        "",        // not in sysfs: never guessed
		"mmcblk0p1":   "",        // its disk is absent from this fixture
		"nonexistent": "",
	}
	for name, want := range cases {
		if got := driveNameFor(name); got != want {
			t.Errorf("driveNameFor(%q) = %q; want %q", name, got, want)
		}
	}
}

func TestPartitionParent(t *testing.T) {
	cases := map[string]string{
		"sda2":       "sda",
		"vda1":       "vda",
		"hdb12":      "hdb",
		"nvme0n1p2":  "nvme0n1",
		"nvme1n1p10": "nvme1n1",
		"mmcblk0p1":  "mmcblk0",
		"md0p1":      "md0",
		"dm-0p1":     "dm-0",
		// A whole device has no partition suffix to strip. The caller never
		// reaches this path for one, but it must not invent a parent either.
		"sda":     "",
		"nvme0n1": "",
		"dm-0":    "",
		"md0":     "",
	}
	for name, want := range cases {
		if got := partitionParent(name); got != want {
			t.Errorf("partitionParent(%q) = %q; want %q", name, got, want)
		}
	}
}

func TestUnescapeMountField(t *testing.T) {
	cases := map[string]string{
		"/data disk": `/data\040disk`,
		`/a\b`:       `/a\134b`,
		"/plain":     "/plain",
	}
	for want, field := range cases {
		if got := unescapeMountField(field); got != want {
			t.Errorf("unescapeMountField(%q) = %q; want %q", field, got, want)
		}
	}
}

func TestTempSensorIndex(t *testing.T) {
	cases := []struct {
		fileName string
		want     int
		ok       bool
	}{
		{"temp1_input", 1, true},
		{"temp10_input", 10, true},
		{"temp_input", 0, false},
		{"tempX_input", 0, false},
		{"temp1_label", 0, false},
	}
	for _, c := range cases {
		got, ok := tempSensorIndex(c.fileName)
		if got != c.want || ok != c.ok {
			t.Errorf("tempSensorIndex(%q) = %d, %v; want %d, %v", c.fileName, got, ok, c.want, c.ok)
		}
	}
}

func TestMilliCelsius(t *testing.T) {
	if got, ok := milliCelsius("38000"); !ok || got != 38 {
		t.Errorf("milliCelsius(38000) = %v, %v; want 38, true", got, ok)
	}
	if _, ok := milliCelsius("n/a"); ok {
		t.Error("an unparsable reading must not be reported")
	}
}

// TestNVMeFlatHwmonPathIsRecognized covers the real-NVMe layout where the hwmon
// device appears directly as device/hwmonN (flat), not nested as the fixture's
// device/hwmon/hwmonN — the latter still passes, but the flat form was the one
// that left the internal NVMe with no temperature on the review host.
func TestNVMeFlatHwmonPathIsRecognized(t *testing.T) {
	files := sysfsFixture()
	// Replace the nested fixture path with the real flat location
	// (device/hwmon0 rather than device/hwmon/hwmon0).
	for key := range files {
		if strings.HasPrefix(key, "block/nvme0n1/device/hwmon/") {
			delete(files, key)
		}
	}
	files["block/nvme0n1/device/hwmon0/name"] = "nvme"
	files["block/nvme0n1/device/hwmon0/temp1_input"] = "49850"
	files["block/nvme0n1/device/hwmon0/temp1_label"] = "Composite"
	files["block/nvme0n1/device/hwmon0/temp3_input"] = "69850"
	files["block/nvme0n1/device/hwmon0/temp3_label"] = "Sensor 2"
	writeSysFixture(t, files)
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil)

	nvme := driveByModel(t, GetInformations(), "Samsung SSD 990 PRO 2TB")
	if got := numericValue(t, nvme, "disk.temp0"); math.Abs(got-49.85) > 1e-9 {
		t.Errorf("disk.temp0 in flat layout = %v; want 49.85", got)
	}
	if got := textValue(t, nvme, tempLabelKeyFor("disk.temp0")); got != "Composite Temperature" {
		t.Errorf("disk.temp0 label in flat layout = %q; want %q", got, "Composite Temperature")
	}
	if got := numericValue(t, nvme, "disk.temp1"); math.Abs(got-69.85) > 1e-9 {
		t.Errorf("disk.temp1 in flat layout = %v; want 69.85", got)
	}
	if got := textValue(t, nvme, tempLabelKeyFor("disk.temp1")); got != "Sensor 2 Temperature" {
		t.Errorf("disk.temp1 label = %q; want %q", got, "Sensor 2 Temperature")
	}
}

// TestSATATempViaUDisksFallback verifies psensor parity: when the drivetemp
// module is not loaded (no hwmon), the temperature is recovered from UDisks2's
// SmartTemperature — the same SMART data psensor reads via libatasmart. The
// stub stands in for the D-Bus service, so the fixture's absence of a real
// hwmon does not pollute the test with the host's drive.
func TestSATATempViaUDisksFallback(t *testing.T) {
	files := sysfsFixture()
	// Remove the drivetemp hwmon so the collector must use the UDisks fallback.
	for key := range files {
		if strings.HasPrefix(key, "block/sda/device/hwmon/") {
			delete(files, key)
		}
	}
	writeSysFixture(t, files)
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, map[string]float64{"sda": 27.0})

	sata := driveByModel(t, GetInformations(), "SAMSUNG MZ7TE512")
	if got := numericValue(t, sata, "disk.temp0"); math.Abs(got-27.0) > 1e-9 {
		t.Errorf("disk.temp0 from UDisks fallback = %v; want 27.0", got)
	}
	if got := textValue(t, sata, tempLabelKeyFor("disk.temp0")); got != "Drive Temperature" {
		t.Errorf("UDisks label = %q; want %q", got, "Drive Temperature")
	}
}

func TestUDisksFallbackIsOnlyUsedWhenHwmonIsMissing(t *testing.T) {
	// The SATA fixture already carries a drivetemp sensor; the UDisks answer is
	// available but must be ignored as long as hwmon is readable, so a working
	// kernel module is not shadowed by a stale D-Bus value.
	writeSysFixture(t, sysfsFixture())
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, map[string]float64{"sda": 99.0})

	sata := driveByModel(t, GetInformations(), "SAMSUNG MZ7TE512")
	if got := numericValue(t, sata, "disk.temp0"); math.Abs(got-31.0) > 1e-9 {
		t.Errorf("disk.temp0 with hwmon present = %v; wanted hwmon 31.0 not UDisks 99.0", got)
	}
}

func TestUDisksEmptyDoesNotFabricateATemperature(t *testing.T) {
	files := sysfsFixture()
	for key := range files {
		if strings.HasPrefix(key, "block/sda/device/hwmon/") {
			delete(files, key)
		}
	}
	writeSysFixture(t, files)
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, nil) // UDisks says "unknown"

	sata := driveByModel(t, GetInformations(), "SAMSUNG MZ7TE512")
	if _, ok := sata.Values["disk.temp0"]; ok {
		t.Error("a drive with no hwmon and no UDisks temperature must not fabricate a row")
	}
}

func TestNVMETempViaUDisksWhenHwmonMissing(t *testing.T) {
	files := sysfsFixture()
	for key := range files {
		if strings.HasPrefix(key, "block/nvme0n1/device/hwmon/") {
			delete(files, key)
		}
	}
	writeSysFixture(t, files)
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, map[string]float64{"nvme0n1": 49.9})

	nvme := driveByModel(t, GetInformations(), "Samsung SSD 990 PRO 2TB")
	if got := numericValue(t, nvme, "disk.temp0"); math.Abs(got-49.9) > 1e-9 {
		t.Errorf("NVMe disk.temp0 from UDisks = %v; want 49.9", got)
	}
	if got := textValue(t, nvme, tempLabelKeyFor("disk.temp0")); got != "Composite Temperature" {
		t.Errorf("NVMe UDisks label = %q; want %q", got, "Composite Temperature")
	}
}

func TestUDisksFallbackIsPerDrive(t *testing.T) {
	files := sysfsFixture()
	for key := range files {
		if strings.HasPrefix(key, "block/sda/device/hwmon/") || strings.HasPrefix(key, "block/nvme0n1/device/hwmon/") {
			delete(files, key)
		}
	}
	writeSysFixture(t, files)
	writeMountsFixture(t, "")
	stubFilesystemUsage(t, nil)
	stubUDisks(t, map[string]float64{"sda": 27.0}) // only sda has a value

	sata := driveByModel(t, GetInformations(), "SAMSUNG MZ7TE512")
	if _, ok := sata.Values["disk.temp0"]; !ok {
		t.Error("sda should have the UDisks temperature")
	}
	nvme := driveByModel(t, GetInformations(), "Samsung SSD 990 PRO 2TB")
	if _, ok := nvme.Values["disk.temp0"]; ok {
		t.Error("nvme0n1 with no stub entry must not receive sda's temperature")
	}
	shim := driveByModel(t, GetInformations(), "WDC WD40EFRX-68N")
	if _, ok := shim.Values["disk.temp0"]; ok {
		t.Error("sdb must not receive any fabricated temperature")
	}
}

// tempLabelKeyFor turns "disk.temp3" into "disk.tempLabel3".
func tempLabelKeyFor(key string) string {
	index := strings.LastIndexFunc(key, func(r rune) bool { return r < '0' || r > '9' })
	if index < 0 {
		return key
	}
	return key[:index+1] + "Label" + key[index+1:]
}
