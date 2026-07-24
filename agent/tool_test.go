package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/invopop/jsonschema"
)

type inferToolArgs struct {
	Mode  string   `json:"mode" jsonschema:"enum=fast,enum=slow" jsonschema_description:"Execution mode."`
	Count int      `json:"count" jsonschema_description:"Number of items."`
	Items []string `json:"items,omitempty" jsonschema:"required,minItems=1,maxItems=3" jsonschema_description:"Bounded item list."`
	Note  string   `json:"note,omitempty"`
}

type inferToolResult struct {
	Total int `json:"total"`
}

func TestInferToolSchemaAndStrictInvoke(t *testing.T) {
	current, err := InferTool("sum", "Adds values", func(_ context.Context, input inferToolArgs) (inferToolResult, error) {
		return inferToolResult{Total: input.Count}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := current.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "sum" || info.Desc != "Adds values" {
		t.Fatalf("info = %#v", info)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"$schema"`) || strings.Contains(string(encoded), `"$ref"`) {
		t.Fatalf("schema is not provider-inline: %s", encoded)
	}
	mode, exists := schema.Properties.Get("mode")
	if !exists || mode.Description != "Execution mode." || len(mode.Enum) != 2 {
		t.Fatalf("mode schema lost description/enum: %#v", mode)
	}
	if !containsString(schema.Required, "mode") || !containsString(schema.Required, "count") || containsString(schema.Required, "note") {
		t.Fatalf("required = %#v", schema.Required)
	}
	items, exists := schema.Properties.Get("items")
	if !exists || items.Description != "Bounded item list." || items.MinItems == nil || *items.MinItems != 1 ||
		items.MaxItems == nil || *items.MaxItems != 3 || !containsString(schema.Required, "items") {
		t.Fatalf("items schema lost required/minItems/maxItems/description: %#v; required=%#v", items, schema.Required)
	}

	output, err := current.InvokableRun(context.Background(), `{"mode":"fast","count":3,"items":["x"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if output != `{"total":3}` {
		t.Fatalf("non-string output = %s", output)
	}
	for _, invalid := range []string{
		`{"mode":"fast","count":3,"items":["x"],"unknown":true}`,
		`{"mode":"fast","count":3,"items":["x"]} {}`,
		`{"mode":"invalid","count":3,"items":["x"]}`,
		`{"mode":"fast","count":3,"items":[]}`,
		`{"mode":"fast","count":3,"items":["a","b","c","d"]}`,
		`{"mode":"fast"}`,
	} {
		if _, err := current.InvokableRun(context.Background(), invalid); err == nil {
			t.Fatalf("expected strict decode error for %s", invalid)
		}
	}
}

func TestInferToolStringOutputAndOneOfPreservation(t *testing.T) {
	current, err := InferTool("echo", "Echoes", func(_ context.Context, input struct {
		Value string `json:"value"`
	}) (string, error) {
		return input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := current.InvokableRun(context.Background(), `{"value":"plain"}`)
	if err != nil {
		t.Fatal(err)
	}
	if output != "plain" {
		t.Fatalf("string output was JSON encoded: %q", output)
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal([]byte(`{
		"type":"object",
		"oneOf":[
			{"required":["a"],"properties":{"a":{"type":"string","enum":["x"]}}},
			{"required":["b"],"properties":{"b":{"type":"integer"}}}
		]
	}`), &schema); err != nil {
		t.Fatal(err)
	}
	params := NewParamsOneOfByJSONSchema(&schema)
	visible, err := params.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if len(visible.OneOf) != 2 || visible.OneOf[0].Properties == nil {
		t.Fatalf("oneOf was lost: %#v", visible)
	}
}

func TestRegistryRejectsDuplicatesAndPreservesOrder(t *testing.T) {
	one, err := InferTool("one", "", func(context.Context, struct{}) (string, error) { return "1", nil })
	if err != nil {
		t.Fatal(err)
	}
	two, err := InferTool("two", "", func(context.Context, struct{}) (string, error) { return "2", nil })
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry(context.Background(), one, two)
	if err != nil {
		t.Fatal(err)
	}
	infos := registry.Schemas()
	if len(infos) != 2 || infos[0].Name != "one" || infos[1].Name != "two" {
		t.Fatalf("registry order = %#v", infos)
	}
	if _, exists := registry.Lookup("two"); !exists {
		t.Fatal("registered tool not found")
	}
	if err := registry.Register(context.Background(), one); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
