package traffic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	name string
	size int64
}

type savedBatch struct {
	Batch
	records  []savedRecord
	sequence uint64
}

type historyStore struct {
	mu        sync.Mutex
	layout    paths.Layout
	batches   []*savedBatch
	byteLimit int64
}

type historyWriter struct {
	queue   chan Record
	done    chan struct{}
	dropped atomic.Bool
}

func openHistory(layout paths.Layout) *historyStore {
	h := &historyStore{layout: layout, byteLimit: historyByteLimit}
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
			saved := savedRecord{file.Name(), info.Size()}
			if _, ok := h.readRecord(batch, saved); !ok {
				_ = os.Remove(h.path(batch.ID, file.Name()))
				continue
			}
			batch.sequence = max(batch.sequence, seq)
			batch.records = append(batch.records, saved)
			batch.Size += info.Size()
			if info.ModTime().After(lastRecord) {
				lastRecord = info.ModTime()
			}
		}
		if batch.StoppedAt == nil {
			batch.StoppedAt = &lastRecord
		}
		batch.Count = len(batch.records)
		h.batches = append(h.batches, batch)
	}
	slices.SortFunc(h.batches, func(a, b *savedBatch) int { return a.StartedAt.Compare(b.StartedAt) })
	if err := h.prune(nil, 0); err != nil {
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
	if h.saveBatch(batch) != nil || h.prune(batch, 0) != nil {
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
	w := &historyWriter{queue: make(chan Record, 16), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		warned := false
		for record := range w.queue {
			if failed := h.append(batch, record) != nil; !warned && (failed || w.dropped.Load()) {
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
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	size := int64(len(data))
	if size > historyRecordLimit || size > h.byteLimit {
		return errors.New("capture record exceeds history limit")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.prune(batch, size); err != nil {
		return err
	}
	for batch.Count >= maxEntries || batch.Size+size > h.byteLimit {
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
	batch.records = append(batch.records, savedRecord{name, size})
	batch.Count++
	batch.Size += size
	return nil
}

func (h *historyStore) removeOldestRecord(batch *savedBatch) error {
	first := batch.records[0]
	if err := os.Remove(h.path(batch.ID, first.name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	batch.records = batch.records[1:]
	batch.Count--
	batch.Size -= first.size
	return nil
}

func (h *historyStore) prune(active *savedBatch, incoming int64) error {
	total := incoming
	for _, batch := range h.batches {
		total += batch.Size
	}
	for i := 0; i < len(h.batches) && (len(h.batches) > historyBatchLimit || total > h.byteLimit); {
		batch := h.batches[i]
		if batch == active {
			i++
			continue
		}
		if err := os.RemoveAll(h.path(batch.ID)); err != nil {
			return err
		}
		total -= batch.Size
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
		for i := len(batch.records) - 1; i >= 0; i-- {
			file := batch.records[i]
			if record, ok := h.readRecord(batch, file); ok {
				out = append(out, record.Entry)
			}
		}
		return out, true
	}
	return out, id == ""
}

func (h *historyStore) readRecord(batch *savedBatch, file savedRecord) (Record, bool) {
	var record Record
	_, id := historyRecordName(file.name)
	ok := readHistoryJSON(h.path(batch.ID, file.name), historyRecordLimit, &record)
	return record, ok && id != "" && record.ID == id
}

func (h *historyStore) record(id string) (Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, batch := range h.batches {
		for _, file := range batch.records {
			if _, candidate := historyRecordName(file.name); candidate == id {
				return h.readRecord(batch, file)
			}
		}
	}
	return Record{}, false
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
