package main

func (c *cli) app(args []string) int {
	return c.forwardModule(args, "app")
}
