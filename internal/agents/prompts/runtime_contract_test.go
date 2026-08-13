package prompts

import (
	"strings"
	"testing"

	"denova/config"
)

func TestUserStatePromptFollowsContractAndBuiltInPrompt(t *testing.T) {
	cfg := &config.Config{}
	base, err := ComposeBuiltinSystemInstruction(cfg, config.AgentKindIDE, "test", "", "builtin", "Built in", "test", "BUILT IN PROMPT")
	if err != nil {
		t.Fatal(err)
	}
	composition, err := AppendUserStatePrompt(cfg, base, "USER STATE PROMPT")
	if err != nil {
		t.Fatal(err)
	}
	instruction := composition.Instruction()

	contractIndex := strings.Index(instruction, "Denova Runtime Contract")
	builtInIndex := strings.Index(instruction, "BUILT IN PROMPT")
	stateIndex := strings.Index(instruction, "USER STATE PROMPT")
	if contractIndex < 0 || builtInIndex < 0 || stateIndex < 0 {
		t.Fatalf("instruction missing expected sections:\n%s", instruction)
	}
	if !(contractIndex < builtInIndex && builtInIndex < stateIndex) {
		t.Fatalf("wrong system prompt order: contract=%d built_in=%d state=%d\n%s", contractIndex, builtInIndex, stateIndex, instruction)
	}
	if strings.Count(instruction, "# User State Prompt") != 1 || strings.Count(instruction, "USER STATE PROMPT") != 1 {
		t.Fatalf("State prompt should be injected once without a repeated generic wrapper:\n%s", instruction)
	}
	if strings.Contains(instruction, "cannot override the runtime contract") {
		t.Fatalf("State prompt should not repeat generic precedence language:\n%s", instruction)
	}
}

func TestProtectedSystemInstructionOmitsEmptyCustomPrompt(t *testing.T) {
	instruction := protectedSystemInstruction(&config.Config{}, config.AgentKindIDE, "BUILT IN PROMPT")
	if strings.Contains(instruction, "# 用户自定义系统提示") {
		t.Fatalf("empty custom prompt should not render custom section:\n%s", instruction)
	}
	if !strings.Contains(instruction, "BUILT IN PROMPT") {
		t.Fatalf("built-in prompt missing:\n%s", instruction)
	}
}

func TestProtectedSystemInstructionGuidesThinkingLanguageFromConfig(t *testing.T) {
	zhInstruction := protectedSystemInstruction(&config.Config{Language: "zh-CN"}, config.AgentKindIDE, "BUILT IN PROMPT")
	for _, required := range []string{"## Thinking Language", "Use Simplified Chinese for internal reasoning", "This controls thinking language only"} {
		if !strings.Contains(zhInstruction, required) {
			t.Fatalf("zh-CN thinking language contract missing %q:\n%s", required, zhInstruction)
		}
	}

	enInstruction := protectedSystemInstruction(&config.Config{Language: "en-US"}, config.AgentKindIDE, "BUILT IN PROMPT")
	for _, required := range []string{"## Thinking Language", "Use English for internal reasoning", "This only controls thinking language"} {
		if !strings.Contains(enInstruction, required) {
			t.Fatalf("en-US thinking language contract missing %q:\n%s", required, enInstruction)
		}
	}
}

func TestSubAgentParentRuntimeContractsIncludeDelegationProtocol(t *testing.T) {
	for _, agentKind := range config.SubAgentParentKinds() {
		t.Run(agentKind, func(t *testing.T) {
			instruction := protectedSystemInstruction(&config.Config{}, agentKind, "BUILT IN PROMPT")
			for _, required := range []string{
				"Use the task tool only when the user or a loaded Skill requests delegation",
				"self-contained goal, constraints, relevant paths or resource IDs, expected output, and write scope",
				"Pass references instead of copying content it can read itself",
				"Verify the returned result before reporting it to the user",
			} {
				if !strings.Contains(instruction, required) {
					t.Fatalf("deep parent %s should include subagent delegation protocol %q:\n%s", agentKind, required, instruction)
				}
			}
		})
	}
}

func TestRuntimeContractsCoverAllAgentKinds(t *testing.T) {
	tests := map[string]string{
		config.AgentKindGeneral:             "General Agent",
		config.AgentKindHarnessOptimizer:    "Harness Optimizer",
		config.AgentKindIDE:                 "CREATOR.md",
		config.AgentKindInteractiveStory:    "Output only the story prose",
		config.AgentKindImage:               "Image Agent",
		config.AgentKindConfigManager:       "Agents-page resources",
		config.AgentKindInteractiveDirector: "does not initialize or review the state schema",
		config.AgentKindVersionSummary:      "Version Summary Agent",
		config.AgentKindToolAgent:           "model-only",
		config.AgentKindAutomation:          "Automation Agent",
	}
	for _, definition := range config.AgentKindDefinitions() {
		required, ok := tests[definition.Kind]
		if !ok {
			t.Fatalf("agent %s should declare a runtime contract assertion", definition.Kind)
		}
		t.Run(definition.Kind, func(t *testing.T) {
			agentKind := definition.Kind
			instruction := protectedSystemInstruction(&config.Config{}, agentKind, "BUILT IN PROMPT")
			if !strings.Contains(instruction, required) {
				t.Fatalf("contract for %s should contain %q:\n%s", agentKind, required, instruction)
			}
		})
	}
}

func TestGeneralAgentInstructionUsesProjectNeutralFilesystemRules(t *testing.T) {
	composition, err := ComposeGeneralInstruction(&config.Config{Workspace: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	instruction := composition.Instruction()
	for _, required := range []string{
		"general-purpose project Agent",
		"current Project root",
		"Prefer dedicated file and search tools",
		"independent tool calls may run in parallel",
		"Discovery respects .gitignore",
		"explicitly named paths remain addressable",
		"Write code in the surrounding style",
		"Report the actual outcome",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("General Agent instruction missing %q:\n%s", required, instruction)
		}
	}
	for _, forbidden := range []string{"CREATOR.md", "资料库写入", "图像生成"} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("General Agent instruction leaked writing-only contract %q:\n%s", forbidden, instruction)
		}
	}
}

func TestInteractiveDirectorContractRejectsStateSchemaOwnership(t *testing.T) {
	instruction := protectedSystemInstruction(&config.Config{}, config.AgentKindInteractiveDirector, "BUILT IN PROMPT")
	for _, required := range []string{"does not initialize or review the state schema", "must not write, overwrite, or correct Actor State or alter a frozen state schema", "opening Game Agent"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive Director boundary missing %q:\n%s", required, instruction)
		}
	}
	for _, forbidden := range []string{"state_schema_initialization", "submit_state_schema_adaptation", "Batch actor_ops"} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("interactive Director must not retain schema task %q:\n%s", forbidden, instruction)
		}
	}
}
