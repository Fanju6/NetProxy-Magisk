package inbound

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPackageQueryPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := listPackageUIDs(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消变为其他错误: %v", err)
	}
}

func TestPackageQueryStopsRunningCommand(t *testing.T) {
	directory := t.TempDir()
	buildFakeCommand(t, directory)
	started := filepath.Join(directory, "started")
	t.Setenv("NETPROXY_PACKAGE_STARTED", started)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := listPackageUIDs(ctx, 0); done <- err }()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("查询未开始: %v", err)
		case <-deadline:
			t.Fatal("查询未启动")
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("取消丢失: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("查询取消未停止进程")
	}
}

func buildFakeCommand(t *testing.T, directory string) string {
	t.Helper()
	binary := filepath.Join(directory, "cmd")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./testdata/fake-package")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建查询桩: %v %s", err, output)
	}
	return binary
}

func TestCommandContextQueriesEachUserOnceAndReportsMissing(t *testing.T) {
	directory := t.TempDir()
	buildFakeCommand(t, directory)
	logPath := filepath.Join(directory, "queries")
	t.Setenv("NETPROXY_TEST_COMMAND_MODE", "packages")
	t.Setenv("NETPROXY_TEST_COMMAND_LOG", logPath)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, backend := range []string{"ebpf", "tun"} {
		config := fixture(t, backend, "", "")
		config.App = AppPolicy{Enabled: true, Mode: "whitelist", ProxyApps: []string{"10:com.example.app", "0:com.example.app", "10:com.example.shared", "0:com.example.app", "10:com.example.missing"}}
		built, err := config.Build(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		include, ranges, _, _ := builtFilters(t, built)
		if !reflect.DeepEqual(include, []uint32{10123, 1010123}) || len(ranges) != 0 {
			t.Fatalf("多用户或共享 UID 投影错误: %v %v", include, ranges)
		}
		if !reflect.DeepEqual(built.MissingPackages, []PackageRef{{UserID: 10, Package: "com.example.missing"}}) {
			t.Fatal(built)
		}
	}
	log, err := os.ReadFile(logPath)
	if err != nil || string(log) != "10\n0\n10\n0\n" {
		t.Fatalf("不是每次按用户一次查询: %q %v", log, err)
	}
	config := fixture(t, "tun", "", "")
	config.App = AppPolicy{Enabled: true, Mode: "whitelist", ProxyApps: []string{"10:com.example.app", "10:com.example.missing"}}
	missing, err := WriteAtomic(t.Context(), filepath.Join(directory, "runtime.json"), config)
	if err != nil || !reflect.DeepEqual(missing, []PackageRef{{UserID: 10, Package: "com.example.missing"}}) {
		t.Fatal(missing, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ResolvePackageUIDs(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("空名单查询吞掉取消", err)
	}
}

func TestParsePackageUIDsAcceptsPackageRowsAndIgnoresOtherOutput(t *testing.T) {
	got, err := parsePackageUIDs(strings.Join([]string{
		"Packages:",
		"package:com.example.first uid:10123",
		"package:com.example.second uid:10124 versionCode:2",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]uint32{
		"com.example.first":  10123,
		"com.example.second": 10124,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed package UIDs = %#v, want %#v", got, want)
	}
}

func TestParsePackageUIDsRejectsMalformedPackageRows(t *testing.T) {
	for _, output := range []string{
		"package:com.example.app",
		"package:com.example.app uid:not-a-number",
		"package: uid:10123",
		"package:com.example.app uid=10123",
	} {
		_, err := parsePackageUIDs(output)
		var validation *ValidationError
		if !errors.As(err, &validation) || len(validation.Diagnostics) != 1 || validation.Diagnostics[0].Code != "inbound.package_list_invalid" {
			t.Fatalf("malformed package output %q returned unexpected error: %v", output, err)
		}
	}
}

func TestResolvePackageUIDsPreservesMissingOrderAndCachesUsers(t *testing.T) {
	refs := []PackageRef{
		{UserID: 10, Package: "com.example.missing-ten"},
		{UserID: 0, Package: "com.example.installed-zero"},
		{UserID: 10, Package: "com.example.installed-ten"},
		{UserID: 0, Package: "com.example.missing-zero"},
	}
	lists := map[uint32]map[string]uint32{
		0:  {"com.example.installed-zero": 10123},
		10: {"com.example.installed-ten": 20123},
	}
	calls := make(map[uint32]int)
	resolution, err := resolvePackageUIDs(refs, func(userID uint32) (map[string]uint32, error) {
		calls[userID]++
		return lists[userID], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolution.UIDs, []uint32{10123, 20123}) {
		t.Fatalf("resolved UID order = %#v", resolution.UIDs)
	}
	wantMissing := []PackageRef{refs[0], refs[3]}
	if !reflect.DeepEqual(resolution.Missing, wantMissing) {
		t.Fatalf("missing package order = %#v, want %#v", resolution.Missing, wantMissing)
	}
	if !reflect.DeepEqual(calls, map[uint32]int{0: 1, 10: 1}) {
		t.Fatalf("package list calls = %#v", calls)
	}
}

func TestResolvePackageUIDsPropagatesPackageListFailure(t *testing.T) {
	expected := errors.New("package service unavailable")
	_, err := resolvePackageUIDs([]PackageRef{{UserID: 0, Package: "com.example.app"}}, func(uint32) (map[string]uint32, error) {
		return nil, expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("package list error was not propagated: %v", err)
	}
}

func TestParsePackageUIDCommandResultTreatsStderrAsFailure(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		stderr      string
		commandErr  error
		wantCode    string
		wantMessage string
	}{
		{
			name:        "successful command with error stderr",
			stderr:      "Error: unknown user",
			wantCode:    "inbound.package_list_failed",
			wantMessage: "Error: unknown user",
		},
		{
			name:        "non zero command",
			stderr:      "permission denied",
			commandErr:  errors.New("exit status 1"),
			wantCode:    "inbound.package_list_failed",
			wantMessage: "exit status 1: permission denied",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parsePackageUIDCommandResult(10, test.output, test.stderr, test.commandErr)
			var validation *ValidationError
			if !errors.As(err, &validation) || len(validation.Diagnostics) != 1 {
				t.Fatalf("unexpected package list error: %v", err)
			}
			diagnostic := validation.Diagnostics[0]
			if diagnostic.Code != test.wantCode || !strings.Contains(diagnostic.Message, test.wantMessage) {
				t.Fatalf("unexpected package list diagnostic: %+v", diagnostic)
			}
		})
	}

	packages, err := parsePackageUIDCommandResult(10, "package:com.example.app uid:10123\n", "", nil)
	if err != nil || !reflect.DeepEqual(packages, map[string]uint32{"com.example.app": 10123}) {
		t.Fatalf("valid package list result failed: packages=%#v err=%v", packages, err)
	}
}
