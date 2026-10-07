package main

import (
	"context"
	"os"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
)

func (c *cli) ebpf(ctx context.Context, args []string) error {
	action := "status"
	if len(args) > 0 {
		action = args[0]
		args = args[1:]
	}
	switch action {
	case "status":
		mode := "configured"
		raw := false
		for _, argument := range args {
			switch argument {
			case "configured", "all", "local", "shared":
				mode = argument
			case "--raw":
				raw = true
			default:
				return usageError("用法: netproxyctl ebpf status [configured|all|local|shared] [--raw]")
			}
		}
		config, err := inbound.Load(c.options.InboundConfig)
		if err != nil {
			return err
		}
		native, err := config.EBPFOptions()
		if err != nil {
			return err
		}
		options, err := inbound.ResolveProbeOptions(native, mode)
		if err != nil {
			return &resultError{Code: "ebpf.status_failed", Message: err.Error()}
		}
		probeOutput, probeErr := inbound.RunProbe(ctx, c.options.SingBoxPath, native, mode)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		report, parseErr := inbound.ParseProbeReport(probeOutput)
		if parseErr != nil {
			content := "无法读取 eBPF 诊断报告，请检查内核版本与核心日志。"
			if probeErr != nil {
				content = "eBPF 诊断命令未能完成，请检查核心文件与 Root 权限。\n" + probeErr.Error()
			}
			if raw {
				content = probeOutput
			}
			return &resultError{
				Code:    "ebpf.status_invalid",
				Message: parseErr.Error(),
				Data:    map[string]any{"raw": raw, "content": content},
			}
		}
		content := probeOutput
		if !raw {
			content = inbound.FormatProbeOutput(report, probeErr)
		}
		data := map[string]any{
			"mode":    options.RequestedMode,
			"raw":     raw,
			"content": content,
			"report":  report,
		}
		if probeErr != nil || report.Result != "preflight_passed" {
			return &resultError{Code: "ebpf.unsupported", Message: "eBPF 能力检查未通过", Data: data}
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "ebpf.status", Message: "eBPF 能力检查完成", Data: data})
		return nil
	default:
		return usageError("用法: netproxyctl ebpf status [configured|all|local|shared] [--raw]")
	}
}
