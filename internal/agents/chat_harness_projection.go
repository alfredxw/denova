package agents

import (
	"context"
	"errors"
	"fmt"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// ErrRuntimeProjectionUnavailable means this ChatService is nil or was not
// constructed with the mandatory durable command harness.
var ErrRuntimeProjectionUnavailable = errors.New("durable agent runtime projection is unavailable")

// RuntimeStatusProjection returns a bounded point-in-time display projection.
// It cannot carry durable messages and is never used as model context.
func (s *ChatService) RuntimeStatusProjection(ctx context.Context, options RunOptions) (runstate.StatusSnapshot, error) {
	if s == nil || s.harness == nil || s.harness.runtime == nil {
		return runstate.StatusSnapshot{}, ErrRuntimeProjectionUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		return runstate.StatusSnapshot{}, fmt.Errorf("derive agent runtime projection binding: %w", err)
	}
	status, err := s.harness.runtime.Project(ctx, binding)
	if err != nil {
		return runstate.StatusSnapshot{}, fmt.Errorf("project agent runtime status: %w", err)
	}
	return status, nil
}

// CloseRuntimeBindings exposes the runtime scope barrier to App lifecycle
// owners without leaking the Harness implementation.
func (s *ChatService) CloseRuntimeBindings(ctx context.Context, selector runstate.BindingSelector) error {
	if s == nil || s.harness == nil || s.harness.runtime == nil {
		return ErrRuntimeProjectionUnavailable
	}
	return s.harness.runtime.CloseBindings(ctx, selector)
}
