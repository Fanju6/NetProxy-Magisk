package main

func (c *cli) logs(args []string) int {
	return c.runModuleCommand(args, "logs", runModuleLogs)
}
