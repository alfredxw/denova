package engine

import (
	"encoding/json"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func CanonicalMessageCheckpoint(encoded json.RawMessage) (PersistedMessageCheckpoint, error) {
	state, err := decodeEngineTranscript(encoded)
	if err != nil {
		return PersistedMessageCheckpoint{}, err
	}
	return canonicalMessageCheckpoint(state)
}

func canonicalMessageCheckpoint(state engineTranscript) (PersistedMessageCheckpoint, error) {
	committed := len(state.Messages)
	for index := len(state.Messages) - 1; index >= 0; index-- {
		message := state.Messages[index]
		if message.Role == agentschema.Assistant && len(message.ToolCalls) > 0 {
			if len(state.Messages)-index-1 < len(message.ToolCalls) {
				committed = index
			}
			break
		}
		if message.Role != agentschema.ToolRole {
			break
		}
	}
	hash, err := agentschema.HashCanonical(state.Messages[:committed])
	if err != nil {
		return PersistedMessageCheckpoint{}, err
	}
	pending := agentschema.CloneMessages(state.Messages[committed:])
	state.Messages = nil
	metadata, err := json.Marshal(state)
	if err != nil {
		return PersistedMessageCheckpoint{}, err
	}
	return PersistedMessageCheckpoint{Hash: hash, MessageCount: state.Archive.Raw(committed), Archive: state.Archive, Metadata: metadata, Pending: pending}, nil
}
