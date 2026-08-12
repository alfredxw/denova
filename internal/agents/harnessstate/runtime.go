package harnessstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

// ContextSource materializes the live prompt and only fragments explicitly
// targeted at agentKind. Its capability identity is intentionally independent
// of file contents: State changes update the next model prefix without turning
// State into a versioned Agent definition.
func (h Harness) ContextSource(cfg *config.Config, agentKind string) agent.ContextSource {
	fragments := make([]agent.ContextFragment, 0, len(h.contexts)+1)
	limit := config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes
	if prompt := strings.TrimSpace(h.Prompt(agentKind)); prompt != "" {
		fragments = append(fragments, agent.ContextFragment{
			Source: "Denova User State", Purpose: "apply user-managed Agent behavior and preferences without overriding runtime or tool contracts",
			Resource:  "prompts/" + strings.TrimSpace(agentKind) + ".md",
			Placement: agent.ContextLeadingMessage, Rendering: agent.ContextRenderAttributed,
			Role: agent.System, Content: prompt, HardLimit: limit,
		})
	}
	for _, fragment := range h.contexts {
		if !contains(fragment.Agents, agentKind) {
			continue
		}
		fragments = append(fragments, agent.ContextFragment{
			Source: "Denova User State", Purpose: fragment.Purpose,
			Resource:  fragment.Resource,
			Placement: fragment.Placement, Rendering: agent.ContextRenderAttributed,
			Role: agent.System, Content: fragment.Content, HardLimit: limit,
		})
	}
	return staticContextSource{
		identity: identity("denova.harness_state.context", struct {
			AgentKind string
		}{agentKind}),
		fragments: fragments,
	}
}

// ApplyToolDescriptions wraps only Tool.Info. The implementation, schema,
// descriptor, permissions, and enabled set remain exactly those assembled by
// the owning Agent.
func (h Harness) ApplyToolDescriptions(definitions []agent.ToolDefinition) []agent.ToolDefinition {
	if len(h.toolDescriptions) == 0 {
		return append([]agent.ToolDefinition(nil), definitions...)
	}
	result := make([]agent.ToolDefinition, len(definitions))
	for index, definition := range definitions {
		result[index] = definition
		if definition.Tool == nil {
			continue
		}
		info, err := definition.Tool.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		if description, ok := h.toolDescriptions[info.Name]; ok {
			result[index].Tool = describedTool{Tool: definition.Tool, description: description}
		}
	}
	return result
}

type describedTool struct {
	agent.Tool
	description string
}

func (tool describedTool) Info(ctx context.Context) (*agent.ToolInfo, error) {
	info, err := tool.Tool.Info(ctx)
	if err != nil || info == nil {
		return info, err
	}
	cloned := *info
	cloned.Desc = tool.description
	return &cloned, nil
}

type staticContextSource struct {
	identity  agent.CapabilityIdentity
	fragments []agent.ContextFragment
}

func (source staticContextSource) Identity() agent.CapabilityIdentity { return source.identity }

func (source staticContextSource) Materialize(context.Context, agent.ContextRequest) ([]agent.ContextFragment, error) {
	return append([]agent.ContextFragment(nil), source.fragments...), nil
}

func identity(kind string, configuration any) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(configuration)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: strings.TrimSpace(kind), Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

func ValidateToolDescriptions(ctx context.Context, harness Harness, definitions []agent.ToolDefinition) error {
	for _, definition := range harness.ApplyToolDescriptions(definitions) {
		if err := definition.Validate(ctx); err != nil {
			return fmt.Errorf("validate Harness State tool description: %w", err)
		}
	}
	return nil
}
