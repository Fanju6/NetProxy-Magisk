package main

import (
	"flag"
	"io"
)

type resultError struct {
	Code    string
	Message string
	Data    any
}

func (e *resultError) Error() string { return e.Message }

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func internalUsageText() string {
	return `netproxyctl __internal - NetProxy 模块内部入口

用法：
  netproxyctl __internal boot --module-dir <模块目录>
  netproxyctl __internal worker <start|stop|run> --module-dir <模块目录>
`
}
