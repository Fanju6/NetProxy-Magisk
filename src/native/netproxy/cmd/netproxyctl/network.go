package main

func (c *cli) network(args []string) int {
	return c.forwardModule(args, "network")
}
