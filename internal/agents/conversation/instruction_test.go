package conversation

import (
	"context"
	agentcontext "denova/internal/agents/context"
	"strings"
	"testing"
)

func TestSingleInstructionConversationPrependsBoundedStableContext(t *testing.T) {
	conversation := &InstructionConversation{
		instruction:           "dynamic instruction",
		stableContextTitle:    "完整常驻资料（source: resident lore; complete=true）",
		stableContext:         "## 规则\n\n生命上限为 100。",
		stableContextMaxBytes: 1024,
	}
	result, err := conversation.AssembleModelContext(context.Background(), "", agentcontext.ModelContextInput{
		UserMessage: "dynamic instruction", Budget: conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	messages := result.Messages
	if len(messages) != 2 || !strings.Contains(messages[0].Content, "完整常驻资料") || !strings.Contains(messages[0].Content, "生命上限为 100") || messages[1].Content != "dynamic instruction" {
		t.Fatalf("stable context must be a separate leading message: %#v", messages)
	}
	if len(conversation.lastContext.Fragments) != 0 {
		t.Fatal("pure director assembly mutated the committed context audit")
	}
	if err := conversation.CommitModelInput(context.Background(), "", result); err != nil {
		t.Fatal(err)
	}
	if len(conversation.lastContext.Fragments) == 0 {
		t.Fatal("director context audit was not published at the explicit commit boundary")
	}
}

func TestSingleInstructionConversationRejectsOversizedStableContext(t *testing.T) {
	conversation := &InstructionConversation{
		instruction: "dynamic", stableContext: "12345", stableContextMaxBytes: 4,
	}
	if _, err := conversation.AssembleModelContext(context.Background(), "", agentcontext.ModelContextInput{UserMessage: "dynamic", Budget: conversation.ModelContextBudget()}); err == nil || !strings.Contains(err.Error(), "exceeds its limit") {
		t.Fatalf("oversized stable context must fail instead of truncating: %v", err)
	}
}

func TestSingleInstructionConversationCapsFinalStableMessageAndTitle(t *testing.T) {
	conversation := &InstructionConversation{
		instruction: "dynamic", stableContextTitle: "1234567890", stableContext: "12345", stableContextMaxBytes: 18,
	}
	if _, err := conversation.AssembleModelContext(context.Background(), "", agentcontext.ModelContextInput{UserMessage: "dynamic", Budget: conversation.ModelContextBudget()}); err == nil || !strings.Contains(err.Error(), "rendered stable model context exceeds its limit") {
		t.Fatalf("the title wrapper must count toward the stable message hard cap: %v", err)
	}
	conversation.stableContextTitle = strings.Repeat("a", maxInstructionStableContextTitleBytes+1)
	conversation.stableContextMaxBytes = 2048
	if _, err := conversation.AssembleModelContext(context.Background(), "", agentcontext.ModelContextInput{UserMessage: "dynamic", Budget: conversation.ModelContextBudget()}); err == nil || !strings.Contains(err.Error(), "title exceeds its limit") {
		t.Fatalf("an unbounded title must be rejected: %v", err)
	}
}
