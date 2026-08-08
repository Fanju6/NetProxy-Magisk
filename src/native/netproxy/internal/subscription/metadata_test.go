package subscription

import (
	"testing"
	"time"
)

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
