package module

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
)

var fakeSingBoxBuild struct {
	sync.Once
	path string
	err  error
}

func fakeSingBox(t *testing.T) string {
	t.Helper()
	fakeSingBoxBuild.Do(func() {
		_, file, _, callerOK := runtime.Caller(0)
		if !callerOK {
			fakeSingBoxBuild.err = errors.New("无法定位配置事务测试目录")
			return
		}
		directory, directoryErr := os.MkdirTemp("", "netproxy-fake-sing-box-")
		if directoryErr != nil {
			fakeSingBoxBuild.err = directoryErr
			return
		}
		output, outputErr := os.CreateTemp(directory, "fake-sing-box-")
		if outputErr != nil {
			fakeSingBoxBuild.err = outputErr
			return
		}
		fakeSingBoxBuild.path = output.Name()
		if err := output.Close(); err != nil {
			fakeSingBoxBuild.err = err
			return
		}
		if runtime.GOOS == "windows" {
			fakeSingBoxBuild.path += ".exe"
		}
		command := exec.Command("go", "build", "-o", fakeSingBoxBuild.path, "./testdata/fake-sing-box")
		command.Dir = filepath.Dir(file)
		if output, err := command.CombinedOutput(); err != nil {
			fakeSingBoxBuild.err = errors.New(string(output))
		}
	})
	if fakeSingBoxBuild.err != nil {
		t.Fatal(fakeSingBoxBuild.err)
	}
	return fakeSingBoxBuild.path
}

func fakeAndroidPackages(t *testing.T) {
	t.Helper()
	directory := t.TempDir()
	binary := filepath.Join(directory, "cmd")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	command := exec.Command("go", "build", "-o", binary, "../inbound/testdata/fake-package")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("构建应用查询桩: %v %s", err, output)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NETPROXY_TEST_COMMAND_MODE", "packages")
}

func withFakeSingBoxResult(t *testing.T, failed bool, run func()) {
	t.Helper()
	old, present := os.LookupEnv("NETPROXY_FAKE_SING_BOX_FAIL")
	value := "0"
	if failed {
		value = "1"
	}
	if err := os.Setenv("NETPROXY_FAKE_SING_BOX_FAIL", value); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if present {
			_ = os.Setenv("NETPROXY_FAKE_SING_BOX_FAIL", old)
		} else {
			_ = os.Unsetenv("NETPROXY_FAKE_SING_BOX_FAIL")
		}
	}()
	run()
}

func configApplyOptions(t *testing.T) (Options, string, string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	options := newTestOptions(root)
	options.StateFile = filepath.Join(root, "state", "service.json")
	options.LogDir = filepath.Join(root, "logs")
	options.SingBoxPath = fakeSingBox(t)
	if err := os.MkdirAll(options.SingBoxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.CatalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.InboundConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.InboundConfig, []byte(testInboundConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(options.SingBoxDir, "config.json")
	oldStatic := "{\"version\":1,\"old\":true}\n"
	newStatic := "{\"version\":1,\"new\":true}\n"
	if err := os.WriteFile(destination, []byte(oldStatic), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "new-config.json")
	if err := os.WriteFile(source, []byte(newStatic), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeContent := map[string]string{}
	for _, name := range []string{"providers.json", "outbounds.json", "inbound.json"} {
		content := "old-" + name + "\n"
		runtimeContent[name] = content
		if err := os.MkdirAll(options.RuntimeDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(options.RuntimeDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return options, destination, source, runtimeContent
}

func isolateConfigApplyHooks(t *testing.T, running bool) {
	t.Helper()
	originalRunning, originalReload, originalRestore, originalJournalWrite, originalSnapshotRestore := configProcessRunning, configReload, configRestoreReload, configJournalWrite, configSnapshotRestore
	configProcessRunning = func(string) bool { return running }
	t.Cleanup(func() {
		configProcessRunning = originalRunning
		configReload = originalReload
		configRestoreReload = originalRestore
		configJournalWrite = originalJournalWrite
		configSnapshotRestore = originalSnapshotRestore
	})
}

func failCommittedJournalWrite(t *testing.T) {
	t.Helper()
	configJournalWrite = func(path string, content []byte, mode os.FileMode) error {
		if bytes.Contains(content, []byte(`"phase":"committed"`)) {
			return errors.New("模拟 committed journal 写入失败")
		}
		return writeConfigAtomic(path, content, mode)
	}
}

func assertRuntimeContent(t *testing.T, options Options, expected map[string]string) {
	t.Helper()
	for name, want := range expected {
		content, err := os.ReadFile(filepath.Join(options.RuntimeDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != want {
			t.Fatalf("runtime %s = %q, want %q", name, content, want)
		}
	}
}

func TestApplyConfigValidateOnlyDoesNotModifyRuntime(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	withFakeSingBoxResult(t, false, func() {
		if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, true, ""); err != nil {
			t.Fatalf("validate config: %v", err)
		}
	})
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"version\":1,\"old\":true}\n" {
		t.Fatalf("static config changed during validation: %q", content)
	}
	assertRuntimeContent(t, options, runtimeContent)
}

func TestCheckServiceUsesTemporaryRuntime(t *testing.T) {
	options, _, _, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	withFakeSingBoxResult(t, false, func() {
		if err := CheckService(context.Background(), options); err != nil {
			t.Fatalf("check service: %v", err)
		}
	})
	assertRuntimeContent(t, options, runtimeContent)
}

func TestApplyConfigSingBoxCheckFailureDoesNotReplaceConfig(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	withFakeSingBoxResult(t, true, func() {
		if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, ""); err == nil {
			t.Fatal("sing-box check failure was accepted")
		}
	})
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"version\":1,\"old\":true}\n" {
		t.Fatalf("static config replaced after failed check: %q", content)
	}
	assertRuntimeContent(t, options, runtimeContent)
}

func TestApplyConfigSavesWhenServiceIsStopped(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	withFakeSingBoxResult(t, false, func() {
		if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, ""); err != nil {
			t.Fatalf("apply config: %v", err)
		}
	})
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"version\":1,\"new\":true}\n" {
		t.Fatalf("new static config was not saved: %q", content)
	}
	assertRuntimeContent(t, options, runtimeContent)
	if _, err := os.Stat(configTransactionPath(options)); !os.IsNotExist(err) {
		t.Fatalf("completed transaction remains: %v", err)
	}
}

func TestApplyConfigReloadSuccessCommitsRuntime(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, true)
	configReload = func(_ context.Context, options Options) error {
		for name := range runtimeContent {
			if err := os.WriteFile(filepath.Join(options.RuntimeDir, name), []byte("new-"+name+"\n"), 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	withFakeSingBoxResult(t, false, func() {
		if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, ""); err != nil {
			t.Fatalf("apply and reload config: %v", err)
		}
	})
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"version\":1,\"new\":true}\n" {
		t.Fatalf("new static config was not committed: %q", content)
	}
	newRuntime := map[string]string{}
	for name := range runtimeContent {
		newRuntime[name] = "new-" + name + "\n"
	}
	assertRuntimeContent(t, options, newRuntime)
}

func TestApplyConfigReloadFailureRestoresStaticAndRuntime(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, true)
	restored := false
	configReload = func(_ context.Context, options Options) error {
		for name := range runtimeContent {
			if err := os.WriteFile(filepath.Join(options.RuntimeDir, name), []byte("new-"+name+"\n"), 0o600); err != nil {
				return err
			}
		}
		return errors.New("模拟 reload/API 失败")
	}
	configRestoreReload = func(_ context.Context, _ Options, _ configApplyJournal) error {
		restored = true
		return nil
	}
	withFakeSingBoxResult(t, false, func() {
		if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, ""); err == nil {
			t.Fatal("reload failure was accepted")
		}
	})
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"version\":1,\"old\":true}\n" {
		t.Fatalf("static config was not restored: %q", content)
	}
	assertRuntimeContent(t, options, runtimeContent)
	if !restored {
		t.Fatal("running instance restore was not attempted")
	}
}

func TestApplyConfigCommitFailureRestoresAndReloadsOldRuntime(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, true)
	configReload = func(_ context.Context, options Options) error {
		for name := range runtimeContent {
			if err := os.WriteFile(filepath.Join(options.RuntimeDir, name), []byte("new-"+name+"\n"), 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	reloadCalled := false
	configRestoreReload = func(_ context.Context, _ Options, _ configApplyJournal) error {
		reloadCalled = true
		assertRuntimeContent(t, options, runtimeContent)
		return nil
	}
	failCommittedJournalWrite(t)
	withFakeSingBoxResult(t, false, func() {
		if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, ""); err == nil || !strings.Contains(err.Error(), "提交配置事务失败") {
			t.Fatalf("unexpected committed journal failure: %v", err)
		}
	})
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"version\":1,\"old\":true}\n" {
		t.Fatalf("static config was not restored after commit failure: %q", content)
	}
	assertRuntimeContent(t, options, runtimeContent)
	if !reloadCalled {
		t.Fatal("old runtime reload was not called after commit failure")
	}
}

func TestApplyConfigCommitFailureReportsRollbackErrors(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, true)
	configReload = func(_ context.Context, options Options) error {
		for name := range runtimeContent {
			if err := os.WriteFile(filepath.Join(options.RuntimeDir, name), []byte("new-"+name+"\n"), 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	configSnapshotRestore = func(snapshot configFileSnapshot) error {
		if err := restoreConfigSnapshot(snapshot); err != nil {
			return err
		}
		if filepath.Base(snapshot.Path) == "config.json" {
			return errors.New("模拟磁盘恢复失败")
		}
		return nil
	}
	configRestoreReload = func(_ context.Context, _ Options, _ configApplyJournal) error {
		t.Fatal("磁盘恢复失败时不能 reload 半恢复配置")
		return nil
	}
	failCommittedJournalWrite(t)
	withFakeSingBoxResult(t, false, func() {
		_, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, "")
		if err == nil {
			t.Fatal("commit rollback failure was accepted")
		}
		message := err.Error()
		for _, expected := range []string{"提交配置事务失败", "磁盘恢复失败"} {
			if !strings.Contains(message, expected) {
				t.Fatalf("compound error missing %q: %v", expected, err)
			}
		}
	})
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"version\":1,\"old\":true}\n" {
		t.Fatalf("static config was not restored after compound rollback failure: %q", content)
	}
	assertRuntimeContent(t, options, runtimeContent)
	if _, err := os.Stat(filepath.Join(configTransactionPath(options), "journal.json")); err != nil {
		t.Fatalf("失败后必须保留恢复 journal: %v", err)
	}
}

func TestApplyConfigReturnsRevisionAfterSelectionNormalization(t *testing.T) {
	options, _, source, _ := configApplyOptions(t)
	isolateConfigApplyHooks(t, true)
	if err := os.WriteFile(source, []byte("ACTIVE_GROUP_ID=missing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configReload = func(ctx context.Context, locked Options) error {
		return locked.updateModule(ctx, map[string]string{"ACTIVE_GROUP_ID": "default"})
	}
	revision, err := ApplyConfig(t.Context(), options, "module", source, false, "")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(options.ModuleConfig)
	if err != nil || revision != configRevision(content) {
		t.Fatalf("返回的 revision 与最终配置不同: %s / %s, %v", revision, configRevision(content), err)
	}
}

func TestRecoverConfigApplyBeforeAndAfterRename(t *testing.T) {
	for _, phase := range []string{"prepared", "reload_started"} {
		t.Run(phase, func(t *testing.T) {
			options, destination, _, runtimeContent := configApplyOptions(t)
			isolateConfigApplyHooks(t, false)
			transaction, err := beginConfigApply(options, destination)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "reload_started" {
				if err := os.WriteFile(destination, []byte("new-static\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := transaction.setPhase(phase); err != nil {
					t.Fatal(err)
				}
			}
			if err := recoverConfigApply(context.Background(), options); err != nil {
				t.Fatalf("recover %s: %v", phase, err)
			}
			content, err := os.ReadFile(destination)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != "{\"version\":1,\"old\":true}\n" {
				t.Fatalf("static config after %s recovery: %q", phase, content)
			}
			assertRuntimeContent(t, options, runtimeContent)
			if _, err := os.Stat(configTransactionPath(options)); !os.IsNotExist(err) {
				t.Fatalf("transaction remains after %s recovery: %v", phase, err)
			}
		})
	}
}

func TestRecoverConfigApplyAfterRollbackCleanupWasInterrupted(t *testing.T) {
	options, destination, _, content := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	transaction, err := beginConfigApply(options, destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.setPhase("rolled_back"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(transaction.journal.Static[0].Backup); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := recoverConfigApply(t.Context(), options); err != nil {
			t.Fatal(err)
		}
	}
	assertRuntimeContent(t, options, content)
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
}

func TestConfigRollbackFailureKeepsSnapshotsForRetry(t *testing.T) {
	options, destination, _, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	transaction, err := beginConfigApply(options, destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("模拟恢复中断")
	configSnapshotRestore = func(snapshot configFileSnapshot) error {
		if filepath.Base(snapshot.Path) == "inbound.json" {
			return failure
		}
		return restoreConfigSnapshot(snapshot)
	}
	for range 2 {
		if err := transaction.rollback(); !errors.Is(err, failure) {
			t.Fatalf("恢复错误丢失: %v", err)
		}
		if _, err := os.Stat(transaction.journalPath); err != nil {
			t.Fatal(err)
		}
		for _, snapshot := range transaction.journal.Static {
			if snapshot.Exists {
				if _, err := os.Stat(snapshot.Backup); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	configSnapshotRestore = restoreConfigSnapshot
	if err := recoverConfigApply(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	assertRuntimeContent(t, options, runtimeContent)
	if _, err := os.Stat(transaction.directory); !os.IsNotExist(err) {
		t.Fatalf("恢复后未清理: %v", err)
	}
}

func TestAutoStartSaveDoesNotReloadRunningService(t *testing.T) {
	options, _, _, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, true)
	configReload = func(context.Context, Options) error {
		t.Fatal("开机自启触发了核心 reload")
		return nil
	}
	for _, value := range []string{"1", "0"} {
		snapshot, err := ReadConfig(options, "module")
		if err != nil {
			t.Fatal(err)
		}
		source := writeSectionSource(t, "AUTO_START="+value+"\n")
		revision, err := ApplyConfig(t.Context(), options, "module", source, false, snapshot["revision"])
		if err != nil {
			t.Fatal(err)
		}
		applied, _ := ReadConfig(options, "module")
		if revision != applied["revision"] {
			t.Fatal("revision 没有对应保存内容")
		}
		assertRuntimeContent(t, options, runtimeContent)
	}
	reloads := 0
	configReload = func(context.Context, Options) error { reloads++; return nil }
	if _, err := ApplyConfig(t.Context(), options, "module", writeSectionSource(t, "WIFI_AUTO_SWITCH=1\n"), false, ""); err != nil {
		t.Fatal(err)
	}
	if reloads != 1 {
		t.Fatal("网络策略修改未应用到运行实例")
	}
}

func TestConfigApplyHoldsWriterLockAndReloadBorrowsIt(t *testing.T) {
	options, _, source, _ := configApplyOptions(t)
	isolateConfigApplyHooks(t, true)
	configReload = func(ctx context.Context, locked Options) error {
		waitContext, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		defer cancel()
		if editor, err := moduleconfig.Lock(waitContext, options.ModuleConfig); !errors.Is(err, context.DeadlineExceeded) {
			if editor != nil {
				editor.Release()
			}
			return fmt.Errorf("reload 期间其他写入未被阻止: %v", err)
		}
		return locked.updateModule(context.Background(), map[string]string{"PROXY_ON_CELLULAR": "0"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	withFakeSingBoxResult(t, false, func() {
		if _, err := ApplyConfig(ctx, options, "singbox/config.json", source, false, ""); err != nil {
			t.Fatal(err)
		}
	})
	if err := moduleconfig.UpdateModule(context.Background(), options.ModuleConfig, map[string]string{"AUTO_START": "0"}); err != nil {
		t.Fatal(err)
	}
	config, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || config.ProxyOnCellular || config.AutoStart {
		t.Fatalf("配置变更丢失: %+v %v", config, err)
	}
}

func TestConcurrentAppAddsKeepEveryPackage(t *testing.T) {
	fakeAndroidPackages(t)
	options, _, _, _ := configApplyOptions(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const count = 16
	start := make(chan struct{})
	results := make(chan error, count)
	for index := range count {
		go func() {
			<-start
			_, err := UpdateApp(ctx, options, "add", fmt.Sprintf("0:com.example.app%d", index))
			results <- err
		}()
	}
	close(start)
	for range count {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	policy, err := LoadAppPolicy(options.InboundConfig)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(policy.BypassApps, ",")) != count {
		t.Fatalf("并发写入丢失名单: %s", policy.BypassApps)
	}
}

func TestConfigApplyRejectsRevisionAfterInternalWrite(t *testing.T) {
	options, _, source, _ := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	original, err := os.ReadFile(options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("AUTO_START=0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := moduleconfig.UpdateModule(context.Background(), options.ModuleConfig, map[string]string{"PROXY_ON_CELLULAR": "0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyConfig(context.Background(), options, "module", source, false, configRevision(original)); err == nil {
		t.Fatal("过期配置覆盖成功")
	}
	config, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || config.ProxyOnCellular {
		t.Fatalf("内部写入丢失: %+v %v", config, err)
	}
}

func TestApplyConfigDiskWriteFailureDoesNotReplaceConfig(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	configJournalWrite = func(string, []byte, os.FileMode) error {
		return errors.New("模拟磁盘写入失败")
	}
	withFakeSingBoxResult(t, false, func() {
		if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, ""); err == nil {
			t.Fatal("disk write failure was accepted")
		}
	})
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"version\":1,\"old\":true}\n" {
		t.Fatalf("static config replaced after disk write failure: %q", content)
	}
	assertRuntimeContent(t, options, runtimeContent)
}
