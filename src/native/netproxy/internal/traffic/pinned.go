package traffic

import (
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"slices"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const (
	maxPinnedEntries = 512
	maxPinnedBytes   = 384 << 10
)

type PinnedBypass struct {
	Host    string `json:"host"`
	UserID  uint32 `json:"user_id"`
	Package string `json:"package"`
}

func pinnedKey(host string, app *SourceApp) (PinnedBypass, bool) {
	if app == nil || !validMITMHost(host) {
		return PinnedBypass{}, false
	}
	key := PinnedBypass{Host: host, UserID: app.UserID, Package: app.Package}
	return key, key.valid()
}

func (key PinnedBypass) valid() bool {
	if !validMITMHost(key.Host) {
		return false
	}
	_, err := inbound.ParsePackageRef((inbound.PackageRef{UserID: key.UserID, Package: key.Package}).String())
	return err == nil
}

func (key PinnedBypass) matches(other PinnedBypass) bool {
	return key == other
}

func (c *Controller) loadPinnedLocked() error {
	if c.pinned != nil {
		return nil
	}
	f, err := os.Open(c.layout.TrafficPinned())
	if errors.Is(err, os.ErrNotExist) {
		c.pinned = []PinnedBypass{}
		return nil
	}
	if err != nil {
		return errors.New("无法读取证书固定跳过列表")
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, maxPinnedBytes+1))
	var entries []PinnedBypass
	if err != nil || len(content) > maxPinnedBytes || json.Unmarshal(content, &entries, json.RejectUnknownMembers(true)) != nil ||
		entries == nil || len(entries) > maxPinnedEntries || slices.ContainsFunc(entries, func(entry PinnedBypass) bool { return !entry.valid() }) {
		return errors.New("证书固定跳过列表无效")
	}
	c.pinned = entries
	return nil
}

func (c *Controller) savePinnedLocked(entries []PinnedBypass) error {
	content, err := json.Marshal(entries)
	if err != nil || len(content) > maxPinnedBytes || provider.WriteAtomic(c.layout.TrafficPinned(), content, 0o600) != nil {
		return errors.New("无法保存证书固定跳过列表")
	}
	c.pinned = entries
	return nil
}

func (c *Controller) PinnedBypasses() ([]PinnedBypass, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadPinnedLocked(); err != nil {
		return nil, err
	}
	return slices.Clone(c.pinned), nil
}

func (c *Controller) ClearPinnedBypasses() ([]PinnedBypass, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := []PinnedBypass{}
	return entries, c.savePinnedLocked(entries)
}

func (c *Controller) isPinned(host string, app *SourceApp) bool {
	key, ok := pinnedKey(host, app)
	if !ok {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadPinnedLocked() == nil && slices.ContainsFunc(c.pinned, key.matches)
}

func (c *Controller) rememberPinned(host string, app *SourceApp) bool {
	key, ok := pinnedKey(host, app)
	if !ok {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadPinnedLocked() != nil {
		return false
	}
	if slices.ContainsFunc(c.pinned, key.matches) {
		return true
	}
	previous := c.pinned
	if len(previous) >= maxPinnedEntries {
		previous = previous[1:]
	}
	return c.savePinnedLocked(append(slices.Clone(previous), key)) == nil
}
