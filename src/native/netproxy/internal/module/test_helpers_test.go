package module

import (
	"path/filepath"
)

func newTestOptions(root string) Options {
	options := NewOptions(root)
	testDevRoot := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-dev")
	options.ProgressDir = filepath.Join(testDevRoot, "subscriptions")
	options.WorkerPIDFile = filepath.Join(testDevRoot, "worker.pid")
	options.WiFiStateFile = filepath.Join(testDevRoot, "wifi_state")
	options.ServiceAddress = "127.0.0.1:0"
	return options
}
