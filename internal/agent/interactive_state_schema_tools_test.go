package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/internal/interactive"
)

func TestInteractiveStateSchemaToolUsesIncrementalBatchContract(t *testing.T) {
	var received interactive.ActorStateSchemaBatch
	tools, err := newInteractiveStateSchemaTools(InteractiveStoryToolContext{
		SubmitStateSchemaBatch: func(_ context.Context, batch interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
			received = batch
			return interactive.ActorStateSchemaBatchResult{
				Accepted: []interactive.ActorStateSchemaBatchAccepted{{ItemID: "status"}},
				Rejected: []interactive.ActorStateSchemaBatchIssue{}, Blocked: []interactive.ActorStateSchemaBatchIssue{},
				DraftAcceptedItems: 1, Finalized: true,
			}, nil
		},
	})
	if err != nil || len(tools) != 1 {
		t.Fatalf("build state schema Batch tool: tools=%d err=%v", len(tools), err)
	}
	invokable, ok := tools[0].(tool.InvokableTool)
	if !ok {
		t.Fatal("state schema tool must be invokable")
	}
	output, err := invokable.InvokableRun(context.Background(), `{"summary":"review","items":[{"item_id":"status","requirements":[{"source":{"kind":"opening","id":"turn-1"},"requirement":"initialize status","evidence_kind":"confirmed","value_policy":"initialize","actor_id":"protagonist","expected_type":"string","decision":"covered","template_id":"protagonist","field_id":"status"}],"adaptation":{"actor_ops":[{"op":"set","actor_id":"protagonist","field_id":"status","value":"ready"}]}}],"finalize":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if !received.Finalize || len(received.Items) != 1 || received.Items[0].ItemID != "status" {
		t.Fatalf("tool did not decode Batch input: %#v", received)
	}
	requirement := received.Items[0].Requirements[0]
	actorOp := received.Items[0].Adaptation.ActorOps[0]
	if requirement.ValuePolicy != interactive.ActorStateSchemaValuePolicyInitialize || requirement.ActorID != "protagonist" || actorOp.Op != "set" || actorOp.FieldID != "status" || actorOp.Value != "ready" {
		t.Fatalf("tool must decode explicit Actor value initialization: requirement=%#v actor_op=%#v", requirement, actorOp)
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"value_policy", "actor_ops set", "精确错误路径"} {
		if !strings.Contains(info.Desc, want) {
			t.Fatalf("state schema tool description missing %q: %s", want, info.Desc)
		}
	}
	params, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	schemaData, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(schemaData), "value_source") {
		t.Fatalf("backend-owned actor provenance must not be exposed in model input schema: %s", schemaData)
	}
	for _, property := range []string{`"value_policy"`, `"actor_id"`, `"field_id"`, `"value"`} {
		if !strings.Contains(string(schemaData), property) {
			t.Fatalf("model input schema missing Actor value contract property %s: %s", property, schemaData)
		}
	}
	var schemaValue any
	if err := json.Unmarshal(schemaData, &schemaValue); err != nil {
		t.Fatal(err)
	}
	if schemaContainsRequirementItemID(schemaValue) {
		t.Fatalf("backend-owned requirement item_id must not be exposed in model input schema: %s", schemaData)
	}
	for _, want := range []string{`"accepted"`, `"rejected"`, `"blocked"`, `"finalized": true`} {
		if !strings.Contains(output, want) {
			t.Fatalf("Batch tool output missing %q: %s", want, output)
		}
	}
}

func schemaContainsRequirementItemID(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if properties, ok := typed["properties"].(map[string]any); ok {
			_, hasSource := properties["source"]
			_, hasRequirement := properties["requirement"]
			_, hasDecision := properties["decision"]
			_, hasItemID := properties["item_id"]
			if hasSource && hasRequirement && hasDecision && hasItemID {
				return true
			}
		}
		for _, child := range typed {
			if schemaContainsRequirementItemID(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if schemaContainsRequirementItemID(child) {
				return true
			}
		}
	}
	return false
}
