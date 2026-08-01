package chat

import (
	"strings"
	"testing"

	agenttoolruntime "denova/internal/agents/toolruntime"
)

func TestToolApprovalDetailsExposeBoundedStructuredArguments(t *testing.T) {
	t.Parallel()
	request := agenttoolruntime.ApprovalRequest{Arguments: `{"action":"run","command":"click","selector":"button.save"}`}
	if got := toolApprovalDetails(request); got != request.Arguments {
		t.Fatalf("details = %q, want %q", got, request.Arguments)
	}
	request.Decision.Command = "git push"
	if got := toolApprovalDetails(request); got != "" {
		t.Fatalf("shell command details = %q, want empty", got)
	}
	request.Decision.Command = ""
	request.Arguments = strings.Repeat("界", toolApprovalDetailsMax)
	got := toolApprovalDetails(request)
	if len(got) > toolApprovalDetailsMax || !strings.Contains(got, "详情已截断") {
		t.Fatalf("bounded details bytes=%d marker=%t", len(got), strings.Contains(got, "详情已截断"))
	}
}
