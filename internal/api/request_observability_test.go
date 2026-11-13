package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"denova/internal/observability"
	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	hertzgzip "github.com/hertz-contrib/gzip"
)

// Browsers always send Accept-Encoding: gzip, and the gzip middleware replaces
// the response with a compressed body stream. The error text must still reach
// both the response envelope and the completion log record.
func TestCompressedErrorResponsesKeepDiagnostics(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	observability.ConfigureStructuredLogging(&logs)
	defer slog.SetDefault(previous)

	server := hertzserver.New()
	server.Use(requestObservabilityMiddleware)
	server.Use(hertzgzip.Gzip(gzip.BestSpeed))
	server.Use(errorEnvelopeMiddleware)
	server.PATCH("/image-presets/:id", func(_ context.Context, c *app.RequestContext) {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "图像方案至少需要一个启用且非空的注入规则"})
	})

	response := ut.PerformRequest(server.Engine, http.MethodPatch, "/image-presets/demo", nil,
		ut.Header{Key: "Accept-Encoding", Value: "gzip"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if encoding := response.Header().Get("Content-Encoding"); encoding != "gzip" {
		t.Fatalf("expected a compressed response, got Content-Encoding=%q", encoding)
	}
	body := gunzipBody(t, response.Body.Bytes())
	if !strings.Contains(string(body), "request_id") || !strings.Contains(string(body), "图像方案至少需要一个启用且非空的注入规则") {
		t.Fatalf("compressed response lost its diagnostics: %s", body)
	}
	if !strings.Contains(logs.String(), "error=") || !strings.Contains(logs.String(), "图像方案至少需要一个启用且非空的注入规则") {
		t.Fatalf("completion log lost the cause: %s", logs.String())
	}
}

func gunzipBody(t *testing.T, data []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

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
