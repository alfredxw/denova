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
}
