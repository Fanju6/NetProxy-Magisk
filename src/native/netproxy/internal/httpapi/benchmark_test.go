package httpapi

import (
	"net/http/httptest"
	"testing"
)

func BenchmarkHTTPRead(b *testing.B) {
	for _, test := range []struct{ name, path string }{
		{"Info", "/api/v1/info"},
		{"ModuleSection", "/api/v1/config?target=module/auto_start"},
	} {
		b.Run(test.name, func(b *testing.B) {
			server := resourceFixture(b)
			request := httptest.NewRequest("GET", test.path, nil)
			request.Host = server.config.Listen
			request.Header.Set("Authorization", "Bearer "+server.config.Secret)
			b.ReportAllocs()
			for b.Loop() {
				writer := httptest.NewRecorder()
				server.ServeHTTP(writer, request)
				if writer.Code != 200 {
					b.Fatal(writer.Body.String())
				}
			}
		})
	}
}
