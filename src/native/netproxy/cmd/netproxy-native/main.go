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

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
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

type headerFlags map[string]string

func (h *headerFlags) String() string {
	return ""
}

func (h *headerFlags) Set(value string) error {
	key, headerValue, found := strings.Cut(value, ":")
	key = strings.TrimSpace(key)
	if !found || key == "" {
		return errors.New("请求头格式必须为 名称:值")
	}
	if *h == nil {
		*h = make(map[string]string)
	}
	(*h)[key] = strings.TrimSpace(headerValue)
	return nil
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

func runService(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Service API 操作")
	}
	action := args[0]
	flags := newFlagSet("service " + action)
	address := flags.String("address", "127.0.0.1:9090", "Service API 地址")
	secretValue := flags.String("secret", "", "Service API 密钥")
	secretFile := flags.String("secret-file", "", "Service API 密钥文件")
	timeout := flags.Duration("timeout", 8*time.Second, "请求超时")
	group := flags.String("group", "", "选择器标签")
	outbound := flags.String("outbound", "", "出站标签")
	mode := flags.String("mode", "", "Clash 模式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	secret := strings.TrimSpace(*secretValue)
	if *secretFile != "" {
		if secret != "" {
			return errors.New("--secret 与 --secret-file 不能同时使用")
		}
		content, err := os.ReadFile(*secretFile)
		if err != nil {
			return fmt.Errorf("读取 Service API 密钥: %w", err)
		}
		secret = strings.TrimSpace(string(content))
	}
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
	case "version":
		data, err = client.Version(requestContext)
	case "started-at":
		data, err = client.StartedAt(requestContext)
	case "status":
		data, err = client.Status(requestContext)
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
		data = map[string]any{
			"memory":            status.Memory,
			"goroutines":        status.Goroutines,
			"connections_in":    status.ConnectionsIn,
			"connections_out":   status.ConnectionsOut,
			"traffic_available": status.TrafficAvailable,
			"uplink":            status.Uplink,
			"downlink":          status.Downlink,
			"uplink_total":      status.UplinkTotal,
			"downlink_total":    status.DownlinkTotal,
			"selected":          selected,
		}
	case "groups":
		data, err = client.Groups(requestContext)
	case "selected":
		if *group == "" {
			return errors.New("selected 需要 --group")
		}
		var groups []serviceapi.Group
		groups, err = client.Groups(requestContext)
		if err == nil {
			found := false
			for _, item := range groups {
				if item.Tag == *group {
					data = map[string]string{"group": item.Tag, "outbound": item.Selected}
					found = true
					break
				}
			}
			if !found {
				err = fmt.Errorf("selector %q not found", *group)
			}
		}
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
	default:
		return fmt.Errorf("未知 Service API 操作 %q", action)
	}
	if err != nil {
		return fmt.Errorf("Service API %s: %w", action, err)
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
	var headers headerFlags
	flags.Var(&headers, "header", "自定义请求头，可重复")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *urlValue == "" || options.output == "" {
		return errors.New("convert subscription 需要 --url 和 --output")
	}
	if *headersFile != "" {
		content, err := os.ReadFile(*headersFile)
		if err != nil {
			return err
		}
		var fileHeaders map[string]string
		if err := json.Unmarshal(content, &fileHeaders); err != nil {
			return fmt.Errorf("自定义请求头文件无效: %w", err)
		}
		if headers == nil {
			headers = make(map[string]string)
		}
		for key, value := range fileHeaders {
			headers[key] = value
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
		return errors.New("缺少 Provider 操作: append、remove、inspect 或 validate")
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
  %s provider validate --input <provider.json>
  %s service <ready|status|snapshot|groups|selected|mode|select|urltest>
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
  --header <名称:值>            自定义请求头，可重复
  --headers-file <文件>         从 JSON 对象读取自定义请求头
  --etag <值>                   发送 If-None-Match
  --last-modified <值>          发送 If-Modified-Since
  --proxy <URL>                 通过 HTTP 代理下载
  --timeout <时长>              下载超时，默认 60s
`, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable)
}
