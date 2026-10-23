package module

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
)

func interruptedStopFixture(t *testing.T, action, phase string) (Options, *configApplyTransaction, []byte) {
	t.Helper()
	options, destination, _, _ := configApplyOptions(t)
	isolateConfigApplyHooks(t, true)
	oldStop, oldBoot, oldWrite := configStop, configBootID, writeServiceState
	t.Cleanup(func() { configStop, configBootID, writeServiceState = oldStop, oldBoot, oldWrite })
	configBootID = func() string { return "boot-one" }
	before := targetCoreRead(t, destination)
	transaction, err := beginConfigApply(options, destination)
	if err != nil {
		t.Fatal(err)
	}
	transaction.journal.Action, transaction.journal.Mode = action, "Direct"
	if err := transaction.setPhase(phase); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte(`{"candidate":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	configStop = func(ctx context.Context, options Options) error {
		err := StopService(ctx, options)
		if err == nil {
			configProcessRunning = func(string) bool { return false }
		}
		return err
	}
	writeServiceState = func(path, state string, pid, startedAt, readyAt int64, message string, identity ...ServiceIdentity) error {
		if state == "preparing" || state == "starting" {
			t.Fatal("明确停服或恢复意外尝试启动核心")
		}
		return WriteServiceState(path, state, pid, startedAt, readyAt, message, identity...)
	}
	return options, transaction, before
}

func assertStoppedConfig(t *testing.T, options Options, before []byte) {
	t.Helper()
	state, err := ReadServiceState(options.StateFile)
	if err != nil || state.State != "stopped" || state.PID != 0 || state.ActiveBackend != "" {
		t.Fatalf("停服终态未收敛: %+v %v", state, err)
	}
	read, err := ReadConfig(options, "singbox/config.json")
	if err != nil || read["content"] != string(before) || read["revision"] != configRevision(before) {
		t.Fatal("磁盘或 revision 未恢复原快照")
	}
	for _, name := range []string{"providers.json", "inbound.json", configTransactionDirectory} {
		if _, err := os.Stat(filepath.Join(options.RuntimeDir, name)); !os.IsNotExist(err) {
			t.Fatalf("停服遗留运行时或 journal: %s %v", name, err)
		}
	}
}

func assertStopJournalBackups(t *testing.T, transaction *configApplyTransaction, keepStopped bool) configApplyJournal {
	t.Helper()
	var journal configApplyJournal
	if err := json.Unmarshal(targetCoreRead(t, transaction.journalPath), &journal); err != nil {
		t.Fatal(err)
	}
	if journal.KeepStopped != keepStopped || journal.Phase == "committed" || journal.Phase == "rolled_back" {
		t.Fatalf("恢复失败时 journal 状态不完整: %+v", journal)
	}
	for _, snapshots := range [][]configFileSnapshot{journal.Static, journal.Runtime, journal.Catalog} {
		for _, snapshot := range snapshots {
			if snapshot.Exists {
				if _, err := os.Stat(snapshot.Backup); err != nil {
					t.Fatal("恢复失败丢失备份", err)
				}
			}
		}
	}
	return journal
}

func TestExplicitStopRecoversInterruptedConfigWithoutRestart(t *testing.T) {
	for _, test := range []struct{ action, phase string }{
		{"reload", "prepared"}, {"reload", "reload_started"},
		{"switch", "switch_started"}, {"switch", "static_replaced"}, {"switch", "reload_started"},
		{"mode", "mode_started"},
	} {
		t.Run(test.action+"/"+test.phase, func(t *testing.T) {
			options, _, before := interruptedStopFixture(t, test.action, test.phase)
			result, err := ManageService(t.Context(), options, "stop")
			if err != nil || result.Status.State != "stopped" {
				t.Fatalf("公共 stop 失败: %+v %v", result, err)
			}
			assertStoppedConfig(t, options, before)
			revision, err := ApplyConfigContent(t.Context(), options, "module/auto_start", []byte(`{"auto_start":true}`), false, "")
			if err != nil {
				t.Fatal(err)
			}
			saved, err := ReadConfig(options, "module/auto_start")
			if err != nil || revision != saved["revision"] {
				t.Fatal("停服后普通保存 revision 不一致")
			}
			assertStoppedConfig(t, options, before)
			for range 2 {
				if err := recoverConfigApply(t.Context(), options); err != nil {
					t.Fatal(err)
				}
				if _, err := ManageService(t.Context(), options, "stop"); err != nil {
					t.Fatal(err)
				}
			}
			assertStoppedConfig(t, options, before)
			startReached := errors.New("显式 start 已通过旧事务恢复入口")
			writeServiceState = func(_ string, state string, _, _, _ int64, _ string, _ ...ServiceIdentity) error {
				if state == "preparing" {
					return startReached
				}
				return nil
			}
			if _, err := ManageService(t.Context(), options, "start"); !errors.Is(err, startReached) {
				t.Fatalf("后续显式 start 未被允许: %v", err)
			}
		})
	}
}

func TestExplicitStopPersistsIntentBeforeStopping(t *testing.T) {
	options, transaction, before := interruptedStopFixture(t, "reload", "reload_started")
	failure := errors.New("模拟停止意图落盘失败")
	configJournalWrite = func(string, []byte, os.FileMode) error { return failure }
	configStop = func(context.Context, Options) error {
		t.Fatal("停止意图落盘失败后不应停止进程或恢复磁盘")
		return nil
	}
	if _, err := ManageService(t.Context(), options, "stop"); !errors.Is(err, failure) {
		t.Fatal("停止意图落盘错误丢失", err)
	}
	assertStopJournalBackups(t, transaction, false)
	if bytes.Equal(targetCoreRead(t, transaction.journal.Static[0].Path), before) {
		t.Fatal("记录停止意图失败却已恢复磁盘")
	}
}

func TestExplicitStopInterruptedFinishingKeepsIntentForPublicApply(t *testing.T) {
	for _, stage := range []string{"stop", "restore", "runtime-cleanup", "stopped-state", "journal-finish"} {
		t.Run(stage, func(t *testing.T) {
			options, transaction, before := interruptedStopFixture(t, "mode", "mode_started")
			failure := errors.New("模拟明确停服中断")
			stop, snapshot, journalWrite, stateWrite := configStop, configSnapshotRestore, configJournalWrite, writeServiceState
			switch stage {
			case "stop":
				configStop = func(context.Context, Options) error { return failure }
			case "restore":
				configSnapshotRestore = func(configFileSnapshot) error { return failure }
			case "runtime-cleanup":
				configSnapshotRestore = func(snapshot configFileSnapshot) error {
					if filepath.Dir(snapshot.Path) == options.RuntimeDir && filepath.Base(snapshot.Path) == "providers.json" {
						return os.MkdirAll(filepath.Join(snapshot.Path, "block-removal"), 0o700)
					}
					return restoreConfigSnapshot(snapshot)
				}
			case "stopped-state":
				stoppedWrites := 0
				writeServiceState = func(path, state string, pid, startedAt, readyAt int64, message string, identity ...ServiceIdentity) error {
					if state == "stopped" {
						stoppedWrites++
						if stoppedWrites == 2 {
							return failure
						}
					}
					return stateWrite(path, state, pid, startedAt, readyAt, message, identity...)
				}
			case "journal-finish":
				configJournalWrite = func(path string, content []byte, mode os.FileMode) error {
					if bytes.Contains(content, []byte(`"phase":"rolled_back"`)) {
						return failure
					}
					return writeConfigAtomic(path, content, mode)
				}
			}
			if _, err := ManageService(t.Context(), options, "stop"); err == nil {
				t.Fatal("中断的停服误报成功")
			}
			assertStopJournalBackups(t, transaction, true)
			configStop, configSnapshotRestore, configJournalWrite, writeServiceState = stop, snapshot, journalWrite, stateWrite
			if stage == "runtime-cleanup" {
				if err := os.Remove(filepath.Join(options.RuntimeDir, "providers.json", "block-removal")); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(options.RuntimeDir, "providers.json")); err != nil {
					t.Fatal(err)
				}
			}
			// 模拟进程重启：只由公共 Apply 重新读取 journal，不复用原 transaction 指针。
			if _, err := ApplyConfigContent(t.Context(), options, "module/auto_start", []byte(`{"auto_start":false}`), false, ""); err != nil {
				t.Fatal("公共 Apply 未能保持停止并重试恢复", err)
			}
			assertStoppedConfig(t, options, before)
		})
	}
}

func TestExplicitStopCleanupFailedBootGuard(t *testing.T) {
	for _, wasRunning := range []bool{false, true} {
		for _, keepStopped := range []bool{false, true} {
			t.Run(fmt.Sprintf("running=%t/stopped=%t", wasRunning, keepStopped), func(t *testing.T) {
				options, transaction, before := interruptedStopFixture(t, "switch", "cleanup_failed")
				transaction.journal.WasRunning, transaction.journal.KeepStopped = wasRunning, keepStopped
				if err := transaction.writeJournal(); err != nil {
					t.Fatal(err)
				}
				journalBefore := targetCoreRead(t, transaction.journalPath)
				stop := configStop
				configStop = func(context.Context, Options) error {
					t.Fatal("同 boot cleanup_failed 守卫被绕过")
					return nil
				}
				for range 2 {
					if _, err := ManageService(t.Context(), options, "stop"); !errors.Is(err, ErrForcedTermination) {
						t.Fatal("同 boot 未拒绝强杀恢复", err)
					}
					if err := recoverConfigApply(t.Context(), options); !errors.Is(err, ErrForcedTermination) {
						t.Fatal("默认恢复绕过同 boot 守卫", err)
					}
				}
				if !bytes.Equal(journalBefore, targetCoreRead(t, transaction.journalPath)) {
					t.Fatal("同 boot 拒绝前改写了 journal")
				}
				assertStopJournalBackups(t, transaction, keepStopped)
				configStop = stop
				configBootID = func() string { return "boot-two" }
				if _, err := ManageService(t.Context(), options, "stop"); err != nil {
					t.Fatal("跨 boot 停服恢复失败", err)
				}
				assertStoppedConfig(t, options, before)
			})
		}
	}
}

func TestDefaultConfigRecoveryStillRestoresRuntime(t *testing.T) {
	options, transaction, _ := interruptedStopFixture(t, "reload", "reload_started")
	configProcessRunning = func(string) bool { return false }
	restored := false
	if err := restoreConfigApply(t.Context(), options, transaction, func(context.Context, Options, configApplyJournal) error {
		restored = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("默认恢复不应丢失恢复旧运行时的语义")
	}
}

func TestExplicitStopAcrossBootForcedAgainBlocksSameBootStart(t *testing.T) {
	for _, bootID := range []string{"boot-two", ""} {
		t.Run("new-boot="+bootID, func(t *testing.T) {
			options, transaction, before := interruptedStopFixture(t, "switch", "cleanup_failed")
			stop := configStop
			configBootID = func() string { return "boot-two" }
			configStop = func(context.Context, Options) error {
				configBootID = func() string { return bootID }
				return ErrForcedTermination
			}
			if _, err := ManageService(t.Context(), options, "stop"); !errors.Is(err, ErrForcedTermination) {
				t.Fatal("跨 boot 再次强杀未报错", err)
			}
			journal := assertStopJournalBackups(t, transaction, true)
			if journal.BootID != bootID || journal.Phase != "cleanup_failed" {
				t.Fatal("新强杀的 boot 与阶段未原子记录")
			}
			configBootID = func() string { return "boot-two" }
			configStop = func(context.Context, Options) error {
				t.Fatal("再次强杀后同 boot 不应再次停止或恢复")
				return nil
			}
			for range 2 {
				if _, err := ManageService(t.Context(), options, "start"); !errors.Is(err, ErrForcedTermination) {
					t.Fatal("新强杀后公共 start 绕过同 boot 守卫", err)
				}
				if err := recoverConfigApply(t.Context(), options); !errors.Is(err, ErrForcedTermination) {
					t.Fatal("新强杀后默认恢复绕过同 boot 守卫", err)
				}
			}
			if bytes.Equal(targetCoreRead(t, transaction.journal.Static[0].Path), before) {
				t.Fatal("再次强杀后不应恢复磁盘")
			}
			if bootID != "" {
				configBootID = func() string { return "boot-three" }
				configStop = stop
				if _, err := ManageService(t.Context(), options, "stop"); err != nil {
					t.Fatal("再次跨 boot 后未允许安全恢复", err)
				}
				assertStoppedConfig(t, options, before)
			}
		})
	}
}

func TestConfigRollbackForcedFailureRecordsCurrentBoot(t *testing.T) {
	options, transaction, _ := interruptedStopFixture(t, "reload", "reload_started")
	configBootID = func() string { return "boot-two" }
	if err := rollbackConfigApply(options, transaction, ErrForcedTermination); !errors.Is(err, ErrForcedTermination) {
		t.Fatal("默认 rollback 丢失强杀错误", err)
	}
	journal := assertStopJournalBackups(t, transaction, false)
	if journal.Phase != "cleanup_failed" || journal.BootID != "boot-two" {
		t.Fatal("默认 rollback 未持久记录当前强杀 boot")
	}
	for range 2 {
		if err := recoverConfigApply(t.Context(), options); !errors.Is(err, ErrForcedTermination) {
			t.Fatal("默认 rollback 后同 boot 守卫被绕过", err)
		}
	}
}

func TestExplicitStopToggleUsesRecoveryPolicy(t *testing.T) {
	options, _, before := interruptedStopFixture(t, "reload", "reload_started")
	if err := WriteServiceState(options.StateFile, "ready", 0, 1, 1, ""); err != nil {
		t.Fatal(err)
	}
	result, err := ManageService(t.Context(), options, "toggle")
	if err != nil || result.Action != "stop" {
		t.Fatal("toggle 未进入明确停服策略", err)
	}
	assertStoppedConfig(t, options, before)
}

func TestExplicitStopRestoresCatalogSnapshot(t *testing.T) {
	options, transaction, before := interruptedStopFixture(t, "reload", "reload_started")
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: "resource", Name: "Resource", Type: "local"}); err != nil {
		t.Fatal(err)
	}
	originals := make(map[string][]byte)
	for _, name := range []string{"meta.json", "provider.json"} {
		path := filepath.Join(options.CatalogRoot, "resource", name)
		originals[path] = targetCoreRead(t, path)
		snapshot, err := createConfigSnapshot(transaction.directory, name, path)
		if err != nil {
			t.Fatal(err)
		}
		transaction.journal.Catalog = append(transaction.journal.Catalog, snapshot)
		if err := os.WriteFile(path, []byte(`{"candidate":true}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := transaction.writeJournal(); err != nil {
		t.Fatal(err)
	}
	if _, err := ManageService(t.Context(), options, "stop"); err != nil {
		t.Fatal(err)
	}
	assertStoppedConfig(t, options, before)
	for path, content := range originals {
		if !bytes.Equal(targetCoreRead(t, path), content) {
			t.Fatal("停服未恢复 Catalog 完整快照")
		}
	}
}

func TestExplicitStopCompletedJournalForcedTerminationKeepsBootGuard(t *testing.T) {
	for _, phase := range []string{"committed", "rolled_back"} {
		t.Run(phase, func(t *testing.T) {
			options, transaction, before := interruptedStopFixture(t, "reload", "reload_started")
			if phase == "rolled_back" {
				if err := os.WriteFile(transaction.journal.Static[0].Path, before, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			diskBefore := targetCoreRead(t, transaction.journal.Static[0].Path)
			if err := transaction.setPhase(phase); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(transaction.journal.Static[0].Backup); err != nil {
				t.Fatal(err)
			}
			stop := configStop
			configStop = func(context.Context, Options) error { return ErrForcedTermination }
			if _, err := ManageService(t.Context(), options, "stop"); !errors.Is(err, ErrForcedTermination) {
				t.Fatal("终结态残留 journal 的强杀未报错", err)
			}
			var journal configApplyJournal
			if err := json.Unmarshal(targetCoreRead(t, transaction.journalPath), &journal); err != nil {
				t.Fatal(err)
			}
			if journal.Phase != "stop_cleanup_failed" || journal.BootID != "boot-one" || !journal.KeepStopped {
				t.Fatal("终结态强杀没有持久记录 boot 门禁")
			}
			for range 2 {
				if _, err := ManageService(t.Context(), options, "start"); !errors.Is(err, ErrForcedTermination) {
					t.Fatal("终结态强杀后公共 start 绕过同 boot 守卫", err)
				}
			}
			configBootID = func() string { return "boot-two" }
			configStop = stop
			if err := recoverConfigApply(t.Context(), options); err != nil {
				t.Fatal("终结态跨 boot 恢复不应依赖已删除的备份", err)
			}
			assertStoppedConfig(t, options, diskBefore)
		})
	}
}
