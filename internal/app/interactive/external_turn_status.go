package interactiveapp

import (
	"fmt"
	"strings"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

// ExternalTurnReplacement restores the existing operation's replacement
// target before the application prepares the corresponding branch context.
func ExternalTurnReplacement(store *interactive.Store, storyID, branchID, interruptionID string) (string, error) {
	pending, err := ExternalTurnInterruption(store, storyID, branchID)
	if err != nil || pending == nil {
		return "", err
	}
	if pending.ID != interruptionID {
		return "", fmt.Errorf("Game interruption changed before preparation")
	}
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		return "", err
	}
	for _, input := range snapshot.PendingPlayerInputs {
		if input.ID == pending.PlayerInputID && strings.HasPrefix(input.AgentOperationID, externalGameOperationPrefix) {
			_, target, _ := strings.Cut(input.AgentOperationID, ".replace.")
			return target, nil
		}
	}
	return "", nil
}

// ExternalTurnInterruption is read-only, including after process loss between
// accepting input and writing an interruption. Its identity derives from the
// existing accepted input; no extra recovery file or record format is needed.
func ExternalTurnInterruption(store *interactive.Store, storyID, branchID string) (*interactive.TurnInterruptedEvent, error) {
	pending, err := store.PendingTurnInterruption(storyID, branchID)
	if err != nil || pending != nil {
		return pending, err
	}
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		return nil, err
	}
	for _, input := range snapshot.PendingPlayerInputs {
		if strings.HasPrefix(input.AgentOperationID, externalGameOperationPrefix) {
			return &interactive.TurnInterruptedEvent{ID: input.ID + "-interrupted", PlayerInputID: input.ID, UserMessage: input.Text, BranchID: branchID, Reason: "external_runtime_interrupted"}, nil
		}
	}
	return nil, nil
}

func (turn *ExternalTurn) Status() agentrun.RuntimeStatus {
	c := turn.config.Conversation
	return agentrun.RuntimeStatus{Binding: agentrun.RuntimeBinding{AgentKind: config.AgentKindInteractiveStory,
		ProjectID: turn.config.Config.ProjectID, Mode: "interactive", Workspace: c.workspace, StoryID: c.storyID, BranchID: c.branchID},
		Phase: agentrun.RunPhaseRunning, ActiveCommandID: turn.identity.CommandID, ActiveOperation: turn.identity.OperationID, ActiveCycle: turn.identity.Cycle}
}

// ConsumedGuidance acknowledges only instructions confirmed by this Game turn.
// A recovered commit can predate guidance accepted by the product controller.
func (turn *ExternalTurn) ConsumedGuidance() int {
	if turn.replayed == nil {
		return len(turn.config.Guidance)
	}
	accepted := map[string]bool{}
	for _, message := range schemaMessagesFromInteractiveContext(turn.replayed.ModelContextMessages) {
		if command := interactive.UserGuidanceCommand(message); command != "" {
			accepted[command] = true
		}
	}
	for index, guidance := range turn.config.Guidance {
		if !accepted[guidance.CommandID] {
			return index
		}
	}
	return len(turn.config.Guidance)
}
