package agentruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/sessionjournal"
)

const controlReceiptPrefix = "denova.external.receipt.v1/"
const controlReceiptUpgrade = "external-control-receipts-v2"

type controlReceipt struct {
	Fingerprint string                  `json:"fingerprint"`
	Receipt     agentrun.CommandReceipt `json:"receipt"`
	Outcome     agentrun.OutcomeStatus  `json:"outcome,omitempty"`
	// AbortedInputCommandID binds a stop request to the input it discarded, so
	// a replay settles that input instead of whichever input is current later.
	AbortedInputCommandID string `json:"aborted_input_command_id,omitempty"`
}

// Only unfinished inputs belong to this snapshot. Historical receipts are
// separate capability records in the same canonical stream, indexed by command.
// receipts stages only this transaction's changes; readReceipt never scans history.
type externalControlState struct {
	Version     int                        `json:"version"`
	Revision    uint64                     `json:"revision"`
	OperationID agentrun.OperationID       `json:"operation_id"`
	CommandID   agentrun.CommandID         `json:"command_id"`
	Phase       agentrun.RunPhase          `json:"phase"`
	Current     *ExternalCycleInput        `json:"current,omitempty"`
	Queue       []ExternalCycleInput       `json:"queue,omitempty"`
	Last        *agentrun.OperationSummary `json:"last,omitempty"`
	receipts    map[string]controlReceipt
	readReceipt sessionjournal.CapabilityReader
}

func controlFingerprint(encoded []byte) string {
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func decodeControl(raw json.RawMessage, present bool) (externalControlState, error) {
	state := externalControlState{Version: 2, Phase: agentrun.RunPhaseIdle, receipts: map[string]controlReceipt{}}
	if present {
		if err := json.Unmarshal(raw, &state); err != nil {
			return state, err
		}
		if state.Version != 1 && state.Version != 2 {
			return state, errors.New("unsupported external control state version")
		}
	}
	return state, nil
}

func (state *externalControlState) receipt(commandID string) (controlReceipt, bool, error) {
	if saved, found := state.receipts[commandID]; found {
		return saved, true, nil
	}
	raw, found, err := state.readReceipt(controlReceiptPrefix + commandID)
	var saved controlReceipt
	if err == nil && found {
		err = json.Unmarshal(raw, &saved)
	}
	return saved, found, err
}

func (state *externalControlState) setOutcome(commandID string, outcome agentrun.OutcomeStatus) error {
	saved, found, err := state.receipt(commandID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("external control receipt %q is missing", commandID)
	}
	if saved.Outcome != outcome {
		saved.Outcome = outcome
		state.receipts[commandID] = saved
	}
	return nil
}

func (control *ExternalController) read(ctx context.Context) (externalControlState, error) {
	raw, present, err := control.store.Read(ctx, externalControlCapability)
	if err != nil {
		return externalControlState{}, err
	}
	state, err := decodeControl(raw, present)
	if err != nil {
		return state, err
	}
	if state.Version == 1 {
		// Migrate once under the canonical transaction fence. A read never
		// resumes a provider, changes accepted inputs, or expires retry receipts.
		if err := control.update(ctx, func(*externalControlState) error { return nil }); err != nil {
			return state, err
		}
		return control.read(ctx)
	}
	state.readReceipt = func(capability string) (json.RawMessage, bool, error) { return control.store.Read(ctx, capability) }
	return state, nil
}

func (control *ExternalController) update(ctx context.Context, mutate func(*externalControlState) error) error {
	return control.store.Transact(ctx, controlReceiptUpgrade, func(read sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
		return externalControlChanges(read, mutate)
	})
}

func externalControlChanges(read sessionjournal.CapabilityReader, mutate func(*externalControlState) error) (map[string]json.RawMessage, error) {
	raw, present, err := read(externalControlCapability)
	if err != nil {
		return nil, err
	}
	state, err := decodeControl(raw, present)
	if err != nil {
		return nil, err
	}
	changes := make(map[string]json.RawMessage)
	migrating := state.Version == 1
	if migrating {
		var released struct {
			Receipts map[string]controlReceipt `json:"receipts"`
		}
		if err := json.Unmarshal(raw, &released); err != nil {
			return nil, err
		}
		for id, saved := range released.Receipts {
			// v0.5.1 Submit stored exact JSON bytes; Start already stored a
			// semantic digest, and recovery receipts have no fingerprint.
			// Hash the original bytes, preserving the released retry equality.
			if strings.HasPrefix(saved.Fingerprint, "{") {
				saved.Fingerprint = controlFingerprint([]byte(saved.Fingerprint))
			}
			body, err := json.Marshal(saved)
			if err != nil {
				return nil, err
			}
			changes[controlReceiptPrefix+id] = body
		}
		state.Version = 2
	}
	state.readReceipt = func(capability string) (json.RawMessage, bool, error) {
		if value, found := changes[capability]; found {
			return value, true, nil
		}
		return read(capability)
	}
	before, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	if err := mutate(&state); err != nil {
		return nil, err
	}
	after, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	changed := !bytes.Equal(before, after) || len(state.receipts) != 0
	if changed {
		state.Revision++
	}
	if changed || migrating {
		changes[externalControlCapability], err = json.Marshal(state)
		if err != nil {
			return nil, err
		}
	}
	for id, saved := range state.receipts {
		changes[controlReceiptPrefix+id], err = json.Marshal(saved)
		if err != nil {
			return nil, err
		}
	}
	return changes, nil
}
