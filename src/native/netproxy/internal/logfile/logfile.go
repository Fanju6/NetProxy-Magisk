package logfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

var (
	eventTokenPattern     = regexp.MustCompile(`[^a-z0-9._-]+`)
	canonicalEntryPattern = regexp.MustCompile(`^\[([^]]+)]\s+\[([A-Za-z]+)]\s+\[([A-Za-z0-9._-]+)]\s+\[([A-Za-z0-9._-]+)]\s+\[([A-Za-z0-9._-]+)]\s+\[([A-Za-z0-9._-]+)](?:\s+(.*))?$`)
)

// Entry 是 Native 运行日志的稳定字段集合。
type Entry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Component string `json:"component"`
	Event     string `json:"event"`
	Result    string `json:"result"`
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

const (
	// MaxFileBytes 是 Native 服务日志单文件的大小上限。
	MaxFileBytes int64 = 4 << 20
	// BackupCount 是 Native 服务日志保留的轮转文件数量。
	BackupCount = 2
	// MaxReadBytes 限制一次日志展示在 Native 侧读取的总字节数。
	MaxReadBytes int64 = 2 << 20
	lockTimeout        = 250 * time.Millisecond
)

// Writer 为每次日志写入重新打开文件，确保跨进程轮转后继续写入当前文件。
type Writer struct {
	path string
}

// NewWriter 返回带轮转限制的日志 Writer。
func NewWriter(path string) *Writer {
	return &Writer{path: path}
}

// FormatEntry 将结构化事件转换为兼容终端查看的单行格式。
func FormatEntry(entry Entry) string {
	if strings.TrimSpace(entry.Timestamp) == "" {
		entry.Timestamp = time.Now().Format("2006-01-02 15:04:05")
	}
	level := strings.ToUpper(strings.TrimSpace(entry.Level))
	if level == "WARNING" {
		level = "WARN"
	}
	if level != "DEBUG" && level != "INFO" && level != "WARN" && level != "ERROR" {
		level = "INFO"
	}
	component := eventToken(entry.Component, "native")
	event := eventToken(entry.Event, "native.event")
	result := eventToken(entry.Result, "info")
	errorCode := eventToken(entry.ErrorCode, "-")
	message := RedactMessage(strings.ReplaceAll(entry.Message, "\x00", ""))
	return fmt.Sprintf("[%s] [%s] [%s] [%s] [%s] [%s] %s", entry.Timestamp, level, component, event, result, errorCode, message)
}

// AppendEntry 以统一格式写入一条 Native 事件。
func AppendEntry(path string, entry Entry) error {
	return Append(path, []byte(FormatEntry(entry)+"\n"))
}

// ParseEntry 将统一格式的 Native 文本日志转换为稳定字段。
func ParseEntry(line string) (Entry, bool) {
	line = strings.TrimSpace(line)
	if groups := canonicalEntryPattern.FindStringSubmatch(line); len(groups) == 8 {
		errorCode := groups[6]
		if errorCode == "-" {
			errorCode = ""
		}
		return Entry{Timestamp: groups[1], Level: normalizeLevel(groups[2]), Component: groups[3], Event: groups[4], Result: groups[5], ErrorCode: errorCode, Message: groups[7]}, true
	}
	return Entry{}, false
}

// ParseEntries 解析非空的 Native 日志行。
func ParseEntries(content string) []Entry {
	entries := make([]Entry, 0)
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if line != "" {
			if entry, ok := ParseEntry(line); ok {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func normalizeLevel(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "WARNING" {
		return "WARN"
	}
	if value == "DEBUG" || value == "INFO" || value == "WARN" || value == "ERROR" || value == "FATAL" {
		if value == "FATAL" {
			return "ERROR"
		}
		return value
	}
	return "UNKNOWN"
}

func eventToken(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Trim(eventTokenPattern.ReplaceAllString(value, "-"), "-._")
	if value == "" {
		return fallback
	}
	return value
}

// Write 原子追加一条已经格式化的日志记录。
func (writer *Writer) Write(content []byte) (int, error) {
	if writer == nil || strings.TrimSpace(writer.path) == "" {
		return 0, errors.New("日志路径不能为空")
	}
	if err := Append(writer.path, content); err != nil {
		return 0, err
	}
	return len(content), nil
}

// Prepare 创建日志目录和文件，并校正权限。
func Prepare(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("日志路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// Append 在跨进程锁内轮转并追加日志。
func Append(path string, content []byte) error {
	if len(content) == 0 {
		return nil
	}
	if err := Prepare(path); err != nil {
		return err
	}
	lock, err := acquire(path + ".lock")
	if err != nil {
		return err
	}
	defer lock.Release()

	if int64(len(content)) > MaxFileBytes {
		content = content[len(content)-int(MaxFileBytes):]
	}
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && info.Size()+int64(len(content)) > MaxFileBytes {
		if err := rotate(path); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	_, err = file.Write(content)
	return err
}

func acquire(path string) (*processlock.Lock, error) {
	deadline := time.Now().Add(lockTimeout)
	for {
		lock, err := processlock.TryAcquire(path)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, processlock.ErrBusy) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("日志文件正忙: %w", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func rotate(path string) error {
	for index := BackupCount; index >= 1; index-- {
		source := path
		if index > 1 {
			source = fmt.Sprintf("%s.%d", path, index-1)
		}
		target := fmt.Sprintf("%s.%d", path, index)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(source, target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// TailLines 从当前日志及轮转文件中有界读取最新若干行。
func TailLines(path string, lines int) ([]byte, error) {
	if lines <= 0 {
		return nil, nil
	}
	remaining := MaxReadBytes
	parts := make([][]byte, 0, BackupCount+1)
	found := false
	for index := 0; index <= BackupCount && remaining > 0; index++ {
		candidate := path
		if index > 0 {
			candidate = fmt.Sprintf("%s.%d", path, index)
		}
		content, truncated, err := readSuffix(candidate, remaining)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		found = true
		if truncated {
			if offset := strings.IndexByte(string(content), '\n'); offset >= 0 {
				content = content[offset+1:]
			} else {
				content = nil
			}
		}
		remaining -= int64(len(content))
		parts = append([][]byte{content}, parts...)
	}
	if !found {
		return nil, nil
	}
	content := strings.ReplaceAll(string(join(parts)), "\r\n", "\n")
	items := strings.Split(content, "\n")
	if len(items) > lines+1 {
		items = items[len(items)-lines-1:]
	}
	return []byte(strings.Join(items, "\n")), nil
}

// ReadSuffix 返回文件末尾至多 limit 字节。
func ReadSuffix(path string, limit int64) ([]byte, error) {
	content, _, err := readSuffix(path, limit)
	return content, err
}

func readSuffix(path string, limit int64) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if limit <= 0 || info.Size() == 0 {
		return nil, info.Size() > 0, nil
	}
	size := min(info.Size(), limit)
	content := make([]byte, size)
	_, err = file.ReadAt(content, info.Size()-size)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	return content, info.Size() > size, nil
}

func join(parts [][]byte) []byte {
	var size int
	for _, part := range parts {
		size += len(part)
	}
	result := make([]byte, 0, size)
	for _, part := range parts {
		result = append(result, part...)
	}
	return result
}
