//go:build !windows

package files

import (
	"fmt"
	"syscall"
)

// GetDiskSpace returns the total and available (free) disk space in bytes for
// the given path. Available space is what unprivileged processes can actually
// use, which is what "free space" means to a user.
func GetDiskSpace(path string) (int64, int64, error) {
	var stat syscall.Statfs_t

	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("failed to get disk stats for %s: %w", path, err)
	}

	// Total space = block size * total blocks
	totalSpace := int64(stat.Blocks) * int64(stat.Bsize)
	// Available space = block size * free blocks available to non-root
	availSpace := int64(stat.Bavail) * int64(stat.Bsize)

	return totalSpace, availSpace, nil
}
