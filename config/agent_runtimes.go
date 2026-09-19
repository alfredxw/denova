package config

import (
	"errors"
	"fmt"
	"strings"
)

// RuntimeID identifies an execution engine, independently of the business
// Agent kind or a model provider. Engine implementations remain in internal.
type RuntimeID string

const (
	RuntimeNative RuntimeID = "native"
	RuntimeCodex  RuntimeID = "codex"
	RuntimeClaude RuntimeID = "claude"
)

var ErrInvalidAgentRuntime = errors.New("invalid Agent runtime configuration")

// CodexRuntimeSettings is one complete model and execution selection. Model-specific effort
// availability is checked by the internal runtime model catalog at execution.
type CodexRuntimeSettings struct {
	// ProfileID selects a Denova API model instead of a CLI-owned Model.
	ProfileID string `toml:"profile_id,omitempty" json:"profile_id,omitempty"`
	Model     string `toml:"model,omitempty" json:"model,omitempty"`
	Effort    string `toml:"effort,omitempty" json:"effort,omitempty"`
	// Sandbox belongs to the conversation and also constrains host tools.
	// Omission allows workspace writes; host paths never enter this setting.
	Sandbox CodexSandbox `toml:"sandbox,omitempty" json:"sandbox,omitempty"`
}

type CodexSandbox string

const (
	CodexReadOnly       CodexSandbox = "read-only"
	CodexWorkspaceWrite CodexSandbox = "workspace-write"
	CodexFullAccess     CodexSandbox = "danger-full-access"
)

func (settings CodexRuntimeSettings) EffectiveSandbox() CodexSandbox {
	if settings.Sandbox == "" {
		return CodexWorkspaceWrite
	}
	return settings.Sandbox
}

// ClaudeRuntimeSettings stores CLI model aliases and its own effort vocabulary.
// Empty effort delegates to the installed runtime default.
type ClaudeRuntimeSettings struct {
	ProfileID string `toml:"profile_id,omitempty" json:"profile_id,omitempty"`
	Model     string `toml:"model,omitempty" json:"model,omitempty"`
	Effort    string `toml:"effort,omitempty" json:"effort,omitempty"`
}

func (settings ClaudeRuntimeSettings) validate() error {
	return validateRuntimeModel(settings.Model, settings.ProfileID, settings.Effort)
}

// RuntimePreferences retains inactive engine settings across selection changes.
// Native settings stay in their existing fields and must not be duplicated here.
type RuntimePreferences struct {
	Selected RuntimeID              `toml:"selected,omitempty" json:"selected,omitempty"`
	Codex    *CodexRuntimeSettings  `toml:"codex,omitempty" json:"codex,omitempty"`
	Claude   *ClaudeRuntimeSettings `toml:"claude,omitempty" json:"claude,omitempty"`
}

// AgentRuntimeSettings deliberately has no default applying to game, image, or
// background agents. A missing role preference inherits the parent layer.
type AgentRuntimeSettings struct {
	IDE     *RuntimePreferences `toml:"ide,omitempty" json:"ide,omitempty"`
	General *RuntimePreferences `toml:"general,omitempty" json:"general,omitempty"`
}

// RuntimeSelection is the resolved, immutable engine branch of a conversation.
// An absent selection in a released conversation always means Native.
type RuntimeSelection struct {
	Kind   RuntimeID              `json:"kind"`
	Codex  *CodexRuntimeSettings  `json:"codex,omitempty"`
	Claude *ClaudeRuntimeSettings `json:"claude,omitempty"`
}

func (preferences RuntimePreferences) Validate() error {
	switch preferences.Selected {
	case "", RuntimeNative, RuntimeCodex, RuntimeClaude:
	default:
		return fmt.Errorf("%w: unknown selected runtime %q", ErrInvalidAgentRuntime, preferences.Selected)
	}
	if preferences.Codex != nil {
		if err := preferences.Codex.validate(); err != nil {
			return err
		}
	}
	if preferences.Claude != nil {
		return preferences.Claude.validate()
	}
	return nil
}

func (settings CodexRuntimeSettings) validate() error {
	switch settings.EffectiveSandbox() {
	case CodexReadOnly, CodexWorkspaceWrite, CodexFullAccess:
	default:
		return fmt.Errorf("%w: unknown execution sandbox %q", ErrInvalidAgentRuntime, settings.Sandbox)
	}
	return validateRuntimeModel(settings.Model, settings.ProfileID, settings.Effort)
}

func validateRuntimeModel(model, profileID, effort string) error {
	if profileID != "" {
		if strings.TrimSpace(profileID) != profileID || model != "" || effort != "" {
			return fmt.Errorf("%w: API profile selection cannot contain CLI model or effort", ErrInvalidAgentRuntime)
		}
		return nil
	}
	if strings.TrimSpace(model) == "" || model != strings.TrimSpace(model) {
		return fmt.Errorf("%w: runtime model must be a non-empty identifier without surrounding whitespace", ErrInvalidAgentRuntime)
	}
	if effort != strings.TrimSpace(effort) {
		return fmt.Errorf("%w: runtime effort contains surrounding whitespace", ErrInvalidAgentRuntime)
	}
	return nil
}

// ModelProfileID returns the active API reference. Empty means CLI configuration;
// credentials and endpoint addresses never belong in a persisted selection.
func (selection RuntimeSelection) ModelProfileID() string {
	switch selection.Kind {
	case RuntimeCodex:
		if selection.Codex != nil {
			return selection.Codex.ProfileID
		}
	case RuntimeClaude:
		if selection.Claude != nil {
			return selection.Claude.ProfileID
		}
	}
	return ""
}

func (selection RuntimeSelection) Validate(agentKind string) error {
	switch selection.Kind {
	case RuntimeNative:
		if selection.Codex != nil || selection.Claude != nil {
			return fmt.Errorf("%w: Native selection contains external settings", ErrInvalidAgentRuntime)
		}
	case RuntimeCodex:
		if agentKind != AgentKindIDE && agentKind != AgentKindGeneral {
			return fmt.Errorf("%w: external runtime does not support Agent kind %q", ErrInvalidAgentRuntime, agentKind)
		}
		if selection.Codex == nil || selection.Claude != nil {
			return fmt.Errorf("%w: Codex settings are required", ErrInvalidAgentRuntime)
		}
		return selection.Codex.validate()
	case RuntimeClaude:
		if agentKind != AgentKindIDE && agentKind != AgentKindGeneral {
			return fmt.Errorf("%w: external runtime does not support Agent kind %q", ErrInvalidAgentRuntime, agentKind)
		}
		if selection.Claude == nil || selection.Codex != nil {
			return fmt.Errorf("%w: Claude settings are required exclusively", ErrInvalidAgentRuntime)
		}
		return selection.Claude.validate()
	default:
		return fmt.Errorf("%w: unknown runtime %q", ErrInvalidAgentRuntime, selection.Kind)
	}
	return nil
}

// Selection projects only the active branch; retained inactive settings cannot
// enter execution input or invalidate the selected engine's context identity.
func (preferences RuntimePreferences) Selection(agentKind string) (RuntimeSelection, error) {
	kind := preferences.Selected
	if kind == "" {
		kind = RuntimeNative
	}
	selection := RuntimeSelection{Kind: kind}
	if kind == RuntimeCodex && preferences.Codex != nil {
		value := *preferences.Codex
		selection.Codex = &value
	}
	if kind == RuntimeClaude && preferences.Claude != nil {
		value := *preferences.Claude
		selection.Claude = &value
	}
	return selection, selection.Validate(agentKind)
}

// MergeAgentRuntimeSettings merges the selector and each engine independently.
// An engine model/effort object replaces its whole parent branch.
func MergeAgentRuntimeSettings(parent, child AgentRuntimeSettings) AgentRuntimeSettings {
	return AgentRuntimeSettings{
		IDE:     mergeRuntimePreferences(parent.IDE, child.IDE),
		General: mergeRuntimePreferences(parent.General, child.General),
	}
}

func mergeRuntimePreferences(parent, child *RuntimePreferences) *RuntimePreferences {
	if parent == nil && child == nil {
		return nil
	}
	merged := RuntimePreferences{}
	if parent != nil {
		merged = *parent
	}
	if child != nil {
		if child.Selected != "" {
			merged.Selected = child.Selected
		}
		if child.Claude != nil {
			merged.Claude = child.Claude
		}
		if child.Codex != nil {
			merged.Codex = child.Codex
		}
	}
	if merged.Codex != nil {
		value := *merged.Codex
		merged.Codex = &value
	}
	if merged.Claude != nil {
		value := *merged.Claude
		merged.Claude = &value
	}
	return &merged
}

func (settings AgentRuntimeSettings) ForAgent(agentKind string) RuntimePreferences {
	var preferences *RuntimePreferences
	switch agentKind {
	case AgentKindIDE:
		preferences = settings.IDE
	case AgentKindGeneral:
		preferences = settings.General
	}
	if preferences == nil {
		return RuntimePreferences{}
	}
	return *mergeRuntimePreferences(nil, preferences)
}

func (settings AgentRuntimeSettings) Validate() error {
	for _, preferences := range []*RuntimePreferences{settings.IDE, settings.General} {
		if preferences != nil {
			if err := preferences.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSettingsRuntimes(settings Settings) error {
	if err := settings.AgentRuntimes.Validate(); err != nil {
		return err
	}
	for _, definition := range settings.CustomAgents {
		if definition.Runtime == nil {
			continue
		}
		if err := definition.Runtime.Validate(); err != nil {
			return fmt.Errorf("custom Agent %q: %w", definition.ID, err)
		}
		kind := CustomAgentRuntimeKind(definition)
		if (definition.Runtime.Selected == RuntimeCodex || definition.Runtime.Selected == RuntimeClaude) && kind != AgentKindIDE && kind != AgentKindGeneral {
			return fmt.Errorf("%w: custom Agent %q does not support external execution", ErrInvalidAgentRuntime, definition.ID)
		}
	}
	return nil
}
