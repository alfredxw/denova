package agentrun

import (
	"testing"
)

func TestRunTraceResolvesChildrenOutsideCatalogAndDetailCaps(t *testing.T) {
	workspace := t.TempDir()
	location := TraceLocation{Workspace: workspace}
	child, err := NewLedgerForRun(workspace, DefaultLoopPolicy().RunLedger, Options{RootAgentName: "writer", SessionID: "child-session"}, "child")
	if err != nil {
		t.Fatal(err)
	}
	if err := child.RecordFinish("success", "", 0); err != nil {
		t.Fatal(err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	parent, err := NewLedgerForRun(workspace, DefaultLoopPolicy().RunLedger, Options{}, "parent")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 700; index++ {
		if index == 350 {
			if err := parent.Record("child_run", map[string]any{"id": "child", "session_id": "child-session", "agent_name": "writer"}); err != nil {
				t.Fatal(err)
			}
			if err := parent.RecordTraceContent(TraceContentRecord{Type: "tool_output", Payload: map[string]any{
				"tool_name": "task", "execution_id": "start-call", "result": `{"results":[{"task":{"ref":{"run":"child","session":"child-session","agent":"writer"}}},{"error":"failed item"},{"task":{"ref":{"run":"old-child","session":"old-session","agent":"reader"}}}]}`,
			}}); err != nil {
				t.Fatal(err)
			}
		}
		if err := parent.Record("agent_cycle", map[string]any{"count": index}); err != nil {
			t.Fatal(err)
		}
	}
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	catalog, err := ListRunTraces(location, 1)
	if err != nil || len(catalog) != 1 || catalog[0].ID != "parent" {
		t.Fatalf("catalog = %#v, %v", catalog, err)
	}
	trace, err := ReadRunTrace(location, "parent")
	if err != nil {
		t.Fatal(err)
	}
	if !trace.Truncated || len(trace.Summary.ChildRuns) != 2 || len(trace.Children) != 2 {
		t.Fatalf("truncated trace lost child references: %#v", trace.Summary)
	}
	if trace.Children[0].ID != "child" || trace.Children[0].Status != "success" || trace.Children[1].Status != "unavailable" {
		t.Fatalf("related traces = %#v", trace.Children)
	}
	if trace.Summary.ChildRuns[0].ParentCallID != "start-call" {
		t.Fatalf("task receipt lost call identity: %#v", trace.Summary.ChildRuns)
	}
}
