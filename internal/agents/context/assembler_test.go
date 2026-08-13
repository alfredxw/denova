package context

import (
	stdcontext "context"
	"strings"
	"testing"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

func TestAssemblerRejectsUnknownPlacement(t *testing.T) {
	_, err := NewAssembler(Budget{}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			Source:    "workspace.invalid",
			Purpose:   "test invalid placement",
			Content:   "payload",
			Placement: Placement("future_placement"),
			Included:  true,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "placement") {
		t.Fatalf("error = %v, want invalid placement", err)
	}
}

func TestAssemblerBoundsInvalidPlacementDiagnosticAsMetadata(t *testing.T) {
	const tail = "UNBOUNDED_PLACEMENT_TAIL"
	_, err := NewAssembler(Budget{MaxMetadataFieldBytes: 7}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			Content:   "payload",
			Placement: Placement(strings.Repeat("界", 100) + tail),
		}},
	})
	if err == nil {
		t.Fatal("expected invalid placement error")
	}
	if !utf8.ValidString(err.Error()) || strings.Contains(err.Error(), tail) || len(err.Error()) > 128 {
		t.Fatalf("invalid placement diagnostic is not bounded metadata: %q", err)
	}
}

func TestAssemblerRejectsTotalBudgetThatCannotFitOneRune(t *testing.T) {
	_, err := NewAssembler(Budget{MaxFragmentBytes: 8, MaxTotalBytes: 1}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			Source:    "workspace.unicode",
			Purpose:   "verify UTF-8 budget safety",
			Content:   "界",
			Placement: PlacementFinalUserPrefix,
			Included:  true,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "context injected bytes") {
		t.Fatalf("total budget error = %v", err)
	}
}

func TestAssemblerTotalBudgetIncludesRenderedWrapperAndTitle(t *testing.T) {
	const userMessage = "继续写"
	const want = "# 动态状态\n\nState snapshots may be stale; tool reads are authoritative.\n\n界\n\n---\n\n# Current User Request (Highest Priority)\n\n继续写"
	maxInjectedBytes := len(want) - len(userMessage)
	result, err := NewAssembler(Budget{
		MaxFragmentBytes: 64,
		MaxTotalBytes:    maxInjectedBytes,
	}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage(userMessage)},
		Fragments: []Fragment{{
			Source:    "workspace.progress",
			Title:     "动态状态",
			Purpose:   "定位续写位置",
			Content:   "界",
			Placement: PlacementFinalUserPrefix,
			Included:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Messages[0].Content; got != want {
		t.Fatalf("model-visible message = %q, want %q", got, want)
	}
	if got := result.InjectedBytes; got != maxInjectedBytes {
		t.Fatalf("injected bytes = %d, want rendered overhead %d", got, maxInjectedBytes)
	}
	if got := result.Fragments[0]; got.Content != "界" || got.Truncated {
		t.Fatalf("fragment = %#v, want complete content after wrapper accounting", got)
	}
}

func TestAssemblerTotalBudgetIncludesLeadingMessageWrapper(t *testing.T) {
	const wantLeading = "# 稳定标题\n\nThe following content is a bounded, low-churn snapshot from the current workspace, placed near the beginning of model input for stable prefix caching. Use tools with the source path when more complete or current content is required.\n\n界"
	result, err := NewAssembler(Budget{
		MaxFragmentBytes: 64,
		MaxTotalBytes:    len(wantLeading),
	}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			Source:    "workspace.stable",
			Title:     "稳定标题",
			Purpose:   "提供稳定创作背景",
			Content:   "界",
			Placement: PlacementLeadingMessage,
			Included:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 2 || result.Messages[0].Content != wantLeading || result.Messages[1].Content != "继续写" {
		t.Fatalf("leading model messages = %#v, want exact bounded wrapper", result.Messages)
	}
	if result.InjectedBytes != len(wantLeading) || result.Fragments[0].Content != "界" || result.Fragments[0].Truncated {
		t.Fatalf("leading injection accounting is incorrect: %#v", result)
	}
}

func TestDefaultBudgetKeepsCreativeContextAbove128KiB(t *testing.T) {
	budget := DefaultBudget()
	const threshold = 128 * 1024
	if budget.MaxFragmentBytes <= threshold || budget.MaxTotalBytes <= threshold {
		t.Fatalf("default budget = %#v, want both hard limits above 128 KiB", budget)
	}
	if budget.MaxFragments <= 0 || budget.MaxMetadataFieldBytes <= 0 {
		t.Fatalf("default budget = %#v, want explicit fragment and metadata bounds", budget)
	}
}

func TestAssemblerRejectsFragmentCountAboveExplicitLimit(t *testing.T) {
	_, err := NewAssembler(Budget{
		MaxFragments:     2,
		MaxFragmentBytes: 64,
		MaxTotalBytes:    256,
	}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{
			{Content: "one", Placement: PlacementAuditOnly},
			{Content: "two", Placement: PlacementAuditOnly},
			{Content: "three", Placement: PlacementAuditOnly},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "fragment count 3 exceeds limit 2") {
		t.Fatalf("error = %v, want explicit fragment-count limit", err)
	}
}

func TestAssemblerBoundsMetadataFieldsAndLedgerPreviewAtUTF8Boundaries(t *testing.T) {
	const metadataLimit = 7
	result, err := NewAssembler(Budget{
		MaxFragments:          4,
		MaxMetadataFieldBytes: metadataLimit,
		MaxFragmentBytes:      12,
		MaxTotalBytes:         1024,
	}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			ID:        "标识标识",
			Source:    "来源来源",
			Title:     "标题标题",
			Purpose:   "用途用途",
			Content:   strings.Repeat("正文", 20),
			Placement: PlacementFinalUserPrefix,
			Included:  true,
			Note:      "备注备注",
		}},
		PreviewChars: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	fragment := result.Fragments[0]
	for name, value := range map[string]string{
		"id": fragment.ID, "source": fragment.Source, "title": fragment.Title,
		"purpose": fragment.Purpose, "note": fragment.Note,
		"ledger preview": result.Ledger[0].Preview,
	} {
		if len(value) > metadataLimit || !utf8.ValidString(value) {
			t.Errorf("%s = %q (%d bytes), want valid UTF-8 bounded to %d bytes", name, value, len(value), metadataLimit)
		}
	}
	if fragment.ID != "标识" || fragment.Source != "来源" || fragment.Title != "标题" || fragment.Purpose != "用途" {
		t.Fatalf("metadata fields were not deterministically bounded: %#v", fragment)
	}
	if !strings.HasPrefix(result.Messages[0].Content, "# 标题\n\n") || strings.Contains(result.Messages[0].Content, "标题标题") {
		t.Fatalf("model-visible title was not bounded: %q", result.Messages[0].Content)
	}
}

func TestAuditOnlyFragmentDoesNotEnterModelMessages(t *testing.T) {
	result, err := NewAssembler(Budget{}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			Source:    "display.thinking",
			Purpose:   "retain bounded diagnostics without model injection",
			Content:   "raw thinking and tool output",
			Placement: PlacementAuditOnly,
			Included:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 1 || result.Messages[0].Content != "继续写" || result.InjectedBytes != 0 {
		t.Fatalf("audit-only content entered model messages: %#v", result)
	}
	if len(result.Fragments) != 1 || result.Fragments[0].Hash == "" || result.Fragments[0].Limit != DefaultMaxFragmentBytes {
		t.Fatalf("audit-only fragment was not bounded and audited: %#v", result.Fragments)
	}
}

func TestAssemblerDoesNotInjectExplicitlyExcludedFragment(t *testing.T) {
	result, err := NewAssembler(Budget{}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			Source:    "workspace.optional",
			Purpose:   "optional context omitted by its projector",
			Content:   "must stay out",
			Placement: PlacementFinalUserPrefix,
			Included:  false,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fragments[0].Included || result.InjectedBytes != 0 || result.Messages[0].Content != "继续写" {
		t.Fatalf("explicitly excluded fragment entered the model: %#v", result)
	}
}

func TestAssemblerDoesNotPrefixNonUserFinalMessage(t *testing.T) {
	result, err := NewAssembler(Budget{}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.AssistantMessage("已有回复", nil)},
		Fragments: []Fragment{{
			Source:    "workspace.progress",
			Purpose:   "turn-scoped context requires a user request",
			Content:   "current progress",
			Placement: PlacementFinalUserPrefix,
			Included:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fragments[0].Included || result.Messages[0].Content != "已有回复" || result.InjectedBytes != 0 {
		t.Fatalf("final-user context was applied to an assistant message: %#v", result)
	}
}

func TestAssemblerEnforcesFragmentBudgetsAndAccountsCompleteInjection(t *testing.T) {
	const userMessage = "继续写"
	const wantMessage = "# 作品大纲\n\nState snapshots may be stale; tool reads are authoritative.\n\nabcde\n\n> " + truncationNotice + "\n\n---\n\n# 作品进度\n\nState snapshots may be stale; tool reads are authoritative.\n\nwxyzw\n\n> " + truncationNotice + "\n\n---\n\n# Current User Request (Highest Priority)\n\n继续写"
	assembler := NewAssembler(Budget{
		MaxFragmentBytes: 5,
		MaxTotalBytes:    4096,
	})
	result, err := assembler.Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage(userMessage)},
		Fragments: []Fragment{
			{
				Source:    "workspace.outline",
				Title:     "作品大纲",
				Purpose:   "约束本轮续写",
				Content:   "abcdef",
				Placement: PlacementFinalUserPrefix,
				Included:  true,
			},
			{
				Source:    "workspace.progress",
				Title:     "作品进度",
				Purpose:   "定位续写位置",
				Content:   strings.Repeat("wxyz", 50),
				Placement: PlacementFinalUserPrefix,
				Included:  true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fragments) != 2 {
		t.Fatalf("fragments = %d, want 2", len(result.Fragments))
	}
	if got := result.Fragments[0]; got.Content != "abcde" || !got.Truncated || got.Limit != 5 {
		t.Fatalf("first fragment = %#v, want single-fragment truncation", got)
	}
	if got := result.Fragments[1]; got.Content != "wxyzw" || !got.Truncated || got.Limit != 5 {
		t.Fatalf("second fragment = %#v, want single-fragment truncation", got)
	}
	for _, fragment := range result.Fragments {
		if fragment.Source == "" || fragment.Purpose == "" || fragment.Placement == "" || fragment.Limit == 0 || fragment.Hash == "" {
			t.Fatalf("fragment metadata is incomplete: %#v", fragment)
		}
	}
	if got := result.Messages[len(result.Messages)-1].Content; got != wantMessage {
		t.Fatalf("model-visible message = %q, want %q", got, wantMessage)
	}
	if want := len(wantMessage) - len(userMessage); result.InjectedBytes != want {
		t.Fatalf("injected bytes = %d, want rendered overhead %d", result.InjectedBytes, want)
	}
	if len(result.Ledger) != 2 || result.Ledger[0].Hash == "" || result.Ledger[1].Hash == "" {
		t.Fatalf("ledger must retain fragment hashes: %#v", result.Ledger)
	}
}
