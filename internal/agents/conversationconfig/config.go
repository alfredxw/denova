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
	AgentKind     string                   `json:"agent_kind"`
	ProfileID     string                   `json:"profile_id"`
	ThinkingLevel string                   `json:"thinking_level"`
	ApprovalMode  config.AgentApprovalMode `json:"approval_mode"`
}

// Snapshot adds the compare-and-swap revision used by the UI mutation seam.
type Snapshot struct {
	Config
	Revision uint64 `json:"revision"`
}

// Patch uses pointers so omitted fields stay unchanged. Null is intentionally
// invalid here because every conversation snapshot is fully resolved.
type Patch struct {
	ProfileID     *string                   `json:"profile_id,omitempty"`
	ThinkingLevel *string                   `json:"thinking_level,omitempty"`
	ApprovalMode  *config.AgentApprovalMode `json:"approval_mode,omitempty"`
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
	resolved := config.ResolveAgentModel(runtime, agentKind)
	profileID := strings.TrimSpace(resolved.ProfileID)
	if profileID == "" {
		profileID = "default"
	}
	return Config{
		AgentKind:     strings.TrimSpace(agentKind),
		ProfileID:     profileID,
		ThinkingLevel: strings.TrimSpace(resolved.ThinkingLevel),
		ApprovalMode:  config.NormalizeAgentApprovalMode(runtimeApprovalMode(runtime)),
	}
}

// Merge returns a validated complete snapshot without mutating the base.
func Merge(runtime *config.Config, base Config, patch Patch) (Config, error) {
	next := base
	if patch.ProfileID != nil {
		next.ProfileID = strings.TrimSpace(*patch.ProfileID)
	}
	if patch.ThinkingLevel != nil {
		next.ThinkingLevel = strings.TrimSpace(*patch.ThinkingLevel)
	}
	if patch.ApprovalMode != nil {
		next.ApprovalMode = *patch.ApprovalMode
	}
	if err := Validate(runtime, next, base.AgentKind); err != nil {
		return Config{}, err
	}
	return next, nil
}

// Validate enforces both the per-conversation vocabulary and the currently
// available model-profile catalog.
func Validate(runtime *config.Config, candidate Config, agentKind string) error {
	agentKind = strings.TrimSpace(agentKind)
	if err := ValidateShape(candidate, agentKind); err != nil {
		return err
	}
	clone := cloneRuntimeConfig(runtime)
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
	if strings.TrimSpace(candidate.ProfileID) == "" {
		return errors.New("conversation model profile is required")
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
	if err := Validate(runtime, selection, selection.AgentKind); err != nil {
		return err
	}
	if err := config.ApplyAgentModelSelection(runtime, selection.AgentKind, selection.ProfileID, selection.ThinkingLevel); err != nil {
		return err
	}
	runtime.AgentApprovalMode = config.NormalizeAgentApprovalMode(selection.ApprovalMode)
	return nil
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
