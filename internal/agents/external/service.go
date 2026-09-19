package external

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/conversationjournal"
	externaljournal "denova/internal/agents/external/journal"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	"denova/internal/agents/toolruntime"
	"denova/internal/book"
	agent "github.com/alfredxw/denova/agent"
)

// Service owns live operations and question wakeups only. Acceptance, command
// idempotency, outcomes and recovery all use the product's existing Session.
// A single application instance shares this service across foreground entries.
type Service struct {
	Interactions Interactions
	mu           sync.Mutex
	active       map[string]*Operation
}

// StartRequest is already prepared and admitted by the product entry. The
// runtime and exact source cursor must match the canonical Session. Preparation
// does not authorize side effects; Start atomically commits the user input.
type StartRequest struct {
	ProjectID             string
	AttachmentRoot        string
	Session               *session.Session
	CommandID             string
	Fingerprint           string
	Revision              uint64
	PreparedCursor        conversationjournal.Cursor
	ContinuesOperationID  string
	Adapter               Adapter
	Input                 Input
	Message               agent.Message
	Metadata              session.MessageMetadata
	Definitions           []agent.ToolDefinition
	ToolPolicy            toolruntime.OrchestratorConfig
	ReviewThreadID        string
	Emit                  func(agentrun.Event)
	InputCommitEffect     agentrun.InputCommitEffect
	BookService           *book.Service
	OnMutationsVerified   func(context.Context, []agenttool.Mutation, agenttool.Verification)
	Checkpoint            *externaljournal.Checkpoint
	ProviderInputMaxBytes int
	Locale                string
	PriorMutations        []agenttool.Mutation
}

func (service *Service) Start(ctx context.Context, request StartRequest) (*Operation, error) {
	if request.Session == nil || request.Adapter == nil || strings.TrimSpace(request.ProjectID) == "" || request.Revision == 0 || request.Fingerprint == "" {
		return nil, errors.New("external operation requires a prepared Project Session and adapter")
	}
	if err := agentrun.ValidateCommandID(request.CommandID); err != nil {
		return nil, err
	}
	if request.Message.Role != agent.User || request.Metadata.MessageID == "" {
		return nil, errors.New("external operation requires its canonical user message")
	}
	definitions, wire, err := prepareTools(ctx, request.Definitions)
	if err != nil {
		return nil, err
	}
	request.Input.Tools = wire
	operation := &Operation{service: service, request: request, definitions: definitions, id: "external-" + rand.Text(), mutations: append([]agenttool.Mutation(nil), request.PriorMutations...)}
	service.mu.Lock()
	defer service.mu.Unlock()
	// This short admission lock covers commit and registration, never model or
	// tool execution. Duplicate commands receive the same live handle.
	err = request.Session.UpdateExternal(ctx, request.Revision, func(state session.ExternalState) (session.ExternalTransaction, error) {
		operation.incarnation = state.Incarnation
		for _, existing := range state.Projection.Operations {
			if existing.CommandID != request.CommandID {
				continue
			}
			if existing.Fingerprint != request.Fingerprint {
				return session.ExternalTransaction{}, agentrun.ErrInvalidCommand
			}
			operation.id, operation.replayed = existing.ID, true
			return session.ExternalTransaction{}, nil
		}
		if request.PreparedCursor != state.Cursor {
			return session.ExternalTransaction{}, session.ErrContextRevisionConflict
		}
		if !reflect.DeepEqual(state.Config.Engine(), request.Input.Selection) {
			return session.ExternalTransaction{}, conversationconfig.ErrRevisionConflict
		}
		record, err := externaljournal.NewRecord(externaljournal.OperationAccepted, operation.id, request.Revision, externaljournal.Accepted{
			CommandID: request.CommandID, Fingerprint: request.Fingerprint, Runtime: request.Input.Selection,
			InputMessageID: request.Metadata.MessageID, ContinuesOperationID: request.ContinuesOperationID,
		})
		return session.ExternalTransaction{Records: []externaljournal.Record{record}, Message: &request.Message, Metadata: request.Metadata}, err
	})
	if err != nil {
		return nil, fmt.Errorf("accept external operation: %w", err)
	}
	key := operation.key()
	if active := service.active[key]; active != nil {
		return active, nil
	}
	if err := request.Session.ReadExternal(ctx, func(state session.ExternalState) error {
		accepted := state.Projection.Operations[operation.id]
		if accepted == nil {
			return errors.New("external acceptance disappeared")
		}
		operation.receipt = agentrun.CommandReceipt{CommandID: agentrun.CommandID(request.CommandID), OperationID: agentrun.OperationID(operation.id), Cursor: agentrun.Cursor(accepted.Accepted.Cursor)}
		return nil
	}); err != nil {
		return nil, err
	}
	if !operation.replayed {
		if service.active == nil {
			service.active = map[string]*Operation{}
		}
		service.active[key] = operation
	}
	return operation, nil
}

func (operation *Operation) key() string {
	return operation.request.ProjectID + "\x00" + operation.incarnation + "\x00" + operation.id
}

// Recover closes abandoned attempts without replaying any tool. Pending Ask
// and unknown effects stay attached to the interrupted operation. Live handles
// are excluded, so a page refresh cannot interrupt an active model.
func (service *Service) Recover(ctx context.Context, projectID string, sess *session.Session) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	var revision uint64
	var needsRecovery bool
	if err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
		revision = state.Config.Revision
		for _, operation := range state.Projection.Operations {
			key := projectID + "\x00" + state.Incarnation + "\x00" + operation.ID
			if operation.Status == externaljournal.Running && service.active[key] == nil {
				needsRecovery = true
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if !needsRecovery {
		return nil
	}
	return sess.UpdateExternal(ctx, revision, func(state session.ExternalState) (session.ExternalTransaction, error) {
		var records []externaljournal.Record
		for _, operation := range state.Projection.Operations {
			key := projectID + "\x00" + state.Incarnation + "\x00" + operation.ID
			if operation.Status != externaljournal.Running || service.active[key] != nil {
				continue
			}
			record, err := externaljournal.NewRecord(externaljournal.OperationClosed, operation.ID, operation.ConfigRevision, externaljournal.Closed{Status: externaljournal.Interrupted, ErrorCode: "agentRuntime.interrupted"})
			if err != nil {
				return session.ExternalTransaction{}, err
			}
			records = append(records, record)
		}
		return session.ExternalTransaction{Records: records}, nil
	})
}
