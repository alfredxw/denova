package agent

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestPlanProtocolParserExtractsBlocksAcrossChunks(t *testing.T) {
	var events []Event
	parser := newPlanProtocolParser(agentEventMetadata{}, func(ev Event) {
		events = append(events, ev)
	})

	var visible strings.Builder
	visible.WriteString(parser.Push("先看一下<plan_ques"))
	visible.WriteString(parser.Push("tions>{\"questions\":["))
	visible.WriteString(parser.Push("]}</plan_questions>然后"))
	visible.WriteString(parser.Push("<proposed_plan># 计划"))
	visible.WriteString(parser.Push("</proposed_plan>"))
	visible.WriteString(parser.Flush())

	if got := visible.String(); got != "先看一下然后" {
		t.Fatalf("visible = %q, want %q", got, "先看一下然后")
	}
	if len(events) != 4 {
		t.Fatalf("events len = %d, want 4: %#v", len(events), events)
	}
	if events[0].Type != "plan_question" || eventDataString(events[0].Data, "status") != "running" || eventDataString(events[0].Data, "id") == "" {
		t.Fatalf("unexpected running question event: %#v", events[0])
	}
	if events[1].Type != "plan_question" || eventDataString(events[1].Data, "status") != "success" || eventDataString(events[1].Data, "id") != eventDataString(events[0].Data, "id") || eventDataString(events[1].Data, "content") != `{"questions":[]}` {
		t.Fatalf("unexpected success question event: %#v", events[1])
	}
	if events[2].Type != "proposed_plan" || eventDataString(events[2].Data, "status") != "running" || eventDataString(events[2].Data, "id") == "" {
		t.Fatalf("unexpected running plan event: %#v", events[2])
	}
	if events[3].Type != "proposed_plan" || eventDataString(events[3].Data, "status") != "success" || eventDataString(events[3].Data, "id") != eventDataString(events[2].Data, "id") || eventDataString(events[3].Data, "content") != "# 计划" {
		t.Fatalf("unexpected success plan event: %#v", events[3])
	}
}

func TestPlanProtocolParserFlushesUnclosedBlockAsVisibleText(t *testing.T) {
	var events []Event
	parser := newPlanProtocolParser(agentEventMetadata{}, func(ev Event) {
		events = append(events, ev)
	})

	got := parser.Push("a<proposed_plan># 未完成") + parser.Flush()
	if got != "a<proposed_plan># 未完成" {
		t.Fatalf("flush visible = %q", got)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2: %#v", len(events), events)
	}
	if events[0].Type != "proposed_plan" || eventDataString(events[0].Data, "status") != "running" {
		t.Fatalf("unexpected running event: %#v", events[0])
	}
	if events[1].Type != "proposed_plan" || eventDataString(events[1].Data, "status") != "error" || eventDataString(events[1].Data, "id") != eventDataString(events[0].Data, "id") {
		t.Fatalf("unexpected cleanup event: %#v", events[1])
	}
}

func TestPlanProtocolParserTruncatesDisplayedBlock(t *testing.T) {
	var events []Event
	parser := newPlanProtocolParser(agentEventMetadata{}, func(ev Event) {
		events = append(events, ev)
	})

	_ = parser.Push("<proposed_plan>" + strings.Repeat("长", planBlockDisplayMaxBytes) + "</proposed_plan>")
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	content := eventDataString(events[1].Data, "content")
	if len(content) > planBlockDisplayMaxBytes {
		t.Fatalf("content bytes = %d, want <= %d", len(content), planBlockDisplayMaxBytes)
	}
	if !strings.Contains(content, "Plan 展示已截断") {
		t.Fatalf("content should include truncation hint: %q", content[len(content)-80:])
	}
}

func TestPlanProtocolParserCarriesRunMetadata(t *testing.T) {
	var events []Event
	parser := newPlanProtocolParser(agentEventMetadata{
		AgentKind:     AgentKindIDE,
		RunID:         "run-plan-1",
		AgentName:     "NovaAgent",
		RootAgentName: "NovaAgent",
		RunPath:       []string{"NovaAgent"},
	}, func(ev Event) {
		events = append(events, ev)
	})

	_ = parser.Push(`<plan_questions>{"questions":[]}</plan_questions>`)
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	for _, ev := range events {
		if eventDataString(ev.Data, "run_id") != "run-plan-1" {
			t.Fatalf("run_id = %q, want %q in event %#v", eventDataString(ev.Data, "run_id"), "run-plan-1", ev)
		}
		if eventDataString(ev.Data, "agent_kind") != AgentKindIDE {
			t.Fatalf("agent_kind = %q, want %q in event %#v", eventDataString(ev.Data, "agent_kind"), AgentKindIDE, ev)
		}
		if eventDataString(ev.Data, "agent_name") != "NovaAgent" {
			t.Fatalf("agent_name = %q, want NovaAgent in event %#v", eventDataString(ev.Data, "agent_name"), ev)
		}
	}
}

func TestPlanModeSuccessfulBlockDiscardsAssistantPreamble(t *testing.T) {
	parser := newPlanProtocolParser(agentEventMetadata{}, func(ev Event) {})
	var content strings.Builder
	var thinking strings.Builder

	content.WriteString(parser.Push("需要确认一下"))
	content.WriteString(parser.Push(`<plan_questions>{"questions":[]}</plan_questions>`))
	content.WriteString("卡片后说明")
	thinking.WriteString("正在整理问题")

	if !parser.HasSuccessfulBlock() {
		t.Fatal("expected parser to track successful plan block")
	}
	discardPlanAssistantContentIfNeeded(true, parser, &content, &thinking)
	if content.Len() != 0 {
		t.Fatalf("content should be discarded after successful plan card, got %q", content.String())
	}
	if thinking.Len() != 0 {
		t.Fatalf("thinking should be discarded with plan assistant content, got %q", thinking.String())
	}
}

func TestPlanProtocolToolCallEmitsPlanCardInsteadOfToolCall(t *testing.T) {
	var events []Event
	parser := newPlanProtocolParser(agentEventMetadata{
		AgentKind: AgentKindIDE,
		RunID:     "run-plan-tool",
	}, func(Event) {})
	msg := schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-plan",
		Function: schema.FunctionCall{
			Name:      "plan_questions",
			Arguments: `{"questions":[{"id":"scope","question":"确认范围？"}]}`,
		},
	}})
	var content strings.Builder
	var thinking strings.Builder

	processNonStreamingEvent(&adk.MessageVariant{Message: msg}, &content, &thinking, 0, agentEventMetadata{
		AgentKind: AgentKindIDE,
		RunID:     "run-plan-tool",
	}, parser, func(ev Event) {
		events = append(events, ev)
	})

	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1: %#v", len(events), events)
	}
	if events[0].Type != "plan_question" {
		t.Fatalf("event type = %q, want plan_question", events[0].Type)
	}
	if eventDataString(events[0].Data, "status") != "success" {
		t.Fatalf("status = %q, want success", eventDataString(events[0].Data, "status"))
	}
	if eventDataString(events[0].Data, "id") != planProtocolToolEventID {
		t.Fatalf("id = %q, want %q", eventDataString(events[0].Data, "id"), planProtocolToolEventID)
	}
	if !strings.Contains(eventDataString(events[0].Data, "content"), `"questions"`) {
		t.Fatalf("content should keep plan question payload: %#v", events[0])
	}
	if !parser.HasSuccessfulBlock() {
		t.Fatal("expected plan tool call to count as a successful plan block")
	}
	if content.Len() != 0 || thinking.Len() != 0 {
		t.Fatalf("plan tool call should not append visible assistant text, content=%q thinking=%q", content.String(), thinking.String())
	}
}
