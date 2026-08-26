package main

func (c *cli) ebpf(args []string) int {
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "status":
		mode := "configured"
		raw := false
		for _, argument := range args[1:] {
			switch argument {
			case "configured", "all", "local", "shared":
				mode = argument
			case "--raw":
				raw = true
			default:
				return c.fail("usage.invalid", "用法: netproxyctl ebpf status [configured|all|local|shared] [--raw]", 2)
			}
		}
		handlerArgs := []string{
			"status",
			"--module-dir", c.moduleDir,
			"--mode", mode,
			"--format", "json",
		}
		if raw {
			handlerArgs = append(handlerArgs, "--raw")
		}
		return c.runCommand(c.context(), runEBPF, handlerArgs...)
	default:
		return c.fail("usage.invalid", "用法: netproxyctl ebpf status [configured|all|local|shared] [--raw]", 2)
	}
}
