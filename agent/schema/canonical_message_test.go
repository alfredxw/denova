package schema

import (
	"testing"
)

func TestCanonicalMessageStripsProviderTelemetry(t *testing.T) {
	message := &Message{
		Role:    Assistant,
		Content: "narrative",
		Extra: map[string]any{
			"provider":              "openai",
			"protocol":              "openai-responses",
			"response_id":           "resp-1",
			"model":                 "gemini-3.8-flash-high",
			"created":               int64(1700000000),
			"response_status":       "completed",
			"openai-request-id":     "req-1",
			"provider-continuation": map[string]any{"payload": []any{map[string]any{"encrypted_content": "token"}}},
		},
	}
	canonical := CanonicalMessage(message)
	if canonical == message {
		t.Fatal("CanonicalMessage must not mutate or alias its input")
	}
	if _, ok := canonical.Extra["provider-continuation"]; !ok {
		t.Fatalf("expected provider-continuation to be retained, got %v", canonical.Extra)
	}
	for _, key := range []string{"provider", "protocol", "response_id", "model", "created", "response_status", "openai-request-id"} {
		if _, ok := canonical.Extra[key]; ok {
			t.Fatalf("expected telemetry key %q to be stripped, got %v", key, canonical.Extra)
		}
	}
}

func TestHashCanonicalMessagesIgnoresProviderTelemetry(t *testing.T) {
	base := []*Message{
		{
			Role:    User,
			Content: "turn input",
		},
		{
			Role:    Assistant,
			Content: "narrative",
			Extra: map[string]any{
				"provider-continuation": map[string]any{"payload": []any{map[string]any{"encrypted_content": "token"}}},
			},
		},
	}
	withTelemetry := []*Message{
		base[0].Clone(),
		func() *Message {
			m := base[1].Clone()
			m.Extra["provider"] = "openai"
			m.Extra["protocol"] = "openai-responses"
			m.Extra["response_id"] = "resp-1"
			m.Extra["model"] = "gemini-3.8-flash-high"
			m.Extra["created"] = int64(1700000000)
			m.Extra["response_status"] = "completed"
			m.Extra["openai-request-id"] = "req-1"
			return m
		}(),
	}

	plain, err := HashCanonicalMessages(base)
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := HashCanonicalMessages(withTelemetry)
	if err != nil {
		t.Fatal(err)
	}
	if plain != telemetry {
		t.Fatalf("hash should be invariant to provider telemetry:\nplain=%s\ntelemetry=%s", plain, telemetry)
	}
}
