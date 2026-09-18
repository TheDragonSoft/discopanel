//go:build windows

package files

import (
	"fmt"
	"syscall"
	"unsafe"
)

// GetDiskSpace returns the total and available (free) disk space in bytes for
// the given path. Available space is what unprivileged processes can actually
// use, which is what "free space" means to a user.
func GetDiskSpace(path string) (int64, int64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes int64

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to convert path to UTF16: %w", err)
	}

	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)

	if ret == 0 {
		return 0, 0, fmt.Errorf("failed to get disk stats for %s: %w", path, err)
	}

	return totalNumberOfBytes, freeBytesAvailable, nil
}
