package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

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
