package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/httpapi"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func apiHostConfig(t *testing.T) (string, apiconfig.Config, moduleapp.Options, net.Listener) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("NETPROXY_MODULE_DIR", root)
	t.Setenv("NETPROXY_DEV_ROOT", t.TempDir())
	path := paths.New(root).APIConfig()
	config, err := apiconfig.Initialize(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	config.Listen = listener.Addr().String()
	writeAPIHostConfig(t, path, config)
	return path, config, moduleapp.NewOptions(root), listener
}

func writeAPIHostConfig(t *testing.T, path string, config apiconfig.Config) {
	t.Helper()
	content, err := json.Marshal(config, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func awaitAPIHost(t *testing.T, config apiconfig.Config) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		info, err := httpapi.Probe(ctx, config)
		if err == nil {
			return info
		}
		select {
		case <-ctx.Done():
			t.Fatalf("API 未自动恢复: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestAPIServerRecoversWithoutWake(t *testing.T) {
	for _, failure := range []string{"occupied_port", "missing_config"} {
		t.Run(failure, func(t *testing.T) {
			path, config, options, listener := apiHostConfig(t)
			if failure == "missing_config" {
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			done, reported := make(chan error, 1), make(chan error, 8)
			go func() {
				done <- runAPIServer(ctx, path, options, nil, func(err error) { reported <- err })
				close(done)
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Errorf("主动取消未正常退出: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Error("API 宿主未完成关停")
				}
			})
			select {
			case err := <-reported:
				if failure == "missing_config" && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("未触发缺失配置错误: %v", err)
				}
				if failure == "occupied_port" {
					if _, ok := errors.AsType[*net.OpError](err); !ok {
						t.Fatalf("未触发真实监听错误: %v", err)
					}
				}
			case <-time.After(5 * time.Second):
				t.Fatal("未观察到初次启动失败")
			}
			if failure == "occupied_port" {
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				writeAPIHostConfig(t, path, config)
			}
			awaitAPIHost(t, config)
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("主动取消未正常退出: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("API 宿主未完成关停")
			}
			listener, err := net.Listen("tcp", config.Listen)
			if err != nil {
				t.Fatalf("退出后监听端口仍被占用: %v", err)
			}
			_ = listener.Close()
			select {
			case err := <-reported:
				t.Fatalf("恢复或主动关停产生多余错误: %v", err)
			default:
			}
		})
	}
}

func TestAPIHostBackoffAndWake(t *testing.T) {
	path, _, _, _ := apiHostConfig(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		wake, done := make(chan struct{}, 1), make(chan error, 1)
		var attempts, reports atomic.Int32
		go func() {
			done <- runAPIHost(ctx, path, wake, func(error) { reports.Add(1) }, func(context.Context, apiconfig.Config) error {
				attempts.Add(1)
				return errors.New("监听失败")
			})
		}()
		synctest.Wait()
		if got := attempts.Load(); got != 1 {
			t.Fatalf("初次启动次数: %d", got)
		}
		for i, delay := range []time.Duration{1, 2, 4, 8, 16, 30, 30} {
			time.Sleep(delay*time.Second - time.Nanosecond)
			synctest.Wait()
			if got := attempts.Load(); got != int32(i+1) {
				t.Fatalf("退避 %s 前提前重试: %d", delay*time.Second, got)
			}
			time.Sleep(time.Nanosecond)
			synctest.Wait()
			if got := attempts.Load(); got != int32(i+2) {
				t.Fatalf("退避 %s 后未重试: %d", delay*time.Second, got)
			}
		}
		wake <- struct{}{}
		synctest.Wait()
		if calls, errors := attempts.Load(), reports.Load(); calls != 9 || errors != 1 {
			t.Fatalf("wake 未立即尝试或重复错误刷屏: attempts=%d reports=%d", calls, errors)
		}
		close(wake)
		synctest.Wait()
		if got := attempts.Load(); got != 9 {
			t.Fatalf("关闭 wake 导致忙循环: %d", got)
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("退避期间取消返回错误: %v", err)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if got := attempts.Load(); got != 9 {
			t.Fatalf("取消后仍在重试: %d", got)
		}
	})
}

func TestAPIHostUnexpectedExitAndStableReset(t *testing.T) {
	path, _, _, _ := apiHostConfig(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		var attempts atomic.Int32
		var reportsMu sync.Mutex
		var reports []string
		reportSnapshot := func() []string {
			reportsMu.Lock()
			defer reportsMu.Unlock()
			return append([]string(nil), reports...)
		}
		go func() {
			done <- runAPIHost(ctx, path, nil, func(err error) {
				reportsMu.Lock()
				defer reportsMu.Unlock()
				reports = append(reports, err.Error())
			}, func(ctx context.Context, _ apiconfig.Config) error {
				switch attempts.Add(1) {
				case 1:
					return nil
				case 2:
					return errors.New("服务退出")
				case 3:
					time.Sleep(30 * time.Second)
					return errors.New("服务退出")
				default:
					<-ctx.Done()
					return nil
				}
			})
		}()
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if calls, errors := attempts.Load(), reportSnapshot(); calls != 3 || len(errors) != 2 || errors[0] != "模块管理接口意外退出" {
			t.Fatalf("意外退出未重试或错误变化未报告: attempts=%d reports=%v", calls, errors)
		}
		time.Sleep(30 * time.Second)
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		if calls, errors := attempts.Load(), reportSnapshot(); calls != 4 || len(errors) != 3 {
			t.Fatalf("稳定运行后退避或错误去重未重置: attempts=%d reports=%v", calls, errors)
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

func TestAPIHostWakeDrainsOldInstance(t *testing.T) {
	path, config, _, _ := apiHostConfig(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		wake, done := make(chan struct{}, 1), make(chan error, 1)
		drain := make(chan struct{})
		release := sync.OnceFunc(func() { close(drain) })
		defer release()
		attempts, reports := 0, 0
		shutdownErr := errors.New("关停收尾失败")
		go func() {
			done <- runAPIHost(ctx, path, wake, func(error) { reports++ }, func(ctx context.Context, _ apiconfig.Config) error {
				attempts++
				<-ctx.Done()
				if attempts == 1 {
					<-drain
				}
				return shutdownErr
			})
		}()
		synctest.Wait()
		wake <- struct{}{}
		synctest.Wait()
		if attempts != 1 {
			t.Fatal("相同配置重启了运行实例")
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		wake <- struct{}{}
		synctest.Wait()
		if attempts != 1 || reports != 1 {
			t.Fatal("配置读取失败影响了运行实例")
		}
		writeAPIHostConfig(t, path, config)
		time.Sleep(time.Second)
		synctest.Wait()
		if attempts != 1 || reports != 1 {
			t.Fatal("配置恢复后重启了相同实例")
		}
		config.Secret = "0000000000000000000000000000000000000000000000000000000000000000"
		writeAPIHostConfig(t, path, config)
		wake <- struct{}{}
		synctest.Wait()
		if attempts != 1 {
			t.Fatal("旧实例收尾前启动了新实例")
		}
		release()
		synctest.Wait()
		if attempts != 2 || reports != 2 {
			t.Fatalf("关停错误延迟了新实例启动: attempts=%d reports=%d", attempts, reports)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if attempts != 2 {
			t.Fatal("旧实例返回值导致新实例重启")
		}
		cancel()
		if err := <-done; !errors.Is(err, shutdownErr) {
			t.Fatalf("最终收尾错误未交还宿主: %v", err)
		}
		if attempts != 2 || reports != 2 {
			t.Fatal("主动关停被当作重启失败")
		}
	})
}

func TestAPIHostCanceledBeforeStart(t *testing.T) {
	path, _, _, _ := apiHostConfig(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		attempts, reports := 0, 0
		err := runAPIHost(ctx, path, nil, func(error) { reports++ }, func(context.Context, apiconfig.Config) error {
			attempts++
			return nil
		})
		if err != nil || attempts != 0 || reports != 0 {
			t.Fatalf("已取消的宿主仍尝试启动: err=%v attempts=%d reports=%d", err, attempts, reports)
		}
	})
}

func TestAPIHostWakeWithPendingExit(t *testing.T) {
	path, config, _, _ := apiHostConfig(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		wake, done := make(chan struct{}, 1), make(chan error, 1)
		exit, releaseReport := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(releaseReport) })
		defer release()
		attempts := 0
		go func() {
			done <- runAPIHost(ctx, path, wake, func(err error) {
				if errors.Is(err, os.ErrNotExist) {
					<-releaseReport
				}
			}, func(ctx context.Context, _ apiconfig.Config) error {
				attempts++
				if attempts == 1 {
					select {
					case <-exit:
						return errors.New("服务退出")
					case <-ctx.Done():
						return nil
					}
				}
				<-ctx.Done()
				return nil
			})
		}()
		synctest.Wait()
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		wake <- struct{}{}
		synctest.Wait()
		close(exit)
		synctest.Wait()
		writeAPIHostConfig(t, path, config)
		wake <- struct{}{}
		release()
		synctest.Wait()
		if attempts != 2 {
			t.Fatalf("wake 与退出同时就绪时仍等待退避: %d", attempts)
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

func TestAPIHostCanceledDuringDrain(t *testing.T) {
	path, config, _, _ := apiHostConfig(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		wake, done := make(chan struct{}, 1), make(chan error, 1)
		drain := make(chan struct{})
		release := sync.OnceFunc(func() { close(drain) })
		defer release()
		attempts := 0
		go func() {
			done <- runAPIHost(ctx, path, wake, func(err error) { t.Errorf("主动关停产生错误: %v", err) }, func(ctx context.Context, _ apiconfig.Config) error {
				attempts++
				<-ctx.Done()
				<-drain
				return nil
			})
		}()
		synctest.Wait()
		config.Secret = "0000000000000000000000000000000000000000000000000000000000000000"
		writeAPIHostConfig(t, path, config)
		wake <- struct{}{}
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("旧实例收尾前宿主退出")
		default:
		}
		release()
		if err := <-done; err != nil || attempts != 1 {
			t.Fatalf("取消后启动了替换实例: err=%v attempts=%d", err, attempts)
		}
	})
}
