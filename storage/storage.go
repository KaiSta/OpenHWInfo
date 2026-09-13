// Package storage collects information about the machine's drives: an SSD or
// hard disk's identity, its temperature, and how full the filesystems mounted
// from it are.
//
// Identity, capacity and filesystem usage come from sysfs and /proc, so no
// external tool and no root privilege is required. The temperature of a SATA
// drive needs the kernel's drivetemp driver (module "drivetemp"), and an NVMe
// drive's temperature comes from the nvme driver's own hwmon device; both expose
// their readings through the same tempN_input attributes, so they are read
// identically. When the drivetemp driver is not loaded (the usual default) the
// SATA temperature is read from the drive's SMART data instead, which is the
// same value psensor shows via libatasmart/udisks2 but without requiring raw
// device access.
//
// The SMART data itself, and the health verdict derived from it, are collected
// from UDisks2 over D-Bus (see smart.go): udisksd runs privileged and already
// talks to the drive, so the app needs neither root nor an external tool. Every
// reading is passive apart from one nowakeup poll of a drive whose SMART has
// never been read, and a sleeping drive is never spun up.
package storage

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KaiSta/OpenHWInfo/shared"
	"github.com/godbus/dbus/v5"
)

// sysDir and mountsPath are variables so tests can point the collector at a
// fixture tree, exactly as the GPU collectors do with sysDir.
var (
	sysDir     = "/sys"
	mountsPath = "/proc/mounts"
)

// Bounds on how many temperature sensors per drive are reported.
const maxDiskTemps = 16

// Keys for the temperature rows this package produces and for the labels that
// name them. The label keys are data rather than metrics: main.go reads them to
// name the generated rows, and they have no template of their own.
const (
	tempKeyPrefix      = "disk.temp"
	tempLabelKeyPrefix = "disk.tempLabel"
)

// virtualDevicePrefixes name block devices that are not physical drives. They
// are skipped explicitly: an optical drive (sr0) carries a model string like any
// drive, so the identity check below would otherwise report it as a hard disk.
// Device-mapper (dm-*) and RAID (md*) arrays report no model at all and are
// filtered out by that check instead — they are aggregations of the physical
// drives that are reported here.
var virtualDevicePrefixes = []string{"loop", "ram", "zram", "sr", "fd", "nbd", "rbd"}

// drive is a physical whole-disk block device: the device-mapper and RAID
// volumes that sysfs also lists under block/ carry no model and are not drives.
type drive struct {
	name  string // "sda", "nvme0n1"
	model string
	size  float64
	path  string // <sysDir>/block/<name>
}

// GetInformations returns one HardwareData per drive. It returns nil when the
// machine has no physical drive to report.
func GetInformations() []shared.HardwareData {
	drives := physicalDrives()
	usage := filesystemUsageByDrive(drives)

	var result []shared.HardwareData
	for _, d := range drives {
		values := make(map[string]shared.HardwareValue)
		values[shared.DiskModel.Key] = shared.NewHardwareValue(d.model, shared.UnknownUnit, shared.Unknown)

		if serial, ok := shared.ReadSysFile(filepath.Join(d.path, "device", "serial")); ok && serial != "" {
			values[shared.DiskSerial.Key] = shared.NewHardwareValue(serial, shared.UnknownUnit, shared.Unknown)
		}
		// NVMe exposes the firmware revision as firmware_rev, SCSI/SATA as rev.
		for _, file := range []string{"firmware_rev", "rev"} {
			if firmware, ok := shared.ReadSysFile(filepath.Join(d.path, "device", file)); ok && firmware != "" {
				values[shared.DiskFirmware.Key] = shared.NewHardwareValue(firmware, shared.UnknownUnit, shared.Unknown)
				break
			}
		}

		values[shared.DiskMedium.Key] = shared.NewHardwareValue(mediumOf(d.name, d.path), shared.UnknownUnit, shared.Unknown)
		if d.size > 0 {
			values[shared.DiskSize.Key] = shared.NewHardwareValue(d.size, shared.Gigabyte, shared.Memory)
		}

		addFilesystemUsage(values, usage[d.name])
		addTemperatures(values, d.path, d.name)

		// SMART is read last: it contributes the wear and defect rows and decides
		// the card's health badge, which stays nil for a drive that reports no
		// SMART at all rather than claiming the drive is fine.
		status := addSmart(values, d.name)

		shared.Debugf("Found drive %s (%s)", d.model, d.name)
		result = append(result, shared.HardwareData{Name: d.model, Values: values, Status: status})
	}

	return result
}

// physicalDrives returns the machine's drives, in a stable order so each drive
// keeps its card position between polls. A device qualifies as a drive when it
// has a model string, which is exactly what separates a physical drive from the
// virtual devices sysfs lists alongside it (loop, zram, device-mapper, RAID)
// and from an optical drive, which the name filter removes instead because it
// does carry a model.
func physicalDrives() []drive {
	paths, _ := filepath.Glob(filepath.Join(sysDir, "block", "*"))
	sort.Strings(paths)

	var drives []drive
	for _, path := range paths {
		name := filepath.Base(path)
		if hasVirtualPrefix(name) {
			continue
		}
		model, _ := shared.ReadSysFile(filepath.Join(path, "device", "model"))
		if model == "" {
			continue
		}
		size, _ := driveSizeGB(path)
		drives = append(drives, drive{name: name, model: model, size: size, path: path})
	}
	return drives
}

// hasVirtualPrefix reports whether a block device name belongs to one of the
// virtual or optical devices that are never reported as drives.
func hasVirtualPrefix(name string) bool {
	for _, prefix := range virtualDevicePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// mediumOf describes what kind of drive this is. The rotational flag is set by
// the kernel and is what distinguishes an SSD from a hard disk; an NVMe drive is
// always an SSD, and saying so makes an otherwise opaque /dev/nvme0n1 readable.
// A drive whose rotational flag cannot be read is described as unknown rather
// than guessed as an SSD.
func mediumOf(name, devicePath string) string {
	rotational, ok := shared.ReadSysFile(filepath.Join(devicePath, "queue", "rotational"))
	if !ok {
		return "Drive"
	}
	if strings.HasPrefix(rotational, "1") {
		return "HDD"
	}
	if strings.HasPrefix(name, "nvme") {
		return "SSD (NVMe)"
	}
	return "SSD"
}

// driveSizeGB reports the drive's raw capacity in decimal gigabytes, which is
// how drives are sold. sysfs reports the size in 512-byte sectors.
func driveSizeGB(devicePath string) (float64, bool) {
	raw, ok := shared.ReadSysFile(filepath.Join(devicePath, "size"))
	if !ok {
		return 0, false
	}
	sectors, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || sectors <= 0 {
		return 0, false
	}
	return sectors * 512 / 1e9, true
}

// addFilesystemUsage records how full the drive's mounted filesystems are. The
// rows are labelled as filesystem figures rather than drive figures because
// that is what they are: only mounted filesystems are counted, so they do not
// add up to the drive's raw capacity.
func addFilesystemUsage(values map[string]shared.HardwareValue, usage []fsUsage) {
	if len(usage) == 0 {
		return
	}
	var total, used, free uint64
	for _, u := range usage {
		total += u.total
		used += u.used
		free += u.available
	}
	if total == 0 {
		return
	}
	values[shared.DiskFSTotal.Key] = shared.NewHardwareValue(bytesToGB(total), shared.Gigabyte, shared.Memory)
	values[shared.DiskFSUsed.Key] = shared.NewHardwareValue(bytesToGB(used), shared.Gigabyte, shared.Memory)
	values[shared.DiskFSFree.Key] = shared.NewHardwareValue(bytesToGB(free), shared.Gigabyte, shared.Memory)
}

// bytesToGB converts bytes to decimal gigabytes, the unit every size on a drive
// card is reported in — capacity and filesystem usage — so those numbers can be
// compared with each other.
func bytesToGB(bytes uint64) float64 {
	return float64(bytes) / 1e9
}

// bytesToTB converts bytes to decimal terabytes, the unit the lifetime-written
// counter is reported in. Total writes are quoted in TB everywhere a drive's
// endurance is, so a card that showed them in gigabytes would be the only place
// in the industry that did not.
func bytesToTB(bytes uint64) float64 {
	return float64(bytes) / 1e12
}

// fsUsage is the space accounting of one mounted filesystem.
type fsUsage struct {
	total     uint64
	used      uint64
	available uint64
}

// filesystemUsageByDrive reports the space used by the filesystems mounted from
// each drive, keyed by drive name (e.g. "nvme0n1").
//
// Mounts are matched to drives through /proc/mounts and sysfs, never by
// guessing: a device that cannot be resolved to a whole disk in sysfs is left
// out, because reporting another drive's usage would be worse than reporting
// none. Filesystems that are not block devices (tmpfs, network mounts, the
// container's overlay) carry no /dev path and are skipped by that same rule.
func filesystemUsageByDrive(drives []drive) map[string][]fsUsage {
	usage := make(map[string][]fsUsage)

	// Only the reported drives are measured: an unmounted drive could not be
	// looked up anyway, and a mapped volume (dm-0, md0) must not have its usage
	// charged to whatever disk happens to sit underneath it.
	reportable := make(map[string]bool, len(drives))
	for _, d := range drives {
		reportable[d.name] = true
	}

	table, err := os.ReadFile(mountsPath)
	if err != nil {
		log.Printf("Could not read %s: %v\n", mountsPath, err)
		return usage
	}

	seen := make(map[string]bool)
	for _, line := range strings.Split(string(table), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "/dev/") {
			continue
		}
		device := unescapeMountField(fields[0])
		mountPoint := unescapeMountField(fields[1])

		// The same filesystem can be listed more than once (bind mounts, btrfs
		// subvolumes); counting it twice would overstate the drive's usage.
		resolved := resolveDevice(device)
		if seen[resolved] {
			continue
		}
		seen[resolved] = true

		drive := driveNameFor(filepath.Base(resolved))
		if drive == "" || !reportable[drive] {
			continue
		}
		total, used, available, ok := filesystemUsage(mountPoint)
		if !ok {
			continue
		}
		usage[drive] = append(usage[drive], fsUsage{total: total, used: used, available: available})
	}

	return usage
}

// resolveDevice follows the symlinks in a /dev path, so that both /dev/root and
// /dev/disk/by-uuid/... resolve to the device node they point at.
func resolveDevice(device string) string {
	if resolved, err := filepath.EvalSymlinks(device); err == nil {
		return resolved
	}
	return device
}

// driveNameFor maps a device node name to the whole-disk block device it belongs
// to: "sda2" and "nvme0n1p2" both resolve to their disk. The candidate name is
// only accepted when sysfs actually has such a disk, so a mis-derived name can
// never attach one drive's usage to another.
func driveNameFor(name string) string {
	if isWholeDisk(name) {
		return name
	}
	if _, ok := shared.ReadSysFile(filepath.Join(sysDir, "class/block", name, "partition")); !ok {
		// sysfs does not mark this as a partition, so there is nothing that
		// says which disk it belongs to.
		return ""
	}
	if parent := partitionParent(name); parent != "" && isWholeDisk(parent) {
		return parent
	}
	return ""
}

// isWholeDisk reports whether sysfs lists name as a whole-disk block device.
// /sys/block contains whole disks only, while /sys/class/block also lists their
// partitions.
func isWholeDisk(name string) bool {
	_, err := os.Stat(filepath.Join(sysDir, "block", name))
	return err == nil
}

// partitionParent derives the whole-disk name from a partition name: "sda2" to
// "sda", "nvme0n1p2" to "nvme0n1", "mmcblk0p1" to "mmcblk0", "md0p1" to "md0".
// It returns "" when the name carries no partition suffix.
//
// The caller has already established from sysfs that name *is* a partition and
// verifies the derived parent against sysfs too, so this only has to know the
// two ways Linux names partitions.
func partitionParent(name string) string {
	// A disk name that itself ends in a digit separates its partitions with a
	// "p" (nvme0n1p2, mmcblk0p1, md0p1, dm-0p1).
	for _, prefix := range []string{"nvme", "mmcblk", "md", "dm-"} {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if index := strings.LastIndex(name, "p"); index > 0 && allDigits(name[index+1:]) {
			return name[:index]
		}
		return ""
	}
	// SCSI, SATA, virtio and IDE append the partition number directly.
	trimmed := strings.TrimRight(name, "0123456789")
	if trimmed == name || trimmed == "" {
		return ""
	}
	return trimmed
}

// allDigits reports whether s is a non-empty run of digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// unescapeMountField decodes the octal escapes /proc/mounts uses for space, tab,
// newline and backslash, so a mount point containing a space can be looked up.
func unescapeMountField(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	replacements := []struct{ from, to string }{
		{`\040`, " "},
		{`\011`, "\t"},
		{`\012`, "\n"},
		{`\134`, `\`},
	}
	for _, r := range replacements {
		field = strings.ReplaceAll(field, r.from, r.to)
	}
	return field
}

// addTemperatures records the drive's temperature sensors. The chip is looked up
// beneath the block device, which is where both the nvme and the drivetemp
// driver attach their hwmon device; a chip that is not one of those two is
// ignored, so a sensor belonging to something else cannot be shown as a drive
// temperature.
//
// Two sysfs layouts are handled: drivetemp (and the fixture's NVMe layout)
// appear as device/hwmon/hwmonN (nested), while a real NVMe controller
// exposes device/hwmonN (flat). When no hwmon is found — the usual case for a
// SATA drive before the drivetemp module is loaded — the temperature is
// obtained from UDisks2's SmartTemperature property instead, which is the same
// data psensor shows via libatasmart/hddtemp/udisks2. That path needs no raw
// device access because UDisks2 already runs as root via D-Bus.
func addTemperatures(values map[string]shared.HardwareValue, devicePath, blockName string) {
	hwmonDir := findDriveHwmonDir(devicePath)
	if hwmonDir != "" {
		found := 0
		for _, sensor := range numberedSensors(hwmonDir) {
			value, ok := milliCelsius(sensor.input)
			if !ok {
				continue
			}
			label := fmt.Sprintf("Drive Temperature %d", sensor.index)
			if sensor.label != "" {
				label = sensor.label + " Temperature"
			}
			addTemperature(values, label, value)
			found++
		}
		if found > 0 {
			return
		}
	}
	// No hwmon reading — take the temperature out of the drive's SMART data
	// (psensor parity). This is the common path for SATA SSDs when drivetemp is
	// not loaded: psensor's libatasmart column reads the same SMART temperature
	// that UDisks2 exposes over D-Bus.
	if info, ok := smartProvider(blockName); ok && info.hasTemperature {
		label := "Drive Temperature"
		if strings.HasPrefix(blockName, "nvme") {
			label = "Composite Temperature"
		}
		addTemperature(values, label, info.temperatureC)
	}
}

// findDriveHwmonDir locates the hwmon directory for a drive, handling both the
// nested device/hwmon/hwmonN layout (drivetemp and the fixture's NVMe) and the
// flat device/hwmonN layout a real NVMe controller uses. It returns "" when no
// suitable chip is present.
func findDriveHwmonDir(devicePath string) string {
	// Nested: device/hwmon/hwmon*  (SATA drivetemp, fixture NVMe)
	if matches, _ := filepath.Glob(filepath.Join(devicePath, "device", "hwmon", "hwmon*")); len(matches) > 0 {
		sort.Strings(matches)
		for _, dir := range matches {
			if chip, ok := shared.ReadSysFile(filepath.Join(dir, "name")); ok && (chip == "nvme" || chip == "drivetemp") {
				return dir
			}
		}
	}
	// Flat: device/hwmon*  (real NVMe controller)
	if matches, _ := filepath.Glob(filepath.Join(devicePath, "device", "hwmon*")); len(matches) > 0 {
		sort.Strings(matches)
		for _, dir := range matches {
			if chip, ok := shared.ReadSysFile(filepath.Join(dir, "name")); ok && (chip == "nvme" || chip == "drivetemp") {
				return dir
			}
		}
	}
	return ""
}

// driveSensor is one numbered hwmon temperature group (tempN_*).
type driveSensor struct {
	index int
	input string
	label string
}

// numberedSensors returns the readable temperature sensors in a hwmon
// directory, ordered by sensor number. The optional tempN_label attribute
// supplies the sensor's name (NVMe drives label theirs "Composite", "Sensor 1",
// ...).
func numberedSensors(dir string) []driveSensor {
	matches, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))

	var sensors []driveSensor
	for _, match := range matches {
		index, ok := tempSensorIndex(filepath.Base(match))
		if !ok || index == 0 || index > maxDiskTemps {
			continue
		}
		input, ok := shared.ReadSysFile(match)
		if !ok || input == "" {
			continue
		}
		label, _ := shared.ReadSysFile(filepath.Join(dir, fmt.Sprintf("temp%d_label", index)))
		sensors = append(sensors, driveSensor{index: index, input: input, label: label})
	}

	// Glob sorts lexicographically, which would place temp10 before temp2.
	sort.Slice(sensors, func(i, j int) bool { return sensors[i].index < sensors[j].index })
	return sensors
}

// tempSensorIndex extracts N from "tempN_input", e.g. "temp10_input" -> 10.
func tempSensorIndex(fileName string) (int, bool) {
	digits, ok := strings.CutSuffix(strings.TrimPrefix(fileName, "temp"), "_input")
	if !ok || !allDigits(digits) {
		return 0, false
	}
	index, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return index, true
}

// addTemperature appends a drive temperature under the next free disk.tempN key,
// remembering its label under the matching label key so main.go can name the row.
func addTemperature(values map[string]shared.HardwareValue, label string, celsius float64) {
	for index := 0; index < maxDiskTemps; index++ {
		key := tempKeyPrefix + strconv.Itoa(index)
		if _, taken := values[key]; taken {
			continue
		}
		values[key] = shared.NewHardwareValue(celsius, shared.Celsius, shared.Temperature)
		values[tempLabelKeyPrefix+strconv.Itoa(index)] = shared.NewHardwareValue(label, shared.UnknownUnit, shared.Unknown)
		return
	}
	log.Println("Drive reports more temperature sensors than can be displayed")
}

// milliCelsius converts a hwmon millidegree-Celsius reading to degrees.
func milliCelsius(raw string) (float64, bool) {
	milli, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return float64(milli) / 1000, true
}

// --- Shared UDisks2 connection ---

var (
	udisksConnMu      sync.Mutex
	udisksConn        *dbus.Conn
	udisksConnErr     error
	udisksConnErrTime time.Time
)

// udisksTimeout bounds the cheap property reads. udisksd serves them from its
// own cache once it knows a drive, so the limit only matters when the bus is
// absent altogether.
const udisksTimeout = 200 * time.Millisecond

func getUDisksConn() (*dbus.Conn, error) {
	udisksConnMu.Lock()
	defer udisksConnMu.Unlock()
	if udisksConn != nil {
		if udisksConn.Connected() {
			return udisksConn, nil
		}
		_ = udisksConn.Close()
		udisksConn = nil
		udisksConnErr = nil
	}
	if udisksConnErr != nil {
		// Last SystemBus attempt failed (e.g. no bus on this host). Retry after
		// a backoff window rather than failing forever; the per-drive SMART cache
		// already rate-limits callers to smartPropsTTL, so a 30s window means at
		// most ~3 extra dial attempts per minute even with many drives.
		if time.Since(udisksConnErrTime) < 30*time.Second {
			return nil, udisksConnErr
		}
		udisksConnErr = nil
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		udisksConnErr = err
		udisksConnErrTime = time.Now()
		return nil, err
	}
	udisksConn = conn
	return conn, nil
}
