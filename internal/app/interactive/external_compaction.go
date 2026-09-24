package interactiveapp

import (
	"context"
	"errors"

	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/runtime/external"
)

// CompactExternal does not accept a player input, create a draft or advance the
// story. The caller holds the Game admission fence and external controller.
func CompactExternal(ctx context.Context, cfg ExternalTurnConfig) (agentcompaction.Result, error) {
	c := cfg.Conversation
	cfg.Request.Message = "Maintain the current story context."
	snapshot, err := c.store.Snapshot(c.storyID, c.branchID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	if snapshot.CurrentTurn == nil {
		return agentcompaction.Result{}, errors.New("Game context maintenance requires a completed turn")
	}
	turn := &ExternalTurn{config: cfg, ctx: ctx}
	input, err := turn.prepareInput(ctx, external.OperationCompact)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	observer := &external.MaintenanceObserver{Append: c.AppendDisplayEvent, EmitEvent: cfg.Emit}
	result, err := cfg.Runtime.Compact(ctx, external.SessionRequest{Key: cfg.Config.ProjectID + "/" + c.storyID + "/" + c.branchID, Boundary: gameRuntimeBoundary(snapshot), Input: input, Prepare: turn.prepareRuntimeInput}, observer)
	err = errors.Join(err, turn.recordUsage(result.Usage))
	if err == nil && !observer.Completed {
		err = errors.New("runtime did not acknowledge Game context compaction")
	}
	return agentcompaction.Result{Triggered: observer.Completed, RuntimeManaged: true}, err
}
