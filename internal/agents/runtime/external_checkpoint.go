package agentruntime

import (
	"context"
	"encoding/json"

	"denova/internal/agents/runtime/external"
	externaljournal "denova/internal/agents/runtime/external/journal"
)

// PrepareExternalHistory is used only for canonical reconstruction. The runtime
// strips History before invoking it for an aligned provider continuation.
func (store ProductState) PrepareExternalHistory(ctx context.Context, preparation external.HistoryPreparation) (external.Input, error) {
	const capability = "denova.external.checkpoint.v1"
	if store.Read != nil {
		raw, found, err := store.Read(ctx, capability)
		if err != nil {
			return external.Input{}, err
		}
		if found {
			if err := json.Unmarshal(raw, &preparation.Checkpoint); err != nil {
				return external.Input{}, err
			}
		}
		preparation.SaveCheckpoint = func(checkpoint externaljournal.Checkpoint) error {
			return store.Update(ctx, capability, func(json.RawMessage, bool) (json.RawMessage, error) { return json.Marshal(checkpoint) })
		}
	}
	return preparation.Prepare(ctx)
}
