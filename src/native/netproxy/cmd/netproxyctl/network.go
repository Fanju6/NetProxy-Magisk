package main

func (c *cli) network(args []string) int {
	return c.runModuleCommand(args, "network", runModuleNetwork)
}
