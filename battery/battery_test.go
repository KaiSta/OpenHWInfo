package battery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KaiSta/OpenHWInfo/shared"
)

// writeSupplies creates a /sys/class/power_supply fixture and points the
// collector at it for the duration of the test.
func writeSupplies(t *testing.T, supplies map[string]map[string]string) {
	t.Helper()

	root := t.TempDir()
	for name, files := range supplies {
		dir := filepath.Join(root, "class/power_supply", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", dir, err)
		}
		for file, content := range files {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
				t.Fatalf("failed to write %s: %v", file, err)
			}
		}
	}

	original := sysDir
	sysDir = root
	t.Cleanup(func() { sysDir = original })
}

// energyBattery is the shape of the review host's own battery: an energy (Wh)
// pack with the identity fields, a cycle count and an AC adapter beside it.
func energyBattery(overrides map[string]string) map[string]map[string]string {
	files := map[string]string{
		"type":               "Battery",
		"status":             "Full",
		"capacity":           "100",
		"energy_now":         "45170000",
		"energy_full":        "45170000",
		"energy_full_design": "54760000",
		"power_now":          "0",
		"voltage_now":        "8168000",
		"voltage_min_design": "7400000",
		"cycle_count":        "0",
		"technology":         "Li-ion",
		"model_name":         "PABAS0241231",
		"manufacturer":       "LENOVO ",
		"serial_number":      "41167",
		"present":            "1",
	}
	for file, content := range overrides {
		files[file] = content
	}
	return map[string]map[string]string{
		"ACAD": {"type": "Mains", "online": "1"},
		"BAT1": files,
	}
}

func cardNamed(t *testing.T, cards []shared.HardwareData, name string) shared.HardwareData {
	t.Helper()
	for _, card := range cards {
		if card.Name == name {
			return card
		}
	}
	t.Fatalf("no card named %q among %d card(s)", name, len(cards))
	return shared.HardwareData{}
}

// numericOf asserts a reading exists and returns it as a number.
func numericOf(t *testing.T, card shared.HardwareData, key string) float64 {
	t.Helper()
	value, ok := card.Values[key]
	if !ok {
		t.Fatalf("%s: key %q missing (got %v)", card.Name, key, keysOf(card.Values))
	}
	got, err := value.GetNumericValue()
	if err != nil {
		t.Fatalf("%s: key %q: %v", card.Name, key, err)
	}
	return got
}

func textOf(t *testing.T, card shared.HardwareData, key string) string {
	t.Helper()
	value, ok := card.Values[key]
	if !ok {
		t.Fatalf("%s: key %q missing (got %v)", card.Name, key, keysOf(card.Values))
	}
	text, ok := value.Value.(string)
	if !ok {
		t.Fatalf("%s: key %q = %#v; want text", card.Name, key, value.Value)
	}
	return text
}

func unitOf(t *testing.T, card shared.HardwareData, key string) string {
	t.Helper()
	value, ok := card.Values[key]
	if !ok {
		t.Fatalf("%s: key %q missing", card.Name, key)
	}
	return value.Unit.Suffix()
}

func keysOf(values map[string]shared.HardwareValue) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}

func assertAbsent(t *testing.T, card shared.HardwareData, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := card.Values[key]; ok {
			t.Errorf("%s: unexpected %s (%v)", card.Name, key, card.Values[key].Value)
		}
	}
}

func TestEnergyBatteryCard(t *testing.T) {
	writeSupplies(t, energyBattery(nil))

	cards := GetInformations()
	if len(cards) != 1 {
		t.Fatalf("expected one battery card, got %d", len(cards))
	}
	card := cards[0]

	if card.Name != "BAT1" {
		t.Errorf("card name = %q; want BAT1 (the supply the kernel lists)", card.Name)
	}

	// Values are reported in the unit the firmware uses: µWh becomes Wh, µV
	// becomes V, and the fraction is kept.
	if got := numericOf(t, card, shared.BatteryEnergyNow.Key); got != 45.17 {
		t.Errorf("energy now = %v; want 45.17", got)
	}
	if got := numericOf(t, card, shared.BatteryEnergyFull.Key); got != 45.17 {
		t.Errorf("energy full = %v; want 45.17", got)
	}
	if got := numericOf(t, card, shared.BatteryEnergyDesign.Key); got != 54.76 {
		t.Errorf("energy design = %v; want 54.76", got)
	}
	if got := unitOf(t, card, shared.BatteryEnergyNow.Key); got != "Wh" {
		t.Errorf("energy unit = %q; want Wh", got)
	}
	if got := numericOf(t, card, shared.BatteryVoltage.Key); got != 8.168 {
		t.Errorf("voltage = %v; want 8.168", got)
	}
	if got := numericOf(t, card, shared.BatteryCharge.Key); got != 100 {
		t.Errorf("charge = %v; want 100", got)
	}
	if got := textOf(t, card, shared.BatteryStatus.Key); got != "Full" {
		t.Errorf("status = %q; want Full", got)
	}
	// The trailing space a power supply attribute can carry must not survive.
	if got := textOf(t, card, shared.BatteryManufacturer.Key); got != "LENOVO" {
		t.Errorf("manufacturer = %q; want LENOVO", got)
	}
	if got := textOf(t, card, shared.BatteryAC.Key); got != "Online" {
		t.Errorf("AC adapter = %q; want Online", got)
	}

	// Health is what it holds when full against what it was designed to hold.
	wantHealth := 45.17 / 54.76 * 100
	if got := numericOf(t, card, shared.BatteryHealth.Key); got != wantHealth {
		t.Errorf("health = %v; want %v", got, wantHealth)
	}
	if card.Status == nil {
		t.Fatal("expected a health verdict")
	}
	if card.Status.Level != shared.StatusGood {
		t.Errorf("verdict level = %q; want %q (%s)", card.Status.Level, shared.StatusGood, card.Status.Detail)
	}
	if card.Status.Text != "Health: Good" {
		t.Errorf("verdict text = %q; want %q", card.Status.Text, "Health: Good")
	}
	for _, want := range []string{"45.17", "54.76", "82%", "Wh"} {
		if !strings.Contains(card.Status.Detail, want) {
			t.Errorf("verdict detail %q should mention %q", card.Status.Detail, want)
		}
	}

	// A full battery on the mains moves no power, so it has no time to report and
	// no charge-family rows.
	assertAbsent(t, card, shared.BatteryTimeEmpty.Key, shared.BatteryTimeFull.Key,
		shared.BatteryChargeNow.Key, shared.BatteryChargeFull.Key, shared.BatteryChargeDesign.Key)
}

func TestChargeBatteryUsesChargeUnits(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"BAT0": {
			"type":               "Battery",
			"status":             "Discharging",
			"capacity":           "70",
			"charge_now":         "3500000",
			"charge_full":        "3500000",
			"charge_full_design": "5000000",
			"voltage_now":        "11400000",
			"power_now":          "12000000",
		},
	})

	cards := GetInformations()
	if len(cards) != 1 {
		t.Fatalf("expected one battery card, got %d", len(cards))
	}
	card := cards[0]

	if got := unitOf(t, card, shared.BatteryChargeNow.Key); got != "Ah" {
		t.Errorf("charge unit = %q; want Ah", got)
	}
	if got := numericOf(t, card, shared.BatteryChargeNow.Key); got != 3.5 {
		t.Errorf("charge now = %v; want 3.5", got)
	}
	if got := numericOf(t, card, shared.BatteryChargeFull.Key); got != 3.5 {
		t.Errorf("charge full = %v; want 3.5", got)
	}
	if got := numericOf(t, card, shared.BatteryChargeDesign.Key); got != 5 {
		t.Errorf("charge design = %v; want 5", got)
	}

	// The energy family is not invented for a charge battery, and no time is
	// estimated from Ah against watts.
	assertAbsent(t, card, shared.BatteryEnergyNow.Key, shared.BatteryTimeEmpty.Key, shared.BatteryTimeFull.Key)

	if card.Status == nil || card.Status.Level != shared.StatusWarn {
		t.Fatalf("expected a Worn verdict for 70%% of design capacity, got %+v", card.Status)
	}
	if !strings.Contains(card.Status.Detail, "Ah") {
		t.Errorf("verdict detail %q should quote the unit the battery reports", card.Status.Detail)
	}
}

func TestTwoBatteriesGetTheirOwnCards(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"BAT0": {"type": "Battery", "status": "Discharging", "capacity": "20", "energy_now": "10000000", "energy_full": "50000000", "energy_full_design": "50000000"},
		"BAT1": {"type": "Battery", "status": "Charging", "capacity": "90", "energy_now": "45000000", "energy_full": "50000000", "energy_full_design": "50000000"},
	})

	cards := GetInformations()
	if len(cards) != 2 {
		t.Fatalf("expected two battery cards, got %d", len(cards))
	}

	// Ordered by supply name, so each battery keeps its card between polls.
	if cards[0].Name != "BAT0" || cards[1].Name != "BAT1" {
		t.Errorf("card order = %q, %q; want BAT0, BAT1", cards[0].Name, cards[1].Name)
	}
	if got := numericOf(t, cards[0], shared.BatteryCharge.Key); got != 20 {
		t.Errorf("BAT0 charge = %v; want 20", got)
	}
	if got := numericOf(t, cards[1], shared.BatteryCharge.Key); got != 90 {
		t.Errorf("BAT1 charge = %v; want 90", got)
	}
	if got := textOf(t, cards[0], shared.BatteryStatus.Key); got != "Discharging" {
		t.Errorf("BAT0 status = %q; want Discharging", got)
	}
	if got := textOf(t, cards[1], shared.BatteryStatus.Key); got != "Charging" {
		t.Errorf("BAT1 status = %q; want Charging", got)
	}
}

// TestTimeToEmptyOnlyWhileDischarging pins the estimate to the direction the
// battery is actually moving: energy over power is a duration only while it is
// leaving or entering the pack.
func TestTimeToEmptyOnlyWhileDischarging(t *testing.T) {
	t.Run("discharging", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{
			"BAT1": {"type": "Battery", "status": "Discharging", "energy_now": "45170000", "power_now": "15000000"},
		})

		card := GetInformations()[0]
		if got := numericOf(t, card, shared.BatteryTimeEmpty.Key); got != 45.17/15 {
			t.Errorf("time to empty = %v; want %v", got, 45.17/15)
		}
		if got := unitOf(t, card, shared.BatteryTimeEmpty.Key); got != "h" {
			t.Errorf("time unit = %q; want h", got)
		}
		assertAbsent(t, card, shared.BatteryTimeFull.Key)
	})

	t.Run("charging", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{
			"BAT1": {"type": "Battery", "status": "Charging", "energy_now": "10000000", "power_now": "20000000"},
		})

		card := GetInformations()[0]
		if got := numericOf(t, card, shared.BatteryTimeFull.Key); got != 0.5 {
			t.Errorf("time to full = %v; want 0.5", got)
		}
		assertAbsent(t, card, shared.BatteryTimeEmpty.Key)
	})

	t.Run("full on the mains", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{
			"BAT1": {"type": "Battery", "status": "Full", "energy_now": "45170000", "power_now": "0"},
		})

		card := GetInformations()[0]
		// A zero power reading is a reading, so the power row stays; the estimate
		// it cannot support does not.
		if got := numericOf(t, card, shared.BatteryPower.Key); got != 0 {
			t.Errorf("power = %v; want 0", got)
		}
		assertAbsent(t, card, shared.BatteryTimeEmpty.Key, shared.BatteryTimeFull.Key)
	})
}

func TestWornBatteryIsWarnedAndWeakBatteryFails(t *testing.T) {
	t.Run("worn", func(t *testing.T) {
		writeSupplies(t, energyBattery(map[string]string{
			"energy_full":        "30000000",
			"energy_full_design": "54760000",
			"cycle_count":        "812",
		}))

		card := cardNamed(t, GetInformations(), "BAT1")
		if card.Status == nil || card.Status.Level != shared.StatusWarn {
			t.Fatalf("expected a Worn verdict at 55%% of design capacity, got %+v", card.Status)
		}
		if card.Status.Text != "Health: Worn" {
			t.Errorf("verdict text = %q; want %q", card.Status.Text, "Health: Worn")
		}
		// A warning must state what it compared: the figures, the unit they are in
		// and the threshold that was crossed.
		for _, want := range []string{"55%", "30.00", "54.76", "Wh", "80%", "812"} {
			if !strings.Contains(card.Status.Detail, want) {
				t.Errorf("verdict detail %q should mention %q", card.Status.Detail, want)
			}
		}
	})

	t.Run("weak", func(t *testing.T) {
		writeSupplies(t, energyBattery(map[string]string{
			"energy_full":        "20000000",
			"energy_full_design": "54760000",
		}))

		card := cardNamed(t, GetInformations(), "BAT1")
		if card.Status == nil || card.Status.Level != shared.StatusBad {
			t.Fatalf("expected a Poor verdict at 37%% of design capacity, got %+v", card.Status)
		}
		for _, want := range []string{"37%", "50%"} {
			if !strings.Contains(card.Status.Detail, want) {
				t.Errorf("verdict detail %q should mention %q", card.Status.Detail, want)
			}
		}
	})
}

// TestNoDesignCapacityMeansNoVerdict: with nothing to compare the full capacity
// against, the card carries no badge at all rather than a guessed one.
func TestNoDesignCapacityMeansNoVerdict(t *testing.T) {
	writeSupplies(t, energyBattery(map[string]string{
		"energy_full_design": "",
	}))

	card := cardNamed(t, GetInformations(), "BAT1")
	if card.Status != nil {
		t.Errorf("expected no verdict without a design capacity, got %+v", card.Status)
	}
	assertAbsent(t, card, shared.BatteryHealth.Key)

	// The rest of the card is unaffected.
	if got := numericOf(t, card, shared.BatteryEnergyFull.Key); got != 45.17 {
		t.Errorf("energy full = %v; want 45.17", got)
	}
}

func TestAbsentFieldsProduceNoRows(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"BAT2": {"type": "Battery", "present": "1", "capacity": "64", "status": "Discharging"},
	})

	cards := GetInformations()
	if len(cards) != 1 {
		t.Fatalf("expected one battery card, got %d", len(cards))
	}
	card := cards[0]

	if got := numericOf(t, card, shared.BatteryCharge.Key); got != 64 {
		t.Errorf("charge = %v; want 64", got)
	}
	assertAbsent(t, card,
		shared.BatteryEnergyNow.Key, shared.BatteryEnergyFull.Key, shared.BatteryEnergyDesign.Key,
		shared.BatteryChargeNow.Key, shared.BatteryVoltage.Key, shared.BatteryCycles.Key,
		shared.BatteryModel.Key, shared.BatterySerial.Key, shared.BatteryAC.Key,
		shared.BatteryTimeEmpty.Key, shared.BatteryTimeFull.Key)
	if card.Status != nil {
		t.Errorf("expected no verdict, got %+v", card.Status)
	}
}

func TestUnreadableValuesProduceNoRows(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"BAT1": {
			"type":               "Battery",
			"status":             "Unknown",
			"capacity":           "not a number",
			"energy_now":         "",
			"energy_full":        "N/A",
			"energy_full_design": "",
			"cycle_count":        "many",
			"voltage_now":        "unreadable",
		},
	})

	cards := GetInformations()
	if len(cards) != 1 {
		t.Fatalf("expected the battery to be reported by its status alone, got %d card(s)", len(cards))
	}
	card := cards[0]

	if got := textOf(t, card, shared.BatteryStatus.Key); got != "Unknown" {
		t.Errorf("status = %q; want the kernel's own word Unknown", got)
	}
	assertAbsent(t, card,
		shared.BatteryCharge.Key, shared.BatteryEnergyNow.Key, shared.BatteryEnergyFull.Key,
		shared.BatteryEnergyDesign.Key, shared.BatteryHealth.Key, shared.BatteryCycles.Key,
		shared.BatteryVoltage.Key, shared.BatteryPower.Key)
	if card.Status != nil {
		t.Errorf("expected no verdict from unreadable figures, got %+v", card.Status)
	}
}

func TestSuppliesWithoutReadingsAreNotBatteries(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"ACAD": {"type": "Mains", "online": "1"},
		"BAT1": {"type": "Battery", "present": "1"}, // nothing else readable
	})

	if cards := GetInformations(); len(cards) != 0 {
		t.Errorf("expected no card for a supply that reports nothing, got %+v", cards)
	}
}

// TestEmptyBatteryBayIsSkipped covers a laptop whose battery is removed: the bay
// exists, the battery does not.
func TestEmptyBatteryBayIsSkipped(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"BAT1": {"type": "Battery", "present": "0"},
	})

	if cards := GetInformations(); len(cards) != 0 {
		t.Errorf("expected no card for an empty bay, got %+v", cards)
	}
}

// TestDeviceScopedBatteryIsSkipped keeps a peripheral's battery off the laptop's
// cards: a wireless mouse reports one too, under scope "Device".
func TestDeviceScopedBatteryIsSkipped(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"hid-00:11:22": {"type": "Battery", "scope": "Device", "capacity": "90"},
		"BAT1":         {"type": "Battery", "scope": "System", "capacity": "50"},
	})

	cards := GetInformations()
	if len(cards) != 1 {
		t.Fatalf("expected only the system battery, got %d card(s)", len(cards))
	}
	if cards[0].Name != "BAT1" {
		t.Errorf("card name = %q; want BAT1", cards[0].Name)
	}
}

// TestUnknownScopeIsAccepted covers kernels that do not report scope: their
// absence is not a claim that the battery belongs to a peripheral.
func TestUnknownScopeIsAccepted(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"BAT1": {"type": "Battery", "capacity": "50"},
	})

	if cards := GetInformations(); len(cards) != 1 {
		t.Errorf("expected a battery on a kernel that reports no scope, got %d card(s)", len(cards))
	}
}

func TestACAdapterState(t *testing.T) {
	battery := map[string]string{"type": "Battery", "capacity": "50"}

	t.Run("plugged in", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{
			"ACAD": {"type": "Mains", "online": "1"},
			"BAT1": battery,
		})

		if got := textOf(t, GetInformations()[0], shared.BatteryAC.Key); got != "Online" {
			t.Errorf("AC adapter = %q; want Online", got)
		}
	})

	t.Run("unplugged", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{
			"ACAD": {"type": "Mains", "online": "0"},
			"BAT1": battery,
		})

		if got := textOf(t, GetInformations()[0], shared.BatteryAC.Key); got != "Offline" {
			t.Errorf("AC adapter = %q; want Offline", got)
		}
	})

	t.Run("usb-c charger", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{
			"ucsi-source-psy-1": {"type": "USB", "online": "1"},
			"BAT1":              battery,
		})

		if got := textOf(t, GetInformations()[0], shared.BatteryAC.Key); got != "Online" {
			t.Errorf("AC adapter = %q; want Online for a USB-C supply", got)
		}
	})

	// A UPS reports itself online while running the machine from its own
	// battery, so it must not be mistaken for the mains.
	t.Run("ups is not the mains", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{
			"UPS0": {"type": "UPS", "online": "1"},
			"BAT1": battery,
		})

		assertAbsent(t, GetInformations()[0], shared.BatteryAC.Key)
	})

	t.Run("no adapter information", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{"BAT1": battery})

		assertAbsent(t, GetInformations()[0], shared.BatteryAC.Key)
	})

	// An adapter whose online state cannot be read is not a claim either.
	t.Run("unreadable adapter", func(t *testing.T) {
		writeSupplies(t, map[string]map[string]string{
			"ACAD": {"type": "Mains", "online": ""},
			"BAT1": battery,
		})

		assertAbsent(t, GetInformations()[0], shared.BatteryAC.Key)
	})
}

// TestDesktopReportsNothing covers the machine shape the collector must stay
// silent on.
func TestDesktopReportsNothing(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"ACAD": {"type": "Mains", "online": "1"},
	})

	if cards := GetInformations(); cards != nil {
		t.Errorf("expected no battery cards on a desktop, got %+v", cards)
	}
}

// TestSupplyWithoutTypeIsIgnored: an unclassifiable supply must not be guessed to
// be a battery.
func TestSupplyWithoutTypeIsIgnored(t *testing.T) {
	writeSupplies(t, map[string]map[string]string{
		"BAT1": {"capacity": "50", "status": "Discharging"},
	})

	if cards := GetInformations(); len(cards) != 0 {
		t.Errorf("expected no card for a supply with no type, got %+v", cards)
	}
}

func TestReadBool(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	write("one", "1")
	write("zero", "0")
	write("garbage", "yes")
	write("empty", "")

	cases := []struct {
		file   string
		want   bool
		wantOK bool
	}{
		{"one", true, true},
		{"zero", false, true},
		{"garbage", false, false},
		{"empty", false, false},
		{"absent", false, false},
	}

	for _, tc := range cases {
		got, ok := readBool(dir, tc.file)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Errorf("readBool(%q) = %v, %v; want %v, %v", tc.file, got, ok, tc.want, tc.wantOK)
		}
	}
}
