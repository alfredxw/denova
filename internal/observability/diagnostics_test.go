package observability

import (
	"strings"
	"testing"
)

func TestErrorDiagnosticsPreserveCauseWithoutSharingCredentials(t *testing.T) {
	payload := map[string]any{"error": "Save failed", "code": "save_failed", "details": map[string]any{
		"detail": `write /Users/alice/Novel/chapter.md: permission denied; api_key=secret-value; https://user:password@example.test/api?token=secret-query`,
	}}
	EnrichError(payload, "POST /api/files")
	details := payload["details"].(map[string]any)
	got := details["detail"].(string)
	for _, secret := range []string{"alice", "secret-value", "password", "secret-query"} {
		if strings.Contains(got, secret) {
			t.Fatalf("diagnostic leaked %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "permission denied") || details["operation"] != "POST /api/files" || details["backend_version"] == "" || details["platform"] == "" {
		t.Fatalf("missing diagnostic context: %#v", payload)
	}
}

func TestDiagnosticTextBoundsAndRedactsPayloads(t *testing.T) {
	for _, value := range []string{`Authorization: Bearer private-token`, `{"Authorization":"Bearer private-token"}`, `{"prompt":"private chapter", "api_key":"secret"}`, `body: private chapter`, `open C:\Users\alice\Novel\chapter.md: access denied`} {
		got := DiagnosticText(value)
		for _, secret := range []string{"private-token", "private chapter", "secret", "alice"} {
			if strings.Contains(got, secret) {
				t.Fatalf("diagnostic leaked %q: %s", secret, got)
			}
		}
	}
	if got := DiagnosticText(strings.Repeat("文", 5000)); len([]rune(got)) > 2050 {
		t.Fatalf("unbounded diagnostic: %d", len([]rune(got)))
	}
}

func TestDiagnosticTextKeepsCauseWhenMethodNameLooksLikePayloadKey(t *testing.T) {
	// Provider failures are reported as "<runtime> <method>: <cause>". A method
	// name sharing a payload key must keep the cause that follows it.
	for _, value := range []string{
		"Grok session/prompt: Grok ACP error -32000: model overloaded",
		"Codex some/messages: request rejected",
		"stream closed in Somebody: capability missing",
	} {
		got := DiagnosticText(value)
		if !strings.Contains(got, "ACP error -32000: model overloaded") &&
			!strings.Contains(got, "request rejected") && !strings.Contains(got, "capability missing") {
			t.Fatalf("cause was dropped: %s", got)
		}
	}
	for _, value := range []string{`prompt: private chapter`, `{"prompt":"private chapter"}`, `ok; body=private chapter`} {
		if got := DiagnosticText(value); strings.Contains(got, "private chapter") {
			t.Fatalf("payload leaked: %s", got)
		}
	}
}
