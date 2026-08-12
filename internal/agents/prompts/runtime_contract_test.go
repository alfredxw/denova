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

	contractIndex := strings.Index(instruction, "Denova 运行时契约")
	builtInIndex := strings.Index(instruction, "BUILT IN PROMPT")
	stateIndex := strings.Index(instruction, "USER STATE PROMPT")
	if contractIndex < 0 || builtInIndex < 0 || stateIndex < 0 {
		t.Fatalf("instruction missing expected sections:\n%s", instruction)
	}
	if !(contractIndex < builtInIndex && builtInIndex < stateIndex) {
		t.Fatalf("wrong system prompt order: contract=%d built_in=%d state=%d\n%s", contractIndex, builtInIndex, stateIndex, instruction)
	}
	if !strings.Contains(instruction, "不得覆盖运行时契约、工具权限、Schema、持久化边界或输出协议") {
		t.Fatalf("State prompt section should state protected boundary:\n%s", instruction)
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

func TestSubAgentParentRuntimeContractsIncludeDelegationProtocol(t *testing.T) {
	for _, agentKind := range config.SubAgentParentKinds() {
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
		config.AgentKindGeneral:             "General Agent",
		config.AgentKindHarnessOptimizer:    "Harness Optimizer",
		config.AgentKindIDE:                 "CREATOR.md",
		config.AgentKindInteractiveStory:    "只输出本回合可展示在故事舞台上的故事正文",
		config.AgentKindImage:               "图像 Agent",
		config.AgentKindConfigManager:       "配置管理 Agent",
		config.AgentKindInteractiveDirector: "不负责状态结构初始化或复审",
		config.AgentKindVersionSummary:      "版本说明 Agent",
		config.AgentKindToolAgent:           "model-only",
		config.AgentKindAutomation:          "自动化Agent",
		config.AgentKindContextCompaction:   "上下文压缩 Agent",
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
		"General Agent",
		"当前 Project 目录就是工作根目录",
		"发现与内容搜索默认遵守 .gitignore",
		"明确指定路径、直接 read/write/edit 的文件不因 .gitignore 被硬屏蔽",
		"不要自动创建或修改 .gitignore",
		"Denova 数据目录",
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
	for _, required := range []string{"不负责状态结构初始化或复审", "不得调整已经冻结的状态结构", "开局 Game Agent"} {
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
