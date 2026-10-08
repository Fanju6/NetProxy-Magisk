package logfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProcessLogAppendsAndClearPreservesLiveHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sing-box.log")
	if err := os.WriteFile(path, []byte("previous start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := OpenForProcess(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("核心日志权限未校正: %o", info.Mode().Perm())
	}
	if _, err := file.WriteString("current start\n"); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "previous start\ncurrent start\n" {
		t.Fatalf("启动未保留日志: %q %v", content, err)
	}
	for index := 1; index <= BackupCount; index++ {
		if err := os.WriteFile(fmt.Sprintf("%s.%d", path, index), []byte("backup\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Clear(path); err != nil {
		t.Fatal(err)
	}
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(info, current) {
		t.Fatalf("清空替换了正在使用的 inode: %v", err)
	}
	if _, err := file.WriteString("after clear\n"); err != nil {
		t.Fatal(err)
	}
	content, err := TailLines(path, 20)
	if err != nil || string(content) != "after clear\n" {
		t.Fatalf("清空后出现空洞或旧备份: %q %v", content, err)
	}
	for index := 1; index <= BackupCount; index++ {
		if _, err := os.Stat(fmt.Sprintf("%s.%d", path, index)); !os.IsNotExist(err) {
			t.Fatalf("清空未删除备份 %d: %v", index, err)
		}
	}
}

func TestProcessLogRotatesOnlyWhenOpened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "sing-box.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("z"), int(MaxFileBytes)), 0o644); err != nil {
		t.Fatal(err)
	}
	for index := range BackupCount + 2 {
		file, err := OpenForProcess(path)
		if err != nil {
			t.Fatal(err)
		}
		content := bytes.Repeat([]byte{byte('a' + index)}, int(MaxFileBytes)+1)
		if _, err := file.Write(content); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, content) {
			_ = file.Close()
			t.Fatalf("运行中日志被轮转或截断: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			info, err := os.Stat(path + ".1")
			if err != nil || info.Size() != MaxFileBytes {
				t.Fatalf("达到阈值的原日志未完整保留: %v", err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Fatalf("轮转前未校正备份权限: %o", info.Mode().Perm())
			}
		}
	}
	for index := 1; index <= BackupCount; index++ {
		got, err := os.ReadFile(fmt.Sprintf("%s.%d", path, index))
		want := bytes.Repeat([]byte{byte('a' + BackupCount + 1 - index)}, int(MaxFileBytes)+1)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("备份 %d 的顺序或内容异常: %v", index, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("保留了多余备份: %v", err)
	}
}

func TestAppendRotatesAndTailLinesReadsAcrossBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	large := bytes.Repeat([]byte("x"), int(MaxFileBytes)-1)
	if err := Append(path, append(large, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, []byte("new-one\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("日志未轮转: %v", err)
	}
	if err := os.WriteFile(path+".1", []byte("new-one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new-two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := TailLines(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); !strings.Contains(got, "new-one\nnew-two") {
		t.Fatalf("尾读未跨轮转文件保留最新记录: %q", got)
	}
}

func TestTailLinesBoundsReadAndDropsPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	prefix := bytes.Repeat([]byte("x"), int(MaxReadBytes)+128)
	content := append(prefix, []byte("\nlast-one\nlast-two\n")...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	tail, err := TailLines(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(tail); got != "last-one\nlast-two\n" {
		t.Fatalf("有界尾读结果异常: %q", got)
	}
}

func TestAppendKeepsTwoBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	for index := range 4 {
		line := []byte(fmt.Sprintf("entry-%d\n", index))
		padding := bytes.Repeat([]byte{byte('a' + index)}, int(MaxFileBytes)-len(line))
		if err := Append(path, append(padding, line...)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Fatalf("第二份轮转日志不存在: %v", err)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("保留了超出上限的轮转日志: %v", err)
	}
}

func TestFormatEntryNormalizesFieldsAndMessage(t *testing.T) {
	line := FormatEntry(Entry{
		Timestamp: "2026-08-13 12:00:00",
		Level:     "warning",
		Component: "Subscription Worker",
		Event:     "subscription.update",
		Result:    "not running",
		ErrorCode: "subscription.runtime_sync_failed",
		Message:   "第一行\n第二行",
	})
	want := "[2026-08-13 12:00:00] [WARN] [subscription-worker] [subscription.update] [not-running] [subscription.runtime_sync_failed] 第一行 第二行"
	if line != want {
		t.Fatalf("统一日志格式异常:\nwant %q\n got %q", want, line)
	}
}

func TestResolveLocalLocationHonorsTimezoneEnvironment(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	location := resolveLocalLocation()
	_, offset := time.Now().In(location).Zone()
	if offset != 8*60*60 {
		t.Fatalf("本地日志时区未读取 TZ: offset=%d", offset)
	}
}

func TestParseEntriesAcceptsOnlyCanonicalFormat(t *testing.T) {
	content := strings.Join([]string{
		"[2026-08-13 12:00:00] [INFO] [subscription] [subscription.update] [success] [-] 订阅更新完成",
		"[2026-08-13 12:01:00] [ERROR] [service] [service.start] [failed] 启动失败",
	}, "\n")
	entries := ParseEntries(content)
	if len(entries) != 1 {
		t.Fatalf("解析日志数量异常: %d", len(entries))
	}
	entry := entries[0]
	if entry.Level != "INFO" || entry.Component != "subscription" || entry.Event != "subscription.update" || entry.Result != "success" || entry.ErrorCode != "" {
		t.Fatalf("统一日志字段异常: %+v", entry)
	}
}

func TestParseEntryAcceptsCanonicalEmptyMessage(t *testing.T) {
	line := FormatEntry(Entry{Level: "INFO", Component: "worker", Event: "worker.run", Result: "started"})
	entry, ok := ParseEntry(line)
	if !ok || entry.Message != "" {
		t.Fatalf("空消息的统一日志无法解析: %+v, %t", entry, ok)
	}
}

func TestFormatEntryRedactsAndBoundsMessageBeforeWrite(t *testing.T) {
	message := "请求 https://user:password@example.test/sub?token=secret 失败，Authorization: Bearer bearer-secret，节点 vmess://node-secret " + strings.Repeat("诊断", maxMessageRunes)
	line := FormatEntry(Entry{Level: "ERROR", Component: "subscription", Event: "subscription.update", Result: "failed", ErrorCode: "subscription.convert_failed", Message: message})
	for _, secret := range []string{"example.test", "password", "secret", "vmess://"} {
		if strings.Contains(line, secret) {
			t.Fatalf("统一日志在落盘前泄露 %q: %s", secret, line)
		}
	}
	if !strings.Contains(line, "[subscription.convert_failed]") || !strings.HasSuffix(line, "…") {
		t.Fatalf("错误码或消息限长异常: %s", line)
	}
}
