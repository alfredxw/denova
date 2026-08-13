package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/book"

	agent "github.com/alfredxw/denova/agent"
)

const projectInstructionsResource = "CREATOR.md"

// NewProjectInstructionContextSource exposes CREATOR.md as one stable project
// instruction before conversation history. Provenance remains host metadata;
// the model sees only a short fixed heading and the project-owned content.
func NewProjectInstructionContextSource(cfg *config.Config, agentKind string, state *book.State) (agent.ContextSource, error) {
	if state == nil {
		return nil, nil
	}
	agentKind = strings.TrimSpace(agentKind)
	if agentKind == "" {
		return nil, fmt.Errorf("project instruction context requires an Agent kind")
	}
	workspace := strings.TrimSpace(state.Workspace())
	limit := config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes
	encoded, err := json.Marshal(struct {
		AgentKind string
		Workspace string
		Resource  string
		Limit     int
	}{agentKind, workspace, projectInstructionsResource, limit})
	if err != nil {
		return nil, fmt.Errorf("encode project instruction context identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return projectInstructionContextSource{
		state: state,
		limit: limit,
		identity: agent.CapabilityIdentity{
			Kind: "denova.project_instructions.context", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
		},
	}, nil
}

type projectInstructionContextSource struct {
	state    *book.State
	limit    int
	identity agent.CapabilityIdentity
}

func (source projectInstructionContextSource) Identity() agent.CapabilityIdentity {
	return source.identity
}

func (source projectInstructionContextSource) Materialize(context.Context, agent.ContextRequest) ([]agent.ContextFragment, error) {
	body := strings.TrimSpace(source.state.ReadCreatorPrompt())
	if body == "" {
		return nil, nil
	}
	content := "# Project instructions\n\n" + body + "\n\nA later explicit user request takes precedence."
	if len(content) > source.limit {
		return nil, fmt.Errorf("%s exceeds the %d-byte project instruction limit", projectInstructionsResource, source.limit)
	}
	digest := sha256.Sum256([]byte(body))
	return []agent.ContextFragment{{
		Source: "workspace CREATOR.md", Purpose: "apply stable workspace-level creative instructions",
		Resource: projectInstructionsResource, Revision: hex.EncodeToString(digest[:]),
		Placement: agent.ContextLeadingMessage, Rendering: agent.ContextRenderVerbatim, Role: agent.User,
		Content: content, HardLimit: source.limit,
	}}, nil
}
