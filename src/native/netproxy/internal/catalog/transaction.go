// Package catalog 提供 Catalog provider 与 meta 的双文件事务提交。
package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	stagingDirName = "staging"
	txPrefix       = "catalog-"
	journalName    = "journal"
	targetName     = "target"
)

var transactionMu sync.Mutex

// CommitPair 原子提交一组 Catalog 的 Provider 与元数据文件。
func CommitPair(root, groupDir string, providerContent, metadataContent []byte) error {
	transactionMu.Lock()
	defer transactionMu.Unlock()
	if strings.TrimSpace(root) == "" || strings.TrimSpace(groupDir) == "" {
		return errors.New("catalog transaction target is empty")
	}
	if err := os.MkdirAll(filepath.Join(root, stagingDirName), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		return err
	}
	txDir, err := os.MkdirTemp(filepath.Join(root, stagingDirName), txPrefix)
	if err != nil {
		return err
	}

	journalPath := filepath.Join(txDir, journalName)
	if err := writeSynced(filepath.Join(txDir, targetName), []byte(groupDir+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(txDir)
		return err
	}
	if err := writeSynced(journalPath, []byte("begin\nprovider\nmeta\n"), 0o600); err != nil {
		_ = os.RemoveAll(txDir)
		return err
	}
	if err := writeSynced(filepath.Join(txDir, "provider.json"), providerContent, 0o600); err != nil {
		_ = os.RemoveAll(txDir)
		return err
	}
	if err := writeSynced(filepath.Join(txDir, "meta.json"), metadataContent, 0o600); err != nil {
		_ = os.RemoveAll(txDir)
		return err
	}

	if err := moveExisting(groupDir, txDir, "provider.json"); err != nil {
		rollback(txDir)
		return err
	}
	if err := moveExisting(groupDir, txDir, "meta.json"); err != nil {
		rollback(txDir)
		return err
	}
	if err := install(txDir, groupDir, "provider.json"); err != nil {
		rollback(txDir)
		return err
	}
	if err := install(txDir, groupDir, "meta.json"); err != nil {
		rollback(txDir)
		return err
	}
	if err := appendSynced(journalPath, []byte("commit\n")); err != nil {
		rollback(txDir)
		return err
	}
	return os.RemoveAll(txDir)
}

// Recover 清理启动前遗留的 Catalog 事务目录并恢复未完成提交。
func Recover(root string) error {
	transactionMu.Lock()
	defer transactionMu.Unlock()
	staging := filepath.Join(root, stagingDirName)
	entries, err := os.ReadDir(staging)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var firstErr error
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), txPrefix) {
			continue
		}
		txDir := filepath.Join(staging, entry.Name())
		if err := recoverTransaction(txDir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func recoverTransaction(txDir string) error {
	journaling, err := os.ReadFile(filepath.Join(txDir, journalName))
	if err != nil {
		return os.RemoveAll(txDir)
	}
	lines := strings.Split(strings.TrimRight(string(journaling), "\r\n"), "\n")
	if len(lines) < 3 || lines[0] != "begin" || lines[1] != "provider" || lines[2] != "meta" {
		return os.RemoveAll(txDir)
	}
	if len(lines) >= 4 && lines[3] == "commit" {
		return os.RemoveAll(txDir)
	}
	target, err := os.ReadFile(filepath.Join(txDir, targetName))
	if err != nil || strings.TrimSpace(string(target)) == "" {
		return os.RemoveAll(txDir)
	}
	groupDir := strings.TrimSpace(string(target))
	for _, name := range []string{"provider.json", "meta.json"} {
		finalPath := filepath.Join(groupDir, name)
		backupPath := filepath.Join(txDir, name+".bak")
		if transactionFileExists(backupPath) {
			if err := os.Remove(finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.Rename(backupPath, finalPath); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(txDir, name)); errors.Is(err, os.ErrNotExist) {
			if err := os.Remove(finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return os.RemoveAll(txDir)
}

func moveExisting(groupDir, txDir, name string) error {
	source := filepath.Join(groupDir, name)
	if !transactionFileExists(source) {
		return nil
	}
	return os.Rename(source, filepath.Join(txDir, name+".bak"))
}

func install(txDir, groupDir, name string) error {
	source := filepath.Join(txDir, name)
	target := filepath.Join(groupDir, name)
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("install %s: %w", name, err)
	}
	return nil
}

func rollback(txDir string) {
	target, err := os.ReadFile(filepath.Join(txDir, targetName))
	if err != nil {
		_ = os.RemoveAll(txDir)
		return
	}
	groupDir := strings.TrimSpace(string(target))
	for _, name := range []string{"provider.json", "meta.json"} {
		finalPath := filepath.Join(groupDir, name)
		backupPath := filepath.Join(txDir, name+".bak")
		if transactionFileExists(backupPath) {
			_ = os.Remove(finalPath)
			_ = os.Rename(backupPath, finalPath)
		} else if _, err := os.Stat(filepath.Join(txDir, name)); errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(finalPath)
		}
	}
	_ = os.RemoveAll(txDir)
}

func writeSynced(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func appendSynced(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func transactionFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
