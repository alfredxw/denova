package execution

import (
	"context"
	"strings"
	"sync"

	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

// RecoveryObservation attaches a new display stream to an active in-process
// Agent Run. It does not resume work after a process restart.
type RecoveryObservation struct {
	publicBackend     *publicBackend
	publicSession     *agent.Session
	publicObservation agent.Observation
	publicInitial     agent.SessionSnapshot
	publicBinding     agentrun.RuntimeBinding
	publicOptions     agentrun.Options
	publicHandle      *publicRunHandle
	cancel            context.CancelFunc

	mu                      sync.Mutex
	publicTerminalDelivered bool
}

func (s *Runtime) OpenRecoveryObservation(ctx context.Context, options agentrun.Options) (*RecoveryObservation, error) {
	if s == nil || s.public == nil {
		return nil, ErrRuntimeProjectionUnavailable
	}
	session, binding, err := s.public.openSession(ctx, options)
	if err != nil {
		return nil, err
	}
	observeCtx, cancel := context.WithCancel(s.public.lifecycle)
	observation, err := session.Observe(observeCtx, 0)
	if err != nil {
		cancel()
		return nil, err
	}
	return &RecoveryObservation{
		publicBackend: s.public, publicSession: session, publicObservation: observation,
		publicInitial: observation.Snapshot, publicBinding: binding,
		publicOptions: options.Normalize(options.Workspace), cancel: cancel,
	}, nil
}

func (r *RecoveryObservation) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *RecoveryObservation) InitialStatus() agentrun.RuntimeStatus {
	if r == nil {
		return agentrun.RuntimeStatus{}
	}
	return publicRuntimeStatus(r.publicBinding, r.publicInitial)
}

func (r *RecoveryObservation) CurrentStatus(ctx context.Context) (agentrun.RuntimeStatus, error) {
	if r == nil || r.publicSession == nil {
		return agentrun.RuntimeStatus{}, ErrRuntimeProjectionUnavailable
	}
	snapshot, err := r.publicSession.Snapshot(ctx)
	if err != nil {
		return agentrun.RuntimeStatus{}, err
	}
	return publicRuntimeStatus(r.publicBinding, snapshot), nil
}

// DisplayMetadata resolves the product display identity from the active Run.
func (r *RecoveryObservation) DisplayMetadata(ctx context.Context, action RuntimeRecoveryAction) (RuntimeRecoveryDisplayMetadata, error) {
	if r == nil || r.publicSession == nil {
		return RuntimeRecoveryDisplayMetadata{}, ErrRuntimeProjectionUnavailable
	}
	input, found, err := r.publicSession.RunInput(ctx, string(action.OperationID))
	if err != nil || !found {
		return RuntimeRecoveryDisplayMetadata{}, err
	}
	data, err := agentlifecycle.DecodeTurnHostData(input)
	if err != nil {
		return RuntimeRecoveryDisplayMetadata{Message: input.Text, Attachments: publicAttachmentDescriptors(input.Attachments)}, nil
	}
	return RuntimeRecoveryDisplayMetadata{
		Message: strings.TrimSpace(data.Caller.Message), RegenerateFromTurnID: data.TurnID,
		Attachments: publicAttachmentDescriptors(input.Attachments),
	}, nil
}

func publicAttachmentDescriptors(values []agent.Attachment) []agent.Attachment {
	result := append([]agent.Attachment(nil), values...)
	for index := range result {
		result[index].Path = ""
		result[index].SHA256 = ""
	}
	return result
}
