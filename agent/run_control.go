package agent

import (
	"context"
	"sync"
)

type runCompletionControlKey struct{}

type runCompletionControl struct {
	mu        sync.RWMutex
	requested bool
	cancel    AgentCancelFunc
}

// RequestCompletionAfterTools asks the current root Agent to settle
// successfully at the next completed tool-batch boundary. It is intended for
// host protocol tools that atomically submit the final structured result after
// already producing the assistant-visible content.
func RequestCompletionAfterTools(ctx context.Context) bool {
	control, _ := ctx.Value(runCompletionControlKey{}).(*runCompletionControl)
	if control == nil {
		return false
	}
	control.mu.Lock()
	control.requested = true
	cancel := control.cancel
	control.mu.Unlock()
	if cancel == nil {
		return false
	}
	_, contributed := cancel(WithAgentCancelMode(CancelAfterToolCalls))
	return contributed
}

func (control *runCompletionControl) requestedCompletion() bool {
	if control == nil {
		return false
	}
	control.mu.RLock()
	defer control.mu.RUnlock()
	return control.requested
}
