package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"denova/config"
	"denova/internal/agents/modelio"
	agent "github.com/alfredxw/denova/agent"
)

func (resolver Models) Resolve(ctx context.Context, profileID string) (agent.BaseChatModel, agent.CapabilityIdentity, error) {
	cfg := resolver.host.ModelConfigSnapshot()
	if err := config.ApplyAgentModelSelection(&cfg, "general", profileID, ""); err != nil {
		return nil, agent.CapabilityIdentity{}, err
	}
	resolved := config.ResolveAgentModel(&cfg, "general")
	modelConfig, err := modelio.ConfigFromResolved(resolved)
	if err != nil {
		return nil, agent.CapabilityIdentity{}, err
	}
	model, err := modelio.NewChatModel(ctx, modelConfig)
	if err != nil {
		return nil, agent.CapabilityIdentity{}, err
	}
	// Credential rotation does not change Session behavior. Model changes do.
	resolved.APIKey = ""
	resolved.Headers = nil
	encoded, err := json.Marshal(resolved)
	if err != nil {
		return nil, agent.CapabilityIdentity{}, err
	}
	digest := sha256.Sum256(encoded)
	return model, agent.CapabilityIdentity{Kind: "denova.platform.model", Version: 1, ConfigHash: hex.EncodeToString(digest[:])}, nil
}

// Models resolves extension model profiles using a detached native config snapshot.
type Models struct {
	host interface{ ModelConfigSnapshot() config.Config }
}

func NewModels(host interface{ ModelConfigSnapshot() config.Config }) *Models {
	return &Models{host: host}
}
