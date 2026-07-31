package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"denova/internal/interactive"
)

func TestOpeningGameStateSchemaToolUsesDedicatedStructureOnlyEntry(t *testing.T) {
	var submitted interactive.ActorStateSchemaBatch
	tools, err := newInteractiveOpeningStateSchemaTools(InteractiveContext{
		SubmitStateSchemaBatch: func(_ context.Context, batch interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
			submitted = batch
			return interactive.ActorStateSchemaBatchResult{Accepted: []interactive.ActorStateSchemaBatchAccepted{}, Rejected: []interactive.ActorStateSchemaBatchIssue{}, Blocked: []interactive.ActorStateSchemaBatchIssue{}, Finalized: true}, nil
		},
	})
	if err != nil || len(tools) != 1 {
		t.Fatalf("build opening Game Agent schema tool: tools=%d err=%v", len(tools), err)
	}
	info, err := tools[0].Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != initializeStoryStateSchemaToolName || !strings.Contains(info.Desc, "schema_only") || !strings.Contains(info.Desc, `source={"kind":"opening","id":"opening-draft"}`) || !strings.Contains(info.Desc, "story 是 actor_id") || !strings.Contains(info.Desc, "template_id 是 story_context") || !strings.Contains(info.Desc, "独立消耗") || !strings.Contains(info.Desc, "默认 10") || !strings.Contains(info.Desc, "关系阶段") || !strings.Contains(info.Desc, "initialization_guide") || !strings.Contains(info.Desc, "原子落盘") {
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
	for _, expected := range []string{`"enum":["opening","lore","trpg"]`, `"enum":["schema_only"]`, `"enum":["number","string","bool","enum","object","list"]`, `"enum":["covered","add","replace","remove","ignored"]`, `"enum":["add","remove","fields"]`, `"maxItems":16`} {
		if !strings.Contains(schemaText, expected) {
			t.Fatalf("opening schema must expose strict bounded enums; missing %s in %s", expected, schemaText)
		}
	}
	if strings.Contains(schemaText, `"evidence_kind"`) || strings.Contains(info.Desc, "evidence_kind") {
		t.Fatalf("opening schema tool must not expose the retired evidence_kind contract: info=%#v schema=%s", info, schemaText)
	}
	if strings.Contains(schemaText, `"initial_actor_ops"`) || strings.Contains(schemaText, `"actor_ops"`) {
		t.Fatalf("opening structure-only schema must not expose Actor value operations: %s", schemaText)
	}
	if _, err := runToolForTest(context.Background(), tools[0], `{"summary":"现有字段覆盖开局需求","items":[{"item_id":"schema-covered-review","requirements":[{"source":{"kind":"opening","id":"opening-draft"},"requirement":"主角姓名需要长期记录，现有字段已覆盖","value_policy":"schema_only","expected_type":"string","decision":"covered","template_id":"protagonist","field_id":"姓名"}],"adaptation":{"template_ops":[]}}],"finalize":true}`); err != nil {
		t.Fatal(err)
	}
	if !submitted.Finalize || len(submitted.Items) != 1 || len(submitted.Items[0].Requirements) != 1 {
		t.Fatalf("opening schema tool did not forward the dedicated input: %#v", submitted)
	}
	requirement := submitted.Items[0].Requirements[0]
	if requirement.Source.Kind != "opening" || requirement.Source.ID != "opening-draft" || requirement.ExpectedType != "string" || requirement.Decision != "covered" || requirement.FieldID != "姓名" {
		t.Fatalf("opening schema tool changed the strict requirement during conversion: %#v", requirement)
	}
	if _, err := runToolForTest(context.Background(), tools[0], `{"items":[{"item_id":"remove-strength","requirements":[{"source":{"kind":"opening","id":"opening-draft"},"requirement":"本故事不采用力量数值","value_policy":"schema_only","decision":"remove","template_id":"protagonist","field_id":"力量","reason":"改用境界体系"}],"adaptation":{"template_ops":[{"op":"fields","template_id":"protagonist","field_ops":[{"op":"remove","field_id":"力量"}]}]}}],"finalize":true}`); err != nil {
		t.Fatal(err)
	}
	removed := submitted.Items[0].Requirements[0]
	if removed.Decision != "remove" || removed.TemplateID != "protagonist" || removed.FieldID != "力量" || removed.Reason != "改用境界体系" {
		t.Fatalf("opening schema tool changed the explicit removal during conversion: %#v", removed)
	}
}
