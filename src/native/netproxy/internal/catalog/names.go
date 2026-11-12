package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func ValidProviderName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name &&
		!strings.ContainsAny(name, "/\\") && strings.IndexFunc(name, unicode.IsControl) < 0
}

// AllocateNameLocked reserves a name in the same critical section as the metadata write.
func AllocateNameLocked(root, id, name string, reserved []string) (string, error) {
	name = strings.TrimSpace(name)
	if !ValidProviderName(name) {
		return "", fmt.Errorf("资源名称不能为空或包含斜线及控制字符")
	}
	used := make(map[string]bool)
	for _, tag := range reserved {
		base, _, _ := strings.Cut(tag, "/")
		used[base] = true
	}
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	for _, entry := range entries {
		if !isGroupDir(entry) || entry.Name() == id {
			continue
		}
		metadata, err := LoadMetadataLocked(filepath.Join(root, entry.Name(), "meta.json"), entry.Name())
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		used[metadata.Name] = true
	}
	candidate := name
	for number := 2; used[candidate]; number++ {
		candidate = fmt.Sprintf("%s (%d)", name, number)
	}
	return candidate, nil
}
