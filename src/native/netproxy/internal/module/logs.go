package module

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	urlCredentialPattern = regexp.MustCompile(`(https?://)[^/@\s]+@`)
	querySecretPattern   = regexp.MustCompile(`(?i)([?&](?:token|key|secret|password|auth|uuid|hwid)=)[^&\s]+`)
	authorizationPattern = regexp.MustCompile(`(?i)((?:authorization|proxy-authorization)\s*:\s*(?:bearer|basic)\s+)[^\r\n\s,;]+`)
	lineSecretPattern    = regexp.MustCompile(`(?i)((?:authorization|proxy-authorization|x-hwid|hwid|token|password|secret|private[_-]?key|custom[_-]?headers)\s*[:=]\s*)[^\r\n\s,;]+`)
)

// LogFile 返回用户可见日志类型对应的文件。
func LogFile(options Options, kind string) (string, error) {
	name := map[string]string{"service": "service.log", "core": "sing-box.log"}[kind]
	if name == "" {
		return "", fmt.Errorf("未知日志类型: %s", kind)
	}
	return filepath.Join(options.LogDir, name), nil
}

// ShowLog 返回末尾日志，并对订阅地址和凭据做脱敏。
func ShowLog(options Options, kind string, lines int) (string, error) {
	path, err := LogFile(options, kind)
	if err != nil {
		return "", err
	}
	if lines <= 0 {
		lines = 200
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	items := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	if len(items) > lines+1 {
		items = items[len(items)-lines-1:]
	}
	return redactText(strings.Join(items, "\n")), nil
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
	files := make([]struct{ source, name string }, 0)
	for _, kind := range []string{"service", "core"} {
		path, _ := LogFile(options, kind)
		files = append(files, struct{ source, name string }{path, "logs/" + filepath.Base(path)})
	}
	files = append(files,
		struct{ source, name string }{options.ModuleConfig, "config/module.conf"},
		struct{ source, name string }{options.EBPFConfig, "config/ebpf.conf"},
	)
	for _, directory := range []struct{ path, name string }{
		{filepath.Join(options.SingBoxDir, "confdir"), "config/singbox/confdir"},
		{filepath.Join(options.SingBoxDir, "source"), "config/singbox/source"},
		{options.CatalogRoot, "data/catalog"},
	} {
		appendDirectoryFiles(&files, directory.path, directory.name, directory.path == options.CatalogRoot)
	}
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		files = append(files, struct{ source, name string }{
			filepath.Join(options.RuntimeDir, name), "runtime/" + name,
		})
	}
	if options.StateFile != "" {
		files = append(files, struct{ source, name string }{options.StateFile, "state/service.json"})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	for _, item := range files {
		content, err := os.ReadFile(item.source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		content = []byte(redactText(string(content)))
		if err := writeTarFile(tarWriter, item.name, content); err != nil {
			return err
		}
	}
	moduleVersionName, moduleVersionCode := moduleVersionInfo(options)
	readme := fmt.Sprintf("NetProxy 诊断包\n管理器版本: %s\n管理器版本号: %s\n模块版本: %s\n模块版本号: %s\n生成时间: %s\n敏感信息已脱敏。\n",
		versionOrUnknown(options.ManagerVersion), versionOrUnknown(options.ManagerVersionCode),
		moduleVersionName, moduleVersionCode, time.Now().Format(time.RFC3339))
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
	content, err := os.ReadFile(filepath.Join(options.ModuleDir, "module.prop"))
	if err != nil {
		return "unknown", "unknown"
	}
	version := "unknown"
	versionCode := "unknown"
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(rawLine, "\ufeff"))
		if strings.HasPrefix(line, "version=") {
			version = versionOrUnknown(strings.TrimPrefix(line, "version="))
		}
		if strings.HasPrefix(line, "versionCode=") {
			versionCode = versionOrUnknown(strings.TrimPrefix(line, "versionCode="))
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

func appendDirectoryFiles(files *[]struct{ source, name string }, root, archiveRoot string, skipStaging bool) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			if skipStaging && info != nil && info.IsDir() && info.Name() == "staging" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err == nil {
			*files = append(*files, struct{ source, name string }{path, filepath.ToSlash(filepath.Join(archiveRoot, relative))})
		}
		return nil
	})
}

func redactText(value string) string {
	var document any
	if json.Unmarshal([]byte(value), &document) == nil {
		switch document.(type) {
		case map[string]any, []any:
			redactJSON(document)
			if encoded, err := json.MarshalIndent(document, "", "  "); err == nil {
				return string(encoded) + "\n"
			}
		}
	}
	value = urlCredentialPattern.ReplaceAllString(value, `$1***@`)
	value = querySecretPattern.ReplaceAllString(value, `${1}***`)
	value = authorizationPattern.ReplaceAllString(value, `${1}***`)
	value = lineSecretPattern.ReplaceAllString(value, `${1}***`)
	return value
}

func redactJSON(value any) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			lower := strings.ToLower(key)
			if lower == "url" || lower == "hwid" || lower == "user_agent" || lower == "custom_headers" || lower == "headers" || lower == "authorization" || lower == "proxy_authorization" || lower == "uuid" || lower == "password" || lower == "token" || lower == "secret" || lower == "private_key" || lower == "public_key" || lower == "short_id" {
				item[key] = "***"
				continue
			}
			redactJSON(child)
		}
	case []any:
		for _, child := range item {
			redactJSON(child)
		}
	}
}
