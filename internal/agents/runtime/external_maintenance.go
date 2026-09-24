package agentruntime

import (
	"context"
	"encoding/json"

	agentcompaction "denova/internal/agents/context/compaction"
	agentrun "denova/internal/agents/run"
)

// Maintain serializes explicit context maintenance with task admission and
// preserves its retry receipt in the product journal, without a Native Session.
func (control *ExternalController) Maintain(ctx context.Context, commandID string, maintain func() (agentcompaction.Result, error)) (agentcompaction.Result, error) {
	if err := agentrun.ValidateCommandID(commandID); err != nil {
		return agentcompaction.Result{}, err
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	const capability = "denova.external.maintenance.v1"
	raw, found, err := control.store.Read(ctx, capability)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	receipts := map[string]agentcompaction.Result{}
	if found {
		if err := json.Unmarshal(raw, &receipts); err != nil {
			return agentcompaction.Result{}, err
		}
	}
	if receipt, ok := receipts[commandID]; ok {
		return receipt, nil
	}
	state, err := control.read(ctx)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	if control.active != nil || state.Phase != agentrun.RunPhaseIdle {
		return agentcompaction.Result{}, ErrOperationActive
	}
	result, err := maintain()
	if err != nil {
		return result, err
	}
	err = control.store.Update(context.WithoutCancel(ctx), capability, func(raw json.RawMessage, present bool) (json.RawMessage, error) {
		if present {
			if err := json.Unmarshal(raw, &receipts); err != nil {
				return nil, err
			}
		}
		receipts[commandID] = result
		return json.Marshal(receipts)
	})
	return result, err
}
