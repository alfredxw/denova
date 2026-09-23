package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"denova/config"
	"denova/internal/agents/runtime/external"
)

// ExternalRuntime captures the selected application's engine connection.
// Native Agent remains a peer runtime and is never constructed by this path.
func (engines *Engines) ExternalRuntime(cfg config.Config) *external.Runtime {
	if cfg.ActiveAgentRuntime == nil || cfg.ActiveAgentRuntime.Kind == config.RuntimeNative {
		return nil
	}
	selection := *cfg.ActiveAgentRuntime
	connectionKey := ""
	if selection.ModelProfileID() != "" {
		if model, err := config.ResolveRuntimeModel(&cfg, selection); err == nil {
			encoded, _ := json.Marshal(model)
			digest := sha256.Sum256(encoded)
			connectionKey = hex.EncodeToString(digest[:])
		}
	}
	return &external.Runtime{Selection: selection, ConnectionKey: connectionKey, Acquire: func(ctx context.Context) (external.Adapter, func(), error) {
		return engines.Acquire(ctx, selection, cfg)
	}}
}
