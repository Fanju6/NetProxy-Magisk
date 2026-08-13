package worker

import "path/filepath"

func newTestOptions(root string) Options {
	options := NewOptions(root)
	testDevRoot := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-dev")
	options.ProgressDir = filepath.Join(testDevRoot, "subscriptions")
	options.PIDFile = filepath.Join(testDevRoot, "worker.pid")
	options.LogFile = filepath.Join(testDevRoot, "worker.log")
	return options
}
