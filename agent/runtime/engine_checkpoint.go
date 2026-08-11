package runtime

import (
	"context"
	"encoding/json"
)

// EngineCheckpointSnapshot is the private, bounded continuation state needed
// by the Agent facade to prepare a structural operation. It is never a display
// projection and must not be exposed by transports.
type EngineCheckpointSnapshot struct {
	Cursor       Cursor
	State        json.RawMessage
	Capabilities map[string]json.RawMessage
}

type engineCheckpointRequest struct{ response chan EngineCheckpointSnapshot }

func (h *Harness) EngineCheckpoint(ctx context.Context) (EngineCheckpointSnapshot, error) {
	if h == nil {
		return EngineCheckpointSnapshot{}, ErrHarnessClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request := engineCheckpointRequest{response: make(chan EngineCheckpointSnapshot, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return EngineCheckpointSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return EngineCheckpointSnapshot{}, ctx.Err()
	}
	select {
	case snapshot := <-request.response:
		return snapshot, nil
	case <-h.done:
		return EngineCheckpointSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return EngineCheckpointSnapshot{}, ctx.Err()
	}
}
