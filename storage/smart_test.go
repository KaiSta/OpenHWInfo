package storage

import (
	"strings"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
	"github.com/godbus/dbus/v5"
)

// The SMART tests need no fixture tree: everything below is either a pure
// function over a smartInfo, or the decoding of a D-Bus reply built by hand in
// the exact shapes udisksd sends (variants with the wire signature's Go type).

// healthySATA is what a healthy SATA drive looks like on this machine: SMART
// supported and read, no failing attribute, no bad sector, a self-test that
// succeeded and a wear attribute that has barely moved.
func healthySATA() smartInfo {
	return smartInfo{
		supported:       true,
		updated:         true,
		numFailing:      0,
		numFailedInPast: 1,
		badSectors:      0,
		hasBadSectors:   true,
		selftest:        "success",
		powerOnHours:    2959,
		temperatureC:    23,
		hasTemperature:  true,
		lifeUsed:        4.2,
		hasLife:         true,
	}
}

// healthyNVMe is a young NVMe drive: no critical warning, plenty of spare
// capacity and a wear figure the controller computed itself.
func healthyNVMe() smartInfo {
	return smartInfo{
		supported:      true,
		updated:        true,
		isNVMe:         true,
		selftest:       "success",
		powerOnHours:   120,
		temperatureC:   38,
		hasTemperature: true,
		lifeUsed:       3,
		hasLife:        true,
		writtenBytes:   41_000_000_000_000,
		hasWritten:     true,
		spare:          100,
		spareThreshold: 10,
		hasSpare:       true,
		mediaErrors:    0,
		hasMediaErrors: true,
	}
}

func TestEvaluateHealthVerdicts(t *testing.T) {
	// Each case starts from a healthy drive and breaks exactly one thing, so the
	// level it produces is attributable to that one reading.
	corrupt := func(mutate func(*smartInfo)) smartInfo {
		info := healthySATA()
		mutate(&info)
		return info
	}
	nvmeCorrupt := func(mutate func(*smartInfo)) smartInfo {
		info := healthyNVMe()
		mutate(&info)
		return info
	}

	cases := []struct {
		name       string
		info       smartInfo
		wantLevel  string
		wantInText string // substring the badge must say
		wantDetail string // substring the tooltip must contain
	}{
		{
			name:      "healthy sata",
			info:      healthySATA(),
			wantLevel: shared.StatusGood,
		},
		{
			name:      "healthy nvme",
			info:      healthyNVMe(),
			wantLevel: shared.StatusGood,
		},
		{
			// The drive's own verdict outranks every inference.
			name:       "drive reports failure",
			info:       corrupt(func(i *smartInfo) { i.failing = true }),
			wantLevel:  shared.StatusBad,
			wantInText: "Failing",
			wantDetail: "reports a SMART failure",
		},
		{
			name:       "attribute below threshold",
			info:       corrupt(func(i *smartInfo) { i.numFailing = 2 }),
			wantLevel:  shared.StatusBad,
			wantInText: "Failing",
			wantDetail: "2 SMART attribute(s) are below their threshold",
		},
		{
			name:       "bad sectors",
			info:       corrupt(func(i *smartInfo) { i.badSectors = 8 }),
			wantLevel:  shared.StatusWarn,
			wantDetail: "8 bad sector(s)",
		},
		{
			// A wear figure still below the limit is the app's own reading, so
			// it warns rather than fails.
			name:       "wear above the warning line",
			info:       corrupt(func(i *smartInfo) { i.lifeUsed = 83 }),
			wantLevel:  shared.StatusWarn,
			wantDetail: "83% of the rated endurance is used",
		},
		{
			name:       "wear above the failing line",
			info:       corrupt(func(i *smartInfo) { i.lifeUsed = 97 }),
			wantLevel:  shared.StatusBad,
			wantDetail: "97% of the rated endurance is used",
		},
		{
			name:      "wear just below the warning line is not a finding",
			info:      corrupt(func(i *smartInfo) { i.lifeUsed = 79 }),
			wantLevel: shared.StatusGood,
		},
		{
			// A past failure is a note: it must not turn a currently healthy
			// drive amber, but it must be visible in the explanation.
			name:       "past attribute failure is only a note",
			info:       healthySATA(),
			wantLevel:  shared.StatusGood,
			wantDetail: "1 attribute(s) failed in the past",
		},
		{
			name:       "fatal self-test",
			info:       corrupt(func(i *smartInfo) { i.selftest = "error_read" }),
			wantLevel:  shared.StatusBad,
			wantDetail: "self-test failed (error read)",
		},
		{
			// An aborted test says something about the host, not the drive.
			name:       "aborted self-test is a note",
			info:       corrupt(func(i *smartInfo) { i.selftest = "aborted" }),
			wantLevel:  shared.StatusGood,
			wantDetail: "self-test aborted",
		},
		{
			name:       "nvme degraded reliability",
			info:       nvmeCorrupt(func(i *smartInfo) { i.criticalWarnings = []string{"degraded"} }),
			wantLevel:  shared.StatusBad,
			wantDetail: "degraded reliability",
		},
		{
			name:       "nvme read-only media",
			info:       nvmeCorrupt(func(i *smartInfo) { i.criticalWarnings = []string{"readonly"} }),
			wantLevel:  shared.StatusBad,
			wantDetail: "placed in read-only mode",
		},
		{
			// Low spare is the controller serving notice, not a dead drive.
			name:       "nvme spare warning from the controller",
			info:       nvmeCorrupt(func(i *smartInfo) { i.criticalWarnings = []string{"spare"} }),
			wantLevel:  shared.StatusWarn,
			wantDetail: "available spare capacity is below its threshold",
		},
		{
			name:       "nvme temperature warning from the controller",
			info:       nvmeCorrupt(func(i *smartInfo) { i.criticalWarnings = []string{"temperature"} }),
			wantLevel:  shared.StatusWarn,
			wantDetail: "temperature outside Safe operating conditions",
		},
		{
			name:       "nvme unknown critical warning is surfaced",
			info:       nvmeCorrupt(func(i *smartInfo) { i.criticalWarnings = []string{"pmr_readonly"} }),
			wantLevel:  shared.StatusBad,
			wantDetail: "unreliable persistent memory region",
		},
		{
			// A warning name a future kernel adds must not be silently dropped.
			name:       "unrecognized nvme warning still warns",
			info:       nvmeCorrupt(func(i *smartInfo) { i.criticalWarnings = []string{"something_new"} }),
			wantLevel:  shared.StatusWarn,
			wantDetail: "something_new",
		},
		{
			name:       "nvme spare below threshold",
			info:       nvmeCorrupt(func(i *smartInfo) { i.spare = 5 }),
			wantLevel:  shared.StatusWarn,
			wantDetail: "spare capacity is 5%, below the 10% threshold",
		},
		{
			name:      "nvme spare at the threshold is not a finding",
			info:      nvmeCorrupt(func(i *smartInfo) { i.spare = 10 }),
			wantLevel: shared.StatusGood,
		},
		{
			name:       "nvme media errors",
			info:       nvmeCorrupt(func(i *smartInfo) { i.mediaErrors = 3 }),
			wantLevel:  shared.StatusWarn,
			wantDetail: "3 media error(s)",
		},
		{
			// Zero media errors is the healthy case and must not warn.
			name:      "nvme zero media errors",
			info:      healthyNVMe(),
			wantLevel: shared.StatusGood,
		},
		{
			name:       "smart supported but never read",
			info:       smartInfo{supported: true},
			wantLevel:  shared.StatusUnknown,
			wantInText: "Unknown",
			wantDetail: "no reading of it is available yet",
		},
		{
			name:       "no smart support at all",
			info:       smartInfo{},
			wantLevel:  shared.StatusUnknown,
			wantInText: "Unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := evaluateHealth(tc.info)
			if status.Level != tc.wantLevel {
				t.Errorf("level = %q; want %q (detail: %s)", status.Level, tc.wantLevel, status.Detail)
			}
			if tc.wantInText != "" && !strings.Contains(status.Text, tc.wantInText) {
				t.Errorf("text = %q; want it to contain %q", status.Text, tc.wantInText)
			}
			if tc.wantDetail != "" && !strings.Contains(status.Detail, tc.wantDetail) {
				t.Errorf("detail = %q; want it to contain %q", status.Detail, tc.wantDetail)
			}
			// A verdict always says which protocol it came from.
			if status.Level != shared.StatusUnknown && !strings.HasPrefix(status.Text, "SMART: ") {
				t.Errorf("text = %q; want a SMART: prefix", status.Text)
			}
		})
	}
}

// TestEvaluateHealthExplainsAGoodVerdict pins that a green badge still states
// what it is based on, so "Good" is not an unsupported claim.
func TestEvaluateHealthExplainsAGoodVerdict(t *testing.T) {
	status := evaluateHealth(healthySATA())
	if status.Level != shared.StatusGood {
		t.Fatalf("level = %q; want good", status.Level)
	}
	for _, want := range []string{"0 attribute(s) below threshold", "0 bad sector(s)", "4% endurance used"} {
		if !strings.Contains(status.Detail, want) {
			t.Errorf("detail = %q; want it to contain %q", status.Detail, want)
		}
	}
	// The critical-warning list is an NVMe concept; an ATA explanation must not
	// claim to have checked one.
	if strings.Contains(status.Detail, "controller") {
		t.Errorf("detail = %q; must not mention controller warnings on an ATA drive", status.Detail)
	}
}

// TestEvaluateHealthDoesNotBorrowATAVocabularyForNVMe keeps the explanation
// honest across the two command sets: NVMe has no attribute thresholds and no
// bad-sector counter, so claiming either would be a fabrication.
func TestEvaluateHealthDoesNotBorrowATAVocabularyForNVMe(t *testing.T) {
	detail := evaluateHealth(healthyNVMe()).Detail
	for _, unwanted := range []string{"attribute", "bad sector"} {
		if strings.Contains(detail, unwanted) {
			t.Errorf("detail = %q; must not mention %q on an NVMe drive", detail, unwanted)
		}
	}
	if !strings.Contains(detail, "no controller warnings") {
		t.Errorf("detail = %q; want the controller's clean warning list reported", detail)
	}
}

func TestLifeUsedFrom(t *testing.T) {
	cases := []struct {
		name      string
		value     int
		threshold int
		wantUsed  float64
		wantOK    bool
	}{
		// A new drive reads 100 and has consumed nothing.
		{name: "as new", value: 100, threshold: 5, wantUsed: 0, wantOK: true},
		// The threshold is the drive's declared end of life, so reaching it is
		// 100% consumed rather than (100 - threshold)%.
		{name: "at threshold is fully consumed", value: 5, threshold: 5, wantUsed: 100, wantOK: true},
		{name: "halfway to threshold", value: 50, threshold: 0, wantUsed: 50, wantOK: true},
		{name: "scaled against a threshold of 10", value: 90, threshold: 10, wantUsed: 11.111111, wantOK: true},
		// Unusable inputs must not become a number: 0 and below are UDisks2's
		// "unknown", and above 100 is not a percentage.
		{name: "zero means unknown", value: 0, threshold: 5, wantOK: false},
		{name: "negative means unknown", value: -1, threshold: 5, wantOK: false},
		{name: "above 100 is not a percentage", value: 253, threshold: 5, wantOK: false},
		// A threshold outside the range carries no information, so the value is
		// read as a plain percentage instead of scaling it by nonsense.
		{name: "threshold of 100 falls back", value: 40, threshold: 100, wantUsed: 60, wantOK: true},
		{name: "threshold of 200 falls back", value: 40, threshold: 200, wantUsed: 60, wantOK: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			used, ok := lifeUsedFrom(tc.value, tc.threshold)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v; want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if diff := used - tc.wantUsed; diff > 0.001 || diff < -0.001 {
				t.Errorf("used = %v; want %v", used, tc.wantUsed)
			}
		})
	}
}

// TestLifeFromAttributes uses the attribute names and numbers of the review
// machine's own SSD, whose wear attribute reads 96 against a threshold of 5.
func TestLifeFromAttributes(t *testing.T) {
	attrs := []ataAttribute{
		{ID: 9, Name: "power-on-hours", Value: 99, Worst: 99, Threshold: 0},
		{ID: 187, Name: "reported-uncorrect", Value: 100, Threshold: 0},
		{ID: 177, Name: "wear-leveling-count", Value: 96, Worst: 96, Threshold: 5},
	}

	used, ok := lifeFromAttributes(attrs)
	if !ok {
		t.Fatal("expected the wear attribute to be found")
	}
	// (100 - 96) / (100 - 5) = 4.2%
	if diff := used - 4.210526; diff > 0.001 || diff < -0.001 {
		t.Errorf("used = %v; want 4.21", used)
	}
}

// TestLifeFromAttributesIgnoresUnknownTables covers the two ways a table can
// fail to yield a wear figure: no recognised attribute, and a recognised one
// whose value is UDisks2's "unknown".
func TestLifeFromAttributesIgnoresUnknownTables(t *testing.T) {
	noLife := []ataAttribute{
		{ID: 5, Name: "reallocated-sector-count", Value: 100, Threshold: 36},
	}
	if _, ok := lifeFromAttributes(noLife); ok {
		t.Error("a table with no life attribute must not yield a wear figure")
	}

	unknownValue := []ataAttribute{
		{ID: 231, Name: "ssd-life-left", Value: -1, Threshold: 10},
	}
	if _, ok := lifeFromAttributes(unknownValue); ok {
		t.Error("an unknown life value must not yield a wear figure")
	}
}

// TestLifeAttributeNamesPinTheAllowlist documents which attribute names are
// interpreted. Vendors reuse attribute ids and normalize them differently, so
// only names whose normalized value counts down as the media wears may be read
// as endurance; adding one here is a deliberate act, not an accident.
func TestLifeAttributeNamesPinTheAllowlist(t *testing.T) {
	want := []string{"ssd-life-left", "media-wearout-indicator", "wear-leveling-count"}
	if len(lifeAttributes) != len(want) {
		t.Fatalf("lifeAttributes = %v; want %v", lifeAttributes, want)
	}
	for i, name := range want {
		if lifeAttributes[i] != name {
			t.Errorf("lifeAttributes[%d] = %q; want %q", i, lifeAttributes[i], name)
		}
		if !isLifeAttribute(name) {
			t.Errorf("%q must be recognised", name)
		}
	}
	// "percent-lifetime-remain" is deliberately absent: some drives put the wear
	// figure in its raw value and leave the normalized value at 100, which would
	// report a worn-out drive as barely used.
	if isLifeAttribute("percent-lifetime-remain") {
		t.Error("percent-lifetime-remain must not be interpreted as endurance")
	}
}

// TestWrittenFromAttributes pins how attribute 241's count becomes bytes. The
// count is what the drive reports and the unit is the vendor's, so the platform's
// SMART library is the only thing that can describe the latter; a unit that is
// not a byte count must yield nothing rather than a converted figure.
func TestWrittenFromAttributes(t *testing.T) {
	// The review machine's own drive would report 20191042413 sectors here
	// (10.34 TB) if it carried the attribute at all.
	cases := []struct {
		name     string
		attr     ataAttribute
		want     uint64
		wantRead bool
	}{
		{
			name:     "sectors, the ATA definition",
			attr:     ataAttribute{ID: 241, Name: "total-lbas-written", Pretty: 20_191_042_413, PrettyUnit: ataUnitSectors},
			want:     20_191_042_413 * 512,
			wantRead: true,
		},
		{
			name:     "a bare count",
			attr:     ataAttribute{ID: 241, Name: "attribute-241", Pretty: 1_000_000, PrettyUnit: ataUnitNone},
			want:     512_000_000,
			wantRead: true,
		},
		{
			name:     "a unit the library does not name",
			attr:     ataAttribute{ID: 241, Name: "total-lbas-written", Pretty: 1_000_000, PrettyUnit: ataUnitUnknown},
			want:     512_000_000,
			wantRead: true,
		},
		{
			name:     "megabytes",
			attr:     ataAttribute{ID: 241, Name: "total-lbas-written", Pretty: 4_000_000, PrettyUnit: ataUnitMB},
			want:     4_000_000_000_000,
			wantRead: true,
		},
		{
			name: "a unit that is not a byte count",
			attr: ataAttribute{ID: 241, Name: "total-lbas-written", Pretty: 299_150, PrettyUnit: ataUnitMkelvin},
		},
		{
			name: "a counter the drive reports as zero",
			attr: ataAttribute{ID: 241, Name: "total-lbas-written", Pretty: 0, PrettyUnit: ataUnitSectors},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bytes, ok := writtenFromAttributes([]ataAttribute{
				{ID: 9, Name: "power-on-hours", Pretty: 10652400000, PrettyUnit: ataUnitMseconds},
				tc.attr,
			})
			if ok != tc.wantRead {
				t.Fatalf("read = %v; want %v", ok, tc.wantRead)
			}
			if bytes != tc.want {
				t.Errorf("bytes = %d; want %d", bytes, tc.want)
			}
		})
	}
}

// TestWrittenFromAttributesIgnoresOtherCounters is the guard against the
// neighbouring attributes: 242 is the read counter and 177 is endurance, and
// neither is a lifetime write count.
func TestWrittenFromAttributesIgnoresOtherCounters(t *testing.T) {
	attrs := []ataAttribute{
		{ID: 242, Name: "total-lbas-read", Pretty: 5_000_000_000, PrettyUnit: ataUnitSectors},
		{ID: 177, Name: "wear-leveling-count", Pretty: 40, PrettyUnit: ataUnitNone},
		{ID: 199, Name: "udma-crc-error-count", Pretty: 3, PrettyUnit: ataUnitNone},
	}
	if bytes, ok := writtenFromAttributes(attrs); ok {
		t.Errorf("bytes = %d; want no figure from attributes that are not 241", bytes)
	}
}

func TestAtaSmartInfoReadsProperties(t *testing.T) {
	// The shapes udisksd actually sends: the failing flag is a boolean, the
	// counts are ints, the power-on time a uint64 of seconds and the
	// temperature a double in Kelvin.
	props := map[string]dbus.Variant{
		"SmartSupported":                    dbus.MakeVariant(true),
		"SmartEnabled":                      dbus.MakeVariant(true),
		"SmartUpdated":                      dbus.MakeVariant(uint64(1789214776)),
		"SmartFailing":                      dbus.MakeVariant(false),
		"SmartPowerOnSeconds":               dbus.MakeVariant(uint64(10652400)),
		"SmartTemperature":                  dbus.MakeVariant(296.15),
		"SmartNumAttributesFailing":         dbus.MakeVariant(int32(0)),
		"SmartNumAttributesFailedInThePast": dbus.MakeVariant(int32(1)),
		"SmartNumBadSectors":                dbus.MakeVariant(int64(0)),
		"SmartSelftestStatus":               dbus.MakeVariant("success"),
		"SmartSelftestPercentRemaining":     dbus.MakeVariant(int32(0)),
	}

	info := ataSmartInfo(props)
	if !info.supported || !info.updated {
		t.Fatalf("supported = %v, updated = %v; want both true", info.supported, info.updated)
	}
	if info.failing || info.numFailing != 0 || info.numFailedInPast != 1 {
		t.Errorf("failing/numFailing/numFailedInPast = %v/%d/%d; want false/0/1",
			info.failing, info.numFailing, info.numFailedInPast)
	}
	if !info.hasBadSectors || info.badSectors != 0 {
		t.Errorf("badSectors = %v (has = %v); want a readable 0", info.badSectors, info.hasBadSectors)
	}
	if info.powerOnHours != 2959 {
		t.Errorf("powerOnHours = %d; want 2959", info.powerOnHours)
	}
	if !info.hasTemperature || info.temperatureC < 22.99 || info.temperatureC > 23.01 {
		t.Errorf("temperature = %v (has = %v); want 23.0", info.temperatureC, info.hasTemperature)
	}
	if info.selftest != "success" {
		t.Errorf("selftest = %q; want success", info.selftest)
	}
}

// TestAtaSmartInfoKeepsAbsentBadSectorsAbsent is the fabrication guard: UDisks2
// omits figures it could not read, and an omitted count must never be rendered
// as "0 bad sectors", which would be a clean bill of health the drive never
// gave.
func TestAtaSmartInfoKeepsAbsentBadSectorsAbsent(t *testing.T) {
	info := ataSmartInfo(map[string]dbus.Variant{
		"SmartSupported": dbus.MakeVariant(true),
		"SmartUpdated":   dbus.MakeVariant(uint64(1789214776)),
		// SmartNumBadSectors deliberately absent.
	})
	if info.hasBadSectors {
		t.Errorf("badSectors = %v marked as read; want it absent", info.badSectors)
	}
	if info.badSectors != 0 {
		t.Errorf("badSectors = %v; want the zero value while absent", info.badSectors)
	}
}

func TestAtaSmartInfoUnsupportedDrive(t *testing.T) {
	// A USB bridge or an old drive: SMART is simply not there.
	info := ataSmartInfo(map[string]dbus.Variant{
		"SmartSupported": dbus.MakeVariant(false),
		"SmartUpdated":   dbus.MakeVariant(uint64(0)),
	})
	if info.supported || info.updated {
		t.Errorf("supported = %v, updated = %v; want both false", info.supported, info.updated)
	}
}

// TestAtaSmartInfoNeverReadIsNotAHealthyDrive covers the state UDisks2
// documents explicitly: every SMART property is meaningless while SmartUpdated
// is 0, so a drive that has never been read must not be reported as healthy.
func TestAtaSmartInfoNeverReadIsNotAHealthyDrive(t *testing.T) {
	info := ataSmartInfo(map[string]dbus.Variant{
		"SmartSupported":     dbus.MakeVariant(true),
		"SmartUpdated":       dbus.MakeVariant(uint64(0)),
		"SmartFailing":       dbus.MakeVariant(false),
		"SmartNumBadSectors": dbus.MakeVariant(int64(0)),
	})
	if info.updated {
		t.Fatal("updated = true; want false while SmartUpdated is 0")
	}
	if status := evaluateHealth(info); status.Level != shared.StatusUnknown {
		t.Errorf("level = %q; want unknown", status.Level)
	}
}

func TestNvmeSmartInfoReadsProperties(t *testing.T) {
	props := map[string]dbus.Variant{
		"SmartUpdated":         dbus.MakeVariant(uint64(1789214776)),
		"SmartCriticalWarning": dbus.MakeVariant([]string{"spare"}),
		"SmartPowerOnHours":    dbus.MakeVariant(uint64(1200)),
		"SmartTemperature":     dbus.MakeVariant(uint16(311)), // 311 K = 37.85 C
		"SmartSelftestStatus":  dbus.MakeVariant("success"),
	}

	info := nvmeSmartInfo(props)
	if !info.supported || !info.updated || !info.isNVMe {
		t.Fatalf("supported/updated/isNVMe = %v/%v/%v; want all true",
			info.supported, info.updated, info.isNVMe)
	}
	if info.powerOnHours != 1200 {
		t.Errorf("powerOnHours = %d; want 1200", info.powerOnHours)
	}
	if !info.hasTemperature || info.temperatureC < 37.84 || info.temperatureC > 37.86 {
		t.Errorf("temperature = %v; want 37.85", info.temperatureC)
	}
	if len(info.criticalWarnings) != 1 || info.criticalWarnings[0] != "spare" {
		t.Errorf("criticalWarnings = %v; want [spare]", info.criticalWarnings)
	}
}

// TestNvmeZeroTemperatureIsUnknown pins the Kelvin convention: 0 is what the
// specification uses for "no reading", and it must not become -273.15 C.
func TestNvmeZeroTemperatureIsUnknown(t *testing.T) {
	info := nvmeSmartInfo(map[string]dbus.Variant{
		"SmartUpdated":     dbus.MakeVariant(uint64(1)),
		"SmartTemperature": dbus.MakeVariant(uint16(0)),
	})
	if info.hasTemperature {
		t.Errorf("temperature = %v marked as read; want unknown", info.temperatureC)
	}
}

func TestApplyNvmeAttributes(t *testing.T) {
	// The NVMe dictionary, with the wire types the specification dictates:
	// percentages are bytes, byte counts are uint64.
	values := map[string]dbus.Variant{
		"percent_used":       dbus.MakeVariant(byte(7)),
		"avail_spare":        dbus.MakeVariant(byte(98)),
		"spare_thresh":       dbus.MakeVariant(byte(10)),
		"total_data_written": dbus.MakeVariant(uint64(12_000_000_000_000)),
		"media_errors":       dbus.MakeVariant(uint64(2)),
	}

	info := applyNvmeAttributes(smartInfo{supported: true, updated: true}, values)
	if !info.hasLife || info.lifeUsed != 7 {
		t.Errorf("life = %v (has = %v); want 7", info.lifeUsed, info.hasLife)
	}
	if !info.hasWritten || info.writtenBytes != 12_000_000_000_000 {
		t.Errorf("written = %v (has = %v); want 12 TB", info.writtenBytes, info.hasWritten)
	}
	if !info.hasSpare || info.spare != 98 || info.spareThreshold != 10 {
		t.Errorf("spare = %d/%d (has = %v); want 98 with threshold 10",
			info.spare, info.spareThreshold, info.hasSpare)
	}
	if !info.hasMediaErrors || info.mediaErrors != 2 {
		t.Errorf("mediaErrors = %d (has = %v); want 2", info.mediaErrors, info.hasMediaErrors)
	}
	// Everything present, so nothing may be invented.
	if !info.hasLife || !info.hasWritten || !info.hasSpare || !info.hasMediaErrors {
		t.Error("every attribute was present and must have been read")
	}
}

// TestApplyNvmeAttributesLeavesAbsentFieldsUnread is the fabrication guard for
// the NVMe path: an empty dictionary must leave every reading absent, not zero.
func TestApplyNvmeAttributesLeavesAbsentFieldsUnread(t *testing.T) {
	info := applyNvmeAttributes(smartInfo{supported: true, updated: true}, map[string]dbus.Variant{})
	if info.hasLife || info.hasWritten || info.hasSpare || info.hasMediaErrors {
		t.Errorf("empty attributes produced readings: life=%v written=%v spare=%v media=%v",
			info.hasLife, info.hasWritten, info.hasSpare, info.hasMediaErrors)
	}
}

// TestApplyNvmeAttributesDefaultsTheSpareThreshold covers a controller that
// reports spare capacity without the threshold it should be compared against.
func TestApplyNvmeAttributesDefaultsTheSpareThreshold(t *testing.T) {
	info := applyNvmeAttributes(smartInfo{}, map[string]dbus.Variant{
		"avail_spare": dbus.MakeVariant(byte(4)),
	})
	if info.spareThreshold != defaultSpareThreshold {
		t.Errorf("spareThreshold = %d; want the %d%% default", info.spareThreshold, defaultSpareThreshold)
	}
	// 4% spare against the default 10% threshold is a warning.
	if status := evaluateHealth(smartInfo{supported: true, updated: true, isNVMe: true,
		hasSpare: true, spare: info.spare, spareThreshold: info.spareThreshold}); status.Level != shared.StatusWarn {
		t.Errorf("level = %q; want warn", status.Level)
	}
}

func TestVariantNumberAcceptsEveryWireType(t *testing.T) {
	// A D-Bus signature decides the Go type a number arrives as, and the two
	// SMART interfaces use different ones for the same kind of figure.
	cases := []struct {
		name  string
		value interface{}
	}{
		{name: "byte", value: byte(7)},
		{name: "int16", value: int16(-1)},
		{name: "uint16", value: uint16(311)},
		{name: "int32", value: int32(0)},
		{name: "uint32", value: uint32(42)},
		{name: "int64", value: int64(9007199254740993)},
		{name: "uint64", value: uint64(10652400)},
		{name: "float64", value: 296.15},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			props := map[string]dbus.Variant{"v": dbus.MakeVariant(tc.value)}
			if _, ok := variantNumber(props, "v"); !ok {
				t.Errorf("%T was not accepted", tc.value)
			}
		})
	}

	// Absent and non-numeric must be refused rather than read as zero.
	if _, ok := variantNumber(map[string]dbus.Variant{}, "v"); ok {
		t.Error("an absent property must not yield a number")
	}
	if _, ok := variantNumber(map[string]dbus.Variant{"v": dbus.MakeVariant("text")}, "v"); ok {
		t.Error("a string must not yield a number")
	}
}

func TestVariantUint64AndByteRejectOutOfRangeValues(t *testing.T) {
	negative := map[string]dbus.Variant{"v": dbus.MakeVariant(int32(-1))}
	if _, ok := variantUint64(negative, "v"); ok {
		t.Error("a negative count must not become an unsigned total")
	}
	tooBig := map[string]dbus.Variant{"v": dbus.MakeVariant(uint64(300))}
	if _, ok := variantByte(tooBig, "v"); ok {
		t.Error("a value above 255 must not become a percentage")
	}
}

// --- Row contract ----------------------------------------------------

// TestAddSmartEmitsTheExpectedRows pins both the presence and the unit of every
// row a fully reporting drive produces, because the frontend scales a row by
// its key and prints the unit in its label.
func TestAddSmartEmitsTheExpectedRows(t *testing.T) {
	info := healthyNVMe()
	info.badSectors, info.hasBadSectors = 3, true

	values := make(map[string]shared.HardwareValue)
	status := addSmartFrom(t, info, values)

	if status == nil {
		t.Fatal("a drive reporting SMART must get a verdict")
	}
	if status.Level != shared.StatusWarn {
		t.Errorf("level = %q; want warn from the 3 bad sectors", status.Level)
	}

	want := map[string]struct {
		value float64
		unit  shared.HardwareUnit
	}{
		// Lifetime bytes are converted to the decimal terabytes total writes are
		// quoted in (the TBW of an endurance rating), while capacity and
		// filesystem sizes stay in gigabytes.
		"disk.smart.written":     {41, shared.Terabyte},
		"disk.smart.life":        {3, shared.Percent},
		"disk.smart.hours":       {120, shared.UnknownUnit},
		"disk.smart.badSectors":  {3, shared.UnknownUnit},
		"disk.smart.spare":       {100, shared.Percent},
		"disk.smart.mediaErrors": {0, shared.UnknownUnit},
	}
	for key, expectation := range want {
		value, ok := values[key]
		if !ok {
			t.Errorf("%s was not collected", key)
			continue
		}
		if value.Unit != expectation.unit {
			t.Errorf("%s unit = %v; want %v", key, value.Unit, expectation.unit)
		}
		number, ok := value.Value.(float64)
		if !ok {
			t.Errorf("%s = %T; want a float64", key, value.Value)
			continue
		}
		if number != expectation.value {
			t.Errorf("%s = %v; want %v", key, number, expectation.value)
		}
	}

	// The self-test is text, and a passing test is still worth a row.
	if text, ok := values["disk.smart.selftest"].Value.(string); !ok || text != "Success" {
		t.Errorf("disk.smart.selftest = %v; want \"Success\"", values["disk.smart.selftest"].Value)
	}
	// No critical warning, so no warnings row: an empty array is the healthy
	// state and the badge already reports it.
	if _, ok := values["disk.smart.warnings"]; ok {
		t.Error("a drive with no critical warning must not get a warnings row")
	}
}

// TestAddSmartReportsALifetimeWriteCountInTerabytes covers the ATA half of the
// written row: it is the same row an NVMe drive gets, so both command sets are
// reported in the unit total writes are quoted in.
func TestAddSmartReportsALifetimeWriteCountInTerabytes(t *testing.T) {
	values := make(map[string]shared.HardwareValue)
	info := healthySATA()
	// 10.34 TB, the figure the review host's drive reports over smartctl.
	info.writtenBytes, info.hasWritten = 10_337_813_555_456, true

	addSmartFrom(t, info, values)

	written, ok := values[shared.DiskSmartWritten.Key]
	if !ok {
		t.Fatal("a drive reporting a lifetime write count must get the row")
	}
	if written.Unit != shared.Terabyte {
		t.Errorf("unit = %v; want %v", written.Unit, shared.Terabyte)
	}
	number, ok := written.Value.(float64)
	if !ok {
		t.Fatalf("value = %T; want a float64", written.Value)
	}
	if diff := number - 10.34; diff > 0.005 || diff < -0.005 {
		t.Errorf("value = %v; want 10.34", number)
	}
}

// TestAddSmartOmitsRowsTheDriveDoesNotReport is the other half of the contract:
// a drive that reports only some figures gets only those rows, so a SATA SSD
// shows no spare capacity and an HDD shows no wear.
func TestAddSmartOmitsRowsTheDriveDoesNotReport(t *testing.T) {
	values := make(map[string]shared.HardwareValue)
	info := smartInfo{
		supported:     true,
		updated:       true,
		hasBadSectors: true,
		badSectors:    0,
		selftest:      "success",
		powerOnHours:  500,
	}

	if status := addSmartFrom(t, info, values); status == nil {
		t.Fatal("expected a verdict")
	}

	for _, key := range []string{"disk.smart.life", "disk.smart.written", "disk.smart.spare",
		"disk.smart.mediaErrors", "disk.smart.warnings"} {
		if _, ok := values[key]; ok {
			t.Errorf("%s was invented for a drive that does not report it", key)
		}
	}
	for _, key := range []string{"disk.smart.hours", "disk.smart.badSectors", "disk.smart.selftest"} {
		if _, ok := values[key]; !ok {
			t.Errorf("%s is reported and must be collected", key)
		}
	}
}

// TestAddSmartReportsUnknownWithoutRows covers the window between "the drive
// supports SMART" and "udisksd has read it": there is a verdict to show, but no
// reading behind it, so the badge says so rather than the rows being fabricated.
func TestAddSmartReportsUnknownWithoutRows(t *testing.T) {
	values := make(map[string]shared.HardwareValue)
	status := addSmartFrom(t, smartInfo{supported: true}, values)

	if status == nil {
		t.Fatal("a SMART-capable drive must still get a verdict")
	}
	if status.Level != shared.StatusUnknown {
		t.Errorf("level = %q; want unknown", status.Level)
	}
	if len(values) != 0 {
		t.Errorf("values = %v; want none while no reading is available", values)
	}
}

func TestAddSmartWithoutSupportGetsNoVerdict(t *testing.T) {
	values := make(map[string]shared.HardwareValue)
	if status := addSmartFrom(t, smartInfo{}, values); status != nil {
		t.Errorf("status = %+v; want none for a drive with no SMART", status)
	}
	if len(values) != 0 {
		t.Errorf("values = %v; want none", values)
	}
}

// TestAddSmartRowsAllHaveTemplates is the contract this package cannot enforce
// alone: a row is only rendered when a template names its key, so a collected
// key with no template is a silently dropped metric. It is the bug class behind
// the missing CPU temperature this project was reported for.
func TestAddSmartRowsAllHaveTemplates(t *testing.T) {
	templates := map[string]bool{}
	for _, template := range []shared.KeyTemplate{
		shared.DiskModel, shared.DiskSerial, shared.DiskFirmware, shared.DiskMedium, shared.DiskSize,
		shared.DiskFSTotal, shared.DiskFSUsed, shared.DiskFSFree,
		shared.DiskSmartLife, shared.DiskSmartWritten, shared.DiskSmartPowerOn,
		shared.DiskSmartBadSectors, shared.DiskSmartSpare, shared.DiskSmartMediaErr,
		shared.DiskSmartSelftest, shared.DiskSmartWarnings,
	} {
		templates[template.Key] = true
	}

	// Every row this package can produce, from the widest possible reading.
	info := healthyNVMe()
	info.badSectors, info.hasBadSectors = 1, true
	info.selftest = "inprogress"
	info.selftestPercent = 40
	info.criticalWarnings = []string{"spare"}

	values := make(map[string]shared.HardwareValue)
	addSmartFrom(t, info, values)
	// The temperature the SMART properties carry also lands on the card.
	values["disk.temp0"] = shared.NewHardwareValue(info.temperatureC, shared.Celsius, shared.Temperature)

	for key := range values {
		switch {
		case key == "disk.temp0":
			// Generated from the collected sensors rather than the fixed list.
		case strings.HasPrefix(key, tempLabelKeyPrefix):
			t.Errorf("label key %q must stay out of the rows", key)
		case !templates[key]:
			t.Errorf("collected key %q has no template and would never be displayed", key)
		}
	}

	// And the reverse: a template whose key is never collected is dead weight,
	// which is what the row-omission above exists to prevent.
	if _, ok := values["disk.smart.warnings"]; !ok {
		t.Error("a drive with a critical warning must produce the warnings row")
	}
}

// addSmartFrom runs the row builder against a stub provider, because addSmart
// itself takes a block name and would otherwise reach the host's D-Bus.
func addSmartFrom(t *testing.T, info smartInfo, values map[string]shared.HardwareValue) *shared.DeviceStatus {
	t.Helper()
	original := smartProvider
	smartProvider = func(string) (smartInfo, bool) { return info, true }
	t.Cleanup(func() { smartProvider = original })
	return addSmart(values, "sda")
}

// TestAddSmartWithAnUnknownDriveIsSilent covers the provider reporting that
// UDisks2 does not know the device at all.
func TestAddSmartWithAnUnknownDriveIsSilent(t *testing.T) {
	original := smartProvider
	smartProvider = func(string) (smartInfo, bool) { return smartInfo{}, false }
	t.Cleanup(func() { smartProvider = original })

	values := make(map[string]shared.HardwareValue)
	if status := addSmart(values, "sda"); status != nil {
		t.Errorf("status = %+v; want none for a drive UDisks2 does not know", status)
	}
}
