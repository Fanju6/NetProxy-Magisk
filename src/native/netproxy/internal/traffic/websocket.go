package traffic

import (
	"encoding/binary"
	"io"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	wsFrameLimit   = 64
	wsPreviewLimit = 256
)

type WebSocketDetail struct {
	Frames    []WebSocketFrame `json:"frames"`
	Truncated bool             `json:"truncated"`
}

type WebSocketFrame struct {
	Direction string `json:"direction"`
	Opcode    byte   `json:"opcode"`
	Size      uint64 `json:"size"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated"`
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
	mu     sync.Mutex
	detail WebSocketDetail
}

func (c *webSocketCapture) relay(dst io.Writer, src io.Reader, direction string) {
	stream := io.TeeReader(src, dst)
	for {
		frame, err := readWebSocketFrame(stream)
		if err != nil {
			return
		}
		frame.Direction = direction
		c.mu.Lock()
		if len(c.detail.Frames) < wsFrameLimit {
			c.detail.Frames = append(c.detail.Frames, frame)
		} else {
			c.detail.Truncated = true
		}
		c.mu.Unlock()
	}
}

func readWebSocketFrame(stream io.Reader) (WebSocketFrame, error) {
	var header [2]byte
	if _, err := io.ReadFull(stream, header[:]); err != nil {
		return WebSocketFrame{}, err
	}
	frame := WebSocketFrame{Opcode: header[0] & 15, Size: uint64(header[1] & 127)}
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
	if frame.Opcode == 1 && header[0]&0x70 == 0 {
		var preview [wsPreviewLimit]byte
		data := preview[:min(size, wsPreviewLimit)]
		if _, err := io.ReadFull(stream, data); err != nil {
			return frame, err
		}
		if masked {
			for i := range data {
				data[i] ^= mask[i%len(mask)]
			}
		}
		frame.Truncated = int64(len(data)) < size
		size -= int64(len(data))
		for len(data) > 0 && !utf8.Valid(data) && frame.Truncated && len(preview)-len(data) < utf8.UTFMax {
			data = data[:len(data)-1]
		}
		if utf8.Valid(data) {
			frame.Text = string(data)
		}
	}
	_, err := io.CopyN(io.Discard, stream, size)
	return frame, err
}
