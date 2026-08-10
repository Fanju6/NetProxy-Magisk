package main

import (
	"reflect"
	"testing"
	"time"
)

func TestModuleArgsKeepsOperationBeforeFlags(t *testing.T) {
	command := &cli{
		moduleDir: "/module",
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

func TestParseCommandArgsSupportsMixedOptions(t *testing.T) {
	args, timeout, err := parseCommandArgs([]string{
		"service", "status", "--json", "--timeout=45s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"service", "status"}) {
		t.Fatalf("clean args = %v", args)
	}
	if timeout != 45*time.Second {
		t.Fatalf("timeout = %s, want 45s", timeout)
	}
}

func TestParseCommandTimeoutAcceptsSecondsAndDurations(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"30": 30 * time.Second,
		"2m": 2 * time.Minute,
	} {
		got, err := parseCommandTimeout(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if got != want {
			t.Fatalf("parse %q = %s, want %s", value, got, want)
		}
	}
	if _, err := parseCommandTimeout("0"); err == nil {
		t.Fatal("zero timeout should fail")
	}
}
