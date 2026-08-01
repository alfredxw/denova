package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestContextHandlerAddsRequestIDFromContext(t *testing.T) {
	var output bytes.Buffer
	handler := contextHandler{Handler: slog.NewJSONHandler(&output, nil)}
	ctx := WithRequestID(context.Background(), "request-123")
	record := slog.NewRecord(time.Unix(0, 0), slog.LevelInfo, "test_message", 0)
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatal(err)
	}

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if got := entry[RequestIDField]; got != "request-123" {
		t.Fatalf("request_id = %#v, want request-123", got)
	}
}

func TestContextHandlerLeavesBackgroundLogsUncorrelated(t *testing.T) {
	var output bytes.Buffer
	handler := contextHandler{Handler: slog.NewJSONHandler(&output, nil)}
	record := slog.NewRecord(time.Unix(0, 0), slog.LevelInfo, "background_message", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if _, exists := entry[RequestIDField]; exists {
		t.Fatalf("background log unexpectedly has request_id: %#v", entry)
	}
}

func TestNewRequestIDReturnsUUIDv7(t *testing.T) {
	seen := make(map[string]struct{}, 128)
	for range 128 {
		requestID, err := NewRequestID()
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := uuid.Parse(requestID)
		if err != nil {
			t.Fatalf("parse request ID %q: %v", requestID, err)
		}
		if parsed.Version() != 7 {
			t.Fatalf("request ID %q version = %d, want 7", requestID, parsed.Version())
		}
		if _, exists := seen[requestID]; exists {
			t.Fatalf("duplicate request ID %q", requestID)
		}
		seen[requestID] = struct{}{}
	}
}
