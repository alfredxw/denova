package agent

import (
	"strings"
	"testing"
)

type constrainedToolArguments struct {
	Count int    `json:"count" jsonschema:"minimum=2,maximum=6,multipleOf=2"`
	Code  string `json:"code" jsonschema:"pattern=^[A-Z]{3}$"`
}

func TestValidateToolArgumentsEnforcesNumericAndPatternKeywords(t *testing.T) {
	info, err := GoStruct2ToolInfo[constrainedToolArguments]("constrained", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateToolArguments(info, `{"count":4,"code":"ABC"}`); err != nil {
		t.Fatalf("valid arguments rejected: %v", err)
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
		if err := ValidateToolArguments(info, test.arguments); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("arguments %s: err=%v, want %q", test.arguments, err, test.want)
		}
	}
}

func TestValidateToolArgumentsAcceptsMathematicallyIntegralExponent(t *testing.T) {
	info, err := GoStruct2ToolInfo[struct {
		Value int `json:"value"`
	}]("integer", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateToolArguments(info, `{"value":1e0}`); err != nil {
		t.Fatalf("integral JSON number rejected: %v", err)
	}
}
