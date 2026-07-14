package prompts

import (
	"strings"
	"testing"
)

func TestInteractiveStateSchemaAdapterPromptTeachesIncrementalBatch(t *testing.T) {
	prompt := BuildInteractiveStateSchemaAdapterSystemInstruction()
	for _, want := range []string{
		"稳定前缀", "全部启用的常驻资料正文", "item_id", "accepted", "rejected", "blocked", "finalize=true",
		"confirmed", "inferred", "default", "合理推测", "剧透", "只提交修正后的 protagonist-life",
		"sources.opening_turn_id", "actor_ops", "value_source 由后端",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("state schema Batch prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `\"id\":\"opening-turn\"`) {
		t.Fatal("state schema example must not teach a fabricated opening source ID")
	}
}
