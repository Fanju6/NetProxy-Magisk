package main

func (c *cli) app(args []string) int {
	return c.runModuleCommand(args, "app", runModuleApp)
}
