package agents

import (
	"strings"
	"testing"

	agentcontext "denova/internal/agents/context"
	"denova/internal/book"
)

const minimumCompleteAgentContextBytes = 128 * 1024

func TestExplicitFileReferenceKeepsContentBelow128KBComplete(t *testing.T) {
	workspace := t.TempDir()
	content := strings.Repeat("x", 96*1024)
	mustWriteTestFile(t, workspace, "references/large.md", content)

	_, assembled := assembleTurnForTest(t, ChatRequest{Message: "请完整参考", References: []string{"references/large.md"}}, nil, book.NewService(workspace), agentcontext.DefaultBudget())
	got := finalAssembledUserMessage(t, assembled)
	if !strings.Contains(got, content) {
		t.Fatalf("explicit reference below 128KB should be included in full, got %d bytes", len(got))
	}
	if strings.Contains(got, "[内容已截断]") {
		t.Fatal("explicit reference below 128KB must not be truncated")
	}
}

func TestImmediateAgentResultLimitsAreAbove128KB(t *testing.T) {
	limits := map[string]int{
		"explicit file reference":          maxReferenceFileBytes,
		"interactive director tool result": interactiveDirectorToolResultMaxBytes,
	}
	for name, limit := range limits {
		if limit <= minimumCompleteAgentContextBytes {
			t.Errorf("%s limit = %d bytes, want above %d", name, limit, minimumCompleteAgentContextBytes)
		}
	}
}
