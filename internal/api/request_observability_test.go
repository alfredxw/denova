package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/google/uuid"

	"denova/internal/observability"
)

func TestRequestObservabilityCorrelatesContextResponseAndLog(t *testing.T) {
	previousLogger := slog.Default()
	defer slog.SetDefault(previousLogger)
	var logOutput bytes.Buffer
	observability.ConfigureStructuredLogging(&logOutput)

	var contextRequestID string
	server := hertzserver.Default()
	server.Use(requestObservabilityMiddleware)
	server.GET("/failure", func(ctx context.Context, c *app.RequestContext) {
		contextRequestID = observability.RequestID(ctx)
		c.JSON(http.StatusTeapot, map[string]any{
			"error": "boom",
			"code":  "test_failure",
		})
	})
	response := ut.PerformRequest(
		server.Engine,
		http.MethodGet,
		"/failure",
		nil,
		ut.Header{Key: observability.RequestIDHeader, Value: "00000000-0000-7000-8000-000000000000"},
	)
	responseRequestID := string(response.Result().Header.Peek(observability.RequestIDHeader))
	if responseRequestID == "" || responseRequestID != contextRequestID {
		t.Fatalf("response request ID = %q, context request ID = %q", responseRequestID, contextRequestID)
	}
	if responseRequestID == "00000000-0000-7000-8000-000000000000" {
		t.Fatal("server trusted the caller-provided request ID")
	}
	parsed, err := uuid.Parse(responseRequestID)
	if err != nil || parsed.Version() != 7 {
		t.Fatalf("response request ID = %q, want UUIDv7: %v", responseRequestID, err)
	}

	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if got := payload[observability.RequestIDField]; got != responseRequestID {
		t.Fatalf("error payload request_id = %#v, want %q", got, responseRequestID)
	}
	entry := logOutput.String()
	for _, expected := range []string{
		"msg=http_request_completed",
		"status=418",
		"error_code=test_failure",
		"error=boom",
		"request_id=" + responseRequestID,
	} {
		if !strings.Contains(entry, expected) {
			t.Fatalf("log does not contain %q: %s", expected, entry)
		}
	}
}

func TestRequestObservabilityKeepsSuccessfulPayloadUnchanged(t *testing.T) {
	server := hertzserver.Default()
	server.Use(requestObservabilityMiddleware)
	server.GET("/ok", func(_ context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]bool{"ok": true})
	})
	response := ut.PerformRequest(server.Engine, http.MethodGet, "/ok", nil)
	if requestID := string(response.Result().Header.Peek(observability.RequestIDHeader)); requestID == "" {
		t.Fatal("successful response is missing X-Request-ID")
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload[observability.RequestIDField]; exists {
		t.Fatalf("successful payload was unexpectedly changed: %#v", payload)
	}
}

func TestCORSExposesRequestIDHeader(t *testing.T) {
	server := hertzserver.Default()
	server.Use(requestObservabilityMiddleware)
	server.Use(corsMiddleware)
	response := ut.PerformRequest(
		server.Engine,
		http.MethodOptions,
		"/api/example",
		nil,
		ut.Header{Key: "Origin", Value: "http://localhost:5173"},
	)
	result := response.Result()
	if got := string(result.Header.Peek("Access-Control-Expose-Headers")); got != observability.RequestIDHeader {
		t.Fatalf("Access-Control-Expose-Headers = %q", got)
	}
	if got := string(result.Header.Peek(observability.RequestIDHeader)); got == "" {
		t.Fatal("CORS response is missing X-Request-ID")
	}
}
