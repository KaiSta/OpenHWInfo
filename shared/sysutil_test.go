package shared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFrequencyMHz(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  float64
	}{
		{"hertz", "1200000000", 1200},
		{"padded", "  1350000000\n", 1350},
		{"zero", "0", 0},
		{"empty", "", 0},
		{"garbage", "not-a-number", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseFrequencyMHz(tt.input); got != tt.want {
				t.Errorf("ParseFrequencyMHz(%q) = %v; want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePower(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		divisor int
		want    float64
	}{
		{"microwatts", "150000000", 1000000, 150},
		{"padded", " 250000000 ", 1000000, 250},
		{"zero", "0", 1000000, 0},
		// A negative power reading (e.g. a sensor mid-reset) must not surface as
		// a negative wattage in the UI.
		{"negative", "-1000", 1000000, 0},
		{"empty", "", 1000000, 0},
		{"garbage", "N/A", 1000000, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParsePower(tt.raw, tt.divisor); got != tt.want {
				t.Errorf("ParsePower(%q, %d) = %v; want %v", tt.raw, tt.divisor, got, tt.want)
			}
		})
	}
}

func TestReadSysFileReportsAbsence(t *testing.T) {
	dir := t.TempDir()

	present := filepath.Join(dir, "temp1_input")
	if err := os.WriteFile(present, []byte("45000\n"), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	got, ok := ReadSysFile(present)
	if !ok {
		t.Fatal("ReadSysFile reported a present file as missing")
	}
	// Trailing newline must be trimmed, otherwise numeric parsing fails.
	if got != "45000" {
		t.Errorf("ReadSysFile = %q; want %q", got, "45000")
	}

	// The ok flag is what lets callers distinguish an absent sensor from a
	// sensor that genuinely reads zero: this file reports 0 but exists.
	zero := filepath.Join(dir, "power1_average")
	if err := os.WriteFile(zero, []byte("0"), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	if _, ok := ReadSysFile(zero); !ok {
		t.Error("ReadSysFile reported an existing zero-valued file as missing")
	}

	if _, ok := ReadSysFile(filepath.Join(dir, "does-not-exist")); ok {
		t.Error("ReadSysFile reported a missing file as readable")
	}
}

func TestFindFirstSubpath(t *testing.T) {
	dir := t.TempDir()

	if got := FindFirstSubpath(dir, "hwmon*"); got != "" {
		t.Errorf("FindFirstSubpath with no match = %q; want empty", got)
	}

	want := filepath.Join(dir, "hwmon3")
	if err := os.Mkdir(want, 0755); err != nil {
		t.Fatalf("failed to create fixture: %v", err)
	}

	if got := FindFirstSubpath(dir, "hwmon*"); got != want {
		t.Errorf("FindFirstSubpath = %q; want %q", got, want)
	}
}

// writeCard creates a mock DRM card under sysDir with the given vendor ID.
func writeCard(t *testing.T, sysDir string, index int, vendor string) {
	t.Helper()
	devicePath := DRMCardDevicePath(sysDir, index)
	if err := os.MkdirAll(devicePath, 0755); err != nil {
		t.Fatalf("failed to create card %d: %v", index, err)
	}
	if err := os.WriteFile(filepath.Join(devicePath, "vendor"), []byte(vendor+"\n"), 0644); err != nil {
		t.Fatalf("failed to write vendor file: %v", err)
	}
}

func TestProbeDRMCardsMatchesVendorNotPosition(t *testing.T) {
	sysDir := t.TempDir()

	// The realistic multi-GPU layout: an Intel iGPU at index 0 and an AMD
	// discrete card at index 1. Probing for AMD must not return the Intel card,
	// which is the bug a positional scan would produce.
	writeCard(t, sysDir, 0, "0x8086")
	writeCard(t, sysDir, 1, "0x1002")

	got := ProbeDRMCards(sysDir, "0x1002")
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("ProbeDRMCards(AMD) = %v; want [1]", got)
	}

	got = ProbeDRMCards(sysDir, "0x10de")
	if len(got) != 0 {
		t.Errorf("ProbeDRMCards(NVIDIA) = %v; want none", got)
	}
}

func TestProbeDRMCardsSkipsUnreadableVendor(t *testing.T) {
	sysDir := t.TempDir()

	// A card node whose vendor file is missing must be skipped rather than
	// guessed at: an unreadable vendor file means the card cannot be identified.
	if err := os.MkdirAll(DRMCardDevicePath(sysDir, 2), 0755); err != nil {
		t.Fatalf("failed to create card: %v", err)
	}

	if got := ProbeDRMCards(sysDir, "0x1002"); len(got) != 0 {
		t.Errorf("ProbeDRMCards = %v; want none for a card with no vendor file", got)
	}
}

func TestProbeDRMCardsEmptySysDir(t *testing.T) {
	if got := ProbeDRMCards(t.TempDir(), "0x1002"); len(got) != 0 {
		t.Errorf("ProbeDRMCards on an empty sysDir = %v; want none", got)
	}
}

func TestResolvePCIDevice(t *testing.T) {
	sysDir := t.TempDir()

	link := DRMCardDevicePath(sysDir, 0)
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatalf("failed to create card dir: %v", err)
	}
	target := filepath.Join(sysDir, "devices/pci0000:00/0000:00:02.0")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	if got, want := ResolvePCIDevice(sysDir, 0), "0000:00:02.0"; got != want {
		t.Errorf("ResolvePCIDevice = %q; want %q", got, want)
	}

	// A missing card yields an empty address rather than a panic or a bogus name.
	if got := ResolvePCIDevice(sysDir, 7); got != "" {
		t.Errorf("ResolvePCIDevice for a missing card = %q; want empty", got)
	}
}

func TestDRMCardDevicePath(t *testing.T) {
	if got, want := DRMCardDevicePath("/sys", 2), "/sys/class/drm/card2/device"; got != want {
		t.Errorf("DRMCardDevicePath = %q; want %q", got, want)
	}
}
