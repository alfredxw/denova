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
	}
	for _, want := range []string{"event_intents", "自然形成", "不要为了引用事件 ID"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt should keep event intents organic %q:\n%s", want, system)
		}
	}
	for _, forbidden := range []string{"优先引用对应事件卡", "type_name/name"} {
		if strings.Contains(system, forbidden) {
			t.Fatalf("system prompt should not ask prose agent to trigger raw event cards %q:\n%s", forbidden, system)
		}
	}
}

func TestInteractiveStoryRuntimeContextIncludesBoundedDirectorPlanVisibleSections(t *testing.T) {
	output := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
		ReplyTargetChars:            800,
		DirectorPlanVisible:         "## 大方向规划 / Mainline\n\n### 目标 / Goal\n外门逆袭\n\n### 节奏、压力与危机 / Pacing, Pressure, Crisis\n学院比拼压力",
		StoryDirectorStrategyPrompt: "- 避免连续两回合使用同类型突发事件。",
	})
	for _, want := range []string{"后台导演三层规划可读区", "source: DirectorPlan visible sections", "limit: 12288 bytes", "外门逆袭", "学院比拼压力"} {
		if !strings.Contains(output, want) {
			t.Fatalf("runtime context should include %q:\n%s", want, output)
		}
	}
	for _, want := range []string{"故事导演 Markdown 策略提示", "source: StoryDirector.strategy.prompt_markdown", "limit: 4000 bytes", "结构化导演策略", "避免连续两回合"} {
		if !strings.Contains(output, want) {
			t.Fatalf("runtime context should include strategy prompt %q:\n%s", want, output)
		}
	}
}

func TestInteractiveDirectorPromptEditsDirectorPlanFiles(t *testing.T) {
	system := BuildInteractiveDirectorSystemInstruction()
	instruction := InteractiveDirectorInstruction(InteractiveDirectorPromptInput{
		Title:                       "外门逆袭",
		Origin:                      "主角被同门轻视",
		StoryTellerID:               "classic",
		BranchID:                    "main",
		DirectorPlanPaths:           "/tmp/mainline.md\n/tmp/current-event.md\n/tmp/next-branches.md",
		DirectorPlanDocs:            `{"mainline":"## 正文Agent可读 / Prose-agent visible","current_event":"## 正文Agent可读 / Prose-agent visible","next_branches":"## 正文Agent可读 / Prose-agent visible"}`,
		PlanningTemplates:           `{"mainline":"# 大方向 / Mainline"}`,
		BranchPlanningTurns:         5,
		TurnAuditJSON:               `{"turn_brief":{"turn_goal":"公开比试"}}`,
		TurnHistory:                 "第 1 回合剧情：主角报名。",
		StoryMemorySummary:          "主角仍被低估。",
		StoryDirectorStrategyPrompt: "- 伏笔回收前至少给一次可感知征兆。",
		DirectorEventCatalog:        `[{"id":"face_slap","category":"打脸"}]`,
	})
	for name, output := range map[string]string{"system": system, "instruction": instruction} {
		for _, want := range []string{"read_file", "write_file", "edit_file", "不负责续写", "RuleResolution", "Director private"} {
			if !strings.Contains(output, want) {
				t.Fatalf("%s director prompt should include %q:\n%s", name, want, output)
			}
		}
		if strings.Contains(output, "故事正文\n") {
			t.Fatalf("%s director prompt should not ask for story prose:\n%s", name, output)
		}
	}
	for _, want := range []string{"mainline.md", "current-event.md", "next-branches.md", "目标 / Goal", "分支处理 / Branch Handling", "打脸", "事件目录", "template"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("director instruction should include %q:\n%s", want, instruction)
		}
	}
	for _, want := range []string{"故事导演 Markdown 策略提示", "source: StoryDirector.strategy.prompt_markdown", "limit: 4000 bytes", "结构化导演策略", "伏笔回收前"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("director instruction should include strategy prompt %q:\n%s", want, instruction)
		}
	}
}
