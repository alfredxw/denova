package context

import (
	stdcontext "context"
	"strings"
	"testing"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

type fixedProjector struct {
	descriptor ContextDescriptor
	fragments  []Fragment
}

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

func TestAssemblerMarksFragmentExcludedWhenBudgetCannotFitOneRune(t *testing.T) {
	result, err := NewAssembler(Budget{MaxFragmentBytes: 8, MaxTotalBytes: 1}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			Source:    "workspace.unicode",
			Purpose:   "verify UTF-8 budget safety",
			Content:   "界",
			Placement: PlacementFinalUserPrefix,
			Included:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fragment := result.Fragments[0]
	if fragment.Included || !fragment.Truncated || fragment.Content != "" {
		t.Fatalf("fragment = %#v, want an explicitly excluded fragment", fragment)
	}
	if result.InjectedBytes != 0 || result.Messages[0].Content != "继续写" {
		t.Fatalf("unfittable context must not enter the model: %#v", result)
	}
}

func TestAssemblerTotalBudgetIncludesRenderedWrapperAndTitle(t *testing.T) {
	const userMessage = "继续写"
	const want = "# 动态状态\n\n状态快照可能过期，以工具读取为准。\n\n界\n\n> " + truncationNotice + "\n\n---\n\n# 本轮用户请求（最高优先级）\n\n继续写"
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
			Content:   strings.Repeat("界", 100),
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
	if got := result.Fragments[0]; got.Content != "界" || !got.Truncated {
		t.Fatalf("fragment = %#v, want UTF-8-safe truncation after wrapper accounting", got)
	}
}

func TestAssemblerTotalBudgetIncludesLeadingMessageWrapper(t *testing.T) {
	const wantLeading = "# 稳定标题\n\n以下内容来自当前 workspace 的低变更率有界状态快照，放在模型输入前部以提升前缀缓存稳定性。需要更完整或最新内容时，按来源路径使用工具读取确认。\n\n界\n\n> " + truncationNotice
	result, err := NewAssembler(Budget{
		MaxFragmentBytes: 64,
		MaxTotalBytes:    len(wantLeading),
	}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Fragments: []Fragment{{
			Source:    "workspace.stable",
			Title:     "稳定标题",
			Purpose:   "提供稳定创作背景",
			Content:   strings.Repeat("界", 100),
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
	if result.InjectedBytes != len(wantLeading) || result.Fragments[0].Content != "界" || !result.Fragments[0].Truncated {
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

func TestAssemblerBoundsMatchingProjectorMetadataBeforeValidation(t *testing.T) {
	result, err := NewAssembler(Budget{
		MaxFragments:          2,
		MaxMetadataFieldBytes: 7,
		MaxFragmentBytes:      64,
		MaxTotalBytes:         512,
	}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Projectors: []ContextProjector{fixedProjector{
			descriptor: ContextDescriptor{
				ID:        "标识标识",
				Source:    "来源来源",
				Purpose:   "用途用途",
				Placement: PlacementFinalUserPrefix,
			},
			fragments: []Fragment{{
				Source:   "来源来源",
				Purpose:  "用途用途",
				Title:    "标题标题",
				Content:  "正文",
				Included: true,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fragment := result.Fragments[0]
	if fragment.ID != "标识" || fragment.Source != "来源" || fragment.Purpose != "用途" || fragment.Title != "标题" {
		t.Fatalf("projector metadata was not bounded consistently: %#v", fragment)
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

func (p fixedProjector) Descriptor() ContextDescriptor {
	return p.descriptor
}

func (p fixedProjector) Project(stdcontext.Context) ([]Fragment, error) {
	return append([]Fragment(nil), p.fragments...), nil
}

func TestAssemblerEnforcesFragmentAndTotalBudgets(t *testing.T) {
	const userMessage = "继续写"
	const wantMessage = "# 作品大纲\n\n状态快照可能过期，以工具读取为准。\n\nabcde\n\n> " + truncationNotice + "\n\n---\n\n# 作品进度\n\n状态快照可能过期，以工具读取为准。\n\nwxy\n\n> " + truncationNotice + "\n\n---\n\n# 本轮用户请求（最高优先级）\n\n继续写"
	assembler := NewAssembler(Budget{
		MaxFragmentBytes: 5,
		MaxTotalBytes:    len(wantMessage) - len(userMessage),
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
	if got := result.Fragments[1]; got.Content != "wxy" || !got.Truncated || got.Limit != 5 {
		t.Fatalf("second fragment = %#v, want total-budget truncation", got)
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

func TestAssemblerAppliesProjectorDescriptorToEveryFragment(t *testing.T) {
	assembler := NewAssembler(Budget{MaxFragmentBytes: 64, MaxTotalBytes: 512})
	result, err := assembler.Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("继续写")},
		Projectors: []ContextProjector{fixedProjector{
			descriptor: ContextDescriptor{
				ID:        "writing.progress",
				Source:    "workspace.progress",
				Purpose:   "定位本轮续写起点",
				Placement: PlacementFinalUserPrefix,
				Limit:     4,
			},
			fragments: []Fragment{{Title: "当前进度", Content: "abcdef", Included: true}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fragments) != 1 {
		t.Fatalf("fragments = %d, want 1", len(result.Fragments))
	}
	fragment := result.Fragments[0]
	if fragment.ID != "writing.progress" || fragment.Source != "workspace.progress" || fragment.Purpose != "定位本轮续写起点" || fragment.Placement != PlacementFinalUserPrefix {
		t.Fatalf("projector descriptor was not applied: %#v", fragment)
	}
	if fragment.Content != "abcd" || fragment.Limit != 4 || !fragment.Truncated || fragment.Hash == "" {
		t.Fatalf("projected fragment was not bounded and audited: %#v", fragment)
	}
}
