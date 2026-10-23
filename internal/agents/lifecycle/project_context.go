package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"denova/config"
	"denova/internal/book"

	agentcontext "github.com/alfredxw/denova/agent/context"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

type projectInstructionDefinition struct {
	resource string
	heading  string
	source   string
	purpose  string
}

var projectInstructionDefinitions = [...]projectInstructionDefinition{
	{
		resource: book.AgentInstructionsFileName,
		heading:  "# Project instructions",
		source:   "workspace AGENTS.md",
		purpose:  "apply stable workspace-level project and workflow instructions",
	},
	{
		resource: book.CreatorFileName,
		heading:  book.CreatorInstructionsHeading,
		source:   "workspace CREATOR.md",
		purpose:  "apply stable workspace-level creative instructions",
	},
}

// NewProjectInstructionsContextSource exposes root AGENTS.md and CREATOR.md as
// independently attributable stable instructions before conversation history.
func NewProjectInstructionsContextSource(cfg *config.Config, agentKind string, state *book.State) (agentcontext.ContextSource, error) {
	if state == nil {
		return nil, nil
	}
	agentKind = strings.TrimSpace(agentKind)
	if agentKind == "" {
		return nil, fmt.Errorf("project instruction context requires an Agent kind")
	}
	workspace := strings.TrimSpace(state.Workspace())
	projectID := ""
	if cfg != nil {
		projectID = strings.TrimSpace(cfg.ProjectID)
	}
	limit := config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes
	encoded, err := json.Marshal(struct {
		AgentKind string
		ProjectID string
		Resources []string
		Limit     int
	}{agentKind, projectID, []string{book.AgentInstructionsFileName, book.CreatorFileName}, limit})
	if err != nil {
		return nil, fmt.Errorf("encode project instruction context identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return projectInstructionsContextSource{
		files: book.NewService(workspace),
		limit: limit,
		identity: agentschema.CapabilityIdentity{
			Kind: "denova.project_instructions.context", Version: 3, ConfigHash: hex.EncodeToString(digest[:]),
		},
	}, nil
}

// StandingProjectInstructions reports whether the fragment is one of the root
// AGENTS.md or CREATOR.md files. They are author-owned rules for the whole
// session, not turn context, so an engine that owns a system-instruction channel
// receives them outside the per-turn text. Every other fragment keeps its
// existing placement.
func StandingProjectInstructions(fragment agentschema.ContextFragment) bool {
	if fragment.Stability != agentschema.ContextStablePrefix || fragment.Placement != agentschema.ContextLeadingMessage {
		return false
	}
	switch fragment.Resource {
	case book.AgentInstructionsFileName, book.CreatorFileName:
		return true
	default:
		return false
	}
}

type projectInstructionsContextSource struct {
	files    *book.Service
	limit    int
	identity agentschema.CapabilityIdentity
}

func (source projectInstructionsContextSource) Identity() agentschema.CapabilityIdentity {
	return source.identity
}

func (source projectInstructionsContextSource) Materialize(context.Context, agentcontext.ContextRequest) ([]agentschema.ContextFragment, error) {
	fragments := make([]agentschema.ContextFragment, 0, len(projectInstructionDefinitions))
	for _, definition := range projectInstructionDefinitions {
		raw, err := source.files.ReadFile(definition.resource)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read project instruction %s: %w", definition.resource, err)
		}
		body := strings.TrimSpace(raw)
		if body == "" {
			continue
		}
		content := book.ProjectInstructionsContent(definition.heading, body)
		if len(content) > source.limit {
			return nil, fmt.Errorf("%s exceeds the %d-byte project instruction limit", definition.resource, source.limit)
		}
		digest := sha256.Sum256([]byte(body))
		fragments = append(fragments, agentschema.ContextFragment{
			Source: definition.source, Purpose: definition.purpose,
			Resource: definition.resource, Revision: hex.EncodeToString(digest[:]),
			Stability: agentschema.ContextStablePrefix, Placement: agentschema.ContextLeadingMessage, Rendering: agentschema.ContextRenderVerbatim, Role: agentschema.User,
			Content: content, HardLimit: source.limit,
		})
	}
	return fragments, nil
}
