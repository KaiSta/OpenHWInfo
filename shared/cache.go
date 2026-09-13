package shared

import "sync"

// probeCache memoizes the result of an identity probe that cannot change while
// the process runs but costs a fork to compute — a GPU's marketing name, a
// driver version. The per-tick collectors only need the live sensor readings;
// re-running `lspci`, `glxinfo` or `modinfo` every 5 seconds for a string
// that is fixed for the lifetime of the app is pure overhead.
var (
	probeCacheMu sync.Mutex
	probeCache   = make(map[string]string)
)

// CachedProbe returns the memoized result of resolve for key, computing it on
// the first request and returning the stored value afterwards. The result is
// cached even when it is empty: a probe that fails once will not succeed a
// second later, and re-running it every tick is the cost this exists to avoid.
//
// The key must include every input resolve depends on — the sysfs root among
// them — so a caller that points its collector at a different fixture tree
// never reads another tree's entry, and two cards never share one name.
func CachedProbe(key string, resolve func() string) string {
	probeCacheMu.Lock()
	cached, ok := probeCache[key]
	probeCacheMu.Unlock()
	if ok {
		return cached
	}

	value := resolve()

	probeCacheMu.Lock()
	probeCache[key] = value
	probeCacheMu.Unlock()
	return value
}

// ResetProbeCache drops every memoized probe. It exists for tests that rewrite a
// fixture in place and expect the collector to observe the change; production
// code never needs it, because the probed values are fixed per boot.
func ResetProbeCache() {
	probeCacheMu.Lock()
	probeCache = make(map[string]string)
	probeCacheMu.Unlock()
}
