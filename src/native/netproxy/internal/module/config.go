package module

import (
	"context"
	"encoding/json"
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
}

// ListConfigs 返回所有可管理的配置文件，不读取运行时 JSON 内容。
func ListConfigs(options Options) ([]ConfigDocument, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	result := make([]ConfigDocument, 0)
	for _, item := range []struct {
		dir        string
		pathPrefix string
		category   string
		editable   bool
	}{
		{paths.SingBoxConfDir(options.SingBoxDir), "confdir", "config", true},
		{paths.SingBoxLocalRulesDir(options.SingBoxDir), "rules/local", "rules", true},
	} {
		entries, err := os.ReadDir(item.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			result = append(result, ConfigDocument{
				ID:       "singbox/" + filepath.ToSlash(filepath.Join(item.pathPrefix, entry.Name())),
				Filename: entry.Name(),
				Category: item.category,
				Editable: item.editable,
			})
		}
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
	return map[string]string{"target": target, "content": string(content)}, nil
}

// ApplyConfig 通过候选文件、校验和原子替换应用配置。
func ApplyConfig(ctx context.Context, options Options, target, source string, validateOnly bool) (err error) {
	event := "config.apply"
	message := "配置保存"
	if validateOnly {
		event = "config.validate"
		message = "配置校验"
	}
	defer func() { logOperation(options, "config", event, message, false, err) }()
	destination, err := ResolveConfig(options, target)
	if err != nil {
		return err
	}
	if strings.HasPrefix(target, "runtime/") {
		return errors.New("运行时配置只读")
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("配置内容文件不存在: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	if err := options.validate(); err != nil {
		return err
	}
	candidate, err := os.CreateTemp(filepath.Dir(destination), ".config-candidate-")
	if err != nil {
		return err
	}
	candidatePath := candidate.Name()
	defer os.Remove(candidatePath)
	if err := candidate.Close(); err != nil {
		return err
	}
	if err := copyFile(candidatePath, source, 0o600); err != nil {
		return err
	}
	var lifecycleLock *lifecycleLock
	if !validateOnly {
		lifecycleLock, err = acquireLifecycleLock(options.StateFile)
		if err != nil {
			return err
		}
		defer lifecycleLock.release()
		if err := recoverConfigApply(ctx, options); err != nil {
			return err
		}
	}
	if err := ValidateConfig(ctx, options, target, candidatePath); err != nil {
		return err
	}
	if validateOnly {
		return nil
	}
	transaction, err := beginConfigApply(options, destination)
	if err != nil {
		return err
	}
	if err := os.Rename(candidatePath, destination); err != nil {
		_ = transaction.rollback()
		return err
	}
	if err := transaction.setPhase("static_replaced"); err != nil {
		rollbackErr := transaction.rollback()
		if rollbackErr != nil {
			return fmt.Errorf("记录配置应用阶段失败: %v；回滚失败: %w", err, rollbackErr)
		}
		return fmt.Errorf("记录配置应用阶段失败: %w", err)
	}
	if !configProcessRunning(options.SingBoxPath) {
		if err := transaction.commit(); err != nil {
			rollbackErr := transaction.rollback()
			if rollbackErr != nil {
				return fmt.Errorf("提交配置事务失败: %v；回滚失败: %w", err, rollbackErr)
			}
			return fmt.Errorf("提交配置事务失败: %w", err)
		}
		return nil
	}
	if err := transaction.setPhase("reload_started"); err != nil {
		rollbackErr := transaction.rollback()
		if rollbackErr != nil {
			return fmt.Errorf("记录配置 reload 阶段失败: %v；回滚失败: %w", err, rollbackErr)
		}
		return fmt.Errorf("记录配置 reload 阶段失败: %w", err)
	}
	if err := configReload(ctx, options); err != nil {
		restoreErr := transaction.restore()
		if restoreErr != nil {
			return fmt.Errorf("配置 reload 失败: %v；恢复旧配置失败: %w", err, restoreErr)
		}
		if configProcessRunning(options.SingBoxPath) {
			if restoreErr := configRestoreReload(ctx, options, transaction.journal); restoreErr != nil {
				return fmt.Errorf("配置 reload 失败，且运行实例恢复失败: %v；%w", err, restoreErr)
			}
		}
		if cleanupErr := transaction.cleanup(); cleanupErr != nil {
			return fmt.Errorf("配置 reload 失败，旧配置已恢复但清理事务失败: %v；%w", err, cleanupErr)
		}
		return fmt.Errorf("配置 reload 失败，已恢复旧配置: %w", err)
	}
	if err := transaction.commit(); err != nil {
		return rollbackAfterCommitFailure(ctx, options, transaction, err)
	}
	return nil
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

// ValidateConfig 校验 module.conf、ebpf.conf 或 sing-box JSON。
func ValidateConfig(ctx context.Context, options Options, target, candidate string) error {
	switch target {
	case "module":
		_, err := moduleconfig.LoadModule(candidate)
		return err
	case "ebpf":
		_, err := ebpf.Load(candidate)
		return err
	}
	if !strings.HasPrefix(target, "singbox/") && !strings.HasPrefix(target, "runtime/") {
		return errors.New("不支持的配置目标")
	}
	if strings.HasPrefix(target, "runtime/") && !isRuntimeConfigName(strings.TrimPrefix(target, "runtime/")) {
		return errors.New("不支持的运行时文件")
	}
	if strings.HasPrefix(target, "singbox/") &&
		!strings.HasPrefix(target, "singbox/confdir/") &&
		!strings.HasPrefix(target, "singbox/rules/local/") {
		return errors.New("不支持的 sing-box 配置目标")
	}
	content, err := os.ReadFile(candidate)
	if err != nil {
		return err
	}
	if !json.Valid(content) {
		return errors.New("配置不是有效 JSON")
	}
	if strings.HasPrefix(target, "singbox/confdir/") {
		return validateSingBoxTree(ctx, options, target, candidate)
	}
	return nil
}

// validateSingBoxTree 在临时配置树中检查候选静态配置，避免直接覆盖用户正在使用的文件。
func validateSingBoxTree(ctx context.Context, options Options, target, candidate string) error {
	temporary, err := os.MkdirTemp("", "netproxy-config-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	for _, directory := range []struct{ source, destination string }{
		{paths.SingBoxConfDir(options.SingBoxDir), paths.SingBoxConfDir(temporary)},
		{paths.SingBoxRulesDir(options.SingBoxDir), paths.SingBoxRulesDir(temporary)},
	} {
		if err := copyDirectory(directory.source, directory.destination); err != nil {
			return err
		}
	}
	targetPath, err := ResolveConfig(options, target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(paths.SingBoxConfDir(options.SingBoxDir), targetPath)
	if err != nil {
		return err
	}
	candidatePath := filepath.Join(paths.SingBoxConfDir(temporary), relative)
	if err := os.MkdirAll(filepath.Dir(candidatePath), 0o700); err != nil {
		return err
	}
	if err := copyFile(candidatePath, candidate, 0o600); err != nil {
		return err
	}
	checkOptions := options
	checkOptions.RuntimeDir = filepath.Join(temporary, "runtime")
	prepared, err := Prepare(ctx, checkOptions, true)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, options.SingBoxPath, "check", "-C", paths.SingBoxConfDir(temporary),
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
	validSingBoxPath := len(parts) == 2 && parts[0] == "confdir" && filepath.Ext(parts[1]) == ".json" && parts[1] != "" && parts[1][0] != '.'
	validLocalRulePath := len(parts) == 3 && parts[0] == "rules" && parts[1] == "local" && filepath.Ext(parts[2]) == ".json" && parts[2] != "" && parts[2][0] != '.'
	if prefix == "singbox/" && !validSingBoxPath && !validLocalRulePath {
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
