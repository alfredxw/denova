package prompts

import (
	"strings"
	"testing"

	"denova/config"
)

func TestDelegationContractMatchesRuntimeAndCapabilities(t *testing.T) {
	for _, kind := range config.SubAgentParentKinds() {
		for _, test := range []struct {
			name       string
			runtime    *config.RuntimeSelection
			override   config.AgentToolOverride
			delegation bool
		}{
			{name: "implicit_native_defaults", delegation: kind != config.AgentKindInteractiveStory},
			{name: "implicit_native_enabled", override: config.AgentToolOverride{config.AgentToolDelegation: true}, delegation: true},
			{name: "native_enabled", runtime: &config.RuntimeSelection{Kind: config.RuntimeNative}, override: config.AgentToolOverride{config.AgentToolDelegation: true}, delegation: true},
			{name: "native_disabled", runtime: &config.RuntimeSelection{Kind: config.RuntimeNative}, override: config.AgentToolOverride{config.AgentToolDelegation: false}},
			{name: "codex", runtime: &config.RuntimeSelection{Kind: config.RuntimeCodex}, override: config.AgentToolOverride{config.AgentToolDelegation: true}},
			{name: "claude", runtime: &config.RuntimeSelection{Kind: config.RuntimeClaude}, override: config.AgentToolOverride{config.AgentToolDelegation: true}},
		} {
			t.Run(kind+"/"+test.name, func(t *testing.T) {
				cfg := &config.Config{
					ActiveAgentRuntime: test.runtime,
					AgentTools: config.AgentToolSettings{
						Default: test.override, InteractiveStory: test.override,
					},
				}
				instruction := protectedSystemInstruction(cfg, kind, "BUILT IN PROMPT")
				if !strings.Contains(instruction, "BUILT IN PROMPT") || !strings.Contains(instruction, "## Output Protocol") {
					t.Fatal("runtime selection lost the built-in workflow or output protocol")
				}
				surfaces := map[string]string{
					"instruction": instruction,
					"blocks":      builtinPromptBlocks(cfg, kind, "BUILT IN PROMPT").RuntimeContract,
				}
				for _, source := range builtinPromptSourceList(cfg, kind, "BUILT IN PROMPT").Sources {
					if source.ID == "runtime_contract" {
						surfaces["sources"] = source.Content
					}
				}
				if _, found := surfaces["sources"]; !found {
					t.Fatal("prompt preview lost the runtime contract source")
				}
				for surface, content := range surfaces {
					for _, marker := range []string{"send delegate", "list_agents", "await is a readiness synchronization point", "TASK_RESULT"} {
						if got := strings.Contains(content, marker); got != test.delegation {
							t.Errorf("%s contains %q = %v, want %v", surface, marker, got, test.delegation)
						}
					}
					if !strings.Contains(content, "## Language Alignment") || !strings.Contains(content, "Follow the current user request") {
						t.Errorf("%s lost shared runtime rules", surface)
					}
				}
			})
		}
	}
}
