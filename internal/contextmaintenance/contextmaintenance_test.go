package contextmaintenance

import "testing"

func TestNormalizeToolResultCleanupCanonicalizesReplacements(t *testing.T) {
	value, err := NormalizeToolResultCleanup(ToolResultCleanup{
		AgentKind: " ide ", SourceStart: 10, SourceEnd: 20,
		Replacements: []ToolResultReplacement{
			{MessageIndex: 15, ToolCallID: " call-b ", Placeholder: "second"},
			{MessageIndex: 11, ToolCallID: "call-a", Placeholder: "first"},
		},
		ReclaimedTokens: 100, TriggeredAtUsage: 900, WarmSuffixTokens: 50, RendererVersion: " v1 ",
	}, func() string { return "cleanup-1" })
	if err != nil {
		t.Fatal(err)
	}
	if value.ID != "cleanup-1" || value.AgentKind != "ide" || value.RendererVersion != "v1" || value.EarliestChanged != 11 {
		t.Fatalf("normalized cleanup = %#v", value)
	}
	if value.Replacements[0].MessageIndex != 11 || value.Replacements[1].ToolCallID != "call-b" {
		t.Fatalf("replacement order = %#v", value.Replacements)
	}
}

func TestNormalizeToolResultCleanupRejectsDuplicateTarget(t *testing.T) {
	_, err := NormalizeToolResultCleanup(ToolResultCleanup{
		ID: "cleanup-1", SourceStart: 0, SourceEnd: 2, ReclaimedTokens: 10, RendererVersion: "v1",
		Replacements: []ToolResultReplacement{
			{MessageIndex: 1, ToolCallID: "call-a", Placeholder: "first"},
			{MessageIndex: 1, ToolCallID: "call-b", Placeholder: "second"},
		},
	}, nil)
	if err == nil {
		t.Fatal("duplicate replacement target should be rejected")
	}
}

func TestAdvanceCompactionHealthCountsOnlySameStructure(t *testing.T) {
	previous := &CompactionHealth{AgentKind: "ide", StructureFingerprint: "structure-a", ConsecutiveFailures: 2}
	next := AdvanceCompactionHealth(previous, CompactionHealth{
		AgentKind: "ide", StructureFingerprint: "structure-a", Outcome: CompactionHealthFailure,
	})
	if next.ConsecutiveFailures != 3 {
		t.Fatalf("same structure failures = %d, want 3", next.ConsecutiveFailures)
	}
	next = AdvanceCompactionHealth(previous, CompactionHealth{
		AgentKind: "interactive_story", StructureFingerprint: "structure-a", Outcome: CompactionHealthFailure,
	})
	if next.ConsecutiveFailures != 1 {
		t.Fatalf("different agent failures = %d, want 1", next.ConsecutiveFailures)
	}
	next = AdvanceCompactionHealth(previous, CompactionHealth{Outcome: CompactionHealthSuccess})
	if next.ConsecutiveFailures != 0 {
		t.Fatalf("success failures = %d, want 0", next.ConsecutiveFailures)
	}
}
