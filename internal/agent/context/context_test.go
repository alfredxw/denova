package context

import (
	stdcontext "context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestBuildPlacesLeadingAndFinalUserSources(t *testing.T) {
	result, err := Build(stdcontext.Background(), Request{
		Messages: []*schema.Message{
			schema.UserMessage("旧请求"),
			schema.AssistantMessage("旧回复", nil),
			schema.UserMessage("继续写"),
		},
		Sources: []Source{
			{Source: "稳定上下文", Title: "稳定作品上下文", Content: "大纲", Placement: PlacementLeadingMessage, Included: true, Note: "prepended_to_model_messages"},
			{Source: "本轮动态上下文", Title: "动态作品状态", Content: "进度", Placement: PlacementFinalUserPrefix, Included: true, Note: "prepended_to_final_user_message"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(result.Messages))
	}
	if !strings.Contains(result.Messages[0].Content, "# 稳定作品上下文") || !strings.Contains(result.Messages[0].Content, "大纲") {
		t.Fatalf("stable source should be first message: %q", result.Messages[0].Content)
	}
	last := result.Messages[len(result.Messages)-1].Content
	if !strings.Contains(last, "# 动态作品状态") || !strings.Contains(last, "# 本轮用户请求") || !strings.Contains(last, "继续写") {
		t.Fatalf("dynamic source should prefix final user message: %q", last)
	}
	if len(result.Ledger) != 2 || result.Ledger[0].Note != "prepended_to_model_messages" || result.Ledger[1].Note != "prepended_to_final_user_message" {
		t.Fatalf("ledger should preserve source notes: %#v", result.Ledger)
	}
}

func TestBuildPreservesSourceOrderWithinPlacements(t *testing.T) {
	result, err := Build(stdcontext.Background(), Request{
		Messages: []*schema.Message{schema.UserMessage("继续写")},
		Sources: []Source{
			{Source: "stable-a", Title: "稳定 A", Content: "A", Placement: PlacementLeadingMessage, Included: true},
			{Source: "stable-b", Title: "稳定 B", Content: "B", Placement: PlacementLeadingMessage, Included: true},
			{Source: "dynamic-a", Title: "动态 A", Content: "DA", Placement: PlacementFinalUserPrefix, Included: true},
			{Source: "dynamic-b", Title: "动态 B", Content: "DB", Placement: PlacementFinalUserPrefix, Included: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(result.Messages))
	}
	if !strings.Contains(result.Messages[0].Content, "# 稳定 A") || !strings.Contains(result.Messages[1].Content, "# 稳定 B") {
		t.Fatalf("leading sources should preserve input order: %#v", result.Messages)
	}
	last := result.Messages[2].Content
	for _, want := range []string{"# 动态 A", "DA", "# 动态 B", "DB", "# 本轮用户请求", "继续写"} {
		if !strings.Contains(last, want) {
			t.Fatalf("final message missing %q: %s", want, last)
		}
	}
	if strings.Index(last, "# 动态 A") > strings.Index(last, "# 动态 B") {
		t.Fatalf("dynamic sources should preserve input order: %s", last)
	}
}

func TestSourceSummaryBoundsPreview(t *testing.T) {
	summary := SourceSummary([]Source{{
		Source:    "文件引用",
		Title:     "@chapter.md",
		Content:   "abcdefghijklmnopqrstuvwxyz",
		Placement: PlacementAuditOnly,
		Included:  true,
		Truncated: true,
		Limit:     10,
	}}, 6)
	for _, want := range []string{`source="文件引用"`, `title="@chapter.md"`, `preview="abcdef..."`, "truncated=true", "limit=10"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q: %s", want, summary)
		}
	}
}

func TestBuildReadsModeAdapterSources(t *testing.T) {
	result, err := Build(stdcontext.Background(), Request{
		Messages: []*schema.Message{schema.UserMessage("继续")},
		Adapter: ModeAdapterFunc(func(stdcontext.Context) ([]Source, error) {
			return []Source{{
				Source:    "adapter",
				Title:     "adapter context",
				Content:   "adapter payload",
				Placement: PlacementFinalUserPrefix,
				Included:  true,
			}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 1 || !strings.Contains(result.Messages[0].Content, "adapter payload") {
		t.Fatalf("adapter source should be included in final message: %#v", result.Messages)
	}
}
