package traffic

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	grpcMessageLimit = 8
	grpcMessageBytes = 2048
)

type GRPCDetail struct {
	Messages  []GRPCMessage `json:"messages"`
	Truncated bool          `json:"truncated,omitempty"`
	Status    string        `json:"status,omitempty"`
	Message   string        `json:"message,omitempty"`
}

type GRPCMessage struct {
	Size       uint32       `json:"size"`
	Compressed bool         `json:"compressed,omitempty"`
	Truncated  bool         `json:"truncated,omitempty"`
	Fields     []ProtoField `json:"fields,omitempty"`
}

type ProtoField struct {
	Number uint32       `json:"number"`
	Wire   byte         `json:"wire"`
	Value  string       `json:"value,omitempty"`
	Fields []ProtoField `json:"fields,omitempty"`
}

func grpcContentType(header http.Header) bool {
	kind, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
	return kind == "application/grpc" || strings.HasPrefix(kind, "application/grpc+")
}

type grpcCapture struct {
	detail    GRPCDetail
	encoding  string
	header    [5]byte
	headerLen int
	remaining uint32
	data      []byte
}

func (c *grpcCapture) offer(p []byte) {
	for len(p) > 0 {
		if len(c.detail.Messages) == grpcMessageLimit {
			c.detail.Truncated = true
			return
		}
		if c.headerLen < len(c.header) {
			n := copy(c.header[c.headerLen:], p)
			p = p[n:]
			c.headerLen += n
			if c.headerLen < len(c.header) {
				return
			}
			c.remaining = binary.BigEndian.Uint32(c.header[1:])
		}
		n := min(uint64(len(p)), uint64(c.remaining))
		if binary.BigEndian.Uint32(c.header[1:]) <= grpcMessageBytes {
			c.data = append(c.data, p[:n]...)
		}
		p = p[n:]
		c.remaining -= uint32(n)
		if c.remaining == 0 {
			c.complete()
		}
	}
}

func (c *grpcCapture) complete() {
	size := binary.BigEndian.Uint32(c.header[1:])
	m := GRPCMessage{Size: size, Compressed: c.header[0] == 1, Truncated: size > grpcMessageBytes || c.header[0] > 1}
	data := c.data
	if m.Compressed && !m.Truncated {
		if c.encoding == "gzip" {
			reader, err := gzip.NewReader(bytes.NewReader(data))
			if err == nil {
				data, err = io.ReadAll(io.LimitReader(reader, grpcMessageBytes+1))
				_ = reader.Close()
			}
			m.Truncated = err != nil || len(data) > grpcMessageBytes
		} else {
			m.Truncated = true
		}
	}
	if !m.Truncated {
		budget := 32
		var ok bool
		m.Fields, ok = protoFields(data, 0, &budget)
		m.Truncated = !ok
	}
	c.detail.Messages = append(c.detail.Messages, m)
	c.headerLen, c.data = 0, c.data[:0]
}

func (c *grpcCapture) snapshot(headers, trailers http.Header) *GRPCDetail {
	detail := c.detail
	detail.Messages = append([]GRPCMessage{}, detail.Messages...)
	detail.Truncated = detail.Truncated || c.headerLen != 0
	status, message := trailers.Get("Grpc-Status"), trailers.Get("Grpc-Message")
	if status == "" {
		status, message = headers.Get("Grpc-Status"), headers.Get("Grpc-Message")
	}
	detail.Status = validPrefix(status, 16)
	if decoded, err := url.PathUnescape(message); err == nil {
		message = decoded
	}
	detail.Message = strings.ToValidUTF8(validPrefix(message, 256), "?")
	return &detail
}

func protoFields(data []byte, depth int, budget *int) ([]ProtoField, bool) {
	var fields []ProtoField
	for len(data) > 0 {
		key, n := binary.Uvarint(data)
		if n <= 0 || *budget <= 0 || key>>3 == 0 || key>>3 >= 1<<29 {
			return fields, false
		}
		*budget--
		f := ProtoField{Number: uint32(key >> 3), Wire: byte(key & 7)}
		data = data[n:]
		switch f.Wire {
		case 0:
			value, n := binary.Uvarint(data)
			if n <= 0 {
				return fields, false
			}
			f.Value, data = strconv.FormatUint(value, 10), data[n:]
		case 1, 5:
			size := 8
			if f.Wire == 5 {
				size = 4
			}
			if len(data) < size {
				return fields, false
			}
			f.Value, data = "0x"+hex.EncodeToString(data[:size]), data[size:]
		case 2:
			length, n := binary.Uvarint(data)
			if n <= 0 || length > uint64(len(data)-n) {
				return fields, false
			}
			value := data[n : n+int(length)]
			data = data[n+int(length):]
			if utf8.Valid(value) && strings.IndexFunc(string(value), func(r rune) bool { return !unicode.IsPrint(r) }) < 0 {
				f.Value = validPrefix(string(value), 96)
			} else {
				if depth < 3 && len(value) > 0 {
					nestedBudget := *budget
					if nested, ok := protoFields(value, depth+1, &nestedBudget); ok {
						*budget, f.Fields = nestedBudget, nested
					}
				}
				if len(f.Fields) == 0 {
					f.Value = "0x" + hex.EncodeToString(value[:min(48, len(value))])
				}
			}
		default:
			return fields, false
		}
		fields = append(fields, f)
	}
	return fields, true
}
