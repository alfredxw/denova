package context

import (
	stdcontext "context"
	"reflect"
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent"
)

func TestAssemblerKeepsTurnContextAfterToolResults(t *testing.T) {
	request := agent.UserMessage("continue")
	state := agent.UserMessage("updated workspace state")
	state.Extra = map[string]any{"agent.context_state": "v1"}
	completion := agent.UserMessage("research complete")
	completion.TaskCompletion = &agent.TaskCompletionMessageMeta{CompletionID: "completion-1", Author: "researcher", Recipient: "writer"}
	assistant := agent.AssistantMessage("checking", []agent.ToolCall{{
		ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: "inspect", Arguments: `{}`},
	}})
	tool := agent.ToolMessage(agent.TextToolResult("evidence"), "call-1", agent.WithToolName("inspect"))
	fragments := []Fragment{
		{Source: "workspace.instructions", Purpose: "provide stable instructions", Content: "instructions", Placement: PlacementLeadingMessage, Included: true},
		{Source: "workspace.selection", Purpose: "preserve the current request", Content: "selected chapter", Placement: PlacementFinalUserPrefix, Included: true},
	}
	assembler := NewAssembler(Budget{})
	initial, err := assembler.Assemble(t.Context(), AssembleRequest{Messages: []*agent.Message{request}, Fragments: fragments})
	if err != nil {
		t.Fatal(err)
	}
	for _, trailing := range [][]*agent.Message{{assistant, tool}, {state, assistant, tool}, {assistant, tool, state, completion}} {
		messages := append([]*agent.Message{request}, trailing...)
		resumed, err := assembler.Assemble(t.Context(), AssembleRequest{Messages: messages, Fragments: fragments})
		if err != nil {
			t.Fatal(err)
		}
		want := append(append([]*agent.Message(nil), initial.Messages...), trailing...)
		if !reflect.DeepEqual(resumed.Messages, want) || !reflect.DeepEqual(resumed.Fragments, initial.Fragments) || resumed.InjectedBytes != initial.InjectedBytes {
			t.Fatalf("turn context changed after tool results: messages=%#v fragments=%#v bytes=%d", resumed.Messages, resumed.Fragments, resumed.InjectedBytes)
		}
		if request.Content != "continue" || tool.Content != "evidence" || state.Content != "updated workspace state" {
			t.Fatal("assembly mutated canonical messages")
		}
	}
	withoutInput, err := assembler.Assemble(t.Context(), AssembleRequest{Messages: []*agent.Message{state, assistant, tool, completion}, Fragments: fragments})
	if err != nil {
		t.Fatal(err)
	}
	if withoutInput.Fragments[1].Included || !reflect.DeepEqual(withoutInput.Messages[1:], []*agent.Message{state, assistant, tool, completion}) {
		t.Fatal("turn context was attached to a state update without a user request")
	}
}

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

func TestAssemblerRejectsTotalBudgetOverflowInsteadOfSilentlyTruncating(t *testing.T) {
	_, err := NewAssembler(Budget{
		MaxFragmentBytes: 64,
		MaxTotalBytes:    16,
	}).Assemble(stdcontext.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("continue")},
		Fragments: []Fragment{{
			Source: "workspace.state", Title: "State", Purpose: "resume work",
			Content: "complete bounded state", Placement: PlacementFinalUserPrefix, Included: true,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "context injected bytes") {
		t.Fatalf("total budget error = %v", err)
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
