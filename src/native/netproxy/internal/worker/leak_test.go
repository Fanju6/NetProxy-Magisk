package worker

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		if err := checkGoroutineLeaks(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

func checkGoroutineLeaks() error {
	profile := pprof.Lookup("goroutineleak")
	if profile == nil {
		return fmt.Errorf("Go 运行时未提供 goroutineleak profile")
	}
	var output bytes.Buffer
	if err := profile.WriteTo(&output, 2); err != nil {
		return fmt.Errorf("生成 goroutine 泄漏报告失败: %w", err)
	}
	if profile.Count() != 0 {
		return fmt.Errorf("Worker 测试遗留 goroutine:\n%s", output.String())
	}
	return nil
}
