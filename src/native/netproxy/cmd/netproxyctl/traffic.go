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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

func (c *cli) trafficAPI(ctx context.Context, args []string) error {
	const maxRequest = traffic.MaxRulesBytes + 1024
	usage := func() error {
		return usageError("用法: netproxyctl api traffic status|start|stop|toggle|list [批次ID]|page [批次ID [游标]]|batches|delete-batch <ID>|detail <ID>|resend <ID>|replay <文件>|breakpoints|set-breakpoints <文件>|paused [ID]|resume <ID> [文件]|abort <ID>|clear|apps|set-apps <文件>|rules|set-rules <文件>|mitm|set-mitm|pinned|clear-pinned|alpn-bypasses|clear-alpn-bypasses|ca|ca-init|ca-export|ca-remove（规则可选 name 标题，enabled=false 可单独停用）")
	}
	if len(args) == 0 {
		return usage()
	}
	action := args[0]
	removeBypass := (action == "pinned" || action == "alpn-bypasses") && len(args) > 1
	archive := action == "export" || action == "import"
	withArgument := action == "set-rules" || action == "set-mitm" || action == "set-apps" || action == "ca-export" ||
		action == "detail" || action == "delete-batch" || action == "replay" || action == "resend" ||
		action == "set-breakpoints" || action == "abort" || ((action == "list" || action == "paused") && len(args) > 1)
	if !archive && !removeBypass && ((action == "page" && len(args) > 3) || (action == "resume" && (len(args) < 2 || len(args) > 3)) ||
		(action != "page" && action != "resume" && ((withArgument && len(args) != 2) || (!withArgument && len(args) != 1)))) {
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
	case "export":
		query := url.Values{}
		for i := 1; i < len(args); i += 2 {
			flag, ok := strings.CutPrefix(args[i], "--")
			switch flag {
			case "batch", "id", "host", "query", "method", "status-class", "tls", "rewritten", "app", "outbound":
			default:
				ok = false
			}
			key := strings.ReplaceAll(flag, "-", "_")
			if !ok || i+1 >= len(args) || query.Has(key) {
				return usageError("用法: netproxyctl api traffic export [--batch ID | --id ID] [--host 域名] [--query 搜索] [--method 方法] [--status-class 1–5] [--tls decrypted|tunnel] [--rewritten true|false] [--app 应用] [--outbound 出站]")
			}
			query.Set(key, args[i+1])
		}
		method, action = http.MethodGet, "export?"+query.Encode()
	case "import":
		if len(args) != 2 {
			return usageError("用法: netproxyctl api traffic import <文件>")
		}
		content, err := readTrafficInput(args[1], traffic.MaxHARImportBytes)
		if err != nil {
			return err
		}
		method, action, body = http.MethodPost, "import?name="+url.QueryEscape(filepath.Base(args[1])), bytes.NewReader(content)
	case "status", "batches", "apps", "rules", "mitm", "pinned", "alpn-bypasses", "ca", "breakpoints":
		method = http.MethodGet
		if removeBypass {
			if len(args) != 4 || args[1] != "remove" || args[2] == "" || args[3] == "" {
				return usageError("用法: netproxyctl api traffic pinned|alpn-bypasses [remove <域名> <用户ID:包名>]")
			}
			query := url.Values{"host": {args[2]}, "app": {args[3]}}
			method, action = http.MethodDelete, action+"?"+query.Encode()
		}
	case "paused", "resume", "abort":
		method = http.MethodPost
		if action == "paused" {
			method = http.MethodGet
		}
		if len(args) > 1 {
			if !traffic.ValidRecordID(args[1]) {
				return usageError("断点 ID 无效")
			}
			action += "?id=" + url.QueryEscape(args[1])
		}
		if len(args) == 3 {
			content, err := readTrafficInput(args[2], 2<<20)
			if err != nil {
				return err
			}
			body = bytes.NewReader(content)
		}
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
	case "resend":
		if !traffic.ValidRecordID(args[1]) {
			return usageError("抓包记录 ID 无效")
		}
		payload, err := json.Marshal(traffic.ReplayInput{ID: args[1]})
		if err != nil {
			return err
		}
		method, action, body = http.MethodPost, "replay", bytes.NewReader(payload)
	case "start", "stop", "toggle", "ca-init":
		method = http.MethodPost
	case "clear":
		method, action = http.MethodDelete, "list"
	case "clear-pinned":
		method, action = http.MethodDelete, "pinned"
	case "clear-alpn-bypasses":
		method, action = http.MethodDelete, "alpn-bypasses"
	case "ca-remove":
		method, action = http.MethodDelete, "ca/trust"
	case "ca-export":
		method, action = http.MethodGet, "ca"
	case "set-rules", "set-mitm", "set-apps", "set-breakpoints", "replay":
		method = http.MethodPut
		limit := int64(maxRequest)
		switch action {
		case "set-mitm":
			action, limit = "mitm", traffic.MaxMITMBytes
		case "set-apps":
			action, limit = "apps", traffic.MaxAppsBytes
		case "replay":
			method, limit = http.MethodPost, 512<<10
		case "set-breakpoints":
			action = "breakpoints"
		default:
			action = "rules"
		}
		content, err := readTrafficInput(args[1], limit)
		if err != nil {
			return err
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
	if args[0] == "export" && response.StatusCode == http.StatusOK {
		if response.Header.Get("Content-Type") != "application/json" || response.Header.Get("Content-Disposition") == "" {
			return &resultError{Code: "traffic.api_unavailable", Message: "管理接口没有返回 HAR 文件"}
		}
		_, err := io.Copy(os.Stdout, response.Body)
		return err
	}
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

func readTrafficInput(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, usageError("无法打开配置文件")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(content)) > limit {
		return nil, usageError("无法读取配置文件或文件超出大小限制")
	}
	return content, nil
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
