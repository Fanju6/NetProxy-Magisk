package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/ebpf"
)

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
