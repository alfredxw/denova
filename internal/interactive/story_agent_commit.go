package interactive

import (
	"context"
	"crypto/sha256"
	interactivestate "denova/internal/interactive/state"
	"encoding/json"
	"errors"
	"fmt"
	agent "github.com/alfredxw/denova/agent"
	"log/slog"
	"strings"
	"time"
)

// ErrAgentTurnIdentityConflict means one durable Agent command attempted to
// commit two different game turns. Returning the original turn for an exact
// retry is safe; accepting a different payload under the same command ID is
// not.
var ErrAgentTurnIdentityConflict = errors.New("agent turn identity conflict")

// DomainCommitIdentity is shared with the durable Agent coordinator without
// coupling story storage to the runtime package.
type DomainCommitIdentity struct {
	CommandID   string `json:"command_id"`
	OperationID string `json:"operation_id"`
	Cycle       int    `json:"cycle"`
}

type DomainCommitIntent struct {
	Identity DomainCommitIdentity
	Hash     string
	Request  AppendTurnWithStateRequest
}

type DomainCommitReceipt struct {
	Identity           DomainCommitIdentity `json:"identity"`
	Hash               string               `json:"hash"`
	AgentCanonicalHash string               `json:"agent_canonical_hash,omitempty"`
	Revision           string               `json:"revision"`
	Turn               TurnEvent            `json:"turn"`
	Delta              *StateDeltaEvent     `json:"delta,omitempty"`
}

func NewDomainCommitIntent(req AppendTurnWithStateRequest) (DomainCommitIntent, error) {
	providerContinuation, err := normalizeProviderContinuation(req.ProviderContinuation)
	if err != nil {
		return DomainCommitIntent{}, err
	}
	req.ProviderContinuation = providerContinuation
	identity := DomainCommitIdentity{
		CommandID:   strings.TrimSpace(req.AgentCommandID),
		OperationID: strings.TrimSpace(req.AgentOperationID),
		Cycle:       req.AgentCycle,
	}
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 {
		return DomainCommitIntent{}, fmt.Errorf("%w: command_id, operation_id, and positive cycle are required together", ErrAgentTurnIdentityConflict)
	}
	hash, err := agentTurnRequestHash(req)
	if err != nil {
		return DomainCommitIntent{}, err
	}
	return DomainCommitIntent{Identity: identity, Hash: hash, Request: req}, nil
}

// CommitDomainTurn publishes the staged game turn through the canonical story
// store. Turn ID is the native append-only revision returned to the actor.
func (s *Store) CommitDomainTurn(storyID string, intent DomainCommitIntent) (DomainCommitReceipt, error) {
	canonical, err := NewDomainCommitIntent(intent.Request)
	if err != nil {
		return DomainCommitReceipt{}, err
	}
	if canonical.Identity != intent.Identity || canonical.Hash != strings.TrimSpace(intent.Hash) {
		return DomainCommitReceipt{}, fmt.Errorf("%w: staged intent identity or hash changed", ErrAgentTurnIdentityConflict)
	}
	turn, delta, err := s.AppendTurnWithState(storyID, intent.Request)
	if err != nil {
		return DomainCommitReceipt{}, err
	}
	return DomainCommitReceipt{
		Identity: canonical.Identity, Hash: canonical.Hash,
		AgentCanonicalHash: strings.TrimSpace(turn.AgentCanonicalHash), Revision: turn.ID,
		Turn: turn, Delta: delta,
	}, nil
}

// AppendTurnWithState atomically publishes one canonical turn and its state
// delta. Durable Agent identities make the write idempotent and allow an
// ambiguous post-rename filesystem error to reconcile the exact committed
// turn without executing the cycle twice.
func (s *Store) AppendTurnWithState(storyID string, req AppendTurnWithStateRequest) (TurnEvent, *StateDeltaEvent, error) {
	providerContinuation, err := normalizeProviderContinuation(req.ProviderContinuation)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	req.ProviderContinuation = providerContinuation
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	lines, err = projectStoryEventOverlays(lines)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnEvent{}, nil, fmt.Errorf("分支不存在: %s", branchID)
	}
	branchProjection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	if existing, delta, found, err := committedAgentTurnForRequest(lines, branchID, req); err != nil {
		return TurnEvent{}, nil, err
	} else if found {
		s.syncStoryIndexProjectionLocked(storyID)
		return existing, delta, nil
	}
	playerInput, hasPlayerInput, err := playerInputForTurnRequest(lines, branchID, req)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	if strings.TrimSpace(req.AgentCommandID) != "" && !hasPlayerInput {
		return TurnEvent{}, nil, fmt.Errorf("%w: canonical player input is missing for completed turn", ErrPlayerInputIdentityConflict)
	}
	var consumedPlayerInputIDs []string
	var resolvedPlayerInputContexts []ResolvedPlayerInputContext
	if hasPlayerInput {
		_, activeAncestry := eventPath(branch.Head, eventsByID(lines))
		pendingInputs, pendingErr := pendingPlayerInputsForBranch(lines, branchID, activeAncestry)
		if pendingErr != nil {
			return TurnEvent{}, nil, pendingErr
		}
		pendingInputs = pendingPlayerInputsFromProjection(pendingInputs, branchProjection.PendingPlayerInputIDs)
		consumedPlayerInputIDs = make([]string, 0, len(pendingInputs))
		currentInputIncluded := false
		for _, pending := range pendingInputs {
			consumedPlayerInputIDs = append(consumedPlayerInputIDs, pending.ID)
			currentInputIncluded = currentInputIncluded || pending.ID == playerInput.ID
		}
		if !currentInputIncluded {
			consumedPlayerInputIDs = append(consumedPlayerInputIDs, playerInput.ID)
		}
		resolvedPlayerInputContexts, err = resolveHistoricalPlayerInputContexts(lines, branchID, pendingInputs, playerInput.ID)
		if err != nil {
			return TurnEvent{}, nil, err
		}
		// Only the current cycle's tool suffix belongs to this Turn. Historical
		// pending cycles retain their own acceptance position and evidence in
		// ResolvedPlayerInputContexts instead of being moved to this later Turn.
		req.ModelContextMessages, err = mergeModelContextBatchesForPlayerInput(lines, branchID, playerInput.ID, req.ModelContextMessages)
		if err != nil {
			return TurnEvent{}, nil, err
		}
	}
	if req.ExpectedParentID != nil && branch.Head != strings.TrimSpace(*req.ExpectedParentID) {
		return TurnEvent{}, nil, fmt.Errorf("%w: 当前分支已前进，拒绝提交基于旧版本的回合: expected_parent=%s current_head=%s", ErrStoryContextRevisionConflict, strings.TrimSpace(*req.ExpectedParentID), branch.Head)
	}
	commitParentID := branch.Head
	if replaceTurnID := strings.TrimSpace(req.ReplaceTurnID); replaceTurnID != "" {
		if err := requireLatestLogicalTurn(meta, lines, branchID, replaceTurnID); err != nil {
			return TurnEvent{}, nil, err
		}
		commitParentID, err = regenerationParentOnCurrentPath(lines, branch.Head, replaceTurnID)
		if err != nil {
			return TurnEvent{}, nil, err
		}
	}
	if branchIsTerminal(lines, commitParentID) {
		return TurnEvent{}, nil, fmt.Errorf("当前分支已终局，请从历史回合创建新分支后继续")
	}
	parentID := any(nil)
	if commitParentID != "" {
		parentID = commitParentID
	}
	state := cloneStoryState(branchProjection.State)
	if strings.TrimSpace(req.ReplaceTurnID) != "" {
		state = cloneStoryState(branchProjection.StateBeforeLatest)
	}
	director := s.storyDirectorForMeta(meta)
	actorState := actorStateSystemFromSnapshot(meta.ActorStateSchema, director.ActorState)
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	terminal := (req.TerminalOutcome != nil && req.TerminalOutcome.Terminal) || (req.RuleResolution != nil && req.RuleResolution.TerminalCandidate != nil)
	turnResult := normalizeTurnResultPointer(req.TurnResult, meta.ChoiceCount, terminal)
	if req.TurnResult != nil && turnResult == nil {
		return TurnEvent{}, nil, fmt.Errorf("TurnResult 未通过校验")
	}
	agentCommitHash := ""
	if strings.TrimSpace(req.AgentCommandID) != "" {
		agentCommitHash, err = agentTurnRequestHash(req)
		if err != nil {
			return TurnEvent{}, nil, err
		}
	}
	turn := TurnEvent{
		V:                           schemaVersion,
		Type:                        StoryEventTypeTurn,
		ID:                          newID("ev"),
		ParentID:                    parentID,
		BranchID:                    branchID,
		Ts:                          now,
		User:                        req.User,
		Attachments:                 append([]agent.Attachment(nil), playerInput.Attachments...),
		UserContextOnly:             playerInput.ContextOnly,
		Narrative:                   req.Narrative,
		Thinking:                    strings.TrimSpace(req.Thinking),
		RunID:                       strings.TrimSpace(req.RunID),
		AgentKind:                   strings.TrimSpace(req.AgentKind),
		AgentCommandID:              strings.TrimSpace(req.AgentCommandID),
		AgentOperationID:            strings.TrimSpace(req.AgentOperationID),
		AgentCycle:                  req.AgentCycle,
		AgentCommitHash:             agentCommitHash,
		AgentCanonicalHash:          strings.TrimSpace(req.AgentCanonicalHash),
		PlayerInputID:               playerInput.ID,
		PlayerInputHash:             playerInput.AgentCommitHash,
		ConsumedPlayerInputIDs:      append([]string(nil), consumedPlayerInputIDs...),
		ResolvedPlayerInputContexts: cloneResolvedPlayerInputContexts(resolvedPlayerInputContexts),
		DisplayEvents:               sanitizeDisplayEvents(req.DisplayEvents),
		ModelContextMessages:        sanitizeModelContextMessages(req.ModelContextMessages),
		RuleResolution:              normalizeRuleResolutionPointer(req.RuleResolution),
		TurnResult:                  turnResult,
		TerminalOutcome:             normalizeTerminalOutcomePointer(req.TerminalOutcome),
		Flags:                       map[string]bool{"pinned": false, "locked": false},
	}
	actorState, openingOps, openingActorOps, err := prepareOpeningGameStateSchemaCommit(&meta, lines, state, actorState, branchID, turn.ID, now, req.StateSchemaProposal)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	ops := normalizeStateOps(append(openingOps, req.Ops...))
	actorOps := normalizeActorStateOps(append(openingActorOps, req.ActorOps...))
	if turn.TurnResult != nil && len(turn.TurnResult.StateUpdates) > 0 {
		compiled, err := CompileTurnStateUpdates(actorState, state, turn.TurnResult.StateUpdates, TurnStateUpdateCompileOptions{
			SourceTurnID:             turn.ID,
			RuleResolution:           turn.RuleResolution,
			RuleStateConsumptionMode: director.Strategy.RuleStateConsumptionMode,
		})
		if err != nil {
			return TurnEvent{}, nil, fmt.Errorf("TurnResult state_updates 校验失败: %w", err)
		}
		turn.TurnResult.StateUpdates = compiled.Updates
		for i := range compiled.Ops {
			compiled.Ops[i].SourceKind = interactivestate.SourceTurnResult
			compiled.Ops[i].SourceID = turn.ID
			compiled.Ops[i].SourceTurnID = turn.ID
		}
		ops = append(ops, compiled.Ops...)
		for i := range compiled.ActorOps {
			compiled.ActorOps[i].SourceKind = interactivestate.SourceTurnResult
			compiled.ActorOps[i].SourceID = turn.ID
			compiled.ActorOps[i].SourceTurnID = turn.ID
		}
		actorOps = append(actorOps, compiled.ActorOps...)
	}
	if turn.RuleResolution != nil {
		ruleOps, ruleActorOps := applyRuleStateConsumptionV2(state, actorState, turn.ID, turn.RuleResolution, director.Strategy.RuleStateConsumptionMode)
		ops = append(ops, ruleOps...)
		actorOps = append(actorOps, ruleActorOps...)
	}
	branch.Head = turn.ID

	var delta *StateDeltaEvent
	actorOps = normalizeActorStateOps(actorOps)
	if len(ops) > 0 || len(actorOps) > 0 {
		for _, op := range ops {
			if err := validateStateOp(op); err != nil {
				return TurnEvent{}, nil, err
			}
		}
		for _, op := range actorOps {
			if err := validateActorStateOp(op); err != nil {
				return TurnEvent{}, nil, err
			}
		}
		stateDelta := newStateDeltaWithActorOps(ops, actorOps)
		turn.StateDelta = &stateDelta
		turn.StateStatus = "ready"
		stateDeltaEvent := newStateDeltaEventWithActorOps(turn.ID, parentIDString(parentID), branchID, now, ops, actorOps)
		delta = &stateDeltaEvent
	} else if turn.TurnResult != nil {
		turn.StateStatus = "ready"
	} else {
		turn.StateStatus = "pending"
	}

	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	newEvents := []any{turn}
	var committedPlanUpdate *string
	if meta.PlanningMode == StoryPlanningModeEnabled && turn.TurnResult != nil {
		committedPlanUpdate = cloneStringPointer(turn.TurnResult.PlanUpdate)
		// Replacing the latest Turn first restores the branch projection to the
		// target's parent. Omission means preserve the current plan, so attach
		// that same content to the replacement Turn instead of losing it.
		if committedPlanUpdate == nil && strings.TrimSpace(req.ReplaceTurnID) != "" && branchProjection.Plan != nil {
			committedPlanUpdate = cloneStringPointer(&branchProjection.Plan.Markdown)
		}
	}
	if committedPlanUpdate != nil {
		newEvents = append(newEvents, BranchPlanUpdatedEvent{
			V: schemaVersion, Type: StoryEventTypeBranchPlanUpdated, ID: newID("bpu"),
			ParentID: turn.ID, BranchID: turn.BranchID, Ts: turn.Ts, TurnID: turn.ID,
			Markdown: normalizeBranchPlanMarkdown(*committedPlanUpdate),
		})
	}
	modelContinuationEvents, err := newModelContextProviderContinuationEvents(turn.ID, turn.BranchID, turn.Ts, turn.ModelContextMessages)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	newEvents = append(newEvents, modelContinuationEvents...)
	if len(req.ProviderContinuation) != 0 {
		newEvents = append(newEvents, newProviderContinuationEvent(turn, req.ProviderContinuation))
	}
	if appendErr := s.appendStoryTransactionLocked(storyID, meta, newEvents...); appendErr != nil {
		return TurnEvent{}, nil, appendErr
	}
	s.syncStoryIndexProjectionLocked(storyID)
	return turn, delta, nil
}

func committedAgentTurnForRequest(lines []StoryEventRecord, branchID string, req AppendTurnWithStateRequest) (TurnEvent, *StateDeltaEvent, bool, error) {
	commandID := strings.TrimSpace(req.AgentCommandID)
	operationID := strings.TrimSpace(req.AgentOperationID)
	hasIdentity := commandID != "" || operationID != "" || req.AgentCycle != 0
	if hasIdentity && (commandID == "" || operationID == "" || req.AgentCycle <= 0) {
		return TurnEvent{}, nil, false, fmt.Errorf("%w: command_id, operation_id, and positive cycle are required together", ErrAgentTurnIdentityConflict)
	}
	if commandID == "" {
		return TurnEvent{}, nil, false, nil
	}
	commitHash, err := agentTurnRequestHash(req)
	if err != nil {
		return TurnEvent{}, nil, false, err
	}
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return TurnEvent{}, nil, false, fmt.Errorf("decode committed agent turn: %w", err)
		}
		if strings.TrimSpace(turn.AgentCommandID) != commandID {
			continue
		}
		if turn.BranchID != branchID || strings.TrimSpace(turn.AgentOperationID) != operationID || turn.AgentCycle != req.AgentCycle || strings.TrimSpace(turn.AgentCommitHash) == "" || turn.AgentCommitHash != commitHash ||
			strings.TrimSpace(req.AgentCanonicalHash) != "" && turn.AgentCanonicalHash != strings.TrimSpace(req.AgentCanonicalHash) {
			return TurnEvent{}, nil, false, fmt.Errorf("%w: command_id=%q existing_operation=%q requested_operation=%q existing_cycle=%d requested_cycle=%d", ErrAgentTurnIdentityConflict, commandID, turn.AgentOperationID, operationID, turn.AgentCycle, req.AgentCycle)
		}
		return turn, stateDeltaEventForCommittedTurn(turn), true, nil
	}
	return TurnEvent{}, nil, false, nil
}

func agentTurnRequestHash(req AppendTurnWithStateRequest) (string, error) {
	// ModelContextMessages are durable side evidence keyed independently by the
	// accepted player input and batch ordinal. Excluding them keeps a staged Turn
	// identity stable when the store atomically folds those batches into it.
	payload := struct {
		BranchID             string
		ExpectedParentID     *string
		ReplaceTurnID        string
		User                 string
		Narrative            string
		Thinking             string
		RunID                string
		AgentKind            string
		ProviderContinuation map[string]any
		DisplayEvents        []DisplayEvent
		Ops                  []interactivestate.Op
		ActorOps             []ActorStateOp
		RuleResolution       *RuleResolution
		TurnResult           *TurnResult
		PlanUpdate           *string
		TerminalOutcome      *TerminalOutcome
		StateSchemaProposal  *ActorStateSchemaProposal
	}{
		BranchID: strings.TrimSpace(req.BranchID), ExpectedParentID: req.ExpectedParentID,
		ReplaceTurnID: strings.TrimSpace(req.ReplaceTurnID),
		User:          req.User, Narrative: req.Narrative, Thinking: req.Thinking,
		RunID: req.RunID, AgentKind: req.AgentKind, ProviderContinuation: req.ProviderContinuation,
		DisplayEvents: req.DisplayEvents,
		Ops:           req.Ops, ActorOps: req.ActorOps, RuleResolution: req.RuleResolution,
		TurnResult: req.TurnResult, PlanUpdate: planUpdateFromTurnResult(req.TurnResult), TerminalOutcome: req.TerminalOutcome,
		StateSchemaProposal: req.StateSchemaProposal,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("hash agent turn commit payload: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum[:]), nil
}

func planUpdateFromTurnResult(result *TurnResult) *string {
	if result == nil {
		return nil
	}
	return cloneStringPointer(result.PlanUpdate)
}

func stateDeltaEventForCommittedTurn(turn TurnEvent) *StateDeltaEvent {
	if turn.StateDelta == nil {
		return nil
	}
	event := newStateDeltaEventWithActorOps(
		turn.ID, parentIDString(turn.ParentID), turn.BranchID, turn.Ts,
		append([]interactivestate.Op(nil), turn.StateDelta.Ops...), append([]ActorStateOp(nil), turn.StateDelta.ActorOps...),
	)
	event.SchemaVersion = turn.StateDelta.SchemaVersion
	return &event
}

// syncStoryIndexProjectionLocked updates the rebuildable story catalog from
// canonical JSONL state. A projection failure is logged but never turns an
// already-durable event into a false-negative operation result.
func (s *Store) syncStoryIndexProjectionLocked(storyID string) {
	if _, err := s.publishStorySummaryLocked(storyID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] index projection write failed after canonical commit story_id=%s err=%v", storyID, err))
	}
}
