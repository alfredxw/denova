package interactive

import (
	"strings"
	"testing"
)

func TestOpeningGameStateSchemaInstructionRequiresEvidenceDrivenFields(t *testing.T) {
	policy := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeAdaptTemplate}
	instruction := OpeningGameStateSchemaInstruction(StoryMeta{
		StateSchemaPolicy: &policy,
		StateSchemaInitialization: &StateSchemaInitializationStatus{
			Status: StateSchemaInitializationWaitingOpening,
		},
	})
	for _, required := range []string{
		"默认状态系统只预置最通用的等级与生命",
		"必须主动",
		"固定 d20 只是裁定随机性的方式",
		"不能作为 D&D 状态字段的依据",
		"开局事实、已读取 Lore 或当前 TRPG state_binding",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("opening state schema instruction must contain %q: %s", required, instruction)
		}
	}
}
