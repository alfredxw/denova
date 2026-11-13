package schema

// providerTelemetryExtraKeys lists message Extra entries that describe how a
// response was produced (provider identity, request/response identifiers,
// timing, status) rather than what the model observed. Canonical history
// hashing strips them so a live in-memory transcript and its persisted,
// reconstructed canonical window agree on the same semantic bytes.
//
// The model-visible continuation entry (providers.ExtraKeyContinuation) is
// intentionally NOT listed here: it is the one opaque protocol entry that
// carries replay semantics and must remain part of the canonical fingerprint.
var providerTelemetryExtraKeys = map[string]struct{}{
	"provider":            {},
	"protocol":            {},
	"response_id":         {},
	"model":               {},
	"created":             {},
	"response_status":     {},
	"openai-request-id":   {},
	"google-request-id":   {},
	"service_tier":        {},
	"system_fingerprint":  {},
}

// CanonicalMessage returns a copy of message with provider telemetry stripped
// from Extra, leaving only model-visible entries such as the provider
// continuation used for stateless replay. It never mutates message.
func CanonicalMessage(message *Message) *Message {
	if message == nil {
		return nil
	}
	if len(message.Extra) == 0 {
		return message.Clone()
	}
	clone := message.Clone()
	stripped := make(map[string]any, len(clone.Extra))
	for key, value := range clone.Extra {
		if _, telemetry := providerTelemetryExtraKeys[key]; telemetry {
			continue
		}
		stripped[key] = value
	}
	clone.Extra = stripped
	return clone
}

// HashCanonicalMessages hashes the model-visible message list after stripping
// provider telemetry from every message, so a live transcript and its
// reconstructed canonical window produce the same fingerprint.
func HashCanonicalMessages(messages []*Message) (string, error) {
	normalized := make([]*Message, len(messages))
	for index, message := range messages {
		normalized[index] = CanonicalMessage(message)
	}
	return HashCanonical(normalized)
}
