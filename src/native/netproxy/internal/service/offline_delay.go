package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
)

const (
	offlineDelayStartupTimeout = 5 * time.Second
	offlineDelayRequestTimeout = 20 * time.Second
)

var (
	offlineDelayRunner  = runOfflineDelay
	offlineDelayCommand = func(ctx context.Context, executable, configPath, workingDir string, output *os.File) *exec.Cmd {
		command := exec.CommandContext(ctx, executable, "run", "-c", configPath)
		command.Dir = workingDir
		command.Stdout = output
		command.Stderr = output
		return command
	}
)

func runOfflineDelay(ctx context.Context, options Options, request delayRequest) (DelayResult, error) {
	if options.SingBoxPath == "" {
		return DelayResult{}, offlineDelayError(request.Target, errors.New("sing-box 二进制路径为空"))
	}
	if _, err := os.Stat(options.SingBoxPath); err != nil {
		return DelayResult{}, offlineDelayError(request.Target, fmt.Errorf("读取 sing-box 二进制: %w", err))
	}
	lock, err := processlock.TryAcquire(filepath.Join(options.DelayDir, "session.lock"))
	if err != nil {
		if errors.Is(err, processlock.ErrBusy) {
			return DelayResult{}, &Error{
				Code: "node.delay_busy", Message: "已有节点测速正在进行",
				Data: map[string]any{"target": request.Target},
			}
		}
		return DelayResult{}, offlineDelayError(request.Target, fmt.Errorf("获取离线测速锁: %w", err))
	}
	defer func() { _ = lock.Release() }()

	document, err := offlineDelayProvider(ctx, options.CatalogRoot, request)
	if err != nil {
		return DelayResult{}, offlineDelayError(request.Target, err)
	}
	sessionDir, err := os.MkdirTemp(options.DelayDir, "session-")
	if err != nil {
		return DelayResult{}, offlineDelayError(request.Target, fmt.Errorf("创建离线测速会话: %w", err))
	}
	if err := os.Chmod(sessionDir, 0o700); err != nil {
		_ = os.RemoveAll(sessionDir)
		return DelayResult{}, offlineDelayError(request.Target, fmt.Errorf("设置离线测速目录权限: %w", err))
	}
	defer func() { _ = os.RemoveAll(sessionDir) }()

	address, port, err := reserveLoopbackPort()
	if err != nil {
		return DelayResult{}, offlineDelayError(request.Target, err)
	}
	secret, err := randomDelaySecret()
	if err != nil {
		return DelayResult{}, offlineDelayError(request.Target, err)
	}
	configPath, err := writeOfflineDelayConfig(ctx, sessionDir, request, document, port, secret)
	if err != nil {
		return DelayResult{}, offlineDelayError(request.Target, err)
	}
	logFile, err := os.OpenFile(filepath.Join(sessionDir, "sing-box.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return DelayResult{}, offlineDelayError(request.Target, fmt.Errorf("创建离线测速日志: %w", err))
	}
	defer func() { _ = logFile.Close() }()

	sessionTimeout := options.RequestTimeout
	if sessionTimeout < offlineDelayRequestTimeout {
		sessionTimeout = offlineDelayRequestTimeout
	}
	sessionContext, cancel := context.WithTimeout(ctx, sessionTimeout)
	command := offlineDelayCommand(sessionContext, options.SingBoxPath, configPath, sessionDir, logFile)
	if err := command.Start(); err != nil {
		cancel()
		return DelayResult{}, offlineDelayError(request.Target, fmt.Errorf("启动离线测速核心: %w", err))
	}
	done := make(chan struct{})
	var processErr error
	go func() {
		processErr = command.Wait()
		close(done)
	}()
	defer stopOfflineDelayProcess(cancel, command, done)

	client, err := serviceapi.New(address, secret)
	if err != nil {
		return DelayResult{}, offlineDelayError(request.Target, err)
	}
	defer client.Close()
	if err := waitOfflineDelayReady(sessionContext, client, done, &processErr); err != nil {
		return DelayResult{}, mapOfflineDelayError(request.Target, err)
	}
	result, err := delayWithClient(sessionContext, client, request.Target, true)
	if err != nil {
		return DelayResult{}, mapOfflineDelayError(request.Target, err)
	}
	return result, nil
}

func offlineDelayProvider(ctx context.Context, root string, request delayRequest) (provider.Document, error) {
	if request.NodeTag != "" {
		return catalog.GroupNode(ctx, root, request.GroupID, request.NodeTag)
	}
	return catalog.GroupProvider(ctx, root, request.GroupID)
}

func writeOfflineDelayConfig(
	ctx context.Context,
	sessionDir string,
	request delayRequest,
	document provider.Document,
	port int,
	secret string,
) (string, error) {
	providerPath := filepath.Join(sessionDir, "provider.json")
	if err := provider.SaveAtomic(ctx, providerPath, document); err != nil {
		return "", fmt.Errorf("写入离线测速 Provider: %w", err)
	}
	runtimeTag := request.Target
	if prefix, value, found := strings.Cut(request.Target, "/"); found {
		if prefix == "Auto" || prefix == "Select" {
			runtimeTag = value
		} else {
			runtimeTag = prefix
		}
	}
	config := map[string]any{
		"log": map[string]any{"level": "error"},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{
					"type": "hosts", "tag": "dns-hosts",
					"predefined": map[string][]string{
						"dns.alidns.com": {"223.5.5.5", "223.6.6.6"},
						"doh.pub":        {"120.53.53.53", "1.12.12.12"},
					},
				},
				map[string]any{"type": "group", "tag": "dns-direct", "servers": []string{"dns-ali", "dns-tencent"}},
				map[string]any{"type": "https", "tag": "dns-ali", "server": "dns.alidns.com", "domain_resolver": "dns-hosts"},
				map[string]any{"type": "https", "tag": "dns-tencent", "server": "doh.pub", "domain_resolver": "dns-hosts"},
			},
			"final":    "dns-direct",
			"strategy": "prefer_ipv4",
		},
		"route": map[string]any{
			"default_domain_resolver": "dns-direct",
			"final":                   "direct",
		},
		"providers": []any{map[string]any{
			"type": "local", "tag": runtimeTag, "path": providerPath,
		}},
		"outbounds": []any{
			map[string]any{"type": "direct", "tag": "direct"},
			map[string]any{
				"type": "urltest", "tag": "Auto/" + runtimeTag,
				"providers": []string{runtimeTag}, "url": "https://www.gstatic.com/generate_204",
				"interval": "3m", "tolerance": 50,
			},
			map[string]any{
				"type": "selector", "tag": "Select/" + runtimeTag,
				"providers": []string{runtimeTag},
			},
		},
		"services": []any{map[string]any{
			"type": "api", "listen": "127.0.0.1", "listen_port": port, "secret": secret,
		}},
	}
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	content = append(content, '\n')
	configPath := filepath.Join(sessionDir, "config.json")
	if err := provider.WriteAtomic(configPath, content, 0o600); err != nil {
		return "", fmt.Errorf("写入离线测速配置: %w", err)
	}
	return configPath, nil
}

func reserveLoopbackPort() (string, int, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", 0, fmt.Errorf("分配离线测速端口: %w", err)
	}
	address := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return "", 0, fmt.Errorf("释放离线测速端口: %w", err)
	}
	return address, port, nil
}

func randomDelaySecret() (string, error) {
	content := make([]byte, 16)
	if _, err := rand.Read(content); err != nil {
		return "", fmt.Errorf("生成离线测速密钥: %w", err)
	}
	return hex.EncodeToString(content), nil
}

func waitOfflineDelayReady(ctx context.Context, client *serviceapi.Client, done <-chan struct{}, processErr *error) error {
	startup := time.NewTimer(offlineDelayStartupTimeout)
	defer startup.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		attemptContext, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		_, err := client.Ready(attemptContext)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			if *processErr == nil {
				return errors.New("离线测速核心已提前退出")
			}
			return fmt.Errorf("离线测速核心已退出: %w", *processErr)
		case <-startup.C:
			return fmt.Errorf("等待离线测速 Service API 超时: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func stopOfflineDelayProcess(cancel context.CancelFunc, command *exec.Cmd, done <-chan struct{}) {
	cancel()
	select {
	case <-done:
		return
	default:
	}
	if command.Process != nil {
		_ = command.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func mapOfflineDelayError(target string, cause error) error {
	var structured *Error
	if errors.As(cause, &structured) {
		return structured
	}
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		return delayTimeoutError(target, cause)
	}
	return offlineDelayError(target, cause)
}

func offlineDelayError(target string, cause error) error {
	return &Error{
		Code:    "node.delay_offline_failed",
		Message: fmt.Sprintf("离线节点测速失败: %v", cause),
		Data: map[string]any{
			"target": target,
			"cause":  cause.Error(),
		},
	}
}
