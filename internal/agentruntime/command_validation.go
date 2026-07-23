package agentruntime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var fallbackIDSequence atomic.Uint64

// validateCommandEnvelope runs before fingerprinting and durable lookup so an
// untrusted transport cannot make either path allocate around unbounded IDs or
// persist an unbounded Abort reason.
func validateCommandEnvelope(command Command, limits InputLimits) error {
	if command == nil {
		return ErrInvalidCommand
	}
	limits = limits.normalized()
	if err := ValidateCommandID(string(command.commandID()), limits); err != nil {
		return err
	}
	var operationID OperationID
	switch typed := command.(type) {
	case Steer:
		operationID = typed.OperationID
	case FollowUp:
		operationID = typed.OperationID
	case NextTurn:
		operationID = typed.AfterOperationID
	case Abort:
		operationID = typed.OperationID
		if len(typed.Reason) > limits.MaxAbortReasonBytes {
			return fmt.Errorf("%w: abort reason exceeds %d bytes", ErrInvalidCommand, limits.MaxAbortReasonBytes)
		}
	}
	if len(operationID) > limits.MaxOperationIDBytes {
		return fmt.Errorf("%w: operation id exceeds %d bytes", ErrInvalidCommand, limits.MaxOperationIDBytes)
	}
	return nil
}

// ValidateCommandID applies the same byte bound used by Harness admission.
// Callers should invoke it before fingerprints, hashes, or registry keys are
// derived from untrusted transport input. Passing a zero InputLimits value uses
// the production defaults returned by DefaultInputLimits.
func ValidateCommandID(commandID string, limits InputLimits) error {
	limits = limits.normalized()
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return fmt.Errorf("%w: command id is required for durable retry", ErrInvalidCommand)
	}
	if len(commandID) > limits.MaxCommandIDBytes {
		return fmt.Errorf("%w: command id exceeds %d bytes", ErrInvalidCommand, limits.MaxCommandIDBytes)
	}
	return nil
}

func validateUserInput(input UserInput, limits InputLimits) error {
	limits = limits.normalized()
	if strings.TrimSpace(input.Text) == "" {
		return ErrInvalidCommand
	}
	if len(input.Text) > limits.MaxTextBytes {
		return fmt.Errorf("%w: input text exceeds %d bytes", ErrInvalidCommand, limits.MaxTextBytes)
	}
	if len(input.TurnSpecRef) > limits.MaxTurnSpecRefBytes {
		return fmt.Errorf("%w: turn spec reference exceeds %d bytes", ErrInvalidCommand, limits.MaxTurnSpecRefBytes)
	}
	if len(input.RestoreDescriptor) > limits.MaxRestoreDescriptorBytes {
		return fmt.Errorf("%w: restore descriptor exceeds %d bytes", ErrInvalidCommand, limits.MaxRestoreDescriptorBytes)
	}
	if len(input.RestoreDescriptor) > 0 && !json.Valid(input.RestoreDescriptor) {
		return fmt.Errorf("%w: restore descriptor must be valid JSON", ErrInvalidCommand)
	}
	if len(input.ContextRefs) > limits.MaxContextRefs {
		return fmt.Errorf("%w: context reference count exceeds %d", ErrInvalidCommand, limits.MaxContextRefs)
	}
	var declaredBytes int64
	for _, ref := range input.ContextRefs {
		if strings.TrimSpace(ref.Source) == "" || strings.TrimSpace(ref.Resource) == "" || ref.ByteLimit <= 0 {
			return fmt.Errorf("%w: every context reference requires source, resource, and a positive byte limit", ErrInvalidCommand)
		}
		if len(ref.Source) > limits.MaxContextRefFieldBytes || len(ref.Resource) > limits.MaxContextRefFieldBytes ||
			len(ref.Selector) > limits.MaxContextRefFieldBytes || len(ref.Revision) > limits.MaxContextRefFieldBytes {
			return fmt.Errorf("%w: context reference field exceeds %d bytes", ErrInvalidCommand, limits.MaxContextRefFieldBytes)
		}
		if ref.ByteLimit > limits.MaxContextRefBytes {
			return fmt.Errorf("%w: context reference byte limit exceeds %d", ErrInvalidCommand, limits.MaxContextRefBytes)
		}
		declaredBytes += int64(ref.ByteLimit)
		if declaredBytes > limits.MaxDeclaredContextBytes {
			return fmt.Errorf("%w: declared context exceeds %d bytes", ErrInvalidCommand, limits.MaxDeclaredContextBytes)
		}
	}
	return nil
}

func validateContextCompactionRef(ref ContextCompactionRef, limits InputLimits, requireCompactionID bool) error {
	limits = limits.normalized()
	fields := []struct {
		name  string
		value string
	}{
		{name: "spec_ref", value: ref.SpecRef},
		{name: "source", value: ref.Source},
		{name: "purpose", value: ref.Purpose},
		{name: "resource", value: ref.Resource},
		{name: "expected_revision", value: ref.ExpectedRevision},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%w: compaction %s is required", ErrInvalidCommand, field.name)
		}
		limit := limits.MaxContextRefFieldBytes
		if field.name == "spec_ref" {
			limit = limits.MaxTurnSpecRefBytes
		}
		if len(field.value) > limit {
			return fmt.Errorf("%w: compaction %s exceeds %d bytes", ErrInvalidCommand, field.name, limit)
		}
	}
	if requireCompactionID && strings.TrimSpace(ref.CompactionID) == "" {
		return fmt.Errorf("%w: compaction_id is required for removal", ErrInvalidCommand)
	}
	if len(ref.CompactionID) > limits.MaxContextRefFieldBytes {
		return fmt.Errorf("%w: compaction_id exceeds %d bytes", ErrInvalidCommand, limits.MaxContextRefFieldBytes)
	}
	if len(ref.RestoreDescriptor) > limits.MaxRestoreDescriptorBytes {
		return fmt.Errorf("%w: compaction restore descriptor exceeds %d bytes", ErrInvalidCommand, limits.MaxRestoreDescriptorBytes)
	}
	if len(ref.RestoreDescriptor) > 0 && !json.Valid(ref.RestoreDescriptor) {
		return fmt.Errorf("%w: compaction restore descriptor must be valid JSON", ErrInvalidCommand)
	}
	return nil
}

func newUserMessage(operationID OperationID, input UserInput) Message {
	cloned := cloneUserInput(input)
	return Message{
		ID: newID("message"), Role: RoleUser, Content: cloned.Text,
		Input: cloned, Operation: operationID,
	}
}

// CommandFingerprint returns the canonical durable identity hash used by
// Harness admission and command-index replay. Recovery fixtures and adapters
// that seed an accepted journal record must use this helper rather than
// duplicating the versioned JSON envelope.
func CommandFingerprint(command Command) (string, error) {
	var envelope any
	switch command := command.(type) {
	case StartTurn:
		envelope = struct {
			Kind  string    `json:"kind"`
			ID    CommandID `json:"id"`
			Input UserInput `json:"input"`
		}{Kind: "start_turn", ID: command.ID, Input: command.Input}
	case Steer:
		envelope = struct {
			Kind        string      `json:"kind"`
			ID          CommandID   `json:"id"`
			OperationID OperationID `json:"operation_id"`
			Input       UserInput   `json:"input"`
		}{Kind: "steer", ID: command.ID, OperationID: command.OperationID, Input: command.Input}
	case FollowUp:
		envelope = struct {
			Kind        string      `json:"kind"`
			ID          CommandID   `json:"id"`
			OperationID OperationID `json:"operation_id"`
			Input       UserInput   `json:"input"`
		}{Kind: "follow_up", ID: command.ID, OperationID: command.OperationID, Input: command.Input}
	case NextTurn:
		envelope = struct {
			Kind             string      `json:"kind"`
			ID               CommandID   `json:"id"`
			AfterOperationID OperationID `json:"after_operation_id"`
			Input            UserInput   `json:"input"`
		}{Kind: "next_turn", ID: command.ID, AfterOperationID: command.AfterOperationID, Input: command.Input}
	case Abort:
		envelope = struct {
			Kind        string      `json:"kind"`
			ID          CommandID   `json:"id"`
			OperationID OperationID `json:"operation_id"`
			Reason      string      `json:"reason"`
		}{Kind: "abort", ID: command.ID, OperationID: command.OperationID, Reason: command.Reason}
	case CompactIfNeeded:
		envelope = struct {
			Kind string               `json:"kind"`
			ID   CommandID            `json:"id"`
			Ref  ContextCompactionRef `json:"ref"`
		}{Kind: "compact_context", ID: command.ID, Ref: command.Ref}
	case RemoveCompaction:
		envelope = struct {
			Kind string               `json:"kind"`
			ID   CommandID            `json:"id"`
			Ref  ContextCompactionRef `json:"ref"`
		}{Kind: "remove_compaction", ID: command.ID, Ref: command.Ref}
	default:
		return "", ErrInvalidCommand
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("%w: encode command fingerprint: %v", ErrInvalidCommand, err)
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("agent-runtime-command.v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	sum := hash.Sum(nil)
	return hex.EncodeToString(sum), nil
}

func fingerprintCommand(command Command) string {
	fingerprint, _ := CommandFingerprint(command)
	return fingerprint
}

func newID(prefix string) string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(random[:])
	}
	// Losing the OS random source must not tear down the single-writer actor.
	// The fallback combines process, wall-clock, and monotonic identities so it
	// remains collision-resistant across both goroutines and process restarts.
	return fmt.Sprintf(
		"%s-fallback-%x-%x-%x",
		prefix,
		time.Now().UTC().UnixNano(),
		os.Getpid(),
		fallbackIDSequence.Add(1),
	)
}
