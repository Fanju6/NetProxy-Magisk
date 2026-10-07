package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	switch os.Getenv("NETPROXY_TEST_COMMAND_MODE") {
	case "packages":
		if len(os.Args) != 7 || strings.Join(os.Args[1:5], " ") != "package list packages --user" || os.Args[6] != "-U" {
			panic("invalid package arguments")
		}
		user, err := strconv.ParseUint(os.Args[5], 10, 32)
		if err != nil {
			panic(err)
		}
		if path := os.Getenv("NETPROXY_TEST_COMMAND_LOG"); path != "" {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				panic(err)
			}
			fmt.Fprintln(file, user)
			if err := file.Close(); err != nil {
				panic(err)
			}
		}
		fmt.Printf("package:com.example.app uid:%d\npackage:com.example.shared uid:%d\n", user*100000+10123, user*100000+10123)
		return
	case "probe":
		if path := os.Getenv("NETPROXY_TEST_COMMAND_LOG"); path != "" {
			if err := os.WriteFile(path, []byte(strings.Join(os.Args[1:], "\n")), 0o600); err != nil {
				panic(err)
			}
		}
		output := os.Getenv("NETPROXY_TEST_PROBE_REPORT")
		if output == "" {
			output = `{"mode":"local","local_data_plane":"cgroup","preflight":true,"exact_object_load":true,"result":"preflight_passed"}`
		}
		fmt.Print(output)
		if os.Getenv("NETPROXY_TEST_PROBE_FAIL") == "1" {
			fmt.Fprintln(os.Stderr, "capability probe failed")
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(os.Getenv("NETPROXY_PACKAGE_STARTED"), []byte("started"), 0o600); err != nil {
		panic(err)
	}
	time.Sleep(time.Minute)
}
