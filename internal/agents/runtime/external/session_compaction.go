package external

import (
	"context"
	"errors"

	"denova/config"
	agentrun "denova/internal/agents/run"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
)

// ProviderCompactionSupported reports whether an engine can compact its own
// provider conversation. Antigravity and Grok expose no compaction protocol, so
// product-side checkpoint maintenance is the only durable option there.
func ProviderCompactionSupported(kind config.RuntimeID) bool {
	switch kind {
	case config.RuntimeAntigravity, config.RuntimeGrok:
		return false
	default:
		return true
	}
}

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
	if !ProviderCompactionSupported(request.Runtime.Selection.Kind) {
		return compactThroughCheckpoint(ctx, request, prepare, projection, observer)
	}
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

// compactThroughCheckpoint is the fallback for engines without a compaction
// protocol. The canonical history is summarized into the product checkpoint,
// which the next rebuild applies, so the command still has a durable effect
// instead of failing on a protocol the engine never implemented.
func compactThroughCheckpoint(ctx context.Context, request StartRequest, prepare func(context.Context, HistoryPreparation) (Input, error), projection *Operation, observer *MaintenanceObserver) (agentcompaction.Result, error) {
	adapter, release, err := request.Runtime.Acquire(ctx)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	defer release()
	input := request.Input
	input.Selection = request.Runtime.Selection
	// Maintenance is provider-session independent: no resume, no accumulated
	// usage and no bound working directory may leak into the summary attempt.
	input.Directory, input.SessionID, input.CumulativeUsage = "", "", nil
	if _, err := prepare(ctx, HistoryPreparation{Input: input, Adapter: adapter,
		ProviderInputMaxBytes: request.ProviderInputMaxBytes, ResolveMedia: projection.media().Resolve, LoadHistory: request.LoadHistory}); err != nil {
		return agentcompaction.Result{}, err
	}
	// The checkpoint is already persisted through SaveCheckpoint; report the
	// maintenance so the conversation leaves its pending state.
	if err := observer.Emit(agentrun.Event{Type: "context_compaction", Data: map[string]any{
		"status": "completed", "phase": "agent", "runtime_managed": false,
	}}); err != nil {
		return agentcompaction.Result{}, err
	}
	return agentcompaction.Result{Triggered: true, RuntimeManaged: true}, nil
}
