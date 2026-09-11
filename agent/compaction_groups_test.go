package agent

import (
	"reflect"
	"strings"
	"testing"
)

func TestCompactionGroupsProtectNewestCompleteStepAndIncompleteSuffix(t *testing.T) {
	old := []*Message{UserMessage("old task"), AssistantMessage(strings.Repeat("old evidence ", 1000), nil)}
	latest := []*Message{UserMessage("current task"), AssistantMessage("", []ToolCall{
		{ID: "a", Function: FunctionCall{Name: "read"}}, {ID: "b", Function: FunctionCall{Name: "read"}},
	}), {Role: ToolRole, ToolCallID: "a", Content: "a result"}, {Role: ToolRole, ToolCallID: "b", Content: "b result"}}
	for _, suffix := range [][]*Message{nil, {UserMessage("unconsumed steering")}, {AssistantMessage("", []ToolCall{{ID: "pending", Function: FunctionCall{Name: "read"}}})}} {
		messages := append(cloneMessages(old), cloneMessages(latest)...)
		messages = append(messages, cloneMessages(suffix)...)
		groups, ends, _ := compactionGroups(messages, messages, compactionRecord{}, false)
		if !reflect.DeepEqual(ends, []int{2}) || len(groups) != 1 || !reflect.DeepEqual(groups[0].Messages, old) {
			t.Fatalf("groups=%#v ends=%v", groups, ends)
		}
		groups[0].Messages[0].Content = "changed"
		if messages[0].Content != "old task" {
			t.Fatal("extension mutated journal source")
		}
	}
	incomplete := append(cloneMessages(old), latest[:3]...)
	if groups, _, _ := compactionGroups(incomplete, incomplete, compactionRecord{}, false); len(groups) != 0 {
		t.Fatal("incomplete batch displaced newest complete step")
	}
}

func TestCompactionGroupsOfferOnlyNewDeltaAfterCheckpoint(t *testing.T) {
	messages := []*Message{UserMessage("old"), AssistantMessage("old answer", nil), UserMessage("new"), AssistantMessage("new answer", nil), UserMessage("latest"), AssistantMessage("latest answer", nil)}
	current := compactionRecord{ReplacementTo: 2}
	groups, ends, _ := compactionGroups(messages, messages, current, true)
	if !reflect.DeepEqual(ends, []int{4}) || len(groups) != 1 || !reflect.DeepEqual(groups[0].Messages, messages[2:4]) {
		t.Fatalf("groups=%#v ends=%v", groups, ends)
	}
}

func TestCompactionViewCannotAcquireCoverageThroughJSONOrCallerFields(t *testing.T) {
	state := compactionStatePointer(compactionRecord{ID: "checkpoint", Revision: 1, Summary: "truth", ReplacementTo: 2}, true)
	raw := []*Message{UserMessage("old"), AssistantMessage("old answer", nil), UserMessage("latest")}
	state.Summary = "caller edit"
	projected, err := state.Project(raw, 1024)
	if err != nil || len(projected) != 2 || !strings.Contains(projected[0].Content, "truth") || strings.Contains(projected[0].Content, "caller edit") {
		t.Fatalf("projection=%#v err=%v", projected, err)
	}
	if _, err := (CompactionState{ID: "forged", Revision: 1, Summary: "forged"}).Project(raw, 1024); err == nil {
		t.Fatal("caller invented projection authority")
	}
}
