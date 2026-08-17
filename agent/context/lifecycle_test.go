package context

import (
	"context"
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent"
)

func TestExportLifecycleFragmentsPreservesExactLeadingRenderingAndAuditsFinalUserContext(t *testing.T) {
	result, err := NewAssembler(Budget{}).Assemble(context.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("continue")},
		Fragments: []Fragment{
			{ID: "rules", Source: "project.rules", Title: "Rules", Purpose: "stable instructions", Content: "Do the work.", Placement: PlacementLeadingMessage, Limit: 128 << 10, Included: true},
			{ID: "selection", Source: "editor.selection", Title: "Selection", Purpose: "current focus", Content: "chapter 3", Placement: PlacementFinalUserPrefix, Limit: 128 << 10, Included: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fragments, err := ExportLifecycleFragments(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 2 {
		t.Fatalf("lifecycle fragments=%d, want 2", len(fragments))
	}
	if fragments[0].Placement != agent.ContextLeadingMessage || fragments[0].Rendering != agent.ContextRenderVerbatim ||
		fragments[0].Role != agent.User || fragments[0].Content != result.Messages[0].Content ||
		!strings.Contains(fragments[0].Content, "# Rules") {
		t.Fatalf("leading lifecycle fragment=%#v", fragments[0])
	}
	if fragments[1].Placement != agent.ContextAuditOnly || fragments[1].Content != "chapter 3" ||
		fragments[1].Resource != "selection" || fragments[1].Revision == "" {
		t.Fatalf("final-user audit fragment=%#v", fragments[1])
	}
	for _, fragment := range fragments {
		if fragment.HardLimit < DefaultLifecycleHardLimit {
			t.Fatalf("hard limit=%d, want at least %d", fragment.HardLimit, DefaultLifecycleHardLimit)
		}
	}
}

func TestExportLifecycleFragmentsMapsSessionStateToDurableStateMessage(t *testing.T) {
	result, err := NewAssembler(Budget{}).Assemble(context.Background(), AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage("continue")},
		Fragments: []Fragment{{
			ID: "workspace", StateID: "workspace", Source: "workspace.snapshot", Purpose: "current workspace state",
			Content: "revision one", Placement: PlacementLeadingMessage, Stability: agent.ContextSessionState,
			Limit: 128 << 10, Included: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fragments, err := ExportLifecycleFragments(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 1 || fragments[0].StateID != "workspace" ||
		fragments[0].Stability != agent.ContextSessionState || fragments[0].Placement != agent.ContextStateMessage ||
		fragments[0].Rendering != agent.ContextRenderVerbatim || fragments[0].Role != "" {
		t.Fatalf("session-state lifecycle fragment = %#v", fragments)
	}
}
