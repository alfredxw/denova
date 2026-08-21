package interactiveapp

import (
	"fmt"
	"strings"
	"testing"

	"denova/internal/agents"
)

func TestInteractiveMessageListSummaryBoundsLongHistory(t *testing.T) {
	messages := make([]*agents.Message, 20)
	for index := range messages {
		messages[index] = agents.UserMessage(fmt.Sprintf("message-%02d", index))
	}

	summary := interactiveMessageListSummary(messages)
	if !strings.Contains(summary, "count=20") || !strings.Contains(summary, "omitted=12") {
		t.Fatalf("long message summary did not report its bounded shape: %s", summary)
	}
	if strings.Contains(summary, "message-10") || !strings.Contains(summary, "message-00") || !strings.Contains(summary, "message-19") {
		t.Fatalf("long message summary did not retain only useful edges: %s", summary)
	}
}
