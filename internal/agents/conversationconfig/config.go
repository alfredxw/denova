// Package conversationconfig owns the durable, per-conversation Agent runtime
// selection shared by writing, AgentChat, configuration, and game adapters.
package conversationconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	"github.com/alfredxw/denova/agent/providers"
)

var (
	ErrRevisionConflict = errors.New("conversation configuration revision conflict")
	ErrNotInitialized   = errors.New("conversation configuration is not initialized")
)

// Config is a complete snapshot. A conversation never follows later Settings
// changes implicitly after this value has been initialized.
type Config struct {
	AgentKind     string                    `json:"agent_kind"`
	CustomAgentID string                    `json:"custom_agent_id,omitempty"`
	CustomAgent   *config.CustomAgentConfig `json:"custom_agent,omitempty"`
	ProfileID     string                    `json:"profile_id"`
	ThinkingLevel string                    `json:"thinking_level"`
	ApprovalMode  config.AgentApprovalMode  `json:"approval_mode"`
	Runtime       *config.RuntimeSelection  `json:"runtime,omitempty"`
}

// Snapshot adds the compare-and-swap revision used by the UI mutation seam.
type Snapshot struct {
	Config
	Revision uint64 `json:"revision"`
}

// Patch uses pointers so omitted fields stay unchanged. Null is intentionally
// invalid here because every conversation snapshot is fully resolved.
type Patch struct {
	CustomAgentID *string                   `json:"custom_agent_id,omitempty"`
	ProfileID     *string                   `json:"profile_id,omitempty"`
	ThinkingLevel *string                   `json:"thinking_level,omitempty"`
	ApprovalMode  *config.AgentApprovalMode `json:"approval_mode,omitempty"`
	Runtime       *config.RuntimeSelection  `json:"runtime,omitempty"`
	// Codex replaces the current Codex model/effort together without switching
	// engines or Agent identity. It also applies to an uncommitted conversation.
	Codex  *config.CodexRuntimeSettings  `json:"codex,omitempty"`
	Claude *config.ClaudeRuntimeSettings `json:"claude,omitempty"`
}

// UnmarshalJSON preserves the semantic difference between omitted and null.
// Conversation snapshots are fully resolved, so null is never a valid clear
// operation and must not be silently treated as an omitted field.
func (patch *Patch) UnmarshalJSON(data []byte) error {
	if patch == nil {
		return errors.New("conversation config patch is nil")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		if err == nil {
			err = errors.New("changes must be a JSON object")
		}
		return fmt.Errorf("invalid conversation config changes: %w", err)
	}
	var next Patch
	for field, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("conversation config field %q cannot be null", field)
		}
		switch field {
		case "codex":
			var value config.CodexRuntimeSettings
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&value); err != nil {
				return fmt.Errorf("invalid conversation Codex model settings: %w", err)
			}
			next.Codex = &value
		case "claude":
			var value config.ClaudeRuntimeSettings
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&value); err != nil {
				return fmt.Errorf("invalid conversation Claude model settings: %w", err)
			}
			next.Claude = &value
		case "runtime":
			var value config.RuntimeSelection
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&value); err != nil {
				return fmt.Errorf("invalid conversation runtime: %w", err)
			}
			next.Runtime = &value
		case "custom_agent_id":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("invalid conversation config field %q: %w", field, err)
			}
			next.CustomAgentID = &value
		case "profile_id":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("invalid conversation config field %q: %w", field, err)
			}
			next.ProfileID = &value
		case "thinking_level":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("invalid conversation config field %q: %w", field, err)
			}
			next.ThinkingLevel = &value
		case "approval_mode":
			var value config.AgentApprovalMode
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("invalid conversation config field %q: %w", field, err)
			}
			next.ApprovalMode = &value
		default:
			return fmt.Errorf("unknown conversation config field %q", field)
		}
	}
	*patch = next
	return nil
}

// Default resolves the bootstrap snapshot from Settings for a specific Agent.
func Default(runtime *config.Config, agentKind string) Config {
	resolved, _ := DefaultWithCustomAgent(runtime, agentKind, "")
	return resolved
}

// DefaultWithCustomAgent resolves a new conversation snapshot from one
// optional custom Agent instance. Existing conversations keep their persisted
// snapshot and never follow later default-selection changes implicitly.
func DefaultWithCustomAgent(runtime *config.Config, agentKind, customAgentID string) (Config, error) {
	clone := cloneRuntimeConfig(runtime)
	var customAgent *config.CustomAgentConfig
	if normalized := config.NormalizeCustomAgentID(customAgentID); normalized != "" {
		definition, ok := config.FindCustomAgent(&clone, normalized)
		if !ok {
			return Config{}, fmt.Errorf("%w: %s", config.ErrCustomAgentNotFound, normalized)
		}
		customAgent = cloneCustomAgent(definition)
	}
	if err := config.ApplyCustomAgent(&clone, agentKind, customAgentID); err != nil {
		return Config{}, err
	}
	preferences := clone.AgentRuntimes.ForAgent(agentKind)
	if customAgent != nil {
		preferences = config.RuntimePreferences{}
		if customAgent.Runtime != nil {
			preferences = *customAgent.Runtime
		}
	}
	selection, err := preferences.Selection(agentKind)
	if err != nil {
		return Config{}, err
	}
	var external *config.RuntimeSelection
	if selection.Kind != config.RuntimeNative {
		external = &selection
	}
	resolved := config.ResolveAgentModel(&clone, agentKind)
	profileID := strings.TrimSpace(resolved.ProfileID)
	if profileID == "" {
		profileID = "default"
	}
	return Config{
		AgentKind:     strings.TrimSpace(agentKind),
		CustomAgentID: config.NormalizeCustomAgentID(customAgentID),
		CustomAgent:   customAgent,
		ProfileID:     profileID,
		ThinkingLevel: strings.TrimSpace(resolved.ThinkingLevel),
		ApprovalMode:  config.NormalizeAgentApprovalMode(runtimeApprovalMode(runtime)),
		Runtime:       external,
	}, nil
}

// Merge returns a validated complete snapshot without mutating the base.
func Merge(runtime *config.Config, base Config, patch Patch) (Config, error) {
	next := base
	if patch.Codex != nil {
		if patch.Claude != nil || base.Engine().Kind != config.RuntimeCodex || patch.Runtime != nil || patch.CustomAgentID != nil {
			return Config{}, ErrRuntimeCapabilityUnsupported
		}
		value := *patch.Codex
		next.Runtime = &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &value}
	}
	if patch.Claude != nil {
		if base.Engine().Kind != config.RuntimeClaude || patch.Runtime != nil || patch.CustomAgentID != nil {
			return Config{}, ErrRuntimeCapabilityUnsupported
		}
		value := *patch.Claude
		next.Runtime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &value}
	}
	if patch.Runtime != nil {
		value := cloneSelection(*patch.Runtime)
		next.Runtime = &value
	}
	if next.Engine().Kind != config.RuntimeNative && (patch.ProfileID != nil || patch.ThinkingLevel != nil || patch.ApprovalMode != nil) {
		return Config{}, ErrRuntimeCapabilityUnsupported
	}
	if patch.Runtime != nil && patch.CustomAgentID != nil && config.NormalizeCustomAgentID(*patch.CustomAgentID) != base.CustomAgentID {
		return Config{}, errors.New("conversation Agent identity cannot change with runtime selection")
	}
	if patch.CustomAgentID != nil {
		customAgentID := config.NormalizeCustomAgentID(*patch.CustomAgentID)
		if customAgentID != base.CustomAgentID {
			defaults, err := DefaultWithCustomAgent(runtime, base.AgentKind, customAgentID)
			if err != nil {
				return Config{}, err
			}
			next.ProfileID = defaults.ProfileID
			next.ThinkingLevel = defaults.ThinkingLevel
			next.CustomAgent = defaults.CustomAgent
		}
		next.CustomAgentID = customAgentID
	}
	if patch.ProfileID != nil {
		next.ProfileID = strings.TrimSpace(*patch.ProfileID)
	}
	if patch.ThinkingLevel != nil {
		next.ThinkingLevel = strings.TrimSpace(*patch.ThinkingLevel)
	}
	if patch.ApprovalMode != nil {
		next.ApprovalMode = *patch.ApprovalMode
	}
	validate := ValidatePersisted
	if patch.CustomAgentID != nil && config.NormalizeCustomAgentID(*patch.CustomAgentID) != base.CustomAgentID {
		validate = Validate
	}
	if err := validate(runtime, next, base.AgentKind); err != nil {
		return Config{}, err
	}
	return next, nil
}

// Validate enforces both the per-conversation vocabulary and the currently
// available model-profile catalog.
func Validate(runtime *config.Config, candidate Config, agentKind string) error {
	return validate(runtime, candidate, agentKind, false)
}

// ValidatePersisted accepts an archived custom Agent only when its durable
// identity is already present on a conversation or branch.
func ValidatePersisted(runtime *config.Config, candidate Config, agentKind string) error {
	return validate(runtime, candidate, agentKind, true)
}

func validate(runtime *config.Config, candidate Config, agentKind string, persisted bool) error {
	agentKind = strings.TrimSpace(agentKind)
	if err := ValidateShape(candidate, agentKind); err != nil {
		return err
	}
	clone := cloneRuntimeConfig(runtime)
	if err := applyConversationCustomAgent(&clone, agentKind, candidate, persisted); err != nil {
		return err
	}
	if candidate.Engine().Kind != config.RuntimeNative {
		if candidate.Engine().ModelProfileID() != "" {
			_, err := config.ResolveRuntimeModel(&clone, candidate.Engine())
			return err
		}
		return nil
	}
	if err := config.ApplyAgentModelSelection(&clone, agentKind, candidate.ProfileID, candidate.ThinkingLevel); err != nil {
		return err
	}
	if _, err := config.ParseAgentApprovalMode(string(candidate.ApprovalMode)); err != nil {
		return err
	}
	return nil
}

// ValidateShape checks the provider-neutral persisted vocabulary without
// requiring access to the current model-profile catalog.
func ValidateShape(candidate Config, agentKind string) error {
	agentKind = strings.TrimSpace(agentKind)
	if agentKind == "" || strings.TrimSpace(candidate.AgentKind) != agentKind {
		return fmt.Errorf("conversation Agent kind must be %q", agentKind)
	}
	if normalized := config.NormalizeCustomAgentID(candidate.CustomAgentID); normalized != strings.TrimSpace(candidate.CustomAgentID) {
		return fmt.Errorf("invalid custom Agent ID %q", candidate.CustomAgentID)
	}
	if candidate.CustomAgentID == "" && candidate.CustomAgent != nil {
		return errors.New("conversation custom Agent snapshot requires custom_agent_id")
	}
	if candidate.CustomAgent != nil {
		if config.NormalizeCustomAgentID(candidate.CustomAgent.ID) != candidate.CustomAgentID {
			return errors.New("conversation custom Agent snapshot ID does not match custom_agent_id")
		}
		if config.CustomAgentRuntimeKind(*candidate.CustomAgent) != agentKind {
			return fmt.Errorf("conversation custom Agent snapshot does not satisfy Agent kind %q", agentKind)
		}
	}
	if strings.TrimSpace(candidate.ProfileID) == "" {
		if candidate.Engine().Kind == config.RuntimeNative {
			return errors.New("conversation model profile is required")
		}
	}
	if err := candidate.Engine().Validate(agentKind); err != nil {
		return err
	}
	if candidate.Engine().Kind != config.RuntimeNative {
		return nil
	}
	if _, err := providers.ParseThinkingLevel(candidate.ThinkingLevel); err != nil {
		return err
	}
	if _, err := config.ParseAgentApprovalMode(string(candidate.ApprovalMode)); err != nil {
		return err
	}
	return nil
}

// Apply injects a validated snapshot into a request-local Config immediately
// before the Agent builder reads model and tool-approval policy.
func Apply(runtime *config.Config, selection Config) error {
	if runtime == nil {
		return errors.New("runtime config is nil")
	}
	if err := ValidatePersisted(runtime, selection, selection.AgentKind); err != nil {
		return err
	}
	if err := applyConversationCustomAgent(runtime, selection.AgentKind, selection, true); err != nil {
		return err
	}
	engine := selection.Engine()
	runtime.ActiveAgentRuntime = &engine
	if engine.Kind != config.RuntimeNative {
		return nil
	}
	if err := config.ApplyAgentModelSelection(runtime, selection.AgentKind, selection.ProfileID, selection.ThinkingLevel); err != nil {
		return err
	}
	runtime.AgentApprovalMode = config.NormalizeAgentApprovalMode(selection.ApprovalMode)
	return nil
}

func applyConversationCustomAgent(runtime *config.Config, agentKind string, selection Config, persisted bool) error {
	if selection.CustomAgent != nil {
		return config.ApplyCustomAgentDefinition(runtime, agentKind, *selection.CustomAgent, persisted)
	}
	if persisted {
		return config.ApplyPersistedCustomAgent(runtime, agentKind, selection.CustomAgentID)
	}
	return config.ApplyCustomAgent(runtime, agentKind, selection.CustomAgentID)
}

func cloneCustomAgent(value config.CustomAgentConfig) *config.CustomAgentConfig {
	encoded, err := json.Marshal(value)
	if err != nil {
		clone := value
		return &clone
	}
	var clone config.CustomAgentConfig
	if err := json.Unmarshal(encoded, &clone); err != nil {
		clone = value
	}
	return &clone
}

func runtimeApprovalMode(runtime *config.Config) config.AgentApprovalMode {
	if runtime == nil {
		return config.AgentApprovalAsk
	}
	return runtime.AgentApprovalMode
}

func cloneRuntimeConfig(runtime *config.Config) config.Config {
	if runtime == nil {
		return config.Config{}
	}
	return *runtime
}
