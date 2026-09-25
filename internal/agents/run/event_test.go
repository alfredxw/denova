package agentrun

import (
	"reflect"
	"testing"
)

func TestErrorDiagnosticsRetainOriginAcrossReplay(t *testing.T) {
	source := map[string]any{"message": "Run failed", "details": map[string]any{"detail": "persist approval: disk full"}}
	first := (Event{Type: "error", Data: source}).WithErrorDiagnostics("request-original", "task-original")
	replayed := first.WithErrorDiagnostics("request-reconnect", "task-reconnect")
	if !reflect.DeepEqual(first, replayed) {
		t.Fatalf("replay changed originating diagnostics: %#v != %#v", first, replayed)
	}
	payload := replayed.Data.(map[string]any)
	details := payload["details"].(map[string]any)
	if payload["request_id"] != "request-original" || payload["task_id"] != "task-original" || details["detail"] != "persist approval: disk full" || details["platform"] == "" {
		t.Fatalf("incomplete error diagnostics: %#v", payload)
	}
	if source["request_id"] != nil || source["details"].(map[string]any)["platform"] != nil {
		t.Fatalf("diagnostic projection mutated the source: %#v", source)
	}
}
