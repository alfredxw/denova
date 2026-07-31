package providers

import (
	"reflect"
	"testing"
)

func TestContinuationRoundTripAndIdentityIsolation(t *testing.T) {
	config := ModelConfig{
		Provider: ProviderOpenAI, Protocol: ProtocolOpenAIResponses,
		Model: "gpt-5", BaseURL: "https://api.openai.com/v1",
	}
	want := []any{map[string]any{"type": "reasoning", "encrypted_content": "encrypted"}}
	continuation, err := NewContinuation(config, want)
	if err != nil {
		t.Fatal(err)
	}
	extra := map[string]any{
		"openai-request-id":  "request-id-is-telemetry",
		ExtraKeyContinuation: continuation,
	}
	selected := ContinuationExtra(extra)
	if len(selected) != 1 {
		t.Fatalf("selected continuation extra = %#v", selected)
	}

	var got []any
	matched, err := DecodeContinuation(selected, config, &got)
	if err != nil || !matched || !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded continuation = %#v matched=%t err=%v", got, matched, err)
	}

	differentModel := config
	differentModel.Model = "gpt-5-mini"
	matched, err = DecodeContinuation(selected, differentModel, &got)
	if err != nil || matched {
		t.Fatalf("cross-model continuation matched=%t err=%v", matched, err)
	}

	// A future continuation schema owned by another routing identity must not
	// block an ordinary message fallback after the user switches models.
	continuation.Version++
	selected[ExtraKeyContinuation] = continuation
	matched, err = DecodeContinuation(selected, differentModel, &got)
	if err != nil || matched {
		t.Fatalf("cross-model future continuation matched=%t err=%v", matched, err)
	}
}

func TestContinuationRejectsMalformedMatchingPayload(t *testing.T) {
	config := ModelConfig{Provider: ProviderOpenAI, Protocol: ProtocolOpenAIResponses, Model: "gpt-5"}
	extra := map[string]any{ExtraKeyContinuation: map[string]any{
		"version":  continuationVersion,
		"provider": string(config.Provider),
		"protocol": string(config.Protocol),
		"model":    config.Model,
		"payload":  "not-an-output-list",
	}}
	var target []any
	matched, err := DecodeContinuation(extra, config, &target)
	if !matched || err == nil {
		t.Fatalf("malformed continuation matched=%t err=%v", matched, err)
	}
}
