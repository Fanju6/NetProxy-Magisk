package module

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestBootCompletedWaitCommandWaitsForPropertyToLeaveZero(t *testing.T) {
	command := newBootCompletedWaitCommand(context.Background())
	want := []string{"resetprop", "-w", "sys.boot_completed", "0"}
	if !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("boot wait command args = %v, want %v", command.Args, want)
	}
}

func TestCoreLogCommandCapturesOutputAfterParentHandleCloses(t *testing.T) {
	if os.Getenv("NETPROXY_TEST_CORE_LOG_CHILD") == "1" {
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(os.Getenv("NETPROXY_TEST_CORE_LOG_SIGNAL")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				os.Exit(3)
			}
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Println("fixture stdout")
		fmt.Fprintln(os.Stderr, "FATAL[0000] startup: fixture error")
		panic("fixture panic")
	}
	options := NewOptions(t.TempDir())
	options.SingBoxPath = os.Args[0]
	options.SingBoxDir = options.ModuleDir
	path, err := LogFile(options, "core")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.LogDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("previous core\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared := PrepareResult{Providers: "providers.json", Outbounds: "outbounds.json", Inbound: "inbound.json"}
	command, file, err := newSingBoxCommand(options, prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	want := []string{os.Args[0], "run", "--disable-color", "-c", paths.SingBoxConfig(options.SingBoxDir), "-c", prepared.Providers, "-c", prepared.Outbounds, "-c", prepared.Inbound}
	if !slices.Equal(command.Args, want) || command.Stdout != file || command.Stderr != file {
		t.Fatalf("核心参数或继承句柄异常: %v", command.Args)
	}
	signal := filepath.Join(options.ModuleDir, "continue")
	command.Args = []string{os.Args[0], "-test.run=^TestCoreLogCommandCapturesOutputAfterParentHandleCloses$"}
	command.Env = append(os.Environ(), "NETPROXY_TEST_CORE_LOG_CHILD=1", "NETPROXY_TEST_CORE_LOG_SIGNAL="+signal)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signal, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("异常核心返回成功")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"previous core", "fixture stdout", "FATAL[0000] startup: fixture error", "panic: fixture panic"} {
		if strings.Count(string(content), marker) != 1 {
			t.Fatalf("核心输出丢失或重复 %q: %s", marker, content)
		}
	}
}
