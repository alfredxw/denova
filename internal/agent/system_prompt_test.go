package agent

import (
	"strings"
	"testing"

	"nova/config"
)

func TestProtectedSystemInstructionOrdersContractUserAndBuiltIn(t *testing.T) {
	cfg := &config.Config{
		AgentPrompts: config.AgentPromptSettings{
			IDE: config.AgentPromptOverride{SystemPrompt: "USER CUSTOM PROMPT"},
		},
	}
	instruction := protectedSystemInstruction(cfg, config.AgentKindIDE, "BUILT IN PROMPT")

	contractIndex := strings.Index(instruction, "Nova 运行时契约")
	userIndex := strings.Index(instruction, "USER CUSTOM PROMPT")
	builtInIndex := strings.Index(instruction, "BUILT IN PROMPT")
	if contractIndex < 0 || userIndex < 0 || builtInIndex < 0 {
		t.Fatalf("instruction missing expected sections:\n%s", instruction)
	}
	if !(contractIndex < userIndex && userIndex < builtInIndex) {
		t.Fatalf("wrong system prompt order: contract=%d user=%d built_in=%d\n%s", contractIndex, userIndex, builtInIndex, instruction)
	}
	if !strings.Contains(instruction, "不得覆盖上一节运行时契约") {
		t.Fatalf("custom prompt section should state protected boundary:\n%s", instruction)
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

func TestRuntimeContractsCoverAllAgentKinds(t *testing.T) {
	tests := map[string]string{
		config.AgentKindIDE:                   "CREATOR.md",
		config.AgentKindInteractiveStory:      "<NARRATIVE>",
		config.AgentKindLoreEditor:            "资料库 Agent",
		config.AgentKindTellerEditor:          "导演 Agent",
		config.AgentKindInteractiveState:      "状态记忆 Agent",
		config.AgentKindInteractiveHotChoices: "快捷选项 Agent",
		config.AgentKindVersionSummary:        "版本说明 Agent",
	}
	for agentKind, required := range tests {
		t.Run(agentKind, func(t *testing.T) {
			instruction := protectedSystemInstruction(&config.Config{}, agentKind, "BUILT IN PROMPT")
			if !strings.Contains(instruction, required) {
				t.Fatalf("contract for %s should contain %q:\n%s", agentKind, required, instruction)
			}
		})
	}
}
