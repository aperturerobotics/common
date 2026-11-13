//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockToolsDir waits for an exclusive lock on toolsPath/.lock and returns its
// release. The lock lives as long as the open file, so a process that dies
// while building releases it.
func lockToolsDir(toolsPath string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(toolsPath, ".lock"), os.O_CREATE|os.O_RDWR, 0o644) //nolint:gosec // Fixed filename inside the tools directory.
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil { //nolint:gosec // A file descriptor fits in an int.
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
