package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

type moduleFlags struct {
	moduleDir, managerVersion, managerVersionCode, catalogRoot, moduleConfig, ebpfConfig, singBox, singBoxDir string
	runtimeDir, progressDir, serviceScript, address, secret, logDir                                           string
	stateFile, workerPID, workerLog                                                                           string
	skipServiceAdapter                                                                                        bool
	timeout                                                                                                   time.Duration
}

func bindModuleFlags(flags *flag.FlagSet) *moduleFlags {
	values := &moduleFlags{}
	flags.StringVar(&values.moduleDir, "module-dir", defaultModuleDir(), "模块根目录")
	flags.StringVar(&values.managerVersion, "manager-version", "unknown", "Android 管理器版本")
	flags.StringVar(&values.managerVersionCode, "manager-version-code", "unknown", "Android 管理器版本号")
	flags.StringVar(&values.catalogRoot, "catalog-root", "", "Catalog 根目录")
	flags.StringVar(&values.moduleConfig, "module-config", "", "module.conf 路径")
	flags.StringVar(&values.ebpfConfig, "ebpf-config", "", "ebpf.conf 路径")
	flags.StringVar(&values.singBox, "sing-box", "", "sing-box 路径")
	flags.StringVar(&values.singBoxDir, "singbox-dir", "", "sing-box 配置目录")
	flags.StringVar(&values.runtimeDir, "runtime-dir", "", "运行时目录")
	flags.StringVar(&values.progressDir, "progress-dir", "/dev/netproxy/subscriptions", "订阅进度目录")
	flags.StringVar(&values.serviceScript, "service-script", "", "服务生命周期适配脚本")
	flags.StringVar(&values.address, "address", "127.0.0.1:9090", "Service API 地址")
	flags.StringVar(&values.secret, "secret", "singbox", "Service API 密钥")
	flags.StringVar(&values.logDir, "log-dir", "", "日志目录")
	flags.StringVar(&values.stateFile, "state-file", "", "服务状态文件")
	flags.StringVar(&values.workerPID, "worker-pid-file", "/dev/netproxy/worker.pid", "Worker PID 文件")
	flags.StringVar(&values.workerLog, "worker-log-file", "", "Worker 日志文件")
	flags.BoolVar(&values.skipServiceAdapter, "skip-service-adapter", false, "服务内部同步时禁止嵌套 reload")
	flags.DurationVar(&values.timeout, "timeout", 8*time.Second, "Service API 超时")
	return values
}

func (values *moduleFlags) options() moduleapp.Options {
	options := moduleapp.NewOptions(values.moduleDir)
	options.ManagerVersion = values.managerVersion
	options.ManagerVersionCode = values.managerVersionCode
	if values.catalogRoot != "" {
		options.CatalogRoot = values.catalogRoot
	}
	if values.moduleConfig != "" {
		options.ModuleConfig = values.moduleConfig
	}
	if values.ebpfConfig != "" {
		options.EBPFConfig = values.ebpfConfig
	}
	if values.singBox != "" {
		options.SingBoxPath = values.singBox
	}
	if values.singBoxDir != "" {
		options.SingBoxDir = values.singBoxDir
	}
	if values.runtimeDir != "" {
		options.RuntimeDir = values.runtimeDir
	}
	if values.progressDir != "" {
		options.ProgressDir = values.progressDir
	}
	if values.serviceScript != "" {
		options.ServiceScript = values.serviceScript
	}
	if values.address != "" {
		options.ServiceAddress = values.address
	}
	if values.secret != "" {
		options.ServiceSecret = values.secret
	}
	if values.logDir != "" {
		options.LogDir = values.logDir
	}
	if values.stateFile != "" {
		options.StateFile = values.stateFile
	}
	if values.workerPID != "" {
		options.WorkerPIDFile = values.workerPID
	}
	if values.workerLog != "" {
		options.WorkerLogFile = values.workerLog
	}
	options.SkipServiceAdapter = values.skipServiceAdapter
	if values.timeout > 0 {
		options.RequestTimeout = values.timeout
	}
	return options
}

func defaultModuleDir() string {
	executable, err := os.Executable()
	if err == nil {
		return filepath.Dir(filepath.Dir(executable))
	}
	return "."
}

func runModule(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少模块业务操作: prepare|select|mode|network|app|node|sub|config|logs|service")
	}
	action := args[0]
	switch action {
	case "prepare":
		return runModulePrepare(ctx, args[1:])
	case "select":
		return runModuleSelect(ctx, args[1:])
	case "sync":
		return runModuleSync(ctx, args[1:])
	case "mode":
		return runModuleMode(ctx, args[1:])
	case "network":
		return runModuleNetwork(ctx, args[1:])
	case "app":
		return runModuleApp(ctx, args[1:])
	case "node":
		return runModuleNode(ctx, args[1:])
	case "sub":
		return runModuleSub(ctx, args[1:])
	case "config":
		return runModuleConfig(ctx, args[1:])
	case "logs":
		return runModuleLogs(ctx, args[1:])
	case "state":
		return runModuleState(args[1:])
	case "service":
		return runModuleService(ctx, args[1:])
	default:
		return fmt.Errorf("未知模块业务操作 %q", action)
	}
}

func runModuleNetwork(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 network 操作: evaluate")
	}
	flags := newFlagSet("module network")
	values := bindModuleFlags(flags)
	networkType := flags.String("type", "not_wifi", "网络类型")
	ssid := flags.String("ssid", "", "当前 WiFi SSID")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if args[0] != "evaluate" {
		return fmt.Errorf("未知 network 操作 %q", args[0])
	}
	data, err := moduleapp.EvaluateNetwork(ctx, values.options(), *networkType, *ssid)
	if err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "network.evaluated", Message: data.Reason, Data: data})
	return nil
}

func runModuleState(args []string) error {
	flags := newFlagSet("module state")
	values := bindModuleFlags(flags)
	state := flags.String("state", "", "服务状态")
	pid := flags.String("pid", "0", "服务 PID")
	startedAt := flags.String("started-at", "0", "启动时间")
	readyAt := flags.String("ready-at", "0", "就绪时间")
	message := flags.String("error", "", "错误信息")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *state == "" {
		return errors.New("module state 需要 --state")
	}
	options := values.options()
	pidValue, err := moduleapp.ParseNonNegativeInt(*pid)
	if err != nil {
		return err
	}
	startedValue, err := moduleapp.ParseNonNegativeInt(*startedAt)
	if err != nil {
		return err
	}
	readyValue, err := moduleapp.ParseNonNegativeInt(*readyAt)
	if err != nil {
		return err
	}
	if err := moduleapp.WriteServiceState(options.StateFile, *state, pidValue, startedValue, readyValue, *message); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "service.state_written", Message: "服务状态已写入", Data: map[string]string{"state": *state}})
	return nil
}

func runModuleSync(ctx context.Context, args []string) error {
	flags := newFlagSet("module sync")
	values := bindModuleFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	data, err := moduleapp.SyncSelection(ctx, values.options())
	if err != nil {
		return err
	}
	module, err := moduleconfig.LoadModule(values.options().ModuleConfig)
	if err != nil {
		return err
	}
	if err := moduleapp.ApplyMode(ctx, values.options(), module.OutboundMode); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "module.selection_synced", Message: "运行时选择已同步", Data: data})
	return nil
}

func runModulePrepare(ctx context.Context, args []string) error {
	flags := newFlagSet("module prepare")
	values := bindModuleFlags(flags)
	allowEmpty := flags.Bool("allow-empty", false, "允许空 Catalog，仅用于配置检查")
	check := flags.Bool("check", false, "生成后执行 sing-box check")
	if err := flags.Parse(args); err != nil {
		return err
	}
	options := values.options()
	var prepared moduleapp.PrepareResult
	var err error
	if *check {
		prepared, err = moduleapp.Check(ctx, options, *allowEmpty)
	} else {
		prepared, err = moduleapp.Prepare(ctx, options, *allowEmpty)
	}
	if err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "module.runtime_prepared", Message: "运行时配置已准备", Data: prepared})
	return nil
}

func runModuleSelect(ctx context.Context, args []string) error {
	flags := newFlagSet("module select")
	values := bindModuleFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	positionals := flags.Args()
	if len(positionals) == 0 {
		return errors.New("module select 需要节点引用或 auto")
	}
	group := ""
	if len(positionals) > 1 {
		group = positionals[1]
	}
	data, err := moduleapp.SelectNode(ctx, values.options(), positionals[0], group)
	if err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.selected", Message: "节点选择已更新", Data: data})
	return nil
}

func runModuleMode(ctx context.Context, args []string) error {
	flags := newFlagSet("module mode")
	values := bindModuleFlags(flags)
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args); err != nil {
		return err
	}
	positionals := flags.Args()
	if len(positionals) == 0 {
		options := values.options()
		module, err := moduleconfig.LoadModule(options.ModuleConfig)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Printf("%s\n", module.OutboundMode)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.current", Message: "当前出站模式", Data: map[string]any{"mode": module.OutboundMode, "available": []string{"rule", "global", "direct", "AllowAds"}}})
		return nil
	}
	if err := moduleapp.ApplyMode(ctx, values.options(), positionals[0]); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.changed", Message: "出站模式已切换", Data: map[string]string{"mode": positionals[0]}})
	return nil
}

func runModuleApp(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 app 操作")
	}
	flags := newFlagSet("module app")
	values := bindModuleFlags(flags)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := values.options()
	action := args[0]
	positionals := flags.Args()
	if action == "list" {
		data, err := moduleapp.MarshalAppData(options.EBPFConfig)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "app.list", Message: "分应用代理配置", Data: data})
		return nil
	}
	value := ""
	users := ""
	if len(positionals) > 0 {
		value = positionals[0]
	}
	if action == "users" {
		users = strings.Join(positionals, " ")
		if users == "" {
			users = "all"
		}
	}
	data, err := moduleapp.UpdateApp(options, action, value, users)
	if err != nil {
		code := "app.update_failed"
		switch action {
		case "add", "remove":
			code = "app.package_invalid"
		case "users":
			code = "app.users_invalid"
		case "mode":
			code = "app.mode_invalid"
		}
		return &resultError{Code: code, Message: err.Error()}
	}
	code := "app." + action
	message := "分应用代理设置已更新"
	if action == "mode" {
		message = "分应用模式已更新"
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: data})
	return nil
}

func runModuleNode(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 node 操作")
	}
	flags := newFlagSet("module node")
	values := bindModuleFlags(flags)
	allowInsecure := flags.Bool("allow-insecure", false, "跳过节点 TLS 校验")
	name := flags.String("name", "", "本地分组名称")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := values.options()
	action := args[0]
	positionals := flags.Args()
	if len(positionals) == 0 && action != "list" {
		return errors.New("node 操作缺少参数")
	}
	switch action {
	case "add":
		group := "default"
		if len(positionals) > 1 {
			group = positionals[1]
		}
		data, err := moduleapp.NodeAppend(ctx, options, group, positionals[0], *allowInsecure)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.added", Message: "节点已加入本地配置", Data: data})
	case "import":
		group, err := moduleapp.NodeImport(ctx, options, positionals[0], *name, *allowInsecure)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.imported", Message: "文件节点已导入", Data: map[string]string{"group_id": group}})
	case "edit":
		if len(positionals) < 2 {
			return errors.New("node edit 需要节点引用和节点内容")
		}
		data, err := moduleapp.NodeEdit(ctx, options, positionals[0], positionals[1], *allowInsecure)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.edited", Message: "节点已更新", Data: data})
	case "remove":
		data, err := moduleapp.NodeRemove(ctx, options, positionals[0])
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.removed", Message: "节点已删除", Data: data})
	default:
		return fmt.Errorf("未知 node 变更操作 %q", action)
	}
	return nil
}

func runModuleSub(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 sub 操作")
	}
	action := args[0]
	flags := newFlagSet("module sub")
	values := bindModuleFlags(flags)
	name := flags.String("name", "", "订阅名称")
	urlValue := flags.String("url", "", "订阅 URL")
	userAgent := flags.String("user-agent", "", "订阅 User-Agent")
	hwid := flags.String("hwid", "", "订阅 HWID")
	headersFile := flags.String("headers-file", "", "请求头 JSON 文件")
	interval := flags.String("interval", "24h", "更新周期")
	autoUpdate := flags.Bool("auto-update", true, "自动更新")
	viaProxy := flags.String("via-proxy", "auto", "更新代理模式")
	include := flags.String("include", "", "节点包含表达式")
	exclude := flags.String("exclude", "", "节点排除表达式")
	allowInsecure := flags.Bool("allow-insecure", false, "跳过 TLS 校验")
	timeout := flags.Int64("download-timeout", 60, "下载超时秒数")
	private := flags.Bool("private", false, "返回订阅私有设置")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := values.options()
	positionals := flags.Args()
	if action == "list" {
		groups, err := catalog.Scan(ctx, catalog.ScanOptions{Root: options.CatalogRoot, Type: "subscription", ActiveGroup: readActiveGroup(options), ProgressDir: options.ProgressDir, WithNodes: false})
		if err != nil {
			return err
		}
		data := make([]catalog.GroupSummary, 0, len(groups))
		for _, group := range groups {
			data = append(data, group.Group)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.list", Message: "订阅列表", Data: data})
		return nil
	}
	if action == "add" {
		if len(positionals) > 0 && *urlValue == "" {
			*urlValue = positionals[len(positionals)-1]
			if len(positionals) > 1 && *name == "" {
				*name = positionals[0]
			}
		}
		if *urlValue == "" {
			return errors.New("sub add 需要 URL")
		}
		seconds, err := subscription.DurationToSeconds(*interval)
		if err != nil {
			return err
		}
		headers := map[string]string{}
		if *headersFile != "" {
			content, readErr := os.ReadFile(*headersFile)
			if readErr != nil {
				return readErr
			}
			if err := json.Unmarshal(content, &headers); err != nil {
				return err
			}
		}
		updated, err := moduleapp.AddSubscription(ctx, moduleapp.SubscriptionOptions{Options: options, Name: *name, URL: *urlValue, UserAgent: *userAgent, HWID: *hwid, Headers: headers, AutoUpdate: *autoUpdate, UpdateInterval: seconds, IntervalSource: "user", UpdateViaProxy: *viaProxy, Include: *include, Exclude: *exclude, AllowInsecure: *allowInsecure, Timeout: *timeout})
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.added", Message: "订阅已添加", Data: updated})
		return nil
	}
	if action == "update" {
		if len(positionals) == 0 {
			return errors.New("sub update 需要订阅")
		}
		updated, err := moduleapp.UpdateSubscription(ctx, options, positionals[0])
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.updated", Message: "订阅更新完成", Data: updated})
		return nil
	}
	if action == "edit" {
		if len(positionals) == 0 {
			return errors.New("sub edit 需要订阅")
		}
		groupID, err := catalog.ResolveGroup(options.CatalogRoot, positionals[0])
		if err != nil {
			return err
		}
		var headers *map[string]string
		if *headersFile != "" {
			content, readErr := os.ReadFile(*headersFile)
			if readErr != nil {
				return readErr
			}
			value := map[string]string{}
			if err := json.Unmarshal(content, &value); err != nil {
				return err
			}
			headers = &value
		}
		var intervalSeconds *int64
		if flagWasSetNative(flags, "interval") {
			value, err := subscription.DurationToSeconds(*interval)
			if err != nil {
				return err
			}
			intervalSeconds = &value
		}
		edit := subscription.EditOptions{Root: options.CatalogRoot, GroupID: groupID, ProgressDir: options.ProgressDir, Now: time.Now(), CustomHeaders: headers}
		if flagWasSetNative(flags, "name") {
			edit.Name = name
		}
		if flagWasSetNative(flags, "url") {
			edit.URL = urlValue
		}
		if flagWasSetNative(flags, "user-agent") {
			edit.UserAgent = userAgent
		}
		if flagWasSetNative(flags, "hwid") {
			edit.HWID = hwid
		}
		if flagWasSetNative(flags, "auto-update") {
			edit.AutoUpdate = autoUpdate
		}
		if intervalSeconds != nil {
			edit.UpdateInterval = intervalSeconds
		}
		if flagWasSetNative(flags, "via-proxy") {
			edit.UpdateViaProxy = viaProxy
		}
		if flagWasSetNative(flags, "include") {
			edit.Include = include
		}
		if flagWasSetNative(flags, "exclude") {
			edit.Exclude = exclude
		}
		if flagWasSetNative(flags, "allow-insecure") {
			edit.AllowInsecure = allowInsecure
		}
		if flagWasSetNative(flags, "download-timeout") {
			edit.Timeout = timeout
		}
		edited, err := subscription.Edit(ctx, edit)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.edited", Message: "订阅设置已更新", Data: edited})
		return nil
	}
	if action == "update-all" {
		summary, err := moduleapp.UpdateAllSubscriptions(ctx, options)
		if err != nil {
			return err
		}
		if len(summary.Failed) > 0 {
			return fmt.Errorf("部分订阅更新失败")
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.updated_all", Message: "全部订阅更新完成", Data: summary})
		return nil
	}
	if action == "activate" {
		if len(positionals) == 0 {
			return errors.New("sub activate 需要订阅")
		}
		data, err := moduleapp.SelectNode(ctx, options, "auto", positionals[0])
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.activated", Message: "活动订阅已切换", Data: data})
		return nil
	}
	if action == "remove" {
		if len(positionals) == 0 {
			return errors.New("sub remove 需要订阅")
		}
		replacement := ""
		if len(positionals) > 1 {
			replacement = positionals[1]
		}
		if err := moduleapp.RemoveSubscription(ctx, options, positionals[0], replacement); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.removed", Message: "订阅已删除", Data: map[string]string{"id": positionals[0]}})
		return nil
	}
	if action == "cancel" {
		if len(positionals) == 0 {
			return errors.New("sub cancel 需要订阅")
		}
		id, err := catalog.ResolveGroup(options.CatalogRoot, positionals[0])
		if err != nil {
			return err
		}
		if err := os.MkdirAll(options.ProgressDir, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(options.ProgressDir, id+".cancel"), []byte("1\n"), 0o600); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.cancelled", Message: "已请求取消订阅更新", Data: map[string]string{"id": id}})
		return nil
	}
	if action == "show" || action == "history" {
		if len(positionals) == 0 {
			return errors.New("sub 操作需要订阅")
		}
		id, err := catalog.ResolveGroup(options.CatalogRoot, positionals[0])
		if err != nil {
			return err
		}
		if action == "history" {
			data, err := subscription.LoadHistory(filepath.Join(options.CatalogRoot, id, "history.jsonl"))
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.history", Message: "订阅更新历史", Data: data})
			return nil
		}
		if *private || (len(positionals) > 1 && positionals[1] == "--private") {
			data, err := catalog.PrivateMetadata(options.CatalogRoot, id)
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.show", Message: "订阅详情", Data: data})
			return nil
		}
		groups, err := catalog.Scan(ctx, catalog.ScanOptions{Root: options.CatalogRoot, GroupID: id, ProgressDir: options.ProgressDir, WithNodes: true})
		if err != nil || len(groups) == 0 {
			if err != nil {
				return err
			}
			return errors.New("订阅不存在")
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.show", Message: "订阅详情", Data: groups[0]})
		return nil
	}
	return fmt.Errorf("未知 sub 操作 %q", action)
}

func runModuleConfig(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 config 操作")
	}
	flags := newFlagSet("module config")
	values := bindModuleFlags(flags)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := values.options()
	action := args[0]
	positionals := flags.Args()
	switch action {
	case "list":
		data, err := moduleapp.ListConfigs(options)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.list", Message: "sing-box 配置列表", Data: data})
	case "read":
		if len(positionals) == 0 {
			return errors.New("config read 需要目标")
		}
		data, err := moduleapp.ReadConfig(options, positionals[0])
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.read", Message: "配置内容", Data: data})
	case "check":
		_, err := moduleapp.Check(ctx, options, true)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.checked", Message: "sing-box 配置检查通过", Data: map[string]any{}})
	case "validate", "apply":
		if len(positionals) < 2 {
			return errors.New("config 操作需要目标和内容文件")
		}
		err := moduleapp.ApplyConfig(ctx, options, positionals[0], positionals[1], action == "validate")
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config." + action, Message: "配置检查通过", Data: map[string]string{"target": positionals[0]}})
	default:
		return fmt.Errorf("未知 config 操作 %q", action)
	}
	return nil
}

func runModuleLogs(_ context.Context, args []string) error {
	flags := newFlagSet("module logs")
	values := bindModuleFlags(flags)
	lines := flags.Int("lines", 200, "显示行数")
	output := flags.String("output", "/sdcard/Download/netproxy-diagnostics.tar.gz", "诊断包路径")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := values.options()
	action := args[0]
	positionals := flags.Args()
	kind := "service"
	if len(positionals) > 0 {
		kind = positionals[0]
	}
	switch action {
	case "show":
		content, err := moduleapp.ShowLog(options, kind, *lines)
		if err != nil {
			return err
		}
		if *format == "text" {
			fmt.Fprint(os.Stdout, content)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "logs.show", Message: "日志内容", Data: map[string]string{"kind": kind, "content": content}})
	case "clear":
		if err := moduleapp.ClearLog(options, kind); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "logs.cleared", Message: "日志已清空", Data: map[string]string{"kind": kind}})
	case "export":
		if len(positionals) > 0 {
			*output = positionals[0]
		}
		if err := moduleapp.ExportLogs(options, *output); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "logs.exported", Message: "诊断包已导出", Data: map[string]string{"path": *output}})
	default:
		return fmt.Errorf("未知 logs 操作 %q", action)
	}
	return nil
}

func runModuleService(ctx context.Context, args []string) error {
	flags := newFlagSet("module service")
	values := bindModuleFlags(flags)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	positionals := flags.Args()
	if len(positionals) == 0 {
		return errors.New("service 需要操作")
	}
	// 生命周期仍由受限 Shell 适配器执行，Go 不复制 setuidgid/nohup/PID 回收逻辑。
	options := values.options()
	shell := "/system/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		shell = "sh"
	}
	command := execCommand(ctx, shell, options.ServiceScript, positionals[0])
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "service." + positionals[0], Message: "服务操作完成", Data: map[string]string{"action": positionals[0]}})
	return nil
}

func readActiveGroup(options moduleapp.Options) string {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return ""
	}
	return module.ActiveGroupID
}

func flagWasSetNative(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == name {
			found = true
		}
	})
	return found
}

// execCommand 单独封装便于在 Windows 单元测试中替换平台命令。
var execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
