package agent

import (
	"strings"
	"testing"

	"denova/internal/interactive"
)

func TestBuildMemoryExtractionInstructionIncludesRoster(t *testing.T) {
	instruction, err := buildMemoryExtractionInstruction(MemoryExtractionInput{
		Turn: interactive.TurnEvent{ID: "turn_1", User: "查看剑", Narrative: "他握紧了那把剑。"},
		Roster: []interactive.MemoryEntity{
			{Name: "蚀骨剑", Mentions: 3, Kinds: []string{interactive.MemoryKindObjectState}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, "已知实体名册") || !strings.Contains(instruction, "蚀骨剑") {
		t.Fatalf("roster section missing from instruction:\n%s", instruction)
	}
	// 名册段必须明确要求照抄写法,否则模型会继续沿用正文里的代称 ——
	// 语义别名只有读得懂正文的抽取器能解,写入路径的确定性对齐解不了。
	if !strings.Contains(instruction, "原样照抄") {
		t.Fatalf("roster instruction must demand canonical spelling:\n%s", instruction)
	}
}

func TestBuildMemoryExtractionInstructionWithoutRoster(t *testing.T) {
	instruction, err := buildMemoryExtractionInstruction(MemoryExtractionInput{
		Turn: interactive.TurnEvent{ID: "turn_1", User: "开场", Narrative: "故事开始。"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(instruction, "已知实体名册") {
		t.Fatalf("empty roster should not render a section:\n%s", instruction)
	}
}
