package module

import (
	"context"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

// ProbeEBPF 预检保存的 eBPF 模板，不表示当前入站已经挂载或接管。
func ProbeEBPF(ctx context.Context, options Options, mode string, raw bool) (map[string]any, error) {
	config, err := inbound.Load(options.InboundConfig)
	if err != nil {
		return nil, err
	}
	native, err := config.EBPFOptions()
	if err != nil {
		return nil, err
	}
	probeOptions, err := inbound.ResolveProbeOptions(native, mode)
	if err != nil {
		return nil, &service.Error{Code: "ebpf.status_failed", Message: err.Error()}
	}
	probeOutput, probeErr := inbound.RunProbe(ctx, options.SingBoxPath, native, mode)
	if ctx.Err() != nil {
		return nil, ctx.Err()
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
		return nil, &service.Error{
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
		"mode":    probeOptions.RequestedMode,
		"raw":     raw,
		"content": content,
		"report":  report,
	}
	if probeErr != nil || report.Result != "preflight_passed" {
		return nil, &service.Error{Code: "ebpf.unsupported", Message: "eBPF 能力检查未通过", Data: data}
	}
	return data, nil
}
