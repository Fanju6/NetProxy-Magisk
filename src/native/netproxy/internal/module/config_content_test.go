package module

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestApplyConfigContentRepairsStoppedWholeConfig(t *testing.T) {
	for _, target := range []string{"module", "inbound", "singbox/config.json"} {
		for _, broken := range []string{`{"broken":`, `[]`} {
			t.Run(target+"/"+broken, func(t *testing.T) {
				options, inboundContent, runtimeContent, _ := inboundApplyFixture(t, "ebpf", false)
				destination, err := ResolveConfig(options, target)
				if err != nil {
					t.Fatal(err)
				}
				replacement, err := os.ReadFile(destination)
				if err != nil {
					t.Fatal(err)
				}
				if target == "inbound" {
					replacement = inboundContent
				}
				if err := os.WriteFile(destination, []byte(broken), 0o600); err != nil {
					t.Fatal(err)
				}
				read, err := ReadConfig(options, target)
				if err != nil || read["content"] != broken {
					t.Fatal("无法读取损坏配置及其 revision", read, err)
				}
				if _, err := ApplyConfigContent(t.Context(), options, target, replacement, false, "stale"); !errors.Is(err, ErrConfigConflict) {
					t.Fatal("修复绕过了 revision 检查", err)
				}
				if revision, err := ApplyConfigContent(t.Context(), options, target, replacement, true, read["revision"]); err != nil || revision != configRevision(replacement) {
					t.Fatal("完整候选校验失败", revision, err)
				}
				if content, err := os.ReadFile(destination); err != nil || string(content) != broken {
					t.Fatal("校验或冲突检查修改了损坏文件", string(content), err)
				}
				assertRuntimeContent(t, options, runtimeContent)
				assertInboundJournalAbsent(t, options)
				revision, err := ApplyConfigContent(t.Context(), options, target, replacement, false, read["revision"])
				if err != nil || revision != configRevision(replacement) {
					t.Fatal("停止时完整替换未修复配置", revision, err)
				}
				if content, err := os.ReadFile(destination); err != nil || !bytes.Equal(content, replacement) {
					t.Fatal("保存内容与候选不一致", string(content), err)
				}
				assertRuntimeContent(t, options, runtimeContent)
				assertInboundJournalAbsent(t, options)
			})
		}
	}
}

func TestApplyConfigContentDoesNotRepairBrokenTreeBySection(t *testing.T) {
	for _, target := range []string{"module/auto_start", "inbound/backend", "singbox/log"} {
		t.Run(target, func(t *testing.T) {
			options, _, runtimeContent, _ := inboundApplyFixture(t, "ebpf", false)
			destination, err := ResolveConfig(options, target)
			if err != nil {
				t.Fatal(err)
			}
			broken := []byte(`{"broken":`)
			if err := os.WriteFile(destination, broken, 0o600); err != nil {
				t.Fatal(err)
			}
			replacement := map[string]string{
				"module/auto_start": `{"auto_start":true}`,
				"inbound/backend":   `{"backend":"tun"}`,
				"singbox/log":       `{"log":{"level":"info"}}`,
			}[target]
			for _, validateOnly := range []bool{true, false} {
				if _, err := ApplyConfigContent(t.Context(), options, target, []byte(replacement), validateOnly, ""); err == nil {
					t.Fatal("分区保存修复了无法合并的损坏整份配置")
				}
			}
			if content, err := os.ReadFile(destination); err != nil || !bytes.Equal(content, broken) {
				t.Fatal("失败后损坏原文被覆盖", string(content), err)
			}
			assertRuntimeContent(t, options, runtimeContent)
			assertInboundJournalAbsent(t, options)
		})
	}
}

func TestApplyConfigContentRepairKeepsRollbackAndRecovery(t *testing.T) {
	options, replacement, runtimeContent, _ := inboundApplyFixture(t, "ebpf", false)
	broken := []byte(`{"backend":`)
	if err := os.WriteFile(options.InboundConfig, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	failCommittedJournalWrite(t)
	failure := errors.New("模拟修复回滚中断")
	configSnapshotRestore = func(snapshot configFileSnapshot) error {
		if snapshot.Path == options.InboundConfig {
			return failure
		}
		return restoreConfigSnapshot(snapshot)
	}
	if _, err := ApplyConfigContent(t.Context(), options, "inbound", replacement, false, configRevision(broken)); !errors.Is(err, failure) {
		t.Fatal("恢复失败未传播", err)
	}
	journal := inboundJournal(t, options)
	if err := validateConfigJournal(options, journal); err != nil {
		t.Fatal("候选路径泄漏到持久事务", err)
	}
	configJournalWrite = writeConfigAtomic
	configSnapshotRestore = restoreConfigSnapshot
	if err := recoverConfigApply(t.Context(), options); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inboundDisk(t, options), broken) {
		t.Fatal("恢复没有保留损坏原文")
	}
	assertRuntimeContent(t, options, runtimeContent)
	assertInboundJournalAbsent(t, options)
	if _, err := ApplyConfigContent(t.Context(), options, "inbound", replacement, false, configRevision(broken)); err != nil {
		t.Fatal("恢复完成后无法重新修复", err)
	}
}

func confirmRepairInstance(t *testing.T, options Options, backend string) {
	t.Helper()
	oldFind, oldStarted := configFindProcess, configStartedAt
	t.Cleanup(func() { configFindProcess, configStartedAt = oldFind, oldStarted })
	configFindProcess = func(string, int) int { return 42 }
	configStartedAt = func(context.Context, Options) (int64, error) { return 1000, nil }
	if err := WriteServiceState(options.StateFile, "ready", 42, 1, 1, "", ServiceIdentity{Backend: backend, StartedAtMillis: 1000}); err != nil {
		t.Fatal(err)
	}
}

func TestApplyConfigContentRepairUsesConfirmedRunningBackend(t *testing.T) {
	for _, actual := range []string{"ebpf", "tun"} {
		for _, next := range []string{"ebpf", "tun"} {
			t.Run(actual+"-to-"+next, func(t *testing.T) {
				options, replacement, _, alive := inboundApplyFixture(t, next, true)
				confirmRepairInstance(t, options, actual)
				broken := []byte(`{"backend":"` + next + `",`)
				if err := os.WriteFile(options.InboundConfig, broken, 0o600); err != nil {
					t.Fatal(err)
				}
				var events []string
				configStop = func(context.Context, Options) error {
					events = append(events, "stop")
					if !bytes.Equal(inboundDisk(t, options), broken) {
						t.Fatal("停止旧核心前已经覆盖坏配置")
					}
					*alive = false
					return nil
				}
				apply := func(context.Context, Options) error {
					journal := inboundJournal(t, options)
					if journal.Backend != actual || journal.Phase != "reload_started" {
						t.Fatal("回滚没有记录真实旧实例", journal)
					}
					if err := validateConfigJournal(options, journal); err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(inboundDisk(t, options), replacement) {
						t.Fatal("应用未使用原始完整候选")
					}
					*alive = true
					return nil
				}
				configStart = func(ctx context.Context, locked Options) error {
					events = append(events, "start")
					return apply(ctx, locked)
				}
				configReload = func(ctx context.Context, locked Options) error {
					events = append(events, "reload")
					return apply(ctx, locked)
				}
				if _, err := ApplyConfigContent(t.Context(), options, "inbound", replacement, false, configRevision(broken)); err != nil {
					t.Fatal(err)
				}
				want := []string{"reload"}
				if actual != next {
					want = []string{"stop", "start"}
				}
				if !reflect.DeepEqual(events, want) || !*alive {
					t.Fatal("从损坏模板猜测了运行后端", events, want)
				}
				assertInboundJournalAbsent(t, options)
			})
		}
	}
}

func TestApplyConfigContentRepairRejectsUnconfirmedInstance(t *testing.T) {
	for _, scenario := range []string{"no-record", "pid-mismatch", "startup-mismatch", "api-failure", "not-ready"} {
		t.Run(scenario, func(t *testing.T) {
			options, replacement, runtimeContent, _ := inboundApplyFixture(t, "ebpf", true)
			confirmRepairInstance(t, options, "tun")
			switch scenario {
			case "no-record":
				if err := os.Remove(options.StateFile); err != nil {
					t.Fatal(err)
				}
			case "pid-mismatch":
				configFindProcess = func(string, int) int { return 43 }
			case "startup-mismatch":
				configStartedAt = func(context.Context, Options) (int64, error) { return 1001, nil }
			case "api-failure":
				configStartedAt = func(context.Context, Options) (int64, error) { return 0, context.DeadlineExceeded }
			case "not-ready":
				if err := WriteServiceState(options.StateFile, "starting", 42, 1, 0, ""); err != nil {
					t.Fatal(err)
				}
			}
			broken := []byte(`{"backend":"tun",`)
			if err := os.WriteFile(options.InboundConfig, broken, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ApplyConfigContent(t.Context(), options, "inbound", replacement, false, configRevision(broken))
			if err == nil || !strings.Contains(err.Error(), "sing-box") {
				t.Fatal("没有真实实例仍修复运行配置", err)
			}
			if scenario == "api-failure" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("API 错误丢失取消语义", err)
			}
			if !bytes.Equal(inboundDisk(t, options), broken) {
				t.Fatal("确认失败仍替换了配置")
			}
			assertRuntimeContent(t, options, runtimeContent)
			assertInboundJournalAbsent(t, options)
		})
	}
}

func TestApplyConfigContentRepairSwitchFailureRestoresActualInstance(t *testing.T) {
	options, replacement, runtimeContent, alive := inboundApplyFixture(t, "tun", true)
	confirmRepairInstance(t, options, "ebpf")
	broken := []byte(`{"backend":"tun",`)
	if err := os.WriteFile(options.InboundConfig, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	configStop = func(context.Context, Options) error { *alive = false; return nil }
	failure := errors.New("模拟候选核心启动失败")
	configStart = func(context.Context, Options) error { return failure }
	restored := false
	configRestoreReload = func(ctx context.Context, locked Options, journal configApplyJournal) error {
		if ctx.Err() != nil || journal.Backend != "ebpf" || journal.Action != "switch" {
			t.Fatal("未使用已确认旧实例恢复", journal, ctx.Err())
		}
		if !bytes.Equal(inboundDisk(t, locked), broken) {
			t.Fatal("回滚改写了原损坏配置")
		}
		assertRuntimeContent(t, locked, runtimeContent)
		restored, *alive = true, true
		return nil
	}
	if _, err := ApplyConfigContent(t.Context(), options, "inbound", replacement, false, configRevision(broken)); !errors.Is(err, failure) {
		t.Fatal("候选启动错误丢失", err)
	}
	if !restored || !*alive {
		t.Fatal("候选启动失败未恢复旧实例")
	}
	assertInboundJournalAbsent(t, options)
}
