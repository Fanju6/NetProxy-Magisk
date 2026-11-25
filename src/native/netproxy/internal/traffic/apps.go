package traffic

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const MaxAppsBytes = 64 << 10
const androidUIDRange = 100000

type SourceApp struct {
	UID     uint32 `json:"uid"`
	UserID  uint32 `json:"user_id"`
	Package string `json:"package,omitempty"`
}

type AppsPolicy struct {
	Apps        []string `json:"apps"`
	DisableQUIC *bool    `json:"disable_quic,omitempty"`
}

func normalizeApps(policy AppsPolicy) (AppsPolicy, map[string]struct{}, error) {
	if policy.Apps == nil || len(policy.Apps) > 1000 {
		return AppsPolicy{}, nil, errors.New("apps 必须是数组，最多允许选择 1000 个应用")
	}
	result := AppsPolicy{Apps: make([]string, 0, len(policy.Apps)), DisableQUIC: policy.DisableQUIC}
	set := make(map[string]struct{}, len(policy.Apps))
	for _, value := range policy.Apps {
		ref, err := inbound.ParsePackageRef(value)
		if err != nil || len(ref.Package) > 256 {
			return AppsPolicy{}, nil, errors.New("抓包应用必须使用 <用户ID>:<包名> 格式")
		}
		key := ref.String()
		if _, known := set[key]; !known {
			set[key] = struct{}{}
			result.Apps = append(result.Apps, key)
		}
	}
	return result, set, nil
}

func (c *Controller) AppsPolicy() (AppsPolicy, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return readApps(c.layout)
}

func readApps(layout paths.Layout) (AppsPolicy, error) {
	file, err := os.Open(layout.TrafficApps())
	if errors.Is(err, os.ErrNotExist) {
		return AppsPolicy{Apps: []string{}, DisableQUIC: new(true)}, nil
	}
	if err != nil {
		return AppsPolicy{}, errors.New("无法读取抓包应用配置")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, MaxAppsBytes+1))
	var policy AppsPolicy
	if err != nil || len(content) > MaxAppsBytes || json.Unmarshal(content, &policy, json.RejectUnknownMembers(true)) != nil {
		return AppsPolicy{}, errors.New("抓包应用配置无效")
	}
	normalized, _, err := normalizeApps(policy)
	if normalized.DisableQUIC == nil {
		normalized.DisableQUIC = new(true)
	}
	return normalized, err
}

func (c *Controller) loadApps() error {
	policy, err := readApps(c.layout)
	if err != nil {
		return err
	}
	_, c.apps, err = normalizeApps(policy)
	return err
}

func (c *Controller) SetAppsPolicy(policy AppsPolicy) (AppsPolicy, error) {
	normalized, set, err := normalizeApps(policy)
	if err != nil {
		return AppsPolicy{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if normalized.DisableQUIC == nil {
		previous, err := readApps(c.layout)
		if err != nil {
			return AppsPolicy{}, err
		}
		normalized.DisableQUIC = previous.DisableQUIC
	}
	content, err := json.Marshal(normalized)
	if err != nil || len(content) > MaxAppsBytes {
		return AppsPolicy{}, errors.New("抓包应用配置超出大小限制")
	}
	if err := provider.WriteAtomic(c.layout.TrafficApps(), content, 0o600); err != nil {
		return AppsPolicy{}, errors.New("无法保存抓包应用配置")
	}
	c.apps = set
	return normalized, nil
}

func CaptureQUICRule(ctx context.Context, layout paths.Layout) (jsontext.Value, error) {
	policy, err := readApps(layout)
	if err != nil {
		return nil, err
	}
	return captureQUICRule(policy, func(user uint32, packages ...string) (map[uint32]bool, error) {
		return inbound.ResolveExclusivePackageUIDs(ctx, user, packages...)
	})
}

func captureQUICRule(policy AppsPolicy, resolve func(uint32, ...string) (map[uint32]bool, error)) (jsontext.Value, error) {
	if policy.DisableQUIC != nil && !*policy.DisableQUIC {
		return nil, nil
	}
	rule := map[string]any{
		"inbound": "netproxy-in", "network": "udp", "port": 443, "action": "reject", "no_drop": true,
	}
	if len(policy.Apps) != 0 {
		byUser := make(map[uint32][]string)
		for _, value := range policy.Apps {
			ref, err := inbound.ParsePackageRef(value)
			if err != nil {
				return nil, err
			}
			byUser[ref.UserID] = append(byUser[ref.UserID], ref.Package)
		}
		var uids []uint32
		for user, packages := range byUser {
			resolved, err := resolve(user, packages...)
			if err != nil {
				return nil, err
			}
			for uid := range resolved {
				uids = append(uids, uid)
			}
		}
		if len(uids) == 0 {
			return nil, nil
		}
		slices.Sort(uids)
		rule["user_id"] = uids
	}
	return json.Marshal(rule, json.Deterministic(true))
}

func (c *Controller) capturesApp(app *SourceApp) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.apps) == 0 {
		return true
	}
	if app == nil || app.Package == "" {
		return false
	}
	_, ok := c.apps[(inbound.PackageRef{UserID: app.UserID, Package: app.Package}).String()]
	return ok
}

type androidOwners struct {
	mu    sync.Mutex
	cache map[uint32]ownerSnapshot
}

type ownerSnapshot struct {
	expires time.Time
	names   map[uint32]string
}

func (o *androidOwners) sourceApp(ctx context.Context, exit *Exit) *SourceApp {
	if exit == nil || exit.UID == nil {
		return nil
	}
	uid := *exit.UID
	app := &SourceApp{UID: uid, UserID: uid / androidUIDRange}
	if ref, err := inbound.ParsePackageRef(strconv.FormatUint(uint64(app.UserID), 10) + ":" + exit.Package); err == nil {
		app.Package = ref.Package
	} else if runtime.GOOS == "android" && uid%androidUIDRange >= 10000 {
		app.Package = o.packageName(ctx, app.UserID, uid)
	}
	return app
}

func (o *androidOwners) packageName(ctx context.Context, user, uid uint32) string {
	if !o.mu.TryLock() {
		return ""
	}
	defer o.mu.Unlock()
	if snapshot, ok := o.cache[user]; ok && time.Now().Before(snapshot.expires) {
		return snapshot.names[uid]
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Millisecond)
	defer cancel()
	packages, _ := inbound.ListPackageUIDs(ctx, user)
	names := make(map[uint32]string)
	for pkg, id := range packages {
		if _, seen := names[id]; seen {
			names[id] = ""
		} else {
			names[id] = pkg
		}
	}
	if o.cache == nil || len(o.cache) >= 16 {
		o.cache = make(map[uint32]ownerSnapshot)
	}
	o.cache[user] = ownerSnapshot{expires: time.Now().Add(30 * time.Second), names: names}
	return names[uid]
}
