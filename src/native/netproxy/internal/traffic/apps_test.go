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
	rule, err = captureQUICRule(AppsPolicy{Mode: "blacklist", Blacklist: []string{"11:app.example"}}, resolve)
	if err != nil || string(rule) != `{"action":"reject","mode":"and","no_drop":true,"rules":[{"inbound":"netproxy-in","network":"udp","port":443},{"invert":true,"user_id":[1110149]}],"type":"logical"}` {
		t.Fatalf("blacklist rule: %s (%v)", rule, err)
	}
	rule, err = captureQUICRule(AppsPolicy{Mode: "whitelist"}, resolve)
	if err != nil || rule != nil {
		t.Fatalf("empty whitelist broadened to all apps: %s (%v)", rule, err)
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

func TestCaptureAppModes(t *testing.T) {
	c := &Controller{}
	app := &SourceApp{UserID: 11, Package: "app.example"}
	other := &SourceApp{Package: "app.example"}
	legacy, err := normalizeApps(AppsPolicy{Apps: []string{"11:app.example"}})
	if err != nil || legacy.Mode != "whitelist" || len(legacy.Whitelist) != 1 {
		t.Fatalf("legacy migration: %+v (%v)", legacy, err)
	}
	c.applyApps(legacy)
	if !c.capturesApp(app) || c.capturesApp(other) || c.capturesApp(nil) {
		t.Fatal("whitelist did not match exact Android profile")
	}
	legacy.Mode, legacy.Blacklist = "blacklist", []string{"11:app.example"}
	c.applyApps(legacy)
	if c.capturesApp(app) || !c.capturesApp(other) || !c.capturesApp(nil) {
		t.Fatal("blacklist did not exclude exact Android profile")
	}
	legacy.Mode = "all"
	c.applyApps(legacy)
	if !c.capturesApp(app) || !c.capturesApp(other) || !c.capturesApp(nil) {
		t.Fatal("all mode applied an inactive list")
	}
}
