package module

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

type archiveFile struct {
	source string
	name   string
	redact bool
	tail   int64
}

const (
	maxDiagnosticFileBytes    = 16 << 20
	maxDiagnosticInputBytes   = 128 << 20
	maxDiagnosticArchiveBytes = 64 << 20
)

var errDiagnosticTooLarge = errors.New("诊断包超出大小限制")

type diagnosticWriter struct {
	output    io.Writer
	remaining int64
}

func (w *diagnosticWriter) Write(content []byte) (int, error) {
	if int64(len(content)) > w.remaining {
		return 0, errDiagnosticTooLarge
	}
	n, err := w.output.Write(content)
	w.remaining -= int64(n)
	return n, err
}

type exportTarget struct {
	info fs.FileInfo
}

// LogSnapshot 是 logs.show 对客户端返回的文本和结构化日志快照。
type LogSnapshot struct {
	Kind    string          `json:"kind"`
	Content string          `json:"content"`
	Entries []logfile.Entry `json:"entries"`
}

// LogFile 返回用户可见日志类型对应的文件。
func LogFile(options Options, kind string) (string, error) {
	name := map[string]string{"service": "service.log", "core": "sing-box.log"}[kind]
	if name == "" {
		return "", fmt.Errorf("未知日志类型: %s", kind)
	}
	return filepath.Join(options.LogDir, name), nil
}

// ReadLog 返回脱敏文本；Native 服务日志同时返回稳定字段条目。
func ReadLog(options Options, kind string, lines int) (LogSnapshot, error) {
	path, err := LogFile(options, kind)
	if err != nil {
		return LogSnapshot{}, err
	}
	if lines <= 0 {
		lines = 200
	}
	content, err := logfile.TailLines(path, lines)
	if err != nil {
		return LogSnapshot{}, err
	}
	redacted := logfile.RedactText(string(content))
	snapshot := LogSnapshot{Kind: kind, Content: redacted, Entries: []logfile.Entry{}}
	if kind == "service" {
		snapshot.Entries = logfile.ParseEntries(redacted)
	}
	return snapshot, nil
}

// ClearLog 清空指定日志。
func ClearLog(options Options, kind string) error {
	path, err := LogFile(options, kind)
	if err != nil {
		return err
	}
	return logfile.Clear(path)
}

// ExportLogs 生成不包含节点凭据和订阅鉴权信息的诊断包。
func ExportLogs(options Options, destination string) error {
	if strings.TrimSpace(destination) == "" {
		return fmt.Errorf("诊断包路径不能为空")
	}
	target, err := inspectExportTarget(destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".diagnostics-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	archiveErr := ExportDiagnostics(options, file)
	if err := errors.Join(archiveErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := applyExportTargetMetadata(file.Name(), target); err != nil {
		return err
	}
	return os.Rename(file.Name(), destination)
}

func inspectExportTarget(destination string) (exportTarget, error) {
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return exportTarget{}, nil
	}
	if err != nil {
		return exportTarget{}, err
	}
	if !info.Mode().IsRegular() {
		return exportTarget{}, fmt.Errorf("诊断包目标不是普通文件")
	}
	return exportTarget{info: info}, nil
}

// ExportDiagnostics 输出完整脱敏诊断包；调用方负责 output 的同步、关闭和完整传输。
func ExportDiagnostics(options Options, output io.Writer) (err error) {
	archive := gzip.NewWriter(&diagnosticWriter{output: output, remaining: maxDiagnosticArchiveBytes})
	tarWriter := tar.NewWriter(archive)
	defer func() { err = errors.Join(err, tarWriter.Close(), archive.Close()) }()
	files := make([]archiveFile, 0)
	if options.StateFile != "" {
		files = append(files, archiveFile{source: options.StateFile, name: "state/service.json", redact: true})
	}
	// Keep current logs ahead of backups and large resource files in the input budget.
	for _, kind := range []string{"service", "core"} {
		path, _ := LogFile(options, kind)
		files = append(files, archiveFile{source: path, name: "logs/" + filepath.Base(path), redact: true, tail: logfile.MaxFileBytes})
	}
	for _, kind := range []string{"service", "core"} {
		path, _ := LogFile(options, kind)
		for index := 1; index <= logfile.BackupCount; index++ {
			backup := fmt.Sprintf("%s.%d", path, index)
			files = append(files, archiveFile{source: backup, name: "logs/" + filepath.Base(backup), redact: true, tail: logfile.MaxFileBytes})
		}
	}
	files = append(files,
		archiveFile{source: options.ModuleConfig, name: "config/module.json", redact: true},
		archiveFile{source: options.InboundConfig, name: "config/inbound.json", redact: true},
		archiveFile{source: paths.SingBoxConfig(options.SingBoxDir), name: "config/singbox/config.json", redact: true},
	)
	for _, directory := range []struct{ path, name string }{
		{paths.SingBoxLocalRulesDir(options.SingBoxDir), "config/singbox/rules/local"},
	} {
		appendDirectoryFiles(&files, directory.path, directory.name, true, false)
	}
	appendDirectoryFiles(&files,
		paths.SingBoxRemoteRulesDir(options.SingBoxDir),
		"config/singbox/rules/remote", false, false,
	)
	appendDirectoryFiles(&files, options.CatalogRoot, "data/catalog", true, true)
	remaining := int64(maxDiagnosticInputBytes)
	omitted := 0
	for _, item := range files {
		var content []byte
		var err error
		switch {
		case remaining <= 0:
			omitted++
			continue
		case item.tail > 0:
			content, err = logfile.ReadSuffix(item.source, min(item.tail, remaining))
		default:
			content, err = readDiagnosticFile(item.source, min(maxDiagnosticFileBytes, remaining))
		}
		if errors.Is(err, errDiagnosticTooLarge) {
			omitted++
			continue
		}
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		remaining -= int64(len(content))
		if item.redact {
			// 损坏的 JSON 无法按字段脱敏，不能把残留的名单或凭据作为诊断原文导出。
			if filepath.Ext(item.name) == ".json" && !jsontext.Value(content).IsValid() {
				content = []byte("{\"diagnostic_error\":\"JSON 配置损坏，已省略原文以保护敏感信息\"}\n")
			}
			content = []byte(logfile.RedactText(string(content)))
		}
		if err := writeTarFile(tarWriter, item.name, content); err != nil {
			return err
		}
	}
	moduleVersionName, moduleVersionCode := moduleVersionInfo(options)
	readme := fmt.Sprintf("NetProxy 诊断包\n管理器版本: %s\n管理器版本号: %s\n模块版本: %s\n模块版本号: %s\n生成时间: %s\n敏感信息已脱敏。\n",
		versionOrUnknown(options.ManagerVersion), versionOrUnknown(options.ManagerVersionCode),
		moduleVersionName, moduleVersionCode, logfile.LocalNow().Format(time.RFC3339))
	if omitted != 0 {
		readme += fmt.Sprintf("为限制诊断包大小，已省略 %d 个条目。\n", omitted)
	}
	return writeTarFile(tarWriter, "README.txt", []byte(readme))
}

func readDiagnosticFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, errDiagnosticTooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err == nil && int64(len(content)) > limit {
		return nil, errDiagnosticTooLarge
	}
	return content, err
}

func versionOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func moduleVersionInfo(options Options) (string, string) {
	content, err := os.ReadFile(paths.New(options.ModuleDir).ModuleProp())
	if err != nil {
		return "unknown", "unknown"
	}
	version := "unknown"
	versionCode := "unknown"
	for rawLine := range strings.SplitSeq(string(content), "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(rawLine, "\ufeff"))
		if after, ok := strings.CutPrefix(line, "version="); ok {
			version = versionOrUnknown(after)
		}
		if after, ok := strings.CutPrefix(line, "versionCode="); ok {
			versionCode = versionOrUnknown(after)
		}
	}
	return version, versionCode
}

func writeTarFile(writer *tar.Writer, name string, content []byte) error {
	header := &tar.Header{Name: filepath.ToSlash(name), Mode: 0o600, Size: int64(len(content)), ModTime: time.Now()}
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	_, err := writer.Write(content)
	return err
}

func appendDirectoryFiles(files *[]archiveFile, root, archiveRoot string, redact, skipStaging bool) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || !info.Mode().IsRegular() {
			if skipStaging && info != nil && info.IsDir() && info.Name() == "staging" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err == nil {
			*files = append(*files, archiveFile{
				source: path,
				name:   filepath.ToSlash(filepath.Join(archiveRoot, relative)),
				redact: redact,
			})
		}
		return nil
	})
}
