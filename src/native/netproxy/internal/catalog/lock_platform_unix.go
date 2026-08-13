//go:build !windows

package catalog

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX)
		if err == unix.EINTR {
			continue
		}
		return err
	}
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
