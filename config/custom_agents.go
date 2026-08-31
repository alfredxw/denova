package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrCustomAgentNotFound = errors.New("custom Agent is unavailable")

const (
	AgentSkillPolicyManaged  = "managed"
	AgentSkillPolicyExplicit = "explicit"

	AgentContextSlotStable  = "stable"
	AgentContextSlotSession = "session"
	AgentContextSlotTurn    = "turn"

	AgentDelegationCompatible = "compatible"
	AgentDelegationSelected   = "selected"
	AgentDelegationDisabled   = "disabled"
)

// AgentSkillPolicy keeps the default case sparse as the Skill catalog grows.
// Managed follows each Skill's audience metadata; Explicit admits only Pinned.
// Blocked always wins and Pinned Skills are advertised to the model eagerly.
type AgentSkillPolicy struct {
	Mode    string   `toml:"mode,omitempty" json:"mode,omitempty"`
	Pinned  []string `toml:"pinned,omitempty" json:"pinned,omitempty"`
	Blocked []string `toml:"blocked,omitempty" json:"blocked,omitempty"`
}

// AgentContextBinding is one user-authored context source with an explicit
// lifecycle slot. Runtime provenance and placement are derived centrally.
type AgentContextBinding struct {
	ID             string `toml:"id,omitempty" json:"id,omitempty"`
	Name           string `toml:"name,omitempty" json:"name,omitempty"`
	Purpose        string `toml:"purpose,omitempty" json:"purpose,omitempty"`
	Slot           string `toml:"slot,omitempty" json:"slot,omitempty"`
	Content        string `toml:"content,omitempty" json:"content,omitempty"`
	HardLimitBytes int    `toml:"hard_limit_bytes,omitempty" json:"hard_limit_bytes,omitempty"`
}

// AgentDelegationPolicy selects from the existing delegated Agent registry.
// Compatible preserves audience-based routing, Selected uses only AgentIDs,
// and Disabled removes delegation from the resolved runtime.
type AgentDelegationPolicy struct {
	Mode     string   `toml:"mode,omitempty" json:"mode,omitempty"`
	AgentIDs []string `toml:"agent_ids,omitempty" json:"agent_ids,omitempty"`
}

// CustomAgentConfig is a complete user-owned Agent definition. Contract is a
// stable runtime boundary, not a live inheritance link to a built-in Agent.
// New definitions are cloned from a built-in template by the UI, after which
// every field below resolves independently from later template changes.
type CustomAgentConfig struct {
	ID                string                `toml:"id,omitempty" json:"id,omitempty"`
	Name              string                `toml:"name,omitempty" json:"name,omitempty"`
	Description       string                `toml:"description,omitempty" json:"description,omitempty"`
	Contract          string                `toml:"contract,omitempty" json:"contract,omitempty"`
	Enabled           *bool                 `toml:"enabled,omitempty" json:"enabled,omitempty"`
	Instructions      string                `toml:"instructions,omitempty" json:"instructions,omitempty"`
	Model             AgentModelOverride    `toml:"model,omitempty" json:"model,omitempty"`
	Tools             AgentToolOverride     `toml:"tools,omitempty" json:"tools,omitempty"`
	ToolGuidance      map[string]string     `toml:"tool_guidance,omitempty" json:"tool_guidance,omitempty"`
	SkillPolicy       AgentSkillPolicy      `toml:"skill_policy,omitempty" json:"skill_policy,omitempty"`
	RuntimeContext    AgentContextOverride  `toml:"runtime_context,omitempty" json:"runtime_context,omitempty"`
	ContextBindings   []AgentContextBinding `toml:"context_bindings,omitempty" json:"context_bindings,omitempty"`
	Delegation        AgentDelegationPolicy `toml:"delegation,omitempty" json:"delegation,omitempty"`
	ImageAPIProfileID string                `toml:"image_api_profile_id,omitempty" json:"image_api_profile_id,omitempty"`
}

// ResolvedAgentDefinition is the safe inspection projection returned by the
// settings API. Runtime-only Engine and Experience fragments are reported by
// the prompt-source catalog and never become editable fields here.
type ResolvedAgentDefinition struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Contract        string                `json:"contract"`
	RuntimeKind     string                `json:"runtime_kind"`
	Revision        string                `json:"revision,omitempty"`
	Instructions    string                `json:"instructions,omitempty"`
	ToolGuidance    map[string]string     `json:"tool_guidance,omitempty"`
	SkillPolicy     AgentSkillPolicy      `json:"skill_policy"`
	ContextBindings []AgentContextBinding `json:"context_bindings,omitempty"`
	Delegation      AgentDelegationPolicy `json:"delegation"`
}

var customizableAgentKinds = []string{AgentKindGeneral, AgentKindIDE, AgentKindInteractiveStory, AgentKindImage}

func CustomizableAgentKinds() []string { return append([]string(nil), customizableAgentKinds...) }

func IsCustomizableAgentKind(kind string) bool {
	_, ok := AgentContractForRuntimeKind(strings.TrimSpace(kind))
	return ok
}

func NormalizeCustomAgentID(id string) string { return NormalizeSubAgentID(id) }

func CustomAgentEnabled(agent CustomAgentConfig) bool { return boolValue(agent.Enabled, true) }

func CustomAgentRuntimeKind(agent CustomAgentConfig) string {
	definition, ok := LookupAgentContract(agent.Contract)
	if !ok {
		return ""
	}
	return definition.RuntimeKind
}

// MergeCustomAgents replaces complete definitions by stable identity. Custom
// Agents are not sparse overlays: a higher settings layer owns every field.
func MergeCustomAgents(parent, child []CustomAgentConfig) []CustomAgentConfig {
	if len(child) == 0 {
		return parent
	}
	out := make([]CustomAgentConfig, 0, len(parent)+len(child))
	index := make(map[string]int, len(parent)+len(child))
	for _, item := range SanitizeCustomAgents(parent) {
		index[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range SanitizeCustomAgents(child) {
		if position, ok := index[item.ID]; ok {
			out[position] = item
			continue
		}
		index[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func SanitizeCustomAgents(agents []CustomAgentConfig) []CustomAgentConfig {
	if len(agents) == 0 {
		return agents
	}
	out := make([]CustomAgentConfig, 0, len(agents))
	seen := make(map[string]bool, len(agents))
	for _, item := range agents {
		item.ID = NormalizeCustomAgentID(item.ID)
		if item.ID == "" || IsReservedAgentID(item.ID) || seen[item.ID] {
			continue
		}
		item.Name = strings.TrimSpace(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		item.Contract = strings.TrimSpace(item.Contract)
		if item.Contract != "" {
			if _, ok := LookupAgentContract(item.Contract); !ok {
				continue
			}
		}
		item.Instructions = strings.TrimSpace(item.Instructions)
		item.Model.ProfileID = normalizeModelProfileID(item.Model.ProfileID)
		if item.Model.ThinkingLevel != "" {
			item.Model.ThinkingLevel = normalizeThinkingLevel(item.Model.ThinkingLevel)
		}
		item.Tools = cloneAgentToolOverride(item.Tools)
		item.ToolGuidance = sanitizeToolGuidance(item.ToolGuidance)
		item.SkillPolicy = sanitizeAgentSkillPolicy(item.SkillPolicy)
		item.RuntimeContext = sanitizeAgentContextOverride(item.RuntimeContext)
		item.ContextBindings = sanitizeAgentContextBindings(item.ContextBindings)
		item.Delegation = sanitizeAgentDelegationPolicy(item.Delegation)
		item.ImageAPIProfileID = strings.TrimSpace(item.ImageAPIProfileID)
		seen[item.ID] = true
		out = append(out, item)
	}
	return out
}

func FindCustomAgent(cfg *Config, id string) (CustomAgentConfig, bool) {
	return findCustomAgent(cfg, id, false)
}

func FindActiveCustomAgent(cfg *Config) (CustomAgentConfig, bool) {
	if cfg == nil || strings.TrimSpace(cfg.ActiveCustomAgentID) == "" {
		return CustomAgentConfig{}, false
	}
	return findCustomAgent(cfg, cfg.ActiveCustomAgentID, true)
}

func findCustomAgent(cfg *Config, id string, includeDisabled bool) (CustomAgentConfig, bool) {
	id = NormalizeCustomAgentID(id)
	if cfg == nil || id == "" {
		return CustomAgentConfig{}, false
	}
	for _, item := range cfg.CustomAgents {
		if item.ID != id || (!includeDisabled && !CustomAgentEnabled(item)) {
			continue
		}
		if strings.TrimSpace(item.Name) == "" || CustomAgentRuntimeKind(item) == "" {
			return CustomAgentConfig{}, false
		}
		return item, true
	}
	return CustomAgentConfig{}, false
}

func ApplyCustomAgent(cfg *Config, runtimeKind, id string) error {
	return applyCustomAgent(cfg, runtimeKind, id, false)
}

func ApplyPersistedCustomAgent(cfg *Config, runtimeKind, id string) error {
	return applyCustomAgent(cfg, runtimeKind, id, true)
}

// ApplyCustomAgentDefinition projects a complete definition onto one
// request-local Config. It is also used by persisted conversation snapshots.
func ApplyCustomAgentDefinition(cfg *Config, runtimeKind string, item CustomAgentConfig, includeDisabled bool) error {
	if cfg == nil {
		return errors.New("runtime config is nil")
	}
	sanitized := SanitizeCustomAgents([]CustomAgentConfig{item})
	if len(sanitized) != 1 || strings.TrimSpace(sanitized[0].Name) == "" {
		return fmt.Errorf("%w: %s", ErrCustomAgentNotFound, item.ID)
	}
	item = sanitized[0]
	if !includeDisabled && !CustomAgentEnabled(item) {
		return fmt.Errorf("%w: %s", ErrCustomAgentNotFound, item.ID)
	}
	if CustomAgentRuntimeKind(item) != strings.TrimSpace(runtimeKind) {
		return fmt.Errorf("custom Agent %q uses contract %q for %q, not %q", item.ID, item.Contract, CustomAgentRuntimeKind(item), runtimeKind)
	}
	definition, ok := LookupAgentKind(runtimeKind)
	if !ok || definition.SetModelOverride == nil || definition.SetToolOverride == nil ||
		definition.SetPromptOverride == nil || definition.SetContextOverride == nil {
		return fmt.Errorf("custom Agent contract runtime %q is unsupported", runtimeKind)
	}

	// Reset behavior settings to product defaults before projecting this
	// complete definition. Later built-in Agent edits cannot change it.
	cfg.AgentModels = AgentModelSettings{}
	cfg.AgentTools = AgentToolSettings{}
	cfg.AgentPrompts = AgentPromptSettings{}
	cfg.AgentSkills = AgentSkillSettings{}
	cfg.AgentContexts = AgentContextSettings{}
	definition.SetModelOverride(&cfg.AgentModels, item.Model)
	definition.SetToolOverride(&cfg.AgentTools, item.Tools)
	definition.SetPromptOverride(&cfg.AgentPrompts, AgentPromptOverride{FlowPrompt: item.Instructions})
	definition.SetContextOverride(&cfg.AgentContexts, item.RuntimeContext)
	if runtimeKind == AgentKindImage && item.ImageAPIProfileID != "" {
		cfg.DefaultImageAPIProfileID = item.ImageAPIProfileID
	}
	if item.Delegation.Mode == AgentDelegationDisabled {
		override := definition.ToolOverride(cfg.AgentTools)
		if override == nil {
			override = AgentToolOverride{}
		}
		override[AgentToolDelegation] = false
		definition.SetToolOverride(&cfg.AgentTools, override)
	}
	cfg.CustomAgents = upsertRuntimeCustomAgent(cfg.CustomAgents, item)
	cfg.ActiveCustomAgentID = item.ID
	cfg.ActiveCustomAgentName = item.Name
	cfg.ActiveCustomAgentRevision = CustomAgentRevision(item)
	return nil
}

func applyCustomAgent(cfg *Config, runtimeKind, id string, includeDisabled bool) error {
	id = NormalizeCustomAgentID(id)
	if id == "" {
		if cfg != nil {
			cfg.ActiveCustomAgentID = ""
			cfg.ActiveCustomAgentName = ""
			cfg.ActiveCustomAgentRevision = ""
		}
		return nil
	}
	if cfg == nil {
		return errors.New("runtime config is nil")
	}
	item, ok := findCustomAgent(cfg, id, includeDisabled)
	if !ok {
		return fmt.Errorf("%w: %s", ErrCustomAgentNotFound, id)
	}
	return ApplyCustomAgentDefinition(cfg, runtimeKind, item, includeDisabled)
}

func CustomAgentRevision(item CustomAgentConfig) string {
	sanitized := SanitizeCustomAgents([]CustomAgentConfig{item})
	if len(sanitized) == 1 {
		item = sanitized[0]
	}
	encoded, _ := json.Marshal(item)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func ResolveAgentDefinitions(agents []CustomAgentConfig) map[string]ResolvedAgentDefinition {
	resolved := make(map[string]ResolvedAgentDefinition, len(agents)+len(agentContractRegistry))
	for _, contract := range agentContractRegistry {
		resolved[contract.RuntimeKind] = ResolvedAgentDefinition{
			ID: contract.RuntimeKind, Contract: contract.ID, RuntimeKind: contract.RuntimeKind,
			SkillPolicy: AgentSkillPolicy{Mode: AgentSkillPolicyManaged},
			Delegation:  AgentDelegationPolicy{Mode: AgentDelegationCompatible},
		}
	}
	for _, item := range SanitizeCustomAgents(agents) {
		if strings.TrimSpace(item.Name) == "" || !CustomAgentEnabled(item) {
			continue
		}
		resolved[item.ID] = ResolvedAgentDefinition{
			ID: item.ID, Name: item.Name, Contract: item.Contract, RuntimeKind: CustomAgentRuntimeKind(item),
			Revision: CustomAgentRevision(item), Instructions: item.Instructions,
			ToolGuidance: mergeStringMap(nil, item.ToolGuidance), SkillPolicy: item.SkillPolicy,
			ContextBindings: append([]AgentContextBinding(nil), item.ContextBindings...), Delegation: item.Delegation,
		}
	}
	return resolved
}

func ResolveActiveAgentSkillPolicy(cfg *Config, runtimeKind string) (string, map[string]bool, []string) {
	mode := AgentSkillPolicyManaged
	overrides := ResolveAgentSkillOverrides(cfg, runtimeKind)
	var pinned []string
	if item, ok := FindActiveCustomAgent(cfg); ok {
		policy := sanitizeAgentSkillPolicy(item.SkillPolicy)
		mode = policy.Mode
		overrides = make(map[string]bool, len(policy.Pinned)+len(policy.Blocked))
		for _, name := range policy.Pinned {
			overrides[name] = true
		}
		for _, name := range policy.Blocked {
			overrides[name] = false
		}
		return mode, overrides, append([]string(nil), policy.Pinned...)
	}
	for name, enabled := range overrides {
		if enabled {
			pinned = append(pinned, name)
		}
	}
	return mode, overrides, normalizedUniqueStrings(pinned, func(value string) string { return strings.TrimSpace(value) })
}

func sanitizeAgentSkillPolicy(policy AgentSkillPolicy) AgentSkillPolicy {
	policy.Mode = strings.TrimSpace(policy.Mode)
	if policy.Mode != AgentSkillPolicyExplicit {
		policy.Mode = AgentSkillPolicyManaged
	}
	policy.Pinned = normalizedUniqueStrings(policy.Pinned, func(value string) string { return strings.TrimSpace(value) })
	policy.Blocked = normalizedUniqueStrings(policy.Blocked, func(value string) string { return strings.TrimSpace(value) })
	blocked := make(map[string]bool, len(policy.Blocked))
	for _, name := range policy.Blocked {
		blocked[name] = true
	}
	filtered := policy.Pinned[:0]
	for _, name := range policy.Pinned {
		if !blocked[name] {
			filtered = append(filtered, name)
		}
	}
	policy.Pinned = filtered
	return policy
}

func sanitizeAgentDelegationPolicy(policy AgentDelegationPolicy) AgentDelegationPolicy {
	policy.Mode = strings.TrimSpace(policy.Mode)
	switch policy.Mode {
	case AgentDelegationSelected, AgentDelegationDisabled:
	default:
		policy.Mode = AgentDelegationCompatible
	}
	policy.AgentIDs = normalizedUniqueStrings(policy.AgentIDs, NormalizeSubAgentID)
	return policy
}

func sanitizeAgentContextBindings(bindings []AgentContextBinding) []AgentContextBinding {
	out := make([]AgentContextBinding, 0, len(bindings))
	seen := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		binding.ID = NormalizeSubAgentID(binding.ID)
		binding.Name = strings.TrimSpace(binding.Name)
		binding.Purpose = strings.TrimSpace(binding.Purpose)
		binding.Content = strings.TrimSpace(binding.Content)
		binding.Slot = strings.TrimSpace(binding.Slot)
		// Keep an empty binding as an editable draft. Runtime assembly skips it
		// until content exists, so autosave never makes a newly added row vanish.
		if binding.ID == "" || seen[binding.ID] {
			continue
		}
		switch binding.Slot {
		case AgentContextSlotSession, AgentContextSlotTurn:
		default:
			binding.Slot = AgentContextSlotStable
		}
		if binding.Name == "" {
			binding.Name = binding.ID
		}
		if binding.Purpose == "" {
			binding.Purpose = "apply user-authored Agent context"
		}
		binding.HardLimitBytes = resolvedPositiveLimit(&binding.HardLimitBytes, DefaultAgentContextMaxFragmentBytes, MaxAgentContextFragmentBytes)
		if len(binding.Content) > binding.HardLimitBytes && len(binding.Content) <= MaxAgentContextFragmentBytes {
			binding.HardLimitBytes = len(binding.Content)
		}
		seen[binding.ID] = true
		out = append(out, binding)
	}
	return out
}

func sanitizeToolGuidance(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for name, guidance := range values {
		name = strings.TrimSpace(name)
		guidance = strings.TrimSpace(guidance)
		if name != "" && guidance != "" {
			out[name] = guidance
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneAgentToolOverride(values AgentToolOverride) AgentToolOverride {
	if len(values) == 0 {
		return values
	}
	out := make(AgentToolOverride, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func mergeStringMap(parent, child map[string]string) map[string]string {
	out := make(map[string]string, len(parent)+len(child))
	for key, value := range parent {
		out[key] = value
	}
	for key, value := range child {
		out[key] = value
	}
	return out
}

func normalizedUniqueStrings(values []string, normalize func(string) string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = normalize(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func upsertRuntimeCustomAgent(values []CustomAgentConfig, item CustomAgentConfig) []CustomAgentConfig {
	out := append([]CustomAgentConfig(nil), values...)
	for index := range out {
		if out[index].ID == item.ID {
			out[index] = item
			return out
		}
	}
	return append(out, item)
}
