package prompts

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/book"
)

func TestBuildInteractiveStoryInstructionIsIsolatedFromIDEPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	instruction := BuildInteractiveStoryInstruction(&config.Config{Workspace: state.Workspace(), InteractiveReplyTargetChars: 777}, state, InteractiveStorySystemInstructionInput{
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
	for _, required := range []string{"Game Mode", "interactive text adventures", "Output only the story prose", "hidden state blocks", "shortcut-choice blocks", "Do not use write, edit", "todo", "<invoke>", "prose RPG", "identify the action", "meaningful new options", "check consistency", "list_lore_items", "read_lore_items", "search_story_history"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive story instruction should contain %q:\n%s", required, instruction)
		}
	}
	if !strings.Contains(instruction, "Director System Rules") || !strings.Contains(instruction, "经典叙事者") {
		t.Fatalf("interactive story instruction should include teller system rules:\n%s", instruction)
	}
	for _, required := range []string{"highest length constraint", "[Highest length constraint]", "about 777 Chinese characters"} {
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
	}, state, InteractiveStorySystemInstructionInput{
		StoryTellerID:           "long",
		StoryTellerName:         "长篇导演",
		StoryTellerDescription:  "偏长",
		StoryTellerSystemPrompt: "每轮至少写 5000 字。",
	})

	for _, required := range []string{"highest length constraint", "must not require exceeding it", "about 650 Chinese characters"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive story instruction should protect story reply target %q:\n%s", required, instruction)
		}
	}
	for _, preserved := range []string{"每轮至少写 5000 字"} {
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
	composition := BuildInteractiveStoryInstructionComposition(&config.Config{Workspace: state.Workspace()}, state, InteractiveStorySystemInstructionInput{
		StoryTellerID:           "classic",
		StoryTellerSystemPrompt: "讲述规则",
	})
	if composition.Instruction() == "" {
		t.Fatal("composition instruction should be populated")
	}
	if got := buf.String(); strings.Contains(got, "[agent-prompt]") {
		t.Fatalf("prompt build should not emit agent-prompt logs, got:\n%s", got)
	}

	composition.LogAdmission("task-1", "session-1")
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
		Name:        "config-manager",
		Description: "All configuration resource routing",
		Content:     "Read skill://config-manager/references/automation.md for automation details.",
	})
	instruction := composition.Instruction()
	for _, want := range []string{"Config Manager Skill", "/config-manager", "skill://config-manager/references/automation.md"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("config manager instruction missing %q:\n%s", want, instruction)
		}
	}

	composition.LogAdmission("task-1", "session-1")
	got := buf.String()
	for _, want := range []string{"configuration Skill", "/config-manager", "task_id=task-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("composition log missing %q:\n%s", want, got)
		}
	}
}

func TestBuildConfigManagerInstructionAllowsAgentConfigTools(t *testing.T) {
	state := book.NewState(t.TempDir())
	instruction := BuildConfigManagerInstruction(&config.Config{Workspace: state.Workspace()}, state)
	for _, want := range []string{"config_read", "config_apply", "resource=agent_profile", "Do not directly edit backing files", "Agent configuration"} {
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

	composition, err := ComposeInstruction(cfg, state, IDEStoryTeller{})
	if err != nil {
		t.Fatal(err)
	}
	instruction := composition.Instruction()
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

func TestIDEContextRuntimeContextIsBoundedPathOnlyState(t *testing.T) {
	longName := strings.Repeat("长", ideContextMaxPathRunes+8) + ".md"
	context := IDEContextRuntimeContext(IDEContextRef{
		CurrentFile: "/chapters/ch01.md",
		OpenFiles: []string{
			"chapters/ch01.md",
			"chapters/ch01.md",
			"../outside.md",
			longName,
		},
	})

	for _, want := range []string{"Focused file: chapters/ch01.md", "Open files: chapters/ch01.md,", "never file contents", "read any needed file explicitly by path", "[truncated]"} {
		if !strings.Contains(context, want) {
			t.Fatalf("IDE context missing %q:\n%s", want, context)
		}
	}
	if strings.Contains(context, "../outside.md") {
		t.Fatalf("IDE context should drop unsafe relative paths:\n%s", context)
	}
	if strings.Count(context, "chapters/ch01.md") != 2 {
		t.Fatalf("IDE context should dedupe open files while preserving current file:\n%s", context)
	}
}

func TestBuildInstructionIncludesStyleRulesInSystemPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{Workspace: state.Workspace()}

	composition, err := ComposeInstruction(cfg, state, IDEStoryTeller{
		ID:         "classic",
		Prompt:     "导演系统规则",
		StyleRules: []StyleRule{{Scene: "激烈打斗", StyleContents: []string{"短句留白"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	instruction := composition.Instruction()

	for _, required := range []string{"## Prose Style References", "Scene: 激烈打斗", "短句留白", "Trigger rule", "system prompt provides them"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("system prompt should include style rule %q:\n%s", required, instruction)
		}
	}
}

func TestBuildInstructionIncludesImagePresetSystemPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{Workspace: state.Workspace()}

	composition, err := ComposeInstruction(cfg, state, IDEStoryTeller{
		ImagePresetID:           "realistic",
		ImagePresetName:         "写实",
		ImagePresetSystemPrompt: "构造图像提示词时保持真实光影。",
	})
	if err != nil {
		t.Fatal(err)
	}
	instruction := composition.Instruction()

	for _, required := range []string{"## Image Preset System Rules (Image Generation Only)", "realistic", "写实", "构造图像提示词时保持真实光影", "ordinary prose writing"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("system prompt should include image preset system rule %q:\n%s", required, instruction)
		}
	}
}

func TestBuildInteractiveStoryInstructionIncludesStyleRulesInSystemPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{Workspace: state.Workspace()}

	instruction := BuildInteractiveStoryInstruction(cfg, state, InteractiveStorySystemInstructionInput{
		StoryTellerID:           "classic",
		StoryTellerSystemPrompt: "导演系统规则",
		StyleRules:              []StyleRule{{Scene: "日常对话", StyleContents: []string{"克制对白"}}},
	})

	for _, required := range []string{"## Prose Style References", "Scene: 日常对话", "克制对白", "prose-style index in the system prompt"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive system prompt should include style rule %q:\n%s", required, instruction)
		}
	}
}

func TestBuiltinAgentPromptsExposeTurnHistorySearchWithoutCustomPrompt(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{
		Workspace: state.Workspace(),
		AgentPrompts: config.AgentPromptSettings{
			InteractiveStory: config.AgentPromptOverride{SystemPrompt: "用户覆盖不应出现在默认展示里"},
		},
	}
	builtin := BuiltinAgentPrompts(cfg, state, IDEStoryTeller{})
	got := builtin.InteractiveStory.SystemPrompt
	for _, required := range []string{"list_lore_items", "read_lore_items", "search_story_history", "turn_id"} {
		if !strings.Contains(got, required) {
			t.Fatalf("builtin interactive prompt missing %q:\n%s", required, got)
		}
	}
	if strings.Contains(got, "用户覆盖不应出现在默认展示里") {
		t.Fatalf("builtin prompt should not include custom prompt:\n%s", got)
	}

	blocks := BuiltinAgentPromptBlocks(cfg, state, IDEStoryTeller{})
	interactive := blocks.InteractiveStory
	if !strings.Contains(interactive.RuntimeContract, "Follow the current user request") {
		t.Fatalf("runtime contract should be populated: %#v", interactive)
	}
	if !strings.Contains(interactive.OutputProtocol, "Output only the story prose") {
		t.Fatalf("output protocol should require direct narrative text: %#v", interactive)
	}
	if !strings.Contains(interactive.EditableSystemPrompt, "search_story_history") || !strings.Contains(interactive.EditableSystemPrompt, "turn_id") {
		t.Fatalf("editable prompt should include turn history recall flow: %#v", interactive)
	}
	if !strings.Contains(interactive.EditableSystemPrompt, "story-level runtime parameter") || strings.Contains(interactive.EditableSystemPrompt, "2000 Chinese characters") {
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
	if !strings.Contains(flowSource.Content, "search_story_history") || !strings.Contains(flowSource.Content, "turn_id") {
		t.Fatalf("flow source should include turn history recall flow: %#v", flowSource)
	}
	customSource := findPromptSource(interactiveSources, "custom")
	if customSource == nil || !customSource.Editable || customSource.Field != "system_prompt" {
		t.Fatalf("custom source should be editable system_prompt: %#v", customSource)
	}
}

func TestBuiltinInteractiveDirectorPromptUsesMaintenanceToolContract(t *testing.T) {
	state := book.NewState(t.TempDir())
	cfg := &config.Config{Workspace: state.Workspace()}

	builtin := BuiltinAgentPrompts(cfg, state, IDEStoryTeller{})
	got := builtin.InteractiveDirector.SystemPrompt
	for _, required := range []string{
		"# Background Director",
		"Turn, including RuleResolution and StateDelta",
		"search_story_history",
		"turn_id",
		"director.md",
		"submit_director_plan_update",
		"keep uses empty updates",
		"Ordinary updates change agent-brief.md by default",
		"Retry only retry_documents",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("builtin interactive director prompt missing %q:\n%s", required, got)
		}
	}
	for _, legacy := range []string{"Memory Recorder", "字段包括 state_ops", "apply_actor_state_patch", "只能使用 read_file、write_file、edit_file"} {
		if strings.Contains(got, legacy) {
			t.Fatalf("builtin interactive director prompt should not contain legacy contract %q:\n%s", legacy, got)
		}
	}
}

func TestContextCompactionPromptIsAnInternalSourceAgentProtocol(t *testing.T) {
	cfg := &config.Config{AgentPrompts: config.AgentPromptSettings{IDE: config.AgentPromptOverride{
		FlowPrompt:   "MUST NOT REPLACE INTERNAL PROTOCOL",
		SystemPrompt: "MUST NOT EXTEND INTERNAL PROTOCOL",
	}}}
	composition, err := ComposeContextCompactionInstruction(cfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	instruction := composition.Instruction()
	if !strings.Contains(instruction, "internal context checkpoint maintenance protocol") {
		t.Fatalf("internal context compaction prompt missing role:\n%s", instruction)
	}
	requiredParts := strings.Split(strings.TrimSpace(agentcontext.CompactionCheckpointSchema()), "\n")
	requiredParts = append(requiredParts, "source_agent_kind", "user message provides the target length range")
	for _, required := range requiredParts {
		if !strings.Contains(instruction, required) {
			t.Fatalf("internal context compaction prompt missing %q:\n%s", required, instruction)
		}
	}
	if !strings.Contains(instruction, "checkpoint is not a new source of truth") ||
		!strings.Contains(instruction, "readable artifact paths") {
		t.Fatalf("internal context compaction prompt should define the checkpoint boundary:\n%s", instruction)
	}
	for _, forbidden := range []string{"MUST NOT REPLACE INTERNAL PROTOCOL", "MUST NOT EXTEND INTERNAL PROTOCOL"} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("source Agent prompt override leaked into internal compaction protocol: %q", forbidden)
		}
	}
}

func TestInteractiveFlowSourceExcludesProjectInstructionsAndKeepsRecallFlow(t *testing.T) {
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
	for _, required := range []string{"Tool-assisted Recall Workflow", "list_lore_items", "read_lore_items", "search_story_history", "turn_id"} {
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
