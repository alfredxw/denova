package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/invopop/jsonschema"
)

type constrainedToolArguments struct {
	Count int    `json:"count" jsonschema:"minimum=2,maximum=6,multipleOf=2"`
	Code  string `json:"code" jsonschema:"pattern=^[A-Z]{3}$"`
}

func TestNormalizeToolArgumentsEnforcesStableConstraints(t *testing.T) {
	info, err := GoStruct2ToolInfo[constrainedToolArguments]("constrained", "")
	if err != nil {
		t.Fatal(err)
	}
	if normalized, err := NormalizeToolArguments(info, `{"count":"4","code":"ABC"}`); err != nil || normalized != `{"code":"ABC","count":4}` {
		t.Fatalf("normalized=%s err=%v", normalized, err)
	}
	for _, test := range []struct {
		arguments string
		want      string
	}{
		{arguments: `{"count":0,"code":"ABC"}`, want: "minimum"},
		{arguments: `{"count":8,"code":"ABC"}`, want: "maximum"},
		{arguments: `{"count":3,"code":"ABC"}`, want: "multiple"},
		{arguments: `{"count":4,"code":"abc"}`, want: "pattern"},
	} {
		if _, err := NormalizeToolArguments(info, test.arguments); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("arguments %s: err=%v, want %q", test.arguments, err, test.want)
		}
	}
}

func TestNormalizeToolArgumentsAppliesDefaultsDropsForbiddenExtrasAndCoercesScalars(t *testing.T) {
	info := toolInfoFromSchema(t, `{
		"type":"object",
		"properties":{
			"count":{"type":"integer"},
			"enabled":{"type":"boolean"},
			"mode":{"type":"string","default":"safe"}
		},
		"required":["count","enabled","mode"],
		"additionalProperties":false
	}`)
	normalized, err := NormalizeToolArguments(info, `{"count":"12","enabled":"false","extra":"ignored"}`)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(normalized), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"count": float64(12), "enabled": false, "mode": "safe"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized arguments = %#v, want %#v", got, want)
	}
}

func TestNormalizeToolArgumentsRejectsInputWithOnlyForbiddenExtras(t *testing.T) {
	info := toolInfoFromSchema(t, `{
		"type":"object",
		"properties":{"paths":{"type":"array","items":{"type":"string"}}},
		"additionalProperties":false
	}`)
	assertToolArgumentIssue(t, info, `{"pattern":"legacy"}`, "unsupported_arguments", "$")
}

func TestNormalizeToolArgumentsDoesNotGuessEnumOrArrayValues(t *testing.T) {
	info := toolInfoFromSchema(t, `{
		"type":"object",
		"properties":{
			"kind":{"type":"integer","enum":[1,2]},
			"paths":{"type":"array","items":{"type":"string"}}
		},
		"required":["kind","paths"],
		"additionalProperties":false
	}`)
	for _, arguments := range []string{
		`{"kind":"1","paths":["a.md"]}`,
		`{"kind":1,"paths":"a.md"}`,
	} {
		_, err := NormalizeToolArguments(info, arguments)
		var argumentErr *ToolArgumentsError
		if !errors.As(err, &argumentErr) || len(argumentErr.Issues) != 1 || argumentErr.Issues[0].Code != "type_conflict" {
			t.Fatalf("arguments=%s err=%#v", arguments, err)
		}
	}
}

func TestNormalizeToolArgumentsRejectsNonJSONNumericStrings(t *testing.T) {
	info := toolInfoFromSchema(t, `{
		"type":"object",
		"properties":{"count":{"type":"integer"}},
		"required":["count"]
	}`)
	for _, arguments := range []string{
		`{"count":"2/1"}`,
		`{"count":"+2"}`,
	} {
		assertToolArgumentIssue(t, info, arguments, "type_conflict", "$.count")
	}
}

func TestNormalizeToolArgumentsReturnsStructuredMissingAndBranchErrors(t *testing.T) {
	missingInfo := toolInfoFromSchema(t, `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	assertToolArgumentIssue(t, missingInfo, `{}`, "missing_required", "$.path")

	ambiguousInfo := toolInfoFromSchema(t, `{
		"oneOf":[
			{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]},
			{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}
		]
	}`)
	assertToolArgumentIssue(t, ambiguousInfo, `{"value":"same"}`, "branch_ambiguity", "$")
}

func TestNormalizeToolArgumentsCanonicalizesIntegralExponentForTypedDecode(t *testing.T) {
	var got int
	tool, err := InferTool("integer", "", func(_ context.Context, input struct {
		Value int `json:"value"`
	}) (string, error) {
		got = input.Value
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Run(context.Background(), `{"value":1e0}`)
	if err != nil || result.ModelContent != "ok" || got != 1 {
		t.Fatalf("result=%#v got=%d err=%v", result, got, err)
	}
}

func toolInfoFromSchema(t *testing.T, raw string) *ToolInfo {
	t.Helper()
	schema := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(raw), schema); err != nil {
		t.Fatal(err)
	}
	return &ToolInfo{Name: "schema_test", ParamsOneOf: NewParamsOneOfByJSONSchema(schema)}
}

func assertToolArgumentIssue(t *testing.T, info *ToolInfo, arguments, code, path string) {
	t.Helper()
	_, err := NormalizeToolArguments(info, arguments)
	var argumentErr *ToolArgumentsError
	if !errors.As(err, &argumentErr) || len(argumentErr.Issues) != 1 ||
		argumentErr.Issues[0].Code != code || argumentErr.Issues[0].Path != path {
		t.Fatalf("arguments=%s err=%#v", arguments, err)
	}
}
