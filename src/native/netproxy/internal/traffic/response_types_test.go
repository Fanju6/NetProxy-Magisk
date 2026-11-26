package traffic

import (
	"net/http"
	"testing"
)

func TestResponseBodyType(t *testing.T) {
	for _, test := range []struct {
		header, media, want string
	}{
		{"application/problem+json; charset=utf-8", "image/png", "json"},
		{"application/xhtml+xml", "", "html"},
		{"application/atom+xml", "", "xml"},
		{"text/css; charset=UTF-8", "", "text"},
		{"", "image/svg+xml", "image"},
		{"video/mp4", "", "video"},
		{"audio/aac", "", "audio"},
		{"application/octet-stream", "", "binary"},
		{"", "", "binary"},
	} {
		record := Record{Entry: Entry{Method: "GET", Status: 200}, HTTP: &HTTPDetail{Response: CapturedMessage{
			Headers: http.Header{"Content-Type": {test.header}}, Body: CapturedBody{ContentType: test.media, Size: 2},
		}}}
		if got := responseTypeNames[responseBodyType(record)]; got != test.want {
			t.Errorf("%q / %q: got %q, want %q", test.header, test.media, got, test.want)
		}
		record.Method = "HEAD"
		if got := responseTypeNames[responseBodyType(record)]; got != "none" {
			t.Errorf("HEAD: got %q, want none", got)
		}
	}
	if got := responseTypeNames[responseBodyType(Record{Entry: Entry{Method: "TCP"}})]; got != "none" {
		t.Errorf("TCP passthrough: got %q, want none", got)
	}
}
