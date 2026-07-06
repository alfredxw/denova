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
		"director maintenance": InteractiveDirectorInstruction(InteractiveDirectorPromptInput{
			Title:             "末日开端",
			Origin:            "主角醒来发现世界已末日",
			StoryTellerID:     "classic",
			BranchID:          "main",
			StoryMemorySchema: "## important_character",
			StoryMemory:       "林川仍在黄泉酒馆。",
			TurnHistory:       "第 1 回合剧情：门后传来低沉的风声。",
			TurnAuditJSON:     `{"user_action":"我点燃火把","narrative":"火光照亮了墙上的新线索。"}`,
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
	for _, want := range []string{"不是所有用户行动都需要检定", "普通观察", "低风险试探", "只有当行动存在明确风险", "需要固定规则裁定时，才调用 prepare_interactive_turn", "不要为了引用事件 ID"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt should include DM-style check rule %q:\n%s", want, system)
		}
	}
	for _, want := range []string{"very_easy/easy/normal/hard/very_hard", "rule 可省略", "dice_check", "1d20"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt should include prepare_interactive_turn enum protocol %q:\n%s", want, system)
		}
	}
	for _, want := range []string{"不是所有用户行动都需要检定", "低风险试探", "应由你直接裁定", "只有当本回合存在明确风险", "需要固定规则裁定时，才调用 prepare_interactive_turn"} {
		if !strings.Contains(turn, want) {
			t.Fatalf("turn prompt should include DM-style check rule %q:\n%s", want, turn)
		}
	}
	for _, want := range []string{"very_easy/easy/normal/hard/very_hard", "不要使用 medium 或 moderate"} {
		if !strings.Contains(turn, want) {
			t.Fatalf("turn prompt should include prepare_interactive_turn enum protocol %q:\n%s", want, turn)
		}
	}
	if strings.Contains(turn, "如果本回合涉及数值、骰子、资源、关系、词条、失败等级或终局候选，请调用 prepare_interactive_turn") {
		t.Fatalf("turn prompt should not force checks for every numeric/resource mention:\n%s", turn)
	}
	for _, forbidden := range []string{"优先引用对应事件卡", "type_name/name"} {
		if strings.Contains(system, forbidden) {
			t.Fatalf("system prompt should not ask prose agent to trigger raw event cards %q:\n%s", forbidden, system)
		}
	}
}

func TestInteractiveStoryPromptRequiresGlobalStyleReferenceRead(t *testing.T) {
	system := BuildInteractiveStorySystemInstruction(InteractiveStorySystemInstructionInput{
		StyleRules: []StyleRule{
			{Global: true, StyleReferences: []StyleReference{{Name: "全局克制", Path: "/tmp/.denova/styles/global.md", DisplayPath: ".denova/styles/global.md"}}},
			{Scene: "激烈打斗", StyleReferences: []StyleReference{{Name: "短促打斗", Path: "/tmp/.denova/styles/fight.md", DisplayPath: ".denova/styles/fight.md"}}},
		},
	})

	for _, want := range []string{
		"全局文风参考：所有正文生成默认生效",
		"path: /tmp/.denova/styles/global.md",
		"互动故事下一回合正文生成时",
		"编制故事正文前必须先用 read_file 读取这些全局参考文件",
		"分场景文风参考仍根据当前章节内容、互动场景或本轮 # 场景选择",
		"不要强行选择分场景参考",
		"不要照搬其中的人物、情节或设定",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("interactive system prompt should include style reference rule %q:\n%s", want, system)
		}
	}
}

func TestInteractiveStoryRuntimeContextIncludesBoundedDirectorPlanVisibleSections(t *testing.T) {
	output := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
		ReplyTargetChars:            800,
		DirectorPlanVisible:         "## 正文Agent可读\n\n### 阶段钩子与阅读欲望\n外门逆袭\n\n### 信息揭示与线索密度\n学院比拼压力",
		StoryDirectorStrategyPrompt: "- 避免连续两回合使用同类型突发事件。",
	})
	for _, want := range []string{"后台导演规划可读区", "source: director.md visible section", "bounded", "外门逆袭", "学院比拼压力"} {
		if !strings.Contains(output, want) {
			t.Fatalf("runtime context should include %q:\n%s", want, output)
		}
	}
	for _, want := range []string{"故事导演 Markdown 策略提示", "source: StoryDirector.strategy.prompt_markdown", "bounded", "结构化导演策略", "避免连续两回合"} {
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
		DirectorPlanPaths:           "/tmp/director.md",
		DirectorPlanDocs:            `{"plan":"## 正文Agent可读"}`,
		PlanningTemplates:           `{"plan":"# 导演规划"}`,
		LoreContext:                 "## 资料库索引（source: lore index, bounded）\n- 沈凝 / 重要角色\n- 青岚盟 / 重要势力",
		BranchPlanningTurns:         5,
		TurnAuditJSON:               `{"turn_brief":{"turn_goal":"公开比试"}}`,
		TurnHistory:                 "第 1 回合剧情：主角报名。",
		StoryMemorySummary:          "主角仍被低估。",
		StoryDirectorStrategyPrompt: "- 伏笔回收前至少给一次可感知征兆。",
		DirectorEventCatalog:        `[{"id":"face_slap","category":"打脸"}]`,
	})
	for name, output := range map[string]string{"system": system, "instruction": instruction} {
		for _, want := range []string{"read_file", "write_file", "edit_file", "不负责续写", "RuleResolution", "后台导演私密"} {
			if !strings.Contains(output, want) {
				t.Fatalf("%s director prompt should include %q:\n%s", name, want, output)
			}
		}
		if strings.Contains(output, "故事正文\n") {
			t.Fatalf("%s director prompt should not ask for story prose:\n%s", name, output)
		}
	}
	for _, want := range []string{"director.md", "资料库导演上下文", "资料库优先", "核心角色", "信息密度", "阶段钩子", "沈凝", "青岚盟", "打脸", "事件目录", "template"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("director instruction should include %q:\n%s", want, instruction)
		}
	}
	for _, forbidden := range []string{"mainline.md", "current-event.md", "next-branches.md"} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("director instruction should not mention legacy doc %q:\n%s", forbidden, instruction)
		}
	}
	for _, want := range []string{"故事导演 Markdown 策略提示", "source: StoryDirector.strategy.prompt_markdown", "bounded", "结构化导演策略", "伏笔回收前"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("director instruction should include strategy prompt %q:\n%s", want, instruction)
		}
	}
}
