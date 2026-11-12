package main

import (
	"context"
	"os"
	"strings"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func (c *cli) policy(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"groups"}
	}
	action := args[0]
	var operands int
	switch action {
	case "groups":
	case "select":
		operands = 2
	case "delay":
		operands = 1
	default:
		return usageError("policy groups | select <group> <member> | delay <group>")
	}
	if len(args) != operands+1 {
		return usageError("policy groups | select <group> <member> | delay <group>")
	}
	for _, arg := range args[1:] {
		if strings.TrimSpace(arg) == "" {
			return usageError("策略组或成员标签不能为空")
		}
	}
	if action == "groups" {
		snapshot, err := moduleapp.ReadPolicyGroups(ctx, c.options)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "policy.groups", Message: "策略组快照", Data: snapshot})
		return nil
	}
	state, err := moduleapp.ReadServiceState(c.options.StateFile)
	if err != nil {
		return err
	}
	if state.State != "ready" || state.CoreStartedAtMillis <= 0 {
		return &service.Error{Code: "policy.core_changed", Message: "核心尚未就绪，请启动后再试"}
	}
	target := service.PolicyTarget{CoreStartedAtMillis: state.CoreStartedAtMillis, GroupTag: args[1]}
	if action == "select" {
		confirmed, err := moduleapp.SelectPolicyGroup(ctx, c.options, target, args[2])
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "policy.selected", Message: "核心已确认选择", Data: confirmed})
		return nil
	}
	if err := moduleapp.RequestPolicyDelay(ctx, c.options, target); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "policy.delay_requested", Message: "测速已请求，请重新查看策略组获取结果"})
	return nil
}
