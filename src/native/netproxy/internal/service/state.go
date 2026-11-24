package service

import (
	json "encoding/json/v2"
	"fmt"
	"os"
)

type RecordedState struct {
	Schema              int    `json:"schema"`
	State               string `json:"state"`
	PID                 int64  `json:"pid"`
	StartedAt           int64  `json:"started_at"`
	ReadyAt             int64  `json:"ready_at"`
	Error               string `json:"error"`
	UpdatedAt           int64  `json:"updated_at"`
	ActiveBackend       string `json:"active_backend"`
	CoreStartedAtMillis int64  `json:"core_started_at_millis"`
}

func ReadRecordedState(path string) (RecordedState, error) {
	stopped := RecordedState{Schema: 1, State: "stopped"}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return stopped, nil
	}
	if err != nil {
		return stopped, err
	}
	state := RecordedState{Schema: 1}
	if err := json.Unmarshal(content, &state); err != nil {
		return stopped, err
	}
	if !ValidRecordedState(state.State) || state.PID < 0 || int64(int(state.PID)) != state.PID {
		return stopped, fmt.Errorf("服务状态记录无效")
	}
	return state, nil
}

func ValidRecordedState(value string) bool {
	switch value {
	case "stopped", "preparing", "starting", "ready", "stopping", "failed":
		return true
	default:
		return false
	}
}
