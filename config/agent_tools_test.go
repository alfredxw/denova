package config

import "testing"

func TestResolveAgentToolsDefaults(t *testing.T) {
	tests := []struct {
		kind    string
		allowed []string
		denied  []string
	}{
		{
			kind: AgentKindIDE,
			allowed: []string{
				AgentToolFilesystemRead, AgentToolWorkspaceWrite, AgentToolShell,
				AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser, AgentToolAsk,
				AgentToolTodo, AgentToolGoal, AgentToolSkills, AgentToolDelegation, AgentToolLoreRead,
				AgentToolLoreWrite, AgentToolImageGeneration,
			},
			denied: []string{AgentToolConfigRead, AgentToolConfigApply},
		},
		{
			kind:    AgentKindInteractiveStory,
			allowed: []string{AgentToolFilesystemRead, AgentToolSkills, AgentToolLoreRead},
			denied:  []string{AgentToolWorkspaceWrite, AgentToolShell, AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser, AgentToolAsk, AgentToolTodo, AgentToolGoal, AgentToolDelegation, AgentToolLoreWrite, AgentToolImageGeneration, AgentToolConfigRead, AgentToolConfigApply},
		},
		{
			kind:    AgentKindConfigManager,
			allowed: []string{AgentToolFilesystemRead, AgentToolAsk, AgentToolSkills, AgentToolConfigRead, AgentToolConfigApply},
			denied:  []string{AgentToolWorkspaceWrite, AgentToolShell, AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser, AgentToolTodo, AgentToolGoal, AgentToolDelegation, AgentToolLoreWrite, AgentToolImageGeneration},
		},
		{
			kind:    AgentKindInteractiveDirector,
			allowed: []string{AgentToolEventRead, AgentToolLoreRead},
			denied:  []string{AgentToolFilesystemRead, AgentToolWorkspaceWrite, AgentToolShell, AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser, AgentToolAsk, AgentToolTodo, AgentToolGoal, AgentToolSkills, AgentToolDelegation, AgentToolLoreWrite, AgentToolImageGeneration, AgentToolConfigRead, AgentToolConfigApply},
		},
		{
			kind:    AgentKindImage,
			allowed: []string{AgentToolSkills, AgentToolImageGeneration},
			denied:  []string{AgentToolFilesystemRead, AgentToolWorkspaceWrite, AgentToolShell, AgentToolWebSearch, AgentToolWebFetch, AgentToolBrowser, AgentToolAsk, AgentToolTodo, AgentToolGoal, AgentToolDelegation, AgentToolLoreRead, AgentToolLoreWrite, AgentToolConfigRead, AgentToolConfigApply},
		},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			resolved := ResolveAgentTools(&Config{}, test.kind)
			for _, capability := range test.allowed {
				if !resolved.Allows(capability) {
					t.Errorf("%s should allow %s: %#v", test.kind, capability, resolved)
				}
			}
			for _, capability := range test.denied {
				if resolved.Allows(capability) {
					t.Errorf("%s should deny %s: %#v", test.kind, capability, resolved)
				}
			}
		})
	}
	for _, kind := range []string{AgentKindVersionSummary, AgentKindToolAgent} {
		resolved := ResolveAgentTools(&Config{}, kind)
		for _, capability := range AgentToolCapabilities() {
			if resolved.Allows(capability.Source) {
				t.Errorf("%s should not allow %s", kind, capability.Source)
			}
		}
	}
	for _, definition := range AgentKindDefinitions() {
		if definition.Kind == AgentKindInteractiveDirector {
			continue
		}
		if ResolveAgentTools(&Config{}, definition.Kind).Allows(AgentToolEventRead) {
			t.Errorf("%s should not allow Director-only %s", definition.Kind, AgentToolEventRead)
		}
	}
}

func TestResolveAgentToolsSparseOverrides(t *testing.T) {
	cfg := &Config{AgentTools: AgentToolSettings{
		Default: AgentToolOverride{AgentToolShell: false, AgentToolWebSearch: false},
		IDE: AgentToolOverride{
			AgentToolShell: true, AgentToolLoreWrite: false, AgentToolWebSearch: true,
		},
	}}
	ide := ResolveAgentTools(cfg, AgentKindIDE)
	if !ide.Allows(AgentToolShell) || !ide.Allows(AgentToolWebSearch) || ide.Allows(AgentToolLoreWrite) {
		t.Fatalf("IDE sparse overrides were not applied: %#v", ide)
	}
	story := ResolveAgentTools(cfg, AgentKindInteractiveStory)
	if story.Allows(AgentToolShell) || story.Allows(AgentToolWebSearch) {
		t.Fatalf("story should retain explicit disabled capabilities: %#v", story)
	}
}

func TestResolveAgentToolsEnforcesRegisteredCapabilityCeilings(t *testing.T) {
	allEnabled := make(AgentToolOverride, len(agentToolCapabilities))
	for _, capability := range agentToolCapabilities {
		allEnabled[capability.Source] = true
	}
	cfg := &Config{AgentTools: AgentToolSettings{
		Default:             allEnabled,
		IDE:                 allEnabled,
		InteractiveStory:    allEnabled,
		ConfigManager:       allEnabled,
		InteractiveDirector: allEnabled,
		VersionSummary:      allEnabled,
		ToolAgent:           allEnabled,
		Image:               allEnabled,
		Automation:          allEnabled,
	}}

	for _, definition := range AgentKindDefinitions() {
		ceiling := make(map[string]bool, len(definition.ToolCapabilities))
		for _, capability := range definition.ToolCapabilities {
			ceiling[capability] = true
		}
		resolved := ResolveAgentTools(cfg, definition.Kind)
		for _, capability := range agentToolCapabilities {
			want := ceiling[capability.Source]
			if definition.Kind == AgentKindHarnessOptimizer && capability.Source == AgentToolScript {
				want = false
			}
			if got := resolved.Allows(capability.Source); got != want {
				t.Errorf("%s capability %s = %t, want ceiling %t", definition.Kind, capability.Source, got, want)
			}
		}
	}

	unknown := ResolveAgentTools(cfg, "unregistered-agent")
	for _, capability := range agentToolCapabilities {
		if unknown.Allows(capability.Source) {
			t.Errorf("unregistered Agent escaped capability ceiling through %s", capability.Source)
		}
	}
}

func TestResolveAgentToolsKeepsShellCapabilityPlatformNeutral(t *testing.T) {
	cfg := &Config{AgentTools: AgentToolSettings{IDE: AgentToolOverride{AgentToolShell: true}}}
	if resolved := resolveAgentToolsForGOOS(cfg, AgentKindIDE, "windows"); !resolved.Allows(AgentToolShell) {
		t.Fatalf("shell capability should resolve independently of platform tool name: %#v", resolved)
	}
	cfg.AgentTools.IDE[AgentToolShell] = false
	if resolved := resolveAgentToolsForGOOS(cfg, AgentKindIDE, "windows"); resolved.Allows(AgentToolShell) {
		t.Fatalf("explicit shell disable should remain effective: %#v", resolved)
	}
}
