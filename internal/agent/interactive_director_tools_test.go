package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/internal/interactive"
)

func TestInteractiveDirectorPlanToolSubmitsMarkdownPatchPayload(t *testing.T) {
	var received interactive.DirectorPlanUpdateSubmission
	tools, err := newInteractiveDirectorPlanTools(InteractiveStoryToolContext{
		SubmitDirectorPlanUpdate: func(_ context.Context, submission interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error) {
			received = submission
			return interactive.DirectorPlanUpdateReceipt{
				Accepted:          []interactive.DirectorPlanDocumentAcceptance{{Document: interactive.DirectorDocumentAgentBrief, Hash: "next-hash"}},
				AcceptedDocuments: []string{interactive.DirectorDocumentAgentBrief},
				ChangedDocuments:  []string{interactive.DirectorDocumentAgentBrief},
				Finalized:         true,
				Decision:          submission.Decision,
			}, nil
		},
	})
	if err != nil || len(tools) != 1 {
		t.Fatalf("build director plan tool: tools=%d err=%v", len(tools), err)
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != submitDirectorPlanUpdateToolName {
		t.Fatalf("tool name = %s", info.Name)
	}
	invokable, ok := tools[0].(tool.InvokableTool)
	if !ok {
		t.Fatal("director plan tool must be invokable")
	}
	output, err := invokable.InvokableRun(context.Background(), `{"decision":{"mode":"patch","reason":"场景变化"},"updates":[{"document":"agent-brief.md","base_hash":"brief-hash","edits":[{"op":"replace_section","heading":"状态连续性","content":"抵达门前。"}]}],"finalize":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if received.Decision.Mode != interactive.PlanDecisionPatch || !received.Finalize || len(received.Updates) != 1 || received.Updates[0].Document != interactive.DirectorDocumentAgentBrief || received.Updates[0].BaseHash != "brief-hash" {
		t.Fatalf("unexpected submission: %#v", received)
	}
	if !strings.Contains(output, `"finalized":true`) || !strings.Contains(output, `"changed_documents":["agent-brief.md"]`) {
		t.Fatalf("unexpected receipt: %s", output)
	}
}
