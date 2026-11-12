package module

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func validateManagedProviderTags(config map[string]jsontext.Value) error {
	for _, key := range []string{"providers", "outbounds", "endpoints"} {
		raw, exists := config[key]
		if !exists {
			continue
		}
		var entries []struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &entries); err != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Tag, "np/") {
				return &service.Error{Code: "catalog.provider_conflict", Message: "np/ 为 Catalog 保留的标签前缀: " + entry.Tag}
			}
		}
	}
	return nil
}

type policyReferences struct {
	Type      string   `json:"type"`
	UseAll    bool     `json:"use_all_providers"`
	Providers []string `json:"providers"`
	Outbounds []string `json:"outbounds"`
	Default   string   `json:"default"`
}

func readPolicyReferences(config map[string]jsontext.Value) ([]policyReferences, error) {
	var outbounds []policyReferences
	if raw, ok := config["outbounds"]; ok {
		if err := json.Unmarshal(raw, &outbounds); err != nil {
			return nil, err
		}
	}
	return outbounds, nil
}

func (g policyReferences) isGroup() bool {
	return g.Type == "selector" || g.Type == "urltest" || g.Type == "loadbalance"
}

func (g policyReferences) needsAnyProvider() bool {
	return g.isGroup() && g.UseAll && len(g.Outbounds) == 0
}

func hasNativeProviders(config map[string]jsontext.Value) bool {
	var entries []jsontext.Value
	return json.Unmarshal(config["providers"], &entries) == nil && len(entries) != 0
}

func validateConfiguredCatalogProviders(config map[string]jsontext.Value, providers []jsontext.Value) error {
	groups, err := readPolicyReferences(config)
	if err != nil {
		return err
	}
	if len(providers) == 0 && !hasNativeProviders(config) && slices.ContainsFunc(groups, policyReferences.needsAnyProvider) {
		return &service.Error{Code: "catalog.provider_missing", Message: "策略组需要至少一个出站或可用 Provider"}
	}
	available := make(map[string]bool, len(providers))
	for _, raw := range providers {
		var item struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		available[item.Tag] = true
	}
	for _, group := range groups {
		if !group.isGroup() || group.UseAll {
			continue
		}
		for _, tag := range group.Providers {
			if strings.HasPrefix(tag, "np/") && !available[tag] {
				return &service.Error{Code: "catalog.provider_missing", Message: "引用的 Catalog Provider 不存在或没有节点: " + tag}
			}
		}
	}
	return nil
}

func runtimeHasProvider(options Options, groupID string) bool {
	content, err := os.ReadFile(paths.RuntimeConfig(options.RuntimeDir))
	if err != nil {
		return false
	}
	var config struct {
		Providers []struct {
			Tag string `json:"tag"`
		} `json:"providers"`
	}
	if json.Unmarshal(content, &config) != nil {
		return false
	}
	tag := catalog.ProviderTag(groupID)
	for _, item := range config.Providers {
		if item.Tag == tag {
			return true
		}
	}
	return false
}

// ProviderCommitGuard serializes the short subscription commit with configuration
// writes. The Catalog locks are already held here, so never wait for lifecycle.
func ProviderCommitGuard(options Options) func(string, provider.Document) (func(), error) {
	return func(id string, document provider.Document) (func(), error) {
		lock, err := acquireLifecycleLock(options.StateFile)
		if err != nil {
			return nil, &subscription.Error{Code: "subscription.busy", Message: "服务配置正在变更，请稍后更新订阅"}
		}
		if _, err := os.Stat(configTransactionPath(options)); !os.IsNotExist(err) {
			lock.release()
			return nil, &subscription.Error{Code: "subscription.busy", Message: "有未完成的配置事务，请先恢复服务配置"}
		}
		if err := validateResourceCandidate(options, id, document); err != nil {
			lock.release()
			if value, ok := errors.AsType[*service.Error](err); ok {
				return nil, &subscription.Error{Code: value.Code, Message: value.Message}
			}
			return nil, &subscription.Error{Code: "provider.invalid", Message: "无法验证订阅节点与原生配置的引用关系"}
		}
		return lock.release, nil
	}
}

// Candidate validation is called while the Catalog mutation lock is held.
func validateResourceCandidate(options Options, id string, document provider.Document) error {
	content, err := os.ReadFile(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return err
	}
	config, err := configObject(content)
	if err != nil {
		return err
	}
	if err := validateManagedProviderTags(config); err != nil {
		return err
	}
	tag := catalog.ProviderTag(id)
	members := make(map[string]bool)
	for _, node := range provider.Inspect(document) {
		members[node.Tag] = true
	}
	outbounds, err := readPolicyReferences(config)
	if err != nil {
		return err
	}
	blocked := &service.Error{Code: "catalog.provider_in_use", Message: "原生配置仍引用 " + tag + " 或其中将被移除的节点"}
	if len(members) == 0 && !hasNativeProviders(config) && slices.ContainsFunc(outbounds, policyReferences.needsAnyProvider) {
		other, err := catalog.HasOtherProviderLocked(options.CatalogRoot, id)
		if err != nil {
			return err
		}
		if !other {
			return blocked
		}
	}
	missing := func(value string) bool {
		member, ok := strings.CutPrefix(value, tag+"/")
		return ok && !members[member]
	}
	for _, group := range outbounds {
		if group.isGroup() && (!group.UseAll && len(members) == 0 && slices.Contains(group.Providers, tag) ||
			slices.ContainsFunc(group.Outbounds, missing) ||
			group.Type == "selector" && missing(group.Default)) {
			return blocked
		}
	}
	// Native routes, DNS and dialers may refer to a qualified member directly.
	var check func(any) bool
	check = func(value any) bool {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if target, ok := child.(string); ok {
					if (key == "detour" || key == "outbound" || key == "final") && missing(target) {
						return true
					}
				} else if check(child) {
					return true
				}
			}
		case []any:
			return slices.ContainsFunc(value, check)
		}
		return false
	}
	for _, key := range []string{"route", "dns", "outbounds", "endpoints", "providers", "http_clients", "services"} {
		if raw, ok := config[key]; ok {
			var section any
			if err := json.Unmarshal(raw, &section); err != nil {
				return err
			}
			if check(section) {
				return blocked
			}
		}
	}
	return nil
}

type resourceChange struct {
	options     Options
	id          string
	transaction *configApplyTransaction
}

func (c *resourceChange) validate(document provider.Document, structureChanged bool) error {
	if err := validateResourceCandidate(c.options, c.id, document); err != nil {
		return err
	}
	if !structureChanged || !configProcessRunning(c.options.SingBoxPath) {
		return nil
	}
	transaction, err := beginConfigApply(c.options, "")
	if err != nil {
		return err
	}
	for _, name := range []string{"meta.json", "provider.json", "history.jsonl"} {
		path := filepath.Join(c.options.CatalogRoot, c.id, name)
		snapshot, err := createConfigSnapshot(transaction.directory, "catalog-"+name, path)
		if err != nil {
			return errors.Join(err, transaction.cleanup())
		}
		transaction.journal.Catalog = append(transaction.journal.Catalog, snapshot)
	}
	if err := transaction.writeJournal(); err != nil {
		return errors.Join(err, transaction.cleanup())
	}
	c.transaction = transaction
	return nil
}

func (c *resourceChange) finish(ctx context.Context, component string, result catalog.MutationResult, mutationErr error) (catalog.MutationResult, error) {
	if c.transaction == nil {
		return result, mutationErr
	}
	err := mutationErr
	if err == nil {
		err = applyCatalogReload(ctx, c.options, c.transaction)
	} else {
		err = rollbackConfigApply(c.options, c.transaction, err)
	}
	if err == nil {
		return result, nil
	}
	if c.transaction.journal.Phase == "rolled_back" {
		return catalog.MutationResult{}, &service.Error{Code: component + ".runtime_sync_failed",
			Message: "资源变更失败，已恢复原有配置和节点", Data: map[string]any{"persisted": false, "rolled_back": true}}
	}
	return result, &service.Error{Code: component + ".runtime_sync_failed",
		Message: "未能确认资源变更或恢复结果", Data: map[string]any{"runtime_synced": false, "cause": err.Error()}}
}

func applyCatalogReload(ctx context.Context, options Options, transaction *configApplyTransaction) error {
	err := transaction.setPhase("reload_started")
	if err == nil {
		err = configReload(ctx, options)
	}
	if err == nil {
		err = transaction.commit()
	}
	if err != nil {
		return rollbackConfigApply(options, transaction, err)
	}
	return nil
}
