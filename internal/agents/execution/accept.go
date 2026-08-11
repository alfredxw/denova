package execution

import (
	"context"
	"denova/internal/agents/run"
)

// StartRequest contains one fully prepared initial cycle and its process-local
// display sink. Initial preparation remains caller-controlled so validation can
// fail before durable command acceptance.
type StartRequest struct {
	Cycle Cycle
	Emit  func(agentrun.Event)
}

// Run admits and waits for one prepared durable cycle.
func (s *Runtime) Run(ctx context.Context, request StartRequest) agentrun.Outcome {
	if s == nil || s.public == nil {
		emitExecutionError(request.Emit, ErrUnavailable)
		return agentrun.NewOutcome(agentrun.OutcomeFailed, ErrUnavailable, ErrUnavailable.Error(), "", "")
	}
	accepted, err := s.public.start(ctx, request)
	if err != nil {
		emitExecutionError(request.Emit, err)
		return agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
	}
	return accepted.Wait(ctx)
}

// Start durably accepts a prepared initial cycle and returns before model
// settlement.
func (s *Runtime) Start(ctx context.Context, request StartRequest) (*Operation, error) {
	if s == nil || s.public == nil {
		return nil, ErrUnavailable
	}
	return s.public.start(ctx, request)
}
