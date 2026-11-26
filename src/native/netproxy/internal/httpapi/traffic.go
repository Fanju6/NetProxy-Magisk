package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

const trafficRoute = "/api/v1/traffic/"

func (s *Server) registerTraffic() {
	s.mux.HandleFunc("GET "+trafficRoute+"body", s.readTrafficBody)
	s.trafficRoute("GET", "status", "抓包状态", func(context.Context) (any, error) {
		return s.traffic.Status(), nil
	})
	s.trafficRoute("GET", "preview", "实时抓包预览", func(context.Context) (any, error) {
		return s.traffic.Preview(), nil
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
	s.route("GET "+trafficRoute+"list", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		query, err := resourceQuery(r, "batch")
		if err != nil {
			return contract.Result{}, err
		}
		id := query.Get("batch")
		if query.Has("batch") && !traffic.ValidRecordID(id) {
			return contract.Result{}, invalid("抓包批次 ID 无效")
		}
		entries, ok := s.traffic.BatchEntries(id)
		if !ok {
			return contract.Result{}, captureBatchNotFound()
		}
		return contract.Success("traffic.list", "抓包记录", entries), nil
	})
	s.trafficRoute("GET", "batches", "抓包批次", func(context.Context) (any, error) {
		return s.traffic.Batches(), nil
	})
	s.route("GET "+trafficRoute+"domains", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		query, err := resourceQuery(r, "batch")
		if err != nil {
			return contract.Result{}, err
		}
		id := query.Get("batch")
		if query.Has("batch") && !traffic.ValidRecordID(id) {
			return contract.Result{}, invalid("抓包批次 ID 无效")
		}
		domains, ok := s.traffic.HistoryDomains(id)
		if !ok {
			return contract.Result{}, captureBatchNotFound()
		}
		return contract.Success("traffic.domains", "抓包域名统计", domains), nil
	})
	s.route("GET "+trafficRoute+"page", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		query, err := resourceQuery(r, "batch", "limit", "before", "after", "since", "host")
		if err != nil {
			return contract.Result{}, err
		}
		request := traffic.PageRequest{Batch: query.Get("batch"), Host: query.Get("host"), Limit: 50}
		if query.Has("batch") && !traffic.ValidRecordID(request.Batch) {
			return contract.Result{}, invalid("抓包批次 ID 无效")
		}
		if query.Has("limit") {
			request.Limit, err = strconv.Atoi(query.Get("limit"))
			if err != nil || request.Limit < 1 || request.Limit > traffic.MaxHistoryPage {
				return contract.Result{}, invalid("抓包分页大小无效")
			}
		}
		for _, key := range []string{"before", "after", "since"} {
			if !query.Has(key) {
				continue
			}
			cursor, err := strconv.ParseUint(query.Get(key), 10, 64)
			if err != nil || cursor == 0 || request.Before != 0 || request.Incremental {
				return contract.Result{}, invalid("抓包分页游标无效")
			}
			if key == "before" {
				request.Before = cursor
			} else {
				request.Since, request.Incremental = cursor, true
			}
		}
		page, ok := s.traffic.HistoryPage(request)
		if !ok {
			return contract.Result{}, captureBatchNotFound()
		}
		return contract.Success("traffic.page", "抓包记录分页", page), nil
	})
	s.route("DELETE "+trafficRoute+"batches", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		query, err := resourceQuery(r, "id")
		if err != nil {
			return contract.Result{}, err
		}
		id := query.Get("id")
		if !traffic.ValidRecordID(id) {
			return contract.Result{}, invalid("抓包批次 ID 无效")
		}
		found, err := s.traffic.DeleteBatch(id)
		if errors.Is(err, traffic.ErrActiveBatch) {
			return contract.Result{}, &contract.Error{Code: "traffic.batch_running", Message: err.Error(), HTTPStatus: http.StatusConflict}
		}
		if err != nil {
			return contract.Result{}, captureHistoryError()
		}
		if !found {
			return contract.Result{}, captureBatchNotFound()
		}
		return contract.Success("traffic.batch_deleted", "已删除抓包批次", nil), nil
	})
	s.trafficRoute("GET", "apps", "抓包应用配置", func(context.Context) (any, error) {
		policy, err := s.traffic.AppsPolicy()
		return policy, mitmAPIError("traffic.apps_unavailable", err)
	})
	s.route("PUT "+trafficRoute+"apps", 60*time.Second, false, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		if _, err := resourceQuery(r); err != nil {
			return contract.Result{}, err
		}
		var input traffic.AppsPolicy
		if err := decodeRequestLimit(r, &input, traffic.MaxAppsBytes); err != nil {
			return contract.Result{}, err
		}
		policy, err := s.setTrafficApps(ctx, input)
		return contract.Success("traffic.apps_saved", "抓包应用配置已保存", policy), err
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
	s.route("POST "+trafficRoute+"replay", 45*time.Second, true, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		if _, err := resourceQuery(r); err != nil {
			return contract.Result{}, err
		}
		var input traffic.ReplayInput
		if err := decodeRequestLimit(r, &input, 512<<10); err != nil {
			return contract.Result{}, err
		}
		id, err := s.traffic.Replay(ctx, input)
		if err != nil {
			code, status := "traffic.replay_response_failed", http.StatusBadGateway
			switch err {
			case traffic.ErrReplayCore:
				code, status = "traffic.replay_core_unavailable", http.StatusServiceUnavailable
			case traffic.ErrReplayRoute:
				code = "traffic.replay_route_failed"
			case traffic.ErrReplayRecord:
				code, status = "traffic.record_not_found", http.StatusNotFound
			case traffic.ErrReplayRequest:
				code, status = "traffic.replay_invalid", http.StatusBadRequest
			}
			return contract.Result{}, &contract.Error{Code: code, Message: err.Error(), HTTPStatus: status}
		}
		return contract.Success("traffic.replayed", "已发送重放请求", map[string]string{"id": id}), nil
	})
	s.trafficRoute("DELETE", "list", "已清空抓包记录", func(context.Context) (any, error) {
		if err := s.traffic.Clear(); err != nil {
			return nil, captureHistoryError()
		}
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
	s.route("DELETE "+trafficRoute+"ca/trust", 20*time.Second, false, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		if err := resourceEmptyRequest(r); err != nil {
			return contract.Result{}, err
		}
		s.trafficMu.Lock()
		defer s.trafficMu.Unlock()
		policy, err := s.traffic.MITMPolicy()
		if err != nil {
			return contract.Result{}, mitmAPIError("traffic.mitm_unavailable", err)
		}
		if policy.Enabled {
			return contract.Result{}, &contract.Error{Code: "traffic.user_trust_in_use", HTTPStatus: http.StatusConflict,
				Message: "请先关闭 HTTPS 解密，再移除个人抓包 CA"}
		}
		err = traffic.RemoveUserTrust(ctx, paths.New(s.options.ModuleDir))
		return contract.Success("traffic.user_trust_removed", "已移除模块安装的个人抓包 CA", nil), mitmAPIError("traffic.user_trust_incomplete", err)
	})
	s.trafficRoute("GET", "mitm", "HTTPS 抓包配置", func(context.Context) (any, error) {
		policy, err := s.traffic.MITMPolicy()
		return policy, mitmAPIError("traffic.mitm_unavailable", err)
	})
	s.trafficRoute("GET", "pinned", "证书固定跳过列表", func(context.Context) (any, error) {
		entries, err := s.traffic.PinnedBypasses()
		return entries, mitmAPIError("traffic.pinned_unavailable", err)
	})
	s.route("DELETE "+trafficRoute+"pinned", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		if err := resourceEmptyRequest(r); err != nil {
			return contract.Result{}, err
		}
		entries, err := s.traffic.ClearPinnedBypasses()
		return contract.Success("traffic.pinned_cleared", "已清除证书固定跳过列表", entries), mitmAPIError("traffic.pinned_clear_failed", err)
	})
	s.trafficRoute("GET", "alpn-bypasses", "ALPN 临时跳过列表", func(context.Context) (any, error) {
		entries, err := s.traffic.ALPNBypasses()
		return entries, mitmAPIError("traffic.alpn_bypasses_unavailable", err)
	})
	s.route("DELETE "+trafficRoute+"alpn-bypasses", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		if err := resourceEmptyRequest(r); err != nil {
			return contract.Result{}, err
		}
		entries, err := s.traffic.ClearALPNBypasses()
		return contract.Success("traffic.alpn_bypasses_cleared", "已清除 ALPN 临时跳过列表", entries), mitmAPIError("traffic.alpn_bypasses_clear_failed", err)
	})
	s.route("PUT "+trafficRoute+"mitm", 40*time.Second, false, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		if _, err := resourceQuery(r); err != nil {
			return contract.Result{}, err
		}
		var input traffic.MITMPolicy
		if err := decodeRequestLimit(r, &input, traffic.MaxMITMBytes); err != nil {
			return contract.Result{}, err
		}
		s.trafficMu.Lock()
		defer s.trafficMu.Unlock()
		policy, err := s.traffic.SetMITMPolicy(input)
		if err != nil {
			return contract.Result{}, mitmAPIError("traffic.mitm_failed", err)
		}
		layout := paths.New(s.options.ModuleDir)
		var userTrustErr error
		if policy.Enabled {
			userTrustErr = traffic.ReconcileUserTrust(ctx, layout)
		}
		if !policy.SystemTrust {
			if err := traffic.RemoveDisabledSystemTrust(ctx, layout); err != nil {
				if userTrustErr != nil {
					return contract.Result{}, &contract.Error{Code: "traffic.user_trust_incomplete", HTTPStatus: http.StatusConflict,
						Message: "配置已保存，但个人和系统 CA 状态未完全同步；请检查证书并重启设备", Data: policy}
				}
				return contract.Result{}, &contract.Error{Code: "traffic.trust_remove_incomplete", HTTPStatus: http.StatusConflict,
					Message: "配置已保存，但系统 CA 卸载未完成；请重启设备", Data: policy}
			}
		}
		if userTrustErr != nil {
			return contract.Result{}, &contract.Error{Code: "traffic.user_trust_incomplete", HTTPStatus: http.StatusConflict,
				Message: "配置已保存，但部分 Android 用户的个人 CA 未完成同步: " + userTrustErr.Error(), Data: policy}
		}
		return contract.Success("traffic.mitm_saved", "HTTPS 抓包配置已保存", policy), nil
	})
}

func (s *Server) readTrafficBody(w http.ResponseWriter, r *http.Request) {
	query, err := resourceQuery(r, "id", "side")
	id, side := query.Get("id"), query.Get("side")
	if err != nil || !traffic.ValidRecordID(id) || side != "request" && side != "response" {
		s.fail(w, r.Context(), invalid("抓包正文标识无效"))
		return
	}
	file, ok := s.traffic.OpenBody(id, side)
	if !ok {
		s.fail(w, r.Context(), &contract.Error{Code: "traffic.body_not_found", Message: "抓包正文已清除或未保存", HTTPStatus: http.StatusNotFound})
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="capture.bin"`)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
	http.ServeContent(w, r, "capture.bin", time.Time{}, file)
}

func captureBatchNotFound() error {
	return &contract.Error{Code: "traffic.batch_not_found", Message: "抓包批次已删除或过期", HTTPStatus: http.StatusNotFound}
}

func captureHistoryError() error {
	return &contract.Error{Code: "traffic.history_failed", Message: "无法更新本地抓包记录", HTTPStatus: http.StatusInternalServerError}
}

func (s *Server) setTrafficApps(ctx context.Context, input traffic.AppsPolicy) (traffic.AppsPolicy, error) {
	s.trafficMu.Lock()
	defer s.trafficMu.Unlock()
	previous, err := s.traffic.AppsPolicy()
	if err != nil {
		return previous, mitmAPIError("traffic.apps_failed", err)
	}
	reload := false
	if traffic.Enabled(paths.New(s.options.ModuleDir)) {
		status, err := service.ReadStatus(ctx, s.controlOptions())
		if err != nil || status.State != "ready" && status.State != "stopped" && status.State != "failed" {
			return previous, &contract.Error{Code: "traffic.service_busy", Message: "代理服务正在切换，暂不能修改抓包设置", HTTPStatus: http.StatusConflict}
		}
		reload = status.State == "ready"
	}
	policy, err := s.traffic.SetAppsPolicy(input)
	if err != nil {
		return policy, mitmAPIError("traffic.apps_failed", err)
	}
	if reload && (*previous.DisableQUIC != *policy.DisableQUIC || *policy.DisableQUIC &&
		(previous.Mode != policy.Mode || !slices.Equal(previous.ActiveApps(), policy.ActiveApps()))) {
		if _, err := moduleapp.ManageService(ctx, s.options, "reload"); err != nil {
			message := "核心重载失败，已还原抓包应用配置"
			if _, restoreErr := s.traffic.SetAppsPolicy(previous); restoreErr != nil {
				message = "核心重载失败且无法还原抓包应用配置"
			}
			return previous, &contract.Error{Code: "traffic.reload_failed", Message: message, HTTPStatus: http.StatusConflict}
		}
	}
	return policy, nil
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
	} else {
		policy, err := s.traffic.MITMPolicy()
		if err == nil && policy.SystemTrust {
			traffic.RestoreSystemTrust(ctx, layout, "capture-start")
		}
	}
	return s.traffic.Status(), nil
}
