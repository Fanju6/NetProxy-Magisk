package main

func (c *cli) mode(args []string) int {
	return c.forwardModule(args, "mode")
}
