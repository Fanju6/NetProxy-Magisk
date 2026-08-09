// Package module 提供模块业务操作的 Go 应用服务。
package module

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/ebpf"
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
	ServiceScript      string
	ServiceAddress     string
	ServiceSecret      string
	WorkerPIDFile      string
	WorkerLogFile      string
	WiFiStateFile      string
	SkipServiceAdapter bool
	RequestTimeout     time.Duration
}

// NewOptions 根据模块根目录返回完整的默认路径。
func NewOptions(moduleDir string) Options {
	configDir := filepath.Join(moduleDir, "config")
	dataDir := filepath.Join(moduleDir, "data")
	runtimeDir := filepath.Join(moduleDir, "runtime")
	singBoxDir := filepath.Join(configDir, "singbox")
	return Options{
		ModuleDir:      moduleDir,
		CatalogRoot:    filepath.Join(dataDir, "catalog"),
		ModuleConfig:   filepath.Join(configDir, "module.conf"),
		EBPFConfig:     filepath.Join(configDir, "ebpf", "ebpf.conf"),
		SingBoxPath:    filepath.Join(moduleDir, "bin", "sing-box"),
		SingBoxDir:     singBoxDir,
		RuntimeDir:     runtimeDir,
		StateFile:      filepath.Join(runtimeDir, "service.json"),
		ProgressDir:    "/dev/netproxy/subscriptions",
		LogDir:         filepath.Join(moduleDir, "logs"),
		ServiceScript:  filepath.Join(moduleDir, "scripts", "core", "service.sh"),
		ServiceAddress: "127.0.0.1:9090",
		ServiceSecret:  "singbox",
		WorkerPIDFile:  "/dev/netproxy/worker.pid",
		WorkerLogFile:  filepath.Join(moduleDir, "logs", "service.log"),
		WiFiStateFile:  "/dev/netproxy/wifi_state",
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
	statePath := filepath.Join(options.RuntimeDir, "catalog.state")
	runtime, err := catalog.BuildRuntime(ctx, catalog.RuntimeOptions{
		Root: options.CatalogRoot, ModuleConfig: options.ModuleConfig,
		ProvidersOutput: providers, OutboundsOutput: outbounds, StateOutput: statePath,
		AllowEmpty: allowEmpty,
	})
	if err != nil {
		return PrepareResult{}, err
	}
	if !allowEmpty {
		if err := syncRuntimeSelection(options.ModuleConfig, runtime); err != nil {
			return PrepareResult{}, err
		}
	}
	config, err := ebpf.Load(options.EBPFConfig)
	if err != nil {
		return PrepareResult{}, err
	}
	if err := ebpf.WriteAtomic(ebpfPath, config); err != nil {
		return PrepareResult{}, err
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
	confDir := filepath.Join(options.SingBoxDir, "confdir")
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
func SelectNode(ctx context.Context, options Options, target, group string) (map[string]string, error) {
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
	if options.SkipServiceAdapter {
		return fmt.Errorf("Service API 切换失败，跳过嵌套服务 reload: %w", err)
	}
	return runServiceAdapter(ctx, options, "reload")
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
func ApplyMode(ctx context.Context, options Options, mode string) error {
	if mode != "rule" && mode != "global" && mode != "direct" && mode != "AllowAds" {
		return fmt.Errorf("未知出站模式: %s", mode)
	}
	if err := moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{"OUTBOUND_MODE": mode}); err != nil {
		return err
	}
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
	if options.SkipServiceAdapter {
		return fmt.Errorf("Service API 模式切换失败，跳过嵌套服务 reload: %w", err)
	}
	return runServiceAdapter(ctx, options, "reload")
}

// UpdateApp 按类型化 eBPF 配置修改分应用策略。
func UpdateApp(options Options, action, value, users string) (map[string]any, error) {
	config, err := ebpf.Load(options.EBPFConfig)
	if err != nil {
		return nil, err
	}
	updates := map[string]string{}
	switch action {
	case "mode":
		if value != "blacklist" && value != "whitelist" {
			return nil, errors.New("应用模式应为 blacklist 或 whitelist")
		}
		updates["APP_PROXY_ENABLE"] = "1"
		updates["APP_PROXY_MODE"] = moduleconfig.Quote(value)
	case "users":
		if users == "all" {
			users = ""
		}
		if err := validateWords(users, true); err != nil {
			return nil, errors.New("Android 用户 ID 只能是非负整数")
		}
		updates["APP_ANDROID_USERS"] = moduleconfig.Quote(users)
	case "add":
		if err := validatePackage(value); err != nil {
			return nil, err
		}
		if config.AppProxyMode == "whitelist" {
			updates["PROXY_APPS_LIST"] = moduleconfig.Quote(addWord(strings.Join(config.ProxyPackages, " "), value))
		} else {
			updates["BYPASS_APPS_LIST"] = moduleconfig.Quote(addWord(strings.Join(config.BypassPackages, " "), value))
		}
		updates["APP_PROXY_ENABLE"] = "1"
	case "remove":
		if err := validatePackage(value); err != nil {
			return nil, err
		}
		updates["PROXY_APPS_LIST"] = moduleconfig.Quote(removeWord(strings.Join(config.ProxyPackages, " "), value))
		updates["BYPASS_APPS_LIST"] = moduleconfig.Quote(removeWord(strings.Join(config.BypassPackages, " "), value))
	case "enable", "disable":
		updates["APP_PROXY_ENABLE"] = map[string]string{"enable": "1", "disable": "0"}[action]
	default:
		return nil, fmt.Errorf("未知应用操作: %s", action)
	}
	if err := moduleconfig.UpdateValidated(options.EBPFConfig, updates, func(candidate string) error {
		_, validateErr := ebpf.Load(candidate)
		return validateErr
	}); err != nil {
		return nil, err
	}
	config, err = ebpf.Load(options.EBPFConfig)
	if err != nil {
		return nil, err
	}
	return appData(config), nil
}

func appData(config ebpf.Config) map[string]any {
	return map[string]any{"enabled": config.AppProxyEnable, "mode": config.AppProxyMode,
		"android_users": joinUint(config.AndroidUsers), "proxy_apps": strings.Join(config.ProxyPackages, " "),
		"bypass_apps": strings.Join(config.BypassPackages, " ")}
}

// NodeAppend 将节点加入本地分组并处理活动状态与运行时 reload。
func NodeAppend(ctx context.Context, options Options, groupID, input string, allowInsecure bool) (catalog.MutationResult, error) {
	if err := ensureDefaultGroup(ctx, options); err != nil {
		return catalog.MutationResult{}, err
	}
	if groupID == "" {
		groupID = "default"
	}
	groupID, err := catalog.ResolveGroup(options.CatalogRoot, groupID)
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

// NodeImport 创建本地文件分组。
func NodeImport(ctx context.Context, options Options, input, name string, allowInsecure bool) (string, error) {
	if err := ensureDefaultGroup(ctx, options); err != nil {
		return "", err
	}
	groupID, err := catalog.NewGroupID(options.CatalogRoot, "file", input)
	if err != nil {
		return "", err
	}
	if _, err := catalog.ImportGroup(ctx, catalog.ImportOptions{Root: options.CatalogRoot, GroupID: groupID, Name: name, Input: input, AllowInsecure: allowInsecure}); err != nil {
		return "", err
	}
	if err := syncCatalogChange(ctx, options, groupID, true); err != nil {
		return groupID, err
	}
	return groupID, nil
}

// NodeEdit 原子替换指定分组的节点。
func NodeEdit(ctx context.Context, options Options, reference, input string, allowInsecure bool) (catalog.MutationResult, error) {
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
func NodeRemove(ctx context.Context, options Options, reference string) (catalog.MutationResult, error) {
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
func RemoveSubscription(ctx context.Context, options Options, query, replacement string) error {
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
				if err := runServiceAdapter(ctx, options, "stop"); err != nil {
					return err
				}
			}
		}
	}
	if err := catalog.DeleteGroup(options.CatalogRoot, groupID); err != nil {
		return err
	}
	if service.ProcessRunning(options.SingBoxPath) {
		return runServiceAdapter(ctx, options, "reload")
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
		return runServiceAdapter(ctx, options, "reload")
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

func runServiceAdapter(ctx context.Context, options Options, action string) error {
	shell := "/system/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		shell = "sh"
	}
	command := exec.CommandContext(ctx, shell, options.ServiceScript, action)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("服务 %s 适配失败: %w", action, err)
	}
	return nil
}

func (options Options) validate() error {
	for name, value := range map[string]string{"模块配置": options.ModuleConfig, "Catalog": options.CatalogRoot, "eBPF 配置": options.EBPFConfig} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s路径不能为空", name)
		}
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 8 * time.Second
	}
	return nil
}

func minTimeout(value, fallback time.Duration) time.Duration {
	if value <= 0 || value > fallback {
		return fallback
	}
	return value
}

func validatePackage(value string) error {
	if value == "" {
		return errors.New("应用包名不能为空")
	}
	for _, char := range value {
		if !(char == '.' || char == '_' || (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')) {
			return errors.New("应用包名格式无效")
		}
	}
	return nil
}

func validateWords(value string, numeric bool) error {
	for _, word := range strings.Fields(value) {
		if numeric {
			for _, char := range word {
				if char < '0' || char > '9' {
					return errors.New("列表值必须为非负整数")
				}
			}
		}
	}
	return nil
}

func addWord(current, value string) string {
	for _, word := range strings.Fields(current) {
		if word == value {
			return current
		}
	}
	if strings.TrimSpace(current) == "" {
		return value
	}
	return strings.TrimSpace(current) + " " + value
}

func removeWord(current, value string) string {
	items := make([]string, 0)
	for _, word := range strings.Fields(current) {
		if word != value {
			items = append(items, word)
		}
	}
	return strings.Join(items, " ")
}

func joinUint(values []uint64) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, fmt.Sprintf("%d", value))
	}
	return strings.Join(items, " ")
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
func AddSubscription(ctx context.Context, options SubscriptionOptions) (subscription.Result, error) {
	if options.URL == "" {
		return subscription.Result{}, errors.New("订阅 URL 不能为空")
	}
	if err := ensureDefaultGroup(ctx, options.Options); err != nil {
		return subscription.Result{}, err
	}
	groupID, err := catalog.NewGroupID(options.CatalogRoot, "subscription", options.URL)
	if err != nil {
		return subscription.Result{}, err
	}
	if err := catalog.InitializeGroup(ctx, catalog.GroupOptions{Root: options.CatalogRoot, GroupID: groupID, Name: options.Name, Type: "subscription", URL: options.URL, UserAgent: options.UserAgent, HWID: options.HWID, CustomHeaders: options.Headers, AutoUpdate: options.AutoUpdate, UpdateInterval: options.UpdateInterval, IntervalSource: options.IntervalSource, UpdateViaProxy: options.UpdateViaProxy, Include: options.Include, Exclude: options.Exclude, AllowInsecure: options.AllowInsecure, Timeout: options.Timeout}); err != nil {
		return subscription.Result{}, err
	}
	workerOptions := workerOptions(options.Options)
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
func UpdateSubscription(ctx context.Context, options Options, query string) (subscription.Result, error) {
	groupID, err := catalog.ResolveGroup(options.CatalogRoot, query)
	if err != nil {
		return subscription.Result{}, err
	}
	return worker.UpdateGroup(ctx, workerOptions(options), groupID, time.Now(), nil)
}

// UpdateAllSubscriptions 按 Catalog 顺序更新全部订阅。
func UpdateAllSubscriptions(ctx context.Context, options Options) (worker.Summary, error) {
	ids, err := catalog.GroupIDs(options.CatalogRoot, "subscription")
	if err != nil {
		return worker.Summary{}, err
	}
	summary := worker.Summary{Updated: []string{}, Failed: []string{}}
	for _, id := range ids {
		if _, updateErr := worker.UpdateGroup(ctx, workerOptions(options), id, time.Now(), nil); updateErr != nil {
			summary.Failed = append(summary.Failed, id)
		} else {
			summary.Updated = append(summary.Updated, id)
		}
	}
	return summary, nil
}

func workerOptions(options Options) worker.Options {
	return worker.Options{Root: options.CatalogRoot, ProgressDir: options.ProgressDir, PIDFile: options.WorkerPIDFile, LogFile: options.WorkerLogFile, ModuleConf: options.ModuleConfig, ReloadScript: options.ServiceScript, SingBoxPath: options.SingBoxPath, ServiceAddress: options.ServiceAddress, ServiceSecret: options.ServiceSecret, Now: time.Now}
}

func hostName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// MarshalAppData 返回应用设置的 JSON 摘要，供 CLI 和客户端复用。
func MarshalAppData(configPath string) (json.RawMessage, error) {
	config, err := ebpf.Load(configPath)
	if err != nil {
		return nil, err
	}
	content, err := json.Marshal(appData(config))
	return content, err
}
