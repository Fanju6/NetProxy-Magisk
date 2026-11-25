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
	s.route("GET "+trafficRoute+"detail", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		query, err := resourceQuery(r, "id")
		if err != nil {
			return contract.Result{}, err
		}
		id := query.Get("id")
		if !traffic.ValidRecordID(id) {
			return contract.Result{}, invalid("抓包记录 ID 无效")
		}
		record, ok := s.traffic.Record(id)
		if !ok {
			return contract.Result{}, &contract.Error{Code: "traffic.record_not_found", Message: "抓包记录已清除或过期", HTTPStatus: http.StatusNotFound}
		}
		return contract.Success("traffic.detail", "抓包详情", record), nil
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
	s.trafficRoute("GET", "ca", "公开抓包 CA", func(context.Context) (any, error) {
		cert, err := s.traffic.Certificate(false)
		return cert, mitmAPIError("traffic.ca_unavailable", err)
	})
	s.trafficRoute("POST", "ca-init", "抓包 CA 已初始化", func(context.Context) (any, error) {
		cert, err := s.traffic.Certificate(true)
		return cert, mitmAPIError("traffic.ca_init_failed", err)
	})
	s.trafficRoute("GET", "mitm", "HTTPS 抓包配置", func(context.Context) (any, error) {
		policy, err := s.traffic.MITMPolicy()
		return policy, mitmAPIError("traffic.mitm_unavailable", err)
	})
	s.route("PUT "+trafficRoute+"mitm", 25*time.Second, false, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		if _, err := resourceQuery(r); err != nil {
			return contract.Result{}, err
		}
		var input traffic.MITMPolicy
		if err := decodeRequestLimit(r, &input, traffic.MaxMITMBytes); err != nil {
			return contract.Result{}, err
		}
		policy, err := s.traffic.SetMITMPolicy(input)
		if err != nil {
			return contract.Result{}, mitmAPIError("traffic.mitm_failed", err)
		}
		if !policy.SystemTrust {
			if err := traffic.RemoveDisabledSystemTrust(ctx, paths.New(s.options.ModuleDir)); err != nil {
				return contract.Result{}, &contract.Error{Code: "traffic.trust_remove_incomplete", HTTPStatus: http.StatusConflict,
					Message: "配置已保存，但系统 CA 卸载未完成；请重启设备", Data: policy}
			}
		}
		return contract.Success("traffic.mitm_saved", "HTTPS 抓包配置已保存", policy), nil
	})
}

func mitmAPIError(code string, err error) error {
	if err == nil {
		return nil
	}
	return &contract.Error{Code: code, Message: err.Error(), HTTPStatus: http.StatusUnprocessableEntity}
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
