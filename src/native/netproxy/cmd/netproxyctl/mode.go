package main

func (c *cli) mode(args []string) int {
	return c.runModuleCommand(args, "mode", runModuleMode)
}
