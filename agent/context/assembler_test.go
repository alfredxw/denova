package context

import (
	stdcontext "context"
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent"
)

func TestAssemblerAccountsForDefaultRendererAndTruncatesContent(t *testing.T) {
	const request = "continue"
	const want = "# State\n\nabc\n\n> " + DefaultTruncationNotice + "\n\n---\n\n# User request\n\ncontinue"
	result, err := NewAssembler(Budget{
		MaxFragmentBytes: 3,
		MaxTotalBytes:    len(want) - len(request),
	}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage(request)},
		Fragments: []Fragment{{
			Source: "workspace.state", Title: "State", Purpose: "resume work",
			Content: "abcd", Placement: PlacementFinalUserPrefix, Included: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages[0].Content != want || result.InjectedBytes != len(want)-len(request) {
		t.Fatalf("assembled message = %q, bytes=%d", result.Messages[0].Content, result.InjectedBytes)
	}
	if result.Fragments[0].Content != "abc" || !result.Fragments[0].Truncated || result.Fragments[0].Hash == "" {
		t.Fatalf("bounded fragment = %#v", result.Fragments[0])
	}
}

func TestAssemblerKeepsAuditOnlyContentOutOfModelMessages(t *testing.T) {
	input := agent.UserMessage("write")
	input.Extra = map[string]any{"nested": []any{"original"}}
	result, err := NewAssembler(Budget{}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{input},
		Fragments: []Fragment{{
			Source: "display.tool", Purpose: "bounded audit", Content: "raw output",
			Placement: PlacementAuditOnly, Included: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result.Messages[0].Extra["nested"].([]any)[0] = "changed"
	if input.Extra["nested"].([]any)[0] != "original" {
		t.Fatal("assembler returned a mutable alias to the input transcript")
	}
	if result.Messages[0].Content != "write" || result.InjectedBytes != 0 || result.Fragments[0].Hash == "" {
		t.Fatalf("audit fragment leaked into model input: %#v", result)
	}
}

type testProjector struct{}

func (testProjector) Descriptor() ContextDescriptor {
	return ContextDescriptor{
		ID: "state", Source: "workspace.state", Purpose: "resume", Placement: PlacementLeadingMessage, Limit: 4,
	}
}

func (testProjector) Project(stdcontext.Context) ([]Fragment, error) {
	return []Fragment{{Title: "State", Content: "abcdef", Included: true}}, nil
}

func TestProjectorCannotEscapeDescriptorOrHardLimit(t *testing.T) {
	result, err := NewAssembler(Budget{MaxFragmentBytes: 64, MaxTotalBytes: 256}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages:   []*agent.Message{agent.UserMessage("continue")},
		Projectors: []ContextProjector{testProjector{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fragment := result.Fragments[0]
	if fragment.ID != "state" || fragment.Source != "workspace.state" || fragment.Content != "abcd" || !fragment.Truncated {
		t.Fatalf("projected fragment = %#v", fragment)
	}
}

func TestAssemblerRejectsUnboundedFragmentCountAndUnknownPlacement(t *testing.T) {
	fragments := []Fragment{{Content: "one"}, {Content: "two"}}
	_, err := NewAssembler(Budget{MaxFragments: 1}).Assemble(stdcontext.Background(), AssembleRequest{Fragments: fragments})
	if err == nil || !strings.Contains(err.Error(), "fragment count") {
		t.Fatalf("fragment count error = %v", err)
	}

	_, err = NewAssembler(Budget{}).Assemble(stdcontext.Background(), AssembleRequest{Fragments: []Fragment{{
		Content: "invalid", Placement: Placement("future"), Included: true,
	}}})
	if err == nil || !strings.Contains(err.Error(), "placement") {
		t.Fatalf("placement error = %v", err)
	}
}
