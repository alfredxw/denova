package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

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
