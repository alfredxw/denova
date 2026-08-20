package agent

import (
	"strings"
	"testing"

	"denova/internal/interactive"
)

func TestAlignMemoryRecordEntitiesRewritesDriftedSpelling(t *testing.T) {
	roster := []interactive.MemoryEntity{
		{Name: "林舟", Mentions: 9},
		{Name: "蚀骨剑", Mentions: 4},
	}
	records := []interactive.NarrativeMemoryRecord{
		{ID: "r1", Kind: interactive.MemoryKindObjectState, Subject: "蚀 骨 剑", Object: "林 舟"},
		{ID: "r2", Kind: interactive.MemoryKindBeat, Subject: "林舟"},
	}

	aligned, log := alignMemoryRecordEntities(records, roster)

	if aligned[0].Subject != "蚀骨剑" || aligned[0].Object != "林舟" {
		t.Fatalf("drifted spellings should be rewritten: %#v", aligned[0])
	}
	// 已经是权威写法的记录不该被改写,也不该留痕。
	if aligned[1].Subject != "林舟" {
		t.Fatalf("canonical spelling must be left alone: %#v", aligned[1])
	}
	if len(log) != 2 {
		t.Fatalf("expected exactly 2 rewrites, got %d: %#v", len(log), log)
	}
	for _, item := range log {
		if item.RecordID != "r1" {
			t.Fatalf("only r1 should be rewritten, got %#v", item)
		}
		if item.From == item.To || item.To == "" {
			t.Fatalf("rewrite log must show a real change: %#v", item)
		}
	}
}

func TestAlignMemoryRecordEntitiesLeavesUnknownEntitiesAlone(t *testing.T) {
	roster := []interactive.MemoryEntity{{Name: "林舟"}}
	records := []interactive.NarrativeMemoryRecord{
		// 名册里没有的全新实体必须原样保留 —— 对齐层只做确定性归一,
		// 不猜测"这个新名字大概是指谁"。
		{ID: "r1", Kind: interactive.MemoryKindBeat, Subject: "无名旅人"},
		// 语义别名不属于这一层的职责:它归一化后与任何名册项都不同键。
		{ID: "r2", Kind: interactive.MemoryKindObjectState, Subject: "那把剑", Object: "他"},
	}

	aligned, log := alignMemoryRecordEntities(records, roster)

	if aligned[0].Subject != "无名旅人" {
		t.Fatalf("new entity must survive untouched, got %q", aligned[0].Subject)
	}
	if aligned[1].Subject != "那把剑" || aligned[1].Object != "他" {
		t.Fatalf("semantic aliases are the prompt layer's job, not this one: %#v", aligned[1])
	}
	if len(log) != 0 {
		t.Fatalf("nothing should be rewritten, got %#v", log)
	}
}

func TestAlignMemoryRecordEntitiesNoRoster(t *testing.T) {
	records := []interactive.NarrativeMemoryRecord{{ID: "r1", Subject: "林 舟"}}
	// 首个回合没有名册可对齐,记录必须原样通过而不是被清空。
	aligned, log := alignMemoryRecordEntities(records, nil)
	if len(aligned) != 1 || aligned[0].Subject != "林 舟" {
		t.Fatalf("records must pass through untouched: %#v", aligned)
	}
	if log != nil {
		t.Fatalf("expected no rewrite log, got %#v", log)
	}
}

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
	// 名册段必须明确要求照抄写法,否则模型会继续沿用正文里的代称。
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
