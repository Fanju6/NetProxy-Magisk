package main

import (
	"reflect"
	"testing"
)

func TestModuleArgsKeepsOperationBeforeFlags(t *testing.T) {
	command := &cli{
		moduleDir:      "/module",
		catalogRoot:    "/module/config/catalog",
		moduleConfig:   "/module/config/module.conf",
		ebpfConfig:     "/module/config/ebpf/ebpf.conf",
		singBoxPath:    "/module/bin/sing-box",
		singBoxDir:     "/module/config/singbox",
		serviceScript:  "/module/scripts/core/service.sh",
		serviceAddress: "127.0.0.1:9090",
		serviceSecret:  "singbox",
		logDir:         "/module/logs",
		stateFile:      "/module/config/runtime/service.json",
		progressDir:    "/dev/netproxy/subscriptions",
		workerPIDFile:  "/dev/netproxy/subworker.pid",
	}

	got := command.moduleArgs("node", "add", "socks://example.com:1080#node")
	wantPrefix := []string{"module", "node", "add"}
	if !reflect.DeepEqual(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("operation prefix = %v, want %v", got[:len(wantPrefix)], wantPrefix)
	}
	if got[len(got)-1] != "socks://example.com:1080#node" {
		t.Fatalf("node argument = %q", got[len(got)-1])
	}

	got = command.moduleArgs("mode", "AllowAds")
	if got[0] != "module" || got[1] != "mode" || got[2] == "AllowAds" {
		t.Fatalf("mode arguments were not placed after flags: %v", got)
	}
	if got[len(got)-1] != "AllowAds" {
		t.Fatalf("mode argument = %q", got[len(got)-1])
	}

	got = command.moduleArgs("network", "evaluate", "--type", "wifi", "--ssid", "办公 WiFi")
	if !reflect.DeepEqual(got[:3], []string{"module", "network", "evaluate"}) {
		t.Fatalf("network operation prefix = %v", got[:3])
	}
	if got[len(got)-1] != "办公 WiFi" {
		t.Fatalf("network SSID argument = %q", got[len(got)-1])
	}
}
