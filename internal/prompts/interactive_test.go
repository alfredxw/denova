package prompts

import (
	"strings"
	"testing"
)

func TestInteractivePromptsSkipLegacyCharacterAndWorldFallback(t *testing.T) {
	outputs := map[string]string{
		"story runtime": InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
			Title:            "末日开端",
			Origin:           "主角醒来发现世界已末日",
			StoryTellerID:    "classic",
			BranchID:         "main",
			ReplyTargetChars: 800,
			LongTermMemory:   "林川仍在黄泉酒馆。",
		}),
		"hot choices": InteractiveHotChoicesInstruction(InteractiveHotChoicesPromptInput{
			Title:         "末日开端",
			Origin:        "主角醒来发现世界已末日",
			StoryTellerID: "classic",
			BranchID:      "main",
			TurnHistory:   "第 1 回合剧情：门后传来低沉的风声。",
		}),
		"state memory": InteractiveStateInstruction(InteractiveStatePromptInput{
			Title:             "末日开端",
			Origin:            "主角醒来发现世界已末日",
			StoryTellerID:     "classic",
			StoryTellerMemory: "沉淀关键状态。",
			BranchID:          "main",
			StoryMemorySchema: "## important_character",
			StoryMemory:       "林川仍在黄泉酒馆。",
			TurnHistory:       "第 1 回合剧情：门后传来低沉的风声。",
			UserAction:        "我点燃火把",
			Narrative:         "火光照亮了墙上的新线索。",
		}),
	}

	for name, output := range outputs {
		for _, forbidden := range []string{"## 角色设定", "## 世界观设定"} {
			if strings.Contains(output, forbidden) {
				t.Fatalf("%s should not include legacy empty block %q:\n%s", name, forbidden, output)
			}
		}
	}
}

func TestInteractiveStoryPromptUsesDirectNarrativeOutputContract(t *testing.T) {
	system := BuildInteractiveStorySystemInstruction(InteractiveStorySystemInstructionInput{
		ReplyTargetChars: 600,
	})
	turn := InteractiveStoryTurnInstruction("我推开门", "", 0, "")
	for name, output := range map[string]string{
		"system": system,
		"turn":   turn,
	} {
		for _, required := range []string{"只输出", "故事正文", "不要输出计划", "状态 JSON", "Markdown 标题", "工具说明"} {
			if !strings.Contains(output, required) {
				t.Fatalf("%s prompt should contain direct narrative contract %q:\n%s", name, required, output)
			}
		}
		if strings.Contains(output, "<NARRATIVE>") {
			t.Fatalf("%s prompt should not require narrative XML wrapper:\n%s", name, output)
		}
	}
	for _, hidden := range []string{"<HOT_STATE>", "<STATE_DELTA>"} {
		if !strings.Contains(system, hidden) {
			t.Fatalf("system prompt should still forbid hidden state output %q:\n%s", hidden, system)
		}
	}
	for _, want := range []string{"event_intents", "事件卡", "type_name"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt should guide event card intents %q:\n%s", want, system)
		}
	}
}

func TestInteractiveStoryRuntimeContextIncludesBoundedDirectorState(t *testing.T) {
	output := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
		ReplyTargetChars:     800,
		DirectorStateSummary: "- 长期主线: 外门逆袭\n- 事件 1: 学院比拼",
	})
	for _, want := range []string{"后台导演状态摘要", "source: DirectorState", "limit: 4096 bytes", "外门逆袭", "学院比拼"} {
		if !strings.Contains(output, want) {
			t.Fatalf("runtime context should include %q:\n%s", want, output)
		}
	}
}

func TestInteractiveDirectorPromptOnlyPlansDirectorStatePatch(t *testing.T) {
	system := BuildInteractiveDirectorSystemInstruction()
	instruction := InteractiveDirectorInstruction(InteractiveDirectorPromptInput{
		Title:                "外门逆袭",
		Origin:               "主角被同门轻视",
		StoryTellerID:        "classic",
		BranchID:             "main",
		DirectorStateJSON:    `{"main_arc":"外门逆袭"}`,
		TurnAuditJSON:        `{"turn_brief":{"turn_goal":"公开比试"}}`,
		TurnHistory:          "第 1 回合剧情：主角报名。",
		StoryMemorySummary:   "主角仍被低估。",
		DirectorEventCatalog: `[{"id":"face_slap","category":"打脸"}]`,
	})
	for name, output := range map[string]string{"system": system, "instruction": instruction} {
		for _, want := range []string{"DirectorState", "patch", "只输出 JSON", "不负责续写", "RuleResolution"} {
			if !strings.Contains(output, want) {
				t.Fatalf("%s director prompt should include %q:\n%s", name, want, output)
			}
		}
		if strings.Contains(output, "故事正文\n") {
			t.Fatalf("%s director prompt should not ask for story prose:\n%s", name, output)
		}
	}
	for _, want := range []string{"beat_queue", "event_queue", "foreshadowing", "potential_characters", "branch_patches", "打脸", "事件卡 Markdown", "template"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("director instruction should include %q:\n%s", want, instruction)
		}
	}
}
