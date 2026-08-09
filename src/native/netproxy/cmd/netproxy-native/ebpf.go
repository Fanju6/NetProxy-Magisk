package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/ebpf"
)

func runEBPF(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 eBPF 操作: runtime|status")
	}
	action := args[0]
	flags := newFlagSet("ebpf " + action)
	configPath := flags.String("config", "", "ebpf.conf 路径")
	outputPath := flags.String("output", "", "运行时 JSON 输出路径")
	singBoxPath := flags.String("sing-box", "", "sing-box 二进制路径")
	mode := flags.String("mode", "configured", "configured|all|local|shared")
	raw := flags.Bool("raw", false, "直接返回 sing-box 原始诊断")
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
	case "status":
		if strings.TrimSpace(*singBoxPath) == "" {
			return errors.New("eBPF 能力检查需要 --sing-box")
		}
		options, err := ebpf.ResolveProbeOptions(*configPath, *mode)
		if err != nil {
			return &resultError{Code: "ebpf.status_failed", Message: err.Error()}
		}
		probeOutput, probeErr := ebpf.RunProbe(ctx, *singBoxPath, options)
		content := probeOutput
		if !*raw {
			content = ebpf.FormatProbeOutput(probeOutput, options.CoreMode, probeErr)
		}
		data := map[string]any{
			"mode":    options.RequestedMode,
			"raw":     *raw,
			"content": content,
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
