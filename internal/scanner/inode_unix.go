//go:build !windows

package scanner

import (
	"os"
	"syscall"
)

func fileInode(info os.FileInfo) uint64 {
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		return sys.Ino
	}
	return 0
}
