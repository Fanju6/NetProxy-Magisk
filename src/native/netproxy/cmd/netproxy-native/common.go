package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
)

type result struct {
	Schema  int    `json:"schema"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type resultError struct {
	Code    string
	Message string
	Data    any
}

func (e *resultError) Error() string {
	return e.Message
}

func writeJSON(writer io.Writer, value any) {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func dependencyVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dependency := range info.Deps {
		if dependency.Path != path {
			continue
		}
		if dependency.Replace != nil && dependency.Replace.Version != "" {
			return dependency.Replace.Version
		}
		return dependency.Version
	}
	return "unknown"
}

func showUsage() {
	executable := filepath.Base(os.Args[0])
	fmt.Print(usageText(executable))
}

func usageText(executable string) string {
	return fmt.Sprintf(`%s - NetProxy 原生组件

用法：
  %s catalog <操作> ...
  %s control <操作> ...
  %s ebpf <runtime|status> ...
  netproxy-native module <boot|prepare|select|mode|network|app|node|sub|config|logs|service> ...
  netproxy-native module node import <文件>
  %s worker <start|stop|run> --module-dir <模块目录>
  netproxy-native version
`, executable, executable, executable, executable, executable)
}
