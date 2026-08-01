//go:build !windows

package app

import "os"

func replaceFile(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
