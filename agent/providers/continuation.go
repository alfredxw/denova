package providers

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtraKeyContinuation is the only message Extra entry reserved for opaque,
// model-visible protocol continuation data. Product persistence may retain
// this entry while discarding response telemetry and UI reasoning fields.
const ExtraKeyContinuation = "provider-continuation"

const continuationVersion = 1

// Continuation contains SDK-free JSON needed to reproduce a stateless request.
// The full routing identity prevents provider-local item IDs or signatures from
// being replayed after the user switches provider, protocol, model, or endpoint.
type Continuation struct {
	Version  int             `json:"version"`
	Provider ProviderID      `json:"provider"`
	Protocol ProtocolID      `json:"protocol"`
	Model    string          `json:"model"`
	BaseURL  string          `json:"base_url,omitempty"`
	Payload  json.RawMessage `json:"payload"`
}

// NewContinuation validates and serializes protocol-owned continuation data.
func NewContinuation(config ModelConfig, payload any) (Continuation, error) {
	if config.Provider == "" || config.Protocol == "" || strings.TrimSpace(config.Model) == "" {
		return Continuation{}, fmt.Errorf("encode provider continuation: provider, protocol, and model are required")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return Continuation{}, fmt.Errorf("encode provider continuation: %w", err)
	}
	if len(data) == 0 || !json.Valid(data) {
		return Continuation{}, fmt.Errorf("encode provider continuation: invalid JSON payload")
	}
	return Continuation{
		Version:  continuationVersion,
		Provider: config.Provider,
		Protocol: config.Protocol,
		Model:    strings.TrimSpace(config.Model),
		BaseURL:  strings.TrimSpace(config.BaseURL),
		Payload:  append(json.RawMessage(nil), data...),
	}, nil
}

// DecodeContinuation decodes matching continuation data into target. A valid
// continuation for a different model identity is ignored rather than sent to
// the wrong endpoint. Malformed matching state fails loudly because silently
// dropping it can corrupt a tool/reasoning sequence.
func DecodeContinuation(extra map[string]any, config ModelConfig, target any) (bool, error) {
	if extra == nil {
		return false, nil
	}
	stored, ok := extra[ExtraKeyContinuation]
	if !ok {
		return false, nil
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return true, fmt.Errorf("encode stored provider continuation: %w", err)
	}
	var continuation Continuation
	if err := json.Unmarshal(data, &continuation); err != nil {
		return true, fmt.Errorf("decode stored provider continuation: %w", err)
	}
	if continuation.Provider != config.Provider ||
		continuation.Protocol != config.Protocol ||
		strings.TrimSpace(continuation.Model) != strings.TrimSpace(config.Model) ||
		strings.TrimSpace(continuation.BaseURL) != strings.TrimSpace(config.BaseURL) {
		return false, nil
	}
	if continuation.Version != continuationVersion {
		return true, fmt.Errorf("decode stored provider continuation: unsupported version %d", continuation.Version)
	}
	if target == nil {
		return true, fmt.Errorf("decode stored provider continuation: target is required")
	}
	if err := json.Unmarshal(continuation.Payload, target); err != nil {
		return true, fmt.Errorf("decode stored provider continuation payload: %w", err)
	}
	return true, nil
}

// ContinuationExtra selects the one protocol-continuation entry that may cross
// a product persistence boundary. Call it on an already cloned Message when
// independent ownership is required.
func ContinuationExtra(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	value, ok := extra[ExtraKeyContinuation]
	if !ok {
		return nil
	}
	return map[string]any{ExtraKeyContinuation: value}
}
