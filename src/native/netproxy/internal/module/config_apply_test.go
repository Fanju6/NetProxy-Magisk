package module

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
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
	if err := os.MkdirAll(filepath.Dir(options.EBPFConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.EBPFConfig, []byte("\n"), 0o600); err != nil {
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
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
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
		assertRuntimeContent(t, options, runtimeContent)
		return errors.New("模拟旧 runtime reload 失败")
	}
	failCommittedJournalWrite(t)
	withFakeSingBoxResult(t, false, func() {
		_, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, "")
		if err == nil {
			t.Fatal("commit rollback failure was accepted")
		}
		message := err.Error()
		for _, expected := range []string{"提交配置事务失败", "磁盘恢复失败", "旧 runtime reload 失败"} {
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
