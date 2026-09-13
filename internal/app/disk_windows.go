//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func freeDiskBytes(path string) (int64, bool) {
	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var available uint64
	result, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pointer)),
		uintptr(unsafe.Pointer(&available)),
		0,
		0,
	)
	return int64(available), result != 0
}
