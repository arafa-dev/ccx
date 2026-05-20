//go:build windows

package scanner

import "os"

// fileInode returns a stable identifier for the file. Windows does not expose
// a true inode through os.FileInfo, so we approximate using size + modtime -
// any modification rotates the value.
func fileInode(info os.FileInfo) uint64 {
	return uint64(info.Size()) ^ uint64(info.ModTime().UnixNano())
}
