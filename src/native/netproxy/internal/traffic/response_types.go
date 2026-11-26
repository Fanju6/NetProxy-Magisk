package traffic

import (
	"mime"
	"slices"
	"strings"
)

var responseTypeNames = [...]string{"json", "html", "xml", "text", "image", "video", "audio", "binary", "none"}

type ResponseTypes uint16

func ParseResponseTypes(value string) (ResponseTypes, bool) {
	var types ResponseTypes
	if value == "" {
		return types, true
	}
	if len(value) > len("json,html,xml,text,image,video,audio,binary,none") {
		return 0, false
	}
	for name := range strings.SplitSeq(value, ",") {
		index := slices.Index(responseTypeNames[:], name)
		if index < 0 {
			return 0, false
		}
		bit := ResponseTypes(1) << index
		if types&bit != 0 {
			return 0, false
		}
		types |= bit
	}
	return types, true
}

func (types ResponseTypes) matches(index int) bool {
	return types == 0 || types&(1<<index) != 0
}

func responseBodyType(record Record) int {
	if record.HTTP == nil || record.Method == "HEAD" || record.Status == 204 || record.Status == 304 ||
		record.Status >= 100 && record.Status < 200 || record.HTTP.Response.Body.Size == 0 {
		return len(responseTypeNames) - 1
	}
	response := record.HTTP.Response
	contentType := response.Headers.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if !strings.Contains(mediaType, "/") {
		mediaType, _, _ = mime.ParseMediaType(response.Body.ContentType)
	}
	name := "binary"
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		name = "image"
	case strings.HasPrefix(mediaType, "video/"):
		name = "video"
	case strings.HasPrefix(mediaType, "audio/"):
		name = "audio"
	case mediaType == "application/json", mediaType == "text/json", strings.HasSuffix(mediaType, "+json"):
		name = "json"
	case mediaType == "text/html", mediaType == "application/xhtml+xml":
		name = "html"
	case mediaType == "application/xml", mediaType == "text/xml", strings.HasSuffix(mediaType, "+xml"):
		name = "xml"
	case strings.HasPrefix(mediaType, "text/"):
		name = "text"
	}
	return slices.Index(responseTypeNames[:], name)
}

type CaptureResponseType struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

func (c *Controller) HistoryResponseTypes(id, host string) ([]CaptureResponseType, bool) {
	types := make([]CaptureResponseType, len(responseTypeNames))
	for i, name := range responseTypeNames {
		types[i].Type = name
	}
	if c.history == nil {
		return types, id == ""
	}
	if id == "" {
		id = c.activeHistoryBatch()
	}
	host = historyHost(host)
	h := c.history
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.batches) - 1; i >= 0; i-- {
		batch := h.batches[i]
		if id != "" && batch.ID != id {
			continue
		}
		for _, record := range batch.records {
			if host == "" || record.host == host {
				types[record.responseType].Count++
			}
		}
		return types, true
	}
	return types, id == ""
}
