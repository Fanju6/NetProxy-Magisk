//go:build !unix

package module

import "os"

func applyExportTargetMetadata(path string, target exportTarget) error {
	if target.info == nil {
		return os.Chmod(path, 0o600)
	}
	return os.Chmod(path, target.info.Mode().Perm())
}
