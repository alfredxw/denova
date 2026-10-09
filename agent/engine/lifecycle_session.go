package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

type inputEnvelope struct {
	Version     uint16                        `json:"version"`
	Context     []agentschema.ContextFragment `json:"context,omitempty"`
	Attachments []agentschema.Attachment      `json:"attachments,omitempty"`
	Goal        *agentschema.GoalMutation     `json:"goal,omitempty"`
	HostData    *agentschema.HostData         `json:"host_data,omitempty"`
}

type PersistedMessageCheckpoint struct {
	Archive      *agenthistory.HistoryArchive `json:"archive,omitempty"`
	Hash         string                       `json:"hash"`
	MessageCount int                          `json:"message_count"`
	// Metadata contains execution state and canonical coordinates, never committed
	// message bodies. Pending preserves a tool batch before its product commit.
	// Released checkpoints may still contain Messages; recovery reads the journal.
	Metadata json.RawMessage        `json:"metadata,omitempty"`
	Pending  []*agentschema.Message `json:"pending,omitempty"`
}

func EncodeInput(input agentschema.Input) (json.RawMessage, UserInput, error) {
	if strings.TrimSpace(input.Text) == "" && len(input.Attachments) == 0 {
		return nil, UserInput{}, errors.New("Agent Input requires Text or Attachments")
	}
	if input.HostData != nil {
		if strings.TrimSpace(input.HostData.Type) == "" || input.HostData.Version == 0 || !json.Valid(input.HostData.Data) {
			return nil, UserInput{}, errors.New("Agent Input HostData requires Type, Version, and valid JSON Data")
		}
	}
	if err := validateContextFragments(input.Context); err != nil {
		return nil, UserInput{}, err
	}
	envelope := inputEnvelope{
		Version: 1, Context: append([]agentschema.ContextFragment(nil), input.Context...),
		Attachments: agentschema.CloneAttachments(input.Attachments),
		Goal:        agentschema.CloneGoalMutation(input.Goal), HostData: agentschema.CloneHostData(input.HostData),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, UserInput{}, fmt.Errorf("encode Agent Input: %w", err)
	}
	references := make([]ContextRef, 0, len(input.Context))
	for _, fragment := range input.Context {
		references = append(references, ContextRef{
			Source: fragment.Source, Resource: fragment.Resource, Revision: fragment.Revision,
			Selector: string(fragment.Placement), ByteLimit: fragment.HardLimit,
		})
	}
	return encoded, UserInput{Text: input.Text, ContextRefs: references, Envelope: encoded}, nil
}

func DecodeInput(input UserInput) (agentschema.Input, error) {
	result := agentschema.Input{Text: input.Text}
	if len(input.Envelope) == 0 {
		return result, nil
	}
	var envelope inputEnvelope
	if err := json.Unmarshal(input.Envelope, &envelope); err != nil {
		return agentschema.Input{}, fmt.Errorf("decode Agent Input: %w", err)
	}
	if envelope.Version != 1 {
		return agentschema.Input{}, fmt.Errorf("unsupported Agent Input version %d", envelope.Version)
	}
	result.Context = append([]agentschema.ContextFragment(nil), envelope.Context...)
	result.Attachments = agentschema.CloneAttachments(envelope.Attachments)
	result.Goal = agentschema.CloneGoalMutation(envelope.Goal)
	result.HostData = agentschema.CloneHostData(envelope.HostData)
	return result, nil
}
