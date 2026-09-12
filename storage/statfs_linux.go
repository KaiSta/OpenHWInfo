//go:build linux

package storage

import "syscall"

// filesystemUsage reports the total, used and available bytes of the filesystem
// mounted at mountPoint. Used space is derived from the free-block count rather
// than the available count: the difference between the two is the root-reserved
// space, which is not free but is certainly not used either.
//
// It is a variable so tests can stub it — the real call needs an actual mount,
// which a fixture tree cannot provide.
var filesystemUsage = func(mountPoint string) (total, used, available uint64, ok bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountPoint, &stat); err != nil {
		return 0, 0, 0, false
	}
	blockSize := uint64(stat.Bsize)
	total = stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	available = stat.Bavail * blockSize
	if total == 0 || free > total {
		return 0, 0, 0, false
	}
	return total, total - free, available, true
}
