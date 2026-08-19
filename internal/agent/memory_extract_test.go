package agent

import (
	"strings"
	"testing"

	"denova/internal/interactive"
)

func TestParseMemoryExtractionContentValidatesAndDrops(t *testing.T) {
	content := `{"records":[
		{"kind":"knowledge","subject":"岚","object":"剑的来历","text":"岚知道剑的来历。","evidence":"警告不要提剑的来历"},
		{"kind":"haha","subject":"x","text":"y","evidence":"z"},
		{"kind":"promise","subject":"剑","text":"来历未揭示。","evidence":"神色微变","status":"open"},
		{"kind":"promise","subject":"剑","text":"来历已揭示。","evidence":"说出来了","status":"maybe"},
		{"kind":"object_state","subject":"蚀骨剑","object":"林舟","text":"剑在林舟手中。","evidence":"得到蚀骨剑","valid_to":"turn_999"},
		{"kind":"beat","subject":"岚","text":"","evidence":"守护"},
		{"kind":"reveal","subject":"石台","text":"石台是石像头部。","evidence":"低潮退去"}
	]}`
	result, err := parseMemoryExtractionContent(content, "turn_src")
	if err != nil {
		t.Fatal(err)
	}
	// 合法:knowledge / promise(open) / reveal = 3 条。
	if len(result.Records) != 3 {
		t.Fatalf("records: %#v", result.Records)
	}
	// 丢弃:kind 非法 / status 非法 / valid_to 非本回合 / text 空 = 4 条。
	reasons := map[string]int{}
	for _, dropped := range result.Dropped {
		reasons[dropped.Reason]++
	}
	if reasons["kind_invalid"] != 1 || reasons["status_invalid"] != 1 || reasons["valid_to_not_source_turn"] != 1 || reasons["text_empty"] != 1 {
		t.Fatalf("drop reasons: %#v", reasons)
	}
	for _, record := range result.Records {
		if record.ValidTo != "" {
			t.Fatalf("kept record should not carry valid_to: %#v", record)
		}
	}
}

func TestParseMemoryExtractionContentMaxRecords(t *testing.T) {
	records := make([]string, 0, memoryExtractionMaxRecords+3)
	for i := 0; i < memoryExtractionMaxRecords+3; i++ {
		records = append(records, `{"kind":"beat","subject":"s`+string(rune('a'+i))+`","text":"t","evidence":"e"}`)
	}
	content := `{"records":[` + strings.Join(records, ",") + `]}`
	result, err := parseMemoryExtractionContent(content, "turn_src")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != memoryExtractionMaxRecords {
		t.Fatalf("records: %d", len(result.Records))
	}
	maxDropped := 0
	for _, dropped := range result.Dropped {
		if dropped.Reason == "max_records" {
			maxDropped++
		}
	}
	if maxDropped != 3 {
		t.Fatalf("max_records drops: %d", maxDropped)
	}
}

func TestBuildMemoryExtractionInstructionBoundedAndCarriesPromises(t *testing.T) {
	input := MemoryExtractionInput{
		StoryID:  "s1",
		BranchID: "main",
		Turn: interactive.TurnEvent{
			ID:        "turn_1",
			User:      "前往银月港",
			Narrative: "林舟在银月港见到岚,得到蚀骨剑,岚神色微变。",
		},
		OpenPromises: []string{"剑的来历未揭示 (turn turn_0)"},
	}
	instruction, err := buildMemoryExtractionInstruction(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"前往银月港", "蚀骨剑", "剑的来历未揭示", "turn_1", "knowledge", "promise", `"records"`} {
		if !strings.Contains(instruction, needle) {
			t.Fatalf("instruction missing %q: %s", needle, instruction)
		}
	}
	if len(instruction) > memoryExtractionInputMaxBytes {
		t.Fatalf("instruction exceeds bound: %d", len(instruction))
	}

	// 超长叙事被截断到指令仍可组装(不整体报错)。
	input.Turn.Narrative = strings.Repeat("很长", 100000)
	instruction, err = buildMemoryExtractionInstruction(input)
	if err != nil {
		t.Fatalf("long narrative should still build: %v", err)
	}
	if len(instruction) > memoryExtractionInputMaxBytes {
		t.Fatalf("truncated instruction exceeds bound: %d", len(instruction))
	}
}
