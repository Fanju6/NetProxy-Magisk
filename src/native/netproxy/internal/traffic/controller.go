package traffic

import (
	"context"
	"crypto/x509"
	json "encoding/json/v2"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const maxEntries = 200

type Entry struct {
	Time       time.Time `json:"time"`
	Method     string    `json:"method"`
	Host       string    `json:"host"`
	Outbound   string    `json:"outbound"`
	Status     int       `json:"status"`
	Rewrite    string    `json:"rewrite,omitempty"`
	TLS        string    `json:"tls,omitempty"`
	DurationMS int64     `json:"duration_ms"`
}

type Status struct {
	Running bool `json:"running"`
	Count   int  `json:"count"`
}

type Controller struct {
	mu            sync.Mutex
	layout        paths.Layout
	rules         []compiledRule
	session       *session
	entries       [maxEntries]Entry
	count         int
	next          int
	mitm          *mitmState
	upstreamRoots *x509.CertPool
}

func New(layout paths.Layout) *Controller {
	return &Controller{layout: layout}
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Status{Running: c.session != nil, Count: c.count}
}

func (c *Controller) Entries() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := make([]Entry, c.count)
	for i := range entries {
		entries[i] = c.entries[(c.next+maxEntries-1-i)%maxEntries]
	}
	return entries
}

func (c *Controller) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearLocked()
}

func (c *Controller) record(owner *session, entry Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == owner {
		c.entries[c.next] = entry
		c.next = (c.next + 1) % maxEntries
		c.count = min(c.count+1, maxEntries)
	}
}

func (c *Controller) Rules() ([]Rule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadRules(); err != nil {
		return nil, err
	}
	rules := make([]Rule, len(c.rules))
	for i := range rules {
		rules[i] = c.rules[i].Rule
	}
	return rules, nil
}

func (c *Controller) ReplaceRules(rules []Rule) error {
	compiled, err := compileRules(rules)
	if err != nil {
		return err
	}
	content, err := json.Marshal(rules)
	if err != nil || len(content) > MaxRulesBytes {
		return errors.New("重写规则超过大小限制")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := provider.WriteAtomic(c.layout.TrafficRules(), content, 0o600); err != nil {
		return errors.New("无法保存重写规则")
	}
	c.rules = compiled
	return nil
}

func (c *Controller) loadRules() error {
	if c.rules != nil {
		return nil
	}
	file, err := os.Open(c.layout.TrafficRules())
	if errors.Is(err, os.ErrNotExist) {
		c.rules = []compiledRule{}
		return nil
	}
	if err != nil {
		return errors.New("无法读取重写规则")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, MaxRulesBytes+1))
	var rules []Rule
	if err != nil || len(content) > MaxRulesBytes || json.Unmarshal(content, &rules, json.RejectUnknownMembers(true)) != nil || rules == nil {
		return errors.New("无法读取重写规则")
	}
	c.rules, err = compileRules(rules)
	return err
}

func (c *Controller) snapshotRules() []compiledRule {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rules
}

func (c *Controller) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return nil
	}
	if err := c.loadRules(); err != nil {
		return err
	}
	policy, err := loadPolicy(c.layout)
	if err != nil {
		return err
	}
	var ca *authority
	if policy.Enabled {
		ca, err = loadCA(c.layout.TrafficCA())
		if err != nil {
			return errors.New("无法加载 HTTPS 抓包 CA")
		}
	}
	path := c.layout.TrafficSocket()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("抓包套接字路径已被占用")
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return errors.New("无法启动透明抓包套接字")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return errors.New("无法保护透明抓包套接字")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{
		controller: c, listener: listener, ctx: ctx, cancel: cancel,
		slots: make(chan struct{}, 32), scriptSlots: make(chan struct{}, 4),
	}
	c.clearLocked()
	c.replaceMITM(ca, policy)
	c.session = s
	go func() {
		s.serve()
		s.close()
		c.mu.Lock()
		if c.session == s {
			c.session = nil
			c.replaceMITM(nil, MITMPolicy{})
		}
		c.mu.Unlock()
	}()
	return nil
}

func (c *Controller) Stop() {
	c.mu.Lock()
	s := c.session
	c.session = nil
	c.replaceMITM(nil, MITMPolicy{})
	if s != nil {
		s.close()
	}
	c.mu.Unlock()
}

func (c *Controller) clearLocked() {
	c.entries, c.count, c.next = [maxEntries]Entry{}, 0, 0
}
