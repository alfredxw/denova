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
