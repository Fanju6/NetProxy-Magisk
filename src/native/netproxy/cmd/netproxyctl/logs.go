package main

func (c *cli) logs(args []string) int {
	return c.forwardModule(args, "logs")
}
