package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/ebpf"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func runEBPF(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 eBPF 操作: runtime|status")
	}
	action := args[0]
	flags := newFlagSet("ebpf " + action)
	moduleDir := flags.String("module-dir", defaultModuleDir(), "模块根目录")
	configPath := flags.String("config", "", "ebpf.conf 路径")
	outputPath := flags.String("output", "", "运行时 JSON 输出路径")
	singBoxPath := flags.String("sing-box", "", "sing-box 二进制路径")
	mode := flags.String("mode", "configured", "configured|all|local|shared")
	raw := flags.Bool("raw", false, "直接返回 sing-box 原始诊断")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	layout := paths.New(*moduleDir)
	if strings.TrimSpace(*configPath) == "" {
		*configPath = layout.EBPFConfig()
	}
	if strings.TrimSpace(*singBoxPath) == "" {
		*singBoxPath = layout.SingBox()
	}
	if strings.TrimSpace(*configPath) == "" {
		return errors.New("eBPF 操作需要 --config")
	}
	toError := func(err error) error {
		if validation, ok := errors.AsType[*ebpf.ValidationError](err); ok {
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
		missingPackages, err := ebpf.WriteAtomic(*outputPath, config)
		if err != nil {
			return toError(err)
		}
		if *format == "text" {
			for _, ref := range missingPackages {
				fmt.Fprintf(os.Stderr, "WARN 分应用代理跳过未安装应用: %s\n", ref.String())
			}
			fmt.Fprintln(os.Stdout, *outputPath)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("ebpf runtime 不支持输出格式 %q", *format)
		}
		data := map[string]any{"path": *outputPath}
		if len(missingPackages) > 0 {
			skipped := make([]string, 0, len(missingPackages))
			for _, ref := range missingPackages {
				skipped = append(skipped, ref.String())
			}
			data["skipped_packages"] = skipped
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "ebpf.runtime_generated", Message: "eBPF 运行时配置已生成", Data: data})
		return nil
	case "status":
		if strings.TrimSpace(*singBoxPath) == "" {
			return errors.New("eBPF 能力检查需要 --sing-box")
		}
		options, err := ebpf.ResolveProbeOptions(*configPath, *mode)
		if err != nil {
			return &resultError{Code: "ebpf.status_failed", Message: err.Error()}
		}
		probeOutput, probeErr := ebpf.RunProbe(ctx, *singBoxPath, options)
		report, parseErr := ebpf.ParseProbeReport(probeOutput)
		if parseErr != nil {
			return &resultError{
				Code:    "ebpf.status_invalid",
				Message: parseErr.Error(),
				Data:    map[string]any{"raw": *raw, "content": probeOutput},
			}
		}
		content := probeOutput
		if !*raw {
			content = ebpf.FormatProbeOutput(report, probeErr)
		}
		data := map[string]any{
			"mode":    options.RequestedMode,
			"raw":     *raw,
			"content": content,
			"report":  report,
		}
		if *format == "text" {
			fmt.Fprintln(os.Stdout, content)
			return probeErr
		}
		if *format != "json" {
			return fmt.Errorf("ebpf %s 不支持输出格式 %q", action, *format)
		}
		if probeErr != nil {
			return &resultError{Code: "ebpf.unsupported", Message: "eBPF 能力检查未通过", Data: data}
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "ebpf.status", Message: "eBPF 能力检查完成", Data: data})
		return nil
	default:
		return fmt.Errorf("未知 eBPF 操作 %q", action)
	}
}
