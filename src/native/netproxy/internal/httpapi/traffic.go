package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

const trafficRoute = "/api/v1/traffic/"

func (s *Server) registerTraffic() {
	s.trafficRoute("GET", "status", "抓包状态", func(context.Context) (any, error) {
		return s.traffic.Status(), nil
	})
	for _, action := range []string{"start", "stop", "toggle"} {
		s.route("POST "+trafficRoute+action, 60*time.Second, false, func(ctx context.Context, r *http.Request) (contract.Result, error) {
			if err := resourceEmptyRequest(r); err != nil {
				return contract.Result{}, err
			}
			status, err := s.setTraffic(ctx, action)
			return contract.Success("traffic."+action, "全局抓包设置已更新", status), err
		})
	}
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

func (s *Server) setTraffic(ctx context.Context, action string) (traffic.Status, error) {
	s.trafficMu.Lock()
	defer s.trafficMu.Unlock()
	layout := paths.New(s.options.ModuleDir)
	previous := traffic.Enabled(layout)
	enable := action == "start" || action == "toggle" && !previous
	failure := func(code, message string) (traffic.Status, error) {
		return s.traffic.Status(), &contract.Error{Code: code, Message: message, HTTPStatus: http.StatusConflict}
	}
	status, err := service.ReadStatus(ctx, s.controlOptions())
	if err != nil || status.State != "ready" && status.State != "stopped" && status.State != "failed" {
		return failure("traffic.service_busy", "代理服务正在切换，暂不能修改抓包设置")
	}
	if enable {
		if err := s.traffic.Start(); err != nil {
			return failure("traffic.start_failed", err.Error())
		}
	}
	if err := traffic.SaveEnabled(layout, enable); err != nil {
		if !previous {
			s.traffic.Stop()
		}
		return failure("traffic.save_failed", "无法保存全局抓包设置")
	}
	if previous != enable && status.State == "ready" {
		if _, err := moduleapp.ManageService(ctx, s.options, "reload"); err != nil {
			if err := traffic.SaveEnabled(layout, previous); err != nil {
				return failure("traffic.reload_failed", "核心重载失败且无法还原抓包设置")
			}
			if !previous {
				s.traffic.Stop()
			}
			return failure("traffic.reload_failed", "核心重载失败，已还原抓包设置")
		}
	}
	if !enable {
		s.traffic.Stop()
	}
	return s.traffic.Status(), nil
}
