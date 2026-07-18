package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"denova/internal/interactive"
)

func TestInteractiveTurnToolsExposeOneStructuredSubmissionTool(t *testing.T) {
	var submitted interactive.TurnSubmissionInput
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		SubmitTurnResult: func(_ context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
			submitted = input
			return interactive.TurnSubmissionReceipt{Ready: true}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("game Agent should receive one turn submission tool, got %d", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	schemaText := string(data)
	if info.Name != interactiveTurnSubmissionToolName || !strings.Contains(schemaText, `"state_changes"`) || !strings.Contains(schemaText, `"choices"`) {
		t.Fatalf("unexpected unified tool schema: name=%q schema=%s", info.Name, schemaText)
	}
	if strings.Contains(schemaText, `"patches"`) || strings.Contains(schemaText, `"path"`) {
		t.Fatalf("model-facing schema must not expose legacy patches or JSON Pointer paths: %s", schemaText)
	}

	turnTool, ok := tools[0].(*submitInteractiveTurnTool)
	if !ok {
		t.Fatalf("unexpected submission tool implementation: %T", tools[0])
	}
	_, err = turnTool.InvokableRun(context.Background(), `{"state_changes":[{"op":"replace","actor_id":"protagonist","field_id":"状态","value":"警惕"}],"choices":["前进","观察","交谈","等待","后退"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if submitted.StateUpdates == nil || len(*submitted.StateUpdates) != 1 || submitted.Choices == nil || len(*submitted.Choices) != 5 {
		t.Fatalf("unified tool did not independently decode both modules: %#v", submitted)
	}
}
