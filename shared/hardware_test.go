package shared

import "testing"

// TestUnitSuffixNamesEveryUnit pins the display text of each unit. A unit with no
// suffix would print a bare number, which is the state this replaced: rows whose
// only statement of their unit was the label beside them.
func TestUnitSuffixNamesEveryUnit(t *testing.T) {
	want := map[HardwareUnit]string{
		Celsius:  "°C",
		MHz:      "MHz",
		Percent:  "%",
		Watt:     "W",
		Megabyte: "MiB",
		Gigabyte: "GB",
		Terabyte: "TB",
		RPM:      "RPM",
		Hour:     "h",
	}

	for unit, suffix := range want {
		if suffix == "" {
			t.Fatalf("test case for unit %d expects an empty suffix", unit)
		}
		if got := unit.Suffix(); got != suffix {
			t.Errorf("unit %d suffix = %q; want %q", unit, got, suffix)
		}
	}

	// UnknownUnit is the one unit that deliberately renders nothing, and a value
	// outside the enum must not be guessed at either.
	if got := UnknownUnit.Suffix(); got != "" {
		t.Errorf("UnknownUnit suffix = %q; want empty", got)
	}
	if got := HardwareUnit(9999).Suffix(); got != "" {
		t.Errorf("out-of-range unit suffix = %q; want empty", got)
	}
}

// TestUnitValuesStayAppendOnly guards the enum's numbering. Units are stored in
// HardwareValue and compared by collectors and tests, so inserting a constant in
// the middle would silently renumber every unit after it.
func TestUnitValuesStayAppendOnly(t *testing.T) {
	ordered := []HardwareUnit{
		UnknownUnit, Celsius, MHz, Percent, Watt, Megabyte, RPM, Gigabyte, Terabyte, Hour,
	}
	for i, unit := range ordered {
		if int(unit) != i {
			t.Errorf("unit in position %d has value %d; want %d (keep the enum append-only)", i, int(unit), i)
		}
	}
}
