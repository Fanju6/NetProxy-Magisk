package module

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ServiceState 描述模块服务的持久状态快照。
type ServiceState struct {
	Schema    int    `json:"schema"`
	State     string `json:"state"`
	PID       int64  `json:"pid"`
	StartedAt int64  `json:"started_at"`
	ReadyAt   int64  `json:"ready_at"`
	Error     string `json:"error"`
	UpdatedAt int64  `json:"updated_at"`
}

// WriteServiceState 原子写入服务状态，避免 Shell 直接拼接 JSON。
func WriteServiceState(path, state string, pid, startedAt, readyAt int64, message string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("服务状态路径不能为空")
	}
	if !validServiceState(state) {
		return fmt.Errorf("服务状态无效: %s", state)
	}
	if pid < 0 || startedAt < 0 || readyAt < 0 {
		return fmt.Errorf("服务状态时间或 PID 不能为负数")
	}
	stateValue := ServiceState{
		Schema: 1, State: state, PID: pid, StartedAt: startedAt,
		ReadyAt: readyAt, Error: message, UpdatedAt: time.Now().Unix(),
	}
	content, err := json.Marshal(stateValue)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".service-state-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func validServiceState(value string) bool {
	switch value {
	case "stopped", "preparing", "starting", "ready", "stopping", "failed":
		return true
	default:
		return false
	}
}

// ParseNonNegativeInt 将 Shell 传入的数字参数转换为受限整数。
func ParseNonNegativeInt(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("无效的非负整数: %s", value)
	}
	return parsed, nil
}
