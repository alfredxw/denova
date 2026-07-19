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
		}),
		"director maintenance": InteractiveDirectorInstruction(InteractiveDirectorPromptInput{
			Title:         "末日开端",
			Origin:        "主角醒来发现世界已末日",
			StoryTellerID: "classic",
			BranchID:      "main",
			TurnHistory:   "第 1 回合剧情：门后传来低沉的风声。",
			TurnAuditJSON: `{"user_action":"我点燃火把","narrative":"火光照亮了墙上的新线索。"}`,
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
	turn := InteractiveStoryTurnInstruction("我推开门", "", "")
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
	for _, want := range []string{"very_easy/easy/normal/hard/very_hard", "rule 可省略", "dice_check", "固定 d20", "difficulty_guidance", "state_effect_guidance", "state_bindings", "binding_id"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt should include prepare_interactive_turn enum protocol %q:\n%s", want, system)
		}
	}
	for _, want := range []string{"不是所有用户行动都需要检定", "低风险试探", "应由你直接裁定", "只有当本回合存在明确风险", "需要固定规则裁定时，才调用 prepare_interactive_turn"} {
		if !strings.Contains(turn, want) {
			t.Fatalf("turn prompt should include DM-style check rule %q:\n%s", want, turn)
		}
	}
	for _, want := range []string{"very_easy/easy/normal/hard/very_hard", "不要使用 medium 或 moderate", "difficulty_guidance", "state_effect_guidance", "固定 d20", "state_bindings"} {
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

func TestInteractiveStoryPromptRequiresStoryContextUpdateEveryTurn(t *testing.T) {
	system := BuildInteractiveStorySystemInstruction(InteractiveStorySystemInstructionInput{})
	turn := InteractiveStoryTurnInstruction("我推开门", "", "")
	for name, output := range map[string]string{"system": system, "turn": turn} {
		for _, want := range []string{"每回合", "state_changes", "actor_id=story", "field_id=当前事件", "当前详细地点"} {
			if !strings.Contains(output, want) {
				t.Fatalf("%s prompt should require story context field %q:\n%s", name, want, output)
			}
		}
		for _, forbidden := range []string{"replace /story/当前事件", "/story/当前详细地点", "patches"} {
			if strings.Contains(output, forbidden) {
				t.Fatalf("%s prompt should not require model-authored state path %q:\n%s", name, forbidden, output)
			}
		}
	}
	if !strings.Contains(system, "story_context") {
		t.Fatalf("system prompt should name the story_context template:\n%s", system)
	}
}

func TestInteractiveStoryPromptUsesConfiguredChoiceCountAndSimplifiedResult(t *testing.T) {
	system := BuildInteractiveStorySystemInstruction(InteractiveStorySystemInstructionInput{ChoiceCount: 7})
	runtime := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{ChoiceCount: 7})
	for name, output := range map[string]string{"system": system, "runtime": runtime} {
		if !strings.Contains(output, "恰好 7 个") {
			t.Fatalf("%s prompt should use the story choice count:\n%s", name, output)
		}
		for _, forbidden := range []string{"scene_result", "fact_candidates", "plan_signals", "expected_state_changes"} {
			if strings.Contains(output, forbidden) {
				t.Fatalf("%s prompt still exposes removed TurnResult field %q:\n%s", name, forbidden, output)
			}
		}
	}
	if !strings.Contains(system, "submit_interactive_turn") || strings.Contains(system, "submit_actor_state_patches") || strings.Contains(system, "submit_choices") {
		t.Fatalf("system prompt should expose one unified turn submission tool:\n%s", system)
	}
	if !strings.Contains(system, "state_changes") || !strings.Contains(system, "actor_id") || !strings.Contains(system, "field_id") || strings.Contains(system, "JSON Pointer") {
		t.Fatalf("system prompt should use structured state fields rather than model-authored paths:\n%s", system)
	}
}

func TestInteractiveDirectorPromptReadsCustomActorStateWithoutWritingIt(t *testing.T) {
	system := BuildInteractiveDirectorSystemInstruction()
	instruction := InteractiveDirectorInstruction(InteractiveDirectorPromptInput{
		Title:            "百日终末",
		Origin:           "世界将在一百天后毁灭",
		StoryTellerID:    "classic",
		BranchID:         "main",
		ActorStateSchema: "templates: world_state, heroine_route",
		TurnHistory:      "第 1 回合剧情：钟声提前响起。",
		TurnAuditJSON:    `{"narrative":"钟声提前响起。"}`,
	})
	combined := system + "\n" + instruction
	for _, want := range []string{
		"world_state",
		"heroine_route",
		"只能读取已提交的 Actor State",
		"不得写 Actor State",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("director prompt should describe customizable state tables %q:\n%s", want, combined)
		}
	}
	for _, forbidden := range []string{
		"主角用 protagonist",
		"重要人物用 important_character",
		"敌人/怪物/规则实体用 opponent",
		"唯一合法分类",
		"apply_actor_state_patch",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("director prompt should not hard-code fixed actor-state categories %q:\n%s", forbidden, combined)
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
		DirectorPlanVisible:         "# 正文 Agent 简报\n\n## 当前目标与可见钩子\n外门逆袭\n\n## 已公开信息与可发现线索\n学院比拼压力",
		ActorState:                  `{"source":{"path":"Snapshot.State.actors"},"actors":{"protagonist":{"traits":[{"name":"隐脉"}]}}}`,
		StoryDirectorStrategyPrompt: "- 避免连续两回合使用同类型突发事件。",
	})
	for _, want := range []string{"正文 Agent 简报", "source: agent-brief.md", "bounded", "外门逆袭", "学院比拼压力"} {
		if !strings.Contains(output, want) {
			t.Fatalf("runtime context should include %q:\n%s", want, output)
		}
	}
	for _, want := range []string{"故事导演 Markdown 策略提示", "source: StoryDirector.strategy.prompt_markdown", "bounded", "结构化导演策略", "避免连续两回合"} {
		if !strings.Contains(output, want) {
			t.Fatalf("runtime context should include strategy prompt %q:\n%s", want, output)
		}
	}
	for _, want := range []string{"Actor 状态手册", "source: Snapshot.State.actors + effective Actor schema", "bounded Markdown", "隐脉"} {
		if !strings.Contains(output, want) {
			t.Fatalf("runtime context should include Actor state %q:\n%s", want, output)
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
		DirectorPlanDocs:            "## 文件：director.md\n\n# 导演私密规划\n\n## 文件：agent-brief.md\n\n# 正文 Agent 简报\n\n## 文件：lore-context.md\n\n# 分支资料工作集",
		PlanningTemplates:           `{"plan":"# 导演私密规划","agent_brief":"# 正文 Agent 简报"}`,
		LoreContext:                 "## 资料库索引（source: lore index, bounded）\n- 沈凝 / 重要角色\n- 青岚盟 / 重要势力",
		BranchPlanningTurns:         5,
		TurnAuditJSON:               `{"turn_result":{"director_update":{"needed":true,"reason":"公开比试"}}}`,
		TurnHistory:                 "第 1 回合剧情：主角报名。",
		StoryDirectorStrategyPrompt: "- 伏笔回收前至少给一次可感知征兆。",
		DirectorEventCatalog:        `[{"id":"face_slap","category":"打脸"}]`,
	})
	for name, output := range map[string]string{"system": system, "instruction": instruction} {
		for _, want := range []string{"submit_director_plan_update", "不负责续写", "RuleResolution", "agent-brief.md", "keep", "patch", "replan"} {
			if !strings.Contains(output, want) {
				t.Fatalf("%s director prompt should include %q:\n%s", name, want, output)
			}
		}
		for _, forbidden := range []string{"read_file", "write_file", "edit_file"} {
			if strings.Contains(output, forbidden) {
				t.Fatalf("%s director prompt should not expose obsolete file tool %q:\n%s", name, forbidden, output)
			}
		}
		if strings.Contains(output, "故事正文\n") {
			t.Fatalf("%s director prompt should not ask for story prose:\n%s", name, output)
		}
	}
	for _, want := range []string{"director.md", "agent-brief.md", "lore-context.md", "资料库导演上下文", "资料库优先", "核心角色", "信息密度", "阶段目标与隐藏钩子", "沈凝", "青岚盟", "打脸", "事件目录", "template"} {
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
