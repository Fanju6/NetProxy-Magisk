//go:build !android || !arm64

package module

func repairColorOSGoogleFirewall(map[uint32]bool) (int, error) {
	return 0, nil
}
