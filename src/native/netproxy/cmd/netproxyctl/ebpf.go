package main

func (c *cli) ebpf(args []string) int {
	action := "diagnose"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "diagnose", "validate", "status":
		return c.runNative(c.context(), "ebpf", "diagnose", "--config", c.ebpfConfig, "--format", "json")
	default:
		return c.fail("usage.invalid", "用法: netproxyctl ebpf status|diagnose|validate", 2)
	}
}
