package subscription

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMetadataStateRoundTrip(t *testing.T) {
	metadata := NewMetadata("group", "测试订阅", "subscription", "https://example.test/sub", time.Unix(1_700_000_000, 0))
	metadata.CustomHeaders["X-Test"] = "value"
	metadata.Usage = json.RawMessage("{\n  \"total\": 100\n}")
	metadata.NodeCount = 3
	metadata.Revision = 2
	metadata.NextUpdateEpoch = 1_700_001_800
	metadata.NextUpdateAt = FormatEpochUTC(metadata.NextUpdateEpoch)

	state, err := EncodeMetadataState(metadata)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMetadataState(state)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != metadata.ID || decoded.Name != metadata.Name || decoded.NodeCount != metadata.NodeCount {
		t.Fatalf("元数据基本字段不一致: %#v", decoded)
	}
	if decoded.CustomHeaders["X-Test"] != "value" {
		t.Fatalf("请求头未恢复: %#v", decoded.CustomHeaders)
	}
	if string(decoded.Usage) != "{\"total\":100}" {
		t.Fatalf("usage 应被压缩为稳定的状态字段: %s", decoded.Usage)
	}
}

func TestDurationAndSchedule(t *testing.T) {
	for _, test := range []struct {
		input string
		want  int64
	}{
		{input: "15m", want: 900},
		{input: "2h", want: 7200},
		{input: "1d", want: 86400},
	} {
		got, err := DurationToSeconds(test.input)
		if err != nil || got != test.want {
			t.Fatalf("DurationToSeconds(%q) = %d, %v", test.input, got, err)
		}
	}
	if _, err := DurationToSeconds("5m"); err == nil {
		t.Fatal("小于 15 分钟的周期应被拒绝")
	}
	metadata := Metadata{UpdateInterval: 1800}
	now := time.Unix(1_700_000_000, 0)
	ScheduleAt(&metadata, now)
	if metadata.NextUpdateEpoch != now.Unix()+1800 || metadata.NextUpdateAt != FormatEpochUTC(now.Unix()+1800) {
		t.Fatalf("调度结果错误: %#v", metadata)
	}
}
