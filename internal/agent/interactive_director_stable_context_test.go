package agent

import (
	"strings"
	"testing"
)

func TestSingleInstructionConversationPrependsBoundedStableContext(t *testing.T) {
	conversation := &singleInstructionConversation{
		instruction:           "dynamic instruction",
		stableContextTitle:    "完整常驻资料（source: resident lore; complete=true）",
		stableContext:         "## 规则\n\n生命上限为 100。",
		stableContextMaxBytes: 1024,
	}
	messages, err := conversation.PrepareMessages("", "dynamic instruction")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || !strings.Contains(messages[0].Content, "完整常驻资料") || !strings.Contains(messages[0].Content, "生命上限为 100") || messages[1].Content != "dynamic instruction" {
		t.Fatalf("stable context must be a separate leading message: %#v", messages)
	}
}

func TestSingleInstructionConversationRejectsOversizedStableContext(t *testing.T) {
	conversation := &singleInstructionConversation{
		instruction: "dynamic", stableContext: "12345", stableContextMaxBytes: 4,
	}
	if _, err := conversation.PrepareMessages("", "dynamic"); err == nil || !strings.Contains(err.Error(), "超过上限") {
		t.Fatalf("oversized stable context must fail instead of truncating: %v", err)
	}
}

func TestSingleInstructionConversationCapsFinalStableMessageAndTitle(t *testing.T) {
	conversation := &singleInstructionConversation{
		instruction: "dynamic", stableContextTitle: "1234567890", stableContext: "12345", stableContextMaxBytes: 18,
	}
	if _, err := conversation.PrepareMessages("", "dynamic"); err == nil || !strings.Contains(err.Error(), "最终消息超过上限") {
		t.Fatalf("the title wrapper must count toward the stable message hard cap: %v", err)
	}
	conversation.stableContextTitle = strings.Repeat("a", maxSingleInstructionStableContextTitleBytes+1)
	conversation.stableContextMaxBytes = 2048
	if _, err := conversation.PrepareMessages("", "dynamic"); err == nil || !strings.Contains(err.Error(), "标题超过上限") {
		t.Fatalf("an unbounded title must be rejected: %v", err)
	}
}
