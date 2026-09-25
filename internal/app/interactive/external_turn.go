package interactiveapp

import (
	"context"
	"crypto/sha256"
	"denova/internal/observability"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"denova/config"
	"denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/i18n"
	"denova/internal/interactive"
	agent "github.com/alfredxw/denova/agent"
)

const externalGameOperationPrefix = "external-game-"

// ExternalTurnConfig is an application-owned Game execution boundary. Native
// Agent is a peer engine: no Native Definition, Session or model loop is used.
type ExternalTurnConfig struct {
	Conversation   *Conversation
	Request        agentchat.ChatRequest
	Config         config.Config
	Assembly       agents.ExternalAssembly
	BookService    *book.Service
	Adapter        external.Adapter
	Runtime        *external.Runtime
	Release        func()
	Emit           func(agentrun.Event)
	Plan           []agent.TodoItem
	ObservePlan    func(context.Context, agentrun.Event) error
	Guidance       []agentchat.ChatRequest
	PrepareHistory func(context.Context, external.HistoryPreparation) (external.Input, error)
}

// ExternalTurn persists only existing player input, draft, tool context and
// final Turn records. Its process object and external rollout are disposable.
type ExternalTurn struct {
	config          ExternalTurnConfig
	identity        agentrun.CycleIdentity
	once            sync.Once
	outcome         agentrun.Outcome
	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	closed          bool
	text, segment   string
	tools           map[string]agent.ToolDefinition
	calls           map[string]externalGameCall
	restored        []*agent.Message
	replayed        *interactive.TurnEvent
	observations    []external.Message
	toolError       error
	runtimeSession  *external.RuntimeSession
	sourceBoundary  string
	providerSettled bool
}

func StartExternalTurn(ctx context.Context, cfg ExternalTurnConfig) (*ExternalTurn, error) {
	c := cfg.Conversation
	if c == nil || (cfg.Adapter == nil && cfg.Runtime == nil) || cfg.Config.ActiveAgentRuntime == nil {
		return nil, errors.New("external Game turn requires a conversation and engine")
	}
	sum := sha256.Sum256([]byte(cfg.Request.CommandID))
	turn := &ExternalTurn{config: cfg, identity: agentrun.CycleIdentity{CommandID: agentrun.CommandID(cfg.Request.CommandID), OperationID: agentrun.OperationID(externalGameOperationPrefix + hex.EncodeToString(sum[:16])), Cycle: 1}, calls: map[string]externalGameCall{}}
	// Replacement identity must survive process loss as part of the accepted
	// operation, otherwise resuming regeneration would append a different turn.
	if target := c.regenerateTargetSnapshot(); target != "" {
		turn.identity.OperationID += agentrun.OperationID(".replace." + target)
	}
	snapshot, err := c.store.Snapshot(c.storyID, c.branchID)
	if err != nil {
		return nil, err
	}
	turn.sourceBoundary = gameRuntimeBoundary(snapshot)
	if target := c.regenerateTargetSnapshot(); target != "" {
		// Regeneration reads the target's parent. The cached head may include the
		// turn being replaced, so it must not resume that provider context.
		turn.sourceBoundary = "replace/" + target + "/" + turn.sourceBoundary
	}
	if cfg.Request.ResumeInterruptionID != "" {
		pending, err := ExternalTurnInterruption(c.store, c.storyID, c.branchID)
		if err != nil {
			return nil, err
		}
		if pending == nil || pending.ID != cfg.Request.ResumeInterruptionID {
			return nil, errors.New("Game interruption changed before resume")
		}
		found := false
		for _, input := range snapshot.PendingPlayerInputs {
			if input.ID != pending.PlayerInputID {
				continue
			}
			if !strings.HasPrefix(input.AgentOperationID, externalGameOperationPrefix) {
				return nil, errors.New("finish the interrupted turn with its original runtime")
			}
			turn.identity = agentrun.CycleIdentity{CommandID: agentrun.CommandID(input.AgentCommandID), OperationID: agentrun.OperationID(input.AgentOperationID), Cycle: input.AgentCycle}
			if cfg.Request.CommandID != input.AgentCommandID && strings.TrimSpace(cfg.Request.Message) != "" {
				turn.config.Guidance = append(turn.config.Guidance, cfg.Request)
			}
			turn.config.Request.Message, turn.config.Request.AttachedFiles = input.Text, input.Attachments
			turn.config.Request.ResumeInterruptionID = ""
			c.user = input.Text
			if input.ContextOnly {
				c.WithInputVisibility(agentrun.InputModelOnly)
			}
			found = true
			break
		}
		if !found {
			return nil, errors.New("interrupted Game input is unavailable")
		}
	}
	for _, input := range snapshot.PendingPlayerInputs {
		if strings.HasPrefix(input.AgentOperationID, externalGameOperationPrefix) && input.AgentOperationID != string(turn.identity.OperationID) {
			return nil, fmt.Errorf("%w: resume the unfinished Game turn before starting another", agent.ErrSessionBusy)
		}
	}
	c.BindAgentCycleIdentity(turn.identity)
	c.draftCommit = func(_ context.Context, draft interactive.TurnDraft, _ *agent.ToolResult) error {
		return c.store.SaveTurnDraft(c.storyID, c.branchID, draft, nil)
	}
	input, err := c.MaterializeAgentCanonicalInput(ctx, turn.config.Request.Message, turn.config.Request.AttachedFiles, nil)
	if err != nil {
		return nil, err
	}
	snapshot, err = c.store.Snapshot(c.storyID, c.branchID)
	if err != nil {
		return nil, err
	}
	pending := false
	for _, accepted := range snapshot.PendingPlayerInputs {
		if accepted.ID == input.Event.ID {
			pending = true
		}
	}
	if !pending {
		for _, settled := range snapshot.Turns {
			if settled.PlayerInputID == input.Event.ID {
				value := settled
				turn.replayed = &value
				return turn, nil
			}
		}
		return nil, fmt.Errorf("%w: Game command is already settled", agent.ErrSessionBusy)
	}
	for _, batch := range snapshot.PendingModelContextBatches {
		if batch.AgentOperationID == string(turn.identity.OperationID) {
			c.modelContextBatchSequence = max(c.modelContextBatchSequence, batch.Sequence+1)
			turn.restored = append(turn.restored, schemaMessagesFromInteractiveContext(batch.Messages)...)
		}
	}
	return turn, nil
}

func (turn *ExternalTurn) OperationID() agentrun.OperationID { return turn.identity.OperationID }

func (turn *ExternalTurn) Wait(ctx context.Context) agentrun.Outcome {
	turn.once.Do(func() {
		if turn.config.Release != nil {
			defer turn.config.Release()
		}
		turn.outcome = turn.run(ctx)
	})
	return turn.outcome
}

func (turn *ExternalTurn) run(ctx context.Context) (outcome agentrun.Outcome) {
	c := turn.config.Conversation
	turn.ctx, turn.cancel = context.WithCancel(ctx)
	defer turn.cancel()
	defer func() {
		if turn.runtimeSession != nil {
			_ = turn.runtimeSession.Close()
		}
	}()
	defer func() {
		if value := recover(); value != nil {
			outcome = agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: fmt.Errorf("external Game panic: %v", value)}
		}
		turn.mu.Lock()
		turn.closed = true
		text := turn.text
		turn.mu.Unlock()
		if outcome.Status == agentrun.OutcomeCompleted {
			turn.send(agentrun.Event{Type: "done", Data: map[string]any{"content": outcome.Content}})
			return
		}
		pending, err := c.store.PendingTurnInterruption(c.storyID, c.branchID)
		if err == nil && pending == nil {
			err = c.MarkInterrupted(c.user, text, "external_runtime_interrupted")
		}
		outcome.Error = errors.Join(outcome.Error, err)
		if turn.providerSettled && turn.runtimeSession != nil && (errors.Is(context.Cause(ctx), external.ErrSuspended) || errors.Is(context.Cause(ctx), external.ErrSteered)) && err == nil {
			snapshot, readErr := c.store.Snapshot(c.storyID, c.branchID)
			if readErr == nil {
				readErr = turn.runtimeSession.Accept(context.WithoutCancel(ctx), gameRuntimeBoundary(snapshot))
			}
			if readErr != nil {
				slog.WarnContext(ctx, "Could not retain interrupted Game runtime cache", "story_id", c.storyID, "error", readErr)
			}
		}
		slog.ErrorContext(ctx, "External Game turn interrupted", "story_id", c.storyID, "branch_id", c.branchID, "operation_id", turn.identity.OperationID, "error", outcome.Error)
		if ctx.Err() != nil {
			outcome.Status = agentrun.OutcomeAborted
			turn.send(agentrun.NewAbortedEvent(agentrun.AbortReasonUserRequested))
		} else {
			turn.send(agentrun.Event{Type: "error", Data: map[string]any{"error_key": "agentRuntime.operationFailed", "message": i18n.New(turn.config.Config.Language).T("agentRuntime.operationFailed"), "details": map[string]any{"detail": observability.ErrorCause(outcome.Error)}, "run_id": string(turn.identity.OperationID)}})
		}
	}()
	turn.send(agentrun.Event{Type: "agent_cycle_started", Data: map[string]any{
		"id": string(turn.identity.OperationID) + "-output", "command_id": turn.config.Request.CommandID,
		"delivery": "start_turn", "message": turn.config.Request.Message,
		"run_started_at": time.Now().UTC().Format(time.RFC3339Nano),
	}})
	if turn.replayed != nil {
		c.mu.Lock()
		c.lastTurn, c.lastStateReady = turn.replayed, true
		c.mu.Unlock()
		if err := c.CommitAgentCycle(ctx, agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
		}
		return agentrun.Outcome{Status: agentrun.OutcomeCompleted, Content: turn.replayed.Narrative}
	}
	input, err := turn.prepareInput(turn.ctx, external.OperationTurn)
	if err != nil {
		return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
	}
	if narrative, err := c.LoadNarrativeCandidate(ctx); err != nil {
		return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
	} else if c.InteractiveNarrativeReady() && narrative != "" {
		return turn.commit(ctx, narrative)
	}
	for {
		if err := turn.ctx.Err(); err != nil {
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
		}
		var result external.Result
		var runErr error
		if runtime := turn.config.Runtime; runtime != nil {
			if turn.runtimeSession == nil {
				response, err := runtime.Run(turn.ctx, external.SessionRequest{
					Key:      turn.config.Config.ProjectID + "/" + c.storyID + "/" + c.branchID,
					Boundary: turn.sourceBoundary, Input: input, Prepare: turn.prepareRuntimeInput,
				}, turn)
				result, turn.runtimeSession, runErr = response.Result, response.Session, err
			} else {
				result, runErr = turn.runtimeSession.Continue(turn.ctx, input, turn)
			}
		} else {
			result, runErr = turn.config.Adapter.Run(turn.ctx, input, turn)
		}
		turn.providerSettled = result.Settled
		if err := turn.recordUsage(result.Usage); err != nil {
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
		}
		turn.mu.Lock()
		if turn.toolError != nil {
			err := turn.toolError
			turn.mu.Unlock()
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
		}
		narrative, err := c.LoadNarrativeCandidate(context.WithoutCancel(ctx))
		if err == nil && narrative == "" && runErr == nil {
			text := turn.segment
			if text == "" {
				text = result.Text
				if text != "" {
					turn.text += text
					turn.send(agentrun.Event{Type: "chunk", Data: map[string]any{"content": text}})
				}
			}
			err = c.AcceptNarrativeCandidate(ctx, text)
			narrative = text
		}
		ready := c.InteractiveNarrativeReady() && narrative != ""
		turn.mu.Unlock()
		if err != nil {
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
		}
		if ready {
			return turn.commit(ctx, narrative)
		}
		if runErr != nil {
			return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: runErr}
		}
		// Continue with the same product draft. The locked prose is never
		// regenerated; only missing submission modules may be repaired.
		input.History = append(input.History, external.Message{Role: "user", Text: input.Text})
		input.History = append(input.History, turn.observations...)
		turn.observations = nil
		turn.segment = ""
		// Provider call IDs are scoped to each disposable attempt.
		turn.calls = map[string]externalGameCall{}
		if narrative != "" {
			input.History = append(input.History, external.Message{Role: "assistant", Text: narrative})
		}
		input.Text = "Complete the current turn by calling submit_interactive_turn for its missing modules. The first narrative is retained; do not repeat or rewrite it. Do not finish before state_changes and choices are accepted."
		if narrative == "" {
			input.Text = "Write the complete player-visible narrative for this turn. Then complete any missing turn submission modules."
			if c.InteractiveNarrativeReady() {
				input.Text = "Turn submission is already accepted. Write the complete player-visible narrative now without any tool calls."
				input.Tools = nil
			}
		}
		if turn.config.Runtime == nil {
			input, err = turn.prepareRuntimeInput(turn.ctx, input, turn.config.Adapter)
			if err != nil {
				return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
			}
		}
	}
}

func (turn *ExternalTurn) commit(ctx context.Context, narrative string) agentrun.Outcome {
	_, err := turn.config.Conversation.CommitAgentCanonicalOutput(context.WithoutCancel(ctx), agent.AssistantMessage(narrative, nil), session.MessageMetadata{RunID: string(turn.identity.OperationID), AgentKind: config.AgentKindInteractiveStory}, nil)
	if err != nil {
		return agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: err}
	}
	if turn.runtimeSession != nil {
		c := turn.config.Conversation
		snapshot, err := c.store.Snapshot(c.storyID, c.branchID)
		if err == nil {
			err = turn.runtimeSession.Accept(ctx, gameRuntimeBoundary(snapshot))
		}
		if err != nil {
			slog.WarnContext(ctx, "Game runtime cache binding could not be saved", "story_id", c.storyID, "branch_id", c.branchID, "error", err)
		}
	}
	return agentrun.Outcome{Status: agentrun.OutcomeCompleted, Content: narrative}
}

func (turn *ExternalTurn) Emit(event agentrun.Event) error {
	turn.mu.Lock()
	defer turn.mu.Unlock()
	if turn.closed {
		return errors.New("external Game turn is closed")
	}
	if err := turn.ctx.Err(); err != nil {
		return err
	}
	switch event.Type {
	case "chunk":
		if candidate, err := turn.config.Conversation.LoadNarrativeCandidate(turn.ctx); err != nil {
			return err
		} else if candidate != "" {
			return nil
		}
		turn.text += event.DataString("content")
		turn.segment += event.DataString("content")
	case "thinking":
	case "todo_updated":
		if observe := turn.config.ObservePlan; observe != nil {
			if err := observe(turn.ctx, event); err != nil {
				return err
			}
		}
		display, err := external.PlanDisplay(event)
		if err != nil {
			return err
		}
		display.RunID, display.AgentKind = string(turn.identity.OperationID), config.AgentKindInteractiveStory
		if err := turn.config.Conversation.AppendDisplayEvent(display); err != nil {
			return err
		}
	case "context_compaction":
		if display, ok := external.CompactionDisplay(event); ok {
			display.RunID, display.AgentKind = string(turn.identity.OperationID), config.AgentKindInteractiveStory
			if err := turn.config.Conversation.AppendDisplayEvent(display); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported external Game event %q", event.Type)
	}
	turn.send(event)
	return nil
}

func (turn *ExternalTurn) send(event agentrun.Event) {
	if turn.config.Emit == nil {
		return
	}
	if source, ok := event.Data.(map[string]any); ok {
		data := make(map[string]any, len(source)+3)
		for key, value := range source {
			data[key] = value
		}
		data["run_id"], data["operation_id"], data["cycle"] = string(turn.identity.OperationID), string(turn.identity.OperationID), turn.identity.Cycle
		event.Data = data
	}
	turn.config.Emit(event)
}
