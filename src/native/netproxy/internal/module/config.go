package module

import (
	"context"
	"crypto/sha256"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/ebpf"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

var (
	configProcessRunning = service.ProcessRunning
	configReload         = reloadAppliedConfig
	configRestoreReload  = reloadConfigSnapshot
)

// ConfigDocument 是配置工作台可见的文件摘要。
type ConfigDocument struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Category string `json:"category"`
	Editable bool   `json:"editable"`
	Section  string `json:"section,omitempty"`
}

var ErrConfigConflict = errors.New("配置已被修改，请重新加载后再保存")

var configSections = map[string]bool{
	"log": true, "experimental": true, "dns": true, "inbounds": true,
	"route": true, "http_clients": true, "services": true, "outbounds": true,
	"providers": false, "endpoints": false, "ntp": false,
	"certificate": false, "certificate_providers": false, "network_namespaces": false,
}

func configSection(target string) string {
	section, hasPrefix := strings.CutPrefix(target, "singbox/")
	if _, exists := configSections[section]; hasPrefix && exists {
		return section
	}
	return ""
}

// ListConfigs 返回所有可管理的配置文件，不读取运行时 JSON 内容。
func ListConfigs(options Options) ([]ConfigDocument, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	result := make([]ConfigDocument, 0)
	if _, err := os.Stat(paths.SingBoxConfig(options.SingBoxDir)); err == nil {
		result = append(result, ConfigDocument{ID: "singbox/config.json", Filename: "config.json", Category: "config", Editable: true})
		// 主配置损坏时仍提供完整编辑入口，避免用户无法打开文件进行修复。
		content, _ := os.ReadFile(paths.SingBoxConfig(options.SingBoxDir))
		object, _ := configObject(content)
		for section, alwaysListed := range configSections {
			if _, exists := object[section]; !alwaysListed && !exists {
				continue
			}
			result = append(result, ConfigDocument{ID: "singbox/" + section, Filename: section, Category: "config", Editable: true, Section: section})
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	entries, err := os.ReadDir(paths.SingBoxLocalRulesDir(options.SingBoxDir))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		result = append(result, ConfigDocument{
			ID:       "singbox/rules/local/" + entry.Name(),
			Filename: entry.Name(),
			Category: "rules",
			Editable: true,
		})
	}
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		path := filepath.Join(options.RuntimeDir, name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		result = append(result, ConfigDocument{
			ID: "runtime/" + name, Filename: name, Category: "runtime", Editable: false,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// ReadConfig 读取一个配置文件并保留原始文本。
func ReadConfig(options Options, target string) (map[string]string, error) {
	path, err := ResolveConfig(options, target)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if section := configSection(target); section != "" {
		object, err := configObject(content)
		if err != nil {
			return nil, err
		}
		content, err = sectionContent(object, section)
		if err != nil {
			return nil, err
		}
	}
	return map[string]string{"target": target, "content": string(content), "revision": configRevision(content)}, nil
}

func configRevision(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func configObject(content []byte) (map[string]jsontext.Value, error) {
	var object map[string]jsontext.Value
	if err := json.Unmarshal(content, &object); err != nil {
		return nil, fmt.Errorf("配置必须是有效 JSON 对象: %w", err)
	}
	if object == nil {
		return nil, errors.New("配置必须是 JSON 对象，不能为 null")
	}
	return object, nil
}

func sectionContent(object map[string]jsontext.Value, section string) ([]byte, error) {
	fragment := make(map[string]jsontext.Value)
	if value, exists := object[section]; exists {
		fragment[section] = value
	}
	return json.Marshal(fragment, json.Deterministic(true), jsontext.WithIndent("  "))
}

func prepareConfigEdit(current, replacement []byte, section, expectedRevision string) ([]byte, string, error) {
	var object map[string]jsontext.Value
	var err error
	if section != "" {
		object, err = configObject(current)
		if err != nil {
			return nil, "", err
		}
		if expectedRevision != "" {
			current, err = sectionContent(object, section)
			if err != nil {
				return nil, "", err
			}
		}
	}
	if expectedRevision != "" && configRevision(current) != expectedRevision {
		return nil, "", ErrConfigConflict
	}
	if section == "" {
		return replacement, configRevision(replacement), nil
	}
	fragment, err := configObject(replacement)
	if err != nil {
		return nil, "", err
	}
	for key := range fragment {
		if key != section {
			return nil, "", fmt.Errorf("当前编辑器只允许修改 %s，不能包含 %s", section, key)
		}
	}
	replacement, err = json.Marshal(fragment, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return nil, "", err
	}
	if value, exists := fragment[section]; exists {
		object[section] = value
	} else {
		delete(object, section)
	}
	merged, err := json.Marshal(object, json.Deterministic(true), jsontext.WithIndent("  "))
	return merged, configRevision(replacement), err
}

// ApplyConfig 通过候选文件、校验和原子替换应用配置。
func ApplyConfig(ctx context.Context, options Options, target, source string, validateOnly bool, expectedRevision string) (revision string, err error) {
	event := "config.apply"
	message := "配置保存"
	if validateOnly {
		event = "config.validate"
		message = "配置校验"
	}
	defer func() { logOperation(options, "config", event, message, false, err) }()
	destination, err := ResolveConfig(options, target)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(target, "runtime/") {
		return "", errors.New("运行时配置只读")
	}
	replacement, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("配置内容文件不存在: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", err
	}
	if err := options.validate(); err != nil {
		return "", err
	}
	var lifecycleLock *lifecycleLock
	if !validateOnly {
		lifecycleLock, err = acquireLifecycleLock(options.StateFile)
		if err != nil {
			return "", err
		}
		defer lifecycleLock.release()
		if err := recoverConfigApply(ctx, options); err != nil {
			return "", err
		}
	}
	// 锁内读取最新主配置后只替换目标分区，不能把客户端的整份旧快照写回。
	section := configSection(target)
	var current []byte
	if section != "" || expectedRevision != "" {
		current, err = os.ReadFile(destination)
		if err != nil {
			return "", err
		}
	}
	content, revision, err := prepareConfigEdit(current, replacement, section, expectedRevision)
	if err != nil {
		return "", err
	}
	candidate, err := os.CreateTemp(filepath.Dir(destination), ".config-candidate-")
	if err != nil {
		return "", err
	}
	candidatePath := candidate.Name()
	defer os.Remove(candidatePath)
	if err := candidate.Close(); err != nil {
		return "", err
	}
	if err := os.WriteFile(candidatePath, content, 0o600); err != nil {
		return "", err
	}
	if section != "" {
		err = validateSingBoxTree(ctx, options, candidatePath)
	} else {
		err = validateConfig(ctx, options, target, candidatePath, content)
	}
	if err != nil {
		return "", err
	}
	if validateOnly {
		return revision, nil
	}
	transaction, err := beginConfigApply(options, destination)
	if err != nil {
		return "", err
	}
	if err := os.Rename(candidatePath, destination); err != nil {
		_ = transaction.rollback()
		return "", err
	}
	if err := transaction.setPhase("static_replaced"); err != nil {
		rollbackErr := transaction.rollback()
		if rollbackErr != nil {
			return "", fmt.Errorf("记录配置应用阶段失败: %v；回滚失败: %w", err, rollbackErr)
		}
		return "", fmt.Errorf("记录配置应用阶段失败: %w", err)
	}
	if !configProcessRunning(options.SingBoxPath) {
		if err := transaction.commit(); err != nil {
			rollbackErr := transaction.rollback()
			if rollbackErr != nil {
				return "", fmt.Errorf("提交配置事务失败: %v；回滚失败: %w", err, rollbackErr)
			}
			return "", fmt.Errorf("提交配置事务失败: %w", err)
		}
		return revision, nil
	}
	if err := transaction.setPhase("reload_started"); err != nil {
		rollbackErr := transaction.rollback()
		if rollbackErr != nil {
			return "", fmt.Errorf("记录配置 reload 阶段失败: %v；回滚失败: %w", err, rollbackErr)
		}
		return "", fmt.Errorf("记录配置 reload 阶段失败: %w", err)
	}
	if err := configReload(ctx, options); err != nil {
		restoreErr := transaction.restore()
		if restoreErr != nil {
			return "", fmt.Errorf("配置 reload 失败: %v；恢复旧配置失败: %w", err, restoreErr)
		}
		if configProcessRunning(options.SingBoxPath) {
			if restoreErr := configRestoreReload(ctx, options, transaction.journal); restoreErr != nil {
				return "", fmt.Errorf("配置 reload 失败，且运行实例恢复失败: %v；%w", err, restoreErr)
			}
		}
		if cleanupErr := transaction.cleanup(); cleanupErr != nil {
			return "", fmt.Errorf("配置 reload 失败，旧配置已恢复但清理事务失败: %v；%w", err, cleanupErr)
		}
		return "", fmt.Errorf("配置 reload 失败，已恢复旧配置: %w", err)
	}
	if err := transaction.commit(); err != nil {
		return "", rollbackAfterCommitFailure(ctx, options, transaction, err)
	}
	return revision, nil
}

func rollbackAfterCommitFailure(ctx context.Context, options Options, transaction *configApplyTransaction, commitErr error) error {
	diskErr := transaction.restore()
	var runtimeErr error
	if configProcessRunning(options.SingBoxPath) {
		runtimeErr = configRestoreReload(ctx, options, transaction.journal)
	}
	cleanupErr := transaction.cleanup()
	details := make([]string, 0, 3)
	if diskErr != nil {
		details = append(details, fmt.Sprintf("磁盘恢复失败: %v", diskErr))
	}
	if runtimeErr != nil {
		details = append(details, fmt.Sprintf("旧 runtime reload 失败: %v", runtimeErr))
	}
	if cleanupErr != nil {
		details = append(details, fmt.Sprintf("事务清理失败: %v", cleanupErr))
	}
	if len(details) == 0 {
		return fmt.Errorf("提交配置事务失败: %w", commitErr)
	}
	return fmt.Errorf("提交配置事务失败: %w；%s", commitErr, strings.Join(details, "；"))
}

func validateConfig(ctx context.Context, options Options, target, candidate string, content []byte) error {
	switch target {
	case "module":
		_, err := moduleconfig.LoadModule(candidate)
		return err
	case "ebpf":
		_, err := ebpf.Load(candidate)
		return err
	}
	if !jsontext.Value(content).IsValid() {
		return errors.New("配置不是有效 JSON")
	}
	if target == "singbox/config.json" {
		if _, err := configObject(content); err != nil {
			return err
		}
		return validateSingBoxTree(ctx, options, candidate)
	}
	return nil
}

// validateSingBoxTree 在临时配置树中检查候选静态配置，避免直接覆盖用户正在使用的文件。
func validateSingBoxTree(ctx context.Context, options Options, candidate string) error {
	temporary, err := os.MkdirTemp("", "netproxy-config-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := copyDirectory(paths.SingBoxRulesDir(options.SingBoxDir), paths.SingBoxRulesDir(temporary)); err != nil {
		return err
	}
	candidatePath := paths.SingBoxConfig(temporary)
	if err := copyFile(candidatePath, candidate, 0o600); err != nil {
		return err
	}
	checkOptions := options
	checkOptions.RuntimeDir = filepath.Join(temporary, "runtime")
	prepared, err := Prepare(ctx, checkOptions, true)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, options.SingBoxPath, "check", "-c", candidatePath,
		"-c", prepared.Providers, "-c", prepared.Outbounds, "-c", prepared.EBPF)
	command.Dir = temporary
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("sing-box 配置检查失败: %w", err)
	}
	return nil
}

func copyDirectory(source, destination string) error {
	info, err := os.Stat(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("不是配置目录: %s", source)
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		return copyFile(target, path, 0o600)
	})
}

// ResolveConfig 将客户端配置 ID 安全解析为模块内文件。
func ResolveConfig(options Options, target string) (string, error) {
	switch target {
	case "module":
		return options.ModuleConfig, nil
	case "ebpf":
		return options.EBPFConfig, nil
	case "singbox/config.json":
		return paths.SingBoxConfig(options.SingBoxDir), nil
	}
	if configSection(target) != "" {
		return paths.SingBoxConfig(options.SingBoxDir), nil
	}
	if !strings.HasPrefix(target, "singbox/") && !strings.HasPrefix(target, "runtime/") {
		return "", errors.New("不支持的配置目标")
	}
	root := options.SingBoxDir
	prefix := "singbox/"
	if strings.HasPrefix(target, "runtime/") {
		root = options.RuntimeDir
		prefix = "runtime/"
	}
	relative := filepath.FromSlash(strings.TrimPrefix(target, prefix))
	parts := strings.Split(filepath.ToSlash(relative), "/")
	validLocalRulePath := len(parts) == 3 && parts[0] == "rules" && parts[1] == "local" && filepath.Ext(parts[2]) == ".json" && parts[2] != "" && parts[2][0] != '.'
	if prefix == "singbox/" && !validLocalRulePath {
		return "", errors.New("配置目标路径无效")
	}
	if prefix == "runtime/" && (len(parts) != 1 || filepath.Ext(parts[0]) != ".json" || parts[0] == "" || parts[0][0] == '.') {
		return "", errors.New("配置目标路径无效")
	}
	name := parts[len(parts)-1]
	if prefix == "runtime/" && !isRuntimeConfigName(name) {
		return "", errors.New("不支持的运行时文件")
	}
	for _, char := range name {
		if !(char == '.' || char == '-' || char == '_' || char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z') {
			return "", errors.New("配置文件名无效")
		}
	}
	return filepath.Join(root, relative), nil
}

func isRuntimeConfigName(name string) bool {
	switch name {
	case "providers.json", "outbounds.json", "ebpf.json":
		return true
	default:
		return false
	}
}

func copyFile(destination, source string, mode fs.FileMode) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(destination, content, mode); err != nil {
		return err
	}
	return os.Chmod(destination, mode)
}
