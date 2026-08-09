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
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
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
		dir      string
		category string
		editable bool
	}{
		{filepath.Join(options.SingBoxDir, "confdir"), "config", true},
		{filepath.Join(options.SingBoxDir, "source"), "source", true},
		{options.RuntimeDir, "runtime", false},
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
			pathCategory := item.category
			pathPrefix := "singbox/"
			if pathCategory == "config" {
				pathCategory = "confdir"
			}
			if item.category == "runtime" {
				pathPrefix = "runtime/"
				pathCategory = ""
			}
			result = append(result, ConfigDocument{ID: pathPrefix + filepath.ToSlash(filepath.Join(pathCategory, entry.Name())), Filename: entry.Name(), Category: item.category, Editable: item.editable})
		}
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
func ApplyConfig(ctx context.Context, options Options, target, source string, validateOnly bool) error {
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
	if err := ValidateConfig(ctx, options, target, candidatePath); err != nil {
		return err
	}
	if validateOnly {
		return nil
	}
	if err := os.Rename(candidatePath, destination); err != nil {
		return err
	}
	if service.ProcessRunning(options.SingBoxPath) {
		if err := runServiceAdapter(ctx, options, "reload"); err != nil {
			return fmt.Errorf("配置已保存，但服务 reload 失败: %w", err)
		}
	}
	return nil
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
	for _, name := range []string{"confdir", "source"} {
		if err := copyDirectory(filepath.Join(options.SingBoxDir, name), filepath.Join(temporary, name)); err != nil {
			return err
		}
	}
	targetPath, err := ResolveConfig(options, target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(filepath.Join(options.SingBoxDir, "confdir"), targetPath)
	if err != nil {
		return err
	}
	candidatePath := filepath.Join(temporary, "confdir", relative)
	if err := os.MkdirAll(filepath.Dir(candidatePath), 0o700); err != nil {
		return err
	}
	if err := copyFile(candidatePath, candidate, 0o600); err != nil {
		return err
	}
	prepared, err := Prepare(ctx, options, true)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, options.SingBoxPath, "check", "-C", filepath.Join(temporary, "confdir"),
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
	if prefix == "singbox/" && (len(parts) != 2 || (parts[0] != "confdir" && parts[0] != "source") || filepath.Ext(parts[1]) != ".json" || parts[1] == "" || parts[1][0] == '.') {
		return "", errors.New("配置目标路径无效")
	}
	if prefix == "runtime/" && (len(parts) != 1 || filepath.Ext(parts[0]) != ".json" || parts[0] == "" || parts[0][0] == '.') {
		return "", errors.New("配置目标路径无效")
	}
	name := parts[len(parts)-1]
	for _, char := range name {
		if !(char == '.' || char == '-' || char == '_' || char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z') {
			return "", errors.New("配置文件名无效")
		}
	}
	return filepath.Join(root, relative), nil
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
