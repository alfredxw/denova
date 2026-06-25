package agent

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nova/config"
	"nova/internal/book"
	"nova/internal/prompts"
)

func TestBuildInteractiveStoryInstructionIsIsolatedFromIDEPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	instruction := BuildInteractiveStoryInstruction(&config.Config{Workspace: state.Workspace(), InteractiveReplyTargetChars: 777}, state, prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           "classic",
		StoryTellerName:         "经典叙事者",
		StoryTellerDescription:  "平衡叙事",
		StoryTellerSystemPrompt: "你是一位经典叙事者。",
	})

	for _, forbidden := range []string{"创建章节文件", "chXX", "progress.md", "setting/outline.md"} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("interactive story instruction should not contain IDE-only prompt %q:\n%s", forbidden, instruction)
		}
	}
	for _, required := range []string{"互动故事模式", "<NARRATIVE>", "<HOT_STATE>", "<STATE_DELTA>", "禁止使用写文件工具", "write_todos", "<invoke>", "文字小说 RPG", "回合裁定循环", "可选择", "一致性自检", "list_lore_items", "read_lore_items", "list_interactive_memories", "read_interactive_memories"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive story instruction should contain %q:\n%s", required, instruction)
		}
	}
	if !strings.Contains(instruction, "导演系统规则") || !strings.Contains(instruction, "经典叙事者") {
		t.Fatalf("interactive story instruction should include teller system rules:\n%s", instruction)
	}
	for _, required := range []string{"每轮目标字数为最高约束", "最高篇幅约束", "777 个中文字左右"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive story instruction should contain reply target priority %q:\n%s", required, instruction)
		}
	}
}

func TestBuildInteractiveStoryInstructionKeepsReplyTargetAboveCustomLengthPrompts(t *testing.T) {
	state := book.NewState(t.TempDir())
	instruction := BuildInteractiveStoryInstruction(&config.Config{
		Workspace:                   state.Workspace(),
		InteractiveReplyTargetChars: 650,
		AgentPrompts: config.AgentPromptSettings{
			InteractiveStory: config.AgentPromptOverride{
				SystemPrompt: "无论如何都写到 10000 字。",
				FlowPrompt:   "每轮都写成长篇。",
			},
		},
	}, state, prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           "long",
		StoryTellerName:         "长篇导演",
		StoryTellerDescription:  "偏长",
		StoryTellerSystemPrompt: "每轮至少写 5000 字。",
	})

	for _, required := range []string{"每轮目标字数为最高约束", "都不得要求超过该目标", "650 个中文字左右"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive story instruction should protect story reply target %q:\n%s", required, instruction)
		}
	}
	for _, preserved := range []string{"无论如何都写到 10000 字", "每轮至少写 5000 字"} {
		if !strings.Contains(instruction, preserved) {
			t.Fatalf("custom/user-authored prompt text should remain visible %q:\n%s", preserved, instruction)
		}
	}
}

func TestBuildInteractiveStoryInstructionDoesNotLogDuringPromptBuild(t *testing.T) {
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(previous)
	})

	state := book.NewState(t.TempDir())
	composition := BuildInteractiveStoryInstructionComposition(&config.Config{Workspace: state.Workspace()}, state, prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           "classic",
		StoryTellerSystemPrompt: "讲述规则",
	})
	if composition.Instruction() == "" {
		t.Fatal("composition instruction should be populated")
	}
	if got := buf.String(); strings.Contains(got, "[agent-prompt]") {
		t.Fatalf("prompt build should not emit agent-prompt logs, got:\n%s", got)
	}

	composition.logForRun(RunOptions{TaskID: "task-1", SessionID: "session-1"})
	got := buf.String()
	if count := strings.Count(got, "[agent-prompt] system composition"); count != 1 {
		t.Fatalf("expected one composition log, got %d:\n%s", count, got)
	}
	if !strings.Contains(got, "task_id=task-1") || !strings.Contains(got, "session_id=session-1") {
		t.Fatalf("composition log should include run identifiers:\n%s", got)
	}
}

func TestBuildConfigManagerInstructionIncludesResourceSkills(t *testing.T) {
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(previous)
	})

	state := book.NewState(t.TempDir())
	composition := BuildConfigManagerInstructionComposition(&config.Config{Workspace: state.Workspace()}, state, ConfigManagerResourceSkill{
		Name:        "automation-config",
		Description: "Automation schema guide",
		Content:     "Use write_mode values read_only, confirm_write, or auto_write.",
	})
	instruction := composition.Instruction()
	for _, want := range []string{"本轮自动加载的配置 Skills", "/automation-config", "write_mode values read_only"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("config manager instruction missing %q:\n%s", want, instruction)
		}
	}

	composition.logForRun(RunOptions{TaskID: "task-1", SessionID: "session-1"})
	got := buf.String()
	for _, want := range []string{"配置 Skill", "/automation-config", "task_id=task-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("composition log missing %q:\n%s", want, got)
		}
	}
}

func TestBuildConfigManagerInstructionAllowsAgentConfigTools(t *testing.T) {
	state := book.NewState(t.TempDir())
	instruction := BuildConfigManagerInstruction(&config.Config{Workspace: state.Workspace()}, state)
	for _, want := range []string{"list_agent_configs", "write_agent_configs", "不要通过文件工具直接改", "Agent 配置"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("config manager instruction missing %q:\n%s", want, instruction)
		}
	}
	if strings.Contains(instruction, "不要修改 Nova 设置、模型、端口、主题、Agent prompt 或工具权限") {
		t.Fatalf("config manager instruction should no longer forbid Agent page config tools:\n%s", instruction)
	}
}

func TestBuildInstructionKeepsWorkspaceStateOutOfSystemPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	if err := os.MkdirAll(state.SettingDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.SettingDir(), "outline.md"), []byte("主角进入废城。"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Workspace: state.Workspace()}

	instruction := BuildInstruction(cfg, state, IDEStoryTeller{})
	if strings.Contains(instruction, "主角进入废城") || strings.Contains(instruction, "# 当前作品状态") {
		t.Fatalf("system prompt should not include dynamic workspace state:\n%s", instruction)
	}
	contexts := IDEWorkspaceRuntimeContextsForState(state)
	if !strings.Contains(contexts.Stable, "主角进入废城") {
		t.Fatalf("stable runtime workspace context should include outline: %#v", contexts)
	}
	if strings.Contains(contexts.Dynamic, "主角进入废城") {
		t.Fatalf("dynamic runtime workspace context should not include stable outline: %#v", contexts)
	}
	if context := IDEWorkspaceRuntimeContext(state); !strings.Contains(context, "主角进入废城") {
		t.Fatalf("legacy runtime workspace context should include state: %q", context)
	}
}

func TestBuildInstructionIncludesStyleRulesInSystemPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{Workspace: state.Workspace()}

	instruction := BuildInstruction(cfg, state, IDEStoryTeller{
		ID:         "classic",
		Prompt:     "导演系统规则",
		StyleRules: []StyleRule{{Scene: "激烈打斗", StyleContents: []string{"短句留白"}}},
	})

	for _, required := range []string{"## 场景化风格规则", "场景：激烈打斗", "短句留白", "触发规则", "system prompt 注入了场景化风格规则"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("system prompt should include style rule %q:\n%s", required, instruction)
		}
	}
}

func TestBuildInteractiveStoryInstructionIncludesStyleRulesInSystemPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{Workspace: state.Workspace()}

	instruction := BuildInteractiveStoryInstruction(cfg, state, prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           "classic",
		StoryTellerSystemPrompt: "导演系统规则",
		StyleRules:              []prompts.StyleRule{{Scene: "日常对话", StyleContents: []string{"克制对白"}}},
	})

	for _, required := range []string{"## 场景化风格规则", "场景：日常对话", "克制对白", "system prompt 中的场景化风格内容"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive system prompt should include style rule %q:\n%s", required, instruction)
		}
	}
}

func TestSystemPromptSourceSummaryUsesStructuredStateParts(t *testing.T) {
	got := systemPromptSourceSummary("ide", "", []book.CompactContextPart{{
		Source:  "setting/character-states.md",
		Title:   "角色状态",
		Content: "林川在废城东区地下仓库。",
	}})
	for _, want := range []string{"作品状态", "角色状态", "setting/character-states.md"} {
		if !strings.Contains(got, want) {
			t.Fatalf("system prompt source summary missing %q:\n%s", want, got)
		}
	}
}

func TestBuiltinAgentPromptsExposeInteractiveMemoryToolsWithoutCustomPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{
		Workspace: state.Workspace(),
		AgentPrompts: config.AgentPromptSettings{
			InteractiveStory: config.AgentPromptOverride{SystemPrompt: "用户覆盖不应出现在默认展示里"},
		},
	}
	builtin := BuiltinAgentPrompts(cfg, state, IDEStoryTeller{})
	got := builtin.InteractiveStory.SystemPrompt
	for _, required := range []string{"list_lore_items", "read_lore_items", "list_interactive_memories", "read_interactive_memories"} {
		if !strings.Contains(got, required) {
			t.Fatalf("builtin interactive prompt missing %q:\n%s", required, got)
		}
	}
	if strings.Contains(got, "用户覆盖不应出现在默认展示里") {
		t.Fatalf("builtin prompt should not include custom prompt:\n%s", got)
	}

	blocks := BuiltinAgentPromptBlocks(cfg, state, IDEStoryTeller{})
	interactive := blocks.InteractiveStory
	if !strings.Contains(interactive.RuntimeContract, "运行时契约") {
		t.Fatalf("runtime contract should be populated: %#v", interactive)
	}
	if !strings.Contains(interactive.OutputProtocol, "<NARRATIVE>") {
		t.Fatalf("output protocol should contain narrative format: %#v", interactive)
	}
	if !strings.Contains(interactive.EditableSystemPrompt, "list_interactive_memories") || !strings.Contains(interactive.EditableSystemPrompt, "read_interactive_memories") {
		t.Fatalf("editable prompt should include memory recall flow: %#v", interactive)
	}
	if strings.Contains(interactive.EditableSystemPrompt, "必须只输出 <NARRATIVE>") {
		t.Fatalf("editable prompt should not include protected output protocol: %s", interactive.EditableSystemPrompt)
	}
	if !strings.Contains(interactive.EditableSystemPrompt, "story 级运行参数") || strings.Contains(interactive.EditableSystemPrompt, "2000 个中文字") {
		t.Fatalf("editable prompt should describe dynamic story reply target without fixed fallback: %s", interactive.EditableSystemPrompt)
	}

	sources := BuiltinAgentPromptSources(cfg, state, IDEStoryTeller{})
	interactiveSources := sources.InteractiveStory.Sources
	runtimeSource := findPromptSource(interactiveSources, "runtime_contract")
	if runtimeSource == nil || runtimeSource.Editable {
		t.Fatalf("runtime source should be read-only: %#v", runtimeSource)
	}
	flowSource := findPromptSource(interactiveSources, "flow")
	if flowSource == nil || !flowSource.Editable || flowSource.Field != "flow_prompt" {
		t.Fatalf("flow source should be editable flow_prompt: %#v", flowSource)
	}
	if !strings.Contains(flowSource.Content, "list_interactive_memories") || !strings.Contains(flowSource.Content, "read_interactive_memories") {
		t.Fatalf("flow source should include memory recall flow: %#v", flowSource)
	}
	if strings.Contains(flowSource.Content, "必须只输出 <NARRATIVE>") {
		t.Fatalf("flow source should not include protected output protocol: %s", flowSource.Content)
	}
	customSource := findPromptSource(interactiveSources, "custom")
	if customSource == nil || !customSource.Editable || customSource.Field != "system_prompt" {
		t.Fatalf("custom source should be editable system_prompt: %#v", customSource)
	}
}

func TestBuiltinInteractiveMemoryPromptUsesStoryMemoryPatchContract(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{Workspace: state.Workspace()}

	builtin := BuiltinAgentPrompts(cfg, state, IDEStoryTeller{})
	got := builtin.InteractiveState.SystemPrompt
	for _, required := range []string{
		"互动记忆 Agent",
		"story_memory_patches",
		"故事记忆结构与字段协议",
		"历史回合上下文",
		"资料库相关人物与设定",
		"本回合前的既有故事记忆",
		"按该表的字段列表逐字段填写",
		"不能只填 required 字段或本回合变化字段",
		"不得省略字段、写空字符串或 null",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("builtin interactive memory prompt missing %q:\n%s", required, got)
		}
	}
	for _, legacy := range []string{"memory_entry", "字段包括 state_ops"} {
		if strings.Contains(got, legacy) {
			t.Fatalf("builtin interactive memory prompt should not contain legacy contract %q:\n%s", legacy, got)
		}
	}
}

func TestBuiltinContextCompactionPromptIsConfigurableInAgentsView(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{Workspace: state.Workspace()}

	builtin := BuiltinAgentPrompts(cfg, state, IDEStoryTeller{})
	if !strings.Contains(builtin.ContextCompaction.SystemPrompt, "互动小说上下文压缩器") {
		t.Fatalf("builtin context compaction prompt missing role:\n%s", builtin.ContextCompaction.SystemPrompt)
	}
	for _, required := range []string{"【事件时间线】", "【长期影响账本】", "【当前阶段快照】", "目标长度由用户消息配置"} {
		if !strings.Contains(builtin.ContextCompaction.SystemPrompt, required) {
			t.Fatalf("builtin context compaction prompt missing %q:\n%s", required, builtin.ContextCompaction.SystemPrompt)
		}
	}
	if !strings.Contains(builtin.ContextCompaction.SystemPrompt, "plot_summary") {
		t.Fatalf("builtin context compaction prompt should mention configured target length:\n%s", builtin.ContextCompaction.SystemPrompt)
	}

	blocks := BuiltinAgentPromptBlocks(cfg, state, IDEStoryTeller{})
	if !strings.Contains(blocks.ContextCompaction.EditableSystemPrompt, "【事件时间线】") {
		t.Fatalf("context compaction editable prompt missing target rule:\n%s", blocks.ContextCompaction.EditableSystemPrompt)
	}

	sources := BuiltinAgentPromptSources(cfg, state, IDEStoryTeller{})
	flowSource := findPromptSource(sources.ContextCompaction.Sources, "flow")
	if flowSource == nil || !flowSource.Editable || flowSource.Field != "flow_prompt" {
		t.Fatalf("context compaction flow source should be editable flow_prompt: %#v", flowSource)
	}
	customSource := findPromptSource(sources.ContextCompaction.Sources, "custom")
	if customSource == nil || !customSource.Editable || customSource.Field != "system_prompt" {
		t.Fatalf("context compaction custom source should be editable system_prompt: %#v", customSource)
	}
}

func TestInteractiveFlowSourceKeepsRecallFlowWithCreatorPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	if err := os.WriteFile(filepath.Join(state.Workspace(), "CREATOR.md"), []byte("只使用第一人称。"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Workspace: state.Workspace()}

	sources := BuiltinAgentPromptSources(cfg, state, IDEStoryTeller{})
	flowSource := findPromptSource(sources.InteractiveStory.Sources, "flow")
	if flowSource == nil {
		t.Fatal("interactive story flow source missing")
	}
	for _, required := range []string{"工具化召回流程", "list_lore_items", "read_lore_items", "list_interactive_memories", "read_interactive_memories"} {
		if !strings.Contains(flowSource.Content, required) {
			t.Fatalf("flow source should keep %q with creator prompt:\n%s", required, flowSource.Content)
		}
	}
	if strings.Contains(flowSource.Content, "只使用第一人称") {
		t.Fatalf("flow source should not include creator prompt:\n%s", flowSource.Content)
	}
}

func findPromptSource(sources []config.AgentPromptSource, id string) *config.AgentPromptSource {
	for i := range sources {
		if sources[i].ID == id {
			return &sources[i]
		}
	}
	return nil
}
