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
	parameters, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	parameterData, err := json.Marshal(parameters)
	if err != nil {
		t.Fatal(err)
	}
	parameterText := string(parameterData)
	if !strings.Contains(parameterText, `"state_changes"`) || !strings.Contains(parameterText, `"type":"array"`) {
		t.Fatalf("state_changes must be a native array in the provider schema: %s", parameterText)
	}
	stateChangesSchema, ok := parameters.Properties.Get("state_changes")
	if !ok || stateChangesSchema.Items == nil || len(stateChangesSchema.Items.OneOf) != 5 {
		t.Fatalf("state_changes items must expose five disjoint operation variants: %s", parameterText)
	}
	if stateChangesSchema.MaxItems != nil {
		t.Fatalf("state_changes must not expose a hard operation-count limit: %#v", stateChangesSchema.MaxItems)
	}
	if !strings.Contains(stateChangesSchema.Description, "常规回合建议不超过 24 项") || !strings.Contains(stateChangesSchema.Description, "不是校验上限") {
		t.Fatalf("state_changes schema should retain explicit soft guidance: %s", stateChangesSchema.Description)
	}
	variants := map[string]bool{}
	for _, variant := range stateChangesSchema.Items.OneOf {
		if variant == nil || variant.Properties == nil {
			t.Fatalf("state change variant must be a closed object schema: %#v", variant)
		}
		opSchema, exists := variant.Properties.Get("op")
		if !exists || len(opSchema.Enum) != 1 {
			t.Fatalf("state change variant must have one constant-like op enum: %#v", variant)
		}
		op, _ := opSchema.Enum[0].(string)
		variants[op] = true
		switch op {
		case "replace", "delta":
			if _, exists := variant.Properties.Get("field_id"); !exists {
				t.Fatalf("%s variant must require field_id: %#v", op, variant)
			}
			if _, exists := variant.Properties.Get("template_id"); exists {
				t.Fatalf("%s variant must not expose create-only fields: %#v", op, variant)
			}
		case "create":
			for _, forbidden := range []string{"field_id", "subpath", "value"} {
				if _, exists := variant.Properties.Get(forbidden); exists {
					t.Fatalf("create variant must not expose %s: %#v", forbidden, variant)
				}
			}
			if _, exists := variant.Properties.Get("initial_state"); !exists {
				t.Fatalf("create variant must expose initial_state: %#v", variant)
			}
			if _, exists := variant.Properties.Get("name"); !exists {
				t.Fatalf("create variant must expose name: %#v", variant)
			}
			nameRequired := false
			for _, required := range variant.Required {
				if required == "name" {
					nameRequired = true
					break
				}
			}
			if !nameRequired {
				t.Fatalf("create variant must require name so it can equal actor_id: %#v", variant)
			}
		case "archive", "restore":
			if _, exists := variant.Properties.Get("reason"); !exists {
				t.Fatalf("%s variant must expose reason: %#v", op, variant)
			}
			for _, forbidden := range []string{"field_id", "subpath", "value", "template_id", "name", "initial_state"} {
				if _, exists := variant.Properties.Get(forbidden); exists {
					t.Fatalf("%s variant must not expose %s: %#v", op, forbidden, variant)
				}
			}
		default:
			t.Fatalf("unexpected state change variant op %q: %#v", op, variant)
		}
	}
	if !variants["replace"] || !variants["delta"] || !variants["create"] || !variants["archive"] || !variants["restore"] {
		t.Fatalf("state change variants incomplete: %#v", variants)
	}
	if !strings.Contains(info.Desc, "JSON.stringify") || !strings.Contains(info.Desc, "常规回合建议不超过 24 项") || !strings.Contains(info.Desc, "这不是校验上限") || !strings.Contains(info.Desc, "同一个 create.initial_state") || !strings.Contains(info.Desc, "不要通过删除") || !strings.Contains(info.Desc, "actor_id 与 name 必须完全相同") || !strings.Contains(info.Desc, "archive") || !strings.Contains(info.Desc, "restore") {
		t.Fatalf("submission tool must explain native arrays and fact-preserving retries: %s", info.Desc)
	}
	if strings.Contains(parameterText, "ASCII ID") {
		t.Fatalf("submission tool must not ask for ASCII Actor IDs: %s", parameterText)
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
