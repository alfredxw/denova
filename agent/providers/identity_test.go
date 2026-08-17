package providers

import (
	"strings"
	"testing"
)

func TestModelIdentityExcludesCredentials(t *testing.T) {
	first, err := ModelIdentity(ModelConfig{
		Provider: ProviderOpenAI, Protocol: ProtocolOpenAIResponses, Model: "gpt-test",
		BaseURL: "https://alice:secret@example.com/v1?token=first#private",
		APIKey:  "first-key",
		Headers: map[string]string{"Authorization": "Bearer first", "X-API-Key": "first", "OpenAI-Beta": "responses=v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ModelIdentity(ModelConfig{
		Provider: ProviderOpenAI, Protocol: ProtocolOpenAIResponses, Model: "gpt-test",
		BaseURL: "https://bob:different@example.com/v1?token=second#other",
		APIKey:  "second-key",
		Headers: map[string]string{"Authorization": "Bearer second", "X-API-Key": "second", "OpenAI-Beta": "responses=v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("credentials changed capability identity: first=%+v second=%+v", first, second)
	}
	for _, secret := range []string{"secret", "first-key", "Bearer first"} {
		if strings.Contains(first.ConfigHash, secret) {
			t.Fatalf("identity exposed credential %q", secret)
		}
	}
}

func TestModelIdentityChangesWithRequestBehavior(t *testing.T) {
	base := ModelConfig{
		Provider: ProviderOpenAI, Protocol: ProtocolOpenAIResponses, Model: "gpt-test",
		Headers: map[string]string{"OpenAI-Beta": "responses=v1"},
	}
	first, err := ModelIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.Headers = map[string]string{"OpenAI-Beta": "responses=v2"}
	second, err := ModelIdentity(changed)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("behavior change did not change model capability identity")
	}
}
