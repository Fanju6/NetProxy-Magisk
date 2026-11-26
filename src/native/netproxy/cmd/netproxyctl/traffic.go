package main

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

func (c *cli) trafficAPI(ctx context.Context, args []string) error {
	const maxRequest = traffic.MaxRulesBytes + 1024
	usage := func() error {
		return usageError("用法: netproxyctl api traffic status|start|stop|toggle|list [批次ID]|page [批次ID [游标]]|batches|delete-batch <ID>|detail <ID>|clear|apps|set-apps <文件>|rules|set-rules <文件>|mitm|set-mitm|pinned|clear-pinned|ca|ca-init|ca-export|ca-remove（规则 enabled=false 可单独停用）")
	}
	if len(args) == 0 {
		return usage()
	}
	action := args[0]
	withArgument := action == "set-rules" || action == "set-mitm" || action == "set-apps" || action == "ca-export" ||
		action == "detail" || action == "delete-batch" || (action == "list" && len(args) > 1)
	if (action == "page" && len(args) > 3) || (action != "page" && ((withArgument && len(args) != 2) || (!withArgument && len(args) != 1))) {
		return usage()
	}
	layout := paths.New(c.options.ModuleDir)
	if action == "trust-remove" {
		if err := traffic.RemoveSystemTrust(ctx, layout); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "traffic.trust_removed", Message: "系统和个人抓包 CA 已卸载"})
		return nil
	}
	var method string
	var body io.Reader
	switch action {
	case "status", "batches", "apps", "rules", "mitm", "pinned", "ca":
		method = http.MethodGet
	case "page":
		method = http.MethodGet
		query := url.Values{}
		if len(args) > 1 {
			if !traffic.ValidRecordID(args[1]) {
				return usageError("抓包批次 ID 无效")
			}
			query.Set("batch", args[1])
		}
		if len(args) > 2 {
			if cursor, err := strconv.ParseUint(args[2], 10, 64); err != nil || cursor == 0 {
				return usageError("抓包分页游标无效")
			}
			query.Set("before", args[2])
		}
		if len(query) != 0 {
			action += "?" + query.Encode()
		}
	case "list", "delete-batch":
		method = http.MethodGet
		if withArgument {
			if !traffic.ValidRecordID(args[1]) {
				return usageError("抓包批次 ID 无效")
			}
			if action == "list" {
				action = "list?batch=" + url.QueryEscape(args[1])
			} else {
				method, action = http.MethodDelete, "batches?id="+url.QueryEscape(args[1])
			}
		}
	case "detail":
		if !traffic.ValidRecordID(args[1]) {
			return usageError("抓包记录 ID 无效")
		}
		method, action = http.MethodGet, "detail?id="+url.QueryEscape(args[1])
	case "start", "stop", "toggle", "ca-init":
		method = http.MethodPost
	case "clear":
		method, action = http.MethodDelete, "list"
	case "clear-pinned":
		method, action = http.MethodDelete, "pinned"
	case "ca-remove":
		method, action = http.MethodDelete, "ca/trust"
	case "ca-export":
		method, action = http.MethodGet, "ca"
	case "set-rules", "set-mitm", "set-apps":
		method = http.MethodPut
		limit := int64(maxRequest)
		switch action {
		case "set-mitm":
			action, limit = "mitm", traffic.MaxMITMBytes
		case "set-apps":
			action, limit = "apps", traffic.MaxAppsBytes
		default:
			action = "rules"
		}
		file, err := os.Open(args[1])
		if err != nil {
			return usageError("无法打开配置文件")
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, limit+1))
		if err != nil || int64(len(content)) > limit {
			return usageError("无法读取配置文件或文件超出大小限制")
		}
		body = bytes.NewReader(content)
	default:
		return usage()
	}
	config, err := apiconfig.Load(layout.APIConfig())
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, config.Connection().Address+"/api/v1/traffic/"+action, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+config.Secret)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	transport := &http.Transport{DisableKeepAlives: true, TLSClientConfig: config.ClientTLS()}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	defer transport.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return &resultError{Code: "traffic.api_unavailable", Message: "无法连接本机管理接口，请检查 api status"}
	}
	defer response.Body.Close()
	const maxResponse = 4 << 20
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	var envelope result
	if err != nil || len(content) > maxResponse || json.Unmarshal(content, &envelope) != nil ||
		envelope.Schema != 1 || envelope.Code == "" || envelope.OK != (response.StatusCode == http.StatusOK) {
		return &resultError{Code: "traffic.api_unavailable", Message: "管理接口没有返回有效的抓包结果"}
	}
	if !envelope.OK {
		return &resultError{Code: envelope.Code, Message: envelope.Message, Data: envelope.Data}
	}
	if args[0] == "ca-export" {
		return exportTrafficCA(args[1], envelope.Data)
	}
	writeJSON(os.Stdout, envelope)
	return nil
}

func exportTrafficCA(path string, data any) error {
	fields, _ := data.(map[string]any)
	value, _ := fields["pem"].(string)
	block, rest := pem.Decode([]byte(value))
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return &resultError{Code: "traffic.ca_unavailable", Message: "管理接口没有返回公开 CA 证书"}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return usageError("无法新建 CA 导出文件；不会覆盖已有文件")
	}
	_, writeErr := f.Write(pem.EncodeToMemory(block))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return usageError("无法写入 CA 导出文件")
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "traffic.ca_exported", Message: "公开抓包 CA 已导出",
		Data: map[string]any{"filename": fields["filename"], "sha256": fields["sha256"]}})
	return nil
}
