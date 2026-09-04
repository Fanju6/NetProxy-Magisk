package module

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

const configTransactionDirectory = ".config-apply"

var configJournalWrite = writeConfigAtomic
var configSnapshotRestore = restoreConfigSnapshot

type configFileSnapshot struct {
	Path   string `json:"path"`
	Backup string `json:"backup,omitempty"`
	Exists bool   `json:"exists"`
	Mode   uint32 `json:"mode,omitzero"`
}

type configApplyJournal struct {
	Version int                  `json:"version"`
	Phase   string               `json:"phase"`
	Static  []configFileSnapshot `json:"static"`
	Runtime []configFileSnapshot `json:"runtime"`
}

type configApplyTransaction struct {
	directory   string
	journal     configApplyJournal
	journalPath string
}

func configTransactionPath(options Options) string {
	return filepath.Join(options.RuntimeDir, configTransactionDirectory)
}

func beginConfigApply(options Options, destination string) (*configApplyTransaction, error) {
	if err := os.MkdirAll(options.RuntimeDir, 0o700); err != nil {
		return nil, err
	}
	directory := configTransactionPath(options)
	if _, err := os.Stat(directory); err == nil {
		return nil, errors.New("存在未完成的配置应用事务")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return nil, err
	}

	transaction := &configApplyTransaction{
		directory:   directory,
		journalPath: filepath.Join(directory, "journal.json"),
		journal: configApplyJournal{
			Version: 1,
			Phase:   "prepared",
			Static:  make([]configFileSnapshot, 0, 2),
			Runtime: make([]configFileSnapshot, 0, 3),
		},
	}
	staticPaths := uniqueConfigPaths(destination, options.ModuleConfig)
	for index, path := range staticPaths {
		snapshot, err := createConfigSnapshot(directory, fmt.Sprintf("static-%d", index), path)
		if err != nil {
			_ = os.RemoveAll(directory)
			return nil, err
		}
		transaction.journal.Static = append(transaction.journal.Static, snapshot)
	}
	for index, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		path := filepath.Join(options.RuntimeDir, name)
		snapshot, err := createConfigSnapshot(directory, fmt.Sprintf("runtime-%d", index), path)
		if err != nil {
			_ = os.RemoveAll(directory)
			return nil, err
		}
		transaction.journal.Runtime = append(transaction.journal.Runtime, snapshot)
	}
	if err := transaction.writeJournal(); err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	return transaction, nil
}

func uniqueConfigPaths(paths ...string) []string {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func createConfigSnapshot(directory, name, path string) (configFileSnapshot, error) {
	snapshot := configFileSnapshot{Path: filepath.Clean(path)}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	if info.IsDir() {
		return snapshot, fmt.Errorf("配置快照目标不是文件: %s", path)
	}
	backup := filepath.Join(directory, name+".bak")
	if err := copyFile(backup, path, info.Mode().Perm()); err != nil {
		return snapshot, err
	}
	snapshot.Backup = backup
	snapshot.Exists = true
	snapshot.Mode = uint32(info.Mode().Perm())
	return snapshot, nil
}

func (transaction *configApplyTransaction) setPhase(phase string) error {
	transaction.journal.Phase = phase
	return transaction.writeJournal()
}

func (transaction *configApplyTransaction) writeJournal() error {
	content, err := json.Marshal(transaction.journal, json.Deterministic(true))
	if err != nil {
		return err
	}
	return configJournalWrite(transaction.journalPath, content, 0o600)
}

func (transaction *configApplyTransaction) commit() error {
	if err := transaction.setPhase("committed"); err != nil {
		return err
	}
	_ = os.RemoveAll(transaction.directory)
	return nil
}

func (transaction *configApplyTransaction) restore() error {
	return restoreConfigSnapshots(transaction.journal)
}

func (transaction *configApplyTransaction) cleanup() error {
	return os.RemoveAll(transaction.directory)
}

func (transaction *configApplyTransaction) rollback() error {
	restoreErr := transaction.restore()
	if cleanupErr := transaction.cleanup(); restoreErr == nil {
		restoreErr = cleanupErr
	}
	return restoreErr
}

func recoverConfigApply(ctx context.Context, options Options) error {
	directory := configTransactionPath(options)
	content, err := os.ReadFile(filepath.Join(directory, "journal.json"))
	if os.IsNotExist(err) {
		if removeErr := os.RemoveAll(directory); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		return nil
	}
	if err != nil {
		return err
	}
	var journal configApplyJournal
	if err := json.Unmarshal(content, &journal); err != nil {
		return fmt.Errorf("读取配置应用事务失败: %w", err)
	}
	if journal.Version != 1 || journal.Phase == "" {
		return errors.New("配置应用事务版本或阶段无效")
	}
	if journal.Phase == "committed" {
		return os.RemoveAll(directory)
	}
	if err := validateConfigJournal(options, journal); err != nil {
		return err
	}
	if err := restoreConfigSnapshots(journal); err != nil {
		return fmt.Errorf("恢复配置应用事务失败: %w", err)
	}
	if journal.Phase == "reload_started" && configProcessRunning(options.SingBoxPath) {
		prepared := prepareFromConfigJournal(options, journal)
		if err := reloadPreparedService(ctx, options, prepared, false); err != nil {
			return fmt.Errorf("恢复配置后重新加载旧运行时失败: %w", err)
		}
	}
	return os.RemoveAll(directory)
}

func validateConfigJournal(options Options, journal configApplyJournal) error {
	allowedStatic := make(map[string]struct{})
	for _, path := range uniqueConfigPaths(options.ModuleConfig, options.EBPFConfig) {
		allowedStatic[path] = struct{}{}
	}
	allowedRuntime := make(map[string]struct{})
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		allowedRuntime[filepath.Clean(filepath.Join(options.RuntimeDir, name))] = struct{}{}
	}
	for _, snapshot := range journal.Static {
		path := filepath.Clean(snapshot.Path)
		if _, ok := allowedStatic[path]; !ok && !isSingBoxStaticPath(options.SingBoxDir, path) {
			return fmt.Errorf("配置事务静态目标不匹配: %s", snapshot.Path)
		}
	}
	for _, snapshot := range journal.Runtime {
		if _, ok := allowedRuntime[filepath.Clean(snapshot.Path)]; !ok {
			return fmt.Errorf("配置事务运行时目标不匹配: %s", snapshot.Path)
		}
	}
	return nil
}

func isSingBoxStaticPath(singBoxDir, path string) bool {
	path = filepath.Clean(path)
	if path == filepath.Clean(paths.SingBoxConfig(singBoxDir)) {
		return true
	}
	relative, err := filepath.Rel(paths.SingBoxLocalRulesDir(singBoxDir), path)
	return err == nil && filepath.Dir(relative) == "." && filepath.Ext(relative) == ".json"
}

func restoreConfigSnapshots(journal configApplyJournal) error {
	var restoreErr error
	for _, snapshot := range append(append([]configFileSnapshot{}, journal.Static...), journal.Runtime...) {
		if err := configSnapshotRestore(snapshot); err != nil && restoreErr == nil {
			restoreErr = err
		}
	}
	return restoreErr
}

func restoreConfigSnapshot(snapshot configFileSnapshot) error {
	if !snapshot.Exists {
		if err := os.Remove(snapshot.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if snapshot.Backup == "" {
		return fmt.Errorf("配置快照缺少备份: %s", snapshot.Path)
	}
	info, err := os.Stat(snapshot.Backup)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("配置快照备份不是文件: %s", snapshot.Backup)
	}
	if err := os.MkdirAll(filepath.Dir(snapshot.Path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(snapshot.Path), ".config-restore-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if closeErr := temporary.Close(); closeErr != nil {
		_ = os.Remove(temporaryPath)
		return closeErr
	}
	defer os.Remove(temporaryPath)
	if err := copyFile(temporaryPath, snapshot.Backup, fs.FileMode(snapshot.Mode)); err != nil {
		return err
	}
	return os.Rename(temporaryPath, snapshot.Path)
}

func prepareFromConfigJournal(options Options, journal configApplyJournal) PrepareResult {
	prepared := PrepareResult{}
	for _, snapshot := range journal.Runtime {
		switch filepath.Base(snapshot.Path) {
		case "providers.json":
			prepared.Providers = snapshot.Path
		case "outbounds.json":
			prepared.Outbounds = snapshot.Path
		case "ebpf.json":
			prepared.EBPF = snapshot.Path
		}
	}
	return prepared
}

func writeConfigAtomic(path string, content []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config-journal-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
