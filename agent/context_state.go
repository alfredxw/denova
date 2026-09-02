package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	contextStateMessageExtraKey  = "agent.context_state"
	contextStateMessageVersion   = "v1"
	contextStateOperationExtra   = "agent.context_state.operation"
	contextStateIDExtra          = "agent.context_state.id"
	contextStateSourceExtra      = "agent.context_state.source"
	contextStatePurposeExtra     = "agent.context_state.purpose"
	contextStateResourceExtra    = "agent.context_state.resource"
	contextStateRevisionExtra    = "agent.context_state.revision"
	contextStateFingerprintExtra = "agent.context_state.fingerprint"
)

type contextStateSnapshot struct {
	Generation uint64                         `json:"generation,omitempty"`
	Sections   map[string]contextStateSection `json:"sections,omitempty"`
}

type contextStateSection struct {
	StateID      string `json:"state_id"`
	Source       string `json:"source"`
	Purpose      string `json:"purpose"`
	Resource     string `json:"resource"`
	Revision     string `json:"revision,omitempty"`
	Fingerprint  string `json:"fingerprint"`
	MessageIndex int    `json:"message_index"`
	Removed      bool   `json:"removed,omitempty"`
}

// IsContextStateMessage reports whether a message is an Agent-owned durable
// Context State update. Product transcript projections and turn-boundary
// selectors must ignore these messages even though their provider-safe wire
// role is User.
func IsContextStateMessage(message *Message) bool {
	if message == nil || message.Extra == nil {
		return false
	}
	version, _ := message.Extra[contextStateMessageExtraKey].(string)
	return version == contextStateMessageVersion
}

func cloneContextStateSnapshot(state contextStateSnapshot) contextStateSnapshot {
	clone := contextStateSnapshot{Generation: state.Generation}
	if len(state.Sections) == 0 {
		return clone
	}
	clone.Sections = make(map[string]contextStateSection, len(state.Sections))
	for id, section := range state.Sections {
		clone.Sections[id] = section
	}
	return clone
}

// advanceContextState computes the append-only state delta for one exact raw
// transcript. It also restores active sections whose latest update falls
// inside the current Compaction replacement range. Returned messages must be
// appended in order before the turn's raw user message.
func advanceContextState(
	raw []*Message,
	fragments []ContextFragment,
	current contextStateSnapshot,
	compaction CompactionState,
	compactionPresent bool,
) ([]*Message, contextStateSnapshot, error) {
	next := cloneContextStateSnapshot(current)
	if next.Sections == nil {
		next.Sections = make(map[string]contextStateSection)
	}
	active := make(map[string]ContextFragment)
	ordered := make([]ContextFragment, 0)
	for _, fragment := range fragments {
		if fragment.Placement != ContextStateMessage {
			continue
		}
		active[fragment.StateID] = fragment
		ordered = append(ordered, fragment)
	}

	appended := make([]*Message, 0)
	appendUpsert := func(fragment ContextFragment, fingerprint, operation, previousRevision string) {
		index := len(raw) + len(appended)
		appended = append(appended, newContextStateMessage(fragment, fingerprint, operation, previousRevision))
		next.Sections[fragment.StateID] = contextStateSection{
			StateID: fragment.StateID, Source: fragment.Source, Purpose: fragment.Purpose,
			Resource: fragment.Resource, Revision: fragment.Revision,
			Fingerprint: fingerprint, MessageIndex: index,
		}
	}

	for _, fragment := range ordered {
		fingerprint, err := contextStateFragmentFingerprint(fragment)
		if err != nil {
			return nil, contextStateSnapshot{}, err
		}
		section, exists := next.Sections[fragment.StateID]
		switch {
		case !exists || section.Removed:
			appendUpsert(fragment, fingerprint, "initialize", "")
		case section.Fingerprint != fingerprint:
			appendUpsert(fragment, fingerprint, "update", section.Revision)
		}
	}

	removed := make([]string, 0)
	for id, section := range next.Sections {
		if _, exists := active[id]; !exists && !section.Removed {
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)
	for _, id := range removed {
		section := next.Sections[id]
		section.MessageIndex = len(raw) + len(appended)
		section.Removed = true
		appended = append(appended, newContextStateRemovalMessage(section))
		next.Sections[id] = section
	}

	if compactionPresent && !compaction.Removed {
		stateIDs := make([]string, 0, len(next.Sections))
		for id := range next.Sections {
			stateIDs = append(stateIDs, id)
		}
		sort.Strings(stateIDs)
		for _, id := range stateIDs {
			section := next.Sections[id]
			if section.MessageIndex < compaction.ReplacementFrom || section.MessageIndex >= compaction.ReplacementTo {
				continue
			}
			if section.Removed {
				section.MessageIndex = len(raw) + len(appended)
				appended = append(appended, newContextStateRemovalMessage(section))
				next.Sections[id] = section
				continue
			}
			fragment, exists := active[id]
			if !exists {
				return nil, contextStateSnapshot{}, fmt.Errorf("active Context State section %q has no materialized fragment", id)
			}
			appendUpsert(fragment, section.Fingerprint, "restore", section.Revision)
		}
	}
	if len(appended) > 0 {
		next.Generation++
	}
	return appended, next, nil
}

func contextStateFragmentFingerprint(fragment ContextFragment) (string, error) {
	if fragment.Placement != ContextStateMessage || fragment.Stability != ContextSessionState {
		return "", errors.New("fingerprint Context State requires a session_state fragment")
	}
	return hashCanonical(struct {
		StateID, Source, Purpose, Resource, Revision string
		Rendering                                    ContextRendering
		Content                                      string
		HardLimit                                    int
	}{
		fragment.StateID, fragment.Source, fragment.Purpose, fragment.Resource, fragment.Revision,
		effectiveContextRendering(fragment.Rendering), fragment.Content, fragment.HardLimit,
	})
}

func newContextStateMessage(fragment ContextFragment, fingerprint, operation, previousRevision string) *Message {
	contentHash := sha256.Sum256([]byte(fragment.Content))
	metadata := []string{
		"# Context state update",
		"",
		"State ID: " + fragment.StateID,
		"Source: " + fragment.Source,
		"Purpose: " + fragment.Purpose,
		"Resource: " + fragment.Resource,
	}
	if revision := strings.TrimSpace(fragment.Revision); revision != "" {
		metadata = append(metadata, "Revision: "+revision)
	}
	if previousRevision = strings.TrimSpace(previousRevision); previousRevision != "" && previousRevision != strings.TrimSpace(fragment.Revision) {
		metadata = append(metadata, "Previous revision: "+previousRevision)
	}
	metadata = append(metadata,
		"Content SHA-256: "+hex.EncodeToString(contentHash[:]),
		"Operation: "+operation,
		"",
		fragment.Content,
	)
	message := UserMessage(strings.Join(metadata, "\n"))
	message.Extra = map[string]any{
		contextStateMessageExtraKey:  contextStateMessageVersion,
		contextStateOperationExtra:   "upsert",
		contextStateIDExtra:          fragment.StateID,
		contextStateSourceExtra:      fragment.Source,
		contextStatePurposeExtra:     fragment.Purpose,
		contextStateResourceExtra:    fragment.Resource,
		contextStateRevisionExtra:    fragment.Revision,
		contextStateFingerprintExtra: fingerprint,
	}
	return message
}

func newContextStateRemovalMessage(section contextStateSection) *Message {
	content := strings.Join([]string{
		"# Context state update",
		"",
		"State ID: " + section.StateID,
		"Source: " + section.Source,
		"Purpose: " + section.Purpose,
		"Resource: " + section.Resource,
		"Operation: remove",
		"",
		"This state section is no longer active. Do not rely on values from its earlier updates.",
	}, "\n")
	message := UserMessage(content)
	message.Extra = map[string]any{
		contextStateMessageExtraKey:  contextStateMessageVersion,
		contextStateOperationExtra:   "remove",
		contextStateIDExtra:          section.StateID,
		contextStateSourceExtra:      section.Source,
		contextStatePurposeExtra:     section.Purpose,
		contextStateResourceExtra:    section.Resource,
		contextStateRevisionExtra:    section.Revision,
		contextStateFingerprintExtra: section.Fingerprint,
	}
	return message
}

func rebuildContextStateSnapshot(messages []*Message) (contextStateSnapshot, error) {
	state := contextStateSnapshot{Sections: make(map[string]contextStateSection)}
	for index, message := range messages {
		if !IsContextStateMessage(message) {
			continue
		}
		id, _ := message.Extra[contextStateIDExtra].(string)
		operation, _ := message.Extra[contextStateOperationExtra].(string)
		source, _ := message.Extra[contextStateSourceExtra].(string)
		purpose, _ := message.Extra[contextStatePurposeExtra].(string)
		resource, _ := message.Extra[contextStateResourceExtra].(string)
		revision, _ := message.Extra[contextStateRevisionExtra].(string)
		fingerprint, _ := message.Extra[contextStateFingerprintExtra].(string)
		if strings.TrimSpace(id) == "" || strings.TrimSpace(source) == "" || strings.TrimSpace(purpose) == "" ||
			strings.TrimSpace(resource) == "" || len(fingerprint) != 64 || (operation != "upsert" && operation != "remove") {
			return contextStateSnapshot{}, fmt.Errorf("canonical Context State message %d is incomplete", index)
		}
		state.Generation++
		state.Sections[id] = contextStateSection{
			StateID: id, Source: source, Purpose: purpose, Resource: resource,
			Revision: revision, Fingerprint: fingerprint, MessageIndex: index,
			Removed: operation == "remove",
		}
	}
	if len(state.Sections) == 0 {
		state.Sections = nil
	}
	return state, validateContextStateSnapshot(state, messages)
}

func validateContextStateSnapshot(state contextStateSnapshot, messages []*Message) error {
	for id, section := range state.Sections {
		if strings.TrimSpace(id) == "" || section.StateID != id || strings.TrimSpace(section.Source) == "" ||
			strings.TrimSpace(section.Purpose) == "" || strings.TrimSpace(section.Resource) == "" ||
			len(section.Fingerprint) != 64 || section.MessageIndex < 0 || section.MessageIndex >= len(messages) {
			return fmt.Errorf("Agent transcript Context State section %q is invalid", id)
		}
		message := messages[section.MessageIndex]
		if message == nil {
			return fmt.Errorf("Agent transcript Context State section %q lost its latest update", id)
		}
		messageID, _ := message.Extra[contextStateIDExtra].(string)
		operation, _ := message.Extra[contextStateOperationExtra].(string)
		expectedOperation := "upsert"
		if section.Removed {
			expectedOperation = "remove"
		}
		if !IsContextStateMessage(message) || messageID != id || operation != expectedOperation {
			return fmt.Errorf("Agent transcript Context State section %q lost its latest update", id)
		}
	}
	return nil
}

func insertMessagesAt(messages []*Message, index int, inserted []*Message) []*Message {
	if len(inserted) == 0 {
		return messages
	}
	if index < 0 || index > len(messages) {
		return messages
	}
	result := make([]*Message, 0, len(messages)+len(inserted))
	result = append(result, cloneMessages(messages[:index])...)
	result = append(result, cloneMessages(inserted)...)
	result = append(result, cloneMessages(messages[index:])...)
	return result
}
