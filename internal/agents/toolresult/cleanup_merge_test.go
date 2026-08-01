package toolresult_test

import (
	"denova/internal/agents/toolresult"
	"testing"
)

func TestMergeToolResultCleanupIsOrderedIdempotentAndStorageNeutral(t *testing.T) {
	merged, err := toolresult.MergeCleanup(
		[]toolresult.PersistedReplacement{{MessageIndex: 4, ToolCallID: "old", Placeholder: "old receipt"}},
		[]toolresult.CleanupReplacement{
			{MessageIndex: 4, ToolCallID: "old", Placeholder: "updated receipt", OriginalTokens: 100, PlaceholderTokens: 10},
			{MessageIndex: 1, ToolCallID: "new", Placeholder: "new receipt", OriginalTokens: 80, PlaceholderTokens: 20},
		},
		0, 6, 90, 120,
	)
	if err != nil {
		t.Fatal(err)
	}
	if merged.SourceStart != 1 || merged.SourceEnd != 6 || merged.ReclaimedTokens != 150 || len(merged.Replacements) != 2 ||
		merged.Replacements[0].MessageIndex != 1 || merged.Replacements[1].Placeholder != "updated receipt" {
		t.Fatalf("unexpected merged cleanup: %#v", merged)
	}
}

func TestMergeToolResultCleanupRejectsOutOfBoundsProjection(t *testing.T) {
	_, err := toolresult.MergeCleanup(nil, []toolresult.CleanupReplacement{{
		MessageIndex: 2, ToolCallID: "outside", Placeholder: "receipt",
	}}, 3, 5, 0, 0)
	if err == nil {
		t.Fatal("out-of-bounds cleanup projection was accepted")
	}
}
