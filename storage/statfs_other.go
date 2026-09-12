//go:build !linux

package storage

// filesystemUsage is unavailable off Linux. The rest of the collector reads
// /sys and /proc, which do not exist there either, so the stub keeps the package
// compiling while reporting no usage rather than failing.
var filesystemUsage = func(mountPoint string) (total, used, available uint64, ok bool) {
	return 0, 0, 0, false
}
