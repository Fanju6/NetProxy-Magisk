package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

func TestWriteJSONPreservesEmptyDataObject(t *testing.T) {
	var output bytes.Buffer
	writeJSON(&output, result{Schema: 1, OK: false, Code: "test.failed", Message: "测试失败", Data: map[string]any{}})
	if !strings.Contains(output.String(), `"data":{}`) {
		t.Fatalf("schema=1 空 data 对象丢失: %s", output.String())
	}
}

func TestActivityExcludesHelpAndInstallationChecks(t *testing.T) {
	for _, args := range [][]string{{"service", "check"}, {"config", "validate"}, {"config", "check"}, {"sub", "--help"}} {
		if !skipActivity(args) {
			t.Fatalf("帮助或安装检查被计为活跃: %v", args)
		}
	}
	for _, args := range [][]string{{"service", "status"}, {"service", "stop"}, {"node", "list"}, {"config", "read", "module"}} {
		if skipActivity(args) {
			t.Fatalf("有效客户端操作未被计为活跃: %v", args)
		}
	}
}

func TestBareEBPFUsesConfiguredStatus(t *testing.T) {
	command := &cli{options: moduleapp.NewOptions(t.TempDir())}
	implicit := command.ebpf(context.Background(), nil)
	explicit := command.ebpf(context.Background(), []string{"status", "configured"})
	if implicit == nil || explicit == nil || implicit.Error() != explicit.Error() {
		t.Fatalf("默认诊断行为不一致: %v / %v", implicit, explicit)
	}
}

func TestNodeImportRejectsExtraGroupBeforeReadingFile(t *testing.T) {
	command := &cli{options: moduleapp.NewOptions(t.TempDir())}
	err := command.node(context.Background(), []string{"import", "nodes.yaml", "custom"})
	structured, ok := errors.AsType[*resultError](err)
	if !ok || structured.Code != "usage.invalid" {
		t.Fatalf("导入参数契约错误: %v", err)
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

func TestDefaultTimeoutDoesNotPreemptSubscriptionMutation(t *testing.T) {
	for _, args := range [][]string{
		{"sub", "add"}, {"sub", "edit"}, {"sub", "update"}, {"sub", "update-all"},
	} {
		if got := defaultTimeoutFor(args); got != 0 {
			t.Fatalf("default timeout for %v = %s, want no outer deadline", args, got)
		}
	}
	if got := defaultTimeoutFor([]string{"sub", "list"}); got != defaultCommandTimeout {
		t.Fatalf("sub list timeout = %s, want %s", got, defaultCommandTimeout)
	}
	if got := defaultTimeoutFor([]string{"service", "start"}); got != serviceStartTimeout {
		t.Fatalf("service start timeout = %s, want %s", got, serviceStartTimeout)
	}
}

func TestInternalUsageOnlyListsProcessEntrypoints(t *testing.T) {
	usage := internalUsageText()
	for _, expected := range []string{"__internal boot", "__internal worker <start|stop|run>"} {
		if !strings.Contains(usage, expected) {
			t.Fatalf("usage missing process entry %q: %s", expected, usage)
		}
	}
	for _, removed := range []string{"__internal module", "__internal catalog", "__internal control", "__internal ebpf", "__internal version", "netproxy-native"} {
		if strings.Contains(usage, removed) {
			t.Fatalf("usage still lists removed entry %q: %s", removed, usage)
		}
	}
}
