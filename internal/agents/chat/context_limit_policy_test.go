package chat

import (
	"strings"
	"testing"

	agentcontext "denova/internal/agents/context"
	"denova/internal/book"
	"denova/internal/interactive"
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
	if strings.Contains(got, "内容已截断") {
		t.Fatal("explicit reference below 128KB must not be truncated")
	}
}

func TestExplicitFileReferenceTruncationIsUTF8SafeVisibleAndFenced(t *testing.T) {
	workspace := t.TempDir()
	content := strings.Repeat("边界内容", 256)
	mustWriteTestFile(t, workspace, "references/oversized.md", content)
	budget := agentcontext.DefaultBudget()
	budget.MaxFragmentBytes = 257
	budget.MaxTotalBytes = 8 * 1024

	_, assembled := assembleTurnForTest(t, ChatRequest{Message: "请参考", References: []string{"references/oversized.md"}}, nil, book.NewService(workspace), budget)
	got := finalAssembledUserMessage(t, assembled)
	for _, marker := range []string{"内容已截断；", "Content truncated;"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("model-visible reference missing truncation marker %q:\n%s", marker, got)
		}
	}
	if strings.Count(got, "```") != 2 {
		t.Fatalf("truncated reference must retain one closed Markdown fence:\n%s", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("truncated reference contains an invalid UTF-8 replacement rune:\n%s", got)
	}
	found := false
	for _, fragment := range assembled.Context.Fragments {
		if fragment.Source != "workspace.file.reference" {
			continue
		}
		found = true
		if !fragment.Included || !fragment.Truncated || len(fragment.Content) > budget.MaxFragmentBytes {
			t.Fatalf("reference fragment = %#v", fragment)
		}
	}
	if !found {
		t.Fatalf("workspace reference fragment missing: %#v", assembled.Context.Fragments)
	}
}

func TestImmediateAgentResultLimitsAreAbove128KB(t *testing.T) {
	limits := map[string]int{
		"explicit file reference":          ReferenceFileByteLimit,
		"interactive director tool result": interactive.DirectorContextMaxBytes,
	}
	for name, limit := range limits {
		if limit <= minimumCompleteAgentContextBytes {
			t.Errorf("%s limit = %d bytes, want above %d", name, limit, minimumCompleteAgentContextBytes)
		}
	}
}
