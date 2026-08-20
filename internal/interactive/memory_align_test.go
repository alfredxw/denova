package interactive

import (
	"testing"
)

func TestAlignRecordsToRosterRewritesDriftedSpelling(t *testing.T) {
	roster := []MemoryEntity{
		{Name: "林舟", Mentions: 9},
		{Name: "蚀骨剑", Mentions: 4},
	}
	records := []NarrativeMemoryRecord{
		{ID: "r1", Kind: MemoryKindObjectState, Subject: "蚀 骨 剑", Object: "林 舟"},
		{ID: "r2", Kind: MemoryKindBeat, Subject: "林舟"},
	}

	log := alignRecordsToRoster(records, roster)

	if records[0].Subject != "蚀骨剑" || records[0].Object != "林舟" {
		t.Fatalf("drifted spellings should be rewritten: %#v", records[0])
	}
	// 已经是权威写法的记录不该被改写,也不该留痕。
	if records[1].Subject != "林舟" {
		t.Fatalf("canonical spelling must be left alone: %#v", records[1])
	}
	if len(log) != 2 {
		t.Fatalf("expected exactly 2 rewrites, got %d: %#v", len(log), log)
	}
	fields := map[string]bool{}
	for _, item := range log {
		if item.RecordID != "r1" {
			t.Fatalf("only r1 should be rewritten, got %#v", item)
		}
		if item.From == item.To || item.To == "" {
			t.Fatalf("rewrite log must show a real change: %#v", item)
		}
		fields[item.Field] = true
	}
	if !fields["subject"] || !fields["object"] {
		t.Fatalf("both fields should be logged with their own name: %#v", log)
	}
}

func TestAlignRecordsToRosterLeavesUnknownEntitiesAlone(t *testing.T) {
	roster := []MemoryEntity{{Name: "林舟"}}
	records := []NarrativeMemoryRecord{
		// 名册里没有的全新实体必须原样保留 —— 对齐层只做确定性归一,
		// 不猜测"这个新名字大概是指谁"。
		{ID: "r1", Kind: MemoryKindBeat, Subject: "无名旅人"},
		// 语义别名不属于这一层的职责:它归一化后与任何名册项都不同键。
		{ID: "r2", Kind: MemoryKindObjectState, Subject: "那把剑", Object: "他"},
	}

	log := alignRecordsToRoster(records, roster)

	if records[0].Subject != "无名旅人" {
		t.Fatalf("new entity must survive untouched, got %q", records[0].Subject)
	}
	if records[1].Subject != "那把剑" || records[1].Object != "他" {
		t.Fatalf("semantic aliases are the prompt layer's job, not this one: %#v", records[1])
	}
	if len(log) != 0 {
		t.Fatalf("nothing should be rewritten, got %#v", log)
	}
}

func TestAlignRecordsToRosterNoRoster(t *testing.T) {
	records := []NarrativeMemoryRecord{{ID: "r1", Subject: "林 舟"}}
	// 首个回合没有名册可对齐,记录必须原样通过而不是被清空。
	if log := alignRecordsToRoster(records, nil); log != nil {
		t.Fatalf("expected no rewrite log, got %#v", log)
	}
	if records[0].Subject != "林 舟" {
		t.Fatalf("records must pass through untouched: %#v", records)
	}
}

// TestAppendNarrativeMemoryAlignsEntities 钉住对齐是写入路径的不变量:
// 不经过抽取器的注入(手动、测试夹具、未来的 API)同样会被对齐。
func TestAppendNarrativeMemoryAlignsEntities(t *testing.T) {
	store, story, first, _, _ := memoryTestStory(t)

	event, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: first.ID,
		Records: []NarrativeMemoryRecord{
			{ID: "manual_1", Kind: MemoryKindBeat, Subject: "蚀 骨 剑", Text: "剑再度出鞘。", Evidence: "出鞘", ValidFrom: first.ID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := event.Records[0].Subject; got != "蚀骨剑" {
		t.Fatalf("manual injection should be aligned to the roster, got %q", got)
	}
	if event.Trace == nil || len(event.Trace.AlignedEntities) != 1 {
		t.Fatalf("alignment must be traced even when the caller supplied no trace: %#v", event.Trace)
	}
	if item := event.Trace.AlignedEntities[0]; item.From != "蚀 骨 剑" || item.To != "蚀骨剑" {
		t.Fatalf("trace should record the rewrite: %#v", item)
	}
}

// TestAppendNarrativeMemoryDoesNotSelfAnchor 确认名册取的是本事件之前的记录:
// 一个事件自己的记录不能成为自己的权威来源。
func TestAppendNarrativeMemoryDoesNotSelfAnchor(t *testing.T) {
	store, story, first, _, _ := memoryTestStory(t)

	event, err := store.AppendNarrativeMemory(story.ID, "main", NarrativeMemoryEvent{
		SourceTurnID: first.ID,
		Records: []NarrativeMemoryRecord{
			// 同一事件内的两种写法都是新实体,谁也不该被改写成另一个。
			{ID: "new_1", Kind: MemoryKindBeat, Subject: "新 人", Text: "新人登场。", Evidence: "登场", ValidFrom: first.ID},
			{ID: "new_2", Kind: MemoryKindBeat, Subject: "新人", Text: "新人开口。", Evidence: "开口", ValidFrom: first.ID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Records[0].Subject != "新 人" || event.Records[1].Subject != "新人" {
		t.Fatalf("records within one event must not align against each other: %#v", event.Records)
	}
}
