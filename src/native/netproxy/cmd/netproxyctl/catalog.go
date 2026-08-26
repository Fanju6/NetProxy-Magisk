package main

import (
	"strings"
)

func (c *cli) catalog(args []string) int {
	action := "list"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "list":
		return c.runCommand(c.context(), runCatalog, append([]string{"groups"}, c.catalogArgs("--type", "all", "--format", "json")...)...)
	case "show":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return c.fail("usage.invalid", "用法: netproxyctl catalog show <分组>", 2)
		}
		return c.runCommand(c.context(), runCatalog, append([]string{"show"}, c.catalogArgs("--group", args[1], "--format", "json")...)...)
	default:
		return c.fail("usage.invalid", "用法: netproxyctl catalog list|show", 2)
	}
}
