package main

func (c *cli) config(args []string) int {
	return c.runModuleCommand(args, "config", runModuleConfig)
}
