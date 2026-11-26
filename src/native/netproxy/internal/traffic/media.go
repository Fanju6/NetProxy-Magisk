package traffic

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"
)

var errMediaBudget = errors.New("capture media exceeds history limit")

const (
	MaxMediaBody = 16 << 20
	mediaBudget  = 256 << 20
	mediaChunk   = 32 << 10
)

func mediaBody(headers http.Header) bool {
	kind, _, _ := mime.ParseMediaType(headers.Get("Content-Type"))
	disposition, _, _ := mime.ParseMediaType(headers.Get("Content-Disposition"))
	return disposition == "attachment" || kind != "" && !textBody(kind)
}

func textBody(kind string) bool {
	return kind == "" || strings.HasPrefix(kind, "text/") || strings.HasSuffix(kind, "+json") ||
		strings.HasSuffix(kind, "+xml") || kind == "application/json" || kind == "application/xml" ||
		kind == "application/x-www-form-urlencoded" || kind == "application/javascript" || kind == "image/svg+xml"
}

type mediaSpool struct {
	store  *historyStore
	batch  *savedBatch
	chunks chan []byte
	done   chan struct{}
	path   string
	size   int64
	queued int
	full   bool
}

func (c *Controller) mediaFactory(owner *session) func() *mediaSpool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != owner || c.writer == nil {
		return nil
	}
	h, batch := c.history, c.batch
	return func() *mediaSpool {
		select {
		case h.mediaSlots <- struct{}{}:
			s := &mediaSpool{store: h, batch: batch, chunks: make(chan []byte, 16), done: make(chan struct{})}
			go s.write()
			return s
		default:
			return nil
		}
	}
}

func (s *mediaSpool) offer(data []byte) {
	for len(data) > 0 && !s.full && s.queued < MaxMediaBody {
		n := min(len(data), mediaChunk, MaxMediaBody-s.queued)
		chunk := append([]byte(nil), data[:n]...)
		select {
		case s.chunks <- chunk:
		default:
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case s.chunks <- chunk:
			case <-timer.C:
				s.full = true
			case <-s.done:
				s.full = true
			}
			timer.Stop()
			if s.full {
				return
			}
		}
		s.queued += n
		data = data[n:]
	}
}

func (s *mediaSpool) write() {
	defer close(s.done)
	file, err := os.CreateTemp(s.store.path(s.batch.ID), ".netproxy-*.tmp")
	if err != nil {
		return
	}
	s.path = file.Name()
	defer file.Close()
	for chunk := range s.chunks {
		h := s.store
		h.mu.Lock()
		if !s.batch.Running {
			h.mu.Unlock()
			break
		}
		err := h.pruneMedia(s.batch, int64(len(chunk)))
		if err == nil {
			h.staged += int64(len(chunk))
		}
		h.mu.Unlock()
		if err != nil {
			break
		}
		n, err := file.Write(chunk)
		s.size += int64(n)
		if n != len(chunk) {
			h.mu.Lock()
			h.staged -= int64(len(chunk) - n)
			h.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
}

func (s *mediaSpool) discard() {
	if s != nil {
		go func() {
			<-s.done
			s.store.mu.Lock()
			defer s.store.mu.Unlock()
			s.release()
		}()
	}
}

func (s *mediaSpool) release() {
	s.store.staged -= s.size
	if s.path != "" {
		_ = os.Remove(s.path)
	}
	<-s.store.mediaSlots
}

func (h *historyStore) pruneMedia(active *savedBatch, incoming int64) error {
	total := h.staged + incoming
	for _, batch := range h.batches {
		total += batch.Size
	}
	for i := 0; i < len(h.batches) && total > h.mediaLimit; {
		batch := h.batches[i]
		if batch == active || batch.Running {
			i++
			continue
		}
		if err := os.RemoveAll(h.path(batch.ID)); err != nil {
			return err
		}
		total -= batch.Size
		h.batches = append(h.batches[:i], h.batches[i+1:]...)
	}
	if total > h.mediaLimit {
		return errMediaBudget
	}
	return nil
}

func mediaName(id, side string) string { return id + "." + side + ".blob" }

func bodyFor(record Record, side string) *CapturedBody {
	if record.HTTP != nil {
		switch side {
		case "request":
			return &record.HTTP.Request.Body
		case "response":
			return &record.HTTP.Response.Body
		}
	}
	return nil
}

func (c *Controller) OpenBody(id, side string) (*os.File, bool) {
	if c.history == nil || !ValidRecordID(id) || side != "request" && side != "response" {
		return nil, false
	}
	h := c.history
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, batch := range h.batches {
		for _, saved := range batch.records {
			if saved.id != id {
				continue
			}
			record, _ := h.readRecord(batch, saved)
			if body := bodyFor(record, side); body != nil && body.Blob {
				file, err := os.Open(h.path(batch.ID, mediaName(id, side)))
				return file, err == nil
			}
			return nil, false
		}
	}
	return nil, false
}
