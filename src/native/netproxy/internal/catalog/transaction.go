// Package catalog 提供 Catalog provider 与 meta 的双文件事务提交。
package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
)

const (
	stagingDirName = "staging"
	txPrefix       = "catalog-"
	journalName    = "journal"
	targetName     = "target"
)

// CommitPair 原子提交一组 Catalog 的 Provider 与元数据文件。
func CommitPair(ctx context.Context, root, groupDir string, providerContent, metadataContent []byte) error {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(groupDir) == "" {
		return errors.New("catalog transaction target is empty")
	}
	groupID := filepath.Base(filepath.Clean(groupDir))
	if !isValidGroupID(groupID) {
		return fmt.Errorf("非法 Catalog 分组 ID: %s", groupID)
	}
	release, err := acquireCatalogMutation(ctx, root, groupID)
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
		return errors.Join(err, rollback(txDir))
	}
	if err := moveExisting(groupDir, txDir, "meta.json"); err != nil {
		return errors.Join(err, rollback(txDir))
	}
	if err := install(txDir, groupDir, "provider.json"); err != nil {
		return errors.Join(err, rollback(txDir))
	}
	if err := install(txDir, groupDir, "meta.json"); err != nil {
		return errors.Join(err, rollback(txDir))
	}
	if err := appendSynced(journalPath, []byte("commit\n")); err != nil {
		// commit 可能已写入但未同步，回滚前必须先持久化未提交状态。
		if resetErr := writeSynced(journalPath, []byte("begin\nprovider\nmeta\n"), 0o600); resetErr != nil {
			return errors.Join(err, resetErr)
		}
		return errors.Join(err, rollback(txDir))
	}
	// 数据已提交，临时文件清理失败不改变提交结果；下次恢复继续清理。
	cleanupCompletedTransaction(txDir)
	return nil
}

// Recover 清理启动前遗留的 Catalog 事务目录并恢复未完成提交。
func Recover(ctx context.Context, root string) error {
	release, err := AcquireRoot(ctx, root)
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
		if errors.Is(err, os.ErrNotExist) {
			return discardUnstartedTransaction(txDir, err)
		}
		return err
	}
	lines := strings.Split(strings.TrimRight(string(journaling), "\r\n"), "\n")
	if len(lines) < 3 || lines[0] != "begin" || lines[1] != "provider" || lines[2] != "meta" {
		return discardUnstartedTransaction(txDir, errors.New("Catalog 事务日志不完整"))
	}
	if slices.Contains(lines[3:], "commit") || slices.Contains(lines[3:], "rolled_back") {
		cleanupCompletedTransaction(txDir)
		return nil
	}
	return rollback(txDir)
}

func discardUnstartedTransaction(txDir string, cause error) error {
	for _, name := range []string{"provider.json.bak", "meta.json.bak"} {
		if _, err := os.Stat(filepath.Join(txDir, name)); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(cause, err)
		}
	}
	return cleanupTransaction(txDir)
}

func moveExisting(groupDir, txDir, name string) error {
	source := filepath.Join(groupDir, name)
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Catalog 事务目标不是文件: %s", source)
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

func rollback(txDir string) error {
	target, err := os.ReadFile(filepath.Join(txDir, targetName))
	if err != nil {
		return err
	}
	groupDir := strings.TrimSpace(string(target))
	if groupDir == "" {
		return errors.New("Catalog 事务缺少恢复目标")
	}
	for _, name := range []string{"provider.json", "meta.json"} {
		finalPath := filepath.Join(groupDir, name)
		backupPath := filepath.Join(txDir, name+".bak")
		if _, err := os.Stat(backupPath); err == nil {
			// 备份直到整组恢复成功才删除，否则二次恢复会把已恢复文件误认为新增文件。
			if err := restoreTransactionFile(backupPath, finalPath); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else if _, err := os.Stat(filepath.Join(txDir, name)); errors.Is(err, os.ErrNotExist) {
			if err := os.Remove(finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	// 清理可能再次中断，先记录恢复完成，防止部分备份已删除后重复回滚。
	if err := appendSynced(filepath.Join(txDir, journalName), []byte("rolled_back\n")); err != nil {
		return err
	}
	cleanupCompletedTransaction(txDir)
	return nil
}

func cleanupTransaction(txDir string) error {
	// 完成标记必须晚于所有备份删除，否则清理中断会留下无法判定的事务。
	for _, name := range []string{"provider.json.bak", "meta.json.bak"} {
		if err := os.Remove(filepath.Join(txDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.RemoveAll(txDir)
}

func cleanupCompletedTransaction(txDir string) {
	if err := cleanupTransaction(txDir); err != nil {
		fmt.Fprintln(os.Stderr, logfile.FormatEntry(logfile.Entry{
			Level: "WARN", Component: "catalog", Event: "transaction.cleanup", Result: "pending",
			Message: fmt.Sprintf("事务已完成，临时文件清理将重试: %v", err),
		}))
	}
}

func restoreTransactionFile(backup, target string) error {
	source, err := os.Open(backup)
	if err != nil {
		return err
	}
	defer source.Close()
	file, err := os.CreateTemp(filepath.Dir(target), ".catalog-restore-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, copyErr := io.Copy(file, source)
	if err := errors.Join(copyErr, file.Chmod(0o600), file.Sync(), file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), target)
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
