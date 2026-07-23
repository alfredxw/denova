package runtime

import (
	"crypto/sha256"
	"encoding/hex"
)

func describePayload(payload []byte) PayloadDescriptor {
	digest := sha256.Sum256(payload)
	return PayloadDescriptor{Bytes: len(payload), SHA256: hex.EncodeToString(digest[:])}
}

func normalizeToolCallState(call ToolCallState) ToolCallState {
	if call.ArgumentsDescriptor.SHA256 == "" && call.Arguments != nil {
		call.ArgumentsDescriptor = describePayload(call.Arguments)
	}
	call.Arguments = nil
	return call
}

func normalizeToolFinished(event ToolCallFinishedEvent) ToolCallFinishedEvent {
	if event.ResultDescriptor.SHA256 == "" && event.Result != "" {
		event.ResultDescriptor = describePayload([]byte(event.Result))
	}
	event.Result = ""
	if len(event.HostEffects) > 0 {
		effects := make([]HostEffect, len(event.HostEffects))
		for index, effect := range event.HostEffects {
			effects[index] = cloneHostEffect(effect)
		}
		event.HostEffects = effects
	}
	return event
}
