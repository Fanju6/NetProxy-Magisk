package traffic

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

const (
	wsFrameLimit   = 64
	wsPreviewLimit = 256
	wsMessageLimit = 64 << 10
)

type WebSocketDetail struct {
	Frames            []WebSocketFrame `json:"frames"`
	Messages          []WebSocketFrame `json:"messages,omitempty"`
	Truncated         bool             `json:"truncated"`
	MessagesTruncated bool             `json:"messages_truncated,omitempty"`
}

type WebSocketFrame struct {
	Direction       string  `json:"direction"`
	Opcode          byte    `json:"opcode"`
	Size            uint64  `json:"size"`
	Text            string  `json:"text,omitempty"`
	CloseCode       *uint16 `json:"close_code,omitempty"`
	Compressed      bool    `json:"compressed,omitempty"`
	Truncated       bool    `json:"truncated"`
	sequence        uint64
	final, reserved bool
	sample          []byte
}

func webSocketUpgrade(headers http.Header) bool {
	for _, value := range headers.Values("Connection") {
		for token := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return strings.EqualFold(headers.Get("Upgrade"), "websocket")
			}
		}
	}
	return false
}

func setWebSocketUpgrade(headers http.Header) {
	headers.Set("Connection", "Upgrade")
	headers.Set("Upgrade", "websocket")
}

type webSocketCapture struct {
	mu                                        sync.Mutex
	next                                      atomic.Uint64
	detail                                    WebSocketDetail
	deflate, clientNoContext, serverNoContext bool
}

type webSocketFrameForwarder struct {
	dst      io.Writer
	next     *atomic.Uint64
	sequence uint64
}

func (f *webSocketFrameForwarder) Write(p []byte) (int, error) {
	// 首次转发前编号；对端可能在本次 Write 返回前发出回复。
	if len(p) != 0 && f.sequence == 0 {
		f.sequence = f.next.Add(1)
	}
	return f.dst.Write(p)
}

func (c *webSocketCapture) relay(dst io.Writer, src io.Reader, direction string) {
	forwarder := &webSocketFrameForwarder{dst: dst, next: &c.next}
	stream := io.TeeReader(src, forwarder)
	noContext := c.clientNoContext
	if direction == "receive" {
		noContext = c.serverNoContext
	}
	var pending *WebSocketFrame
	inflater := wsInflater{noContext: noContext}
	var scratch [wsMessageLimit]byte
	defer func() {
		if pending != nil {
			pending.sample, pending.Truncated = nil, true
			c.recordMessage(*pending)
		}
	}()
	for {
		forwarder.sequence = 0
		frame, err := readWebSocketFrame(stream, scratch[:])
		if err != nil {
			return
		}
		frame.Direction = direction
		frame.sequence = forwarder.sequence
		switch frame.Opcode {
		case 1, 2:
			if pending != nil {
				pending.sample, pending.Truncated = nil, true
				c.recordMessage(*pending)
			}
			pending = &WebSocketFrame{Direction: direction, Opcode: frame.Opcode, sequence: frame.sequence,
				Compressed: frame.Compressed, Truncated: frame.reserved || frame.Compressed && !c.deflate}
		}
		if pending != nil && (frame.Opcode == 0 || frame.Opcode == 1 || frame.Opcode == 2) {
			pending.Size += frame.Size
			pending.Truncated = pending.Truncated || frame.reserved || frame.Opcode == 0 && frame.Compressed
			if pending.Opcode == 1 || pending.Compressed {
				pending.sample = append(pending.sample, frame.sample[:min(len(frame.sample), wsMessageLimit-len(pending.sample))]...)
				pending.Truncated = pending.Truncated || pending.Size > wsMessageLimit
			}
			if frame.final {
				if pending.Compressed && c.deflate {
					pending.sample, pending.Truncated = inflater.decode(pending.sample, pending.Truncated)
				}
				if pending.Opcode == 1 {
					pending.Text, pending.Truncated = wsText(pending.sample, pending.Truncated, wsPreviewLimit)
				}
				pending.sample = nil
				c.recordMessage(*pending)
				pending = nil
			}
		}
		frame.sample = nil
		c.record(frame)
	}
}

func (c *webSocketCapture) record(frame WebSocketFrame) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.detail.Frames = keepWebSocket(c.detail.Frames, frame, &c.detail.Truncated)
}

func (c *webSocketCapture) recordMessage(message WebSocketFrame) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.detail.Messages = keepWebSocket(c.detail.Messages, message, &c.detail.MessagesTruncated)
}

func keepWebSocket(frames []WebSocketFrame, frame WebSocketFrame, truncated *bool) []WebSocketFrame {
	index := sort.Search(len(frames), func(i int) bool { return frames[i].sequence > frame.sequence })
	if len(frames) < wsFrameLimit {
		frames = append(frames, WebSocketFrame{})
	} else {
		*truncated = true
		if index == len(frames) {
			return frames
		}
	}
	copy(frames[index+1:], frames[index:])
	frames[index] = frame
	return frames
}

func readWebSocketFrame(stream io.Reader, scratch []byte) (WebSocketFrame, error) {
	var header [2]byte
	if _, err := io.ReadFull(stream, header[:]); err != nil {
		return WebSocketFrame{}, err
	}
	frame := WebSocketFrame{Opcode: header[0] & 15, Size: uint64(header[1] & 127), final: header[0]&0x80 != 0,
		Compressed: header[0]&0x40 != 0, reserved: header[0]&0x30 != 0}
	switch frame.Size {
	case 126:
		var length uint16
		if err := binary.Read(stream, binary.BigEndian, &length); err != nil {
			return frame, err
		}
		frame.Size = uint64(length)
	case 127:
		if err := binary.Read(stream, binary.BigEndian, &frame.Size); err != nil {
			return frame, err
		}
		if frame.Size>>63 != 0 {
			return frame, io.ErrUnexpectedEOF
		}
	}
	var mask [4]byte
	masked := header[1]&128 != 0
	if masked {
		if _, err := io.ReadFull(stream, mask[:]); err != nil {
			return frame, err
		}
	}
	size := int64(frame.Size)
	if frame.Opcode <= 2 || frame.Opcode == 8 {
		data := scratch[:min(size, int64(len(scratch)))]
		if _, err := io.ReadFull(stream, data); err != nil {
			return frame, err
		}
		if masked {
			for i := range data {
				data[i] ^= mask[i%len(mask)]
			}
		}
		size -= int64(len(data))
		frame.sample = data
		if frame.Opcode == 1 && !frame.Compressed && !frame.reserved {
			frame.Text, frame.Truncated = wsText(data, size != 0, wsPreviewLimit)
		}
		if frame.Opcode == 8 && len(data) >= 2 {
			code := binary.BigEndian.Uint16(data)
			frame.CloseCode = &code
			frame.Text, frame.Truncated = wsText(data[2:], size != 0, wsPreviewLimit)
		}
	}
	_, err := io.CopyN(io.Discard, stream, size)
	return frame, err
}

func wsText(data []byte, truncated bool, limit int) (string, bool) {
	truncated = truncated || len(data) > limit
	data = data[:min(len(data), limit)]
	for clipped := 0; truncated && len(data) != 0 && clipped < utf8.UTFMax-1 && !utf8.Valid(data); clipped++ {
		data = data[:len(data)-1]
	}
	if utf8.Valid(data) {
		return string(data), truncated
	}
	return "", true
}

type wsInflater struct {
	dict      []byte
	noContext bool
	unusable  bool
}

func (i *wsInflater) decode(data []byte, truncated bool) ([]byte, bool) {
	if truncated || i.unusable {
		i.unusable = !i.noContext
		return nil, true
	}
	src := io.MultiReader(bytes.NewReader(data), bytes.NewReader([]byte{0, 0, 0xff, 0xff}))
	r := flate.NewReaderDict(src, i.dict)
	body, err := io.ReadAll(io.LimitReader(r, wsMessageLimit+1))
	_ = r.Close()
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) || len(body) > wsMessageLimit {
		i.unusable = !i.noContext
		return body[:min(len(body), wsPreviewLimit)], true
	}
	if !i.noContext {
		i.dict = append(i.dict, body...)
		if len(i.dict) > 32<<10 {
			i.dict = append(i.dict[:0], i.dict[len(i.dict)-(32<<10):]...)
		}
	}
	return body, false
}

func (c *webSocketCapture) configure(headers http.Header) {
	for _, line := range headers.Values("Sec-WebSocket-Extensions") {
		for extension := range strings.SplitSeq(line, ",") {
			name, options, _ := strings.Cut(extension, ";")
			if strings.TrimSpace(name) != "permessage-deflate" {
				continue
			}
			c.deflate = true
			for option := range strings.SplitSeq(options, ";") {
				switch strings.TrimSpace(option) {
				case "client_no_context_takeover":
					c.clientNoContext = true
				case "server_no_context_takeover":
					c.serverNoContext = true
				}
			}
		}
	}
}
