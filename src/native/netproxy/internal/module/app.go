// Package module 提供模块业务操作的 Go 应用服务。
package module

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/telemetry"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
	"github.com/sagernet/sing-box/option"
)

// Options 描述模块目录、运行时目录和平台适配器路径。
type Options struct {
	ModuleDir          string
	ManagerVersion     string
	ManagerVersionCode string
	CatalogRoot        string
	ModuleConfig       string
	InboundConfig      string
	SingBoxPath        string
	SingBoxDir         string
	RuntimeDir         string
	StateFile          string
	ProgressDir        string
	LogDir             string
	ServiceAddress     string
	ServiceSecret      string
	WorkerPIDFile      string
	WorkerLogFile      string
	WiFiStateFile      string
	RequestTimeout     time.Duration
	// TransferContext 仅在服务生命周期结束时取消订阅下载，不取消持久事务。
	TransferContext    context.Context
	NetworkStateReader worker.NetworkStateReader
	Telemetry          *telemetry.Reporter
	configEditors      map[string]*moduleconfig.Editor
	networkEvaluation  *NetworkEvaluation
}

// NewOptions 根据模块根目录返回完整的默认路径。
func NewOptions(moduleDir string) Options {
	layout := paths.New(moduleDir)
	return Options{
		ModuleDir:      layout.Root(),
		CatalogRoot:    layout.Catalog(),
		ModuleConfig:   layout.ModuleConfig(),
		InboundConfig:  layout.InboundConfig(),
		SingBoxPath:    layout.SingBox(),
		SingBoxDir:     layout.SingBoxDir(),
		RuntimeDir:     layout.Runtime(),
		StateFile:      layout.ServiceState(),
		ProgressDir:    layout.ProgressDir(),
		LogDir:         layout.Logs(),
		ServiceAddress: "127.0.0.1:9090",
		ServiceSecret:  "singbox",
		WorkerPIDFile:  layout.WorkerPID(),
		WorkerLogFile:  layout.ServiceLog(),
		WiFiStateFile:  layout.WiFiState(),
		RequestTimeout: 8 * time.Second,
		Telemetry:      telemetry.New(layout),
	}
}

// PrepareResult 描述一次运行时准备结果。
type PrepareResult struct {
	catalog.RuntimeResult
	Providers string             `json:"providers"`
	Inbound   string             `json:"inbound"`
	Backend   string             `json:"backend"`
	Network   *NetworkEvaluation `json:"-"`
}

// AppPolicy 描述分应用代理的持久设置。
type AppPolicy struct {
	Enabled    bool   `json:"enabled"`
	Mode       string `json:"mode"`
	ProxyApps  string `json:"proxy_apps"`
	BypassApps string `json:"bypass_apps"`
}

// Prepare 装配 Catalog Provider 与当前受管入站，不改写用户出站。
func Prepare(ctx context.Context, options Options) (PrepareResult, error) {
	if err := options.validate(); err != nil {
		return PrepareResult{}, err
	}
	for _, path := range []string{options.RuntimeDir, filepath.Dir(options.StateFile)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return PrepareResult{}, err
		}
	}
	providers := filepath.Join(options.RuntimeDir, "providers.json")
	inboundPath := filepath.Join(options.RuntimeDir, "inbound.json")
	config, err := inbound.Load(options.InboundConfig)
	if err != nil {
		return PrepareResult{}, err
	}
	if err := validateManagedConfig(options, config); err != nil {
		return PrepareResult{}, err
	}
	if err := validateNativePolicyReferences(ctx, options); err != nil {
		return PrepareResult{}, err
	}
	if config.Backend == "tun" {
		groups, err := catalog.GroupIDs(ctx, options.CatalogRoot, "all")
		if err != nil {
			return PrepareResult{}, err
		}
		for _, id := range groups {
			document, err := catalog.GroupProvider(ctx, options.CatalogRoot, id)
			if err != nil {
				return PrepareResult{}, err
			}
			for _, outbound := range document.Outbounds {
				if dialer, ok := outbound.Options.(option.DialerOptionsWrapper); ok && dialer.TakeDialerOptions().RoutingMark != 0 {
					return PrepareResult{}, tunRoutingMarkConflict("catalog.routing_mark")
				}
			}
			for _, endpoint := range document.Endpoints {
				if dialer, ok := endpoint.Options.(option.DialerOptionsWrapper); ok && dialer.TakeDialerOptions().RoutingMark != 0 {
					return PrepareResult{}, tunRoutingMarkConflict("catalog.routing_mark")
				}
			}
		}
	}
	tags, err := nativeProviderTags(options)
	if err != nil {
		return PrepareResult{}, err
	}
	runtime, err := catalog.BuildRuntime(ctx, catalog.RuntimeOptions{Root: options.CatalogRoot, ProvidersOutput: providers, UserProviderTags: tags})
	if err != nil {
		return PrepareResult{}, err
	}
	network, err := configuredNetwork(ctx, options)
	if err != nil {
		return PrepareResult{}, err
	}
	if network.DesiredMode == "Direct" {
		if err := validateDirectRouting(options); err != nil {
			return PrepareResult{}, err
		}
	}
	config, err = config.WithDNSBypass(network.DesiredMode == "Direct")
	if err != nil {
		return PrepareResult{}, err
	}
	missingPackages, err := inbound.WriteAtomic(ctx, inboundPath, config)
	if err != nil {
		return PrepareResult{}, err
	}
	for _, ref := range missingPackages {
		logService(options, "WARN", "inbound.package", "skipped", "分应用代理跳过未安装应用: %s", ref.String())
	}
	return PrepareResult{RuntimeResult: runtime, Providers: providers, Inbound: inboundPath, Backend: config.Backend, Network: &network}, nil
}

// Check 生成隔离运行时配置并执行 sing-box check。
func Check(ctx context.Context, options Options) (PrepareResult, error) {
	prepared, err := Prepare(ctx, options)
	if err != nil {
		return PrepareResult{}, err
	}
	if options.SingBoxPath == "" {
		return prepared, errors.New("sing-box 路径为空")
	}
	return prepared, checkPreparedConfiguration(ctx, options, prepared)
}

// UpdateApp 在配置事务锁内修改最新应用策略，并应用到运行实例。
func UpdateApp(ctx context.Context, options Options, action, value string) (data AppPolicy, err error) {
	persisted := false
	defer func() { logOperation(options, "app", "app-policy.update", "分应用策略更新", persisted, err) }()
	if err := options.validate(); err != nil {
		return AppPolicy{}, err
	}
	options, release, err := lockConfigApply(ctx, options, options.InboundConfig, false)
	if err != nil {
		return AppPolicy{}, err
	}
	defer release()
	content, err := os.ReadFile(options.InboundConfig)
	if err != nil {
		return AppPolicy{}, err
	}
	config, err := inbound.Parse(content)
	if err != nil {
		return AppPolicy{}, err
	}
	policy := config.App
	switch action {
	case "mode":
		if value != "blacklist" && value != "whitelist" {
			return AppPolicy{}, errors.New("应用模式应为 blacklist 或 whitelist")
		}
		policy.Enabled = true
		policy.Mode = value
	case "add":
		ref, err := inbound.ParsePackageRef(value)
		if err != nil {
			return AppPolicy{}, err
		}
		if policy.Mode == "whitelist" {
			policy.ProxyApps = addPackageRef(policy.ProxyApps, ref.String())
		} else {
			policy.BypassApps = addPackageRef(policy.BypassApps, ref.String())
		}
		policy.Enabled = true
	case "remove":
		ref, err := inbound.ParsePackageRef(value)
		if err != nil {
			return AppPolicy{}, err
		}
		policy.ProxyApps = slices.DeleteFunc(policy.ProxyApps, func(item string) bool { return item == ref.String() })
		policy.BypassApps = slices.DeleteFunc(policy.BypassApps, func(item string) bool { return item == ref.String() })
	case "enable", "disable":
		policy.Enabled = action == "enable"
	default:
		return AppPolicy{}, fmt.Errorf("未知应用操作: %s", action)
	}
	content, err = json.Marshal(map[string]inbound.AppPolicy{"app": policy}, json.Deterministic(true))
	if err != nil {
		return AppPolicy{}, err
	}
	if _, err := applyConfigLocked(ctx, options, "inbound/app", options.InboundConfig, content, false, ""); err != nil {
		return AppPolicy{}, err
	}
	persisted = true
	config.App = policy
	return appPolicy(config), nil
}

func appPolicy(config inbound.Config) AppPolicy {
	return AppPolicy{
		Enabled:    config.App.Enabled,
		Mode:       config.App.Mode,
		ProxyApps:  strings.Join(config.App.ProxyApps, ","),
		BypassApps: strings.Join(config.App.BypassApps, ","),
	}
}

// NodeAppend 将节点加入本地资源，并同步原生 Provider。
func NodeAppend(ctx context.Context, options Options, groupID, input string, allowInsecure bool) (mutation catalog.MutationResult, err error) {
	return nodeAppend(ctx, options, groupID, "append", allowInsecure, func(mutationOptions catalog.MutationOptions) (catalog.MutationResult, error) {
		mutationOptions.Input = input
		return catalog.AppendNode(ctx, mutationOptions)
	})
}

// NodeAppendContent 将原始内容加入本地分组，不读取客户端指定的文件路径。
func NodeAppendContent(ctx context.Context, options Options, groupID string, content []byte, allowInsecure bool) (catalog.MutationResult, error) {
	return nodeAppend(ctx, options, groupID, "append", allowInsecure, func(mutationOptions catalog.MutationOptions) (catalog.MutationResult, error) {
		return catalog.AppendNodeContent(ctx, mutationOptions, content)
	})
}

// NodeImport 将本地文件中的节点追加到 default 本地配置组。
func NodeImport(ctx context.Context, options Options, input string, allowInsecure bool) (mutation catalog.MutationResult, err error) {
	return nodeAppend(ctx, options, "default", "import", allowInsecure, func(mutationOptions catalog.MutationOptions) (catalog.MutationResult, error) {
		mutationOptions.Input = input
		return catalog.AppendNode(ctx, mutationOptions)
	})
}

// NodeImportContent 将原始节点内容追加到 default 组，不读取客户端指定的文件路径。
func NodeImportContent(ctx context.Context, options Options, content []byte, allowInsecure bool) (catalog.MutationResult, error) {
	return nodeAppend(ctx, options, "default", "import", allowInsecure, func(mutationOptions catalog.MutationOptions) (catalog.MutationResult, error) {
		return catalog.AppendNodeContent(ctx, mutationOptions, content)
	})
}

func nodeAppend(ctx context.Context, options Options, groupID, action string, allowInsecure bool, appendNodes func(catalog.MutationOptions) (catalog.MutationResult, error)) (mutation catalog.MutationResult, err error) {
	message := "节点添加"
	if action == "import" {
		message = "节点导入"
	}
	defer func() { logOperation(options, "node", "node."+action, message, mutation.Revision > 0, err) }()
	options, release, err := lockCatalogChange(ctx, options)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	defer release()
	if err := ensureDefaultGroup(ctx, options); err != nil {
		return catalog.MutationResult{}, err
	}
	if groupID == "" {
		groupID = "default"
	}
	groupID, err = catalog.ResolveGroup(ctx, options.CatalogRoot, groupID)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	change := newCatalogResourceChange(options, groupID, "node")
	result, err := appendNodes(catalog.MutationOptions{
		GroupDir:      filepath.Join(options.CatalogRoot, groupID),
		GroupID:       groupID,
		Type:          "local",
		AllowInsecure: allowInsecure,
		Validate:      change.validate,
	})
	return change.finish(ctx, result, err)
}

// NodeEdit 原子替换指定分组的节点。
func NodeEdit(ctx context.Context, options Options, reference, input string, allowInsecure bool) (mutation catalog.MutationResult, err error) {
	return nodeEdit(ctx, options, reference, allowInsecure, func(mutationOptions catalog.MutationOptions) (catalog.MutationResult, error) {
		mutationOptions.Input = input
		return catalog.EditNode(ctx, mutationOptions)
	})
}

// NodeEditContent 使用原始内容替换指定节点，不解释设备文件路径。
func NodeEditContent(ctx context.Context, options Options, reference string, content []byte, allowInsecure bool) (catalog.MutationResult, error) {
	return nodeEdit(ctx, options, reference, allowInsecure, func(mutationOptions catalog.MutationOptions) (catalog.MutationResult, error) {
		return catalog.EditNodeContent(ctx, mutationOptions, content)
	})
}

func nodeEdit(ctx context.Context, options Options, reference string, allowInsecure bool, editNode func(catalog.MutationOptions) (catalog.MutationResult, error)) (mutation catalog.MutationResult, err error) {
	defer func() { logOperation(options, "node", "node.edit", "节点编辑", mutation.Revision > 0, err) }()
	options, release, err := lockCatalogChange(ctx, options)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	defer release()
	groupID, tag, err := splitReference(reference)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	groupID, err = catalog.ResolveGroup(ctx, options.CatalogRoot, groupID)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	change := newCatalogResourceChange(options, groupID, "node")
	result, err := editNode(catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Tag: tag, AllowInsecure: allowInsecure, Validate: change.validate})
	return change.finish(ctx, result, err)
}

// NodeRemove 删除资源节点，不改写原生策略组。
func NodeRemove(ctx context.Context, options Options, reference string) (mutation catalog.MutationResult, err error) {
	defer func() { logOperation(options, "node", "node.remove", "节点删除", mutation.Revision > 0, err) }()
	options, release, err := lockCatalogChange(ctx, options)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	defer release()
	groupID, tag, err := splitReference(reference)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	groupID, err = catalog.ResolveGroup(ctx, options.CatalogRoot, groupID)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	change := newCatalogResourceChange(options, groupID, "node")
	result, err := catalog.RemoveNode(ctx, catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Tag: tag, Validate: change.validate})
	return change.finish(ctx, result, err)
}

// RemoveSubscription never edits a user's native group references.
func RemoveSubscription(ctx context.Context, options Options, query string) (err error) {
	deleted := false
	defer func() { logOperation(options, "subscription", "subscription.remove", "订阅删除", deleted, err) }()
	options, release, err := lockCatalogChange(ctx, options)
	if err != nil {
		return err
	}
	defer release()
	groupID, err := catalog.ResolveGroup(ctx, options.CatalogRoot, query)
	if err != nil {
		return err
	}
	typ, err := catalog.GroupType(ctx, options.CatalogRoot, groupID)
	if err != nil || typ != "subscription" {
		return errors.New("目标不是 URL 订阅")
	}
	deleted, err = removeCatalogResource(ctx, options, groupID, "subscription")
	return err
}

func lockCatalogChange(ctx context.Context, options Options) (Options, func(), error) {
	if err := options.validate(); err != nil {
		return options, nil, err
	}
	lock, err := waitLifecycleLock(ctx, options.StateFile)
	if err != nil {
		return options, nil, err
	}
	if err := recoverConfigApply(ctx, options); err != nil {
		lock.release()
		return options, nil, err
	}
	options, release, err := lockConfigFiles(ctx, options, options.InboundConfig, paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		lock.release()
		return options, nil, err
	}
	return options, func() { release(); lock.release() }, nil
}

// SyncCatalog serializes committed Catalog effects without rewriting native outbounds.
func SyncCatalog(ctx context.Context, options Options, groupID string, forceReload bool) (string, bool, error) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	options, release, err := lockCatalogChange(cleanup, options)
	if err != nil {
		return "", false, err
	}
	defer release()
	return syncCatalogChange(ctx, options, groupID, service.ProcessRunning(options.SingBoxPath), forceReload)
}

func catalogChangeError(component string, attempted bool, err error) error {
	if err == nil {
		return nil
	}
	code, message := "persisted_effect_failed", "数据已保存，但资源校验失败"
	if attempted {
		code, message = "runtime_sync_failed", "数据已保存，但运行时同步失败"
	}
	return &service.Error{Code: component + "." + code, Message: message, Data: map[string]any{"persisted": true, "runtime_synced": false, "cause": err.Error()}}
}

func ensureDefaultGroup(ctx context.Context, options Options) error {
	if err := os.MkdirAll(options.CatalogRoot, 0o700); err != nil {
		return err
	}
	return catalog.EnsureGroup(ctx, catalog.GroupOptions{Root: options.CatalogRoot, GroupID: "default", Name: "本地配置", Type: "local"})
}

func splitReference(reference string) (string, string, error) {
	group, tag, found := strings.Cut(reference, "/")
	if !found || group == "" || tag == "" {
		return "", "", errors.New("节点引用格式应为 <group-id>/<tag>")
	}
	return group, tag, nil
}

func (options Options) validate() error {
	for name, value := range map[string]string{"模块配置": options.ModuleConfig, "Catalog": options.CatalogRoot, "入站配置": options.InboundConfig} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s路径不能为空", name)
		}
	}
	return nil
}

func minTimeout(value, fallback time.Duration) time.Duration {
	if value <= 0 || value > fallback {
		return fallback
	}
	return value
}

func addPackageRef(current []string, value string) []string {
	if slices.Contains(current, value) {
		return current
	}
	return append(current, value)
}

// SubscriptionOptions 描述订阅业务的公共路径和 Service 适配器。
type SubscriptionOptions struct {
	Options
	Name           string
	URL            string
	UserAgent      string
	HWID           string
	Headers        map[string]string
	AutoUpdate     bool
	UpdateInterval int64
	IntervalSource string
	UpdateViaProxy string
	Include        string
	Exclude        string
	AllowInsecure  bool
	Timeout        int64
}

// AddSubscription 创建订阅并立即执行一次验证更新。
func AddSubscription(ctx context.Context, options SubscriptionOptions) (result subscription.Result, err error) {
	defer func() {
		logOperation(options.Options, "subscription", "subscription.add", "订阅添加", result.Persisted, err)
	}()
	if options.URL == "" {
		return subscription.Result{}, errors.New("订阅 URL 不能为空")
	}
	if err := subscription.ValidateSettings(options.UpdateInterval, options.UpdateViaProxy, options.Timeout); err != nil {
		return subscription.Result{}, err
	}
	if err := ensureDefaultGroup(ctx, options.Options); err != nil {
		return subscription.Result{}, err
	}
	groupID, err := catalog.NewSubscriptionGroupID(ctx, options.CatalogRoot)
	if err != nil {
		return subscription.Result{}, err
	}
	if err := catalog.InitializeGroup(ctx, catalog.GroupOptions{Root: options.CatalogRoot, GroupID: groupID, Name: options.Name, Type: "subscription", URL: options.URL, UserAgent: options.UserAgent, HWID: options.HWID, CustomHeaders: options.Headers, AutoUpdate: options.AutoUpdate, UpdateInterval: options.UpdateInterval, IntervalSource: options.IntervalSource, UpdateViaProxy: options.UpdateViaProxy, Include: options.Include, Exclude: options.Exclude, AllowInsecure: options.AllowInsecure, Timeout: options.Timeout}); err != nil {
		return subscription.Result{}, err
	}
	workerOptions := workerOptions(options.Options)
	// 分组初始化已经提交；首次下载失败时也必须向客户端报告设置已持久化。
	workerOptions.PersistedBeforeUpdate = true
	updated, err := worker.UpdateGroup(ctx, workerOptions, groupID, time.Now(), nil)
	if err != nil {
		if options.Name == "" {
			if fallback := hostName(options.URL); fallback != "" {
				_ = catalog.SetGroupName(ctx, options.CatalogRoot, groupID, fallback, time.Now())
			}
		}
		return updated, err
	}
	return updated, nil
}

// UpdateSubscription 执行指定订阅更新并处理更新后的运行时副作用。
func UpdateSubscription(ctx context.Context, options Options, query string) (result subscription.Result, err error) {
	defer func() {
		logOperation(options, "subscription", "subscription.update", "订阅更新", result.Persisted, err)
	}()
	groupID, err := catalog.ResolveGroup(ctx, options.CatalogRoot, query)
	if err != nil {
		return subscription.Result{}, err
	}
	return worker.UpdateGroup(ctx, workerOptions(options), groupID, time.Now(), nil)
}

// EditSubscription 保存订阅编辑，并将需要变更的运行时状态交给 Worker 应用。
func EditSubscription(ctx context.Context, options Options, query string, edit subscription.EditOptions) (result subscription.EditResult, err error) {
	defer func() {
		logOperation(options, "subscription", "subscription.edit", "订阅编辑", result.Persisted, err)
	}()
	groupID, err := catalog.ResolveGroup(ctx, options.CatalogRoot, query)
	if err != nil {
		return subscription.EditResult{}, err
	}
	edit.Root = options.CatalogRoot
	edit.GroupID = groupID
	edit.ProgressDir = options.ProgressDir
	edit.DeferUpdate = true
	if edit.Now.IsZero() {
		edit.Now = time.Now()
	}
	edited, err := subscription.Edit(ctx, edit)
	if err != nil {
		return edited, err
	}
	if !edited.RequiresUpdate && !edited.NameChanged {
		if !service.ProcessRunning(options.SingBoxPath) {
			if err := subscription.RecordRuntimeSyncNotRunning(ctx, options.CatalogRoot, groupID, edit.Now); err != nil {
				return edited, err
			}
			edited.RuntimeSynced = false
			edited.RuntimeSyncState = subscription.RuntimeSyncNotRunning
		}
		return edited, nil
	}

	var updated subscription.Result
	workerOpts := workerOptions(options)
	workerOpts.ProxyURL = edit.ProxyURL
	workerOpts.FallbackDirect = edit.FallbackDirect
	workerOpts.PersistedBeforeUpdate = true
	if edited.RequiresUpdate {
		updated, err = worker.UpdateGroup(ctx, workerOpts, groupID, edit.Now, nil)
	} else {
		updated, err = worker.SyncEditedGroup(ctx, workerOpts, groupID, edit.Now, nil)
	}
	if err != nil {
		// 编辑设置已经在调用 Worker 前提交；更新失败只保留旧 Provider，不能把设置伪装成未保存。
		if updated.Persisted {
			edited.NodeCount = updated.NodeCount
			if updated.Revision != 0 {
				edited.Revision = updated.Revision
			}
			edited.StructureChanged = updated.StructureChanged
			edited.NotModified = updated.NotModified
			edited.RuntimeSynced = updated.RuntimeSynced
			edited.RuntimeSyncState = updated.RuntimeSyncState
			edited.RuntimeSyncPending = updated.RuntimeSyncPending
		}
		edited.Persisted = true
		return edited, err
	}
	edited.NodeCount = updated.NodeCount
	if updated.Revision != 0 {
		edited.Revision = updated.Revision
	}
	edited.StructureChanged = updated.StructureChanged
	edited.NotModified = updated.NotModified
	edited.Persisted = updated.Persisted
	edited.RuntimeSynced = updated.RuntimeSynced
	edited.RuntimeSyncState = updated.RuntimeSyncState
	edited.RuntimeSyncPending = updated.RuntimeSyncPending
	return edited, err
}

// UpdateAllSubscriptions 按 Catalog 顺序更新全部订阅。
func UpdateAllSubscriptions(ctx context.Context, options Options) (result worker.Summary, err error) {
	defer func() {
		logOperation(options, "subscription", "subscription.update-all", "全部订阅更新", false, err)
	}()
	ids, err := catalog.GroupIDs(ctx, options.CatalogRoot, "subscription")
	if err != nil {
		return worker.Summary{}, err
	}
	summary := worker.Summary{Updated: []string{}, Failed: []string{}}
	var firstUpdateErr error
	for _, id := range ids {
		if _, updateErr := worker.UpdateGroup(ctx, workerOptions(options), id, time.Now(), nil); updateErr != nil {
			summary.Failed = append(summary.Failed, id)
			if firstUpdateErr == nil {
				firstUpdateErr = updateErr
			}
		} else {
			summary.Updated = append(summary.Updated, id)
		}
	}
	return summary, firstUpdateErr
}

func workerOptions(options Options) worker.Options {
	workerOptions := worker.Options{
		Root:                options.CatalogRoot,
		ProgressDir:         options.ProgressDir,
		TransferContext:     options.TransferContext,
		PIDFile:             options.WorkerPIDFile,
		LogFile:             options.WorkerLogFile,
		ModuleConfig:        options.ModuleConfig,
		APIConfig:           paths.New(options.ModuleDir).APIConfig(),
		SingBoxPath:         options.SingBoxPath,
		ServiceAddress:      options.ServiceAddress,
		ServiceSecret:       options.ServiceSecret,
		NetworkWatchEnabled: true,
		SyncCatalog: func(ctx context.Context, groupID string, structureChanged bool) (string, bool, error) {
			return SyncCatalog(ctx, options, groupID, structureChanged)
		},
		ValidateProvider: func(groupID string, document provider.Document) error {
			return validateResourceDocument(options, groupID, document)
		},
		Now: time.Now,
		NetworkEvaluate: func(ctx context.Context, networkType, ssid string) error {
			_, err := EvaluateNetwork(ctx, options, networkType, ssid)
			return err
		},
	}
	if options.Telemetry != nil {
		workerOptions.Telemetry = options.Telemetry
		workerOptions.CoreRunning = func() bool {
			state, err := ReadServiceState(options.StateFile)
			return err == nil && state.State == "ready" && state.PID > 0 && service.FindProcess(options.SingBoxPath, int(state.PID)) == int(state.PID)
		}
	}
	return workerOptions
}

// RecordActivity 仅记录公共客户端的活跃；停服入口不能为统计重新启动 Worker。
func RecordActivity(ctx context.Context, options Options, startWorker bool) {
	if options.Telemetry == nil || ctx.Err() != nil {
		return
	}
	if added, err := options.Telemetry.RecordActive(time.Now()); added && err == nil {
		options.Telemetry.Notify()
		if startWorker {
			_ = EnsureWorker(ctx, options)
		}
	}
}

func hostName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// LoadAppPolicy 读取分应用代理设置。
func LoadAppPolicy(configPath string) (AppPolicy, error) {
	config, err := inbound.Load(configPath)
	if err != nil {
		return AppPolicy{}, err
	}
	return appPolicy(config), nil
}
