package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/sharelink"
)

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
