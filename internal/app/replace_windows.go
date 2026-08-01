//go:build windows

package app

import (
	"fmt"
	"syscall"
	"unsafe"
)

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func replaceFile(oldPath, newPath string) error {
	oldPointer, err := syscall.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPointer, err := syscall.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	const moveFileReplaceExisting = 0x1
	const moveFileWriteThrough = 0x8
	result, _, callErr := moveFileEx.Call(
		uintptr(unsafe.Pointer(oldPointer)),
		uintptr(unsafe.Pointer(newPointer)),
		moveFileReplaceExisting|moveFileWriteThrough,
	)
	if result == 0 {
		return fmt.Errorf("replace file: %w", callErr)
	}
	return nil
}
