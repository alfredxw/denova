package agents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
)

const (
	contextCheckpointLabelBytes = 256
	contextReceiptLimit         = 64
	contextReceiptOverflowTool  = "context_receipt_overflow"
)

type contextWindowConversation interface {
	ActiveContextCheckpoints(string) ([]session.ContextOperation, error)
	FreezeContextWindowBoundary([]*agent.Message) (*session.ContextCheckpointBoundary, error)
	StageContextOperation(session.ContextOperation) error
}

type contextWindowCheckpoint struct {
	operation     session.ContextOperation
	receiptOffset int
}

type pendingContextRewind struct {
	checkpoint session.ContextOperation
	receiptsAt int
	report     string
}

// runContextWindowController keeps exploratory transcript rewrites local to a
// run and stages only their structural metadata. Canonical/display messages
// remain append-only, and workspace/config/external side effects are never
// rolled back.
type runContextWindowController struct {
	mu             sync.Mutex
	conversation   contextWindowConversation
	agentKind      string
	baseline       []*agent.Message
	boundary       *session.ContextCheckpointBoundary
	boundaryErr    error
	checkpoints    []contextWindowCheckpoint
	receipts       []session.ContextMutationReceipt
	pending        *pendingContextRewind
	pendingErr     error
	reminded       bool
	appliedRewrite *agent.ContextWindowRewrite
}

func newRunContextWindowController(conversation Conversation, agentKind string) agent.ContextWindowController {
	backend, ok := conversation.(contextWindowConversation)
	if !ok {
		return nil
	}
	agentKind = strings.TrimSpace(agentKind)
	if agentKind != AgentKindIDE && agentKind != AgentKindConfigManager {
		return nil
	}
	controller := &runContextWindowController{conversation: backend, agentKind: agentKind}
	operations, err := backend.ActiveContextCheckpoints(agentKind)
	if err != nil {
		controller.pendingErr = fmt.Errorf("restore context checkpoints: %w", err)
		return controller
	}
	for _, operation := range operations {
		receiptOffset := len(controller.receipts)
		controller.receipts = append(controller.receipts, operation.MutationReceipts...)
		controller.checkpoints = append(controller.checkpoints, contextWindowCheckpoint{
			operation: operation, receiptOffset: receiptOffset,
		})
	}
	return controller
}

func (controller *runContextWindowController) BeforeModel(_ context.Context, messages []*agent.Message) ([]*agent.Message, error) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.pendingErr != nil {
		return nil, controller.pendingErr
	}
	if controller.pending == nil {
		controller.baseline = cloneContextMessages(messages)
		controller.boundary, controller.boundaryErr = controller.freezeBoundary(messages)
		return messages, nil
	}

	pending := controller.pending
	checkpointBoundary, err := session.CloneContextCheckpointBoundary(pending.checkpoint.Boundary)
	if err != nil {
		return nil, fmt.Errorf("restore context checkpoint %q: %w", pending.checkpoint.CheckpointID, err)
	}
	operation := session.ContextOperation{
		Kind:         session.ContextOperationRewind,
		AgentKind:    controller.agentKind,
		CheckpointID: pending.checkpoint.CheckpointID,
		Purpose:      pending.checkpoint.Purpose,
		MessageCount: pending.checkpoint.MessageCount,
		Boundary:     checkpointBoundary,
		Report:       pending.report,
	}
	if pending.receiptsAt < len(controller.receipts) {
		operation.MutationReceipts = append([]session.ContextMutationReceipt(nil), controller.receipts[pending.receiptsAt:]...)
	}
	if err := controller.conversation.StageContextOperation(operation); err != nil {
		return nil, fmt.Errorf("stage context rewind: %w", err)
	}

	rewritten := cloneContextMessages(checkpointBoundary.EffectivePrefix)
	rewritten = append(rewritten, newContextRewindSummaryMessage(operation))
	rewrittenCanonical := cloneContextMessages(checkpointBoundary.CanonicalPrefix)
	rewrittenCanonical = append(rewrittenCanonical, newContextRewindSummaryMessage(operation))
	controller.boundary, controller.boundaryErr = session.NewContextCheckpointBoundary(
		checkpointBoundary.Cursor, rewritten, rewrittenCanonical, checkpointBoundary.LimitBytes,
	)
	if controller.boundaryErr != nil {
		return nil, fmt.Errorf("freeze rewound context: %w", controller.boundaryErr)
	}
	controller.pending = nil
	controller.receipts = nil
	controller.baseline = cloneContextMessages(rewritten)
	controller.appliedRewrite = &agent.ContextWindowRewrite{Kind: session.ContextOperationRewind, CheckpointID: operation.CheckpointID}
	return rewritten, nil
}

func (controller *runContextWindowController) freezeBoundary(messages []*agent.Message) (*session.ContextCheckpointBoundary, error) {
	if controller.boundary != nil && contextMessagesHavePrefix(messages, controller.boundary.EffectivePrefix) {
		suffix := cloneContextMessages(messages[len(controller.boundary.EffectivePrefix):])
		canonical := append(cloneContextMessages(controller.boundary.CanonicalPrefix), suffix...)
		return session.NewContextCheckpointBoundary(
			controller.boundary.Cursor, messages, canonical, controller.boundary.LimitBytes,
		)
	}
	return controller.conversation.FreezeContextWindowBoundary(messages)
}

// TakeContextWindowRewrite consumes the ordered rewrite marker after
// BeforeModel has successfully staged and applied it.
func (controller *runContextWindowController) TakeContextWindowRewrite() (agent.ContextWindowRewrite, bool) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.appliedRewrite == nil {
		return agent.ContextWindowRewrite{}, false
	}
	rewrite := *controller.appliedRewrite
	controller.appliedRewrite = nil
	return rewrite, true
}

func (controller *runContextWindowController) BeforeComplete(_ context.Context, messages []*agent.Message) ([]*agent.Message, bool, error) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.pendingErr != nil {
		return nil, false, controller.pendingErr
	}
	if len(controller.checkpoints) == 0 {
		return messages, false, nil
	}
	checkpoint := controller.checkpoints[len(controller.checkpoints)-1].operation
	if controller.reminded {
		return nil, false, fmt.Errorf("active checkpoint %q was not rewound", checkpoint.CheckpointID)
	}
	controller.reminded = true
	reminder := agent.UserMessage(fmt.Sprintf(
		"[denova-context-checkpoint] checkpoint=%s is still active. Call rewind now with a concise, factual report before yielding. Context rewind never rolls back files, configuration, browser actions, or other side effects.",
		checkpoint.CheckpointID,
	))
	return append(cloneContextMessages(messages), reminder), true, nil
}

func (controller *runContextWindowController) Checkpoint(_ context.Context, request agent.ContextCheckpointRequest) (agent.ContextCheckpointResult, error) {
	id, err := newContextCheckpointID()
	if err != nil {
		return agent.ContextCheckpointResult{}, err
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if len(controller.baseline) == 0 {
		return agent.ContextCheckpointResult{}, fmt.Errorf("checkpoint requires an active model context")
	}
	if controller.boundaryErr != nil {
		return agent.ContextCheckpointResult{}, fmt.Errorf("freeze context checkpoint: %w", controller.boundaryErr)
	}
	boundary, err := session.CloneContextCheckpointBoundary(controller.boundary)
	if err != nil {
		return agent.ContextCheckpointResult{}, fmt.Errorf("freeze context checkpoint: %w", err)
	}
	if len(controller.checkpoints) > 0 {
		return agent.ContextCheckpointResult{}, fmt.Errorf("checkpoint %q is still active; rewind it before creating another", controller.checkpoints[len(controller.checkpoints)-1].operation.CheckpointID)
	}
	purpose := truncateContextUTF8(strings.TrimSpace(request.Purpose), contextCheckpointLabelBytes)
	operation := session.ContextOperation{
		Kind:         session.ContextOperationCheckpoint,
		AgentKind:    controller.agentKind,
		CheckpointID: id,
		Purpose:      purpose,
		MessageCount: boundary.Cursor.MessageCount,
		Boundary:     boundary,
	}
	if err := controller.conversation.StageContextOperation(operation); err != nil {
		return agent.ContextCheckpointResult{}, fmt.Errorf("stage context checkpoint: %w", err)
	}
	controller.checkpoints = append(controller.checkpoints, contextWindowCheckpoint{
		operation: operation, receiptOffset: len(controller.receipts),
	})
	return agent.ContextCheckpointResult{ID: id, Purpose: purpose, Staged: true}, nil
}

func (controller *runContextWindowController) Rewind(_ context.Context, request agent.ContextRewindRequest) (agent.ContextRewindResult, error) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.pending != nil {
		return agent.ContextRewindResult{}, fmt.Errorf("a context rewind is already pending")
	}
	checkpointIndex := controller.checkpointIndex(strings.TrimSpace(request.CheckpointID))
	if checkpointIndex < 0 {
		return agent.ContextRewindResult{}, fmt.Errorf("context checkpoint %q is not active", strings.TrimSpace(request.CheckpointID))
	}
	checkpoint := controller.checkpoints[checkpointIndex]
	boundary, err := session.CloneContextCheckpointBoundary(checkpoint.operation.Boundary)
	if err != nil {
		return agent.ContextRewindResult{}, fmt.Errorf("context checkpoint %q has no recoverable projection: %w", checkpoint.operation.CheckpointID, err)
	}
	dropped := len(controller.baseline) - len(boundary.EffectivePrefix)
	if dropped < 0 {
		dropped = 0
	}
	controller.pending = &pendingContextRewind{
		checkpoint: checkpoint.operation,
		receiptsAt: checkpoint.receiptOffset,
		report:     strings.TrimSpace(request.Report),
	}
	controller.reminded = false
	controller.checkpoints = controller.checkpoints[:checkpointIndex]
	return agent.ContextRewindResult{
		CheckpointID: checkpoint.operation.CheckpointID,
		Dropped:      dropped,
		Staged:       true,
	}, nil
}

func (controller *runContextWindowController) ObserveTool(_ context.Context, observation agent.ContextToolObservation) {
	if observation.Descriptor.MutationScope == agent.ToolMutationNone ||
		observation.Result.Status == agent.ToolResultBlocked ||
		observation.Result.Status == agent.ToolResultSkipped ||
		observation.Name == "checkpoint" || observation.Name == "rewind" {
		return
	}
	status := observation.Result.Status
	if status == "" {
		status = agent.ToolResultSuccess
	}
	summary := "status=" + string(status)
	if observation.Result.SyntheticReason != "" {
		summary += "; synthetic_reason=" + string(observation.Result.SyntheticReason)
	}
	controller.mu.Lock()
	if len(controller.checkpoints) == 0 {
		controller.mu.Unlock()
		return
	}
	receipt := session.ContextMutationReceipt{
		Tool: strings.TrimSpace(observation.Name), CallID: strings.TrimSpace(observation.CallID),
		Scope: string(observation.Descriptor.MutationScope), Summary: summary,
	}
	controller.appendMutationReceipt(receipt)
	index := len(controller.checkpoints) - 1
	checkpoint := &controller.checkpoints[index]
	checkpoint.operation.MutationReceipts = append(
		[]session.ContextMutationReceipt(nil), controller.receipts[checkpoint.receiptOffset:]...,
	)
	if err := controller.conversation.StageContextOperation(checkpoint.operation); err != nil && controller.pendingErr == nil {
		controller.pendingErr = fmt.Errorf("stage context checkpoint receipts: %w", err)
	}
	controller.mu.Unlock()
}

func (controller *runContextWindowController) appendMutationReceipt(receipt session.ContextMutationReceipt) {
	if len(controller.receipts) < contextReceiptLimit-1 {
		controller.receipts = append(controller.receipts, receipt)
		return
	}
	overflow := session.ContextMutationReceipt{
		Tool: contextReceiptOverflowTool, Scope: "multiple",
		Summary: "Additional mutation receipts were omitted at the context boundary; inspect current state before continuing.",
	}
	if len(controller.receipts) == contextReceiptLimit-1 {
		controller.receipts = append(controller.receipts, overflow)
		return
	}
	controller.receipts[contextReceiptLimit-1] = overflow
}

func (controller *runContextWindowController) checkpointIndex(id string) int {
	if len(controller.checkpoints) == 0 {
		return -1
	}
	if id == "" {
		return len(controller.checkpoints) - 1
	}
	for index := len(controller.checkpoints) - 1; index >= 0; index-- {
		if controller.checkpoints[index].operation.CheckpointID == id {
			return index
		}
	}
	return -1
}

func newContextCheckpointID() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create context checkpoint id: %w", err)
	}
	return "ctx-" + hex.EncodeToString(value[:]), nil
}

func cloneContextMessages(messages []*agent.Message) []*agent.Message {
	if messages == nil {
		return nil
	}
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = agent.CloneMessage(message)
	}
	return result
}

func contextMessagesHavePrefix(messages, prefix []*agent.Message) bool {
	return len(messages) >= len(prefix) && contextMessagesEqual(messages[:len(prefix)], prefix)
}

func truncateContextUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	data := []byte(value[:limit])
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data)
}
