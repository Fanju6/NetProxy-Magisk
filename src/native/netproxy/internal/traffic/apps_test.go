package traffic

import (
	"errors"
	"strings"
	"testing"
)

func TestCaptureQUICRuleForSelectedAndroidUsers(t *testing.T) {
	resolve := func(user uint32, names ...string) (map[uint32]bool, error) {
		if len(names) != 1 || names[0] != "app.example" {
			t.Fatalf("unexpected selection: %d %v", user, names)
		}
		return map[uint32]bool{user*100000 + 10149: true}, nil
	}
	rule, err := captureQUICRule(AppsPolicy{Apps: []string{"0:app.example", "11:app.example"}}, resolve)
	if err != nil || string(rule) != `{"action":"reject","inbound":"netproxy-in","network":"udp","no_drop":true,"port":443,"user_id":[10149,1110149]}` {
		t.Fatalf("scoped rule: %s (%v)", rule, err)
	}
	rule, err = captureQUICRule(AppsPolicy{}, resolve)
	if err != nil || len(rule) == 0 || strings.Contains(string(rule), "user_id") {
		t.Fatalf("all-apps rule: %s (%v)", rule, err)
	}
	rule, err = captureQUICRule(AppsPolicy{DisableQUIC: new(false)}, resolve)
	if err != nil || rule != nil {
		t.Fatalf("opted out: %s (%v)", rule, err)
	}
	for _, failure := range []error{nil, errors.New("package service unavailable")} {
		rule, err = captureQUICRule(AppsPolicy{Apps: []string{"0:app.example"}}, func(uint32, ...string) (map[uint32]bool, error) {
			return nil, failure
		})
		if err != failure || rule != nil {
			t.Fatalf("unresolved selection broadened to all apps: %s (%v)", rule, err)
		}
	}
}
