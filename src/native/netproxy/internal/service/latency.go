package service

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
)

type NodeLatencyTarget struct {
	CoreStartedAtMillis int64  `json:"core_started_at_millis"`
	NodeTag             string `json:"node_tag"`
	ProviderTag         string `json:"provider_tag,omitempty"`
}

type NodeLatency struct {
	DelayMS int `json:"delay_ms"`
}

func TestNodeLatency(ctx context.Context, options Options, runtimeConfig string, target NodeLatencyTarget) (result NodeLatency, err error) {
	if target.CoreStartedAtMillis <= 0 || strings.TrimSpace(target.NodeTag) == "" ||
		(target.ProviderTag != "" && !strings.HasPrefix(target.NodeTag, target.ProviderTag+"/")) {
		return result, policyError("invalid", "必须提供有效的节点标签及核心实例标识")
	}
	state, err := ReadRecordedState(options.StateFile)
	if err != nil {
		return result, err
	}
	if state.State != "ready" || state.CoreStartedAtMillis != target.CoreStartedAtMillis {
		return result, policyError("core_changed", "核心尚未就绪或已变化，请刷新后再试")
	}
	_, err = withPolicyCore(ctx, options, state, func(ctx context.Context, options Options, state RecordedState, client *serviceapi.Client) (PolicySnapshot, error) {
		result, err = queryNodeLatency(ctx, runtimeConfig, target)
		if err != nil {
			return PolicySnapshot{}, err
		}
		return PolicySnapshot{}, confirmPolicyCore(ctx, options, state, client)
	})
	return result, err
}

func queryNodeLatency(ctx context.Context, runtimeConfig string, target NodeLatencyTarget) (NodeLatency, error) {
	var options struct {
		Experimental struct {
			Clash struct {
				Controller string `json:"external_controller"`
				Secret     string `json:"secret"`
			} `json:"clash_api"`
		} `json:"experimental"`
	}
	content, err := os.ReadFile(runtimeConfig)
	if err != nil || json.Unmarshal(content, &options) != nil {
		return NodeLatency{}, policyError("api_failed", "无法读取核心测速接口配置")
	}
	clash := options.Experimental.Clash
	host, port, err := net.SplitHostPort(clash.Controller)
	n, portErr := strconv.ParseUint(port, 10, 16)
	if err != nil || portErr != nil || n == 0 || !net.ParseIP(host).IsLoopback() || clash.Secret == "" {
		return NodeLatency{}, policyError("api_failed", "节点测速需要启用带密钥的本机 Clash 接口")
	}
	rawPath := "/proxies/" + escapeLatencyTag(target.NodeTag) + "/delay"
	if target.ProviderTag != "" {
		rawPath = "/providers/proxies/" + escapeLatencyTag(target.ProviderTag) + "/" + escapeLatencyTag(target.NodeTag) + "/healthcheck"
	}
	path, _ := url.PathUnescape(rawPath)
	endpoint := url.URL{Scheme: "http", Host: clash.Controller, Path: path, RawPath: rawPath, RawQuery: "timeout=5000"}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return NodeLatency{}, policyError("api_failed", "无法访问核心节点测速接口")
	}
	request.Header.Set("Authorization", "Bearer "+clash.Secret)
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return NodeLatency{}, policyError("latency_timeout", "节点测速超时")
		}
		return NodeLatency{}, policyError("api_failed", "无法访问核心节点测速接口")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotFound:
		return NodeLatency{}, policyError("member_not_found", "核心中没有这个节点")
	case http.StatusGatewayTimeout:
		return NodeLatency{}, policyError("latency_timeout", "节点测速超时")
	case http.StatusOK:
	default:
		return NodeLatency{}, policyError("latency_failed", "节点测速失败")
	}
	var payload struct {
		Delay *int `json:"delay"`
	}
	if err := json.UnmarshalRead(io.LimitReader(response.Body, 4096), &payload); err != nil || payload.Delay == nil || *payload.Delay <= 0 {
		return NodeLatency{}, policyError("latency_failed", "核心未返回有效延迟")
	}
	return NodeLatency{DelayMS: *payload.Delay}, nil
}

func escapeLatencyTag(tag string) string {
	var escaped strings.Builder
	for _, b := range []byte(tag) {
		fmt.Fprintf(&escaped, "%%%02X", b)
	}
	return escaped.String()
}
