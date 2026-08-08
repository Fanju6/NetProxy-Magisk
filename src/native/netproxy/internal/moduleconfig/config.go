package moduleconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Read 读取简单的 KEY=value 配置，不执行配置内容。
func Read(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || !validKey(key) {
			continue
		}
		values[key] = decodeValue(value)
	}
	return values, nil
}

// ReadValue 读取一个配置值，键不存在时返回 fallback。
func ReadValue(path, key, fallback string) (string, error) {
	if !validKey(key) {
		return "", fmt.Errorf("非法配置键: %s", key)
	}
	values, err := Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fallback, nil
		}
		return "", err
	}
	if value, ok := values[key]; ok {
		return value, nil
	}
	return fallback, nil
}

// Update 原子更新若干 KEY=value，保留原文件的注释和键顺序。
func Update(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	for key := range updates {
		if !validKey(key) {
			return fmt.Errorf("非法配置键: %s", key)
		}
	}
	lockPath := path + ".lock"
	if err := acquireLock(lockPath); err != nil {
		return err
	}
	defer os.RemoveAll(lockPath)

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	written := make(map[string]bool, len(updates))
	for index, line := range lines {
		key, _, found := strings.Cut(line, "=")
		if found {
			if value, ok := updates[key]; ok {
				lines[index] = key + "=" + value
				written[key] = true
			}
		}
	}
	for key, value := range updates {
		if !written[key] {
			lines = append(lines, key+"="+value)
		}
	}
	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".module-conf-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = tmp.WriteString(updated); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Quote 生成与模块配置兼容的双引号值。
func Quote(value string) string {
	return strconv.Quote(value)
}

func validKey(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return false
	}
	return true
}

func decodeValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
	}
	return value
}

func acquireLock(path string) error {
	for attempt := 0; attempt < 50; attempt++ {
		if err := os.Mkdir(path, 0o700); err == nil {
			return nil
		} else if !os.IsExist(err) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("配置文件正忙: %s", path)
}
