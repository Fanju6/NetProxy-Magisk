package main

func (c *cli) subscription(args []string) int {
	return c.forwardModule(args, "sub")
}
