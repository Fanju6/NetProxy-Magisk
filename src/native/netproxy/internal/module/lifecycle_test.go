package module

import (
	"context"
	"reflect"
	"testing"
)

func TestBootCompletedWaitCommandWaitsForPropertyToLeaveZero(t *testing.T) {
	command := newBootCompletedWaitCommand(context.Background())
	want := []string{"resetprop", "-w", "sys.boot_completed", "0"}
	if !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("boot wait command args = %v, want %v", command.Args, want)
	}
}
