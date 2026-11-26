package traffic

import (
	"bytes"
	"os"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func Enabled(layout paths.Layout) bool {
	content, err := os.ReadFile(layout.TrafficEnabled())
	return err == nil && bytes.Equal(bytes.TrimSpace(content), []byte("true"))
}

func SaveEnabled(layout paths.Layout, enabled bool) error {
	content := []byte("false\n")
	if enabled {
		content = []byte("true\n")
	}
	return provider.WriteAtomic(layout.TrafficEnabled(), content, 0o600)
}
