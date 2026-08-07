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
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/sharelink"
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
	case "service":
		return runService(ctx, args[1:])
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

func runCatalog(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Catalog 操作")
	}
	action := args[0]
	flags := newFlagSet("catalog " + action)
	root := flags.String("root", "", "Catalog 根目录")
	active := flags.String("active", "", "活动分组 ID")
	progressDir := flags.String("progress-dir", "", "订阅更新进度目录")
	groupType := flags.String("type", "all", "分组类型筛选")
	groupID := flags.String("group", "", "指定分组 ID")
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
	if *root == "" {
		return errors.New("Catalog 操作需要 --root")
	}

	switch action {
	case "groups", "snapshot", "group", "show":
		if (action == "group" || action == "show") && *groupID == "" {
			return fmt.Errorf("Catalog %s 需要 --group", action)
		}
		groups, err := catalog.Scan(ctx, catalog.ScanOptions{
			Root: *root, ActiveGroup: *active, ProgressDir: *progressDir,
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
			Root: *root, ProvidersOutput: *providersOutput, OutboundsOutput: *outboundsOutput, StateOutput: *stateOutput,
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
	if info, err := os.Stat(input); err == nil && !info.IsDir() {
		content, err := os.ReadFile(input)
		if err != nil {
			return provider.ParseResult{}, err
		}
		return convert.Content(ctx, string(content), allowInsecure)
	}
	if strings.Contains(input, "://") && !strings.Contains(input, "\n") {
		return convert.Link(ctx, input, allowInsecure)
	}
	return convert.Content(ctx, input, allowInsecure)
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
  %s provider inspect --input <provider.json> --format json
  %s provider export --input <provider.json> --tag <标签>
  %s provider validate --input <provider.json>
  %s catalog <groups|snapshot|runtime> --root <catalog>
  %s service <ready|status|snapshot|groups|selected|mode|select|urltest|close-all>
  %s version

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
`, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable)
}
