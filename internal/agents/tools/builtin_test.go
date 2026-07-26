package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTodoToolReplacesCompletePlanWithStructuredResult(t *testing.T) {
	definition, err := newTodoTool()
	if err != nil {
		t.Fatal(err)
	}
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "todo" {
		t.Fatalf("tool name = %q, want todo", info.Name)
	}

	result, err := runToolResultForTest(context.Background(), definition, `{"plan":[{"step":"Inspect auth flow","status":"completed"},{"step":"Fix refresh logic","status":"in_progress"},{"step":"Add regression test","status":"pending"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var got todoPlanResult
	if err := json.Unmarshal([]byte(result.ModelContent), &got); err != nil {
		t.Fatalf("todo result is not JSON: %v\n%s", err, result.ModelContent)
	}
	if got.Schema != todoPlanSchema || len(got.Plan) != 3 || got.Plan[1].Step != "Fix refresh logic" {
		t.Fatalf("todo result = %+v", got)
	}
	if string(result.Details) != result.ModelContent {
		t.Fatalf("todo details must match canonical result: details=%s model=%s", result.Details, result.ModelContent)
	}
}

func TestTodoToolAllowsClearingPlan(t *testing.T) {
	definition, err := newTodoTool()
	if err != nil {
		t.Fatal(err)
	}
	output, err := runToolForTest(context.Background(), definition, `{"plan":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if output != `{"schema":"todo.plan.v1","plan":[]}` {
		t.Fatalf("clear result = %s", output)
	}
}

func TestTodoToolRejectsInvalidPlan(t *testing.T) {
	definition, err := newTodoTool()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "empty step", args: `{"plan":[{"step":"  ","status":"pending"}]}`, want: "step is required"},
		{name: "multiple active", args: `{"plan":[{"step":"one","status":"in_progress"},{"step":"two","status":"in_progress"}]}`, want: "at most one"},
		{name: "invalid status", args: `{"plan":[{"step":"one","status":"blocked"}]}`, want: "status"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runToolForTest(context.Background(), definition, test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}
