package chat

import (
	"encoding/json"
	"strings"
	"testing"

	agentplan "denova/internal/agents/plan"
	agentrun "denova/internal/agents/run"
)

func TestPlanProtocolParserExtractsProposalAcrossChunks(t *testing.T) {
	var events []agentrun.Event
	parser := agentplan.NewParser(agentplan.Metadata{}, planEventEmitter(func(ev agentrun.Event) { events = append(events, ev) }))

	var visible strings.Builder
	visible.WriteString(parser.Push(`旧协议<plan_questions>{"questions":[]}</plan_questions>然后`))
	visible.WriteString(parser.Push("<proposed_"))
	visible.WriteString(parser.Push("plan># 计划"))
	visible.WriteString(parser.Push("</proposed_plan>"))
	visible.WriteString(parser.Flush())

	if got := visible.String(); got != `旧协议<plan_questions>{"questions":[]}</plan_questions>然后` {
		t.Fatalf("visible = %q", got)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2: %#v", len(events), events)
	}
	if events[0].Type != "proposed_plan" || eventDataString(events[0].Data, "status") != "running" || eventDataString(events[0].Data, "id") == "" {
		t.Fatalf("unexpected running plan event: %#v", events[0])
	}
	if events[1].Type != "proposed_plan" || eventDataString(events[1].Data, "status") != "success" || eventDataString(events[1].Data, "id") != eventDataString(events[0].Data, "id") || eventDataString(events[1].Data, "content") != "# 计划" {
		t.Fatalf("unexpected successful plan event: %#v", events[1])
	}
}

func TestPlanProtocolParserFlushesUnclosedBlockAsVisibleText(t *testing.T) {
	var events []agentrun.Event
	parser := agentplan.NewParser(agentplan.Metadata{}, planEventEmitter(func(ev agentrun.Event) { events = append(events, ev) }))

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

func TestPlanProtocolParserPreservesDisplayedBlock(t *testing.T) {
	var events []agentrun.Event
	parser := agentplan.NewParser(agentplan.Metadata{}, planEventEmitter(func(ev agentrun.Event) { events = append(events, ev) }))
	rawContent := strings.Repeat("长计划内容。", 3000) + "计划尾部必须完整展示"

	_ = parser.Push("<proposed_plan>" + rawContent + "</proposed_plan>")
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if content := eventDataString(events[1].Data, "content"); content != rawContent {
		t.Fatalf("displayed plan was changed: got_bytes=%d want_bytes=%d", len(content), len(rawContent))
	}
}

func TestPlanProtocolToolCallPreservesLongInputContent(t *testing.T) {
	rawPlan := strings.Repeat("核对实施步骤。", 3000) + "工具输入尾部必须完整展示"
	args, err := json.Marshal(map[string]string{"content": rawPlan})
	if err != nil {
		t.Fatal(err)
	}
	var events []agentrun.Event
	handled, successful := agentplan.EmitToolCall("proposed_plan", string(args), agentplan.Metadata{}, planEventEmitter(func(ev agentrun.Event) { events = append(events, ev) }))
	if !handled || !successful || len(events) != 1 {
		t.Fatalf("plan tool result handled=%t successful=%t events=%#v", handled, successful, events)
	}
	if got := eventDataString(events[0].Data, "content"); got != rawPlan {
		t.Fatalf("plan tool input was changed: got_bytes=%d want_bytes=%d", len(got), len(rawPlan))
	}
}

func TestPlanProtocolParserCarriesRunMetadata(t *testing.T) {
	var events []agentrun.Event
	parser := agentplan.NewParser(agentplan.Metadata{
		AgentKind: agentrun.AgentKindIDE, RunID: "run-plan-1", AgentName: "DenovaAgent",
		RootAgentName: "DenovaAgent", RunPath: []string{"DenovaAgent"},
	}, planEventEmitter(func(ev agentrun.Event) { events = append(events, ev) }))

	_ = parser.Push(`<proposed_plan># Plan</proposed_plan>`)
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	for _, event := range events {
		if eventDataString(event.Data, "run_id") != "run-plan-1" ||
			eventDataString(event.Data, "agent_kind") != agentrun.AgentKindIDE ||
			eventDataString(event.Data, "agent_name") != "DenovaAgent" {
			t.Fatalf("plan event lost run metadata: %#v", event)
		}
	}
}

func TestPlanQuestionProtocolIsRetiredFromExecution(t *testing.T) {
	if agentplan.IsToolName("plan_questions") || agentplan.IsToolName("plan_question") {
		t.Fatal("legacy plan-question names must not be executable protocol tools")
	}
	handled, successful := agentplan.EmitToolCall("plan_questions", `{"questions":[]}`, agentplan.Metadata{}, func(agentplan.Event) {})
	if handled || successful {
		t.Fatalf("legacy question tool handled=%t successful=%t", handled, successful)
	}
}
