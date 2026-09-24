// Package externaljournal defines the external execution lane embedded in a
// Product Session JSONL. Engine rollouts and runtime memory are disposable.
package externaljournal

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"denova/config"
	agent "github.com/alfredxw/denova/agent"
)

const RecordType = "external_runtime"
const Version = 1

type Kind string

const (
	OperationAccepted Kind = "operation.accepted"
	GuidanceDelivered Kind = "guidance.delivered"
	ToolStarted       Kind = "tool.started"
	ToolFinished      Kind = "tool.finished"
	OperationClosed   Kind = "operation.closed"
	ContextCheckpoint Kind = "context.checkpoint"
)

type Status string

const (
	Running     Status = "running"
	Completed   Status = "completed"
	Failed      Status = "failed"
	Cancelled   Status = "cancelled"
	Interrupted Status = "interrupted"
)

type Recovery string

const (
	ReadOnly          Recovery = "read_only"
	ReceiptVerifiable Recovery = "receipt_verifiable"
	NonReplayable     Recovery = "non_replayable"
)

type Record struct {
	Type           string          `json:"type"`
	Version        int             `json:"version"`
	Kind           Kind            `json:"kind"`
	OperationID    string          `json:"operation_id"`
	ConfigRevision uint64          `json:"config_revision"`
	CreatedAt      time.Time       `json:"created_at"`
	Data           json.RawMessage `json:"data"`
}

type Accepted struct {
	CommandID            string                  `json:"command_id"`
	InputCommandID       string                  `json:"input_command_id,omitempty"`
	GuidanceCount        int                     `json:"guidance_count,omitempty"`
	Fingerprint          string                  `json:"fingerprint"`
	Runtime              config.RuntimeSelection `json:"runtime"`
	InputMessageID       string                  `json:"input_message_id"`
	ContinuesOperationID string                  `json:"continues_operation_id,omitempty"`
}

type StartedTool struct {
	ExecutionID string          `json:"execution_id"`
	AgentKind   string          `json:"agent_kind,omitempty"`
	Tool        string          `json:"tool"`
	Arguments   json.RawMessage `json:"arguments"`
	Recovery    Recovery        `json:"recovery"`
}

// DeliveredGuidance records the accepted input prefix in the same transaction
// as its canonical user message. Provider caches are not delivery receipts.
type DeliveredGuidance struct {
	CommandID string `json:"command_id"`
	MessageID string `json:"message_id"`
	Count     int    `json:"count"`
}

type FinishedTool struct {
	ExecutionID string       `json:"execution_id"`
	Success     bool         `json:"success"`
	Result      string       `json:"result"`
	Receipt     *ToolReceipt `json:"receipt,omitempty"`
}

// ToolReceipt preserves domain effects and portable artifacts independently
// of model text. Execution, recovery and history share this journal contract.
type ToolReceipt struct {
	Details     json.RawMessage         `json:"details,omitempty"`
	Effects     []agent.Effect          `json:"effects,omitempty"`
	Artifacts   []agent.ToolArtifactRef `json:"artifacts,omitempty"`
	Attachments []agent.Attachment      `json:"attachments,omitempty"`
}

type Closed struct {
	Status    Status            `json:"status"`
	MessageID string            `json:"message_id,omitempty"`
	ErrorCode string            `json:"error_code,omitempty"`
	AgentKind string            `json:"agent_kind,omitempty"`
	Usage     *agent.TokenUsage `json:"usage,omitempty"`
}

// Checkpoint summarizes an exact canonical source interval. The source hash
// makes a checkpoint invalid after a restore or a different transcript prefix.
type Checkpoint struct {
	// Version identifies summary semantics. Legacy records remain readable but
	// cannot cover source that the old text-only summarizer never inspected.
	Version        uint16           `json:"version,omitempty"`
	Summary        string           `json:"summary"`
	SourceStart    uint64           `json:"source_start"`
	SourceEnd      uint64           `json:"source_end"`
	SourceHash     string           `json:"source_hash"`
	SourceBoundary string           `json:"source_boundary,omitempty"`
	Runtime        config.RuntimeID `json:"runtime"`
	EngineVersion  string           `json:"engine_version"`
}

func NewRecord(kind Kind, operationID string, revision uint64, data any) (Record, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return Record{}, err
	}
	record := Record{Type: RecordType, Version: Version, Kind: kind, OperationID: operationID, ConfigRevision: revision, CreatedAt: time.Now().UTC(), Data: body}
	return record, record.Validate()
}

func (record Record) Validate() error {
	if record.Type != RecordType || record.Version != Version || record.OperationID == "" || record.ConfigRevision == 0 || !json.Valid(record.Data) {
		return errors.New("invalid external runtime record envelope")
	}
	switch record.Kind {
	case OperationAccepted:
		var data Accepted
		if err := json.Unmarshal(record.Data, &data); err != nil {
			return err
		}
		if data.CommandID == "" || data.Fingerprint == "" || data.InputMessageID == "" || (data.Runtime.Kind != config.RuntimeCodex && data.Runtime.Kind != config.RuntimeClaude) {
			return errors.New("external acceptance requires command, fingerprint, input and external runtime")
		}
		return data.Runtime.Validate(config.AgentKindGeneral)
	case GuidanceDelivered:
		var data DeliveredGuidance
		if err := json.Unmarshal(record.Data, &data); err != nil {
			return err
		}
		if data.CommandID == "" || data.MessageID == "" || data.Count < 1 {
			return errors.New("external guidance requires command, message and accepted prefix")
		}
	case ToolStarted:
		var data StartedTool
		if err := json.Unmarshal(record.Data, &data); err != nil {
			return err
		}
		if data.ExecutionID == "" || data.Tool == "" || !json.Valid(data.Arguments) {
			return errors.New("invalid external tool start")
		}
		if data.Tool == "ask" {
			if _, err := QuestionRequest(data.ExecutionID, data.Arguments); err != nil {
				return err
			}
		}
		switch data.Recovery {
		case ReadOnly, ReceiptVerifiable, NonReplayable:
			return nil
		default:
			return fmt.Errorf("invalid external tool recovery class %q", data.Recovery)
		}
	case ToolFinished:
		var data FinishedTool
		if err := json.Unmarshal(record.Data, &data); err != nil {
			return err
		}
		if data.ExecutionID == "" {
			return errors.New("external tool result requires execution ID")
		}
	case OperationClosed:
		var data Closed
		if err := json.Unmarshal(record.Data, &data); err != nil {
			return err
		}
		switch data.Status {
		case Completed:
			if data.MessageID == "" {
				return errors.New("external completion requires a canonical message")
			}
		case Failed, Cancelled, Interrupted:
		default:
			return fmt.Errorf("invalid external terminal status %q", data.Status)
		}
	case ContextCheckpoint:
		var data Checkpoint
		if err := json.Unmarshal(record.Data, &data); err != nil {
			return err
		}
		if data.Summary == "" || data.SourceStart == 0 || data.SourceEnd < data.SourceStart || data.SourceHash == "" || data.Runtime == "" || data.EngineVersion == "" {
			return errors.New("invalid external context checkpoint")
		}
	default:
		return fmt.Errorf("unknown external runtime record kind %q", record.Kind)
	}
	return nil
}
