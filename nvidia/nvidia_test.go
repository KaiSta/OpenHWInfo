package nvidia

import (
	"reflect"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// TestGPUQueryFieldsOrder locks down the column contract between the
// --query-gpu field list and the fixed indices parseGPURecord uses. If a field
// is inserted or reordered without updating the indices, metrics would be
// silently attributed to the wrong label.
func TestGPUQueryFieldsOrder(t *testing.T) {
	want := []string{
		"name",
		"driver_version",
		"temperature.gpu",
		"temperature.memory",
		"clocks.sm",
		"clocks.mem",
		"memory.total",
		"memory.used",
		"utilization.gpu",
		"power.limit",
		"power.draw",
		"fan.speed",
		// Appended last so the metric columns above keep their positions while
		// one query still reports every GPU. The parser reads it by position.
		"index",
	}

	if !reflect.DeepEqual(gpuQueryFields, want) {
		t.Fatalf("gpuQueryFields = %v; want %v", gpuQueryFields, want)
	}

	if gpuIndexColumn != len(want)-1 {
		t.Errorf("gpuIndexColumn = %d; want %d (the last column)", gpuIndexColumn, len(want)-1)
	}
}

// TestParseGPUOutputSplitsRows covers the multi-GPU response shape. Querying
// nvidia-smi without -i returns one row per card, and a card name containing a
// comma must not shift that row's columns.
func TestParseGPUOutputSplitsRows(t *testing.T) {
	output := "\n" +
		`"NVIDIA GeForce RTX 4090, Founders Edition",550.54.14,62,70,2520,10501,24564,1024,45,450.00,120.50,38,0` + "\n" +
		"NVIDIA GeForce GTX 860M,535.183.01,49,0,405,2505,4096,512,7,60.00,12.30,30,1\n"

	rows := parseGPUOutput(output)
	if len(rows) != 2 {
		t.Fatalf("parseGPUOutput returned %d rows; want 2", len(rows))
	}
	if got := rows[0][0]; got != "NVIDIA GeForce RTX 4090, Founders Edition" {
		t.Errorf("row 0 name = %q; want the quoted name intact", got)
	}
	if got := rows[1][gpuIndexColumn]; got != "1" {
		t.Errorf("row 1 index column = %q; want %q", got, "1")
	}
}

func TestParseFloatField(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   float64
		wantOK bool
	}{
		{"plain integer", "45", 45, true},
		{"float", "12.5", 12.5, true},
		{"memory with unit", "10240 MiB", 10240, true},
		{"percent with unit", "45 %", 45, true},
		{"negative", "-1", -1, true},
		{"not available", "[N/A]", 0, false},
		{"not supported", "[Not Supported]", 0, false},
		{"empty", "", 0, false},
		{"unit only", "MiB", 0, false},
		{"lone dot", ".", 0, false},
		{"surrounding whitespace", "  73  ", 73, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseFloatField(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parseFloatField(%q) ok = %v; want %v", tc.in, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("parseFloatField(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseCSVFields verifies that GPU names containing commas do not shift the
// remaining columns, which is why a real CSV parser is used instead of a plain
// strings.Split.
func TestParseCSVFields(t *testing.T) {
	line := `"NVIDIA GeForce RTX 4090, Founders Edition",550.54.14,45,1024 MiB`
	fields := parseCSVFields(line)

	want := []string{"NVIDIA GeForce RTX 4090, Founders Edition", "550.54.14", "45", "1024 MiB"}
	if !reflect.DeepEqual(fields, want) {
		t.Errorf("parseCSVFields(quoted) = %v; want %v", fields, want)
	}
}

func TestParseCSVFieldsSimpleLine(t *testing.T) {
	fields := parseCSVFields("NVIDIA GeForce RTX 3080,535.104.05,62,10 MiB")
	want := []string{"NVIDIA GeForce RTX 3080", "535.104.05", "62", "10 MiB"}
	if !reflect.DeepEqual(fields, want) {
		t.Errorf("parseCSVFields(simple) = %v; want %v", fields, want)
	}
}

// TestParseCSVFieldsMalformedFallsBack ensures a malformed line still yields
// something usable rather than an empty result.
func TestParseCSVFieldsMalformedFallsBack(t *testing.T) {
	fields := parseCSVFields(`"unterminated,1,2`)
	if len(fields) != 3 {
		t.Fatalf("parseCSVFields(malformed) = %v; want 3 fields", fields)
	}
}

// TestFieldMappingMatchesQueryColumns checks that each metric reads the column
// that its label implies, driving the real parser rather than a copy of its
// mapping (a copy would only prove the test agrees with itself).
func TestFieldMappingMatchesQueryColumns(t *testing.T) {
	line := "NVIDIA GeForce RTX 4090,550.54.14,62,70,2520,10501,24564,1024,45,450.00,120.50,38,0"

	info := parseGPURecord(parseCSVFields(line))

	if info.Name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("card name = %q; want %q", info.Name, "NVIDIA GeForce RTX 4090")
	}
	if got := info.Values[shared.GPUDriverVersion.Key].Value; got != "550.54.14" {
		t.Errorf("driver version = %v; want %q", got, "550.54.14")
	}

	// (expected hardware key, expected parsed value)
	checks := []struct {
		key  string
		want float64
	}{
		{shared.GPUTemp.Key, 62},
		{shared.VRAMTemp.Key, 70},
		{shared.GPUFrequency.Key, 2520},
		{shared.VRAMLrequency.Key, 10501},
		{shared.VRAMTotal.Key, 24564},
		{shared.VRAMUsed.Key, 1024},
		{shared.GPUUsage.Key, 45},
		{shared.GPUPowerMax.Key, 450},
		{shared.GPUPowerAvg.Key, 120.50},
		{shared.FanSpeed.Key, 38},
	}

	for _, c := range checks {
		value, ok := info.Values[c.key]
		if !ok {
			t.Errorf("%s missing from the parsed record", c.key)
			continue
		}
		got, err := value.GetNumericValue()
		if err != nil {
			t.Errorf("%s: %v", c.key, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s = %v; want %v", c.key, got, c.want)
		}
	}

	// The index column is a query detail, not a metric.
	if _, ok := info.Values["index"]; ok {
		t.Error("the query's index column leaked into the metric values")
	}
}

// TestParseGPURecordOmitsUnavailableFields ensures a card that reports [N/A]
// (no fan, no driver-reported sensor) omits those rows instead of showing a
// bogus reading.
func TestParseGPURecordOmitsUnavailableFields(t *testing.T) {
	line := "NVIDIA GeForce RTX 4090,550.54.14,62,70,2520,10501,24564,1024,45,450.00,120.50,[N/A],0"

	info := parseGPURecord(parseCSVFields(line))

	if _, ok := info.Values[shared.FanSpeed.Key]; ok {
		t.Error("fan speed present despite [N/A]")
	}
	if _, ok := info.Values[shared.GPUTemp.Key]; !ok {
		t.Error("GPU temperature missing despite a real reading")
	}
}

// TestParseGPURecordCommaNameKeepsColumnsAligned goes through the parser rather
// than only the CSV splitter, so a shifted column would be caught here.
func TestParseGPURecordCommaNameKeepsColumnsAligned(t *testing.T) {
	line := `"NVIDIA GeForce RTX 4090, Founders Edition",550.54.14,62,70,2520,10501,24564,1024,45,450.00,120.50,38,0`

	info := parseGPURecord(parseCSVFields(line))

	if info.Name != "NVIDIA GeForce RTX 4090, Founders Edition" {
		t.Errorf("card name = %q; want the quoted name intact", info.Name)
	}
	if got := info.Values[shared.GPUDriverVersion.Key].Value; got != "550.54.14" {
		t.Errorf("driver version = %v; want %q (columns shifted)", got, "550.54.14")
	}
	value, ok := info.Values[shared.GPUPowerAvg.Key]
	if !ok {
		t.Fatal("power draw missing")
	}
	if got, _ := value.GetNumericValue(); got != 120.50 {
		t.Errorf("power draw = %v; want 120.50 (columns shifted)", got)
	}
}

// TestParseGPURecordTruncatedRow ensures a short row (an older driver that
// rejects some fields) does not panic and still yields the name.
func TestParseGPURecordTruncatedRow(t *testing.T) {
	info := parseGPURecord(parseCSVFields("NVIDIA GeForce GTX 860M"))

	if info.Name != "NVIDIA GeForce GTX 860M" {
		t.Errorf("card name = %q; want %q", info.Name, "NVIDIA GeForce GTX 860M")
	}
	if info.HasTelemetry() {
		t.Error("a name-only card reported telemetry")
	}
}
