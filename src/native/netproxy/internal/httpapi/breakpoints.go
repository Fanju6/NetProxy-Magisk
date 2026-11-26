package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

const breakpointEditLimit = 2 << 20

func (s *Server) registerBreakpointRoutes() {
	s.trafficRoute("GET", "breakpoints", "断点配置", func(context.Context) (any, error) {
		return s.traffic.Breakpoints()
	})
	s.route("PUT "+trafficRoute+"breakpoints", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		if _, err := resourceQuery(r); err != nil {
			return contract.Result{}, err
		}
		var input traffic.BreakpointConfig
		if err := decodeRequestLimit(r, &input, traffic.MaxRulesBytes+1024); err != nil {
			return contract.Result{}, err
		}
		if input.Rules == nil {
			return contract.Result{}, invalid("缺少 rules 数组")
		}
		if err := s.traffic.ReplaceBreakpoints(input); err != nil {
			return contract.Result{}, &contract.Error{Code: "traffic.breakpoints_invalid", Message: err.Error(), HTTPStatus: http.StatusUnprocessableEntity}
		}
		saved, err := s.traffic.Breakpoints()
		return contract.Success("traffic.breakpoints_saved", "断点配置已保存", saved), err
	})
	s.route("GET "+trafficRoute+"paused", 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
		query, err := resourceQuery(r, "id")
		if err != nil {
			return contract.Result{}, err
		}
		if !query.Has("id") {
			paused, _ := s.traffic.Paused()
			return contract.Success("traffic.paused", "待处理断点", paused), nil
		}
		id := query.Get("id")
		if !traffic.ValidRecordID(id) {
			return contract.Result{}, invalid("断点 ID 无效")
		}
		paused, ok := s.traffic.PausedDetail(id)
		if !ok {
			return contract.Result{}, breakpointError(traffic.ErrBreakpointGone)
		}
		return contract.Success("traffic.paused_detail", "断点详情", paused), nil
	})
	for _, action := range []string{"resume", "abort"} {
		s.route("POST "+trafficRoute+action, 10*time.Second, false, func(_ context.Context, r *http.Request) (contract.Result, error) {
			query, err := resourceQuery(r, "id")
			if err != nil {
				return contract.Result{}, err
			}
			id := query.Get("id")
			if !traffic.ValidRecordID(id) {
				return contract.Result{}, invalid("断点 ID 无效")
			}
			var edit *traffic.BreakpointEdit
			if action == "resume" && r.ContentLength != 0 {
				var input traffic.BreakpointEdit
				if err := decodeRequestLimit(r, &input, breakpointEditLimit); err != nil {
					return contract.Result{}, err
				}
				if input.Method != nil || input.URL != nil || input.Status != nil || input.Headers != nil || input.Body != nil {
					edit = &input
				}
			}
			if err := s.traffic.ResolveBreakpoint(id, edit, action == "abort"); err != nil {
				return contract.Result{}, breakpointError(err)
			}
			return contract.Success("traffic.breakpoint_"+action, "断点操作已接受", nil), nil
		})
	}
}

func breakpointError(err error) error {
	if err == traffic.ErrBreakpointGone {
		return &contract.Error{Code: "traffic.breakpoint_gone", Message: err.Error(), HTTPStatus: http.StatusNotFound}
	}
	return &contract.Error{Code: "traffic.breakpoint_edit_invalid", Message: err.Error(), HTTPStatus: http.StatusUnprocessableEntity}
}
