package config

import (
	"path/filepath"
	"testing"
)

func TestResolveAgentPromptMergesDefaultAndAgent(t *testing.T) {
	cfg := &Config{
		AgentPrompts: AgentPromptSettings{
			Default: AgentPromptOverride{FlowPrompt: "default flow", SystemPrompt: "default prompt"},
			IDE:     AgentPromptOverride{FlowPrompt: "ide flow", SystemPrompt: "ide prompt"},
		},
	}

	if got := ResolveAgentPrompt(cfg, AgentKindIDE).SystemPrompt; got != "ide prompt" {
		t.Fatalf("ide prompt = %q", got)
	}
	if got := ResolveAgentPrompt(cfg, AgentKindIDE).FlowPrompt; got != "ide flow" {
		t.Fatalf("ide flow prompt = %q", got)
	}
	if got := ResolveAgentPrompt(cfg, AgentKindVersionSummary).SystemPrompt; got != "default prompt" {
		t.Fatalf("version summary prompt should inherit default, got %q", got)
	}
	if got := ResolveAgentPrompt(cfg, AgentKindVersionSummary).FlowPrompt; got != "default flow" {
		t.Fatalf("version summary flow prompt should inherit default, got %q", got)
	}
}

func TestMergeAgentPromptSettingsEmptyChildInherits(t *testing.T) {
	parent := AgentPromptSettings{
		Default: AgentPromptOverride{SystemPrompt: "default prompt"},
		IDE:     AgentPromptOverride{SystemPrompt: "ide prompt"},
	}
	child := AgentPromptSettings{
		Default: AgentPromptOverride{SystemPrompt: "workspace default"},
		IDE:     AgentPromptOverride{SystemPrompt: "   "},
	}

	out := MergeAgentPromptSettings(parent, child)
	if out.Default.SystemPrompt != "workspace default" {
		t.Fatalf("default prompt should override: %q", out.Default.SystemPrompt)
	}
	if out.IDE.SystemPrompt != "ide prompt" {
		t.Fatalf("empty child prompt should inherit parent: %q", out.IDE.SystemPrompt)
	}
}

func TestAgentPromptsSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	in := Settings{
		AgentPrompts: AgentPromptSettings{
			Default: AgentPromptOverride{FlowPrompt: "default flow", SystemPrompt: "default prompt"},
			IDE:     AgentPromptOverride{FlowPrompt: "ide flow", SystemPrompt: "ide prompt"},
		},
	}
	if err := WriteSettingsFile(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.AgentPrompts.Default.SystemPrompt != "default prompt" {
		t.Fatalf("default prompt lost: %q", out.AgentPrompts.Default.SystemPrompt)
	}
	if out.AgentPrompts.Default.FlowPrompt != "default flow" {
		t.Fatalf("default flow prompt lost: %q", out.AgentPrompts.Default.FlowPrompt)
	}
	if out.AgentPrompts.IDE.SystemPrompt != "ide prompt" {
		t.Fatalf("ide prompt lost: %q", out.AgentPrompts.IDE.SystemPrompt)
	}
	if out.AgentPrompts.IDE.FlowPrompt != "ide flow" {
		t.Fatalf("ide flow prompt lost: %q", out.AgentPrompts.IDE.FlowPrompt)
	}
}

func TestAgentPromptsBlankPromptIsSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	in := Settings{
		AgentPrompts: AgentPromptSettings{
			IDE: AgentPromptOverride{FlowPrompt: "   ", SystemPrompt: "   "},
		},
	}
	if err := WriteSettingsFile(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.AgentPrompts.IDE.SystemPrompt != "" {
		t.Fatalf("blank prompt should be sanitized: %q", out.AgentPrompts.IDE.SystemPrompt)
	}
	if out.AgentPrompts.IDE.FlowPrompt != "" {
		t.Fatalf("blank flow prompt should be sanitized: %q", out.AgentPrompts.IDE.FlowPrompt)
	}
}
