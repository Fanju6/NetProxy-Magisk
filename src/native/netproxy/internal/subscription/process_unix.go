//go:build !windows

package subscription

import (
	"os"
	"strconv"
)

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat("/proc/" + strconv.Itoa(pid))
	return err == nil
}
