package main

import (
	"context"
	"os"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func (c *cli) policy(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"groups"}
	}
	var (
		snapshot service.PolicySnapshot
		err      error
		code     string
	)
	switch args[0] {
	case "groups":
		if len(args) != 1 {
			return usageError("policy groups")
		}
		snapshot, err = moduleapp.ReadPolicyGroups(ctx, c.options)
		code = "policy.groups"
	case "select":
		if len(args) != 3 || args[1] == "" || args[2] == "" {
			return usageError("policy select <group> <member>")
		}
		snapshot, err = moduleapp.SelectPolicyGroup(ctx, c.options, args[1], args[2])
		code = "policy.selected"
	case "delay":
		if len(args) != 2 || args[1] == "" {
			return usageError("policy delay <group>")
		}
		snapshot, err = moduleapp.DelayPolicyGroup(ctx, c.options, args[1])
		code = "policy.delay"
	default:
		return usageError("policy groups|select <group> <member>|delay <group>")
	}
	if err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: "Policy snapshot", Data: snapshot})
	return nil
}
