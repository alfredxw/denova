package external

import (
	"context"
	"errors"

	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
)

// CompactSession performs maintenance without admitting a user or assistant
// message. The caller holds its product admission fence for the entire call.
func CompactSession(ctx context.Context, request StartRequest, prepare func(context.Context, HistoryPreparation) (Input, error)) (agentcompaction.Result, error) {
	_, wire, err := prepareTools(ctx, request.Definitions)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	request.Input.Tools = wire
	var incarnation string
	if err := request.Session.ReadExternal(ctx, func(state session.ExternalState) error {
		incarnation = state.Incarnation
		return state.Projection.RequireIdle()
	}); err != nil {
		return agentcompaction.Result{}, err
	}
	projection := &Operation{request: request}
	observer := &MaintenanceObserver{Append: request.Session.AppendDisplayEvent, EmitEvent: request.Emit}
	result, err := request.Runtime.Compact(ctx, SessionRequest{
		Key:      request.ProjectID + "/" + request.Session.ID + "/" + incarnation,
		Boundary: request.SourceBoundary, Input: request.Input,
		Prepare: func(ctx context.Context, input Input, adapter Adapter) (Input, error) {
			return prepare(ctx, HistoryPreparation{Input: input, Adapter: adapter,
				ProviderInputMaxBytes: request.ProviderInputMaxBytes, ResolveMedia: projection.media().Resolve, LoadHistory: request.LoadHistory})
		},
	}, observer)
	if result.Usage != nil {
		display := UsageDisplay(result.Usage)
		display.AgentKind = request.ToolPolicy.AgentKind
		err = errors.Join(err, request.Session.AppendDisplayEvent(display))
	}
	if err == nil && !observer.Completed {
		err = errors.New("runtime did not acknowledge context compaction")
	}
	return agentcompaction.Result{Triggered: observer.Completed, RuntimeManaged: true}, err
}
