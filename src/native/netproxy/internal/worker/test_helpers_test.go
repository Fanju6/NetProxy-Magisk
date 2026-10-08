package worker

import (
	"context"
	"path/filepath"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func newTestOptions(root string) Options {
	options := NewOptions(root)
	testDevRoot := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-dev")
	options.ProgressDir = filepath.Join(testDevRoot, "subscriptions")
	options.PIDFile = filepath.Join(testDevRoot, "worker.pid")
	options.LogFile = filepath.Join(testDevRoot, "worker.log")
	options.ModuleConf = filepath.Join(root, "module.conf")
	options.SyncCatalog = func(ctx context.Context, groupID string, _ bool) (string, bool, error) {
		editor, err := moduleconfig.Lock(ctx, options.ModuleConf)
		if err != nil {
			return subscription.RuntimeSyncNotRunning, false, err
		}
		defer editor.Release()
		module, err := moduleconfig.LoadModule(options.ModuleConf)
		if err != nil {
			return subscription.RuntimeSyncNotRunning, false, err
		}
		selection, _, err := catalog.NormalizeSelection(ctx, root, module.Selection, groupID)
		if err == nil && selection != module.Selection {
			err = editor.Update(selection.Updates(), nil)
		}
		return subscription.RuntimeSyncNotRunning, false, err
	}
	return options
}
