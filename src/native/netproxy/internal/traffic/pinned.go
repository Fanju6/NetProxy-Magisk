package traffic

import (
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"slices"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const alpnBypassTTL = 15 * time.Minute

type ALPNBypass struct {
	PinnedBypass
	ExpiresAt time.Time `json:"expires_at"`
}

func (c *Controller) ALPNBypasses() ([]ALPNBypass, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadALPNLocked(); err != nil {
		return nil, err
	}
	c.expireALPNLocked(time.Now())
	return slices.Clone(c.alpn), nil
}

func (c *Controller) ClearALPNBypasses() ([]ALPNBypass, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := []ALPNBypass{}
	return entries, c.saveALPNLocked(entries)
}

func (c *Controller) RemoveALPNBypass(host, app string) ([]ALPNBypass, error) {
	key, err := bypassKey(host, app)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadALPNLocked(); err != nil {
		return nil, err
	}
	c.expireALPNLocked(time.Now())
	entries := slices.DeleteFunc(slices.Clone(c.alpn), func(entry ALPNBypass) bool { return key.matches(entry.PinnedBypass) })
	return slices.Clone(entries), c.saveALPNLocked(entries)
}

func (c *Controller) loadALPNLocked() error {
	if c.alpn != nil {
		return nil
	}
	f, err := os.Open(c.layout.TrafficALPNBypasses())
	if errors.Is(err, os.ErrNotExist) {
		c.alpn = []ALPNBypass{}
		return nil
	}
	if err != nil {
		return errors.New("无法读取 ALPN 临时跳过列表")
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, maxPinnedBytes+1))
	var entries []ALPNBypass
	if err != nil || len(content) > maxPinnedBytes || json.Unmarshal(content, &entries, json.RejectUnknownMembers(true)) != nil ||
		entries == nil || len(entries) > maxPinnedEntries || slices.ContainsFunc(entries, func(entry ALPNBypass) bool {
		return !entry.valid() || entry.ExpiresAt.IsZero()
	}) {
		return errors.New("ALPN 临时跳过列表无效")
	}
	c.alpn = entries
	return nil
}

func (c *Controller) saveALPNLocked(entries []ALPNBypass) error {
	content, err := json.Marshal(entries)
	if err != nil || len(content) > maxPinnedBytes || provider.WriteAtomic(c.layout.TrafficALPNBypasses(), content, 0o600) != nil {
		return errors.New("无法保存 ALPN 临时跳过列表")
	}
	c.alpn = entries
	return nil
}

func (c *Controller) expireALPNLocked(now time.Time) {
	c.alpn = slices.DeleteFunc(c.alpn, func(entry ALPNBypass) bool { return !entry.ExpiresAt.After(now) })
}

func (c *Controller) isALPNBypassed(host string, app *SourceApp) bool {
	key, ok := pinnedKey(canonicalHost(host), app)
	if !ok {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadALPNLocked() != nil {
		return false
	}
	c.expireALPNLocked(time.Now())
	return slices.ContainsFunc(c.alpn, func(entry ALPNBypass) bool { return key.matches(entry.PinnedBypass) })
}

func (c *Controller) rememberALPN(host string, app *SourceApp) {
	key, ok := pinnedKey(canonicalHost(host), app)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.loadALPNLocked()
	now := time.Now()
	c.expireALPNLocked(now)
	c.alpn = slices.DeleteFunc(c.alpn, func(entry ALPNBypass) bool { return key.matches(entry.PinnedBypass) })
	if len(c.alpn) == maxPinnedEntries {
		c.alpn = slices.Delete(c.alpn, 0, 1)
	}
	c.alpn = append(c.alpn, ALPNBypass{key, now.Add(alpnBypassTTL)})
	_ = c.saveALPNLocked(c.alpn)
}

const (
	maxPinnedEntries = 512
	maxPinnedBytes   = 384 << 10
)

type PinnedBypass struct {
	Host      string    `json:"host"`
	UserID    uint32    `json:"user_id"`
	Package   string    `json:"package"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"`
}

var ErrBypassKey = errors.New("跳过条目的域名或应用无效")

func bypassKey(host, app string) (PinnedBypass, error) {
	ref, err := inbound.ParsePackageRef(app)
	if err != nil {
		return PinnedBypass{}, ErrBypassKey
	}
	key, ok := pinnedKey(canonicalHost(host), &SourceApp{UserID: ref.UserID, Package: ref.Package})
	if !ok {
		return PinnedBypass{}, ErrBypassKey
	}
	return key, nil
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
	return key.Host == other.Host && key.UserID == other.UserID && key.Package == other.Package
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

func (c *Controller) RemovePinnedBypass(host, app string) ([]PinnedBypass, error) {
	key, err := bypassKey(host, app)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadPinnedLocked(); err != nil {
		return nil, err
	}
	entries := slices.DeleteFunc(slices.Clone(c.pinned), key.matches)
	return slices.Clone(entries), c.savePinnedLocked(entries)
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

func (c *Controller) rememberPinned(host string, app *SourceApp, reason string) bool {
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
	key.Reason, key.CreatedAt = reason, time.Now().UTC()
	return c.savePinnedLocked(append(slices.Clone(previous), key)) == nil
}
