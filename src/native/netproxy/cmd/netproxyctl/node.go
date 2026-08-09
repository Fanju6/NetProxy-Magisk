package main

func (c *cli) node(args []string) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	action := args[0]
	positionals := args[1:]
	switch action {
	case "list":
		controlArgs := []string{"nodes", "--format", "json"}
		if len(positionals) > 0 && positionals[0] != "" {
			controlArgs = append(controlArgs, "--group", positionals[0])
		}
		return c.runNative(c.context(), c.controlArgs(controlArgs[0], controlArgs[1:]...)...)
	case "snapshot":
		controlArgs := []string{"snapshot", "--format", "json"}
		if len(positionals) > 0 && positionals[0] != "" {
			controlArgs = append(controlArgs, "--group", positionals[0])
		}
		return c.runNative(c.context(), c.controlArgs(controlArgs[0], controlArgs[1:]...)...)
	case "current":
		return c.runNative(c.context(), c.controlArgs("selection", "--format", "json")...)
	case "show":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node show <分组>", 2)
		}
		return c.runNative(c.context(), append([]string{"catalog", "show"}, c.catalogArgs("--group", positionals[0], "--format", "json")...)...)
	case "get", "export":
		group, tag, ok := splitReference(first(positionals))
		if !ok {
			return c.fail("node.ref_invalid", "节点引用格式应为 <group-id>/<tag>", 2)
		}
		operation := "node-get"
		if action == "export" {
			operation = "node-export"
		}
		return c.runNative(c.context(), append([]string{"catalog", operation}, c.catalogArgs("--group", group, "--tag", tag)...)...)
	case "add":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node add <节点链接> [分组]", 2)
		}
		return c.runNative(c.context(), c.moduleArgs("node", append([]string{"add"}, positionals...)...)...)
	case "import":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node import <文件> [名称]", 2)
		}
		input := []string{"import", positionals[0]}
		if len(positionals) > 1 {
			input = []string{"import", "--name", positionals[1], positionals[0]}
		}
		return c.runNative(c.context(), c.moduleArgs("node", input...)...)
	case "edit":
		if len(positionals) < 2 {
			return c.fail("usage.invalid", "用法: netproxyctl node edit <分组/tag> <节点链接|文件>", 2)
		}
		return c.runNative(c.context(), c.moduleArgs("node", "edit", positionals[0], positionals[1])...)
	case "remove", "rm":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node remove <分组/tag>", 2)
		}
		return c.runNative(c.context(), c.moduleArgs("node", "remove", positionals[0])...)
	case "use":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node use <分组/tag|auto> [分组]", 2)
		}
		return c.runNative(c.context(), c.moduleArgs("select", positionals...)...)
	case "delay":
		controlArgs := []string{"delay", "--format", "json"}
		if len(positionals) > 0 && positionals[0] != "" {
			controlArgs = append(controlArgs, "--target", positionals[0])
		}
		if len(positionals) > 1 && positionals[1] != "" {
			controlArgs = append(controlArgs, "--group", positionals[1])
		}
		return c.runNative(c.context(), c.controlArgs(controlArgs[0], controlArgs[1:]...)...)
	default:
		return c.fail("usage.invalid", "用法: netproxyctl node list|snapshot|current|show|get|export|delay|add|import|edit|remove|use", 2)
	}
}
