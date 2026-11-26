package traffic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const (
	historyBatchLimit  = 32
	historyByteLimit   = 64 << 20
	historyRecordLimit = 2 << 20
	historyCountLimit  = 8192
	MaxHistoryPage     = 100
)

type Batch struct {
	ID        string     `json:"id"`
	StartedAt time.Time  `json:"started_at"`
	StoppedAt *time.Time `json:"stopped_at,omitempty"`
	Running   bool       `json:"running"`
	Count     int        `json:"count"`
	Size      int64      `json:"size"`
}

type savedRecord struct {
	name      string
	id        string
	host      string
	size      int64
	sequence  uint64
	mediaSize int64
}

type savedBatch struct {
	Batch
	records   []savedRecord
	sequence  uint64
	mediaSize int64
}

type historyStore struct {
	mu         sync.Mutex
	layout     paths.Layout
	batches    []*savedBatch
	byteLimit  int64
	mediaLimit int64
	staged     int64
	mediaSlots chan struct{}
}

type pendingRecord struct {
	record            Record
	request, response *mediaSpool
}

type historyWriter struct {
	queue   chan pendingRecord
	done    chan struct{}
	dropped atomic.Bool
}

func openHistory(layout paths.Layout) *historyStore {
	h := &historyStore{layout: layout, byteLimit: historyByteLimit, mediaLimit: mediaBudget, mediaSlots: make(chan struct{}, 8)}
	dirs, err := os.ReadDir(layout.TrafficHistory())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		h.warn()
	}
	for _, dir := range dirs {
		if !dir.IsDir() || !ValidRecordID(dir.Name()) {
			continue
		}
		batch := &savedBatch{}
		if !readHistoryJSON(h.path(dir.Name(), "batch.json"), 4096, &batch.Batch) ||
			batch.ID != dir.Name() || batch.StartedAt.IsZero() {
			continue
		}
		batch.Running = false
		batch.Count, batch.Size = 0, 0
		lastRecord := batch.StartedAt
		files, _ := os.ReadDir(h.path(batch.ID))
		blobs := make(map[string]bool)
		for _, file := range files {
			if strings.HasPrefix(file.Name(), ".netproxy-") && strings.HasSuffix(file.Name(), ".tmp") {
				_ = os.Remove(h.path(batch.ID, file.Name()))
				continue
			}
			var seq uint64
			var id string
			if file.Type().IsRegular() {
				seq, id = historyRecordName(file.Name())
			}
			if id == "" {
				continue
			}
			info, err := file.Info()
			if err != nil || info.Size() == 0 || info.Size() > historyRecordLimit {
				continue
			}
			saved := savedRecord{name: file.Name(), id: id, size: info.Size(), sequence: seq}
			record, ok := h.readRecord(batch, saved)
			if !ok {
				_ = os.Remove(h.path(batch.ID, file.Name()))
				continue
			}
			saved.host = historyHost(record.Host)
			for _, side := range []string{"request", "response"} {
				if body := bodyFor(record, side); body != nil && body.Blob {
					blobs[mediaName(id, side)] = true
					saved.mediaSize += int64(body.CapturedSize)
				}
			}
			batch.mediaSize += saved.mediaSize
			batch.sequence = max(batch.sequence, seq)
			batch.records = append(batch.records, saved)
			batch.Size += info.Size() + saved.mediaSize
			if info.ModTime().After(lastRecord) {
				lastRecord = info.ModTime()
			}
		}
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".blob") && !blobs[file.Name()] {
				_ = os.Remove(h.path(batch.ID, file.Name()))
			}
		}
		if batch.StoppedAt == nil {
			batch.StoppedAt = &lastRecord
		}
		batch.Count = len(batch.records)
		h.batches = append(h.batches, batch)
	}
	slices.SortFunc(h.batches, func(a, b *savedBatch) int { return a.StartedAt.Compare(b.StartedAt) })
	if err := h.prune(nil, 0, 0); err != nil {
		h.warn()
	}
	if err := h.pruneMedia(nil, 0); err != nil {
		h.warn()
	}
	return h
}

func historyRecordName(name string) (uint64, string) {
	if len(name) != 16+1+26+5 || name[16] != '-' || !strings.HasSuffix(name, ".json") || !ValidRecordID(name[17:43]) {
		return 0, ""
	}
	var seq uint64
	if n, err := fmt.Sscanf(name[:16], "%x", &seq); n != 1 || err != nil {
		return 0, ""
	}
	return seq, name[17:43]
}

func readHistoryJSON(path string, limit int64, value any) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	return err == nil && int64(len(data)) <= limit && json.Unmarshal(data, value) == nil
}

func (h *historyStore) path(parts ...string) string {
	return filepath.Join(append([]string{h.layout.TrafficHistory()}, parts...)...)
}

func (h *historyStore) warn() {
	_ = logfile.AppendEntry(h.layout.ServiceLog(), logfile.Entry{
		Level: "WARN", Component: "traffic", Event: "traffic.history", Result: "failed",
		ErrorCode: "traffic.history_incomplete", Message: "部分抓包记录未保存到本地",
	})
}

func (h *historyStore) saveBatch(batch *savedBatch) error {
	metadata := batch.Batch
	metadata.Count, metadata.Size, metadata.Running = 0, 0, false
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return provider.WriteAtomic(h.path(batch.ID, "batch.json"), data, 0o600)
}

func (h *historyStore) start(batch *savedBatch) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.batches = append(h.batches, batch)
	if h.saveBatch(batch) != nil || h.prune(batch, 0, 0) != nil {
		h.warn()
	}
}

func (h *historyStore) finish(batch *savedBatch) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now().UTC()
	batch.Running, batch.StoppedAt = false, &now
	if h.saveBatch(batch) != nil {
		h.warn()
	}
}

func (h *historyStore) writer(batch *savedBatch) *historyWriter {
	w := &historyWriter{queue: make(chan pendingRecord, 16), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		warned := false
		for record := range w.queue {
			if failed := h.appendPending(batch, record) != nil; !warned && (failed || w.dropped.Load()) {
				h.warn()
				warned = true
			}
		}
		if !warned && w.dropped.Load() {
			h.warn()
		}
	}()
	return w
}

func (w *historyWriter) stop() {
	close(w.queue)
	<-w.done
}

func (h *historyStore) append(batch *savedBatch, record Record) error {
	return h.appendPending(batch, pendingRecord{record: record})
}

func (h *historyStore) appendPending(batch *savedBatch, pending pendingRecord) error {
	record := pending.record
	spools := [...]*mediaSpool{pending.request, pending.response}
	for _, spool := range spools {
		if spool != nil {
			<-spool.done
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var mediaSize int64
	var moved []string
	committed := false
	defer func() {
		for _, spool := range spools {
			if spool != nil {
				spool.release()
			}
		}
		if !committed {
			for _, path := range moved {
				_ = os.Remove(path)
			}
		}
	}()
	if record.hasMedia {
		detail := *record.HTTP
		record.HTTP = &detail
	}
	for i, side := range []string{"request", "response"} {
		spool := spools[i]
		if spool == nil || spool.size == 0 {
			continue
		}
		path := h.path(batch.ID, mediaName(record.ID, side))
		if err := os.Rename(spool.path, path); err != nil {
			continue
		}
		moved = append(moved, path)
		body := bodyFor(record, side)
		body.Blob, body.CapturedSize = true, int(spool.size)
		body.Truncated = body.incomplete || spool.size < body.Size
		mediaSize += spool.size
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	size := int64(len(data))
	if size > historyRecordLimit || size > h.byteLimit {
		return errors.New("capture record exceeds history limit")
	}
	if err := h.prune(batch, size, 1); err != nil {
		return err
	}
	if err := h.pruneMedia(batch, size); err != nil {
		return err
	}
	for batch.Count >= historyCountLimit || batch.Size-batch.mediaSize+size > h.byteLimit {
		if err := h.removeOldestRecord(batch); err != nil {
			return err
		}
	}
	if _, err := os.Stat(h.path(batch.ID, "batch.json")); errors.Is(err, os.ErrNotExist) {
		if err := h.saveBatch(batch); err != nil {
			return err
		}
	}
	batch.sequence++
	name := fmt.Sprintf("%016x-%s.json", batch.sequence, record.ID)
	if err := provider.WriteAtomic(h.path(batch.ID, name), data, 0o600); err != nil {
		return err
	}
	batch.records = append(batch.records, savedRecord{name, record.ID, historyHost(record.Host), size, batch.sequence, mediaSize})
	batch.Count++
	batch.Size += size + mediaSize
	batch.mediaSize += mediaSize
	committed = true
	return nil
}

func (h *historyStore) removeOldestRecord(batch *savedBatch) error {
	first := batch.records[0]
	if err := os.Remove(h.path(batch.ID, first.name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if first.mediaSize != 0 {
		for _, side := range []string{"request", "response"} {
			_ = os.Remove(h.path(batch.ID, mediaName(first.id, side)))
		}
	}
	batch.records = batch.records[1:]
	batch.Count--
	batch.Size -= first.size + first.mediaSize
	batch.mediaSize -= first.mediaSize
	return nil
}

func (h *historyStore) prune(active *savedBatch, incoming int64, count int) error {
	total := incoming
	for _, batch := range h.batches {
		total += batch.Size - batch.mediaSize
		count += batch.Count
	}
	for i := 0; i < len(h.batches) && (len(h.batches) > historyBatchLimit || total > h.byteLimit || count > historyCountLimit); {
		batch := h.batches[i]
		if batch == active {
			i++
			continue
		}
		if err := os.RemoveAll(h.path(batch.ID)); err != nil {
			return err
		}
		total -= batch.Size - batch.mediaSize
		count -= batch.Count
		h.batches = slices.Delete(h.batches, i, i+1)
	}
	return nil
}

func (h *historyStore) list() []Batch {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]Batch, 0, len(h.batches))
	for i := len(h.batches) - 1; i >= 0; i-- {
		result = append(result, h.batches[i].Batch)
	}
	return result
}

func (h *historyStore) entries(id string) ([]Entry, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []Entry{}
	for i := len(h.batches) - 1; i >= 0; i-- {
		batch := h.batches[i]
		if id != "" && batch.ID != id {
			continue
		}
		for i := len(batch.records) - 1; i >= 0 && len(out) < maxEntries; i-- {
			file := batch.records[i]
			if entry, ok := h.readEntry(batch, file); ok {
				out = append(out, entry)
			}
		}
		return out, true
	}
	return out, id == ""
}

func (h *historyStore) readRecord(batch *savedBatch, file savedRecord) (Record, bool) {
	var record Record
	ok := readHistoryJSON(h.path(batch.ID, file.name), historyRecordLimit, &record)
	if ok && record.ID == file.id {
		for _, side := range []string{"request", "response"} {
			if body := bodyFor(record, side); body != nil && body.Blob {
				info, err := os.Lstat(h.path(batch.ID, mediaName(record.ID, side)))
				if err != nil || !info.Mode().IsRegular() || info.Size() != int64(body.CapturedSize) || info.Size() <= 0 || info.Size() > MaxMediaBody {
					body.Blob, body.CapturedSize, body.Truncated = false, 0, true
				}
			}
		}
	}
	return record, ok && record.ID == file.id
}

func (h *historyStore) readEntry(batch *savedBatch, file savedRecord) (Entry, bool) {
	var entry Entry
	ok := readHistoryJSON(h.path(batch.ID, file.name), historyRecordLimit, &entry)
	return entry, ok && entry.ID == file.id
}

func (h *historyStore) record(id string) (Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, batch := range h.batches {
		for _, file := range batch.records {
			if file.id == id {
				return h.readRecord(batch, file)
			}
		}
	}
	return Record{}, false
}

type HistoryPage struct {
	Batch   string  `json:"batch,omitempty"`
	Entries []Entry `json:"entries"`
	Before  string  `json:"before,omitempty"`
	Since   string  `json:"since,omitempty"`
	Oldest  string  `json:"oldest,omitempty"`
}

type PageRequest struct {
	Batch         string
	Host          string
	Before, Since uint64
	Incremental   bool
	Limit         int
}

func (c *Controller) HistoryPage(request PageRequest) (HistoryPage, bool) {
	if c.history == nil {
		return HistoryPage{Entries: []Entry{}}, request.Batch == ""
	}
	return c.history.page(request)
}

func (h *historyStore) page(request PageRequest) (HistoryPage, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	page := HistoryPage{Entries: []Entry{}}
	for i := len(h.batches) - 1; i >= 0; i-- {
		batch := h.batches[i]
		if request.Batch != "" && request.Batch != batch.ID {
			continue
		}
		page.Batch = batch.ID
		if len(batch.records) == 0 {
			return page, true
		}
		limit := min(MaxHistoryPage, max(1, request.Limit))
		page.Oldest = strconv.FormatUint(batch.records[0].sequence, 10)
		records := batch.records
		if request.Host != "" {
			host := historyHost(request.Host)
			records = nil
			for _, record := range batch.records {
				if record.host == host {
					records = append(records, record)
				}
			}
		}
		low, high := 0, len(records)
		if request.Incremental {
			low, _ = slices.BinarySearchFunc(records, request.Since, compareSequence)
			for low < high && records[low].sequence <= request.Since {
				low++
			}
			high = min(high, low+limit)
		} else {
			if request.Before != 0 {
				high, _ = slices.BinarySearchFunc(records, request.Before, compareSequence)
			}
			low = max(0, high-limit)
			if low > 0 {
				page.Before = strconv.FormatUint(records[low].sequence, 10)
			}
		}
		if request.Host != "" && request.Before == 0 && high == len(records) {
			page.Since = strconv.FormatUint(batch.records[len(batch.records)-1].sequence, 10)
		} else if high > 0 {
			page.Since = strconv.FormatUint(records[high-1].sequence, 10)
		}
		for i := high - 1; i >= low; i-- {
			if entry, ok := h.readEntry(batch, records[i]); ok {
				page.Entries = append(page.Entries, entry)
			}
		}
		return page, true
	}
	return page, request.Batch == ""
}

type CaptureDomain struct {
	Host  string `json:"host"`
	Count int    `json:"count"`
}

func (c *Controller) HistoryDomains(id string) ([]CaptureDomain, bool) {
	if c.history == nil {
		return []CaptureDomain{}, id == ""
	}
	return c.history.domains(id)
}

func (h *historyStore) domains(id string) ([]CaptureDomain, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	domains := []CaptureDomain{}
	for i := len(h.batches) - 1; i >= 0; i-- {
		batch := h.batches[i]
		if id != "" && batch.ID != id {
			continue
		}
		counts := map[string]int{}
		for _, record := range batch.records {
			if record.host != "" {
				counts[record.host]++
			}
		}
		for host, count := range counts {
			domains = append(domains, CaptureDomain{Host: host, Count: count})
		}
		slices.SortFunc(domains, func(a, b CaptureDomain) int { return strings.Compare(a.Host, b.Host) })
		return domains, true
	}
	return domains, id == ""
}

func historyHost(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	return canonicalHost(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
}

func compareSequence(record savedRecord, sequence uint64) int {
	switch {
	case record.sequence < sequence:
		return -1
	case record.sequence > sequence:
		return 1
	default:
		return 0
	}
}

func (h *historyStore) remove(id string, active *savedBatch) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := 0; i < len(h.batches); {
		batch := h.batches[i]
		if id != "" && batch.ID != id {
			i++
			continue
		}
		if batch == active {
			for batch.Count != 0 {
				if err := h.removeOldestRecord(batch); err != nil {
					return false, err
				}
			}
			i++
		} else {
			if err := os.RemoveAll(h.path(batch.ID)); err != nil {
				return false, err
			}
			h.batches = slices.Delete(h.batches, i, i+1)
		}
		if id != "" {
			return true, nil
		}
	}
	return id == "", nil
}
