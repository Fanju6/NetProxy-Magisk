package main

func (c *cli) subscription(args []string) int {
	return c.runModuleCommand(args, "sub", runModuleSub)
}
