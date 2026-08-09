package main

func (c *cli) config(args []string) int {
	return c.forwardModule(args, "config")
}
