package module

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
)

func inboundApplyFixture(t *testing.T, backend string, running bool) (Options, []byte, map[string]string, *bool) {
	t.Helper()
	options, destination, _, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, running)
	oldStop, oldStart := configStop, configStart
	t.Cleanup(func() { configStop, configStart = oldStop, oldStart })
	alive := running
	configProcessRunning = func(string) bool { return alive }
	configStop = func(context.Context, Options) error { t.Fatal("意外调用 configStop"); return nil }
	configStart = func(context.Context, Options) error { t.Fatal("意外调用 configStart"); return nil }
	configReload = func(context.Context, Options) error { t.Fatal("意外调用 configReload"); return nil }
	configRestoreReload = func(context.Context, Options, configApplyJournal) error {
		t.Fatal("意外调用 configRestoreReload")
		return nil
	}
	if err := os.WriteFile(destination, []byte(`{"route":{"auto_detect_interface":true},"inbounds":[{"type":"mixed","tag":"user-mixed","listen":"127.0.0.1","listen_port":1080}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	content := []byte(strings.Replace(testInboundConfig, `"backend": "ebpf"`, `"backend": "`+backend+`"`, 1))
	if err := os.WriteFile(options.InboundConfig, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return options, content, runtimeContent, &alive
}

func inboundDisk(t *testing.T, options Options) []byte {
	t.Helper()
	content, err := os.ReadFile(options.InboundConfig)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestAppUpdateDoesNotOverwriteRecoverableInboundSnapshot(t *testing.T) {
	options, original, _, _ := inboundApplyFixture(t, "ebpf", false)
	transaction, err := beginConfigApply(options, options.InboundConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateApp(t.Context(), options, "add", "0:com.example.app"); err == nil {
		t.Fatal("未完成的事务仍接受了名单写入")
	}
	if !bytes.Equal(inboundDisk(t, options), original) {
		t.Fatal("待恢复的配置被改写")
	}
	if err := transaction.rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateApp(t.Context(), options, "add", "0:com.example.app"); err != nil {
		t.Fatal("恢复完成后名单仍不可写", err)
	}
}

func inboundJSONEqual(left, right []byte) bool {
	a, b := jsontext.Value(slices.Clone(left)), jsontext.Value(slices.Clone(right))
	if a.Compact() != nil || b.Compact() != nil {
		return false
	}
	return bytes.Equal(a, b)
}

func inboundJournal(t *testing.T, options Options) configApplyJournal {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(configTransactionPath(options), "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var journal configApplyJournal
	if err := json.Unmarshal(content, &journal); err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range append(slices.Clone(journal.Static), journal.Runtime...) {
		if snapshot.Exists {
			if _, err := os.Stat(snapshot.Backup); err != nil {
				t.Fatalf("快照备份丢失: %s: %v", snapshot.Path, err)
			}
		}
	}
	return journal
}

func assertInboundJournalAbsent(t *testing.T, options Options) {
	t.Helper()
	if _, err := os.Stat(configTransactionPath(options)); !os.IsNotExist(err) {
		t.Fatalf("事务未清理: %v", err)
	}
}

func assertInboundRuntimeBackend(t *testing.T, options Options, backend string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(options.RuntimeDir, "inbound.json"))
	if err != nil {
		t.Fatal(err)
	}
	var runtime struct {
		Inbounds []struct {
			Type string `json:"type"`
			Tag  string `json:"tag"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(content, &runtime); err != nil {
		t.Fatal(err)
	}
	if len(runtime.Inbounds) != 1 || runtime.Inbounds[0].Type != backend || runtime.Inbounds[0].Tag != inbound.Tag {
		t.Fatalf("实际 runtime 后端不匹配: %s", content)
	}
}

func TestInboundConfigTargetsAndPartitionRevisions(t *testing.T) {
	options, original, runtimeContent, _ := inboundApplyFixture(t, "ebpf", false)
	documents, err := ListConfigs(options)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"inbound", "inbound/backend", "inbound/ebpf", "inbound/tun", "runtime/inbound.json"} {
		index := slices.IndexFunc(documents, func(document ConfigDocument) bool { return document.ID == target })
		if index < 0 || documents[index].Editable == strings.HasPrefix(target, "runtime/") {
			t.Fatalf("配置目标缺失或权限错误: %s", target)
		}
		path, err := ResolveConfig(options, target)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(target, "runtime/") && path != options.InboundConfig {
			t.Fatal("分区没有共享事实源", path)
		}
	}
	for _, target := range []string{"ebpf", "inbound/app", "inbound/unknown", "runtime/ebpf.json"} {
		if _, err := ResolveConfig(options, target); err == nil {
			t.Fatalf("接受旧目标或非法分区: %s", target)
		}
	}
	full, _ := ReadConfig(options, "inbound")
	ebpf, _ := ReadConfig(options, "inbound/ebpf")
	tun, _ := ReadConfig(options, "inbound/tun")
	appBefore, _ := configObject(original)
	ebpfSource := writeSectionSource(t, `{"ebpf":{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true,"data_plane":"tc","dns_mode":"off"},"shared":{"enabled":false}}}`)
	first, err := ApplyConfig(t.Context(), options, "inbound/ebpf", ebpfSource, false, ebpf["revision"])
	if err != nil {
		t.Fatal(err)
	}
	tunSource := writeSectionSource(t, `{"tun":{"type":"tun","tag":"netproxy-in","interface_name":"saved-tun","address":["172.19.0.1/30"],"auto_route":true,"auto_redirect":true,"strict_route":true}}`)
	second, err := ApplyConfig(t.Context(), options, "inbound/tun", tunSource, false, tun["revision"])
	if err != nil {
		t.Fatalf("不同分区的旧 revision 不应冲突: %v", err)
	}
	saved := inboundDisk(t, options)
	object, _ := configObject(saved)
	if !bytes.Contains(object["ebpf"], []byte(`"tc"`)) || !bytes.Contains(object["tun"], []byte(`"saved-tun"`)) || !inboundJSONEqual(object["app"], appBefore["app"]) {
		t.Fatalf("分区保存覆盖其他内容: %s", saved)
	}
	for _, test := range []struct{ target, revision string }{{"inbound/ebpf", first}, {"inbound/tun", second}} {
		read, err := ReadConfig(options, test.target)
		if err != nil || read["revision"] != test.revision {
			t.Fatal(read, err)
		}
	}
	if _, err := ApplyConfig(t.Context(), options, "inbound/ebpf", ebpfSource, false, ebpf["revision"]); !errors.Is(err, ErrConfigConflict) {
		t.Fatal("同分区旧 revision 未拒绝", err)
	}
	if _, err := ApplyConfig(t.Context(), options, "inbound", writeSectionSource(t, string(original)), false, full["revision"]); !errors.Is(err, ErrConfigConflict) {
		t.Fatal("整份旧 revision 未拒绝", err)
	}
	if _, err := ApplyConfig(t.Context(), options, "runtime/inbound.json", tunSource, false, ""); err == nil {
		t.Fatal("允许修改 runtime")
	}
	for _, fragment := range []string{`{}`, `{"tun":null}`, `{"tun":{},"ebpf":{}}`} {
		if _, err := ApplyConfig(t.Context(), options, "inbound/tun", writeSectionSource(t, fragment), false, ""); err == nil {
			t.Fatal("允许删除或混写必需分区", fragment)
		}
	}
	if !bytes.Equal(saved, inboundDisk(t, options)) {
		t.Fatal("拒绝的操作仍修改了文件")
	}
	assertRuntimeContent(t, options, runtimeContent)
	assertInboundJournalAbsent(t, options)
}

func TestInboundUnselectedSaveDoesNotReload(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		t.Run(backend, func(t *testing.T) {
			options, original, runtimeContent, alive := inboundApplyFixture(t, backend, true)
			target := "inbound/tun"
			source := `{"tun":{"type":"tun","tag":"netproxy-in","interface_name":"saved-tun","address":"172.19.0.1/30","auto_route":true,"auto_redirect":true}}`
			if backend == "tun" {
				target = "inbound/ebpf"
				source = `{"ebpf":{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true,"data_plane":"tc"},"shared":{"enabled":false}}}`
			}
			configJournalWrite = func(path string, content []byte, mode os.FileMode) error {
				var journal configApplyJournal
				if err := json.Unmarshal(content, &journal); err != nil {
					return err
				}
				if journal.Phase != "prepared" && journal.Action != "none" {
					t.Fatalf("未选分区触发运行事务: %#v", journal)
				}
				return writeConfigAtomic(path, content, mode)
			}
			if _, err := ApplyConfig(t.Context(), options, target, writeSectionSource(t, source), true, ""); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, inboundDisk(t, options)) {
				t.Fatal("validateOnly 修改了持久模板")
			}
			assertInboundJournalAbsent(t, options)
			if _, err := ApplyConfig(t.Context(), options, target, writeSectionSource(t, source), false, ""); err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(original, inboundDisk(t, options)) || !*alive {
				t.Fatal("未保存分区或扰动运行状态")
			}
			config, err := inbound.Load(options.InboundConfig)
			if err != nil || config.Backend != backend {
				t.Fatal(config, err)
			}
			assertRuntimeContent(t, options, runtimeContent)
			assertInboundJournalAbsent(t, options)
		})
	}
}

func TestInboundBackendSwitchRunningAndStopped(t *testing.T) {
	for _, oldBackend := range []string{"ebpf", "tun"} {
		newBackend := "tun"
		if oldBackend == "tun" {
			newBackend = "ebpf"
		}
		for _, running := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-to-%s/running=%t", oldBackend, newBackend, running), func(t *testing.T) {
				options, original, runtimeContent, alive := inboundApplyFixture(t, oldBackend, running)
				var events []string
				configStop = func(ctx context.Context, locked Options) error {
					events = append(events, "stop")
					if !bytes.Equal(original, inboundDisk(t, locked)) {
						t.Fatal("停旧实例前已经替换配置")
					}
					assertRuntimeContent(t, locked, runtimeContent)
					journal := inboundJournal(t, locked)
					if journal.Action != "switch" || journal.Phase != "switch_started" || !journal.WasRunning || journal.Backend != oldBackend {
						t.Fatal(journal)
					}
					*alive = false
					return nil
				}
				configStart = func(ctx context.Context, locked Options) error {
					events = append(events, "start")
					if *alive {
						t.Fatal("旧实例未停止就启动新实例")
					}
					config, err := inbound.Load(locked.InboundConfig)
					if err != nil || config.Backend != newBackend {
						t.Fatal(config, err)
					}
					journal := inboundJournal(t, locked)
					if journal.Phase != "reload_started" || journal.Backend != oldBackend {
						t.Fatal(journal)
					}
					prepared, err := Prepare(ctx, locked, true)
					if err != nil || prepared.Backend != newBackend {
						t.Fatal(prepared, err)
					}
					assertInboundRuntimeBackend(t, locked, newBackend)
					*alive = true
					return nil
				}
				read, _ := ReadConfig(options, "inbound/backend")
				revision, err := ApplyConfig(t.Context(), options, "inbound/backend", writeSectionSource(t, `{"backend":"`+newBackend+`"}`), false, read["revision"])
				if err != nil {
					t.Fatal(err)
				}
				config, err := inbound.Load(options.InboundConfig)
				if err != nil || config.Backend != newBackend {
					t.Fatal(config, err)
				}
				before, _ := configObject(original)
				after, _ := configObject(inboundDisk(t, options))
				for _, field := range []string{"ebpf", "tun", "app"} {
					if !inboundJSONEqual(before[field], after[field]) {
						t.Fatal("切换转换或覆盖了模板", field)
					}
				}
				latest, _ := ReadConfig(options, "inbound/backend")
				if latest["revision"] != revision {
					t.Fatal("返回 revision 与磁盘不匹配")
				}
				if running {
					if !reflect.DeepEqual(events, []string{"stop", "start"}) || !*alive {
						t.Fatal(events)
					}
					assertInboundRuntimeBackend(t, options, newBackend)
				} else {
					if len(events) != 0 || *alive {
						t.Fatal("停止状态保存启动了核心", events)
					}
					assertRuntimeContent(t, options, runtimeContent)
				}
				assertInboundJournalAbsent(t, options)
			})
		}
	}
}

func TestInboundRecordedBackendMismatchCannotStopWithoutStart(t *testing.T) {
	for _, configured := range []string{"ebpf", "tun"} {
		t.Run(configured, func(t *testing.T) {
			options, original, _, alive := inboundApplyFixture(t, configured, true)
			actual := "tun"
			if configured == "tun" {
				actual = "ebpf"
			}
			if err := WriteServiceState(options.StateFile, "ready", 42, 1, 1, "", ServiceIdentity{Backend: actual, StartedAtMillis: 1000}); err != nil {
				t.Fatal(err)
			}
			var events []string
			configStop = func(context.Context, Options) error {
				events = append(events, "stop")
				*alive = false
				journal := inboundJournal(t, options)
				if journal.Backend != actual || journal.Action != "switch" {
					t.Fatal("journal 丢失旧实际后端", journal)
				}
				return nil
			}
			configStart = func(ctx context.Context, locked Options) error {
				events = append(events, "start")
				if _, err := Prepare(ctx, locked, true); err != nil {
					t.Fatal(err)
				}
				*alive = true
				return nil
			}
			if _, err := ApplyConfig(t.Context(), options, "inbound/backend", writeSectionSource(t, `{"backend":"`+configured+`"}`), false, ""); err != nil {
				t.Fatal(err)
			}
			if !*alive || !reflect.DeepEqual(events, []string{"stop", "start"}) {
				t.Fatal("有效内容相同但实际后端不符时只停未启", events)
			}
			before, _ := configObject(original)
			after, _ := configObject(inboundDisk(t, options))
			for _, field := range []string{"backend", "app", "ebpf", "tun"} {
				if !inboundJSONEqual(before[field], after[field]) {
					t.Fatal("相同模板被转换", field)
				}
			}
			assertInboundRuntimeBackend(t, options, configured)
			assertInboundJournalAbsent(t, options)
		})
	}
}

func TestInboundCandidateCheckFailureDoesNotStopOrWrite(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		t.Run(backend, func(t *testing.T) {
			options, original, runtimeContent, alive := inboundApplyFixture(t, backend, true)
			next := "tun"
			if backend == "tun" {
				next = "ebpf"
			}
			withFakeSingBoxResult(t, true, func() {
				if _, err := ApplyConfig(t.Context(), options, "inbound/backend", writeSectionSource(t, `{"backend":"`+next+`"}`), false, ""); err == nil {
					t.Fatal("候选核心 check 失败仍继续切换")
				}
			})
			if !*alive || !bytes.Equal(original, inboundDisk(t, options)) {
				t.Fatal("预检查失败影响旧实例或模板")
			}
			assertRuntimeContent(t, options, runtimeContent)
			assertInboundJournalAbsent(t, options)
		})
	}
}

func TestInboundSwitchFailureRestoresDiskRuntimeAndInstance(t *testing.T) {
	for _, oldBackend := range []string{"ebpf", "tun"} {
		newBackend := "tun"
		if oldBackend == "tun" {
			newBackend = "ebpf"
		}
		for _, failureStage := range []string{"stop", "start-exited", "start-running", "commit", "cancel", "restore"} {
			t.Run(oldBackend+"/"+failureStage, func(t *testing.T) {
				options, original, runtimeContent, alive := inboundApplyFixture(t, oldBackend, true)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				failure := errors.New("模拟切换失败")
				restoreFailure := errors.New("模拟旧实例恢复失败")
				var events []string
				configStop = func(ctx context.Context, locked Options) error {
					events = append(events, "stop")
					if len(events) == 1 && !bytes.Equal(original, inboundDisk(t, locked)) {
						t.Fatal("停止前改写配置")
					}
					*alive = false
					if failureStage == "stop" && len(events) == 1 {
						return failure
					}
					return nil
				}
				configStart = func(ctx context.Context, locked Options) error {
					events = append(events, "start")
					prepared, err := Prepare(ctx, locked, true)
					if err != nil || prepared.Backend != newBackend {
						t.Fatal(prepared, err)
					}
					assertInboundRuntimeBackend(t, locked, newBackend)
					*alive = failureStage == "start-running" || failureStage == "commit"
					if failureStage == "cancel" {
						cancel()
						return ctx.Err()
					}
					if failureStage == "commit" {
						return nil
					}
					return failure
				}
				configRestoreReload = func(restoreCtx context.Context, locked Options, journal configApplyJournal) error {
					events = append(events, "restore")
					if restoreCtx.Err() != nil {
						t.Fatal("恢复继承了已取消 context")
					}
					if deadline, ok := restoreCtx.Deadline(); !ok || time.Until(deadline) <= 0 {
						t.Fatal("恢复没有独立有效期限")
					}
					if !bytes.Equal(original, inboundDisk(t, locked)) {
						t.Fatal("未先恢复静态文件")
					}
					assertRuntimeContent(t, locked, runtimeContent)
					if !journal.WasRunning || journal.Backend != oldBackend || journal.Action != "switch" {
						t.Fatal(journal)
					}
					inboundJournal(t, locked)
					if failureStage == "restore" {
						return restoreFailure
					}
					*alive = true
					return nil
				}
				if failureStage == "commit" {
					failCommittedJournalWrite(t)
				}
				_, err := ApplyConfig(ctx, options, "inbound/backend", writeSectionSource(t, `{"backend":"`+newBackend+`"}`), false, "")
				if err == nil {
					t.Fatal("切换失败返回成功")
				}
				if failureStage == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if failureStage != "cancel" && failureStage != "commit" && !errors.Is(err, failure) {
					t.Fatal("原错误丢失", err)
				}
				if !bytes.Equal(original, inboundDisk(t, options)) {
					t.Fatal("失败后静态模板未恢复")
				}
				assertRuntimeContent(t, options, runtimeContent)
				want := []string{"stop", "start", "restore"}
				if failureStage == "stop" {
					want = []string{"stop", "restore"}
				}
				if failureStage == "start-running" || failureStage == "commit" {
					want = []string{"stop", "start", "stop", "restore"}
				}
				if !reflect.DeepEqual(events, want) {
					t.Fatalf("切换恢复时序错误: %v / %v", events, want)
				}
				if failureStage == "restore" {
					if !errors.Is(err, restoreFailure) || *alive {
						t.Fatal(err)
					}
					inboundJournal(t, options)
				} else {
					if !*alive {
						t.Fatal("原先运行的实例未恢复")
					}
					assertInboundJournalAbsent(t, options)
				}
			})
		}
	}
}

func TestInboundForcedStopKeepsJournalAndNeverStarts(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		t.Run(backend, func(t *testing.T) {
			options, original, runtimeContent, alive := inboundApplyFixture(t, backend, true)
			next := "tun"
			if backend == "tun" {
				next = "ebpf"
			}
			stops := 0
			configStop = func(context.Context, Options) error { stops++; *alive = false; return ErrForcedTermination }
			if _, err := ApplyConfig(t.Context(), options, "inbound/backend", writeSectionSource(t, `{"backend":"`+next+`"}`), false, ""); !errors.Is(err, ErrForcedTermination) {
				t.Fatal(err)
			}
			if stops != 1 || *alive || !bytes.Equal(original, inboundDisk(t, options)) {
				t.Fatal("强停之后仍切换或启动")
			}
			journal := inboundJournal(t, options)
			if journal.Phase != "cleanup_failed" || journal.Action != "switch" || journal.Backend != backend || !journal.WasRunning {
				t.Fatal(journal)
			}
			assertRuntimeContent(t, options, runtimeContent)
			if err := recoverConfigApply(t.Context(), options); !errors.Is(err, ErrForcedTermination) {
				t.Fatal("同次开机清理未知时恢复未中止", err)
			}
			if stops != 1 {
				t.Fatal("再次恢复触发停止副作用")
			}
			inboundJournal(t, options)
		})
	}
}

func TestInboundCandidateForcedCleanupKeepsRecoverySnapshots(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		t.Run(backend, func(t *testing.T) {
			options, original, _, alive := inboundApplyFixture(t, backend, true)
			next := "tun"
			if backend == "tun" {
				next = "ebpf"
			}
			failure := errors.New("候选实例启动确认失败")
			stops, starts := 0, 0
			configStop = func(context.Context, Options) error {
				stops++
				*alive = false
				if stops == 2 {
					return ErrForcedTermination
				}
				return nil
			}
			configStart = func(ctx context.Context, locked Options) error {
				starts++
				if _, err := Prepare(ctx, locked, true); err != nil {
					t.Fatal(err)
				}
				*alive = true
				return failure
			}
			_, err := ApplyConfig(t.Context(), options, "inbound/backend", writeSectionSource(t, `{"backend":"`+next+`"}`), false, "")
			if !errors.Is(err, failure) || !errors.Is(err, ErrForcedTermination) {
				t.Fatal("复合清理错误丢失", err)
			}
			if starts != 1 || stops != 2 || *alive {
				t.Fatal("候选清理未知时仍恢复启动", starts, stops)
			}
			journal := inboundJournal(t, options)
			if journal.Phase != "cleanup_failed" || journal.Backend != backend {
				t.Fatal(journal)
			}
			found := false
			for _, snapshot := range journal.Static {
				if snapshot.Path == options.InboundConfig {
					backup, err := os.ReadFile(snapshot.Backup)
					if err != nil || !bytes.Equal(backup, original) {
						t.Fatal("旧入站恢复备份被丢弃", err)
					}
					found = true
				}
			}
			if !found {
				t.Fatal("journal 缺少旧入站快照")
			}
			assertInboundRuntimeBackend(t, options, next)
			config, err := inbound.Load(options.InboundConfig)
			if err != nil || config.Backend != next {
				t.Fatal("无法确认候选清理前提前改写恢复配置", config, err)
			}
		})
	}
}

func TestInboundRejectsStaticManagedConflictsBeforeSideEffects(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, entry := range []string{`{"type":"tun","tag":"user-tun"}`, `{"type":"ebpf","tag":"user-ebpf"}`, `{"type":"mixed","tag":"netproxy-in"}`} {
			t.Run(backend+"/"+entry, func(t *testing.T) {
				options, original, runtimeContent, _ := inboundApplyFixture(t, backend, true)
				main := filepath.Join(options.SingBoxDir, "config.json")
				static := []byte(`{"route":{"auto_detect_interface":true},"inbounds":[` + entry + `]}`)
				if err := os.WriteFile(main, static, 0o600); err != nil {
					t.Fatal(err)
				}
				_, err := ApplyConfig(t.Context(), options, "inbound/backend", writeSectionSource(t, `{"backend":"`+backend+`"}`), false, "")
				var validation *inbound.ValidationError
				if !errors.As(err, &validation) || len(validation.Diagnostics) == 0 || validation.Diagnostics[0].Code != "inbound.static_conflict" {
					t.Fatal("未返回结构化重复入站诊断", err)
				}
				if !bytes.Equal(original, inboundDisk(t, options)) {
					t.Fatal("冲突修改持久文件")
				}
				after, _ := os.ReadFile(main)
				if !bytes.Equal(after, static) {
					t.Fatal("冲突移除了用户入站")
				}
				assertRuntimeContent(t, options, runtimeContent)
				assertInboundJournalAbsent(t, options)
			})
		}
	}
}

func TestInboundConcurrentAppsPreserveBothNativeTemplates(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		t.Run(backend, func(t *testing.T) {
			options, original, runtimeContent, _ := inboundApplyFixture(t, backend, false)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			const count = 12
			start := make(chan struct{})
			results := make(chan error, count)
			for index := range count {
				go func() {
					<-start
					_, err := UpdateApp(ctx, options, "add", fmt.Sprintf("%d:com.example.app%d", index%2*10, index))
					results <- err
				}()
			}
			close(start)
			for range count {
				if err := <-results; err != nil {
					t.Fatal(err)
				}
			}
			config, err := inbound.Load(options.InboundConfig)
			if err != nil || config.Backend != backend || !config.App.Enabled || len(config.App.BypassApps) != count {
				t.Fatal(config, err)
			}
			before, _ := configObject(original)
			after, _ := configObject(inboundDisk(t, options))
			for _, field := range []string{"backend", "ebpf", "tun"} {
				if !inboundJSONEqual(before[field], after[field]) {
					t.Fatal("并发 app 写覆盖原生对象", field)
				}
			}
			for index := range count {
				if !slices.Contains(config.App.BypassApps, fmt.Sprintf("%d:com.example.app%d", index%2*10, index)) {
					t.Fatal("并发丢失应用", index)
				}
			}
			assertRuntimeContent(t, options, runtimeContent)
			assertInboundJournalAbsent(t, options)
		})
	}
}

func TestInboundAppWaitsForPartitionWriterAndKeepsLatestBackend(t *testing.T) {
	options, _, runtimeContent, _ := inboundApplyFixture(t, "ebpf", false)
	initial := strings.Replace(testInboundConfig, `"local": {"enabled": true}, "shared": {"enabled": false}`, `"local": {"enabled": false}, "shared": {"enabled": true, "interface": "ap0"}`, 1)
	if err := os.WriteFile(options.InboundConfig, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	result := make(chan error, 1)
	launched := false
	configJournalWrite = func(path string, content []byte, mode os.FileMode) error {
		if !launched && bytes.Contains(content, []byte(`"phase":"prepared"`)) {
			launched = true
			attempted := make(chan struct{})
			go func() {
				close(attempted)
				_, err := UpdateApp(ctx, options, "add", "10:com.example.concurrent")
				result <- err
			}()
			<-attempted
			select {
			case err := <-result:
				t.Fatalf("app 绕过配置锁提前完成: %v", err)
			case <-time.After(60 * time.Millisecond):
			}
		}
		return writeConfigAtomic(path, content, mode)
	}
	source := writeSectionSource(t, `{"tun":{"type":"tun","tag":"netproxy-in","interface_name":"latest-tun","address":"172.19.0.1/30","auto_route":true,"auto_redirect":true}}`)
	if _, err := ApplyConfig(ctx, options, "inbound/tun", source, false, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	config, err := inbound.Load(options.InboundConfig)
	if err != nil || config.Backend != "ebpf" || !config.App.Enabled || !reflect.DeepEqual(config.App.BypassApps, []string{"10:com.example.concurrent"}) {
		t.Fatal(config, err)
	}
	native, err := config.TUNOptions()
	if err != nil || native.InterfaceName != "latest-tun" {
		t.Fatal("app 写回旧分区", native, err)
	}
	ebpf, err := config.EBPFOptions()
	local, shared := ebpf.EffectiveEnablement()
	if err != nil || local || !shared {
		t.Fatal(ebpf, err)
	}
	assertRuntimeContent(t, options, runtimeContent)
	assertInboundJournalAbsent(t, options)
}
