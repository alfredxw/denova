package execution

import (
	"context"
	"fmt"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

// bindChildTrace gives the child its own recorder and event consumer. Task
// forwarding uses Session.Observe, so consuming this Run's events cannot steal
// display events. Preparation is serialized by the child Session lifecycle.
func (backend *publicBackend) bindChildTrace(ctx context.Context, request agent.PrepareRequest, options agentrun.Options, parent *publicCycleRegistration) (agent.Middleware, error) {
	backend.mu.RLock()
	handle := backend.runs[request.Run.ID]
	backend.mu.RUnlock()
	if handle == nil {
		session, err := backend.agent.Session(ctx, request.Session.Key)
		if err != nil {
			return nil, err
		}
		run, found, err := session.AttachRun(ctx, request.Run.ID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("delegated Run %q is unavailable for tracing", request.Run.ID)
		}
		trace := newPublicAgentRunTrace(run.ID())
		if parent != nil {
			parent.mu.RLock()
			parentTrace := parent.trace
			parent.mu.RUnlock()
			if parentTrace != nil {
				parentTrace.mu.Lock()
				trace.parentRunID = parentTrace.runID
				parentTrace.mu.Unlock()
			}
		}
		traceOptions := options
		traceOptions.SessionID = request.Session.Key.ID
		traceOptions.RootAgentName = request.Session.Key.Attributes["agent"]
		registration := &publicCycleRegistration{options: traceOptions, trace: trace}
		handle = backend.trackRun(session, run, registration, "")
	}
	// Keep workspace effects in their parent product scope; only the trace
	// binder uses the independent child Session identity.
	return agent.IdentifyMiddleware(
		agentchat.NewPublicHostMiddleware(agentchat.ChatRequest{}, options, handle.registration),
		publicCapabilityIdentity("denova.child_trace", request.Session.Key),
	), nil
}
