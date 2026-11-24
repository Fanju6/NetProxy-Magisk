package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

const trafficRoute = "/api/v1/traffic/"

func (s *Server) registerTraffic() {
	s.trafficRoute("GET", "status", "抓包状态", func(context.Context) (any, error) {
		return s.traffic.Status(), nil
	})
	s.trafficRoute("POST", "start", "抓包代理已启动", func(ctx context.Context) (any, error) {
		if current := s.traffic.Status(); current.Running {
			return current, nil
		}
		status, err := service.ReadStatus(ctx, s.controlOptions())
		if err != nil || status.State != "ready" || status.ActiveBackend == nil {
			return nil, upstreamUnavailable()
		}
		layout := paths.New(s.options.ModuleDir)
		upstream, err := traffic.CoreUpstream(paths.RuntimeConfig(layout.Runtime()))
		if err != nil {
			return nil, upstreamUnavailable()
		}
		if err := s.traffic.Start(upstream); err != nil {
			return nil, &contract.Error{Code: "traffic.start_failed", Message: err.Error()}
		}
		return s.traffic.Status(), nil
	})
	s.trafficRoute("POST", "stop", "抓包代理已停止", func(context.Context) (any, error) {
		s.traffic.Stop()
		return s.traffic.Status(), nil
	})
	s.trafficRoute("GET", "access", "抓包代理凭据，仅供本机客户端使用", func(context.Context) (any, error) {
		data, err := s.traffic.Access()
		if err != nil {
			err = &contract.Error{Code: "traffic.not_running", Message: err.Error(), HTTPStatus: http.StatusConflict}
		}
		return data, err
	})
	s.trafficRoute("GET", "list", "抓包记录", func(context.Context) (any, error) {
		return s.traffic.Entries(), nil
	})
	s.trafficRoute("DELETE", "list", "已清空抓包记录", func(context.Context) (any, error) {
		s.traffic.Clear()
		return s.traffic.Status(), nil
	})
	s.trafficRoute("GET", "rules", "重写规则", func(context.Context) (any, error) {
		data, err := s.traffic.Rules()
		if err != nil {
			return nil, &contract.Error{Code: "traffic.rules_unavailable", Message: err.Error()}
		}
		return trafficRules{Rules: data}, nil
	})
	s.route("PUT "+trafficRoute+"rules", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		if _, err := resourceQuery(r); err != nil {
			return contract.Result{}, err
		}
		var input trafficRules
		if err := decodeRequestLimit(r, &input, traffic.MaxRulesBytes+1024); err != nil {
			return contract.Result{}, err
		}
		if input.Rules == nil {
			return contract.Result{}, invalid("缺少 rules 数组")
		}
		if err := s.traffic.ReplaceRules(input.Rules); err != nil {
			return contract.Result{}, &contract.Error{Code: "traffic.rules_failed", Message: err.Error(), HTTPStatus: http.StatusUnprocessableEntity}
		}
		return contract.Success("traffic.rules_saved", "重写规则已保存", len(input.Rules)), nil
	})
}

type trafficRules struct {
	Rules []traffic.Rule `json:"rules"`
}

func (s *Server) trafficRoute(method, action, message string, handle func(context.Context) (any, error)) {
	s.route(method+" "+trafficRoute+action, 10*time.Second, false, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		if err := resourceEmptyRequest(r); err != nil {
			return contract.Result{}, err
		}
		code := "traffic." + action
		if method == "DELETE" {
			code = "traffic.clear"
		}
		data, err := handle(ctx)
		return contract.Success(code, message, data), err
	})
}

func upstreamUnavailable() error {
	return &contract.Error{Code: "traffic.upstream_unavailable", Message: traffic.ErrNoUpstream.Error(), HTTPStatus: http.StatusConflict}
}
