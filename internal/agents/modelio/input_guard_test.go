package modelio

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
	"denova/internal/agents/attachment"
)

func TestNativeImageAdmissionSeparatesVisualTokensFromEncodedBytes(t *testing.T) {
	var encoded bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 768, 768))); err != nil {
		t.Fatal(err)
	}
	files, err := attachment.Materialize(t.TempDir(), attachment.SessionScope("images"), "input", []attachment.Upload{{
		Name: "reference.png", MediaType: "image/png", DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()),
	}})
	if err != nil {
		t.Fatal(err)
	}
	model := providers.ModelConfig{Provider: providers.ProviderAnthropic, Model: "claude-sonnet-4-6"}
	messages := []*agent.Message{agent.UserMessageWithAttachments("Inspect this reference.", files)}
	for _, kind := range []string{config.AgentKindIDE, config.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			if err := ValidateInput(kind, model, messages, nil, 4<<20, 400000); err != nil {
				t.Fatalf("valid image was rejected before provider I/O: %v", err)
			}
			size, err := model.InputEstimator().Estimate(messages, nil)
			if err != nil || size.Tokens-agent.EstimateRequestTextTokens(messages, nil) != 784 {
				t.Fatalf("visual budget = %+v, error %v", size, err)
			}
			// The native parts alone exceed 4 MiB, but still fit the visual window.
			twoImages := append(append([]*agent.Message(nil), messages...), messages[0])
			if err := ValidateInput(kind, model, twoImages, nil, 4<<20, 400000); err != nil {
				t.Fatalf("two valid native images were rejected as context bytes: %v", err)
			}
			var limit *ProviderInputLimitError
			err = ValidateInput(kind, model, messages, nil, 4<<20, 500)
			if !errors.As(err, &limit) || limit.Tokens <= limit.MaxTokens || limit.Bytes >= limit.MaxBytes {
				t.Fatalf("actual visual token pressure did not enforce the context limit: %v", err)
			}
		})
	}
}

func TestProviderHardLimitRejectsLongHistoryWhenSemanticCompactionIsDisabled(t *testing.T) {
	disabled := false
	maxBytes := 32 * 1024
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		CompactionEnabled: &disabled, MaxProviderInputBytes: &maxBytes,
	}}}
	resolved := config.ResolveAgentContext(cfg, config.AgentKindIDE)
	if resolved.CompactionEnabled {
		t.Fatal("test requires user-controlled semantic compaction to be disabled")
	}
	messages := []*agent.Message{agent.UserMessage(strings.Repeat("历史正文。", maxBytes))}
	err := ValidateInput(config.AgentKindIDE, providers.ModelConfig{}, messages, nil, resolved.MaxProviderInputBytes, config.ResolveAgentModel(cfg, config.AgentKindIDE).ContextWindowTokens)
	var limitErr *ProviderInputLimitError
	if !errors.As(err, &limitErr) || limitErr.Bytes <= limitErr.MaxBytes {
		t.Fatalf("complete provider input was not rejected by the non-disableable hard limit: %v", err)
	}
}

func TestStandaloneProviderBoundaryUsesResolvedAgentLimit(t *testing.T) {
	maxBytes := 32 * 1024
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		MaxProviderInputBytes: &maxBytes,
	}}}
	messages := []*agent.Message{
		agent.SystemMessage("bounded standalone agent"),
		agent.UserMessage(strings.Repeat("语义触发证据。", maxBytes)),
	}
	err := ValidateConfiguredInput(cfg, config.AgentKindIDE, messages, nil)
	var limitErr *ProviderInputLimitError
	if !errors.As(err, &limitErr) || limitErr.AgentKind != config.AgentKindIDE || limitErr.MaxBytes != maxBytes {
		t.Fatalf("standalone provider boundary = %#v, err=%v", limitErr, err)
	}
}
