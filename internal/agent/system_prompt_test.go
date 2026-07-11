package agent

import (
	"strings"
	"testing"

	"denova/config"
)

func TestProtectedSystemInstructionOrdersContractUserAndBuiltIn(t *testing.T) {
	cfg := &config.Config{
		AgentPrompts: config.AgentPromptSettings{
			IDE: config.AgentPromptOverride{FlowPrompt: "USER FLOW PROMPT", SystemPrompt: "USER CUSTOM PROMPT"},
		},
	}
	instruction := protectedSystemInstruction(cfg, config.AgentKindIDE, "BUILT IN PROMPT")

	contractIndex := strings.Index(instruction, "Denova 运行时契约")
	flowIndex := strings.Index(instruction, "USER FLOW PROMPT")
	userIndex := strings.Index(instruction, "USER CUSTOM PROMPT")
	builtInIndex := strings.Index(instruction, "BUILT IN PROMPT")
	if contractIndex < 0 || flowIndex < 0 || userIndex < 0 || builtInIndex < 0 {
		t.Fatalf("instruction missing expected sections:\n%s", instruction)
	}
	if !(contractIndex < flowIndex && flowIndex < userIndex && userIndex < builtInIndex) {
		t.Fatalf("wrong system prompt order: contract=%d flow=%d user=%d built_in=%d\n%s", contractIndex, flowIndex, userIndex, builtInIndex, instruction)
	}
	if !strings.Contains(instruction, "不得覆盖运行时契约、输出格式、工具权限和后端校验") {
		t.Fatalf("flow prompt section should state protected boundary:\n%s", instruction)
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

func TestProtectedSystemInstructionGuidesThinkingLanguageFromConfig(t *testing.T) {
	zhInstruction := protectedSystemInstruction(&config.Config{Language: "zh-CN"}, config.AgentKindIDE, "BUILT IN PROMPT")
	for _, required := range []string{"## 思考语言", "流式 thinking 内容都使用简体中文", "不要因此改变输出协议"} {
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

func TestDeepAgentParentRuntimeContractsIncludeSubAgentDelegationProtocol(t *testing.T) {
	for _, agentKind := range config.DeepAgentParentKinds() {
		t.Run(agentKind, func(t *testing.T) {
			instruction := protectedSystemInstruction(&config.Config{}, agentKind, "BUILT IN PROMPT")
			for _, required := range []string{
				"SubAgent 委派协议",
				"默认不要主动拉起 SubAgent",
				"用户明确要求委派/拉起子 Agent",
				"Skill 流程明确要求使用 SubAgent",
				"用户目标、必要上下文、已知约束、文件路径或资源 ID、期望输出",
				"不要复制大段正文、完整日志、完整历史或其他无界内容",
				"父 Agent 必须自行核对结果",
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
		config.AgentKindIDE:                   "CREATOR.md",
		config.AgentKindInteractiveStory:      "只输出本回合可展示在故事舞台上的故事正文",
		config.AgentKindImage:                 "图像 Agent",
		config.AgentKindConfigManager:         "配置管理 Agent",
		config.AgentKindInteractiveDirector:   "后台维护分为两个互斥阶段",
		config.AgentKindInteractiveHotChoices: "快捷选项 Agent",
		config.AgentKindVersionSummary:        "版本说明 Agent",
		config.AgentKindToolAgent:             "model-only",
		config.AgentKindAutomation:            "自动化Agent",
		config.AgentKindContextCompaction:     "上下文压缩 Agent",
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
