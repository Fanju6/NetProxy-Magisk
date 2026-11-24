package main

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"
	"os"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

func (c *cli) trafficAPI(ctx context.Context, args []string) error {
	const maxRequest = traffic.MaxRulesBytes + 1024
	usage := func() error {
		return usageError("用法: netproxyctl api traffic status|start|stop|access|list|clear|rules|set-rules <文件>")
	}
	if len(args) == 0 || (args[0] == "set-rules" && len(args) != 2) || (args[0] != "set-rules" && len(args) != 1) {
		return usage()
	}
	var method string
	var body io.Reader
	action := args[0]
	switch action {
	case "status", "access", "list", "rules":
		method = http.MethodGet
	case "start", "stop":
		method = http.MethodPost
	case "clear":
		method, action = http.MethodDelete, "list"
	case "set-rules":
		method, action = http.MethodPut, "rules"
		file, err := os.Open(args[1])
		if err != nil {
			return usageError("无法打开重写规则文件")
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, maxRequest+1))
		if err != nil || len(content) > maxRequest {
			return usageError("无法读取重写规则文件或文件超出大小限制")
		}
		body = bytes.NewReader(content)
	default:
		return usage()
	}
	config, err := apiconfig.Load(paths.New(c.options.ModuleDir).APIConfig())
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
	const maxResponse = 512 << 10
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	var envelope result
	if err != nil || len(content) > maxResponse || json.Unmarshal(content, &envelope) != nil ||
		envelope.Schema != 1 || envelope.Code == "" || envelope.OK != (response.StatusCode == http.StatusOK) {
		return &resultError{Code: "traffic.api_unavailable", Message: "管理接口没有返回有效的抓包结果"}
	}
	if !envelope.OK {
		return &resultError{Code: envelope.Code, Message: envelope.Message, Data: envelope.Data}
	}
	writeJSON(os.Stdout, envelope)
	return nil
}
