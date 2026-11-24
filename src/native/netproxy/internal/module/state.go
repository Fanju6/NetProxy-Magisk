package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

// ServiceState 描述模块服务的持久状态快照。
type ServiceState = service.RecordedState

type ServiceIdentity struct {
	Backend         string
	StartedAtMillis int64
}

// ReadServiceState 读取服务状态；损坏必须交由调用方决定如何恢复。
func ReadServiceState(path string) (ServiceState, error) {
	if strings.TrimSpace(path) == "" {
		return ServiceState{Schema: 1, State: "stopped"}, fmt.Errorf("服务状态路径不能为空")
	}
	return service.ReadRecordedState(path)
}

// WriteServiceState 原子写入服务状态，避免 Shell 直接拼接 JSON。
func WriteServiceState(path, state string, pid, startedAt, readyAt int64, message string, identity ...ServiceIdentity) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("服务状态路径不能为空")
	}
	if !service.ValidRecordedState(state) {
		return fmt.Errorf("服务状态无效: %s", state)
	}
	if pid < 0 || startedAt < 0 || readyAt < 0 {
		return fmt.Errorf("服务状态时间或 PID 不能为负数")
	}
	stateValue := ServiceState{
		Schema: 1, State: state, PID: pid, StartedAt: startedAt,
		ReadyAt: readyAt, Error: message, UpdatedAt: time.Now().Unix(),
	}
	if len(identity) > 1 {
		return fmt.Errorf("服务状态只能记录一个核心身份")
	}
	if state == "ready" && len(identity) == 1 {
		if (identity[0].Backend != "ebpf" && identity[0].Backend != "tun") || identity[0].StartedAtMillis <= 0 {
			return fmt.Errorf("服务运行身份无效")
		}
		stateValue.ActiveBackend = identity[0].Backend
		stateValue.CoreStartedAtMillis = identity[0].StartedAtMillis
	}
	content, err := json.Marshal(stateValue, json.Deterministic(true))
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
