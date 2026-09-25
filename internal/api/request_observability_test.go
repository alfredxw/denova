package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"denova/internal/observability"
	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestHTTPErrorDiagnosticsKeepCauseAndRequestContext(t *testing.T) {
	server := hertzserver.New()
	server.Use(requestObservabilityMiddleware)
	server.POST("/files/:id", func(_ context.Context, c *app.RequestContext) {
		c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "Save failed", "code": "file.save_failed",
			"details": map[string]any{"detail": "write /Users/alice/Novel/chapter.md: disk full; api_key=private-key"},
		})
	})
	response := ut.PerformRequest(server.Engine, http.MethodPost, "/files/private-title", nil)
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	details := body["details"].(map[string]any)
	if body["error"] != "Save failed" || body["code"] != "file.save_failed" || body["request_id"] == "" || body["request_id"] != response.Header().Get(observability.RequestIDHeader) || details["operation"] != "POST /files/:id" || details["backend_version"] == "" || details["platform"] == "" {
		t.Fatalf("incomplete HTTP diagnostics: %s", response.Body.String())
	}
	if !strings.Contains(details["detail"].(string), "disk full") {
		t.Fatalf("cause was lost: %#v", details)
	}
	for _, secret := range []string{"alice", "private-key", "private-title"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("diagnostic exposed %q: %s", secret, response.Body.String())
		}
	}
}
