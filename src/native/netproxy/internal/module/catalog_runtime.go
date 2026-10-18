package module

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

func nativeProviderTags(options Options) ([]string, error) {
	content, err := os.ReadFile(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return nil, err
	}
	var document struct {
		Providers []struct {
			Tag string `json:"tag"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(document.Providers))
	for _, item := range document.Providers {
		tags = append(tags, item.Tag)
	}
	return tags, nil
}

func validateResourceRemoval(options Options, groupID string) error {
	return validateResourceDocument(options, groupID, provider.Document{})
}

func resourceValidator(options Options, groupID string) func(provider.Document) error {
	return func(document provider.Document) error { return validateResourceDocument(options, groupID, document) }
}

func validateResourceDocument(options Options, groupID string, document provider.Document) error {
	available := make(map[string]bool)
	for _, item := range provider.Inspect(document) {
		available[item.Tag] = true
	}
	if len(available) != 0 {
		tags, err := nativeProviderTags(options)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			if tag == catalog.ProviderTag(groupID) {
				return &service.Error{Code: "catalog.provider_conflict", Message: "原生 Provider 与 Catalog 标签冲突: " + tag}
			}
		}
	}
	content, err := os.ReadFile(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(content, &value); err != nil {
		return err
	}
	tag := catalog.ProviderTag(groupID)
	var referenced func(any, string) bool
	referenced = func(value any, field string) bool {
		switch current := value.(type) {
		case string:
			if field == "providers" && current == tag {
				return len(available) == 0
			}
			if field == "outbounds" || field == "default" || field == "detour" || field == "outbound" || field == "final" {
				member, matches := strings.CutPrefix(current, tag+"/")
				return matches && !available[member]
			}
			return false
		case []any:
			for _, child := range current {
				if referenced(child, field) {
					return true
				}
			}
		case map[string]any:
			for key, child := range current {
				if referenced(child, key) {
					return true
				}
			}
		}
		return false
	}
	if referenced(value, "") {
		return &service.Error{Code: "catalog.provider_in_use", Message: fmt.Sprintf("原生配置仍引用 Provider %s 或将被移除的节点；请先修改策略组或出站引用", tag)}
	}
	return nil
}

type catalogResourceChange struct {
	options     Options
	groupID     string
	component   string
	running     bool
	transaction *configApplyTransaction
}

func newCatalogResourceChange(options Options, groupID, component string) *catalogResourceChange {
	return &catalogResourceChange{options: options, groupID: groupID, component: component, running: configProcessRunning(options.SingBoxPath)}
}

func (change *catalogResourceChange) validate(document provider.Document) error {
	if err := validateResourceDocument(change.options, change.groupID, document); err != nil {
		return err
	}
	if !change.running {
		return nil
	}
	transaction, err := beginConfigApply(change.options, "")
	if err != nil {
		return err
	}
	for index, name := range []string{"meta.json", "provider.json", "history.jsonl"} {
		path := filepath.Join(change.options.CatalogRoot, change.groupID, name)
		snapshot, err := createConfigSnapshot(transaction.directory, fmt.Sprintf("catalog-%d", index), path)
		if err != nil {
			return errors.Join(err, transaction.cleanup())
		}
		transaction.journal.Catalog = append(transaction.journal.Catalog, snapshot)
	}
	if err := transaction.writeJournal(); err != nil {
		return errors.Join(err, transaction.cleanup())
	}
	change.transaction = transaction
	return nil
}

func (change *catalogResourceChange) finish(ctx context.Context, result catalog.MutationResult, mutationErr error) (catalog.MutationResult, error) {
	if mutationErr != nil {
		if change.transaction != nil {
			mutationErr = errors.Join(mutationErr, change.transaction.rollback())
		}
		return catalog.MutationResult{}, mutationErr
	}
	_, attempted, err := syncCatalogChangeWithTransaction(ctx, change.options, change.groupID, change.running, result.ReloadRequired, change.transaction)
	if err == nil {
		return result, nil
	}
	failure := catalogChangeError(change.component, attempted, err).(*service.Error)
	if change.transaction != nil && change.transaction.journal.Phase == "rolled_back" {
		failure.Message = "Resource change failed; previous state restored"
		failure.Data = map[string]any{"persisted": false, "runtime_synced": false, "rolled_back": true, "cause": err.Error()}
		return catalog.MutationResult{}, failure
	}
	return result, failure
}

func removeCatalogResource(ctx context.Context, options Options, groupID, component string) (bool, error) {
	change := newCatalogResourceChange(options, groupID, component)
	mutationErr := catalog.DeleteGroupWithGuard(ctx, options.CatalogRoot, groupID, func() error {
		return change.validate(provider.Document{})
	})
	_, err := change.finish(ctx, catalog.MutationResult{}, mutationErr)
	persisted := mutationErr == nil && (change.transaction == nil || change.transaction.journal.Phase != "rolled_back")
	return persisted, err
}

// Membership changes use the native file watcher; unacknowledged content changes reload.
func syncCatalogChange(ctx context.Context, options Options, groupID string, running, forceReload bool) (string, bool, error) {
	return syncCatalogChangeWithTransaction(ctx, options, groupID, running, forceReload, nil)
}

func syncCatalogChangeWithTransaction(ctx context.Context, options Options, groupID string, running, forceReload bool, transaction *configApplyTransaction) (state string, attempted bool, err error) {
	defer func() {
		if transaction == nil {
			return
		}
		if err == nil {
			err = transaction.commit()
		}
		if err != nil {
			err = rollbackConfigApply(options, transaction, err)
		}
	}()
	if !running {
		return subscription.RuntimeSyncNotRunning, false, nil
	}
	reload := func() error {
		if transaction == nil {
			transaction, err = beginConfigApply(options, "")
			if err != nil {
				return err
			}
		}
		if err := transaction.setPhase("reload_started"); err != nil {
			return err
		}
		// The lifecycle lock and config editors are already owned by this operation.
		return configReload(ctx, options)
	}
	temporary, err := os.MkdirTemp("", "netproxy-catalog-check-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(temporary)
	candidate := options
	candidate.RuntimeDir = temporary
	prepared, err := Prepare(ctx, candidate)
	if err != nil {
		return "", false, err
	}
	if err := checkPreparedConfiguration(ctx, candidate, prepared); err != nil {
		return "", false, err
	}
	current, err := os.ReadFile(filepath.Join(options.RuntimeDir, "providers.json"))
	if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	next, err := os.ReadFile(prepared.Providers)
	if err != nil {
		return "", false, err
	}
	if forceReload || !bytes.Equal(current, next) {
		if err := reload(); err != nil {
			return "", true, err
		}
	} else if groupID != "" {
		document, err := catalog.GroupProvider(ctx, options.CatalogRoot, groupID)
		if err != nil && !os.IsNotExist(err) {
			return "", false, err
		}
		if len(provider.Inspect(document)) != 0 {
			if err := worker.VerifyRuntimeState(ctx, workerOptions(options), groupID); err != nil {
				// Android filesystems may not deliver a Provider replacement event.
				// Keep the native watcher fast path, but reload when its observed
				// state cannot be proven instead of leaving a persisted change stale.
				if reloadErr := reload(); reloadErr != nil {
					return "", true, errors.Join(err, reloadErr)
				}
			}
		}
	}
	return subscription.RuntimeSyncApplied, true, nil
}
