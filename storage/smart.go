package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KaiSta/OpenHWInfo/shared"
	"github.com/godbus/dbus/v5"
)

// SMART collection. Every reading comes from UDisks2 over D-Bus, which is what
// makes this possible without root: udisksd runs privileged and already talks to
// the drive, so the app itself never needs raw device access or an external
// tool. `smartctl` would need root (it fails with "Permission denied" for a
// normal user) and `nvme-cli` may not be installed at all, so neither is used.
//
// Two D-Bus interfaces carry the data, one per command set:
//
//	org.freedesktop.UDisks2.Drive.Ata        SATA/ATA drives
//	org.freedesktop.UDisks2.NVMe.Controller  NVMe drives
//
// The interface's SMART properties (failing flag, bad sectors, self-test
// result, power-on time) are plain D-Bus property reads served from udisksd's
// cache, so they are cheap enough to refresh every poll. The attribute table is
// a method call that sends a command to the drive, so it is read rarely and
// never wakes a sleeping drive.
const (
	udisksService = "org.freedesktop.UDisks2"
	ataInterface  = "org.freedesktop.UDisks2.Drive.Ata"
	nvmeInterface = "org.freedesktop.UDisks2.NVMe.Controller"
)

const (
	// smartPropsTTL is how long a drive's SMART properties are reused. It
	// matches the temperature cache: the poll interval is 5s, so a 10s window
	// costs at most one D-Bus round trip per drive per two polls.
	smartPropsTTL = 10 * time.Second
	// smartAttrsTTL is how long the attribute table is reused. It is much
	// longer than the property TTL because these attributes move slowly (wear,
	// bytes written) and reading them is the only part that commands the drive.
	smartAttrsTTL = 60 * time.Second
	// smartCommandTimeout bounds the two calls that reach the drive rather than
	// udisksd's cache. Measured at ~10ms on an idling SSD, so this is generous
	// room for a drive that has to spin up, without stalling the poll for long.
	smartCommandTimeout = time.Second
)

// Attribute-derived readings. The attribute table is only consulted for the few
// attributes whose meaning is unambiguous; see addSmartAttributes.
var (
	smartCacheMu sync.Mutex
	smartCache   = make(map[string]smartCacheEntry)
)

// smartProvider is the function used to obtain a drive's SMART data. It
// defaults to the cached D-Bus query and is replaced in tests so a fixture tree
// never reaches the host's bus.
var smartProvider = fetchSmart

// smartInfo is the health picture of one drive, normalized across the two
// command sets so the evaluation below does not care which one answered.
type smartInfo struct {
	// supported is true when the drive answered a SMART interface at all;
	// updated is true when that interface holds a reading (UDisks2 documents
	// every other SMART property as meaningless until it does).
	supported bool
	updated   bool
	// isNVMe distinguishes the two command sets, whose readings need different
	// wording: NVMe has no attribute thresholds and no bad-sector counter.
	isNVMe bool

	// The drive's own verdict, taken at face value: the ATA failing flag and
	// the number of attributes currently below their threshold.
	failing         bool
	numFailing      int
	numFailedInPast int
	badSectors      int64
	hasBadSectors   bool
	selftest        string
	selftestPercent int

	powerOnHours   uint64
	temperatureC   float64
	hasTemperature bool

	// NVMe critical warnings, empty when the controller reports none.
	criticalWarnings []string

	// From the attribute table (see readSmartAttributes).
	lifeUsed       float64
	hasLife        bool
	writtenBytes   uint64
	hasWritten     bool
	spare          int
	spareThreshold int
	hasSpare       bool
	mediaErrors    uint64
	hasMediaErrors bool
}

// Health thresholds for the readings SMART does not itself judge. The drive
// flags a failing attribute on its own, but an attribute that is merely close
// to its limit, or a wear estimate still below it, only the app can call out.
const (
	// lifeWarnPercent and lifeBadPercent are consumed endurance percentages.
	lifeWarnPercent = 80
	lifeBadPercent  = 95
	// defaultSpareThreshold is used when a controller does not report its own
	// available-spare threshold. 10% is what NVMe drives ship with.
	defaultSpareThreshold = 10
)

// --- Cache and entry point -------------------------------------------

type smartCacheEntry struct {
	info         smartInfo
	ok           bool
	propsAt      time.Time
	attrsAt      time.Time
	refreshTried bool
}

// fetchSmart returns a drive's SMART data, reusing a recent answer. The second
// return value reports whether UDisks2 knew the drive at all: false means there
// is nothing to say about it, which callers must not turn into a fabricated
// reading.
func fetchSmart(blockName string) (smartInfo, bool) {
	// In a fixture (sysDir != "/sys") no query is made, so tests are not
	// polluted by the host's real drives; tests that want SMART replace
	// smartProvider with a stub.
	if sysDir != "/sys" {
		return smartInfo{}, false
	}

	smartCacheMu.Lock()
	defer smartCacheMu.Unlock()

	now := time.Now()
	entry, ok := refreshSmartEntry(smartCache[blockName], blockName, now,
		readSmartProperties, readSmartAttributes)
	if !ok {
		// Nothing answered: drop whatever was cached, including the attributes,
		// so a drive that disappears leaves no stale rows.
		smartCache[blockName] = smartCacheEntry{ok: false, propsAt: now}
		return smartInfo{}, false
	}

	smartCache[blockName] = entry
	return entry.info, true
}

// refreshSmartEntry applies one poll's two SMART reads to a cached entry and
// reports whether the drive answered at all. It is kept apart from the cache so
// that the two read schedules can be tested without a bus.
func refreshSmartEntry(entry smartCacheEntry, blockName string, now time.Time,
	readProperties func(string, bool) (smartInfo, bool, bool),
	readAttributes func(string, smartInfo) smartInfo) (smartCacheEntry, bool) {

	if !entry.ok || now.Sub(entry.propsAt) >= smartPropsTTL {
		info, ok, refreshTried := readProperties(blockName, entry.refreshTried)
		if !ok {
			return smartCacheEntry{}, false
		}
		// The properties are the cheap half and the reading is now current, but
		// this read says nothing about the attributes: their readings are carried
		// over rather than discarded. Overwriting them here is what made the wear
		// row disappear on every poll between the two schedules, and the written,
		// spare and media-error rows would have gone the same way.
		entry.info = info.withCachedAttributeReadings(entry.info)
		entry.ok = true
		entry.refreshTried = refreshTried
		entry.propsAt = now
	}

	// The attribute table is the expensive half, so it is refreshed on its own
	// (slower) schedule. A failure is remembered too: a drive that refuses the
	// command must not be asked again on every poll.
	if now.Sub(entry.attrsAt) >= smartAttrsTTL {
		entry.info = readAttributes(blockName, entry.info)
		entry.attrsAt = now
	}

	return entry, true
}

// withCachedAttributeReadings returns a freshly read set of SMART properties
// with the readings that come from the attribute table taken from the cached
// entry, which is where the last attribute read left them. A drive whose command
// set changed, or that no longer reports SMART at all, is a different drive and
// inherits nothing.
func (info smartInfo) withCachedAttributeReadings(cached smartInfo) smartInfo {
	if !info.supported || info.isNVMe != cached.isNVMe {
		return info
	}
	if cached.hasLife {
		info.lifeUsed, info.hasLife = cached.lifeUsed, true
	}
	if cached.hasWritten {
		info.writtenBytes, info.hasWritten = cached.writtenBytes, true
	}
	if cached.hasSpare {
		info.spare, info.spareThreshold, info.hasSpare = cached.spare, cached.spareThreshold, true
	}
	if cached.hasMediaErrors {
		info.mediaErrors, info.hasMediaErrors = cached.mediaErrors, true
	}
	return info
}

// clearAttributeReadings drops the readings that come from the attribute table.
// A read that fails falls back to this: leaving the previous poll's wear or byte
// count standing would be the app reporting a figure the drive has not just
// given.
func (info smartInfo) clearAttributeReadings() smartInfo {
	info.lifeUsed, info.hasLife = 0, false
	info.writtenBytes, info.hasWritten = 0, false
	info.spare, info.spareThreshold, info.hasSpare = 0, 0, false
	info.mediaErrors, info.hasMediaErrors = 0, false
	return info
}

// --- D-Bus reads -----------------------------------------------------

// readSmartProperties reads the cheap SMART properties of a drive, trying the
// ATA interface first and the NVMe one second. refreshTried records whether a
// one-off SmartUpdate has already been attempted for this drive.
func readSmartProperties(blockName string, refreshTried bool) (smartInfo, bool, bool) {
	conn, err := getUDisksConn()
	if err != nil {
		return smartInfo{}, false, refreshTried
	}
	ctx, cancel := context.WithTimeout(context.Background(), udisksTimeout)
	defer cancel()

	drivePath, ok := driveObjectPath(ctx, conn, blockName)
	if !ok {
		return smartInfo{}, false, refreshTried
	}
	obj := conn.Object(udisksService, drivePath)

	if props, ok := interfaceProperties(ctx, obj, ataInterface); ok {
		info := ataSmartInfo(props)
		// UDisks2 populates these properties when the drive is probed, so a
		// zero SmartUpdated normally means no one has asked yet. Ask once, with
		// nowakeup so a powered-down drive is left alone, and read again.
		if info.supported && !info.updated && !refreshTried {
			if smartUpdate(ctx, obj, ataInterface) {
				if retry, ok := interfaceProperties(ctx, obj, ataInterface); ok {
					info = ataSmartInfo(retry)
				}
			}
			refreshTried = true
		}
		return info, true, refreshTried
	}

	if props, ok := interfaceProperties(ctx, obj, nvmeInterface); ok {
		info := nvmeSmartInfo(props)
		if !info.updated && !refreshTried {
			if smartUpdate(ctx, obj, nvmeInterface) {
				if retry, ok := interfaceProperties(ctx, obj, nvmeInterface); ok {
					info = nvmeSmartInfo(retry)
				}
			}
			refreshTried = true
		}
		return info, true, refreshTried
	}

	return smartInfo{}, false, refreshTried
}

// driveObjectPath walks from a block device to the drive object that carries the
// SMART interfaces. Partition and namespace block devices both point at their
// whole drive through the Block.Drive property.
func driveObjectPath(ctx context.Context, conn *dbus.Conn, blockName string) (dbus.ObjectPath, bool) {
	blockPath := dbus.ObjectPath("/org/freedesktop/UDisks2/block_devices/" + blockName)
	blockObj := conn.Object(udisksService, blockPath)

	var v dbus.Variant
	if err := blockObj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0,
		"org.freedesktop.UDisks2.Block", "Drive").Store(&v); err != nil {
		return "", false
	}
	path, ok := v.Value().(dbus.ObjectPath)
	if !ok {
		// Some bindings surface an object path as a string.
		if s, isString := v.Value().(string); isString {
			path = dbus.ObjectPath(s)
		} else {
			return "", false
		}
	}
	if path == "" || path == "/" {
		return "", false
	}
	return path, true
}

// interfaceProperties reads every property of one interface in a single call.
func interfaceProperties(ctx context.Context, obj dbus.BusObject, iface string) (map[string]dbus.Variant, bool) {
	var props map[string]dbus.Variant
	if err := obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, iface).Store(&props); err != nil {
		return nil, false
	}
	return props, true
}

// smartUpdate asks udisksd to re-read a drive's SMART data. The nowakeup option
// makes it fail instead of spinning up a sleeping drive, and a sleeping drive is
// left as it is: a monitor has no business waking a disk.
func smartUpdate(ctx context.Context, obj dbus.BusObject, iface string) bool {
	commandCtx, cancel := context.WithTimeout(ctx, smartCommandTimeout)
	defer cancel()
	options := map[string]dbus.Variant{"nowakeup": dbus.MakeVariant(true)}
	return obj.CallWithContext(commandCtx, iface+".SmartUpdate", 0, options).Err == nil
}

// ataSmartInfo reads the Drive.Ata SMART properties.
func ataSmartInfo(props map[string]dbus.Variant) smartInfo {
	info := smartInfo{
		supported: variantBool(props, "SmartSupported"),
		updated:   variantUint64Or(props, "SmartUpdated", 0) != 0,
	}
	if !info.supported {
		return info
	}
	info.failing = variantBool(props, "SmartFailing")
	info.numFailing = variantIntOr(props, "SmartNumAttributesFailing", -1)
	info.numFailedInPast = variantIntOr(props, "SmartNumAttributesFailedInThePast", -1)
	// -1 is what UDisks2 reports for an unreadable figure, and only a figure
	// that was actually read may become a row: a missing bad-sector count must
	// not be shown as zero bad sectors.
	if sectors, ok := variantInt64(props, "SmartNumBadSectors"); ok {
		info.badSectors, info.hasBadSectors = sectors, true
	}
	info.selftest = variantString(props, "SmartSelftestStatus")
	info.selftestPercent = variantIntOr(props, "SmartSelftestPercentRemaining", -1)
	info.powerOnHours = variantUint64Or(props, "SmartPowerOnSeconds", 0) / 3600
	// ATA reports this one as a double in Kelvin, with 0 for "unknown".
	if kelvin := variantFloat(props, "SmartTemperature"); kelvin > 0 {
		info.temperatureC = kelvin - 273.15
		info.hasTemperature = true
	}
	return info
}

// nvmeSmartInfo reads the NVMe.Controller properties. There is no support flag
// on this interface; a controller that answers one holds SMART data.
func nvmeSmartInfo(props map[string]dbus.Variant) smartInfo {
	info := smartInfo{
		supported: true,
		isNVMe:    true,
		updated:   variantUint64Or(props, "SmartUpdated", 0) != 0,
	}
	info.criticalWarnings = variantStringSlice(props, "SmartCriticalWarning")
	info.selftest = variantString(props, "SmartSelftestStatus")
	info.selftestPercent = variantIntOr(props, "SmartSelftestPercentRemaining", -1)
	info.powerOnHours = variantUint64Or(props, "SmartPowerOnHours", 0)
	// NVMe reports the composite temperature in Kelvin, 0 for "unknown".
	if kelvin := variantIntOr(props, "SmartTemperature", 0); kelvin > 0 {
		info.temperatureC = float64(kelvin) - 273.15
		info.hasTemperature = true
	}
	return info
}

// ataAttribute is one row of the SmartGetAttributes reply, which is a D-Bus
// struct (ysqiiixia{sv}): id, name, flags, value, worst, threshold, pretty
// value, pretty unit, expansion. The field order is the wire order and must not
// be changed.
type ataAttribute struct {
	ID         byte
	Name       string
	Flags      uint16
	Value      int32
	Worst      int32
	Threshold  int32
	Pretty     int64
	PrettyUnit int32
	Expansion  map[string]dbus.Variant
}

// Life attributes, matched by name because that is the authoritative identifier:
// libatasmart derives it from the drive's own IDENTIFY data, which is what
// disambiguates attribute ids that different vendors reuse.
//
// Only attributes whose normalized value is documented to count down from 100
// as the media wears are listed. Most drives that carry one of these also stop
// counting at the drive's own fail threshold, which is why the consumed
// fraction is scaled against the threshold rather than against 0.
var lifeAttributes = []string{
	"ssd-life-left",           // 231, Sandisk/WD/Kingston
	"media-wearout-indicator", // 233, Intel/Samsung
	"wear-leveling-count",     // 177, Samsung/Intel SSDs
}

// readSmartAttributes reads the attribute table and folds the few attributes
// with a meaning the drive does not contradict into the info: its endurance and
// its lifetime bytes written. Failures are silent: the properties already
// collected still describe the drive's health, and these rows are simply absent.
func readSmartAttributes(blockName string, info smartInfo) smartInfo {
	// Everything below either replaces the readings carried in from the last
	// attribute read or leaves them absent, so clear them first: a read that
	// fails must not leave the previous poll's figures standing as if the drive
	// had just reported them.
	info = info.clearAttributeReadings()
	if !info.supported || !info.updated {
		return info
	}
	conn, err := getUDisksConn()
	if err != nil {
		return info
	}
	ctx, cancel := context.WithTimeout(context.Background(), udisksTimeout)
	defer cancel()

	drivePath, ok := driveObjectPath(ctx, conn, blockName)
	if !ok {
		return info
	}
	obj := conn.Object(udisksService, drivePath)
	commandCtx, cancelCommand := context.WithTimeout(context.Background(), smartCommandTimeout)
	defer cancelCommand()

	// ATA: a typed array of attribute structs.
	var attrs []ataAttribute
	if err := obj.CallWithContext(commandCtx, ataInterface+".SmartGetAttributes", 0, noWakeup()).Store(&attrs); err == nil {
		// The interface answered, so this is an ATA drive and the NVMe call
		// below would only fail: report what the attributes contained.
		info.lifeUsed, info.hasLife = lifeFromAttributes(attrs)
		info.writtenBytes, info.hasWritten = writtenFromAttributes(attrs)
		return info
	}

	// NVMe: a dictionary of variant values.
	var values map[string]dbus.Variant
	if err := obj.CallWithContext(commandCtx, nvmeInterface+".SmartGetAttributes", 0, noWakeup()).Store(&values); err != nil {
		return info
	}
	return applyNvmeAttributes(info, values)
}

// ataTotalLbasWrittenID is the attribute that carries a drive's lifetime writes.
// It is matched by id rather than by name because, unlike the endurance
// attributes whose ids vendors share for different meanings, this one has the
// same meaning everywhere; what varies between drives is only its unit (see
// writtenFromAttributes).
const ataTotalLbasWrittenID = 241

// The values of libatasmart's SkSmartAttributeUnit, which is what udisksd sends
// in an attribute's pretty_unit field. They say how to read that same struct's
// pretty value: a figure the library has already converted for the units it
// knows (milliseconds, millikelvin, megabytes, percentages), and the drive's raw
// counter for the ones that carry no unit of their own.
//
// libatasmart's enum is append-only, which is why the numbers are spelled out
// here rather than mirrored from a header the app cannot import.
const (
	ataUnitUnknown  = 0 // the library knows of no such attribute
	ataUnitNone     = 1 // a bare count
	ataUnitMseconds = 2
	ataUnitSectors  = 3 // 512-byte sectors
	ataUnitMkelvin  = 4
	ataUnitSmallPct = 5
	ataUnitPercent  = 6
	ataUnitMB       = 7
)

// writtenFromAttributes reads a drive's lifetime bytes written out of an ATA
// attribute table. It reports false when the drive carries no such counter,
// which is common: an SSD that reports its endurance is not obliged to report
// its total writes, and the review host's drive is one of those.
//
// The count itself is unambiguous — ATA defines attribute 241 as a count of
// 512-byte sectors — but its unit is not: some vendors, SanDisk and WD among
// them, count in 32 MiB units instead, and smartmontools needs a per-model entry
// in its drive database to tell the two conventions apart. Nothing in the reading
// distinguishes them, so this reads the sector convention the standard specifies
// and honors the unit the platform's SMART library reports when it reports a
// different byte count. A unit that is not a byte count at all is refused rather
// than converted, and a counter of zero yields nothing: a drive that reports
// "none" and a drive that reports nothing are indistinguishable, and printing
// 0.00 TB for either would be the app's claim rather than the drive's.
func writtenFromAttributes(attrs []ataAttribute) (uint64, bool) {
	for _, attr := range attrs {
		if int(attr.ID) != ataTotalLbasWrittenID || attr.Pretty <= 0 {
			continue
		}
		switch attr.PrettyUnit {
		case ataUnitUnknown, ataUnitNone, ataUnitSectors:
			// The ATA definition, and how a count with no unit of its own is
			// read.
			return uint64(attr.Pretty) * 512, true
		case ataUnitMB:
			return uint64(attr.Pretty) * 1_000_000, true
		}
		return 0, false
	}
	return 0, false
}

// lifeFromAttributes picks the drive's endurance reading out of an ATA
// attribute table. A drive reports at most one of the life attributes in
// practice, so the first one that yields a usable figure wins.
func lifeFromAttributes(attrs []ataAttribute) (float64, bool) {
	for _, attr := range attrs {
		if !isLifeAttribute(attr.Name) {
			continue
		}
		if used, ok := lifeUsedFrom(int(attr.Value), int(attr.Threshold)); ok {
			return used, true
		}
	}
	return 0, false
}

// applyNvmeAttributes folds the NVMe SMART attribute dictionary into the info.
// The values arrive as variants whose types follow the NVMe specification:
// percentages are single bytes and the byte counts are 64-bit.
func applyNvmeAttributes(info smartInfo, values map[string]dbus.Variant) smartInfo {
	if used, ok := variantByte(values, "percent_used"); ok {
		info.lifeUsed, info.hasLife = float64(used), true
	}
	if written, ok := variantUint64(values, "total_data_written"); ok {
		info.writtenBytes, info.hasWritten = written, true
	}
	if spare, ok := variantByte(values, "avail_spare"); ok {
		threshold, hasThreshold := variantByte(values, "spare_thresh")
		if !hasThreshold {
			threshold = defaultSpareThreshold
		}
		info.spare, info.spareThreshold, info.hasSpare = int(spare), int(threshold), true
	}
	if errors, ok := variantUint64(values, "media_errors"); ok {
		info.mediaErrors, info.hasMediaErrors = errors, true
	}
	return info
}

// noWakeup is the option set passed to the SMART methods that read the drive, so
// a sleeping drive answers with an error instead of being spun up.
func noWakeup() map[string]dbus.Variant {
	return map[string]dbus.Variant{"nowakeup": dbus.MakeVariant(true)}
}

func isLifeAttribute(name string) bool {
	for _, life := range lifeAttributes {
		if name == life {
			return true
		}
	}
	return false
}

// lifeUsedFrom converts a life attribute's normalized value into the consumed
// fraction of the drive's endurance.
//
// SMART normalizes every attribute to 100 at its best and to the drive's own
// threshold at its worst, so the threshold is the drive's declared end of life
// and the consumed fraction is measured against it. A threshold of 0 or 100
// carries no information, and then the value is read as a plain percentage of
// 100.
func lifeUsedFrom(value, threshold int) (float64, bool) {
	// A value of 0 or a negative one is documentation for "unknown"; above 100
	// is not a percentage and would silently become a negative wear figure.
	if value <= 0 || value > 100 {
		return 0, false
	}
	used := float64(100 - value)
	if threshold > 0 && threshold < 100 {
		used = used / float64(100-threshold) * 100
	}
	if used < 0 {
		used = 0
	}
	return used, true
}

// --- Evaluation ------------------------------------------------------

// Severities of a health finding. A note never changes the verdict: it is a fact
// worth showing next to a healthy drive (a past attribute failure, say) but not
// a reason to call the drive unwell.
const (
	severityNote = iota
	severityWarn
	severityBad
)

type finding struct {
	severity int
	text     string
}

// evaluateHealth turns a SMART reading into the verdict shown on the drive's
// card. The rules are ordered so the drive's own judgement outranks any
// inference: a flagged attribute or a critical warning is the drive saying it is
// unwell, whereas the wear and spare thresholds below are the app's reading of a
// figure the drive has not yet faulted.
func evaluateHealth(info smartInfo) shared.DeviceStatus {
	if !info.supported || !info.updated {
		return shared.DeviceStatus{
			Level:  shared.StatusUnknown,
			Text:   "SMART: Unknown",
			Detail: "The drive supports SMART but no reading of it is available yet.",
		}
	}

	var findings []finding
	add := func(severity int, format string, args ...interface{}) {
		findings = append(findings, finding{severity, fmt.Sprintf(format, args...)})
	}

	// The drive's own verdict.
	if info.failing {
		add(severityBad, "the drive reports a SMART failure")
	}
	if info.numFailing > 0 {
		add(severityBad, "%d SMART attribute(s) are below their threshold", info.numFailing)
	}
	for _, warning := range info.criticalWarnings {
		switch warning {
		case "degraded", "readonly", "volatile_mem", "pmr_readonly":
			add(severityBad, "the controller reports %s", nvmeWarningText(warning))
		case "spare", "temperature":
			add(severityWarn, "the controller reports %s", nvmeWarningText(warning))
		default:
			add(severityWarn, "the controller reports an unknown critical warning (%s)", warning)
		}
	}
	if info.numFailedInPast > 0 {
		add(severityNote, "%d attribute(s) failed in the past", info.numFailedInPast)
	}

	// Defects the drive has already flagged, counted rather than judged.
	if info.hasBadSectors && info.badSectors > 0 {
		add(severityWarn, "%d bad sector(s)", info.badSectors)
	}
	if info.hasMediaErrors && info.mediaErrors > 0 {
		add(severityWarn, "%d media error(s)", info.mediaErrors)
	}

	// Self-test result. A test that merely never ran reports "success", so only
	// a real failure is a finding.
	if severity, text, ok := selftestFinding(info.selftest); ok {
		add(severity, "%s", text)
	}

	// Endurance and spare capacity: figures the drive tracks but does not fault
	// until they cross the limit, so they are worth an early warning.
	if info.hasLife {
		switch {
		case info.lifeUsed >= lifeBadPercent:
			add(severityBad, "%.0f%% of the rated endurance is used", info.lifeUsed)
		case info.lifeUsed >= lifeWarnPercent:
			add(severityWarn, "%.0f%% of the rated endurance is used", info.lifeUsed)
		}
	}
	if info.hasSpare {
		threshold := info.spareThreshold
		if threshold <= 0 {
			threshold = defaultSpareThreshold
		}
		if info.spare < threshold {
			add(severityWarn, "spare capacity is %d%%, below the %d%% threshold", info.spare, threshold)
		}
	}

	// The worst finding wins, except that a note never raises the level. Problems
	// and notes are kept apart because they are written to different sentences.
	level := shared.StatusGood
	var problems, notes []string
	for _, f := range findings {
		if f.severity == severityNote {
			notes = append(notes, f.text)
			continue
		}
		problems = append(problems, f.text)
		switch f.severity {
		case severityBad:
			level = shared.StatusBad
		case severityWarn:
			if level != shared.StatusBad {
				level = shared.StatusWarn
			}
		}
	}

	text := map[string]string{
		shared.StatusGood: "SMART: Good",
		shared.StatusWarn: "SMART: Warning",
		shared.StatusBad:  "SMART: Failing",
	}[level]

	// A verdict always states its basis. describeHealthy is only valid here when
	// the drive is in fact healthy: it asserts there is nothing below threshold,
	// which would be a lie beside a problem.
	var detail string
	if len(problems) > 0 {
		detail = "SMART reports: " + strings.Join(problems, "; ") + "."
	} else {
		detail = "SMART reports no problems: " + describeHealthy(info)
		if len(notes) > 0 {
			detail += "; " + strings.Join(notes, "; ")
		}
		detail += "."
	}

	return shared.DeviceStatus{Level: level, Text: text, Detail: detail}
}

// describeHealthy summarizes the reassuring figures of a drive with nothing to
// report, so a "Good" badge still says what it is based on.
func describeHealthy(info smartInfo) string {
	var parts []string
	if !info.isNVMe {
		parts = append(parts, fmt.Sprintf("%d attribute(s) below threshold", max0(info.numFailing)))
		if info.hasBadSectors {
			parts = append(parts, fmt.Sprintf("%d bad sector(s)", info.badSectors))
		}
	}
	// "Critical warning" is NVMe vocabulary: an ATA drive has no warning list
	// to be empty.
	if info.isNVMe && len(info.criticalWarnings) == 0 {
		parts = append(parts, "no controller warnings")
	}
	if info.hasLife {
		parts = append(parts, fmt.Sprintf("%.0f%% endurance used", info.lifeUsed))
	}
	if info.hasSpare {
		parts = append(parts, fmt.Sprintf("%d%% spare capacity", info.spare))
	}
	if text, ok := selftestText(info.selftest); ok {
		parts = append(parts, "self-test "+strings.ToLower(text))
	}
	return strings.Join(parts, ", ")
}

func max0(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// nvmeWarningText names a NVMe critical warning the way the NVMe specification
// describes it.
func nvmeWarningText(warning string) string {
	switch warning {
	case "spare":
		return "that the available spare capacity is below its threshold"
	case "temperature":
		return "a temperature outside Safe operating conditions"
	case "degraded":
		return "degraded reliability"
	case "readonly":
		return "that the media was placed in read-only mode"
	case "volatile_mem":
		return "a failed volatile memory backup device"
	case "pmr_readonly":
		return "an unreliable persistent memory region"
	}
	return warning
}

// selftestFinding classifies a self-test status. UDisks2 documents "success"
// both for a passed test and for a test that never ran, so it is not a finding;
// "inprogress" describes the drive's activity rather than its health.
func selftestFinding(status string) (int, string, bool) {
	switch status {
	case "", "success", "inprogress":
		return 0, "", false
	case "aborted", "interrupted", "aborted_format", "aborted_sanitize",
		"aborted_unknown", "ctrl_reset", "ns_removed", "unknown_seg_fail":
		return severityNote, "self-test " + strings.ReplaceAll(status, "_", " "), true
	default:
		// "fatal", "fatal_error" and the "error_*" families: the last self-test
		// ran and reported a fault in the drive.
		return severityBad, "self-test failed (" + strings.ReplaceAll(status, "_", " ") + ")", true
	}
}

// selftestText renders a self-test status for display.
func selftestText(status string) (string, bool) {
	if status == "" {
		return "", false
	}
	text := strings.ReplaceAll(status, "_", " ")
	if text != "" {
		text = strings.ToUpper(text[:1]) + text[1:]
	}
	return text, true
}

// --- D-Bus variant accessors -----------------------------------------
//
// A SMART reading arrives as a variant whose concrete Go type follows the wire
// signature, and the signatures differ between the two interfaces: a Kelvin
// temperature is a double on ATA and a uint16 on NVMe, and the failing-attribute
// count is an int on both. These accessors accept every integer width a
// signature could have produced and keep "absent" distinct from "zero", because
// a drive that reports no reading must not be shown as reporting nothing wrong.

func variantBool(props map[string]dbus.Variant, key string) bool {
	value, ok := props[key]
	if !ok {
		return false
	}
	enabled, ok := value.Value().(bool)
	return ok && enabled
}

func variantString(props map[string]dbus.Variant, key string) string {
	value, ok := props[key]
	if !ok {
		return ""
	}
	text, _ := value.Value().(string)
	return text
}

func variantStringSlice(props map[string]dbus.Variant, key string) []string {
	value, ok := props[key]
	if !ok {
		return nil
	}
	texts, _ := value.Value().([]string)
	return texts
}

func variantFloat(props map[string]dbus.Variant, key string) float64 {
	number, _ := variantNumber(props, key)
	return number
}

func variantIntOr(props map[string]dbus.Variant, key string, fallback int) int {
	number, ok := variantNumber(props, key)
	if !ok {
		return fallback
	}
	return int(number)
}

// variantInt64 reads a signed 64-bit count. It reports whether an integer was
// present at all, which is how "no reading" is kept apart from "zero".
func variantInt64(props map[string]dbus.Variant, key string) (int64, bool) {
	number, ok := variantNumber(props, key)
	if !ok {
		return 0, false
	}
	return int64(number), true
}

func variantUint64Or(props map[string]dbus.Variant, key string, fallback uint64) uint64 {
	number, ok := variantNumber(props, key)
	if !ok || number < 0 {
		return fallback
	}
	return uint64(number)
}

// variantByte reads a value the interface documents as a single byte, such as an
// NVMe percentage. It reports whether the property was usable.
func variantByte(props map[string]dbus.Variant, key string) (byte, bool) {
	number, ok := variantNumber(props, key)
	if !ok || number < 0 || number > 255 {
		return 0, false
	}
	return byte(number), true
}

// variantUint64 reads a value the interface documents as an unsigned 64-bit
// count, such as NVMe bytes written.
func variantUint64(props map[string]dbus.Variant, key string) (uint64, bool) {
	number, ok := variantNumber(props, key)
	if !ok || number < 0 {
		return 0, false
	}
	return uint64(number), true
}

// variantNumber converts any numeric D-Bus value to a float64. Only the integer
// and floating point types a D-Bus signature can produce are accepted.
func variantNumber(props map[string]dbus.Variant, key string) (float64, bool) {
	value, ok := props[key]
	if !ok {
		return 0, false
	}
	switch number := value.Value().(type) {
	case byte:
		return float64(number), true
	case int16:
		return float64(number), true
	case uint16:
		return float64(number), true
	case int32:
		return float64(number), true
	case uint32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint64:
		return float64(number), true
	case float64:
		return number, true
	}
	return 0, false
}

// --- Rows ------------------------------------------------------------

// addSmart adds the SMART rows a drive reports and returns its health verdict.
// It returns nil when the drive reports no SMART at all, so a card is only given
// a badge when there is something behind it.
func addSmart(values map[string]shared.HardwareValue, blockName string) *shared.DeviceStatus {
	info, ok := smartProvider(blockName)
	if !ok || !info.supported {
		return nil
	}

	status := evaluateHealth(info)
	if !info.updated {
		// Nothing to show rows from: the properties are documented as
		// meaningless until udisksd has read the drive.
		return &status
	}

	if info.hasLife {
		values[shared.DiskSmartLife.Key] = shared.NewHardwareValue(info.lifeUsed, shared.Percent, shared.Usage)
	}
	if info.hasWritten {
		values[shared.DiskSmartWritten.Key] = shared.NewHardwareValue(bytesToTB(info.writtenBytes), shared.Terabyte, shared.Memory)
	}
	if info.powerOnHours > 0 {
		values[shared.DiskSmartPowerOn.Key] = shared.NewHardwareValue(float64(info.powerOnHours), shared.UnknownUnit, shared.Usage)
	}
	if info.hasBadSectors {
		values[shared.DiskSmartBadSectors.Key] = shared.NewHardwareValue(float64(info.badSectors), shared.UnknownUnit, shared.Usage)
	}
	if info.hasSpare {
		values[shared.DiskSmartSpare.Key] = shared.NewHardwareValue(float64(info.spare), shared.Percent, shared.Usage)
	}
	if info.hasMediaErrors {
		values[shared.DiskSmartMediaErr.Key] = shared.NewHardwareValue(float64(info.mediaErrors), shared.UnknownUnit, shared.Usage)
	}
	if text, ok := selftestText(info.selftest); ok {
		if info.selftest == "inprogress" && info.selftestPercent >= 0 {
			text = fmt.Sprintf("%s (%d%% remaining)", text, info.selftestPercent)
		}
		values[shared.DiskSmartSelftest.Key] = shared.NewHardwareValue(text, shared.UnknownUnit, shared.Unknown)
	}
	// Only listed when there is one: an empty array is the healthy state and the
	// badge already says so.
	if len(info.criticalWarnings) > 0 {
		warnings := append([]string(nil), info.criticalWarnings...)
		sort.Strings(warnings)
		values[shared.DiskSmartWarnings.Key] = shared.NewHardwareValue(
			strings.Join(warnings, ", "), shared.UnknownUnit, shared.Unknown)
	}

	return &status
}
