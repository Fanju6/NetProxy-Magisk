// Package catalog 提供 Catalog provider 与 meta 的双文件事务提交。
package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	stagingDirName = "staging"
	txPrefix       = "catalog-"
	journalName    = "journal"
	targetName     = "target"
)

// CommitPair 原子提交一组 Catalog 的 Provider 与元数据文件。
func CommitPair(root, groupDir string, providerContent, metadataContent []byte) error {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(groupDir) == "" {
		return errors.New("catalog transaction target is empty")
	}
	groupID := filepath.Base(filepath.Clean(groupDir))
	if !isValidGroupID(groupID) {
		return fmt.Errorf("非法 Catalog 分组 ID: %s", groupID)
	}
	release, err := acquireCatalogMutation(root, groupID)
	if err != nil {
		return err
	}
	defer release()
	return commitPairLocked(root, groupDir, providerContent, metadataContent)
}

// CommitPairLocked 在调用方已持有分组锁和根锁时提交事务。
func CommitPairLocked(root, groupDir string, providerContent, metadataContent []byte) error {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(groupDir) == "" {
		return errors.New("catalog transaction target is empty")
	}
	return commitPairLocked(root, groupDir, providerContent, metadataContent)
}

func commitPairLocked(root, groupDir string, providerContent, metadataContent []byte) error {
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
	if err := writeSynced(journalPath, []byte("begin\nprovider\nmeta\n"+lockOwnerJournal()), 0o600); err != nil {
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
	release, err := AcquireRoot(root)
	if err != nil {
		return err
	}
	defer release()
	return recoverTransactionsLocked(root)
}

// RecoverLocked 在调用方已持有根锁时恢复遗留事务。
func RecoverLocked(root string) error {
	return recoverTransactionsLocked(root)
}

func recoverTransactionsLocked(root string) error {
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
	for _, line := range lines[3:] {
		if line == "commit" {
			return os.RemoveAll(txDir)
		}
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
	if err := os.Rename(source, filepath.Join(txDir, name+".bak")); err != nil {
		return err
	}
	transactionRenameHook("move-" + name)
	return nil
}

func install(txDir, groupDir, name string) error {
	source := filepath.Join(txDir, name)
	target := filepath.Join(groupDir, name)
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("install %s: %w", name, err)
	}
	transactionRenameHook("install-" + name)
	return nil
}

// transactionRenameHook 仅供同包测试模拟 rename 完成后的进程中断。
var transactionRenameHook = func(string) {}

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
