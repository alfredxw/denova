package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"denova/internal/observability"
)

// requestObservabilityMiddleware gives every HTTP request a server-owned ID,
// propagates it through context, and emits one completion record. Keeping the
// ID server-owned avoids collisions and untrusted values in logs.
func requestObservabilityMiddleware(ctx context.Context, c *app.RequestContext) {
	startedAt := time.Now()
	requestID, generationErr := observability.NewRequestID()
	ctx = observability.WithRequestID(ctx, requestID)
	c.Header(observability.RequestIDHeader, requestID)

	if generationErr != nil {
		slog.ErrorContext(ctx, "request_id_generation_degraded", "error", generationErr)
	}

	c.Next(ctx)

	status := c.Response.StatusCode()
	errorMessage, errorCode, errorDetail := enrichJSONErrorResponse(c, requestID, status)
	route := c.FullPath()
	if strings.TrimSpace(route) == "" {
		route = string(c.Request.Path())
	}
	level := slog.LevelDebug
	if status >= 500 {
		level = slog.LevelError
	} else if status >= 400 {
		level = slog.LevelWarn
	}
	attrs := []slog.Attr{
		slog.String("method", string(c.Request.Method())),
		slog.String("route", route),
		slog.Int("status", status),
		slog.Duration("duration", time.Since(startedAt)),
	}
	if errorCode != "" {
		attrs = append(attrs, slog.String("error_code", errorCode))
	}
	if errorMessage != "" {
		attrs = append(attrs, slog.String("error", errorMessage))
	}
	if errorDetail != "" {
		attrs = append(attrs, slog.String("error_detail", errorDetail))
	}
	slog.LogAttrs(ctx, level, "http_request_completed", attrs...)
}

// enrichJSONErrorResponse guarantees the correlation contract at the HTTP
// boundary, including errors produced by authentication middleware and new
// handlers that do not use the shared response helpers yet.
func enrichJSONErrorResponse(c *app.RequestContext, requestID string, status int) (message, code, detail string) {
	contentType := strings.ToLower(string(c.Response.Header.ContentType()))
	if status < 400 || c.Response.IsBodyStream() || (!strings.Contains(contentType, "application/json") && !strings.Contains(contentType, "+json")) {
		return "", "", ""
	}
	body := c.Response.Body()
	if len(body) == 0 {
		return "", "", ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return "", "", ""
	}
	payload[observability.RequestIDField] = requestID
	if code, _ := payload["code"].(string); code == "" {
		payload["code"] = fmt.Sprintf("http.%d", status)
	}
	// The registered route identifies the operation without exposing resource
	// names, query strings, or absolute paths supplied by the user.
	observability.EnrichError(payload, string(c.Method())+" "+c.FullPath())
	updated, err := json.Marshal(payload)
	if err != nil {
		return "", "", ""
	}
	c.Response.SetBody(updated)
	message, _ = payload["error"].(string)
	code, _ = payload["code"].(string)
	details, _ := payload["details"].(map[string]any)
	detail, _ = details["detail"].(string)
	return strings.TrimSpace(message), strings.TrimSpace(code), detail
}
