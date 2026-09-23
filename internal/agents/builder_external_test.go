package agents

import (
	"strings"
	"testing"

	"denova/config"
	agentinteractive "denova/internal/agents/interactive"
	"denova/internal/agents/prompts"
)

func TestExternalAssembliesExcludeNativeDelegation(t *testing.T) {
	for _, runtime := range []config.RuntimeSelection{
		{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "test-model"}},
		{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test-model"}},
	} {
		for _, kind := range config.SubAgentParentKinds() {
			t.Run(string(runtime.Kind)+"/"+kind, func(t *testing.T) {
				cfg := &config.Config{
					Workspace: t.TempDir(), DenovaDir: t.TempDir(),
					ActiveAgentRuntime: &runtime,
					// Saved Native delegation must stay dormant for external engines.
					AgentTools: config.AgentToolSettings{
						Default:          config.AgentToolOverride{config.AgentToolDelegation: true},
						InteractiveStory: config.AgentToolOverride{config.AgentToolDelegation: true},
					},
				}
				host := AgentHostCapabilities{Interactive: true}
				var assembly ExternalAssembly
				var err error
				if kind == config.AgentKindInteractiveStory {
					assembly, err = BuildExternalGameAssembly(t.Context(), cfg, nil, prompts.InteractiveStorySystemInstructionInput{}, host, agentinteractive.InteractiveStoryToolContext{})
				} else {
					assembly, err = BuildExternalConversationAssembly(t.Context(), cfg, nil, prompts.IDEStoryTeller{}, kind, host)
				}
				if err != nil {
					t.Fatal(err)
				}
				instruction := assembly.Composition.Instruction()
				for _, marker := range []string{"send delegate", "list_agents", "await is a readiness synchronization point", "TASK_RESULT"} {
					if strings.Contains(instruction, marker) {
						t.Errorf("external instruction leaked Native coordination guidance %q", marker)
					}
				}
				for _, required := range []string{"Denova Runtime Contract", "## Output Protocol", "## Language Alignment"} {
					if !strings.Contains(instruction, required) {
						t.Errorf("external instruction lost shared product rules %q", required)
					}
				}
				if assembly.ToolSettings.Allows(config.AgentToolDelegation) || len(assembly.Tools) == 0 {
					t.Fatal("external assembly must expose product tools without Native delegation")
				}
				for _, definition := range assembly.Tools {
					info, err := definition.Tool.Info(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					switch info.Name {
					case "send", "await", "list_agents", "task", "task_wait":
						t.Errorf("external assembly exposed Native delegation tool %q", info.Name)
					}
				}
			})
		}
	}
}
