//go:build windows

package subscription

func isProcessAlive(pid int) bool {
	return false
}
