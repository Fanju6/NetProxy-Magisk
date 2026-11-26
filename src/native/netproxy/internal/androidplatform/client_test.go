package androidplatform

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
)

func TestDecodeResponseDoesNotExposePlatformError(t *testing.T) {
	_, err := decodeResponse([]byte(`{"schema":1,"ok":false,"code":"android.query_failed","message":"secret=private-fixture https://invalid.example/subscription"}`), errors.New("private-process-error"))
	failure, ok := errors.AsType[*contract.Error](err)
	if !ok || failure.Code != "android.query_failed" || failure.Message != "无法读取 Android 平台信息" {
		t.Fatal(err)
	}
}

func TestDecodeResponsePreservesPlatformContracts(t *testing.T) {
	for _, test := range []struct {
		output string
		runErr error
		code   string
	}{
		{"", errors.New("process failed"), "android.unavailable"},
		{"{}", nil, "android.invalid_output"},
		{`{"schema":1,"ok":true,"code":"android.apps","unexpected":true}`, nil, "android.invalid_output"},
		{`{"schema":2,"ok":false,"code":"android.query_failed"}`, nil, "android.invalid_output"},
		{`{"schema":1,"ok":false,"code":"android.query_failed"}`, nil, "android.query_failed"},
		{`{"schema":1,"ok":true,"code":"android.apps","data":{"users":[]}}`, errors.New("process failed"), "android.query_failed"},
	} {
		_, err := decodeResponse([]byte(test.output), test.runErr)
		failure, ok := errors.AsType[*contract.Error](err)
		if !ok || failure.Code != test.code {
			t.Fatalf("want %s, got %v", test.code, err)
		}
	}
	data, err := decodeResponse([]byte(`{"schema":1,"ok":true,"code":"android.apps","data":{"users":[]}}`), nil)
	if err != nil || string(data) != `{"users":[]}` {
		t.Fatal(string(data), err)
	}
}

func TestDecodeResponseAfterAbnormalHelperExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell exit status")
	}
	const valid = `{"schema":1,"ok":true,"code":"android.apps","data":{"users":[]}}`
	for _, tt := range []struct {
		name, output, terminator, stop, code string
	}{
		{"signal", valid, "\n", "kill -SEGV $$", ""},
		{"nonzero", valid, "\n", "exit 7", ""},
		{"no terminator", valid, "", "kill -SEGV $$", "android.invalid_output"},
		{"truncated", valid[:len(valid)-1], "", "kill -SEGV $$", "android.invalid_output"},
		{"trailing document", valid + " {}", "\n", "kill -SEGV $$", "android.invalid_output"},
		{"missing data", `{"schema":1,"ok":true,"code":"android.apps"}`, "\n", "exit 7", "android.invalid_output"},
		{"failed operation", `{"schema":1,"ok":false,"code":"android.query_failed"}`, "\n", "kill -SEGV $$", "android.query_failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), "sh", "-c", `ulimit -c 0; printf '%s' "$1"; `+tt.stop, "helper", tt.output+tt.terminator)
			output, runErr := command.Output()
			if _, ok := errors.AsType[*exec.ExitError](runErr); !ok {
				t.Fatalf("fixture must exit abnormally: %v", runErr)
			}
			data, err := decodeResponse(output, runErr)
			if tt.code == "" {
				if err != nil || string(data) != `{"users":[]}` {
					t.Fatalf("want complete response, got %s: %v", data, err)
				}
				return
			}
			if failure, ok := errors.AsType[*contract.Error](err); !ok || failure.Code != tt.code {
				t.Fatalf("want %s, got %v", tt.code, err)
			}
		})
	}
}
