package tools

import (
	"context"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/configresources"
)

const (
	agentProfileKindAgent           = "agent"
	agentProfileKindGeneralSubAgent = "general_sub_agent"
	agentProfileKindSubAgent        = "sub_agent"
	agentProfileSnapshotID          = "registry"
)

type agentProfileConfigValue struct {
	Kind     string                       `json:"kind,omitempty"`
	Model    *config.AgentModelOverride   `json:"model,omitempty"`
	Tools    *config.AgentToolOverride    `json:"tools,omitempty"`
	Prompt   *config.AgentPromptOverride  `json:"prompt,omitempty"`
	Skills   *config.AgentSkillOverride   `json:"skills,omitempty"`
	Context  *config.AgentContextOverride `json:"context,omitempty"`
	Enabled  *bool                        `json:"enabled,omitempty"`
	SubAgent *config.SubAgentConfig       `json:"sub_agent,omitempty"`
}

type agentProfileDeleteValue struct {
	Kind string `json:"kind"`
}

type agentProfileReadResult struct {
	ID        string                   `json:"id"`
	Revisions config.SettingsRevisions `json:"revisions"`
	Snapshot  agentConfigSnapshot      `json:"snapshot"`
}

func newAgentProfileResource(cfg *config.Config) configresources.Adapter {
	return configResourceAdapter{
		descriptor: configresources.Descriptor{
			Name: "agent_profile", Description: "Singleton registry snapshot for layered model, capability, prompt, Skill, context, and SubAgent configuration; its read ID is registry.",
			Scopes: []string{"user", "workspace"}, Operations: configCRUDOperations(), RevisionField: "revision", Reference: "references/agent-profile.md",
		},
		list: func(_ context.Context, request configresources.ReadRequest) (any, error) {
			if err := validateAgentProfileReadRequest(request, false); err != nil {
				return nil, err
			}
			return readAgentProfiles(cfg)
		},
		get: func(_ context.Context, request configresources.ReadRequest) (any, error) {
			if err := validateAgentProfileReadRequest(request, true); err != nil {
				return nil, err
			}
			return readAgentProfiles(cfg)
		},
		apply: func(_ context.Context, mutation configresources.Mutation) (any, error) {
			scope := strings.TrimSpace(mutation.Scope)
			if scope != "user" && scope != "workspace" {
				return nil, fmt.Errorf("agent_profile scope must be user or workspace")
			}
			var value agentProfileConfigValue
			if mutation.Operation == configresources.ApplyDelete {
				var deleteValue agentProfileDeleteValue
				if err := decodeConfigValue(mutation.Value, &deleteValue); err != nil {
					return nil, fmt.Errorf("agent_profile delete requires value.kind: %w", err)
				}
				value.Kind = strings.TrimSpace(deleteValue.Kind)
				if value.Kind == "" {
					return nil, fmt.Errorf("agent_profile delete requires value.kind to be agent, general_sub_agent, or sub_agent")
				}
			} else {
				if err := decodeConfigValue(mutation.Value, &value); err != nil {
					return nil, err
				}
			}
			kind := strings.TrimSpace(value.Kind)
			if kind == "" {
				kind = agentProfileKindAgent
			}
			if mutation.Operation == configresources.ApplyCreate {
				if strings.TrimSpace(mutation.Revision) == "" {
					return nil, fmt.Errorf("agent_profile create requires the latest %s scope revision", scope)
				}
				if kind != agentProfileKindSubAgent {
					return nil, fmt.Errorf("agent_profile create is only valid for a new sub_agent; fixed Agent profiles use update")
				}
			}
			receiptID := strings.TrimSpace(mutation.ID)
			if receiptID == "" && value.SubAgent != nil {
				receiptID = config.NormalizeSubAgentID(value.SubAgent.ID)
			}
			path, err := writableAgentConfigPath(cfg, scope)
			if err != nil {
				return nil, err
			}
			layered, err := loadAgentConfigLayered(cfg)
			if err != nil {
				return nil, err
			}
			revision, err := config.MutateSettingsFile(path, mutation.Revision, func(settings config.Settings) (config.Settings, error) {
				if err := applyAgentProfileMutation(&settings, layered, scope, kind, mutation, value); err != nil {
					return config.Settings{}, err
				}
				if agentProfileMutationAffectsConfigManagerTools(kind, mutation, value) {
					if err := validateConfigManagerRequestedToolOverride(mutation, value); err != nil {
						return config.Settings{}, err
					}
					if err := validateConfigManagerToolCeiling(layered, scope, settings); err != nil {
						return config.Settings{}, err
					}
				}
				return settings, nil
			})
			if err != nil {
				return nil, err
			}
			return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: receiptID, Revision: revision}, nil
		},
	}
}

func validateAgentProfileReadRequest(request configresources.ReadRequest, exact bool) error {
	if strings.TrimSpace(request.Scope) != "" {
		return fmt.Errorf("agent_profile reads use the singleton %q snapshot and do not accept scope; select user or workspace only on config_apply", agentProfileSnapshotID)
	}
	if strings.TrimSpace(request.Query) != "" {
		return fmt.Errorf("agent_profile snapshot does not support query")
	}
	if !exact {
		if len(normalizeConfigIDs(request.IDs)) != 0 {
			return fmt.Errorf("agent_profile list does not accept ids; use get with id %q", agentProfileSnapshotID)
		}
		return nil
	}
	ids := normalizeConfigIDs(request.IDs)
	if len(ids) != 1 || ids[0] != agentProfileSnapshotID {
		return fmt.Errorf("agent_profile get requires the exact singleton id %q", agentProfileSnapshotID)
	}
	return nil
}

func readAgentProfiles(cfg *config.Config) (agentProfileReadResult, error) {
	layered, err := loadAgentConfigLayered(cfg)
	if err != nil {
		return agentProfileReadResult{}, err
	}
	snapshot := agentConfigSnapshot{
		Paths: layered.Paths, Agents: agentConfigDefinitions(), SubAgentParents: config.SubAgentParentKinds(),
		ToolCapabilities: agentConfigToolCapabilities(),
		Layers: agentConfigLayeredSnapshot{
			User: agentConfigLayer(layered.User), Workspace: agentConfigLayer(layered.Workspace), Effective: agentConfigLayer(layered.Effective),
		},
		SubAgentIndex: agentConfigSubAgentIndex(layered),
		Notes: []string{
			"scope must be user or workspace; model overrides are user-only",
			"API keys and other secrets are never returned by this resource",
			"kind selects agent, general_sub_agent, or sub_agent",
			"sub_agent create requires the latest target-scope revision; fixed profiles use update",
			"delete requires value.kind to disambiguate agent, general_sub_agent, or sub_agent",
		},
	}
	return agentProfileReadResult{ID: agentProfileSnapshotID, Revisions: layered.Revisions, Snapshot: snapshot}, nil
}

func applyAgentProfileMutation(settings *config.Settings, layered config.LayeredSettings, scope, kind string, mutation configresources.Mutation, value agentProfileConfigValue) error {
	id := strings.TrimSpace(mutation.ID)
	if id == "" && value.SubAgent != nil {
		id = value.SubAgent.ID
	}
	switch kind {
	case agentProfileKindAgent:
		if !validAgentConfigKey(id) {
			return fmt.Errorf("invalid agent kind %q", id)
		}
		if scope == "workspace" && value.Model != nil {
			return fmt.Errorf("agent model selection is user-scoped")
		}
		if mutation.Operation == configresources.ApplyDelete {
			setAgentModelOverride(settings, id, config.AgentModelOverride{})
			setAgentToolOverride(settings, id, config.AgentToolOverride{})
			setAgentPromptOverride(settings, id, config.AgentPromptOverride{})
			setAgentSkillOverride(settings, id, config.AgentSkillOverride{})
			setAgentContextOverride(settings, id, config.AgentContextOverride{})
			return nil
		}
		if value.Model == nil && value.Tools == nil && value.Prompt == nil && value.Skills == nil && value.Context == nil {
			return fmt.Errorf("agent_profile value must include model, tools, prompt, skills, or context")
		}
		if value.Model != nil {
			setAgentModelOverride(settings, id, *value.Model)
		}
		if value.Tools != nil {
			setAgentToolOverride(settings, id, *value.Tools)
		}
		if value.Prompt != nil {
			setAgentPromptOverride(settings, id, *value.Prompt)
		}
		if value.Skills != nil {
			setAgentSkillOverride(settings, id, *value.Skills)
		}
		if value.Context != nil {
			setAgentContextOverride(settings, id, *value.Context)
		}
		return nil
	case agentProfileKindGeneralSubAgent:
		if !validGeneralSubAgentKey(id) {
			return fmt.Errorf("invalid general SubAgent parent %q", id)
		}
		if mutation.Operation == configresources.ApplyDelete {
			setGeneralSubAgentOverride(settings, id, nil)
			return nil
		}
		setGeneralSubAgentOverride(settings, id, value.Enabled)
		return nil
	case agentProfileKindSubAgent:
		if mutation.Operation == configresources.ApplyDelete {
			id = config.NormalizeSubAgentID(id)
			if id == "" {
				return fmt.Errorf("sub_agent delete requires id")
			}
			settings.SubAgents = deleteSubAgent(settings.SubAgents, id)
			return nil
		}
		if value.SubAgent == nil {
			return fmt.Errorf("sub_agent create/update requires value.sub_agent")
		}
		sub := *value.SubAgent
		if strings.TrimSpace(sub.ID) == "" {
			sub.ID = id
		}
		sub.ID = config.NormalizeSubAgentID(sub.ID)
		if mutation.Operation == configresources.ApplyCreate {
			if sub.ID == "" {
				return fmt.Errorf("sub_agent create requires a stable id")
			}
			if _, exists := findSubAgentByID(layered.Effective.SubAgents, sub.ID); exists {
				return fmt.Errorf("sub_agent %q already exists; use update", sub.ID)
			}
		}
		sub = fillSubAgentRequiredFields(sub, settings.SubAgents, layered.Effective.SubAgents)
		sanitized := config.SanitizeSubAgents([]config.SubAgentConfig{sub})
		if len(sanitized) != 1 {
			return fmt.Errorf("invalid SubAgent: id, description, and system_prompt are required")
		}
		settings.SubAgents = upsertSubAgent(settings.SubAgents, sanitized[0])
		return nil
	default:
		return fmt.Errorf("invalid agent_profile kind %q", kind)
	}
}

var configManagerToolCeiling = map[string]struct{}{
	config.AgentToolWorkspaceRead: {},
	config.AgentToolAsk:           {},
	config.AgentToolSkills:        {},
	config.AgentToolConfigRead:    {},
	config.AgentToolConfigApply:   {},
}

func agentProfileMutationAffectsConfigManagerTools(kind string, mutation configresources.Mutation, value agentProfileConfigValue) bool {
	if kind != agentProfileKindAgent {
		return false
	}
	id := strings.TrimSpace(mutation.ID)
	if id != "default" && id != config.AgentKindConfigManager {
		return false
	}
	return mutation.Operation == configresources.ApplyDelete || value.Tools != nil
}

// validateConfigManagerRequestedToolOverride is intentionally fail-closed for
// unknown capability names. Otherwise a dormant key written today could become
// an escalation when a future release registers a capability with that name.
func validateConfigManagerRequestedToolOverride(mutation configresources.Mutation, value agentProfileConfigValue) error {
	if strings.TrimSpace(mutation.ID) != config.AgentKindConfigManager || value.Tools == nil {
		return nil
	}
	for capability, enabled := range *value.Tools {
		if !enabled {
			continue
		}
		if _, allowed := configManagerToolCeiling[capability]; allowed {
			continue
		}
		return fmt.Errorf("Config Manager cannot enable capability %q through agent_profile / 配置 Agent 不允许通过 agent_profile 自行启用能力 %q", capability, capability)
	}
	return nil
}

// validateConfigManagerToolCeiling evaluates the post-mutation layered config,
// not just the submitted map. This prevents deleting a restrictive workspace
// override or replacing it with a sparse map from revealing a sensitive
// capability inherited from another layer.
func validateConfigManagerToolCeiling(layered config.LayeredSettings, scope string, nextLayer config.Settings) error {
	user := layered.User
	workspace := layered.Workspace
	switch scope {
	case "user":
		user = nextLayer
	case "workspace":
		workspace = nextLayer
	default:
		return fmt.Errorf("agent_profile scope must be user or workspace")
	}
	effective := config.Merge(config.Merge(config.Merge(layered.Default, layered.Global), user), workspace)
	resolved := config.ResolveAgentTools(&config.Config{AgentTools: effective.AgentTools}, config.AgentKindConfigManager)
	for _, capability := range config.AgentToolCapabilities() {
		if !resolved.Allows(capability.Source) {
			continue
		}
		if _, allowed := configManagerToolCeiling[capability.Source]; allowed {
			continue
		}
		return fmt.Errorf("Config Manager cannot enable capability %q through agent_profile / 配置 Agent 不允许通过 agent_profile 自行启用能力 %q", capability.Source, capability.Source)
	}
	return nil
}
