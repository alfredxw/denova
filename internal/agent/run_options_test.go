package agent

import (
	"strings"
	"testing"
)

func TestRunOptionsIdleTimeoutDefaultsToNoLimit(t *testing.T) {
	options := RunOptions{}.normalized("")
	if options.IdleTimeout != 0 {
		t.Fatalf("idle timeout = %s, want no limit", options.IdleTimeout)
	}
}

func TestRunOptionsIdleTimeoutNegativeDisablesTimeout(t *testing.T) {
	options := RunOptions{IdleTimeout: -1}.normalized("")
	if options.IdleTimeout != 0 {
		t.Fatalf("idle timeout = %s, want disabled zero duration", options.IdleTimeout)
	}
}

func TestRunOptionsNormalizesInteractiveTraceMetadata(t *testing.T) {
	options := RunOptions{
		StoryID:         " story-1 ",
		BranchID:        " main ",
		TurnID:          " turn-1 ",
		MaintenanceTask: " director_plan_update ",
	}.normalized("")

	if options.StoryID != "story-1" || options.BranchID != "main" || options.TurnID != "turn-1" || options.MaintenanceTask != "director_plan_update" {
		t.Fatalf("interactive trace metadata was not normalized: %#v", options)
	}
}

func TestRunTraceMetadataReporterFillsCommittedTurnAndBoundsValues(t *testing.T) {
	conversation := &contextLedgerReportingConversation{metadata: RunTraceMetadata{
		StoryID:         strings.Repeat("s", runTraceMetadataValueMaxBytes+20),
		BranchID:        "branch-committed",
		TurnID:          "turn-committed",
		MaintenanceTask: "director_plan_update",
	}}
	metadata := runTraceMetadataForConversation(RunOptions{StoryID: "story-initial", BranchID: "main"}, conversation)

	if len(metadata.StoryID) > runTraceMetadataValueMaxBytes || metadata.BranchID != "branch-committed" || metadata.TurnID != "turn-committed" || metadata.MaintenanceTask != "director_plan_update" {
		t.Fatalf("committed run metadata was not merged and bounded: %#v", metadata)
	}
}
