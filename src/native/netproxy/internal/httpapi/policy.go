package httpapi

import (
	"context"
	"net/http"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func (s *Server) readPolicyGroups(ctx context.Context, r *http.Request) (contract.Result, error) {
	query, err := resourceQuery(r, "source")
	if err != nil {
		return contract.Result{}, err
	}
	var groups service.PolicySnapshot
	switch {
	case !query.Has("source"):
		groups, err = module.ReadPolicyGroups(ctx, s.options)
	case query.Get("source") == "config":
		groups, err = service.ReadConfiguredPolicyGroups(ctx, s.controlOptions())
	default:
		return contract.Result{}, invalid("策略组来源无效")
	}
	return contract.Success("policy.groups", "策略组快照", groups), err
}

func (s *Server) selectPolicyGroup(ctx context.Context, r *http.Request) (contract.Result, error) {
	var input struct {
		CoreStartedAtMillis *int64 `json:"core_started_at_millis"`
		GroupTag            string `json:"group_tag"`
		MemberTag           string `json:"member_tag"`
	}
	if err := resourceDecode(r, &input); err != nil {
		return contract.Result{}, err
	}
	if input.CoreStartedAtMillis == nil {
		return contract.Result{}, &service.Error{Code: "policy.invalid", Message: "必须提供核心实例标识，停服时为 0"}
	}
	target := service.PolicyTarget{CoreStartedAtMillis: *input.CoreStartedAtMillis, GroupTag: input.GroupTag}
	groups, err := module.SelectPolicyGroup(ctx, s.options, target, input.MemberTag)
	return contract.Success("policy.selected", "策略组选择已确认", groups), err
}

func (s *Server) requestPolicyDelay(ctx context.Context, r *http.Request) (contract.Result, error) {
	var target service.PolicyTarget
	if err := resourceDecode(r, &target); err != nil {
		return contract.Result{}, err
	}
	err := module.RequestPolicyDelay(ctx, s.options, target)
	return contract.Success("policy.delay_requested", "核心已接受策略组测速请求", target), err
}
