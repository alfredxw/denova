package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/internal/interactive"
)

func TestOpeningGameStateSchemaToolUsesDedicatedStructureOnlyEntry(t *testing.T) {
	var submitted interactive.ActorStateSchemaBatch
	tools, err := newInteractiveOpeningStateSchemaTools(InteractiveStoryToolContext{
		SubmitStateSchemaBatch: func(_ context.Context, batch interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
			submitted = batch
			return interactive.ActorStateSchemaBatchResult{Accepted: []interactive.ActorStateSchemaBatchAccepted{}, Rejected: []interactive.ActorStateSchemaBatchIssue{}, Blocked: []interactive.ActorStateSchemaBatchIssue{}, Finalized: true}, nil
		},
	})
	if err != nil || len(tools) != 1 {
		t.Fatalf("build opening Game Agent schema tool: tools=%d err=%v", len(tools), err)
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != initializeStoryStateSchemaToolName || !strings.Contains(info.Desc, "schema_only") || !strings.Contains(info.Desc, `source={"kind":"opening","id":"opening-draft"}`) || !strings.Contains(info.Desc, "最小可照抄示例") || !strings.Contains(info.Desc, "原子落盘") {
		t.Fatalf("unexpected opening schema tool contract: %#v", info)
	}
	parameters, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(parameters)
	if err != nil {
		t.Fatal(err)
	}
	schemaText := string(data)
	for _, expected := range []string{`"enum":["opening","lore","trpg"]`, `"enum":["confirmed","inferred","default"]`, `"enum":["schema_only"]`, `"enum":["number","string","bool","enum","object","list"]`, `"enum":["covered","add","replace","ignored"]`, `"enum":["add","remove","fields"]`, `"maxItems":16`} {
		if !strings.Contains(schemaText, expected) {
			t.Fatalf("opening schema must expose strict bounded enums; missing %s in %s", expected, schemaText)
		}
	}
	if strings.Contains(schemaText, `"initial_actor_ops"`) || strings.Contains(schemaText, `"actor_ops"`) {
		t.Fatalf("opening structure-only schema must not expose Actor value operations: %s", schemaText)
	}
	invokable := tools[0].(tool.InvokableTool)
	if _, err := invokable.InvokableRun(context.Background(), `{"summary":"现有字段覆盖开局需求","items":[{"item_id":"schema-covered-review","requirements":[{"source":{"kind":"opening","id":"opening-draft"},"requirement":"主角姓名需要长期记录，现有字段已覆盖","evidence_kind":"confirmed","value_policy":"schema_only","expected_type":"string","decision":"covered","template_id":"protagonist","field_id":"姓名"}],"adaptation":{"template_ops":[]}}],"finalize":true}`); err != nil {
		t.Fatal(err)
	}
	if !submitted.Finalize || len(submitted.Items) != 1 || len(submitted.Items[0].Requirements) != 1 {
		t.Fatalf("opening schema tool did not forward the dedicated input: %#v", submitted)
	}
	requirement := submitted.Items[0].Requirements[0]
	if requirement.Source.Kind != "opening" || requirement.Source.ID != "opening-draft" || requirement.ExpectedType != "string" || requirement.Decision != "covered" || requirement.FieldID != "姓名" {
		t.Fatalf("opening schema tool changed the strict requirement during conversion: %#v", requirement)
	}
}
