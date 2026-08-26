// Package module 提供模块业务操作的 Go 应用服务。
package module

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/ebpf"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

// Options 描述模块目录、运行时目录和平台适配器路径。
type Options struct {
	ModuleDir          string
	ManagerVersion     string
	ManagerVersionCode string
	CatalogRoot        string
	ModuleConfig       string
	EBPFConfig         string
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
	SkipServiceReload  bool
	RequestTimeout     time.Duration
}

// NewOptions 根据模块根目录返回完整的默认路径。
func NewOptions(moduleDir string) Options {
	layout := paths.New(moduleDir)
	return Options{
		ModuleDir:      layout.Root(),
		CatalogRoot:    layout.Catalog(),
		ModuleConfig:   layout.ModuleConfig(),
		EBPFConfig:     layout.EBPFConfig(),
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
	}
}

// PrepareResult 描述一次运行时准备结果。
type PrepareResult struct {
	catalog.RuntimeResult
	Providers string `json:"providers"`
	Outbounds string `json:"outbounds"`
	EBPF      string `json:"ebpf"`
}

// AppPolicy 描述分应用代理的持久设置。
type AppPolicy struct {
	Enabled    bool   `json:"enabled"`
	Mode       string `json:"mode"`
	ProxyApps  string `json:"proxy_apps"`
	BypassApps string `json:"bypass_apps"`
}

// Prepare 生成 Catalog、出站和 eBPF 运行时配置，并同步规范化选择状态。
func Prepare(ctx context.Context, options Options, allowEmpty bool) (PrepareResult, error) {
	if err := options.validate(); err != nil {
		return PrepareResult{}, err
	}
	for _, path := range []string{options.RuntimeDir, filepath.Dir(options.StateFile)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return PrepareResult{}, err
		}
	}
	providers := filepath.Join(options.RuntimeDir, "providers.json")
	outbounds := filepath.Join(options.RuntimeDir, "outbounds.json")
	ebpfPath := filepath.Join(options.RuntimeDir, "ebpf.json")
	runtime, err := catalog.BuildRuntime(ctx, catalog.RuntimeOptions{
		Root: options.CatalogRoot, ModuleConfig: options.ModuleConfig,
		ProvidersOutput: providers, OutboundsOutput: outbounds,
		AllowEmpty: allowEmpty,
	})
	if err != nil {
		return PrepareResult{}, err
	}
	config, err := ebpf.Load(options.EBPFConfig)
	if err != nil {
		return PrepareResult{}, err
	}
	missingPackages, err := ebpf.WriteAtomic(ebpfPath, config)
	if err != nil {
		return PrepareResult{}, err
	}
	for _, ref := range missingPackages {
		logService(options, "WARN", "ebpf.package", "skipped", "分应用代理跳过未安装应用: %s", ref.String())
	}
	return PrepareResult{RuntimeResult: runtime, Providers: providers, Outbounds: outbounds, EBPF: ebpfPath}, nil
}

func syncRuntimeSelection(path string, runtime catalog.RuntimeResult) error {
	module, err := moduleconfig.LoadModule(path)
	if err != nil {
		return err
	}
	updates := map[string]string{}
	if module.ActiveGroupID != runtime.ActiveGroup {
		updates["ACTIVE_GROUP_ID"] = moduleconfig.Quote(runtime.ActiveGroup)
	}
	if module.SelectorMode != runtime.SelectorMode {
		updates["SELECTOR_MODE"] = runtime.SelectorMode
	}
	if module.SelectedNodeRef != runtime.SelectedNodeRef {
		updates["SELECTED_NODE_REF"] = moduleconfig.Quote(runtime.SelectedNodeRef)
	}
	return moduleconfig.UpdateModule(path, updates)
}

// Check 生成隔离运行时配置并执行 sing-box check。
func Check(ctx context.Context, options Options, allowEmpty bool) (PrepareResult, error) {
	prepared, err := Prepare(ctx, options, allowEmpty)
	if err != nil {
		return PrepareResult{}, err
	}
	if options.SingBoxPath == "" {
		return prepared, errors.New("sing-box 路径为空")
	}
	confDir := paths.SingBoxConfDir(options.SingBoxDir)
	command := exec.CommandContext(ctx, options.SingBoxPath, "check", "-C", confDir,
		"-c", prepared.Providers, "-c", prepared.Outbounds, "-c", prepared.EBPF)
	command.Dir = options.SingBoxDir
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return prepared, fmt.Errorf("sing-box 配置检查失败: %w", err)
	}
	return prepared, nil
}

// SelectNode 更新持久选择，并在服务运行时通过 Service API 同步选择器。
func SelectNode(ctx context.Context, options Options, target, group string) (data map[string]string, err error) {
	persisted := false
	defer func() { logOperation(options, "node", "node.select", "节点选择", persisted, err) }()
	if err := options.validate(); err != nil {
		return nil, err
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return nil, err
	}
	if target == "auto" {
		if strings.TrimSpace(group) == "" {
			group = module.ActiveGroupID
		}
		group, err = catalog.ResolveGroup(options.CatalogRoot, group)
		if err != nil {
			return nil, err
		}
		hasNodes, err := catalog.GroupHasNodes(ctx, options.CatalogRoot, group)
		if err != nil || !hasNodes {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("目标分组没有可用节点")
		}
		updates := map[string]string{
			"ACTIVE_GROUP_ID": moduleconfig.Quote(group), "SELECTOR_MODE": "urltest",
			"SELECTED_NODE_REF": moduleconfig.Quote(""),
		}
		if err := moduleconfig.UpdateModule(options.ModuleConfig, updates); err != nil {
			return nil, err
		}
		persisted = true
		runtimeTag, err := catalog.RuntimeTag(options.CatalogRoot, group)
		if err != nil {
			return nil, err
		}
		if err := syncRuntimeSelector(ctx, options, "Auto/"+runtimeTag, ""); err != nil {
			return nil, err
		}
		return map[string]string{"group_id": group, "mode": "urltest", "selected": "Auto/" + runtimeTag}, nil
	}
	groupID, tag, found := strings.Cut(target, "/")
	if !found || groupID == "" || tag == "" {
		return nil, errors.New("节点引用格式应为 <group-id>/<tag>")
	}
	groupID, err = catalog.ResolveGroup(options.CatalogRoot, groupID)
	if err != nil {
		return nil, err
	}
	present, err := catalog.GroupContainsTag(ctx, options.CatalogRoot, groupID, tag)
	if err != nil || !present {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("未找到节点: %s/%s", groupID, tag)
	}
	runtimeTag, err := catalog.RuntimeTag(options.CatalogRoot, groupID)
	if err != nil {
		return nil, err
	}
	if err := moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{
		"ACTIVE_GROUP_ID": moduleconfig.Quote(groupID), "SELECTOR_MODE": "manual",
		"SELECTED_NODE_REF": moduleconfig.Quote(groupID + "/" + tag),
	}); err != nil {
		return nil, err
	}
	persisted = true
	if err := syncRuntimeSelector(ctx, options, "Select/"+runtimeTag, runtimeTag+"/"+tag); err != nil {
		return nil, err
	}
	return map[string]string{"group_id": groupID, "mode": "manual", "selected": runtimeTag + "/" + tag}, nil
}

// SyncSelection 将 module.conf 中保存的选择同步到运行中的 sing-box。
func SyncSelection(ctx context.Context, options Options) (map[string]string, error) {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(module.ActiveGroupID) == "" {
		return map[string]string{"mode": "urltest", "selected": ""}, nil
	}
	if module.SelectorMode == "manual" && strings.TrimSpace(module.SelectedNodeRef) != "" {
		return SelectNode(ctx, options, module.SelectedNodeRef, "")
	}
	return SelectNode(ctx, options, "auto", module.ActiveGroupID)
}

func syncRuntimeSelector(ctx context.Context, options Options, active, inner string) error {
	if !service.ProcessRunning(options.SingBoxPath) {
		return nil
	}
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err == nil {
		err = retryRuntimeSelection(ctx, client, options, active, inner)
		client.Close()
		if err == nil {
			return nil
		}
	}
	if options.SkipServiceReload {
		return fmt.Errorf("Service API 切换失败，跳过嵌套服务 reload: %w", err)
	}
	_, reloadErr := ManageService(ctx, options, "reload")
	return reloadErr
}

// retryRuntimeSelection 等待 reload 后的 selector 完成注册，再同步组内与顶层选择器。
func retryRuntimeSelection(ctx context.Context, client *serviceapi.Client, options Options, active, inner string) error {
	const backoff = 300 * time.Millisecond
	deadline := time.Now().Add(minTimeout(options.RequestTimeout, 6*time.Second))
	var lastErr error
	for time.Now().Before(deadline) {
		requestContext, cancel := context.WithTimeout(ctx, minTimeout(options.RequestTimeout, time.Second))
		lastErr = nil
		if inner != "" {
			lastErr = client.Select(requestContext, active, inner)
			if lastErr == nil {
				lastErr = client.Select(requestContext, "Proxy", active)
			}
		} else {
			lastErr = client.Select(requestContext, "Proxy", active)
		}
		cancel()
		if lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return lastErr
}

// ApplyMode 持久化出站模式，并优先使用 Service API 同步运行实例。
func ApplyMode(ctx context.Context, options Options, mode string) (err error) {
	persisted := false
	defer func() { logOperation(options, "mode", "mode.apply", "出站模式切换", persisted, err) }()
	if mode != "rule" && mode != "global" && mode != "direct" && mode != "AllowAds" {
		return fmt.Errorf("未知出站模式: %s", mode)
	}
	if err := moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{"OUTBOUND_MODE": mode}); err != nil {
		return err
	}
	persisted = true
	if !service.ProcessRunning(options.SingBoxPath) {
		return nil
	}
	mapped := map[string]string{"rule": "Rule", "global": "Global", "direct": "Direct", "AllowAds": "AllowAds"}[mode]
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err == nil {
		requestContext, cancel := context.WithTimeout(ctx, minTimeout(options.RequestTimeout, 4*time.Second))
		err = client.SetMode(requestContext, mapped)
		cancel()
		client.Close()
		if err == nil {
			return nil
		}
	}
	if options.SkipServiceReload {
		return fmt.Errorf("Service API 模式切换失败，跳过嵌套服务 reload: %w", err)
	}
	_, reloadErr := ManageService(ctx, options, "reload")
	return reloadErr
}

// UpdateApp 按类型化 eBPF 配置修改分应用策略。
func UpdateApp(options Options, action, value string) (data AppPolicy, err error) {
	persisted := false
	defer func() { logOperation(options, "app", "app-policy.update", "分应用策略更新", persisted, err) }()
	config, err := ebpf.Load(options.EBPFConfig)
	if err != nil {
		return AppPolicy{}, err
	}
	updates := map[string]string{}
	switch action {
	case "mode":
		if value != "blacklist" && value != "whitelist" {
			return AppPolicy{}, errors.New("应用模式应为 blacklist 或 whitelist")
		}
		updates["APP_PROXY_ENABLE"] = "1"
		updates["APP_PROXY_MODE"] = moduleconfig.Quote(value)
	case "add":
		ref, err := ebpf.ParsePackageRef(value)
		if err != nil {
			return AppPolicy{}, err
		}
		if config.AppProxyMode == "whitelist" {
			updates["PROXY_APPS_LIST"] = moduleconfig.Quote(addPackageRef(config.ProxyPackages, ref))
		} else {
			updates["BYPASS_APPS_LIST"] = moduleconfig.Quote(addPackageRef(config.BypassPackages, ref))
		}
		updates["APP_PROXY_ENABLE"] = "1"
	case "remove":
		ref, err := ebpf.ParsePackageRef(value)
		if err != nil {
			return AppPolicy{}, err
		}
		updates["PROXY_APPS_LIST"] = moduleconfig.Quote(removePackageRef(config.ProxyPackages, ref))
		updates["BYPASS_APPS_LIST"] = moduleconfig.Quote(removePackageRef(config.BypassPackages, ref))
	case "enable", "disable":
		updates["APP_PROXY_ENABLE"] = map[string]string{"enable": "1", "disable": "0"}[action]
	default:
		return AppPolicy{}, fmt.Errorf("未知应用操作: %s", action)
	}
	if err := moduleconfig.UpdateValidated(options.EBPFConfig, updates, func(candidate string) error {
		_, validateErr := ebpf.Load(candidate)
		return validateErr
	}); err != nil {
		return AppPolicy{}, err
	}
	persisted = true
	config, err = ebpf.Load(options.EBPFConfig)
	if err != nil {
		return AppPolicy{}, err
	}
	return appPolicy(config), nil
}

func appPolicy(config ebpf.Config) AppPolicy {
	return AppPolicy{
		Enabled:    config.AppProxyEnable,
		Mode:       config.AppProxyMode,
		ProxyApps:  joinPackageRefs(config.ProxyPackages),
		BypassApps: joinPackageRefs(config.BypassPackages),
	}
}

// NodeAppend 将节点加入本地分组并处理活动状态与运行时 reload。
func NodeAppend(ctx context.Context, options Options, groupID, input string, allowInsecure bool) (mutation catalog.MutationResult, err error) {
	defer func() { logOperation(options, "node", "node.append", "节点添加", mutation.Revision > 0, err) }()
	if err := ensureDefaultGroup(ctx, options); err != nil {
		return catalog.MutationResult{}, err
	}
	if groupID == "" {
		groupID = "default"
	}
	groupID, err = catalog.ResolveGroup(options.CatalogRoot, groupID)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	result, err := catalog.AppendNode(ctx, catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Type: "local", Input: input, AllowInsecure: allowInsecure})
	if err != nil {
		return catalog.MutationResult{}, err
	}
	if err := syncCatalogChange(ctx, options, groupID, result.StructureChanged); err != nil {
		return result, err
	}
	return result, nil
}

// NodeImport 将本地文件中的节点追加到 default 本地配置组。
func NodeImport(ctx context.Context, options Options, input string, allowInsecure bool) (mutation catalog.MutationResult, err error) {
	defer func() { logOperation(options, "node", "node.import", "节点导入", mutation.Revision > 0, err) }()
	if err := ensureDefaultGroup(ctx, options); err != nil {
		return catalog.MutationResult{}, err
	}
	const groupID = "default"
	result, err := catalog.AppendNode(ctx, catalog.MutationOptions{
		GroupDir:      filepath.Join(options.CatalogRoot, groupID),
		GroupID:       groupID,
		Type:          "local",
		Input:         input,
		AllowInsecure: allowInsecure,
	})
	if err != nil {
		return catalog.MutationResult{}, err
	}
	if err := syncCatalogChange(ctx, options, groupID, result.StructureChanged); err != nil {
		return result, err
	}
	return result, nil
}

// NodeEdit 原子替换指定分组的节点。
func NodeEdit(ctx context.Context, options Options, reference, input string, allowInsecure bool) (mutation catalog.MutationResult, err error) {
	defer func() { logOperation(options, "node", "node.edit", "节点编辑", mutation.Revision > 0, err) }()
	groupID, tag, err := splitReference(reference)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	groupID, err = catalog.ResolveGroup(options.CatalogRoot, groupID)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	result, err := catalog.EditNode(ctx, catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Tag: tag, Input: input, AllowInsecure: allowInsecure})
	if err != nil {
		return catalog.MutationResult{}, err
	}
	if err := syncCatalogChange(ctx, options, groupID, result.StructureChanged); err != nil {
		return result, err
	}
	return result, nil
}

// NodeRemove 删除指定节点，并在手动节点消失时回退 Auto。
func NodeRemove(ctx context.Context, options Options, reference string) (mutation catalog.MutationResult, err error) {
	defer func() { logOperation(options, "node", "node.remove", "节点删除", mutation.Revision > 0, err) }()
	groupID, tag, err := splitReference(reference)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	groupID, err = catalog.ResolveGroup(options.CatalogRoot, groupID)
	if err != nil {
		return catalog.MutationResult{}, err
	}
	result, err := catalog.RemoveNode(ctx, catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Tag: tag})
	if err != nil {
		return catalog.MutationResult{}, err
	}
	if err := fallbackMissingNode(ctx, options, groupID); err != nil {
		return result, err
	}
	if err := syncCatalogChange(ctx, options, groupID, result.StructureChanged); err != nil {
		return result, err
	}
	return result, nil
}

// RemoveSubscription 删除订阅并处理活动分组替代。
func RemoveSubscription(ctx context.Context, options Options, query, replacement string) (err error) {
	deleted := false
	defer func() { logOperation(options, "subscription", "subscription.remove", "订阅删除", deleted, err) }()
	groupID, err := catalog.ResolveGroup(options.CatalogRoot, query)
	if err != nil {
		return err
	}
	typ, err := catalog.GroupType(options.CatalogRoot, groupID)
	if err != nil || typ != "subscription" {
		return errors.New("目标不是 URL 订阅")
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return err
	}
	if module.ActiveGroupID == groupID {
		if replacement != "" {
			replacement, err = catalog.ResolveGroup(options.CatalogRoot, replacement)
			if err != nil {
				return err
			}
		} else {
			replacement, err = catalog.FirstNonEmptyGroup(ctx, options.CatalogRoot, groupID)
			if err != nil {
				return err
			}
		}
		if replacement != "" {
			if _, err := SelectNode(ctx, options, "auto", replacement); err != nil {
				return err
			}
		} else {
			if err := moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{"ACTIVE_GROUP_ID": moduleconfig.Quote(""), "SELECTOR_MODE": "urltest", "SELECTED_NODE_REF": moduleconfig.Quote("")}); err != nil {
				return err
			}
			if service.ProcessRunning(options.SingBoxPath) {
				if _, err := ManageService(ctx, options, "stop"); err != nil {
					return err
				}
			}
		}
	}
	if err := catalog.DeleteGroup(options.CatalogRoot, groupID); err != nil {
		return err
	}
	deleted = true
	if service.ProcessRunning(options.SingBoxPath) {
		_, reloadErr := ManageService(ctx, options, "reload")
		return reloadErr
	}
	return nil
}

func syncCatalogChange(ctx context.Context, options Options, groupID string, structureChanged bool) error {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return err
	}
	hasActive, err := catalog.GroupHasNodes(ctx, options.CatalogRoot, module.ActiveGroupID)
	if err != nil {
		return err
	}
	if !hasActive {
		if hasNodes, _ := catalog.GroupHasNodes(ctx, options.CatalogRoot, groupID); hasNodes {
			_, err := SelectNode(ctx, options, "auto", groupID)
			return err
		}
		replacement, _ := catalog.FirstNonEmptyGroup(ctx, options.CatalogRoot, module.ActiveGroupID)
		if replacement != "" {
			_, err := SelectNode(ctx, options, "auto", replacement)
			return err
		}
		if _, statErr := os.Stat(filepath.Join(options.CatalogRoot, "default", "meta.json")); statErr == nil {
			if err := moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{"ACTIVE_GROUP_ID": moduleconfig.Quote("default"), "SELECTOR_MODE": "urltest", "SELECTED_NODE_REF": moduleconfig.Quote("")}); err != nil {
				return err
			}
		} else if err := moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{"ACTIVE_GROUP_ID": moduleconfig.Quote(""), "SELECTOR_MODE": "urltest", "SELECTED_NODE_REF": moduleconfig.Quote("")}); err != nil {
			return err
		}
	}
	if structureChanged && service.ProcessRunning(options.SingBoxPath) {
		_, reloadErr := ManageService(ctx, options, "reload")
		return reloadErr
	}
	return nil
}

func fallbackMissingNode(ctx context.Context, options Options, groupID string) error {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.SelectorMode != "manual" {
		return err
	}
	selectedGroup, tag, found := strings.Cut(module.SelectedNodeRef, "/")
	if !found || selectedGroup != groupID || tag == "" {
		return nil
	}
	present, err := catalog.GroupContainsTag(ctx, options.CatalogRoot, groupID, tag)
	if err != nil || present {
		return err
	}
	_, err = SelectNode(ctx, options, "auto", groupID)
	return err
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
	for name, value := range map[string]string{"模块配置": options.ModuleConfig, "Catalog": options.CatalogRoot, "eBPF 配置": options.EBPFConfig} {
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

func addPackageRef(current []ebpf.PackageRef, value ebpf.PackageRef) string {
	if slices.Contains(current, value) {
		return joinPackageRefs(current)
	}
	return joinPackageRefs(append(append([]ebpf.PackageRef{}, current...), value))
}

func removePackageRef(current []ebpf.PackageRef, value ebpf.PackageRef) string {
	items := make([]ebpf.PackageRef, 0, len(current))
	for _, ref := range current {
		if ref != value {
			items = append(items, ref)
		}
	}
	return joinPackageRefs(items)
}

func joinPackageRefs(values []ebpf.PackageRef) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, value.String())
	}
	return strings.Join(items, ",")
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
	if err := ensureDefaultGroup(ctx, options.Options); err != nil {
		return subscription.Result{}, err
	}
	groupID, err := catalog.NewSubscriptionGroupID(options.CatalogRoot)
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
	groupID, err := catalog.ResolveGroup(options.CatalogRoot, query)
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
	groupID, err := catalog.ResolveGroup(options.CatalogRoot, query)
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
	ids, err := catalog.GroupIDs(options.CatalogRoot, "subscription")
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
		PIDFile:             options.WorkerPIDFile,
		LogFile:             options.WorkerLogFile,
		ModuleConf:          options.ModuleConfig,
		SingBoxPath:         options.SingBoxPath,
		ServiceAddress:      options.ServiceAddress,
		ServiceSecret:       options.ServiceSecret,
		NetworkWatchEnabled: true,
		ReloadService: func(ctx context.Context) error {
			return ReloadService(ctx, options)
		},
		Now: time.Now,
		NetworkEvaluate: func(ctx context.Context, networkType, ssid string) error {
			_, err := EvaluateNetwork(ctx, options, networkType, ssid)
			return err
		},
	}
	return workerOptions
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
	config, err := ebpf.Load(configPath)
	if err != nil {
		return AppPolicy{}, err
	}
	return appPolicy(config), nil
}
