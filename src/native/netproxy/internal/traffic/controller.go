package traffic

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	json "encoding/json/v2"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const maxEntries = 200

type Entry struct {
	ID         string     `json:"id"`
	Time       time.Time  `json:"time"`
	Method     string     `json:"method"`
	Host       string     `json:"host"`
	Target     string     `json:"target,omitempty"`
	Outbound   string     `json:"outbound"`
	Status     int        `json:"status"`
	Rewrite    string     `json:"rewrite,omitempty"`
	Rewritten  bool       `json:"rewritten,omitzero"`
	Exit       *Exit      `json:"exit,omitempty"`
	SourceApp  *SourceApp `json:"source_app,omitempty"`
	TLS        string     `json:"tls,omitempty"`
	DurationMS int64      `json:"duration_ms"`
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
	entries       [maxEntries]Record
	detailBytes   int
	count         int
	next          int
	mitm          *mitmState
	upstreamRoots *x509.CertPool
	apps          map[string]struct{}
	owners        androidOwners
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
		entries[i] = c.entries[(c.next+maxEntries-1-i)%maxEntries].Entry
	}
	return entries
}

func ValidRecordID(id string) bool {
	return len(id) == 26 && strings.IndexFunc(id, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '2' && r <= '7')
	}) < 0
}

func (c *Controller) Record(id string) (Record, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < c.count; i++ {
		record := c.entries[(c.next+maxEntries-1-i)%maxEntries]
		if record.ID == id {
			return record, true
		}
	}
	return Record{}, false
}

func (c *Controller) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearLocked()
}

func (c *Controller) record(owner *session, entry Entry) {
	c.recordDetail(owner, entry, nil)
}

func (c *Controller) recordDetail(owner *session, entry Entry, detail *HTTPDetail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == owner {
		size := detail.size()
		for c.count == maxEntries || c.detailBytes+size > captureDetailBudget {
			oldest := (c.next + maxEntries - c.count) % maxEntries
			c.detailBytes -= c.entries[oldest].HTTP.size()
			c.entries[oldest] = Record{}
			c.count--
		}
		entry.ID = rand.Text()
		c.entries[c.next] = Record{Entry: entry, HTTP: detail}
		c.next = (c.next + 1) % maxEntries
		c.count++
		c.detailBytes += size
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
	if err := c.loadApps(); err != nil {
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
	c.entries, c.count, c.next, c.detailBytes = [maxEntries]Record{}, 0, 0, 0
}
