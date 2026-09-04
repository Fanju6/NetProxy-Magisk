package module

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// ExportLogs 生成不包含节点凭据和订阅鉴权信息的诊断包。
func ExportLogs(options Options, destination string) error {
	if strings.TrimSpace(destination) == "" {
		return fmt.Errorf("诊断包路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	archive := gzip.NewWriter(file)
	defer archive.Close()
	tarWriter := tar.NewWriter(archive)
	defer tarWriter.Close()
	files := make([]archiveFile, 0)
	for _, kind := range []string{"service", "core"} {
		path, _ := LogFile(options, kind)
		if kind == "service" {
			for index := logfile.BackupCount; index >= 1; index-- {
				backup := fmt.Sprintf("%s.%d", path, index)
				files = append(files, archiveFile{source: backup, name: "logs/" + filepath.Base(backup), redact: true, tail: logfile.MaxFileBytes})
			}
		}
		files = append(files, archiveFile{source: path, name: "logs/" + filepath.Base(path), redact: true, tail: logfile.MaxFileBytes})
	}
	files = append(files,
		archiveFile{source: options.ModuleConfig, name: "config/module.conf", redact: true},
		archiveFile{source: options.EBPFConfig, name: "config/ebpf.conf", redact: true},
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
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		files = append(files, archiveFile{
			source: filepath.Join(options.RuntimeDir, name),
			name:   "runtime/" + name,
			redact: true,
		})
	}
	if options.StateFile != "" {
		files = append(files, archiveFile{source: options.StateFile, name: "state/service.json", redact: true})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	for _, item := range files {
		var content []byte
		var err error
		if item.tail > 0 {
			content, err = logfile.ReadSuffix(item.source, item.tail)
		} else {
			content, err = os.ReadFile(item.source)
		}
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if item.redact {
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
	return writeTarFile(tarWriter, "README.txt", []byte(readme))
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
		if walkErr != nil || info == nil || info.IsDir() {
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
