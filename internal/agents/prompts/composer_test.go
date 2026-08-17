package prompts

import (
	"strings"
	"testing"
	"unicode/utf8"

	"denova/config"
)

func TestSystemPromptComposerRejectsRequiredFragmentOverflow(t *testing.T) {
	cfg := systemPromptBudgetConfig(8, 1024, 16, 256)
	_, err := composeSystemPrompt(cfg, config.AgentKindIDE, "test", "", []SystemPromptFragment{{
		ID: "required", Source: "test", Purpose: "test required admission", Content: "123456789",
		Required: true, Overflow: SystemPromptOverflowReject,
	}})
	if err == nil || !strings.Contains(err.Error(), "required") || !strings.Contains(err.Error(), "per-source limit") {
		t.Fatalf("expected required fragment overflow, got %v", err)
	}
}

func TestSystemPromptComposerTruncatesOptionalUTF8WithVisibleMarkerAndHashes(t *testing.T) {
	maxFragment := len(systemPromptTruncationMarker) + 11
	cfg := systemPromptBudgetConfig(maxFragment, 4096, 16, 256)
	original := strings.Repeat("风", 100)
	composition, err := composeSystemPrompt(cfg, config.AgentKindIDE, "test", "", []SystemPromptFragment{{
		ID: "optional", Source: "test", Purpose: "test optional admission", Content: original,
		Overflow: SystemPromptOverflowTruncate,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(composition.Instruction()) || !strings.HasSuffix(composition.Instruction(), systemPromptTruncationMarker) {
		t.Fatalf("truncated instruction must be valid UTF-8 with marker: %q", composition.Instruction())
	}
	entry := composition.Manifest()[0]
	if !entry.Included || !entry.Truncated || entry.Rejected || entry.OriginalBytes != len(original) || entry.IncludedBytes != len(composition.Instruction()) {
		t.Fatalf("unexpected manifest: %#v", entry)
	}
	if entry.OriginalSHA != systemPromptSHA(original) || entry.IncludedSHA != systemPromptSHA(composition.Instruction()) {
		t.Fatalf("manifest hashes must cover original and exact included content: %#v", entry)
	}
}

func TestSystemPromptComposerCountsWrappersInTotalBudget(t *testing.T) {
	cfg := systemPromptBudgetConfig(64, 4, 16, 256)
	_, err := composeSystemPrompt(cfg, config.AgentKindIDE, "test", "", []SystemPromptFragment{{
		ID: "required", Source: "test", Purpose: "test rendered budget", Content: "x", Prefix: "1234",
		Required: true, Overflow: SystemPromptOverflowReject,
	}})
	if err == nil || !strings.Contains(err.Error(), "total budget") {
		t.Fatalf("wrapper bytes must be charged to total budget, got %v", err)
	}
}

func TestSystemPromptComposerRejectsDuplicateIDs(t *testing.T) {
	cfg := systemPromptBudgetConfig(64, 1024, 16, 256)
	_, err := composeSystemPrompt(cfg, config.AgentKindIDE, "test", "", []SystemPromptFragment{
		{ID: "same", Source: "a", Purpose: "a", Content: "a", Required: true},
		{ID: "same", Source: "b", Purpose: "b", Content: "b", Required: true},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestComposeInstructionBudgetsDynamicTellerMetadataAndStyleProtocolOnce(t *testing.T) {
	description := strings.Repeat("导演描述", 80*1024)
	composition, err := ComposeInstruction(&config.Config{}, nil, IDEStoryTeller{
		ID: "director", Name: "Director", Description: description, Prompt: "导演正文规则",
		StyleRules: []StyleRule{
			{Global: true, StyleContents: []string{"全局风格"}},
			{Scene: "战斗", StyleContents: []string{"战斗风格"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(composition.Instruction(), "The current narrative style provides the following prose-style reference index"); count != 1 {
		t.Fatalf("style protocol header count=%d, want 1", count)
	}
	if count := strings.Count(composition.Instruction(), "Trigger rule: use prose-style references only"); count != 1 {
		t.Fatalf("style protocol footer count=%d, want 1", count)
	}
	var tellerEntry *SystemPromptManifestEntry
	for _, entry := range composition.Manifest() {
		if entry.ID == "ide_teller" {
			copy := entry
			tellerEntry = &copy
			break
		}
	}
	if tellerEntry == nil || !tellerEntry.Truncated || !tellerEntry.Included {
		t.Fatalf("dynamic teller metadata must be visibly budgeted: %#v", tellerEntry)
	}
	original := normalizeSystemPromptFragment(tellerSystemPromptFragment("ide_teller", "Default Writing Director Rules", "director", "Director", description, "导演正文规则")).Content
	if tellerEntry.OriginalBytes != len(original) || tellerEntry.OriginalSHA != systemPromptSHA(original) {
		t.Fatalf("teller manifest must hash metadata and prompt together: %#v", tellerEntry)
	}
}

func systemPromptBudgetConfig(maxFragment, maxTotal, maxFragments, maxMetadata int) *config.Config {
	return &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		MaxFragmentBytes: &maxFragment, MaxTotalInjectedBytes: &maxTotal,
		MaxFragments: &maxFragments, MaxMetadataFieldBytes: &maxMetadata,
	}, VersionSummary: config.AgentContextOverride{
		MaxFragmentBytes: &maxFragment, MaxTotalInjectedBytes: &maxTotal,
		MaxFragments: &maxFragments, MaxMetadataFieldBytes: &maxMetadata,
	}}}
}
