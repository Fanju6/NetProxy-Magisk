package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/control"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/ebpf"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/moduleconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/sharelink"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subworker"
)

var (
	version = "development"
	commit  = "unknown"
)

type result struct {
	Schema  int    `json:"schema"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type resultError struct {
	Code    string
	Message string
	Data    any
}

func (e *resultError) Error() string {
	return e.Message
}

type convertOptions struct {
	output          string
	metadataOutput  string
	diagnosticsFile string
	allowInsecure   bool
	include         string
	exclude         string
}

type serviceSnapshot struct {
	Memory           uint64 `json:"memory"`
	Goroutines       int32  `json:"goroutines"`
	ConnectionsIn    int32  `json:"connections_in"`
	ConnectionsOut   int32  `json:"connections_out"`
	TrafficAvailable bool   `json:"traffic_available"`
	Uplink           int64  `json:"uplink"`
	Downlink         int64  `json:"downlink"`
	UplinkTotal      int64  `json:"uplink_total"`
	DownlinkTotal    int64  `json:"downlink_total"`
	Selected         string `json:"selected"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		var structured *resultError
		if errors.As(err, &structured) {
			writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: structured.Code, Message: structured.Message, Data: structured.Data})
		} else {
			writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: "command.failed", Message: err.Error()})
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		showUsage()
		return nil
	}
	switch args[0] {
	case "convert":
		return runConvert(ctx, args[1:])
	case "provider":
		return runProvider(ctx, args[1:])
	case "catalog":
		return runCatalog(ctx, args[1:])
	case "subscription":
		return runSubscription(ctx, args[1:])
	case "service":
		return runService(ctx, args[1:])
	case "control":
		return runControl(ctx, args[1:])
	case "ebpf":
		return runEBPF(ctx, args[1:])
	case "config":
		return runConfig(ctx, args[1:])
	case "module":
		return runModule(ctx, args[1:])
	case "subworker":
		return runSubworker(ctx, args[1:])
	case "sub":
		if len(args) > 1 && args[1] == "worker" {
			return runSubworker(ctx, args[2:])
		}
		return fmt.Errorf("未知 sub 操作")
	case "version":
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "version", Message: "版本信息", Data: map[string]string{
			"netproxy_native": version,
			"commit":          commit,
			"sing_box":        dependencyVersion("github.com/sagernet/sing-box"),
		}})
		return nil
	default:
		return fmt.Errorf("未知命令 %q", args[0])
	}
}

func runConfig(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少配置操作: module-get|module-set|ebpf-get|ebpf-set")
	}
	action := args[0]
	flags := newFlagSet("config " + action)
	path := flags.String("path", "", "配置文件路径")
	key := flags.String("key", "", "读取的配置键")
	format := flags.String("format", "json", "输出格式")
	assignments := make([]string, 0)
	flags.Func("set", "设置 KEY=value，可重复使用", func(value string) error {
		assignments = append(assignments, value)
		return nil
	})
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" {
		return errors.New("配置操作需要 --path")
	}

	switch action {
	case "module-get":
		config, err := moduleconfig.LoadModule(*path)
		if err != nil {
			return &resultError{Code: "config.invalid", Message: "module.conf 配置无效", Data: map[string]string{"error": err.Error()}}
		}
		if *key != "" {
			value, err := moduleConfigValue(config, *key)
			if err != nil {
				return err
			}
			switch *format {
			case "text":
				fmt.Fprintln(os.Stdout, value)
			case "tsv":
				fmt.Fprintf(os.Stdout, "%s\t%s\n", *key, value)
			case "json":
				writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.module_read", Message: "模块配置已读取", Data: map[string]string{*key: value}})
			default:
				return fmt.Errorf("不支持的配置输出格式: %s", *format)
			}
			return nil
		}
		if *format == "tsv" {
			writeModuleConfigTSV(config)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("读取完整 module.conf 只支持 json 或 tsv")
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.module_read", Message: "模块配置已读取", Data: config})
		return nil
	case "module-set":
		updates, err := parseAssignments(assignments)
		if err != nil {
			return err
		}
		if err := moduleconfig.UpdateModule(*path, updates); err != nil {
			return &resultError{Code: "config.invalid", Message: "module.conf 修改未通过校验", Data: map[string]string{"error": err.Error()}}
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.module_updated", Message: "模块配置已更新", Data: map[string]string{"path": *path}})
		return nil
	case "ebpf-get":
		config, err := ebpf.Load(*path)
		if err != nil {
			return &resultError{Code: "config.invalid", Message: "ebpf.conf 配置无效", Data: map[string]string{"error": err.Error()}}
		}
		if *key == "" {
			if *format != "tsv" {
				return fmt.Errorf("读取完整 ebpf.conf 只支持 tsv")
			}
			writeEBPFConfigTSV(config)
			return nil
		}
		value, err := ebpfConfigValue(config, *key)
		if err != nil {
			return err
		}
		switch *format {
		case "text":
			fmt.Fprintln(os.Stdout, value)
		case "tsv":
			fmt.Fprintf(os.Stdout, "%s\t%s\n", *key, value)
		case "json":
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.ebpf_read", Message: "eBPF 配置已读取", Data: map[string]string{*key: value}})
		default:
			return fmt.Errorf("不支持的配置输出格式: %s", *format)
		}
		return nil
	case "ebpf-set":
		updates, err := parseAssignments(assignments)
		if err != nil {
			return err
		}
		err = moduleconfig.UpdateValidated(*path, updates, func(candidate string) error {
			_, validateErr := ebpf.Load(candidate)
			return validateErr
		})
		if err != nil {
			return &resultError{Code: "config.invalid", Message: "ebpf.conf 修改未通过校验", Data: map[string]string{"error": err.Error()}}
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.ebpf_updated", Message: "eBPF 配置已更新", Data: map[string]string{"path": *path}})
		return nil
	default:
		return fmt.Errorf("未知配置操作 %q", action)
	}
}

func parseAssignments(values []string) (map[string]string, error) {
	updates := make(map[string]string, len(values))
	for _, assignment := range values {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("配置修改必须使用 KEY=value: %s", assignment)
		}
		if _, exists := updates[key]; exists {
			return nil, fmt.Errorf("配置键重复: %s", key)
		}
		updates[key] = value
	}
	if len(updates) == 0 {
		return nil, errors.New("配置修改至少需要一个 --set KEY=value")
	}
	return updates, nil
}

func moduleConfigValue(config moduleconfig.ModuleConfig, key string) (string, error) {
	switch key {
	case "AUTO_START":
		return boolString(config.AutoStart), nil
	case "OUTBOUND_MODE":
		return config.OutboundMode, nil
	case "SELECTOR_MODE":
		return config.SelectorMode, nil
	case "ACTIVE_GROUP_ID":
		return config.ActiveGroupID, nil
	case "SELECTED_NODE_REF":
		return config.SelectedNodeRef, nil
	case "WIFI_AUTO_SWITCH":
		return boolString(config.WiFiAutoSwitch), nil
	case "WIFI_SSID_MODE":
		return config.WiFiSSIDMode, nil
	case "WIFI_SSID_LIST":
		return config.WiFiSSIDList, nil
	case "PROXY_ON_CELLULAR":
		return boolString(config.ProxyOnCellular), nil
	default:
		return "", fmt.Errorf("不支持的 module.conf 配置键: %s", key)
	}
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func writeModuleConfigTSV(config moduleconfig.ModuleConfig) {
	keys := []string{"AUTO_START", "OUTBOUND_MODE", "SELECTOR_MODE", "ACTIVE_GROUP_ID", "SELECTED_NODE_REF", "WIFI_AUTO_SWITCH", "WIFI_SSID_MODE", "WIFI_SSID_LIST", "PROXY_ON_CELLULAR"}
	for _, key := range keys {
		value, _ := moduleConfigValue(config, key)
		fmt.Fprintf(os.Stdout, "%s\t%s\n", key, value)
	}
}

func ebpfConfigValue(config ebpf.Config, key string) (string, error) {
	switch key {
	case "EBPF_NETWORK":
		return config.Network, nil
	case "EBPF_UDP_TIMEOUT":
		return config.UDPTimeout, nil
	case "EBPF_DNS_MODE":
		return config.DNSMode, nil
	case "EBPF_CGROUP_ENABLED":
		return boolString(config.CgroupEnabled), nil
	case "EBPF_CGROUP_PATH":
		return config.CgroupPath, nil
	case "EBPF_IPV6_MODE":
		return config.IPv6Mode, nil
	case "EBPF_BYPASS_RULE_SETS":
		return strings.Join(config.BypassRuleSets, " "), nil
	case "APP_PROXY_ENABLE":
		return boolString(config.AppProxyEnable), nil
	case "APP_PROXY_MODE":
		return config.AppProxyMode, nil
	case "APP_ANDROID_USERS":
		return joinUintValues(config.AndroidUsers), nil
	case "PROXY_APPS_LIST":
		return strings.Join(config.ProxyPackages, " "), nil
	case "BYPASS_APPS_LIST":
		return strings.Join(config.BypassPackages, " "), nil
	case "EBPF_SHARED_NETWORK":
		return boolString(config.SharedNetwork), nil
	case "EBPF_SHARED_INTERFACES":
		return strings.Join(config.SharedInterfaces, " "), nil
	case "EBPF_SHARED_INCLUDE_SOURCE_CIDRS":
		return strings.Join(config.SharedIncludeSourceCIDRs, " "), nil
	case "EBPF_SHARED_EXCLUDE_SOURCE_CIDRS":
		return strings.Join(config.SharedExcludeSourceCIDRs, " "), nil
	case "EBPF_SHARED_INCLUDE_MAC_ADDRESSES":
		return strings.Join(config.SharedIncludeMACAddresses, " "), nil
	case "EBPF_SHARED_EXCLUDE_MAC_ADDRESSES":
		return strings.Join(config.SharedExcludeMACAddresses, " "), nil
	case "EBPF_TCP_MAP_CAPACITY":
		return strconv.FormatUint(config.TCPMapCapacity, 10), nil
	case "EBPF_UDP_MAP_CAPACITY":
		return strconv.FormatUint(config.UDPMapCapacity, 10), nil
	case "EBPF_SOCKET_MAP_CAPACITY":
		return strconv.FormatUint(config.SocketMapCapacity, 10), nil
	case "EBPF_SHARED_MAP_CAPACITY":
		return strconv.FormatUint(config.SharedMapCapacity, 10), nil
	default:
		return "", fmt.Errorf("不支持的 ebpf.conf 配置键: %s", key)
	}
}

func joinUintValues(values []uint64) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, strconv.FormatUint(value, 10))
	}
	return strings.Join(items, " ")
}

func writeEBPFConfigTSV(config ebpf.Config) {
	keys := []string{
		"EBPF_NETWORK", "EBPF_UDP_TIMEOUT", "EBPF_DNS_MODE", "EBPF_CGROUP_ENABLED",
		"EBPF_CGROUP_PATH", "EBPF_IPV6_MODE", "EBPF_BYPASS_RULE_SETS", "APP_PROXY_ENABLE",
		"APP_PROXY_MODE", "APP_ANDROID_USERS", "PROXY_APPS_LIST", "BYPASS_APPS_LIST",
		"EBPF_SHARED_NETWORK", "EBPF_SHARED_INTERFACES", "EBPF_SHARED_INCLUDE_SOURCE_CIDRS",
		"EBPF_SHARED_EXCLUDE_SOURCE_CIDRS", "EBPF_SHARED_INCLUDE_MAC_ADDRESSES",
		"EBPF_SHARED_EXCLUDE_MAC_ADDRESSES", "EBPF_TCP_MAP_CAPACITY", "EBPF_UDP_MAP_CAPACITY",
		"EBPF_SOCKET_MAP_CAPACITY", "EBPF_SHARED_MAP_CAPACITY",
	}
	for _, key := range keys {
		value, _ := ebpfConfigValue(config, key)
		fmt.Fprintf(os.Stdout, "%s\t%s\n", key, value)
	}
}

func runEBPF(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 eBPF 操作: runtime|validate|diagnose")
	}
	action := args[0]
	flags := newFlagSet("ebpf " + action)
	configPath := flags.String("config", "", "ebpf.conf 路径")
	outputPath := flags.String("output", "", "运行时 JSON 输出路径")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*configPath) == "" {
		return errors.New("eBPF 操作需要 --config")
	}
	toError := func(err error) error {
		var validation *ebpf.ValidationError
		if errors.As(err, &validation) {
			return &resultError{Code: "ebpf.config_invalid", Message: validation.Error(), Data: map[string]any{"diagnostics": validation.Diagnostics}}
		}
		return err
	}
	switch action {
	case "runtime":
		if strings.TrimSpace(*outputPath) == "" {
			return errors.New("eBPF runtime 需要 --output")
		}
		config, err := ebpf.Load(*configPath)
		if err != nil {
			return toError(err)
		}
		if err := ebpf.WriteAtomic(*outputPath, config); err != nil {
			return toError(err)
		}
		if *format == "text" {
			fmt.Fprintln(os.Stdout, *outputPath)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("ebpf runtime 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "ebpf.runtime_generated", Message: "eBPF 运行时配置已生成", Data: map[string]string{"path": *outputPath}})
		return nil
	case "validate", "diagnose":
		report := ebpf.Diagnose(*configPath)
		if *format == "text" {
			fmt.Fprintf(os.Stdout, "配置检查: %s\n", map[bool]string{true: "通过", false: "失败"}[report.Valid])
			for _, diagnostic := range report.Diagnostics {
				if diagnostic.Field == "" {
					fmt.Fprintf(os.Stdout, "[%s] %s\n", diagnostic.Level, diagnostic.Message)
				} else {
					fmt.Fprintf(os.Stdout, "[%s] %s: %s\n", diagnostic.Field, diagnostic.Code, diagnostic.Message)
				}
			}
			if !report.Valid {
				return &resultError{Code: "ebpf.config_invalid", Message: "eBPF 配置检查未通过", Data: report}
			}
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("ebpf %s 不支持输出格式 %q", action, *format)
		}
		if !report.Valid {
			return &resultError{Code: "ebpf.config_invalid", Message: "eBPF 配置检查未通过", Data: report}
		}
		code := "ebpf.config_valid"
		message := "eBPF 配置有效"
		if action == "diagnose" {
			code = "ebpf.diagnosed"
			message = "eBPF 配置诊断完成"
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: report})
		return nil
	default:
		return fmt.Errorf("未知 eBPF 操作 %q", action)
	}
}

func runControl(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少控制面操作: status|nodes|snapshot|selection|groups|mode|delay|close-all")
	}
	action := args[0]
	flags := newFlagSet("control " + action)
	catalogRoot := flags.String("catalog-root", "", "Catalog 根目录")
	moduleConfig := flags.String("module-config", "", "模块配置文件")
	stateFile := flags.String("state-file", "", "服务状态文件")
	progressDir := flags.String("progress-dir", "/dev/netproxy/subscriptions", "订阅进度目录")
	workerPIDFile := flags.String("worker-pid-file", "/dev/netproxy/subworker.pid", "订阅 Worker PID 文件")
	singBox := flags.String("sing-box", "", "sing-box 二进制路径")
	address := flags.String("address", "127.0.0.1:9090", "Service API 地址")
	secret := flags.String("secret", "singbox", "Service API 密钥")
	timeout := flags.Duration("timeout", 8*time.Second, "Service API 请求超时")
	target := flags.String("target", "", "测速目标")
	group := flags.String("group", "", "测速分组")
	mode := flags.String("mode", "", "出站模式")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := control.Options{
		CatalogRoot: *catalogRoot, ModuleConfig: *moduleConfig, StateFile: *stateFile,
		ProgressDir: *progressDir, WorkerPIDFile: *workerPIDFile, SingBoxPath: *singBox,
		ServiceAddress: *address, ServiceSecret: *secret, RequestTimeout: *timeout,
	}
	switch action {
	case "status":
		status, err := control.ReadStatus(ctx, options)
		if err != nil {
			return err
		}
		if *format == "text" {
			fmt.Fprintf(os.Stdout, "服务状态: %s\n运行时间: %d 秒\n出站模式: %s\n活动分组: %s\n节点选择: %s\n订阅更新: %s\n",
				status.State, status.UptimeSeconds, status.OutboundMode, status.ActiveGroupName,
				status.RuntimeSelected, status.SubscriptionWorker)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("control status 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "service.status", Message: "服务状态", Data: status})
		return nil
	case "groups":
		groups, err := control.ReadGroups(ctx, options)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control groups 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "service.groups", Message: "节点组状态", Data: groups})
		return nil
	case "nodes":
		groups, err := control.ReadNodes(ctx, options, *group)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control nodes 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.list", Message: "节点列表", Data: groups})
		return nil
	case "snapshot":
		snapshot, err := control.ReadSnapshot(ctx, options, *group)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control snapshot 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.snapshot", Message: "节点快照", Data: snapshot})
		return nil
	case "selection":
		selection, err := control.ReadSelection(ctx, options)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control selection 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.current", Message: "当前节点选择", Data: selection})
		return nil
	case "mode":
		state, err := control.ReadMode(ctx, options)
		if err != nil {
			return err
		}
		if *format == "text" {
			fmt.Fprintln(os.Stdout, state.Mode)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("control mode 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.current", Message: "当前出站模式", Data: state})
		return nil
	case "runtime-mode":
		runtimeMode, err := control.ReadRuntimeMode(ctx, options)
		if err != nil {
			return err
		}
		if *format == "text" {
			fmt.Fprintln(os.Stdout, runtimeMode)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("control runtime-mode 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.runtime", Message: "运行时出站模式", Data: map[string]string{"mode": runtimeMode}})
		return nil
	case "set-mode":
		if *mode == "" {
			return errors.New("control set-mode 需要 --mode")
		}
		if err := control.SetMode(ctx, options, *mode); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.changed", Message: "运行时出站模式已切换", Data: map[string]string{"mode": *mode}})
		return nil
	case "close-all":
		if err := control.CloseAllConnections(ctx, options); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "connection.closed_all", Message: "已关闭全部连接", Data: map[string]bool{"closed": true}})
		return nil
	case "delay":
		delay, err := control.Delay(ctx, options, *target, *group)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control delay 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.delay", Message: "节点测速完成", Data: delay})
		return nil
	default:
		return fmt.Errorf("未知控制面操作 %q", action)
	}
}

func runSubworker(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Worker 操作: start|stop|restart|wake|status|once|run")
	}
	action := args[0]
	flags := newFlagSet("subworker " + action)
	root := flags.String("root", "", "Catalog 根目录")
	progressDir := flags.String("progress-dir", "/dev/netproxy/subscriptions", "订阅进度目录")
	pidFile := flags.String("pid-file", "/dev/netproxy/subworker.pid", "Worker PID 文件")
	logFile := flags.String("log-file", "", "Worker 日志文件")
	moduleConf := flags.String("module-conf", "", "模块配置文件")
	reloadScript := flags.String("reload-script", "", "服务 reload 适配脚本")
	singBox := flags.String("sing-box", "", "sing-box 二进制路径")
	serviceAddress := flags.String("service-address", "127.0.0.1:9090", "Service API 地址")
	serviceSecret := flags.String("service-secret", "singbox", "Service API 密钥")
	group := flags.String("group", "", "指定单个订阅分组")
	now := flags.Int64("now", time.Now().Unix(), "当前 Unix 时间")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" {
		return errors.New("subworker 需要 --root")
	}
	options := subworker.NewOptions(*root)
	options.ProgressDir = *progressDir
	options.PIDFile = *pidFile
	options.LogFile = *logFile
	options.ModuleConf = *moduleConf
	options.ReloadScript = *reloadScript
	options.SingBoxPath = *singBox
	options.ServiceAddress = *serviceAddress
	options.ServiceSecret = *serviceSecret
	if options.ModuleConf == "" {
		return errors.New("subworker 需要 --module-conf")
	}
	if options.LogFile == "" {
		options.LogFile = filepath.Join(filepath.Dir(*root), "..", "logs", "subscription.log")
	}
	if options.ReloadScript == "" {
		options.ReloadScript = filepath.Join(filepath.Dir(*root), "..", "scripts", "core", "service.sh")
	}
	switch action {
	case "run":
		logger, closer, err := subworker.OpenLogger(options.LogFile)
		if err != nil {
			return err
		}
		defer closer.Close()
		return subworker.RunProcess(ctx, options, logger)
	case "start":
		status, err := subworker.Start(ctx, options, os.Args[0])
		if err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.started", "订阅 Worker 已启动", status)
	case "stop":
		if err := subworker.Stop(options); err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.stopped", "订阅 Worker 已停止", subworker.Status{State: "stopped"})
	case "restart":
		if err := subworker.Stop(options); err != nil {
			return err
		}
		status, err := subworker.Start(ctx, options, os.Args[0])
		if err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.restarted", "订阅 Worker 已重启", status)
	case "wake":
		status, err := subworker.Wake(ctx, options, os.Args[0])
		if err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.woken", "订阅 Worker 已唤醒", status)
	case "status":
		status, err := subworker.ReadStatus(options)
		if err != nil {
			return err
		}
		if *format == "tsv" {
			fmt.Printf("state\t%s\npid\t%d\nnearest\t%d\n", status.State, status.PID, status.Nearest)
			return nil
		}
		return writeWorkerResult(*format, "subworker.status", "订阅 Worker 状态", status)
	case "once":
		logger, closer, err := subworker.OpenLogger(options.LogFile)
		if err != nil {
			return err
		}
		defer closer.Close()
		currentTime := time.Unix(*now, 0)
		if *group != "" {
			updated, updateErr := subworker.UpdateGroup(ctx, options, *group, currentTime, logger)
			if updateErr != nil {
				return updateErr
			}
			if *format == "kv" {
				fmt.Printf("group_id=%s\nnode_count=%d\nrevision=%d\nnot_modified=%t\nstructure_changed=%t\n", updated.GroupID, updated.NodeCount, updated.Revision, updated.NotModified, updated.StructureChanged)
				return nil
			}
			return writeWorkerResult(*format, "subworker.once", "订阅更新完成", updated)
		}
		summary, err := subworker.RunDue(ctx, options, currentTime, logger)
		if err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.once", "订阅到期任务已处理", summary)
	default:
		return fmt.Errorf("未知 Worker 操作 %q", action)
	}
}

func writeWorkerResult(format, code, message string, data any) error {
	if format == "tsv" {
		encoded, err := json.Marshal(data)
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	}
	if format != "json" {
		return fmt.Errorf("Worker 不支持输出格式 %q", format)
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: data})
	return nil
}

func runSubscription(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少订阅操作: update")
	}
	if args[0] == "edit" {
		return runSubscriptionEdit(ctx, args[1:])
	}
	if args[0] != "update" {
		return fmt.Errorf("未知订阅操作 %q", args[0])
	}
	flags := newFlagSet("subscription update")
	root := flags.String("root", "", "Catalog 根目录")
	groupID := flags.String("group", "", "订阅分组 ID")
	progressDir := flags.String("progress-dir", "", "订阅进度目录")
	proxyURL := flags.String("proxy", "", "通过 HTTP 代理下载")
	fallbackDirect := flags.Bool("fallback-direct", false, "代理失败后回退直连")
	now := flags.Int64("now", time.Now().Unix(), "当前 Unix 时间")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" || strings.TrimSpace(*groupID) == "" {
		return errors.New("subscription update 需要 --root 和 --group")
	}
	updated, err := subscription.Update(ctx, subscription.UpdateOptions{
		Root:               *root,
		GroupID:            *groupID,
		ProgressDir:        *progressDir,
		ProxyURL:           *proxyURL,
		UseConfiguredProxy: true,
		FallbackDirect:     *fallbackDirect,
		Now:                time.Unix(*now, 0),
	})
	if err != nil {
		var structured *subscription.Error
		if errors.As(err, &structured) {
			return &resultError{Code: structured.Code, Message: structured.Message, Data: structured.Data}
		}
		return err
	}
	code := "subscription.updated"
	message := "订阅更新完成"
	if updated.NotModified {
		if *format == "kv" {
			fmt.Printf("group_id=%s\nnode_count=%d\nrevision=%d\nnot_modified=true\nstructure_changed=false\n", updated.GroupID, updated.NodeCount, updated.Revision)
			return nil
		}
		code = "subscription.not_modified"
		message = "订阅未发生变化"
	}
	if *format == "kv" {
		fmt.Printf("group_id=%s\nnode_count=%d\nrevision=%d\nnot_modified=%t\nstructure_changed=%t\n", updated.GroupID, updated.NodeCount, updated.Revision, updated.NotModified, updated.StructureChanged)
		return nil
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: updated})
	return nil
}

func runSubscriptionEdit(ctx context.Context, args []string) error {
	flags := newFlagSet("subscription edit")
	root := flags.String("root", "", "Catalog 根目录")
	groupID := flags.String("group", "", "订阅分组 ID")
	progressDir := flags.String("progress-dir", "", "订阅更新进度目录")
	proxyURL := flags.String("proxy", "", "通过 HTTP 代理下载")
	fallbackDirect := flags.Bool("fallback-direct", false, "代理失败后回退直连")
	name := flags.String("name", "", "订阅名称")
	subscriptionURL := flags.String("url", "", "订阅地址")
	userAgent := flags.String("user-agent", "", "订阅 User-Agent")
	hwid := flags.String("hwid", "", "订阅 HWID")
	headersFile := flags.String("headers-file", "", "自定义请求头 JSON 文件")
	autoUpdate := flags.Bool("auto-update", false, "自动更新开关")
	interval := flags.String("interval", "", "更新周期")
	updateViaProxy := flags.String("via-proxy", "", "订阅更新代理模式")
	include := flags.String("include", "", "节点包含表达式")
	exclude := flags.String("exclude", "", "节点排除表达式")
	allowInsecure := flags.Bool("allow-insecure", false, "跳过 TLS 证书校验")
	timeout := flags.Int64("timeout", 0, "下载超时秒数")
	now := flags.Int64("now", time.Now().Unix(), "当前 Unix 时间")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" || strings.TrimSpace(*groupID) == "" {
		return errors.New("subscription edit 需要 --root 和 --group")
	}
	var headers *map[string]string
	if flagWasSet(flags, "headers-file") {
		value, err := readHeadersFile(*headersFile)
		if err != nil {
			return err
		}
		headers = &value
	}
	var intervalSeconds *int64
	if flagWasSet(flags, "interval") {
		value, err := subscription.DurationToSeconds(*interval)
		if err != nil {
			return err
		}
		intervalSeconds = &value
	}
	options := subscription.EditOptions{
		Root: *root, GroupID: *groupID, ProgressDir: *progressDir, ProxyURL: *proxyURL,
		FallbackDirect: *fallbackDirect, Now: time.Unix(*now, 0), CustomHeaders: headers,
	}
	if flagWasSet(flags, "name") {
		options.Name = name
	}
	if flagWasSet(flags, "url") {
		options.URL = subscriptionURL
	}
	if flagWasSet(flags, "user-agent") {
		options.UserAgent = userAgent
	}
	if flagWasSet(flags, "hwid") {
		options.HWID = hwid
	}
	if flagWasSet(flags, "auto-update") {
		options.AutoUpdate = autoUpdate
	}
	options.UpdateInterval = intervalSeconds
	if flagWasSet(flags, "via-proxy") {
		options.UpdateViaProxy = updateViaProxy
	}
	if flagWasSet(flags, "include") {
		options.Include = include
	}
	if flagWasSet(flags, "exclude") {
		options.Exclude = exclude
	}
	if flagWasSet(flags, "allow-insecure") {
		options.AllowInsecure = allowInsecure
	}
	if flagWasSet(flags, "timeout") {
		options.Timeout = timeout
	}
	edited, err := subscription.Edit(ctx, options)
	if err != nil {
		var structured *subscription.Error
		if errors.As(err, &structured) {
			return &resultError{Code: structured.Code, Message: structured.Message, Data: structured.Data}
		}
		return err
	}
	if flagWasSet(flags, "format") && *format == "kv" {
		fmt.Printf("group_id=%s\nname_changed=%t\nrequires_update=%t\nnode_count=%d\nrevision=%d\nnot_modified=%t\nstructure_changed=%t\n", edited.GroupID, edited.NameChanged, edited.RequiresUpdate, edited.NodeCount, edited.Revision, edited.NotModified, edited.StructureChanged)
		return nil
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.edited", Message: "订阅设置已更新", Data: edited})
	return nil
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == name {
			set = true
		}
	})
	return set
}

func runCatalog(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Catalog 操作")
	}
	action := args[0]
	flags := newFlagSet("catalog " + action)
	input := flags.String("input", "", "输入路径或内容")
	value := flags.String("value", "", "元数据字段值")
	root := flags.String("root", "", "Catalog 根目录")
	moduleConfig := flags.String("module-config", "", "module.conf 路径")
	groupDir := flags.String("group-dir", "", "Catalog 分组目录")
	active := flags.String("active", "", "活动分组 ID")
	progressDir := flags.String("progress-dir", "", "订阅更新进度目录")
	groupType := flags.String("type", "all", "分组类型筛选")
	kind := flags.String("kind", "subscription", "分组 ID 类型")
	groupID := flags.String("group", "", "指定分组 ID")
	name := flags.String("name", "", "分组显示名称")
	tag := flags.String("tag", "", "节点标签")
	allowInsecure := flags.Bool("allow-insecure", false, "跳过节点 TLS 证书校验")
	subscriptionURL := flags.String("url", "", "订阅地址")
	userAgent := flags.String("user-agent", "", "订阅 User-Agent")
	hwid := flags.String("hwid", "", "订阅 HWID")
	headersFile := flags.String("headers-file", "", "自定义请求头 JSON 文件")
	autoUpdate := flags.Bool("auto-update", true, "是否自动更新")
	updateInterval := flags.Int64("update-interval", 86400, "更新间隔秒数")
	intervalSource := flags.String("interval-source", "default", "更新间隔来源")
	updateViaProxy := flags.String("update-via-proxy", "auto", "订阅更新代理模式")
	include := flags.String("include", "", "节点包含表达式")
	exclude := flags.String("exclude", "", "节点排除表达式")
	timeout := flags.Int64("timeout", 60, "订阅请求超时秒数")
	providersOutput := flags.String("providers-output", "", "运行时 Provider 配置输出")
	outboundsOutput := flags.String("outbounds-output", "", "运行时出站配置输出")
	stateOutput := flags.String("state-output", "", "运行时状态输出")
	selector := flags.String("selector", "urltest", "选择模式")
	selected := flags.String("selected", "", "手动节点引用")
	allowEmpty := flags.Bool("allow-empty", false, "允许空 Catalog")
	now := flags.Int64("now", time.Now().Unix(), "当前 Unix 时间")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if action == "duration" {
		seconds, err := subscription.DurationToSeconds(*value)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Println(seconds)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.duration", Message: "更新周期已解析", Data: map[string]int64{"seconds": seconds}})
		return nil
	}
	if action == "time" {
		if *format == "raw" {
			fmt.Println(subscription.FormatEpochUTC(*now))
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.time", Message: "时间已格式化", Data: map[string]string{"value": subscription.FormatEpochUTC(*now)}})
		return nil
	}
	if action == "schedule-next" {
		interval, err := subscription.DurationToSeconds(*value)
		if err != nil {
			return err
		}
		metadata := subscription.Metadata{UpdateInterval: interval}
		subscription.ScheduleAt(&metadata, time.Unix(*now, 0))
		if *format == "tsv" {
			fmt.Printf("interval\t%d\nnext_update_epoch\t%d\nnext_update_at\t%s\n", metadata.UpdateInterval, metadata.NextUpdateEpoch, metadata.NextUpdateAt)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.schedule_next", Message: "下一次更新时间已计算", Data: map[string]any{"interval": metadata.UpdateInterval, "next_update_epoch": metadata.NextUpdateEpoch, "next_update_at": metadata.NextUpdateAt}})
		return nil
	}
	if action == "new-id" {
		if *root == "" {
			return errors.New("Catalog new-id 需要 --root")
		}
		id, err := catalog.NewGroupID(*root, *kind, *input)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Println(id)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.new_id", Message: "Catalog 分组 ID 已生成", Data: map[string]string{"group_id": id}})
		return nil
	}
	if action == "resolve" {
		if *root == "" || *groupID == "" {
			return errors.New("Catalog resolve 需要 --root 和 --group")
		}
		resolved, err := catalog.ResolveGroup(*root, *groupID)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Println(resolved)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.resolve", Message: "Catalog 分组已解析", Data: map[string]string{"group_id": resolved}})
		return nil
	}
	if action == "group-has-nodes" || action == "group-first-tag" || action == "group-contains-tag" || action == "first-nonempty" || action == "group-type" || action == "node-get" || action == "node-export" || action == "group-private" || action == "group-delete" || action == "history" {
		if action == "first-nonempty" {
			if *root == "" {
				return errors.New("Catalog first-nonempty 需要 --root")
			}
			group, err := catalog.FirstNonEmptyGroup(ctx, *root, *groupID)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(group)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.first_nonempty", Message: "首个非空分组", Data: map[string]string{"group_id": group}})
			return nil
		}
		if *root == "" || *groupID == "" {
			return fmt.Errorf("Catalog %s 需要 --root 和 --group", action)
		}
		switch action {
		case "group-has-nodes":
			hasNodes, err := catalog.GroupHasNodes(ctx, *root, *groupID)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(hasNodes)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_has_nodes", Message: "Catalog 分组节点状态", Data: map[string]bool{"has_nodes": hasNodes}})
		case "group-type":
			groupType, err := catalog.GroupType(*root, *groupID)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(groupType)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_type", Message: "Catalog 分组类型", Data: map[string]string{"type": groupType}})
		case "group-first-tag":
			firstTag, err := catalog.GroupFirstTag(ctx, *root, *groupID)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(firstTag)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_first_tag", Message: "Catalog 分组首个节点", Data: map[string]string{"tag": firstTag}})
		case "group-contains-tag":
			if *tag == "" {
				return errors.New("Catalog group-contains-tag 需要 --tag")
			}
			contains, err := catalog.GroupContainsTag(ctx, *root, *groupID, *tag)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(contains)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_contains_tag", Message: "Catalog 分组节点标签状态", Data: map[string]bool{"contains": contains}})
		case "node-get":
			if *tag == "" {
				return errors.New("Catalog node-get 需要 --tag")
			}
			document, err := catalog.GroupNode(ctx, *root, *groupID, *tag)
			if err != nil {
				return err
			}
			content, err := provider.Marshal(ctx, document)
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.loaded", Message: "节点配置已读取", Data: json.RawMessage(content)})
		case "node-export":
			if *tag == "" {
				return errors.New("Catalog node-export 需要 --tag")
			}
			exported, err := catalog.ExportGroupNode(ctx, *root, *groupID, *tag)
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.exported", Message: "节点分享链接已生成", Data: exported})
		case "group-private":
			metadata, err := catalog.PrivateMetadata(*root, *groupID)
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_private", Message: "Catalog 分组私有信息", Data: metadata})
		case "group-delete":
			if err := catalog.DeleteGroup(*root, *groupID); err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_deleted", Message: "Catalog 分组已删除", Data: map[string]string{"group_id": *groupID}})
		case "history":
			history, err := subscription.LoadHistory(filepath.Join(*root, *groupID, "history.jsonl"))
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.history", Message: "Catalog 分组更新历史", Data: history})
		}
		return nil
	}
	if action == "group-init" || action == "group-ensure" {
		if *root == "" || *groupID == "" {
			return fmt.Errorf("Catalog %s 需要 --root 和 --group", action)
		}
		groupType := *groupType
		if groupType == "all" {
			if action == "group-ensure" {
				groupType = "local"
			} else {
				return errors.New("Catalog group-init 需要 --type")
			}
		}
		headers, err := readHeadersFile(*headersFile)
		if err != nil {
			return err
		}
		options := catalog.GroupOptions{
			Root: *root, GroupID: *groupID, Name: *name, Type: groupType,
			URL: *subscriptionURL, UserAgent: *userAgent, HWID: *hwid,
			CustomHeaders: headers, AutoUpdate: *autoUpdate, UpdateInterval: *updateInterval,
			IntervalSource: *intervalSource, UpdateViaProxy: *updateViaProxy,
			Include: *include, Exclude: *exclude, AllowInsecure: *allowInsecure,
			Timeout: *timeout, Now: time.Unix(*now, 0),
		}
		if action == "group-init" {
			err = catalog.InitializeGroup(ctx, options)
		} else {
			err = catalog.EnsureGroup(ctx, options)
		}
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog." + action, Message: "Catalog 分组已准备", Data: map[string]string{"group_id": *groupID}})
		return nil
	}
	if action == "group-name" {
		if *root == "" || *groupID == "" || strings.TrimSpace(*name) == "" {
			return errors.New("Catalog group-name 需要 --root、--group 和 --name")
		}
		if err := catalog.SetGroupName(ctx, *root, *groupID, *name, time.Unix(*now, 0)); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_name", Message: "Catalog 分组名称已更新", Data: map[string]string{"group_id": *groupID, "name": strings.TrimSpace(*name)}})
		return nil
	}
	if *root == "" && *groupDir == "" {
		return errors.New("Catalog 操作需要 --root")
	}

	switch action {
	case "group-import":
		if *root == "" || *groupID == "" || *input == "" {
			return errors.New("Catalog group-import 需要 --root、--group 和 --input")
		}
		mutation, err := catalog.ImportGroup(ctx, catalog.ImportOptions{
			Root: *root, GroupID: *groupID, Name: *name, Input: *input,
			AllowInsecure: *allowInsecure, Now: time.Unix(*now, 0),
		})
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_imported", Message: "Catalog 分组已导入", Data: mutation})
		return nil
	case "node-append", "node-remove", "node-edit":
		if *groupDir == "" {
			return fmt.Errorf("Catalog %s 需要 --group-dir", action)
		}
		mutationType := *groupType
		if mutationType == "all" {
			mutationType = "local"
		}
		options := catalog.MutationOptions{
			GroupDir: *groupDir, GroupID: *groupID, Name: *name, Type: mutationType,
			Input: *input, Tag: *tag, AllowInsecure: *allowInsecure, Now: time.Unix(*now, 0),
		}
		var mutation catalog.MutationResult
		var err error
		switch action {
		case "node-append":
			mutation, err = catalog.AppendNode(ctx, options)
		case "node-remove":
			mutation, err = catalog.RemoveNode(ctx, options)
		case "node-edit":
			mutation, err = catalog.EditNode(ctx, options)
		}
		if err != nil {
			return err
		}
		if *format == "kv" {
			fmt.Printf("group_id=%s\nnode_count=%d\nrevision=%d\nstructure_changed=%t\n", mutation.GroupID, mutation.NodeCount, mutation.Revision, mutation.StructureChanged)
			return nil
		}
		code := "catalog.node_updated"
		message := "Catalog 节点已更新"
		if action == "node-append" {
			code = "catalog.node_appended"
			message = "Catalog 节点已加入"
		} else if action == "node-remove" {
			code = "catalog.node_removed"
			message = "Catalog 节点已移除"
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: mutation})
		return nil
	case "groups", "snapshot", "group", "show":
		if (action == "group" || action == "show") && *groupID == "" {
			return fmt.Errorf("Catalog %s 需要 --group", action)
		}
		activeGroup := *active
		if activeGroup == "" && *moduleConfig != "" {
			module, err := moduleconfig.LoadModule(*moduleConfig)
			if err != nil {
				return err
			}
			activeGroup = module.ActiveGroupID
		}
		groups, err := catalog.Scan(ctx, catalog.ScanOptions{
			Root: *root, ActiveGroup: activeGroup, ProgressDir: *progressDir,
			Type: *groupType, WithNodes: action == "snapshot" || action == "show", GroupID: *groupID,
		})
		if err != nil {
			return err
		}
		if action == "group" || action == "show" {
			if len(groups) == 0 {
				return fmt.Errorf("Catalog 分组不存在: %s", *groupID)
			}
			data := any(groups[0])
			if action == "group" {
				data = groups[0].Group
			}
			if *format == "tsv" {
				group := groups[0].Group
				fmt.Printf("id\t%s\nname\t%s\nruntime_tag\t%s\ntype\t%s\nnode_count\t%d\nrevision\t%d\nactive\t%t\n", group.ID, group.Name, group.RuntimeTag, group.Type, group.NodeCount, group.Revision, group.Active)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog." + action, Message: "Catalog 分组快照", Data: data})
		} else if action == "groups" {
			summaries := make([]catalog.GroupSummary, 0, len(groups))
			for _, group := range groups {
				summaries = append(summaries, group.Group)
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.groups", Message: "Catalog 分组快照", Data: summaries})
		} else {
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.snapshot", Message: "Catalog 节点快照", Data: groups})
		}
		return nil
	case "runtime":
		data, err := catalog.BuildRuntime(ctx, catalog.RuntimeOptions{
			Root: *root, ModuleConfig: *moduleConfig, ProvidersOutput: *providersOutput, OutboundsOutput: *outboundsOutput, StateOutput: *stateOutput,
			ActiveGroup: *active, SelectorMode: *selector, SelectedNodeRef: *selected,
			AllowEmpty: *allowEmpty,
		})
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.runtime", Message: "Catalog 运行时配置已生成", Data: data})
		return nil
	case "schedule":
		data, err := catalog.Schedule(*root, *now)
		if err != nil {
			return err
		}
		if *format == "tsv" {
			fmt.Printf("nearest\t%d\n", data.Nearest)
			for _, group := range data.Due {
				fmt.Printf("due\t%s\n", group)
			}
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("Catalog schedule 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.schedule", Message: "订阅调度快照", Data: data})
		return nil
	case "tag":
		if *groupID == "" {
			return errors.New("Catalog tag 需要 --group")
		}
		tag, err := catalog.RuntimeTag(*root, *groupID)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Println(tag)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("Catalog tag 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.tag", Message: "Catalog 运行时标签", Data: map[string]string{"tag": tag}})
		return nil
	case "ids":
		ids, err := catalog.GroupIDs(*root, *groupType)
		if err != nil {
			return err
		}
		if *format == "raw" {
			for _, id := range ids {
				fmt.Println(id)
			}
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("Catalog ids 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.ids", Message: "Catalog 分组 ID", Data: ids})
		return nil
	default:
		return fmt.Errorf("未知 Catalog 操作 %q", action)
	}
}

func runService(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Service API 操作")
	}
	action := args[0]
	flags := newFlagSet("service " + action)
	address := flags.String("address", "127.0.0.1:9090", "Service API 地址")
	secretValue := flags.String("secret", "", "Service API 密钥")
	timeout := flags.Duration("timeout", 8*time.Second, "请求超时")
	group := flags.String("group", "", "选择器标签")
	outbound := flags.String("outbound", "", "出站标签")
	mode := flags.String("mode", "", "出站模式")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	secret := strings.TrimSpace(*secretValue)
	requestContext, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	client, err := serviceapi.New(*address, secret)
	if err != nil {
		return err
	}
	defer client.Close()

	var data any
	switch action {
	case "ready":
		data, err = client.Ready(requestContext)
	case "started-at":
		data, err = client.StartedAt(requestContext)
	case "snapshot":
		status, statusErr := client.Status(requestContext)
		if statusErr != nil {
			err = statusErr
			break
		}
		groups, groupsErr := client.Groups(requestContext)
		if groupsErr != nil {
			err = groupsErr
			break
		}
		selected := ""
		targetGroup := *group
		if targetGroup == "" {
			targetGroup = "Proxy"
		}
		for _, item := range groups {
			if item.Tag == targetGroup {
				selected = item.Selected
				break
			}
		}
		data = serviceSnapshot{
			Memory:           status.Memory,
			Goroutines:       status.Goroutines,
			ConnectionsIn:    status.ConnectionsIn,
			ConnectionsOut:   status.ConnectionsOut,
			TrafficAvailable: status.TrafficAvailable,
			Uplink:           status.Uplink,
			Downlink:         status.Downlink,
			UplinkTotal:      status.UplinkTotal,
			DownlinkTotal:    status.DownlinkTotal,
			Selected:         selected,
		}
	case "groups":
		data, err = client.Groups(requestContext)
	case "mode":
		if *mode == "" {
			data, err = client.Mode(requestContext)
		} else {
			err = client.SetMode(requestContext, *mode)
			data = map[string]string{"mode": *mode}
		}
	case "select":
		if *group == "" || *outbound == "" {
			return errors.New("select 需要 --group 和 --outbound")
		}
		err = client.Select(requestContext, *group, *outbound)
		data = map[string]string{"group": *group, "outbound": *outbound}
	case "urltest":
		if *outbound == "" {
			return errors.New("urltest 需要 --outbound")
		}
		err = client.URLTest(requestContext, *outbound)
		data = map[string]string{"outbound": *outbound}
	case "close-all":
		err = client.CloseAllConnections(requestContext)
		data = map[string]bool{"closed": err == nil}
	default:
		return fmt.Errorf("未知 Service API 操作 %q", action)
	}
	if err != nil {
		return fmt.Errorf("Service API %s: %w", action, err)
	}
	if action == "snapshot" && *format == "tsv" {
		snapshot := data.(serviceSnapshot)
		fmt.Printf("selected\t%s\nmemory\t%d\nconnections_in\t%d\nconnections_out\t%d\nuplink_total\t%d\ndownlink_total\t%d\n",
			snapshot.Selected, snapshot.Memory, snapshot.ConnectionsIn, snapshot.ConnectionsOut,
			snapshot.UplinkTotal, snapshot.DownlinkTotal)
		return nil
	}
	if action == "started-at" && *format == "raw" {
		startedAt := data.(serviceapi.StartedAt)
		fmt.Printf("%d\n", startedAt.UnixMilli)
		return nil
	}
	if *format != "json" {
		return fmt.Errorf("操作 %s 不支持输出格式 %q", action, *format)
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "service." + action, Message: "Service API 操作完成", Data: data})
	return nil
}

func runConvert(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少转换类型: link、file 或 subscription")
	}
	switch args[0] {
	case "link":
		flags := newFlagSet("convert link")
		input := flags.String("input", "", "节点链接")
		options := bindConvertFlags(flags)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || options.output == "" {
			return errors.New("convert link 需要 --input 和 --output")
		}
		parsed, err := convert.Link(ctx, *input, options.allowInsecure)
		return saveConversion(ctx, options, parsed, err)

	case "file":
		flags := newFlagSet("convert file")
		input := flags.String("input", "", "输入文件")
		options := bindConvertFlags(flags)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || options.output == "" {
			return errors.New("convert file 需要 --input 和 --output")
		}
		content, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		parsed, parseErr := convert.Content(ctx, string(content), options.allowInsecure)
		return saveConversion(ctx, options, parsed, parseErr)

	case "subscription":
		return runConvertSubscription(ctx, args[1:])
	default:
		return fmt.Errorf("未知转换类型 %q", args[0])
	}
}

func runConvertSubscription(ctx context.Context, args []string) error {
	flags := newFlagSet("convert subscription")
	urlValue := flags.String("url", "", "订阅地址")
	options := bindConvertFlags(flags)
	userAgent := flags.String("user-agent", "", "请求 User-Agent")
	hwid := flags.String("hwid", "", "请求 X-HWID")
	etag := flags.String("etag", "", "条件请求 ETag")
	lastModified := flags.String("last-modified", "", "条件请求 Last-Modified")
	proxyURL := flags.String("proxy", "", "下载代理地址")
	headersFile := flags.String("headers-file", "", "JSON 格式自定义请求头文件")
	timeout := flags.Duration("timeout", 60*time.Second, "下载超时")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *urlValue == "" || options.output == "" {
		return errors.New("convert subscription 需要 --url 和 --output")
	}
	var headers map[string]string
	if *headersFile != "" {
		content, err := os.ReadFile(*headersFile)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(content, &headers); err != nil {
			return fmt.Errorf("自定义请求头文件无效: %w", err)
		}
	}
	response, err := fetch.Subscription(ctx, fetch.Request{
		URL:           *urlValue,
		UserAgent:     *userAgent,
		HWID:          *hwid,
		Headers:       headers,
		ETag:          *etag,
		LastModified:  *lastModified,
		ProxyURL:      *proxyURL,
		AllowInsecure: options.allowInsecure,
		Timeout:       *timeout,
	})
	if metadataErr := writeOptionalJSON(options.metadataOutput, response.Metadata); metadataErr != nil {
		return metadataErr
	}
	if err != nil {
		return err
	}
	if response.Metadata.NotModified {
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.not_modified", Message: "订阅未发生变化", Data: response.Metadata})
		return nil
	}
	parsed, parseErr := convert.Content(ctx, string(response.Body), options.allowInsecure)
	parsed.Diagnostics = append(response.Metadata.Diagnostics, parsed.Diagnostics...)
	return saveConversion(ctx, options, parsed, parseErr)
}

func runProvider(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Provider 操作: append、remove、inspect、get、export 或 validate")
	}
	switch args[0] {
	case "append":
		flags := newFlagSet("provider append")
		target := flags.String("target", "", "目标 provider.json")
		input := flags.String("input", "", "节点链接或输入文件")
		tag := flags.String("tag", "", "只追加输入 Provider 中的指定标签")
		allowInsecure := flags.Bool("allow-insecure", false, "跳过节点 TLS 证书校验")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *target == "" || *input == "" {
			return errors.New("provider append 需要 --target 和 --input")
		}
		var targetDocument provider.Document
		if _, err := os.Stat(*target); err == nil {
			targetDocument, err = provider.LoadAllowEmpty(ctx, *target)
			if err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		source, err := parseInput(ctx, *input, *allowInsecure)
		if err != nil {
			return err
		}
		if *tag != "" {
			selected, found := provider.Select(source.Document, *tag)
			if !found {
				return fmt.Errorf("输入 Provider 中未找到节点标签 %q", *tag)
			}
			source.Document = selected
		}
		provider.Append(&targetDocument, source.Document)
		if err := provider.SaveAtomic(ctx, *target, targetDocument); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "provider.appended", Message: "节点已加入 Provider", Data: provider.Inspect(targetDocument)})
		return nil

	case "remove":
		flags := newFlagSet("provider remove")
		target := flags.String("target", "", "目标 provider.json")
		tag := flags.String("tag", "", "节点标签")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *target == "" || *tag == "" {
			return errors.New("provider remove 需要 --target 和 --tag")
		}
		document, err := provider.Load(ctx, *target)
		if err != nil {
			return err
		}
		if !provider.Remove(&document, *tag) {
			return fmt.Errorf("未找到节点标签 %q", *tag)
		}
		if err := provider.SaveAtomicAllowEmpty(ctx, *target, document); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "provider.removed", Message: "节点已从 Provider 移除", Data: provider.Inspect(document)})
		return nil

	case "inspect":
		flags := newFlagSet("provider inspect")
		input := flags.String("input", "", "provider.json")
		format := flags.String("format", "json", "输出格式")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *format != "json" {
			return errors.New("provider inspect 需要 --input，且当前仅支持 --format json")
		}
		document, err := provider.LoadAllowEmpty(ctx, *input)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "provider.inspected", Message: "Provider 摘要", Data: provider.Inspect(document)})
		return nil

	case "get":
		flags := newFlagSet("provider get")
		input := flags.String("input", "", "provider.json")
		tag := flags.String("tag", "", "节点标签")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *tag == "" {
			return errors.New("provider get 需要 --input 和 --tag")
		}
		document, err := provider.Load(ctx, *input)
		if err != nil {
			return err
		}
		selected, found := provider.Select(document, *tag)
		if !found {
			return fmt.Errorf("未找到节点标签 %q", *tag)
		}
		content, err := provider.Marshal(ctx, selected)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "provider.loaded", Message: "节点配置已读取", Data: json.RawMessage(content)})
		return nil

	case "export":
		flags := newFlagSet("provider export")
		input := flags.String("input", "", "provider.json")
		tag := flags.String("tag", "", "节点标签")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *tag == "" {
			return errors.New("provider export 需要 --input 和 --tag")
		}
		document, err := provider.Load(ctx, *input)
		if err != nil {
			return err
		}
		exported, err := sharelink.Export(document, *tag)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "provider.exported", Message: "节点分享链接已生成", Data: exported})
		return nil

	case "validate":
		flags := newFlagSet("provider validate")
		input := flags.String("input", "", "provider.json")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" {
			return errors.New("provider validate 需要 --input")
		}
		document, err := provider.Load(ctx, *input)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "provider.valid", Message: "Provider 配置有效", Data: map[string]int{
			"node_count": len(document.Outbounds) + len(document.Endpoints),
		}})
		return nil

	default:
		return fmt.Errorf("未知 Provider 操作 %q", args[0])
	}
}

func bindConvertFlags(flags *flag.FlagSet) *convertOptions {
	options := &convertOptions{}
	flags.StringVar(&options.output, "output", "", "输出 provider.json")
	flags.StringVar(&options.metadataOutput, "metadata-output", "", "HTTP 元数据输出文件")
	flags.StringVar(&options.diagnosticsFile, "diagnostics-output", "", "解析诊断输出文件")
	flags.BoolVar(&options.allowInsecure, "allow-insecure", false, "跳过节点或下载 TLS 证书校验")
	flags.StringVar(&options.include, "include", "", "只保留标签匹配的节点")
	flags.StringVar(&options.exclude, "exclude", "", "排除标签匹配的节点")
	return options
}

func saveConversion(ctx context.Context, options *convertOptions, parsed provider.ParseResult, parseErr error) error {
	if err := writeOptionalJSON(options.diagnosticsFile, parsed.Diagnostics); err != nil {
		return err
	}
	if parseErr != nil {
		return &resultError{Code: "conversion.failed", Message: parseErr.Error(), Data: parsed.Diagnostics}
	}
	filtered, err := provider.Filter(parsed.Document, options.include, options.exclude)
	if err != nil {
		return err
	}
	if err := provider.SaveAtomic(ctx, options.output, filtered); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "conversion.completed", Message: "转换完成", Data: map[string]any{
		"node_count":  len(filtered.Outbounds) + len(filtered.Endpoints),
		"diagnostics": parsed.Diagnostics,
	}})
	return nil
}

func parseInput(ctx context.Context, input string, allowInsecure bool) (provider.ParseResult, error) {
	return convert.Input(ctx, input, allowInsecure)
}

func readHeadersFile(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var headers map[string]string
	if err := json.Unmarshal(content, &headers); err != nil {
		return nil, fmt.Errorf("自定义请求头 JSON 无效: %w", err)
	}
	if headers == nil {
		return map[string]string{}, nil
	}
	return headers, nil
}

func writeOptionalJSON(path string, value any) error {
	if path == "" {
		return nil
	}
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return provider.WriteAtomic(path, content, 0o600)
}

func writeJSON(writer io.Writer, value any) {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func dependencyVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dependency := range info.Deps {
		if dependency.Path != path {
			continue
		}
		if dependency.Replace != nil && dependency.Replace.Version != "" {
			return dependency.Replace.Version
		}
		return dependency.Version
	}
	return "unknown"
}

func showUsage() {
	executable := filepath.Base(os.Args[0])
	fmt.Printf(`%s - NetProxy 原生组件

用法：
  %s convert link --input <链接> --output <provider.json>
  %s convert file --input <文件> --output <provider.json>
  %s convert subscription --url <地址> --output <provider.json>
  %s provider append --target <provider.json> --input <链接或文件>
  %s provider remove --target <provider.json> --tag <标签>
  %s catalog group-import --root <catalog> --group <分组 ID> --input <文件>
  %s catalog group-init --root <catalog> --group <分组 ID> --type <local|subscription>
  %s catalog group-ensure --root <catalog> --group <分组 ID> --type local
  %s catalog group-name --root <catalog> --group <分组 ID> --name <名称>
  %s catalog new-id --root <catalog> --kind <subscription|file> [--input <文件>]
  %s catalog node-append --group-dir <分组目录> --group <分组 ID> --input <链接或文件>
  %s catalog node-remove --group-dir <分组目录> --group <分组 ID> --tag <标签>
  %s catalog node-edit --group-dir <分组目录> --group <分组 ID> --tag <标签> --input <链接或文件>
  %s catalog resolve|group-has-nodes|group-first-tag|group-contains-tag ...
  %s catalog group-type|first-nonempty|node-get|node-export ...
  %s catalog group-private|group-delete|history ...
  %s provider inspect --input <provider.json> --format json
  %s provider export --input <provider.json> --tag <标签>
  %s provider validate --input <provider.json>
  %s catalog <groups|snapshot|group|show|runtime|schedule> --root <catalog>
  %s subscription update|edit --root <catalog> --group <group-id>
  %s service <ready|started-at|snapshot|groups|mode|select|urltest|close-all>
  netproxy-native module <prepare|sync|select|mode|app|node|sub|config|logs|state|service> ...
  control <status|nodes|snapshot|selection|groups|mode|runtime-mode|set-mode|delay|close-all> ...
  netproxy-native ebpf <runtime|validate|diagnose> --config <ebpf.conf> [--output <ebpf.json>]
  %s subworker <start|stop|restart|wake|status|once|run> --root <catalog> --module-conf <module.conf>
  netproxy-native version

转换选项：
  --include <正则>              仅保留匹配的节点
  --exclude <正则>              排除匹配的节点
  --allow-insecure              显式跳过 TLS 证书校验
  --diagnostics-output <文件>   写入结构化解析诊断
  --metadata-output <文件>      写入订阅 HTTP 元数据

订阅选项：
  --user-agent <值>             自定义 User-Agent
  --hwid <值>                   自定义 X-HWID
  --headers-file <文件>         从 JSON 对象读取自定义请求头
  --etag <值>                   发送 If-None-Match
  --last-modified <值>          发送 If-Modified-Since
  --proxy <URL>                 通过 HTTP 代理下载
  --timeout <时长>              下载超时，默认 60s
`, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable)
}
