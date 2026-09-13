package shared

import "testing"

func TestCachedProbeComputesOnce(t *testing.T) {
	ResetProbeCache()
	t.Cleanup(ResetProbeCache)

	calls := 0
	resolve := func() string {
		calls++
		return "Radeon RX 7900 XT"
	}

	for i := 0; i < 3; i++ {
		if got := CachedProbe("amd.gpuName:/sys:0", resolve); got != "Radeon RX 7900 XT" {
			t.Fatalf("call %d: CachedProbe = %q; want %q", i, got, "Radeon RX 7900 XT")
		}
	}
	if calls != 1 {
		t.Errorf("resolve ran %d times; want 1", calls)
	}
}

// TestCachedProbeKeysAreIndependent pins the reason the key carries the sysfs
// root and the card identity: two keys must not share one value, or a fixture
// test would read another fixture's — or another card's — name.
func TestCachedProbeKeysAreIndependent(t *testing.T) {
	ResetProbeCache()
	t.Cleanup(ResetProbeCache)

	first := CachedProbe("intel.gpuName:/sys:0", func() string { return "Intel UHD Graphics 630" })
	second := CachedProbe("intel.gpuName:/sys:1", func() string { return "Intel Iris Xe Graphics" })

	if first != "Intel UHD Graphics 630" {
		t.Errorf("first = %q; want %q", first, "Intel UHD Graphics 630")
	}
	if second != "Intel Iris Xe Graphics" {
		t.Errorf("second = %q; want %q", second, "Intel Iris Xe Graphics")
	}
}

// TestCachedProbeCachesEmptyResult documents that a probe which produced nothing
// is still remembered, so a failing external command is not retried every tick.
func TestCachedProbeCachesEmptyResult(t *testing.T) {
	ResetProbeCache()
	t.Cleanup(ResetProbeCache)

	calls := 0
	resolve := func() string {
		calls++
		return ""
	}

	CachedProbe("intel.driverVersion:/sys", resolve)
	CachedProbe("intel.driverVersion:/sys", resolve)

	if calls != 1 {
		t.Errorf("resolve ran %d times; want 1", calls)
	}
}

func TestResetProbeCacheDropsEntries(t *testing.T) {
	ResetProbeCache()
	t.Cleanup(ResetProbeCache)

	if got := CachedProbe("k", func() string { return "first" }); got != "first" {
		t.Fatalf("CachedProbe = %q; want %q", got, "first")
	}

	ResetProbeCache()

	if got := CachedProbe("k", func() string { return "second" }); got != "second" {
		t.Errorf("CachedProbe after reset = %q; want %q", got, "second")
	}
}
