package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/internal/interactive"
)

func TestInteractiveDirectorPlanToolSubmitsOneStructuredPayload(t *testing.T) {
	var received interactive.DirectorPlanUpdateSubmission
	tools, err := newInteractiveDirectorPlanTools(InteractiveStoryToolContext{
		SubmitDirectorPlanUpdate: func(_ context.Context, submission interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error) {
			received = submission
			return interactive.DirectorPlanUpdateReceipt{Accepted: true, Mode: submission.Decision.Mode, DocsUpdated: submission.Docs != nil, Decision: submission.Decision}, nil
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
	output, err := invokable.InvokableRun(context.Background(), `{"decision":{"mode":"patch","reason":"场景变化"},"docs":{"plan":"完整规划","lore_context":"完整资料工作集"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if received.Decision.Mode != interactive.PlanDecisionPatch || received.Docs == nil || received.Docs.Plan != "完整规划" || received.Docs.LoreContext != "完整资料工作集" {
		t.Fatalf("unexpected submission: %#v", received)
	}
	if !strings.Contains(output, `"accepted":true`) || !strings.Contains(output, `"docs_updated":true`) {
		t.Fatalf("unexpected receipt: %s", output)
	}
}
