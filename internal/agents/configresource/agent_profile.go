package configresource

import (
	"context"
	"fmt"
	"strings"

	"denova/config"
)

const (
	agentProfileKindAgent           = "agent"
	agentProfileKindCustomAgent     = "custom_agent"
	agentProfileKindGeneralSubAgent = "general_sub_agent"
	agentProfileKindSubAgent        = "sub_agent"
	agentProfileSnapshotID          = "registry"
)

type agentProfileConfigValue struct {
	Kind        string                       `json:"kind,omitempty"`
	Model       *config.AgentModelOverride   `json:"model,omitempty"`
	Tools       *config.AgentToolOverride    `json:"tools,omitempty"`
	Prompt      *config.AgentPromptOverride  `json:"prompt,omitempty"`
	Skills      *config.AgentSkillOverride   `json:"skills,omitempty"`
	Context     *config.AgentContextOverride `json:"context,omitempty"`
	Enabled     *bool                        `json:"enabled,omitempty"`
	CustomAgent *config.CustomAgentConfig    `json:"custom_agent,omitempty"`
	SubAgent    *config.SubAgentConfig       `json:"sub_agent,omitempty"`
}

type agentProfileDeleteValue struct {
	Kind string `json:"kind"`
}

type agentProfileReadResult struct {
	ID        string                   `json:"id"`
	Revisions config.SettingsRevisions `json:"revisions"`
	Snapshot  agentConfigSnapshot      `json:"snapshot"`
}

func newAgentProfileResource(cfg *config.Config) Adapter {
	return configResourceAdapter{
		descriptor: Descriptor{
			Name: "agent_profile", Description: "Singleton registry snapshot for layered fixed Agent, custom Agent, capability, prompt, Skill, context, and SubAgent configuration; its read ID is registry.",
			Scopes: []string{"user", "workspace"}, Operations: configCRUDOperations(), RevisionField: "revision", Reference: "references/agent-profile.md",
		},
		list: func(_ context.Context, request ReadRequest) (any, error) {
			if err := validateAgentProfileReadRequest(request, false); err != nil {
				return nil, err
			}
			item, err := readAgentProfiles(cfg)
			if err != nil {
				return nil, err
			}
			return NewCatalog([]agentProfileReadResult{item}), nil
		},
		get: func(_ context.Context, request ReadRequest) (any, error) {
			if err := validateAgentProfileReadRequest(request, true); err != nil {
				return nil, err
			}
			return readAgentProfiles(cfg)
		},
		apply: func(_ context.Context, mutation Mutation) (any, error) {
			scope := strings.TrimSpace(mutation.Scope)
			if scope != "user" && scope != "workspace" {
				return nil, fmt.Errorf("agent_profile scope must be user or workspace")
			}
			var value agentProfileConfigValue
			if mutation.Operation == ApplyDelete {
				var deleteValue agentProfileDeleteValue
				if err := decodeConfigValue(mutation.Value, &deleteValue); err != nil {
					return nil, fmt.Errorf("agent_profile delete requires value.kind: %w", err)
				}
				value.Kind = strings.TrimSpace(deleteValue.Kind)
				if value.Kind == "" {
					return nil, fmt.Errorf("agent_profile delete requires value.kind to be agent, custom_agent, general_sub_agent, or sub_agent")
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
			if mutation.Operation == ApplyCreate {
				if strings.TrimSpace(mutation.Revision) == "" {
					return nil, fmt.Errorf("agent_profile create requires the latest %s scope revision", scope)
				}
				if kind != agentProfileKindSubAgent && kind != agentProfileKindCustomAgent {
					return nil, fmt.Errorf("agent_profile create is only valid for a new custom_agent or sub_agent; fixed Agent profiles use update")
				}
			}
			receiptID := strings.TrimSpace(mutation.ID)
			if receiptID == "" && value.CustomAgent != nil {
				receiptID = config.NormalizeCustomAgentID(value.CustomAgent.ID)
			} else if receiptID == "" && value.SubAgent != nil {
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
			mutate := config.MutateSettingsFile
			if scope == "user" {
				mutate = func(_ string, revision string, apply func(config.Settings) (config.Settings, error)) (string, error) {
					dataDir := ""
					if cfg != nil {
						dataDir = cfg.DataDir()
					}
					return config.MutateUserSettings(dataDir, revision, apply)
				}
			}
			revision, err := mutate(path, mutation.Revision, func(settings config.Settings) (config.Settings, error) {
				if err := applyAgentProfileMutation(&settings, layered, scope, kind, mutation, value); err != nil {
					return config.Settings{}, err
				}
				if kind == agentProfileKindAgent && value.Tools != nil {
					if err := validateAgentToolOverride(mutation.ID, *value.Tools); err != nil {
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

func validateAgentProfileReadRequest(request ReadRequest, exact bool) error {
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
		Paths: layered.Paths, Agents: agentConfigDefinitions(), AgentContracts: config.AgentContractDefinitions(), SubAgentParents: config.SubAgentParentKinds(),
		ToolCapabilities: agentConfigToolCapabilities(),
		Layers: agentConfigLayeredSnapshot{
			User: agentConfigLayer(layered.User), Workspace: agentConfigLayer(layered.Workspace), Effective: agentConfigLayer(layered.Effective),
		},
		SubAgentIndex: agentConfigSubAgentIndex(layered),
		Notes: []string{
			"custom Agents and model overrides are user-only; fixed Agent and SubAgent behavior may use user or workspace scope",
			"API keys and other secrets are never returned by this resource",
			"kind selects agent, custom_agent, general_sub_agent, or sub_agent",
			"custom_agent and sub_agent create require the latest target-scope revision; fixed profiles use update",
			"delete requires value.kind to disambiguate agent, custom_agent, general_sub_agent, or sub_agent",
		},
	}
	return agentProfileReadResult{ID: agentProfileSnapshotID, Revisions: layered.Revisions, Snapshot: snapshot}, nil
}

func applyAgentProfileMutation(settings *config.Settings, layered config.LayeredSettings, scope, kind string, mutation Mutation, value agentProfileConfigValue) error {
	id := strings.TrimSpace(mutation.ID)
	if id == "" && value.CustomAgent != nil {
		id = value.CustomAgent.ID
	} else if id == "" && value.SubAgent != nil {
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
		if mutation.Operation == ApplyDelete {
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
	case agentProfileKindCustomAgent:
		if scope != "user" {
			return fmt.Errorf("custom Agents are user-scoped")
		}
		return applyCustomAgentMutation(settings, layered, scope, id, mutation.Operation, value.CustomAgent)
	case agentProfileKindGeneralSubAgent:
		if !validGeneralSubAgentKey(id) {
			return fmt.Errorf("invalid general SubAgent parent %q", id)
		}
		if mutation.Operation == ApplyDelete {
			setGeneralSubAgentOverride(settings, id, nil)
			return nil
		}
		setGeneralSubAgentOverride(settings, id, value.Enabled)
		return nil
	case agentProfileKindSubAgent:
		if mutation.Operation == ApplyDelete {
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
		if mutation.Operation == ApplyCreate {
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

func applyCustomAgentMutation(
	settings *config.Settings,
	layered config.LayeredSettings,
	scope string,
	id string,
	operation string,
	value *config.CustomAgentConfig,
) error {
	id = config.NormalizeCustomAgentID(id)
	if operation == ApplyDelete {
		if id == "" {
			return fmt.Errorf("custom_agent delete requires id")
		}
		settings.CustomAgents = deleteCustomAgent(settings.CustomAgents, id)
		return nil
	}
	if value == nil {
		return fmt.Errorf("custom_agent create/update requires value.custom_agent")
	}
	agent := *value
	valueID := config.NormalizeCustomAgentID(agent.ID)
	if id != "" && valueID != "" && id != valueID {
		return fmt.Errorf("custom_agent id %q does not match mutation id %q", valueID, id)
	}
	if id == "" {
		id = valueID
	}
	if id == "" {
		return fmt.Errorf("custom_agent create/update requires a stable id")
	}
	agent.ID = id

	existing, exists := findCustomAgentByID(layered.Effective.CustomAgents, id)
	if operation == ApplyCreate {
		if exists {
			return fmt.Errorf("custom_agent %q already exists; use update", id)
		}
		if strings.TrimSpace(agent.Name) == "" {
			return fmt.Errorf("invalid custom Agent: name is required")
		}
		if _, ok := config.LookupAgentContract(agent.Contract); !ok {
			return fmt.Errorf("invalid custom Agent: contract must match one entry from agent_contracts")
		}
	} else {
		if !exists {
			return fmt.Errorf("custom_agent %q does not exist; use create", id)
		}
		if strings.TrimSpace(agent.Contract) != "" && strings.TrimSpace(agent.Contract) != existing.Contract {
			return fmt.Errorf("custom Agent contract is immutable: %q uses %q", id, existing.Contract)
		}
	}

	contractID := strings.TrimSpace(agent.Contract)
	if contractID == "" {
		contractID = existing.Contract
	}
	contract, ok := config.LookupAgentContract(contractID)
	if !ok {
		return fmt.Errorf("invalid custom Agent contract %q", contractID)
	}
	if err := validateAgentToolOverride(contract.RuntimeKind, agent.Tools); err != nil {
		return err
	}
	sanitized := config.SanitizeCustomAgents([]config.CustomAgentConfig{agent})
	if len(sanitized) != 1 {
		return fmt.Errorf("invalid custom Agent %q", id)
	}
	settings.CustomAgents = upsertCustomAgent(settings.CustomAgents, sanitized[0])
	return nil
}

func findCustomAgentByID(agents []config.CustomAgentConfig, id string) (config.CustomAgentConfig, bool) {
	id = config.NormalizeCustomAgentID(id)
	for _, agent := range agents {
		if config.NormalizeCustomAgentID(agent.ID) == id {
			return agent, true
		}
	}
	return config.CustomAgentConfig{}, false
}

func upsertCustomAgent(current []config.CustomAgentConfig, agent config.CustomAgentConfig) []config.CustomAgentConfig {
	id := config.NormalizeCustomAgentID(agent.ID)
	out := append([]config.CustomAgentConfig{}, current...)
	for index := range out {
		if config.NormalizeCustomAgentID(out[index].ID) == id {
			out[index] = agent
			return out
		}
	}
	return append(out, agent)
}

func deleteCustomAgent(current []config.CustomAgentConfig, id string) []config.CustomAgentConfig {
	id = config.NormalizeCustomAgentID(id)
	out := make([]config.CustomAgentConfig, 0, len(current))
	for _, agent := range current {
		if config.NormalizeCustomAgentID(agent.ID) != id {
			out = append(out, agent)
		}
	}
	return out
}

// validateAgentToolOverride rejects dormant capability names and capabilities
// outside one fixed Agent's ceiling before they can be persisted. The runtime
// ceiling remains authoritative after layering; this check keeps the editable
// configuration explicit and safe across future capability additions.
func validateAgentToolOverride(agentKind string, override config.AgentToolOverride) error {
	known := make(map[string]struct{}, len(config.AgentToolCapabilities()))
	for _, capability := range config.AgentToolCapabilities() {
		known[capability.Source] = struct{}{}
	}
	agentKind = strings.TrimSpace(agentKind)
	var ceiling map[string]struct{}
	if agentKind != "default" {
		definition, ok := config.LookupAgentKind(agentKind)
		if !ok {
			return fmt.Errorf("invalid agent kind %q", agentKind)
		}
		ceiling = make(map[string]struct{}, len(definition.ToolCapabilities))
		for _, capability := range definition.ToolCapabilities {
			ceiling[capability] = struct{}{}
		}
	}
	for capability := range override {
		if _, ok := known[capability]; !ok {
			return fmt.Errorf("unknown Agent capability %q", capability)
		}
		if ceiling != nil {
			if _, ok := ceiling[capability]; !ok {
				return fmt.Errorf("Agent %q does not support capability %q", agentKind, capability)
			}
		}
	}
	return nil
}
