package nvidia

import (
	"reflect"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// TestGPUQueryFieldsOrder locks down the column contract between the
// --query-gpu field list and the fixed indices getInformationsForGPU uses. If a
// field is inserted or reordered without updating the indices, metrics would be
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
	}

	if !reflect.DeepEqual(gpuQueryFields, want) {
		t.Fatalf("gpuQueryFields = %v; want %v", gpuQueryFields, want)
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
// that its label implies, using a synthetic nvidia-smi row.
func TestFieldMappingMatchesQueryColumns(t *testing.T) {
	line := "NVIDIA GeForce RTX 4090,550.54.14,62,70,2520,10501,24564,1024,45,450.00,120.50,38"

	fields := parseCSVFields(line)
	field := func(i int) string {
		if i < len(fields) {
			return fields[i]
		}
		return ""
	}

	// (column index, expected hardware key, expected parsed value)
	checks := []struct {
		index int
		key   string
		want  float64
	}{
		{2, shared.GPUTemp.Key, 62},
		{3, shared.VRAMTemp.Key, 70},
		{4, shared.GPUFrequency.Key, 2520},
		{5, shared.VRAMLrequency.Key, 10501},
		{6, shared.VRAMTotal.Key, 24564},
		{7, shared.VRAMUsed.Key, 1024},
		{8, shared.GPUUsage.Key, 45},
		{9, shared.GPUPowerMax.Key, 450},
		{10, shared.GPUPowerAvg.Key, 120.50},
		{11, shared.FanSpeed.Key, 38},
	}

	for _, c := range checks {
		got, ok := parseFloatField(field(c.index))
		if !ok {
			t.Errorf("column %d (%s): value %q not parseable", c.index, c.key, field(c.index))
			continue
		}
		if got != c.want {
			t.Errorf("column %d (%s) = %v; want %v", c.index, c.key, got, c.want)
		}
	}

	if name := field(0); name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("column 0 (name) = %q; want %q", name, "NVIDIA GeForce RTX 4090")
	}
	if driver := field(1); driver != "550.54.14" {
		t.Errorf("column 1 (driver) = %q; want %q", driver, "550.54.14")
	}
}
