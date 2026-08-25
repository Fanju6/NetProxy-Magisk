package main

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWriteJSONPreservesEmptyDataObject(t *testing.T) {
	var output bytes.Buffer
	writeJSON(&output, result{Schema: 1, OK: false, Code: "test.failed", Message: "测试失败", Data: map[string]any{}})
	if !strings.Contains(output.String(), `"data":{}`) {
		t.Fatalf("schema=1 空 data 对象丢失: %s", output.String())
	}
}

func TestCommandContextConsumesForwardedDeadline(t *testing.T) {
	want := time.Now().Add(time.Minute).Truncate(time.Millisecond)
	t.Setenv("NETPROXY_COMMAND_DEADLINE_MILLIS", strconv.FormatInt(want.UnixMilli(), 10))
	ctx, cancel := commandContext(context.Background())
	defer cancel()
	got, ok := ctx.Deadline()
	if !ok || !got.Equal(want) {
		t.Fatalf("command deadline = %s, %v, want %s", got, ok, want)
	}
	if value := os.Getenv("NETPROXY_COMMAND_DEADLINE_MILLIS"); value != "" {
		t.Fatalf("forwarded deadline leaked to child processes: %q", value)
	}
}

func TestUsageListsCurrentModuleActions(t *testing.T) {
	usage := usageText("netproxy-native")
	want := "module <boot|prepare|select|mode|network|app|node|sub|config|logs|service>"
	if !strings.Contains(usage, want) {
		t.Fatalf("usage missing current module actions: %s", usage)
	}
	for _, removed := range []string{"|sync|", "|state|"} {
		if strings.Contains(usage, removed) {
			t.Fatalf("usage still lists removed action %q: %s", removed, usage)
		}
	}
}
