package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	"denova/internal/agents/prompts"
	appagentruntime "denova/internal/app/agentruntime"
	"denova/internal/book"
)

// ErrWorkspaceRequired means a workspace-scoped settings mutation was
// requested while no Book is open.
var ErrWorkspaceRequired = errors.New("no workspace is open")

// Runtime is an immutable settings projection captured under the composition
// root lock. It prevents settings I/O and prompt construction from observing a
// mixture of two workspace generations.
type Runtime struct {
	Config            config.Config
	Workspace         string
	ProjectConfigPath string
	BookState         *book.State
}

// Host owns the process-local effects of a persisted settings mutation.
type Host interface {
	SettingsRuntime() Runtime
	ApplySettings(config.LayeredSettings, config.SettingsLayer)
}

// Service owns layered settings persistence and projection.
type Service struct{ host Host }

func NewService(host Host) *Service { return &Service{host: host} }

// Snapshot returns the canonical persisted layers, runtime URLs, and built-in
// Agent prompt projections for the current workspace generation.
func (service *Service) Snapshot() (config.LayeredSettings, error) {
	runtime := service.runtime()
	layered, err := config.LoadLayeredWithStartupConfigAt(
		runtime.Config.DataDir(), runtime.Workspace, runtime.ProjectConfigPath,
	)
	if err != nil {
		return config.LayeredSettings{}, err
	}
	if runtime.Config.RuntimeWebPort > 0 {
		layered.Access.LocalURL = config.LocalHTTPURL(runtime.Config.RuntimeWebPort)
		layered.Access.LANURL = config.LANHTTPURL(runtime.Config.RuntimeWebPort)
	}
	layered.Runtime.DevMode = runtime.Config.DevMode

	promptConfig := runtime.Config
	promptConfig.Workspace = runtime.Workspace
	ApplyLayer(&promptConfig, layered.User)
	ApplyLayer(&promptConfig, layered.Workspace)
	promptConfig.AgentPrompts = config.AgentPromptSettings{}
	teller := appagentruntime.WritingTellerForConfig(&promptConfig)
	layered.BuiltinAgentPrompts = prompts.BuiltinAgentPrompts(&promptConfig, runtime.BookState, teller)
	layered.BuiltinAgentPromptBlocks = prompts.BuiltinAgentPromptBlocks(&promptConfig, runtime.BookState, teller)
	layered.BuiltinAgentPromptSources = prompts.BuiltinAgentPromptSources(&promptConfig, runtime.BookState, teller)
	return layered, nil
}

// Patch applies a presence-aware partial mutation to exactly one persisted
// settings layer, then refreshes the process-local runtime from the canonical
// post-write snapshot.
func (service *Service) Patch(layer config.SettingsLayer, changes json.RawMessage, baseRevision string) (config.LayeredSettings, error) {
	switch layer {
	case config.SettingsLayerUser:
		return service.patchUser(changes, baseRevision)
	case config.SettingsLayerWorkspace:
		return service.patchWorkspace(changes, baseRevision)
	}
	return config.LayeredSettings{}, fmt.Errorf("%w: %q", config.ErrUnsupportedSettingsLayer, layer)
}

func (service *Service) patchUser(changes json.RawMessage, baseRevision string) (config.LayeredSettings, error) {
	runtime := service.runtime()
	path := config.UserConfigPath(runtime.Config.DataDir())
	if _, err := config.MutateSettingsFile(path, baseRevision, func(existing config.Settings) (config.Settings, error) {
		merged, err := config.ApplySettingsMergePatch(existing, changes)
		if err != nil {
			return config.Settings{}, err
		}
		return config.PrepareUserSettingsForWrite(existing, merged)
	}); err != nil {
		return config.LayeredSettings{}, err
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[app/settings] applied partial user settings mutation path=%s", path))
	return service.refresh(config.SettingsLayerUser)
}

func (service *Service) patchWorkspace(changes json.RawMessage, baseRevision string) (config.LayeredSettings, error) {
	if err := config.ValidateWorkspaceSettingsPatch(changes); err != nil {
		return config.LayeredSettings{}, err
	}
	runtime := service.runtime()
	if runtime.Workspace == "" {
		return config.LayeredSettings{}, ErrWorkspaceRequired
	}
	if runtime.ProjectConfigPath == "" {
		return config.LayeredSettings{}, fmt.Errorf("project config path is unavailable")
	}
	if _, err := config.MutateSettingsFile(runtime.ProjectConfigPath, baseRevision, func(existing config.Settings) (config.Settings, error) {
		merged, err := config.ApplySettingsMergePatch(existing, changes)
		if err != nil {
			return config.Settings{}, err
		}
		return config.PrepareWorkspaceAgentSettingsForWrite(existing, merged), nil
	}); err != nil {
		return config.LayeredSettings{}, err
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[app/settings] applied partial workspace settings mutation path=%s", runtime.ProjectConfigPath))
	return service.refresh(config.SettingsLayerWorkspace)
}

// EnsureAgentApprovalRule atomically adds one server-generated user rule. The
// deterministic rule ID makes retries idempotent while rejecting the extremely
// unlikely case where one ID names a different authorization boundary.
func (service *Service) EnsureAgentApprovalRule(rule config.AgentApprovalRule) (bool, error) {
	rules := config.NormalizeAgentApprovalRules([]config.AgentApprovalRule{rule})
	if err := config.ValidateAgentApprovalRules(rules); err != nil {
		return false, err
	}
	rule = rules[0]
	runtime := service.runtime()
	path := config.UserConfigPath(runtime.Config.DataDir())
	created := false
	if _, err := config.MutateSettingsFile(path, "", func(existing config.Settings) (config.Settings, error) {
		existing.AgentApprovalRules = config.NormalizeAgentApprovalRules(existing.AgentApprovalRules)
		for _, current := range existing.AgentApprovalRules {
			if current.ID != rule.ID {
				continue
			}
			if current.Scope != rule.Scope || current.ProjectID != rule.ProjectID ||
				current.Workspace != rule.Workspace || current.ToolName != rule.ToolName ||
				current.MatcherVersion != rule.MatcherVersion || current.CommandKey != rule.CommandKey ||
				current.CommandPattern != rule.CommandPattern {
				return config.Settings{}, fmt.Errorf("agent approval rule id %q is already bound to another command", rule.ID)
			}
			return config.PrepareUserSettingsForWrite(existing, existing)
		}
		existing.AgentApprovalRules = append(existing.AgentApprovalRules, rule)
		created = true
		return config.PrepareUserSettingsForWrite(existing, existing)
	}); err != nil {
		return created, err
	}
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[app/settings] persisted workspace command approval rule id=%s project_id=%s pattern=%q path=%s",
		rule.ID, rule.ProjectID, rule.CommandPattern, path,
	))
	_, err := service.refresh(config.SettingsLayerUser)
	return created, err
}

// RemoveAgentApprovalRule atomically revokes a user rule. It is also used to
// roll back a newly persisted rule when the corresponding pending ask cannot
// be resolved and therefore cannot execute.
func (service *Service) RemoveAgentApprovalRule(id string) (bool, config.LayeredSettings, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, config.LayeredSettings{}, fmt.Errorf("agent approval rule id is required")
	}
	runtime := service.runtime()
	path := config.UserConfigPath(runtime.Config.DataDir())
	removed := false
	if _, err := config.MutateSettingsFile(path, "", func(existing config.Settings) (config.Settings, error) {
		filtered := make([]config.AgentApprovalRule, 0, len(existing.AgentApprovalRules))
		for _, rule := range existing.AgentApprovalRules {
			if rule.ID == id {
				removed = true
				continue
			}
			filtered = append(filtered, rule)
		}
		existing.AgentApprovalRules = filtered
		return config.PrepareUserSettingsForWrite(existing, existing)
	}); err != nil {
		return removed, config.LayeredSettings{}, err
	}
	if removed {
		slog.InfoContext(context.Background(), fmt.Sprintf(
			"[app/settings] removed workspace command approval rule id=%s path=%s", id, path,
		))
	}
	layered, err := service.refresh(config.SettingsLayerUser)
	return removed, layered, err
}

func (service *Service) refresh(layer config.SettingsLayer) (config.LayeredSettings, error) {
	layered, err := service.Snapshot()
	if err != nil {
		return config.LayeredSettings{}, err
	}
	if service != nil && service.host != nil {
		service.host.ApplySettings(layered, layer)
	}
	return layered, nil
}

func (service *Service) runtime() Runtime {
	if service == nil || service.host == nil {
		return Runtime{}
	}
	return service.host.SettingsRuntime()
}
