package traffic

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	json "encoding/json/v2"
	"errors"
	"fmt"
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
	ID                string     `json:"id"`
	Time              time.Time  `json:"time"`
	Method            string     `json:"method"`
	Host              string     `json:"host"`
	Target            string     `json:"target,omitempty"`
	Outbound          string     `json:"outbound"`
	Status            int        `json:"status"`
	Rewrite           string     `json:"rewrite,omitempty"`
	Rewritten         bool       `json:"rewritten,omitzero"`
	Exit              *Exit      `json:"exit,omitempty"`
	SourceApp         *SourceApp `json:"source_app,omitempty"`
	TLS               string     `json:"tls,omitempty"`
	PassthroughReason string     `json:"passthrough_reason,omitempty"`
	DurationMS        int64      `json:"duration_ms"`
}

type Status struct {
	Running bool `json:"running"`
	Count   int  `json:"count"`
}

type Preview struct {
	Status
	Entries []Entry `json:"entries"`
}

type Controller struct {
	mu            sync.Mutex
	layout        paths.Layout
	rules         []compiledRule
	rulesByHost   map[string][]int
	session       *session
	entries       [maxEntries]Record
	detailBytes   int
	count         int
	recorded      int
	next          int
	mitm          *mitmState
	upstreamRoots *x509.CertPool
	apps          map[string]struct{}
	appMode       string
	owners        androidOwners
	pinned        []PinnedBypass
	history       *historyStore
	batch         *savedBatch
	writer        *historyWriter
}

func New(layout paths.Layout) *Controller {
	return &Controller{layout: layout, history: openHistory(layout)}
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	status := Status{Running: c.session != nil, Count: c.count}
	useHistory := c.batch == nil && !status.Running && c.history != nil
	c.mu.Unlock()
	if useHistory {
		batches := c.history.list()
		if len(batches) != 0 {
			status.Count = batches[0].Count
		}
	}
	return status
}

func (c *Controller) Entries() []Entry {
	entries, _ := c.BatchEntries("")
	return entries
}

func (c *Controller) Preview() Preview {
	c.mu.Lock()
	defer c.mu.Unlock()
	preview := Preview{Entries: []Entry{}}
	if c.session != nil {
		preview.Status = Status{Running: true, Count: c.recorded}
		for i := 0; i < min(c.count, 3); i++ {
			preview.Entries = append(preview.Entries, c.entries[(c.next+maxEntries-1-i)%maxEntries].Entry)
		}
	}
	return preview
}

func (c *Controller) BatchEntries(id string) ([]Entry, bool) {
	c.mu.Lock()
	if c.history != nil && ((id != "" && (c.batch == nil || id != c.batch.ID)) ||
		(id == "" && c.batch == nil && c.session == nil && c.count == 0)) {
		c.mu.Unlock()
		return c.history.entries(id)
	}
	entries := make([]Entry, c.count)
	for i := range entries {
		entries[i] = c.entries[(c.next+maxEntries-1-i)%maxEntries].Entry
	}
	c.mu.Unlock()
	return entries, true
}

func (c *Controller) Batches() []Batch {
	if c.history == nil {
		return []Batch{}
	}
	return c.history.list()
}

func ValidRecordID(id string) bool {
	return len(id) == 26 && strings.IndexFunc(id, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '2' && r <= '7')
	}) < 0
}

func (c *Controller) Record(id string) (Record, bool) {
	c.mu.Lock()
	for i := 0; i < c.count; i++ {
		record := c.entries[(c.next+maxEntries-1-i)%maxEntries]
		if record.ID == id {
			c.mu.Unlock()
			if record.hasMedia && c.history != nil {
				if persisted, ok := c.history.record(id); ok {
					return persisted, true
				}
			}
			return record, true
		}
	}
	c.mu.Unlock()
	if c.history != nil {
		return c.history.record(id)
	}
	return Record{}, false
}

func (c *Controller) Clear() error {
	_, err := c.removeBatch("")
	return err
}

var ErrActiveBatch = errors.New("请先停止抓包，再删除当前批次")

func (c *Controller) DeleteBatch(id string) (bool, error) {
	return c.removeBatch(id)
}

func (c *Controller) removeBatch(id string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.history == nil {
		c.clearLocked()
		return id == "", nil
	}
	var active *savedBatch
	if c.writer != nil {
		active = c.batch
		if id == active.ID {
			return false, ErrActiveBatch
		}
		if id == "" {
			c.writer.stop()
			defer func() { c.writer = c.history.writer(active) }()
		}
	}
	found, err := c.history.remove(id, active)
	if err == nil && (id == "" || (c.batch != nil && id == c.batch.ID)) {
		c.clearLocked()
		if active == nil {
			c.batch = nil
		}
	}
	return found, err
}

func (c *Controller) record(owner *session, entry Entry) {
	c.recordDetail(owner, entry, nil)
}

func (c *Controller) recordDetail(owner *session, entry Entry, detail *HTTPDetail) {
	c.recordMediaDetail(owner, entry, detail, nil, nil)
}

func (c *Controller) recordMediaDetail(owner *session, entry Entry, detail *HTTPDetail, request, response *mediaSpool) {
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
		record := Record{Entry: entry, HTTP: detail, hasMedia: request != nil || response != nil}
		c.entries[c.next] = record
		if c.writer != nil {
			select {
			case c.writer.queue <- pendingRecord{record, request, response}:
				request, response = nil, nil
			default:
				c.writer.dropped.Store(true)
			}
		}
		c.next = (c.next + 1) % maxEntries
		c.count++
		c.recorded++
		c.detailBytes += size
	}
	request.discard()
	response.discard()
}

func (c *Controller) Rules() ([]Rule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadRules(); err != nil {
		return nil, err
	}
	rules := make([]Rule, len(c.rules))
	for i := range rules {
		rules[i] = c.rules[i].Rule.copyEnabled()
	}
	return rules, nil
}

func (c *Controller) ReplaceRules(rules []Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	existing := make(map[[3]string]int)
	for _, rule := range rules {
		if len(rule.Domains) != 0 {
			continue
		}
		if err := c.loadRules(); err != nil {
			return err
		}
		for _, current := range c.rules {
			if len(current.Domains) == 0 {
				existing[[3]string{current.Pattern, current.Phase, current.Script}]++
			}
		}
		break
	}
	for i, rule := range rules {
		if len(rule.Domains) == 0 {
			key := [3]string{rule.Pattern, rule.Phase, rule.Script}
			if existing[key] == 0 {
				return fmt.Errorf("第 %d 条重写规则需要 1–32 个域名", i+1)
			}
			existing[key]--
		}
	}
	compiled, err := compileRules(rules, true)
	if err != nil {
		return err
	}
	normalized := make([]Rule, len(compiled))
	for i, rule := range compiled {
		normalized[i] = rule.Rule
	}
	content, err := json.Marshal(normalized)
	if err != nil || len(content) > MaxRulesBytes {
		return errors.New("重写规则超过大小限制")
	}
	if err := provider.WriteAtomic(c.layout.TrafficRules(), content, 0o600); err != nil {
		return errors.New("无法保存重写规则")
	}
	c.rules = compiled
	c.rulesByHost = indexRules(compiled)
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
	c.rules, err = compileRules(rules, true)
	c.rulesByHost = indexRules(c.rules)
	return err
}

func (c *Controller) snapshotRules(host string) ([]compiledRule, []int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rules, c.rulesByHost[canonicalHost(host)]
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
	if c.history != nil {
		c.batch = &savedBatch{Batch: Batch{ID: rand.Text(), StartedAt: time.Now().UTC(), Running: true}}
		c.history.start(c.batch)
		c.writer = c.history.writer(c.batch)
	}
	c.replaceMITM(ca, policy)
	c.session = s
	go func() {
		s.serve()
		s.close()
		c.mu.Lock()
		if c.session == s {
			c.session = nil
			c.replaceMITM(nil, MITMPolicy{})
			c.finishBatchLocked()
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
	c.finishBatchLocked()
	c.mu.Unlock()
}

func (c *Controller) finishBatchLocked() {
	if c.writer != nil {
		c.writer.stop()
		c.writer = nil
		c.history.finish(c.batch)
	}
}

func (c *Controller) clearLocked() {
	c.entries, c.count, c.next, c.detailBytes = [maxEntries]Record{}, 0, 0, 0
	c.recorded = 0
}
