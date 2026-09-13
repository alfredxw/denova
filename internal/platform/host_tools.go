package platform

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"sort"
	"sync"

	"denova/config"
	agent "github.com/alfredxw/denova/agent"
)

// HostAgentTools reads the installed plugins and shared settings for a new
// execution. It persists nothing; calls remain bound to the caller's Project
// and conversation. Existing executions finish with their already loaded tools.
func (m *Manager) HostAgentTools(cfg *config.Config, agentKind string) (agent.Toolset, error) {
	if cfg == nil || cfg.AgentPluginScope == (config.AgentPluginScope{}) {
		return nil, nil
	}
	scope := Scope{ProjectID: cfg.ProjectID, SessionID: cfg.AgentPluginScope.SessionID, StoryID: cfg.AgentPluginScope.StoryID, BranchID: cfg.AgentPluginScope.BranchID}
	switch {
	case scope.ProjectID == "":
		return nil, failure("INVALID_ARGUMENT", "Plugin tools require a Project")
	case scope.SessionID != "" && scope.StoryID == "" && scope.BranchID == "":
		scope.Kind = "session"
	case scope.SessionID == "" && scope.StoryID != "" && scope.BranchID != "":
		scope.Kind = "story"
	default:
		return nil, failure("INVALID_ARGUMENT", "Plugin tools require a Product Session or Story branch")
	}
	if _, _, err := m.registry.Resolve(scope.ProjectID, true); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items, err := m.List(Plugin)
	if err != nil {
		return nil, err
	}
	manifest := Manifest{}
	providers := []string{}
	for _, item := range items {
		if !item.Enabled || item.Removed {
			continue
		}
		release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: item.ID}, ReleaseID: item.CurrentRelease})
		if err != nil {
			return nil, err
		}
		if len(release.Manifest.Contributes.Tools) == 0 {
			continue
		}
		// An unavailable optional extension must not prevent ordinary Agent
		// work. The Extensions catalog exposes the same dependency failure.
		if _, err := m.resolveDependencies(release.Manifest, nil); err != nil {
			slog.Warn("platform_agent_plugin_unavailable", "plugin", item.ID, "error", err)
			continue
		}
		if !slices.Contains(release.Grants, "tools.invoke") {
			slog.Warn("platform_agent_plugin_unavailable", "plugin", item.ID, "reason", "tools.invoke permission is missing")
			continue
		}
		contributions := make([]string, 0, len(release.Manifest.Contributes.Tools))
		for _, tool := range release.Manifest.Contributes.Tools {
			contributions = append(contributions, tool.ID)
		}
		providers = append(providers, item.ID)
		manifest.Requires = append(manifest.Requires, Dependency{PluginID: item.ID, VersionRange: "=" + release.Manifest.Version, Contributions: contributions})
	}
	if len(providers) == 0 {
		return nil, nil
	}
	dependencies, err := m.resolveDependencies(manifest, nil)
	if err != nil {
		return nil, err
	}
	releases := make(map[string]Release, len(dependencies))
	settings := make(map[string]map[string]any, len(dependencies))
	for _, dependency := range dependencies {
		release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: dependency.PluginID}, ReleaseID: dependency.ReleaseID})
		if err != nil {
			return nil, err
		}
		values, err := m.settingsValues(release, "installed", nil)
		if err != nil {
			return nil, err
		}
		releases[dependency.PluginID], settings[dependency.PluginID] = release, values
	}
	profileID := config.ResolveAgentModel(cfg, agentKind).ProfileID
	if profileID == "" {
		profileID = "default"
	}
	return &hostPluginToolset{manager: m, providers: providers, releases: releases, settings: settings, scope: scope, models: map[string]string{"builtin/assistant": profileID}}, nil
}

type hostPluginToolset struct {
	manager   *Manager
	providers []string
	releases  map[string]Release
	settings  map[string]map[string]any
	scope     Scope
	models    map[string]string
}

func (t *hostPluginToolset) Identity() agent.CapabilityIdentity {
	// Reuse the Agent's behavior check so a paused tool batch cannot silently
	// resume against changed implementations or shared settings.
	configuration := make(map[string]any, len(t.releases))
	for id, release := range t.releases {
		configuration[id] = []any{release.Ref.ReleaseID, t.settings[id]}
	}
	raw, _ := json.Marshal(configuration)
	return agent.CapabilityIdentity{Kind: "denova.plugin.tools", Version: 1, ConfigHash: stableID(string(raw))}
}

func (t *hostPluginToolset) PrepareTools(ctx context.Context, request agent.ToolRequest) ([]agent.ToolDefinition, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	invocation := &hostPluginInvocation{toolset: t, owner: ctx, releases: t.releases, settings: t.settings, runtimes: map[string]*Runtime{}}
	definitions := []agent.ToolDefinition{}
	for _, id := range t.providers {
		release := t.releases[id]
		for _, tool := range release.Manifest.Contributes.Tools {
			definition, err := t.manager.agentTool(release, tool, t.settings[id], func(ctx context.Context, input json.RawMessage) (ToolResult, error) {
				return invocation.invoke(ctx, id, tool.ID, input)
			})
			if err != nil {
				return nil, err
			}
			definitions = append(definitions, definition)
		}
	}
	return definitions, nil
}

// One prepared run owns its lazy processes. Tools cannot restart a runtime
// after cancellation or permission revocation using an old prepared binding.
type hostPluginInvocation struct {
	toolset  *hostPluginToolset
	owner    context.Context
	releases map[string]Release
	settings map[string]map[string]any
	mu       sync.Mutex
	runtimes map[string]*Runtime
}

func (call *hostPluginInvocation) invoke(ctx context.Context, provider, tool string, input json.RawMessage) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	call.mu.Lock()
	runtime, err := call.runtime(provider)
	call.mu.Unlock()
	if err != nil {
		return ToolResult{}, err
	}
	if runtime.ctx.Err() != nil {
		return ToolResult{}, failure("RUNTIME_UNAVAILABLE", "Plugin runtime was stopped; start a new task")
	}
	linked, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(runtime.ctx, cancel)
	defer stop()
	defer cancel()
	return runtime.invokeTool(linked, runtime.owner, provider, tool, input, authorizedAgentToolInvocation)
}

func (call *hostPluginInvocation) runtime(provider string) (*Runtime, error) {
	if runtime := call.runtimes[provider]; runtime != nil {
		return runtime, nil
	}
	if err := call.owner.Err(); err != nil {
		return nil, err
	}
	m := call.toolset.manager
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	// A disabled package may finish an admitted run; removal or changed grants
	// revoke that admission even if its first tool has not started a process yet.
	for id, admitted := range call.releases {
		current, installed, err := m.release(admitted.Ref)
		if err != nil {
			return nil, err
		}
		if installed.Removed || !slices.Equal(current.Grants, admitted.Grants) {
			return nil, failure("PERMISSION_DENIED", "Plugin %s authorization changed", id)
		}
	}
	release := call.releases[provider]
	// Resolve just this provider's graph using the already loaded releases.
	pins := []DependencyPin{}
	var add func(Manifest)
	seen := map[string]bool{}
	add = func(manifest Manifest) {
		for _, dep := range manifest.Requires {
			if !seen[dep.PluginID] {
				seen[dep.PluginID] = true
				bound := call.releases[dep.PluginID]
				pins = append(pins, DependencyPin{PluginID: dep.PluginID, ReleaseID: bound.Ref.ReleaseID})
				add(bound.Manifest)
			}
		}
	}
	add(release.Manifest)
	sort.Slice(pins, func(i, j int) bool { return pins[i].PluginID < pins[j].PluginID })
	id := "agent-" + randomToken()
	runtime, err := m.startRuntime(id, release, call.toolset.scope, pins, RuntimeConfiguration{frozenSettings: call.settings}, call.toolset.models, false, OpenOptions{hostOnly: true})
	if err != nil {
		return nil, err
	}
	m.runtimes[id], call.runtimes[provider] = runtime, runtime
	context.AfterFunc(call.owner, func() {
		launch("platform_agent_cleanup", func() {}, func() {
			if err := m.Stop(context.Background(), id); err != nil {
				slog.Error("platform_agent_cleanup_failed", "runtime", id, "error", err)
			}
		})
	})
	return runtime, nil
}
