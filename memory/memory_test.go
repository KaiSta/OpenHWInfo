package memory

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// writeFixture points the collector at a temporary /proc/meminfo, a temporary
// pressure file and a temporary block directory. Leaving psi empty models a
// kernel without PSI, where the file does not exist at all; the block directory
// starts empty, which is a machine without zram, and zram tests add devices to
// it with writeZramDevice. It returns the fixture directory.
func writeFixture(t *testing.T, meminfo, psi string) string {
	t.Helper()

	dir := t.TempDir()
	blockDir := filepath.Join(dir, "block")
	if err := os.MkdirAll(blockDir, 0o755); err != nil {
		t.Fatalf("failed to create %s: %v", blockDir, err)
	}

	originalMeminfo, originalPSI, originalZram := meminfoPath, psiPath, zramPath
	meminfoPath = filepath.Join(dir, "meminfo")
	psiPath = filepath.Join(dir, "pressure")
	zramPath = blockDir
	t.Cleanup(func() {
		meminfoPath, psiPath, zramPath = originalMeminfo, originalPSI, originalZram
	})

	if err := os.WriteFile(meminfoPath, []byte(meminfo), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", meminfoPath, err)
	}
	if psi != "" {
		if err := os.WriteFile(psiPath, []byte(psi), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", psiPath, err)
		}
	}
	return dir
}

// writeZramDevice writes one device's sysfs attributes into the fixture's block
// directory, the way the kernel exposes them under /sys/block/zramN. An empty
// argument leaves that file out, which is how the tests model a device whose
// mm_stat (or algorithm) cannot be read.
func writeZramDevice(t *testing.T, dir, name, disksize, mmStat, algorithm string) {
	t.Helper()

	deviceDir := filepath.Join(dir, "block", name)
	if err := os.MkdirAll(deviceDir, 0o755); err != nil {
		t.Fatalf("failed to create %s: %v", deviceDir, err)
	}
	for file, content := range map[string]string{
		"disksize":       disksize,
		"mm_stat":        mmStat,
		"comp_algorithm": algorithm,
	} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(deviceDir, file), []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", filepath.Join(deviceDir, file), err)
		}
	}
}

// writeMissingFixture points the collector at paths that do not exist, so
// nothing can be read.
func writeMissingFixture(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	originalMeminfo, originalPSI, originalZram := meminfoPath, psiPath, zramPath
	meminfoPath = filepath.Join(dir, "absent-meminfo")
	psiPath = filepath.Join(dir, "absent-pressure")
	zramPath = filepath.Join(dir, "absent-block")
	t.Cleanup(func() {
		meminfoPath, psiPath, zramPath = originalMeminfo, originalPSI, originalZram
	})
}

// sixteenGiB is a machine with 16 GiB of RAM and 8 GiB of swap, of which 2 GiB
// is in use — the shape the size and percentage assertions below are written
// against.
const sixteenGiB = `MemTotal:       16777216 kB
MemFree:         1048576 kB
MemAvailable:    4194304 kB
Buffers:           65536 kB
Cached:          2097152 kB
SwapTotal:       8388608 kB
SwapFree:        6291456 kB
`

const pressureBoth = `some avg10=0.34 avg60=0.20 avg300=0.10 total=12345
full avg10=0.00 avg60=0.00 avg300=0.00 total=0
`

// row returns a reported value by key, failing the test when it is absent.
func row(t *testing.T, data shared.HardwareData, key string) shared.HardwareValue {
	t.Helper()
	value, ok := data.Values[key]
	if !ok {
		t.Fatalf("key %q missing from the memory card (got keys %v)", key, keysOf(data))
	}
	return value
}

func keysOf(data shared.HardwareData) []string {
	out := make([]string, 0, len(data.Values))
	for key := range data.Values {
		out = append(out, key)
	}
	return out
}

// number reads a row the way the UI does: through the numeric accessor, so a
// value that could not be rendered as a number would fail here.
func number(t *testing.T, data shared.HardwareData, key string) float64 {
	t.Helper()
	value := row(t, data, key)
	got, err := value.GetNumericValue()
	if err != nil {
		t.Fatalf("key %q: cannot convert %v to numeric: %v", key, value.Value, err)
	}
	return got
}

// TestGetInformationsReportsMemoryInGiB covers the common machine and pins both
// the arithmetic and the units: /proc/meminfo counts in KiB, a 16 GiB machine
// reports 16777216 there, and the row must read exactly 16 GiB.
func TestGetInformationsReportsMemoryInGiB(t *testing.T) {
	writeFixture(t, sixteenGiB, pressureBoth)

	data := GetInformations()

	if data.Name != "Memory" {
		t.Errorf("Name = %q; want %q", data.Name, "Memory")
	}

	checks := map[string]float64{
		"mem.total":     16, // 16777216 kB
		"mem.used":      12, // total - available
		"mem.available": 4,  // MemAvailable
		"mem.usage":     75, // 12 of 16
		"swap.total":    8,
		"swap.used":     2,  // 8 - 6
		"swap.usage":    25, // 2 of 8
	}
	for key, want := range checks {
		if got := number(t, data, key); math.Abs(got-want) > 0.001 {
			t.Errorf("%s = %v; want %v", key, got, want)
		}
	}
}

// TestGetInformationsUnitsMatchTheRows pins the unit contract: a size is GiB and
// a share is percent, because the unit — not the row label — is what the value
// is rendered with. A size tagged Percent would print "16.00 %" beside a 16 GiB
// reading.
func TestGetInformationsUnitsMatchTheRows(t *testing.T) {
	writeFixture(t, sixteenGiB, pressureBoth)

	data := GetInformations()

	want := map[string]shared.HardwareUnit{
		"mem.total":         shared.Gibibyte,
		"mem.used":          shared.Gibibyte,
		"mem.available":     shared.Gibibyte,
		"mem.usage":         shared.Percent,
		"swap.total":        shared.Gibibyte,
		"swap.used":         shared.Gibibyte,
		"swap.usage":        shared.Percent,
		"mem.pressure.some": shared.Percent,
		"mem.pressure.full": shared.Percent,
	}
	for key, unit := range want {
		if got := row(t, data, key).Unit; got != unit {
			t.Errorf("%s unit = %v; want %v", key, got, unit)
		}
	}
}

// TestGetInformationsFormatsSizesWithTheirUnit is the end-to-end check of what
// the user actually reads: the value carries "GiB", so the row cannot claim to
// be a percentage or a bare number.
func TestGetInformationsFormatsSizesWithTheirUnit(t *testing.T) {
	writeFixture(t, sixteenGiB, pressureBoth)

	data := GetInformations()

	if got, want := data.Values["mem.total"].Unit.Suffix(), "GiB"; got != want {
		t.Errorf("mem.total suffix = %q; want %q", got, want)
	}
	if got, want := data.Values["mem.usage"].Unit.Suffix(), "%"; got != want {
		t.Errorf("mem.usage suffix = %q; want %q", got, want)
	}
}

// TestGetInformationsReadsMemoryPressure covers the PSI rows: the kernel's own
// stall accounting, in percent of the last 10 seconds.
func TestGetInformationsReadsMemoryPressure(t *testing.T) {
	writeFixture(t, sixteenGiB, pressureBoth)

	data := GetInformations()

	if got := number(t, data, "mem.pressure.some"); math.Abs(got-0.34) > 0.0001 {
		t.Errorf("mem.pressure.some = %v; want 0.34", got)
	}
	// "full" being zero is still a reading: the kernel reports it, so it is a
	// row saying the machine was never fully stalled.
	if got := number(t, data, "mem.pressure.full"); got != 0 {
		t.Errorf("mem.pressure.full = %v; want 0", got)
	}
}

// TestGetInformationsWithoutKernelPSI covers a kernel built without PSI: the
// file does not exist, so no pressure rows are reported rather than zeroes.
func TestGetInformationsWithoutKernelPSI(t *testing.T) {
	writeFixture(t, sixteenGiB, "")

	data := GetInformations()

	for _, key := range []string{"mem.pressure.some", "mem.pressure.full"} {
		if _, exists := data.Values[key]; exists {
			t.Errorf("unexpected %s on a kernel without PSI", key)
		}
	}
	if _, exists := data.Values["mem.total"]; !exists {
		t.Error("expected the memory rows to be unaffected by a missing PSI file")
	}
}

// TestGetInformationsOmitsSwapWhenNotConfigured covers the swapless machine. A
// system with no swap reports SwapTotal 0, and three zero rows would be noise
// rather than information.
func TestGetInformationsOmitsSwapWhenNotConfigured(t *testing.T) {
	writeFixture(t, `MemTotal:       16777216 kB
MemAvailable:    4194304 kB
SwapTotal:             0 kB
SwapFree:              0 kB
`, "")

	data := GetInformations()

	for _, key := range []string{"swap.total", "swap.used", "swap.usage"} {
		if _, exists := data.Values[key]; exists {
			t.Errorf("unexpected %s on a machine with no swap", key)
		}
	}
	if got := number(t, data, "mem.total"); got != 16 {
		t.Errorf("mem.total = %v; want 16", got)
	}
}

// TestGetInformationsOmitsSwapWhenItsFreeCountIsMissing covers a half-reported
// swap: without SwapFree the used figure cannot be derived, and reporting the
// whole of swap as used would be a fabricated reading.
func TestGetInformationsOmitsSwapWhenItsFreeCountIsMissing(t *testing.T) {
	writeFixture(t, `MemTotal:       16777216 kB
MemAvailable:    4194304 kB
SwapTotal:       8388608 kB
`, "")

	data := GetInformations()

	if _, exists := data.Values["swap.total"]; !exists {
		t.Error("expected swap.total to be reported")
	}
	for _, key := range []string{"swap.used", "swap.usage"} {
		if _, exists := data.Values[key]; exists {
			t.Errorf("unexpected %s when SwapFree could not be read", key)
		}
	}
}

// TestGetInformationsFallsBackWithoutMemAvailable covers kernels before 3.14:
// the availability figure is then derived the way those kernels' own tooling
// derived it, from the free memory plus the reclaimable caches.
func TestGetInformationsFallsBackWithoutMemAvailable(t *testing.T) {
	writeFixture(t, `MemTotal:       1048576 kB
MemFree:         524288 kB
Buffers:          65536 kB
Cached:          131072 kB
SReclaimable:     65536 kB
`, "")

	data := GetInformations()

	// 524288 + 65536 + 131072 + 65536 = 786432 kB of 1048576 kB.
	if got := number(t, data, "mem.available"); math.Abs(got-0.75) > 0.001 {
		t.Errorf("mem.available = %v; want 0.75 GiB", got)
	}
	if got := number(t, data, "mem.used"); math.Abs(got-0.25) > 0.001 {
		t.Errorf("mem.used = %v; want 0.25 GiB", got)
	}
	if got := number(t, data, "mem.usage"); math.Abs(got-25) > 0.001 {
		t.Errorf("mem.usage = %v; want 25", got)
	}
}

// TestGetInformationsOmitsUnreportableUsage keeps the collector from guessing:
// with neither MemAvailable nor MemFree there is no honest answer for how much
// memory is in use, so the total is reported on its own.
func TestGetInformationsOmitsUnreportableUsage(t *testing.T) {
	writeFixture(t, `MemTotal:       16777216 kB
`, "")

	data := GetInformations()

	if got := number(t, data, "mem.total"); got != 16 {
		t.Errorf("mem.total = %v; want 16", got)
	}
	for _, key := range []string{"mem.used", "mem.available", "mem.usage"} {
		if _, exists := data.Values[key]; exists {
			t.Errorf("unexpected %s when availability could not be read", key)
		}
	}
}

// TestGetInformationsSkipsUnparsableFields covers a damaged file: a field whose
// value is not a number is skipped rather than read as zero.
func TestGetInformationsSkipsUnparsableFields(t *testing.T) {
	writeFixture(t, `MemTotal:       16777216 kB
MemFree:        N/A kB
HugePages_Total:       0
garbage line without a colon
`, "")

	data := GetInformations()

	if got := number(t, data, "mem.total"); got != 16 {
		t.Errorf("mem.total = %v; want 16", got)
	}
	// MemFree was unreadable, so there is no availability figure to derive.
	if _, exists := data.Values["mem.available"]; exists {
		t.Error("unexpected mem.available derived from an unreadable MemFree")
	}
}

// TestGetInformationsWithoutMeminfo covers the only way a Linux machine has
// nothing to report: the file cannot be read.
func TestGetInformationsWithoutMeminfo(t *testing.T) {
	writeMissingFixture(t)

	data := GetInformations()

	if len(data.Values) != 0 {
		t.Errorf("expected no rows without /proc/meminfo, got %v", keysOf(data))
	}
	if data.Name != "Memory" {
		t.Errorf("Name = %q; want %q", data.Name, "Memory")
	}
}

func TestMeminfoParsing(t *testing.T) {
	writeFixture(t, `MemTotal:       16777216 kB
MemFree:         1048576 kB
SomeField:            42
Padded:        7 kB
`, "")

	fields, ok := readMeminfo()
	if !ok {
		t.Fatal("readMeminfo reported failure for a readable file")
	}

	cases := map[string]float64{
		"MemTotal":  16777216,
		"MemFree":   1048576,
		"SomeField": 42, // a field without the "kB" suffix still parses
		"Padded":    7,  // the unit is stripped before parsing
	}
	for name, want := range cases {
		if got := fields[name]; got != want {
			t.Errorf("%s = %v; want %v", name, got, want)
		}
	}

	if _, exists := fields["garbage line without a colon"]; exists {
		t.Error("a line without a colon became a field")
	}
}

// TestPressureMalformedLinesAreIgnored is a regression test for hand-rolled
// parsing: a line without avg10, an unparsable figure and an unknown kind must
// all be skipped rather than become rows or panics.
func TestPressureMalformedLinesAreIgnored(t *testing.T) {
	writeFixture(t, sixteenGiB, `garbage

some avg60=1.00
some avg10=not-a-number
full avg10=0.5 extra=1
weird avg10=2.00
full avg10=
`)

	data := GetInformations()

	if got := number(t, data, "mem.pressure.full"); got != 0.5 {
		t.Errorf("mem.pressure.full = %v; want 0.5", got)
	}
	// The first "some" line has no avg10 and the second one cannot be parsed, so
	// there is nothing honest to report for it.
	if _, exists := data.Values["mem.pressure.some"]; exists {
		t.Error("unexpected mem.pressure.some from malformed lines")
	}

	// Exactly the rows the meminfo fixture and the one parsable PSI line provide:
	// nothing was invented from the lines that could not be read.
	want := []string{
		"mem.total", "mem.used", "mem.available", "mem.usage",
		"swap.total", "swap.used", "swap.usage",
		"mem.pressure.full",
	}
	if len(data.Values) != len(want) {
		t.Errorf("got rows %v; want exactly %v", keysOf(data), want)
	}
	for _, key := range want {
		if _, exists := data.Values[key]; !exists {
			t.Errorf("expected %s to be reported, got rows %v", key, keysOf(data))
		}
	}
}

// zram fixture values. One device with 8 GiB of uncompressed capacity holding
// 2 GiB of data, compressed to 512 MiB and costing 640 MiB of RAM: a 4:1 ratio,
// a quarter of the device in use, and a RAM figure that is deliberately larger
// than the compressed size because the kernel's mem_used_total includes the
// allocator's own overhead.
const (
	zram8GiB     = "8589934592"
	zramDense    = "2147483648 536870912 671088640 0 671088640 0 0 0 0\n"
	zramAlgoList = "lzo-rle lzo [lz4] lz4hc zstd deflate\n"
)

// TestZramReportsTheCompressedStore covers the common machine: zram is present,
// configured and holding data. The rows describe the store itself — how big it
// is, how much of it is in use, what it costs in RAM and how well it compressed.
func TestZramReportsTheCompressedStore(t *testing.T) {
	dir := writeFixture(t, sixteenGiB, pressureBoth)
	writeZramDevice(t, dir, "zram0", zram8GiB, zramDense, zramAlgoList)

	data := GetInformations()

	checks := map[string]float64{
		"zram.size":   8,   // disksize, the uncompressed capacity
		"zram.usage":  25,  // 2 of 8 GiB
		"zram.memory": 640, // mem_used_total, in MiB
		"zram.ratio":  4,   // 2 GiB stored in 512 MiB
	}
	for key, want := range checks {
		if got := number(t, data, key); math.Abs(got-want) > 0.001 {
			t.Errorf("%s = %v; want %v", key, got, want)
		}
	}

	if got, want := row(t, data, "zram.algorithm").Value, "lz4"; got != want {
		t.Errorf("zram.algorithm = %v; want %q (the bracketed one)", got, want)
	}
	// One device is the common case, and every other row is then that device's
	// own figures rather than a sum, so a count row would say nothing.
	if _, exists := data.Values["zram.devices"]; exists {
		t.Error("a single device should not produce a device count")
	}
}

// TestZramUnitsMatchTheRows pins the units: the device size in GiB, the fill
// level and the ratio as they are, and the RAM cost in MiB. A RAM figure in GiB
// would round an idle device's few hundred kilobytes down to 0.00 on every
// machine that is not under memory pressure.
func TestZramUnitsMatchTheRows(t *testing.T) {
	dir := writeFixture(t, sixteenGiB, pressureBoth)
	writeZramDevice(t, dir, "zram0", zram8GiB, zramDense, zramAlgoList)

	data := GetInformations()

	want := map[string]shared.HardwareUnit{
		"zram.size":   shared.Gibibyte,
		"zram.usage":  shared.Percent,
		"zram.memory": shared.Megabyte,
		"zram.ratio":  shared.Ratio,
	}
	for key, unit := range want {
		if got := row(t, data, key).Unit; got != unit {
			t.Errorf("%s unit = %v; want %v", key, got, unit)
		}
	}

	// The ratio is the one row whose suffix is written against the number: a
	// ratio is a dimensionless figure, and "4.00 :1" would read as two tokens.
	if got, want := data.Values["zram.ratio"].Unit.Suffix(), ":1"; got != want {
		t.Errorf("zram.ratio suffix = %q; want %q", got, want)
	}
}

// TestZramUnconfiguredDevicesAreSkipped covers the module loaded with a device
// count: the extra devices exist as directories with no size, hold nothing and
// cost nothing, so they must not turn into a 0 GiB row or a device count on a
// machine that effectively has one device.
func TestZramUnconfiguredDevicesAreSkipped(t *testing.T) {
	dir := writeFixture(t, sixteenGiB, pressureBoth)
	writeZramDevice(t, dir, "zram0", zram8GiB, zramDense, zramAlgoList)
	writeZramDevice(t, dir, "zram1", "0", "0 0 0 0 0 0 0 0 0\n", zramAlgoList)
	writeZramDevice(t, dir, "zram2", "", "", "")

	data := GetInformations()

	if got := number(t, data, "zram.size"); math.Abs(got-8) > 0.001 {
		t.Errorf("zram.size = %v; want 8 (the configured device only)", got)
	}
	if _, exists := data.Values["zram.devices"]; exists {
		t.Error("unconfigured devices should not be counted")
	}
}

// TestZramAggregatesEveryConfiguredDevice covers machines that spread swap over
// several devices: the figures are the machine's totals, and the count row says
// so, because otherwise the rows would look like one device's own numbers.
func TestZramAggregatesEveryConfiguredDevice(t *testing.T) {
	dir := writeFixture(t, sixteenGiB, pressureBoth)
	// Two 4 GiB devices, each holding 1 GiB compressed to 256 MiB at a cost of
	// 320 MiB: the totals are 8 GiB capacity, 2 GiB stored, 640 MiB of RAM.
	writeZramDevice(t, dir, "zram0", "4294967296", "1073741824 268435456 335544320 0 0 0 0 0 0\n", zramAlgoList)
	writeZramDevice(t, dir, "zram1", "4294967296", "1073741824 268435456 335544320 0 0 0 0 0 0\n", zramAlgoList)

	data := GetInformations()

	checks := map[string]float64{
		"zram.size":   8,
		"zram.usage":  25,
		"zram.memory": 640,
		"zram.ratio":  4,
	}
	for key, want := range checks {
		if got := number(t, data, key); math.Abs(got-want) > 0.001 {
			t.Errorf("%s = %v; want %v (summed over both devices)", key, got, want)
		}
	}
	if got, want := row(t, data, "zram.devices").Value, "2"; got != want {
		t.Errorf("zram.devices = %v; want %q", got, want)
	}
}

// TestZramNamesAnAlgorithmOnlyWhenTheDevicesShareOne: naming one of several
// algorithms would be a claim about devices that do not use it.
func TestZramNamesAnAlgorithmOnlyWhenTheDevicesShareOne(t *testing.T) {
	dir := writeFixture(t, sixteenGiB, pressureBoth)
	writeZramDevice(t, dir, "zram0", zram8GiB, zramDense, zramAlgoList)
	writeZramDevice(t, dir, "zram1", zram8GiB, zramDense, "lzo-rle [zstd]\n")

	data := GetInformations()

	if _, exists := data.Values["zram.algorithm"]; exists {
		t.Errorf("unexpected zram.algorithm with mixed algorithms: %v", data.Values["zram.algorithm"])
	}
	// The rest of the card is unaffected.
	if got := number(t, data, "zram.size"); math.Abs(got-16) > 0.001 {
		t.Errorf("zram.size = %v; want 16", got)
	}
}

// TestZramAlgorithmWithoutBracketsIsNotGuessed covers a comp_algorithm file
// that lists the supported algorithms without marking the active one. There is
// no way to tell which is in use, so the row is omitted rather than filled with
// the first name in the file.
func TestZramAlgorithmWithoutBracketsIsNotGuessed(t *testing.T) {
	dir := writeFixture(t, sixteenGiB, pressureBoth)
	writeZramDevice(t, dir, "zram0", zram8GiB, zramDense, "lzo lz4 zstd\n")

	data := GetInformations()

	if _, exists := data.Values["zram.algorithm"]; exists {
		t.Error("an unmarked algorithm list should not be guessed at")
	}
}

// TestZramEmptyStoreHasNoRatio: a configured device holding nothing has no
// ratio, which is not the same reading as a ratio of zero. Its size and its
// zero fill level are still reported, because those were read.
func TestZramEmptyStoreHasNoRatio(t *testing.T) {
	dir := writeFixture(t, sixteenGiB, pressureBoth)
	writeZramDevice(t, dir, "zram0", zram8GiB, "0 0 20480 0 20480 0 0 0 0\n", zramAlgoList)

	data := GetInformations()

	if _, exists := data.Values["zram.ratio"]; exists {
		t.Error("unexpected ratio for a store holding no data")
	}
	if got := number(t, data, "zram.usage"); got != 0 {
		t.Errorf("zram.usage = %v; want 0 (the device is configured and empty)", got)
	}
	// 20480 bytes is what the kernel allocates for a device holding nothing,
	// which is why the row is quoted in MiB: two decimals of a GiB would show
	// 0.00 and make an idle zram look like it costs nothing at all.
	if got := number(t, data, "zram.memory"); math.Abs(got-20480.0/(1024*1024)) > 1e-9 {
		t.Errorf("zram.memory = %v MiB; want %v", got, 20480.0/(1024*1024))
	}
}

// TestZramWithoutUsableCountersOmitsTheAggregates: a device whose mm_stat is
// missing, truncated or garbled still has a size, but its counters are unknown.
// Summing the devices that did answer would understate the machine's use, so
// the counters are withheld entirely rather than reported too low.
func TestZramWithoutUsableCountersOmitsTheAggregates(t *testing.T) {
	cases := map[string]string{
		"missing":   "",
		"truncated": "2147483648 536870912\n",
		"garbled":   "2147483648 not-a-number 671088640 0 0\n",
	}
	for name, mmStat := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeFixture(t, sixteenGiB, pressureBoth)
			writeZramDevice(t, dir, "zram0", zram8GiB, mmStat, zramAlgoList)

			data := GetInformations()

			for _, key := range []string{"zram.usage", "zram.memory", "zram.ratio"} {
				if _, exists := data.Values[key]; exists {
					t.Errorf("%s = %v; want no row for a device whose counters are unknown", key, data.Values[key].Value)
				}
			}
			if got := number(t, data, "zram.size"); math.Abs(got-8) > 0.001 {
				t.Errorf("zram.size = %v; want 8 (the size was readable)", got)
			}
		})
	}
}

// TestZramAbsentOnAMachineWithoutIt: no zram devices means no zram rows, which
// is the state of every machine that does not use it.
func TestZramAbsentOnAMachineWithoutIt(t *testing.T) {
	writeFixture(t, sixteenGiB, pressureBoth)

	data := GetInformations()

	for key := range data.Values {
		if strings.HasPrefix(key, "zram.") {
			t.Errorf("unexpected %s on a machine without zram", key)
		}
	}
}
