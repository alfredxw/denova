package webfs

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedFrontendRoutesAndAssets(t *testing.T) {
	handler := spaHandler(fstest.MapFS{
		"index.html":    {Data: []byte("<html>current build</html>")},
		"assets/app.js": {Data: []byte("console.log('current build')")},
	})
	for _, tc := range []struct{ method, path, body, contentType string }{
		{"GET", "/", "<html>current build</html>", "text/html"},
		{"GET", "/writing/book?denova_reload=next", "<html>current build</html>", "text/html"},
		{"GET", "/game/story", "<html>current build</html>", "text/html"},
		{"GET", "/assets/", "<html>current build</html>", "text/html"},
		{"GET", "/assets/app.js", "console.log('current build')", "javascript"},
		{"HEAD", "/writing/book", "", "text/html"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != 200 || response.Body.String() != tc.body || !strings.Contains(response.Header().Get("Content-Type"), tc.contentType) {
				t.Fatalf("response: %d %v %q", response.Code, response.Header(), response.Body.String())
			}
			if tc.contentType == "text/html" && response.Header().Get("Cache-Control") != "no-cache" {
				t.Fatal("SPA shell can remain cached across updates")
			}
			if strings.HasPrefix(tc.path, "/assets/") && tc.contentType != "text/html" {
				if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
					t.Fatalf("hashed asset must be immutable, got Cache-Control %q", got)
				}
			}
		})
	}
}
