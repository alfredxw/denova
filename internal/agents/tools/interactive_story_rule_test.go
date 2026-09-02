package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/interactive"
)

func TestPrepareInteractiveTurnHidesAuthoritativeModifiersFromAgent(t *testing.T) {
	definitions, err := newInteractiveTurnTools(InteractiveContext{
		PrepareTurn: func(context.Context, interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
			return interactive.RuleResolution{
				ID:      "rr_hidden",
				Request: interactive.TurnCheckRequest{Cost: "The patrol arrives.", Difficulty: "hard"},
				Result: interactive.RuleResult{
					Label: "Lock check", Dice: "1d20", RollMode: "normal", Rolls: []int{20}, KeptRoll: 20,
					BonusTotal: 10, BaseTarget: 15, Total: 30, Target: 18, DifficultyShift: 1,
					Outcome: "critical_success", Result: "The lock opens silently.",
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 {
		t.Fatalf("prepare tool count = %d, want 1", len(definitions))
	}
	info, err := definitions[0].Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	rule, ok := schema.Properties.Get("rule")
	if !ok || rule == nil || rule.Properties == nil {
		t.Fatalf("prepare tool has no rule schema: %#v", rule)
	}
	if _, ok := rule.Properties.Get("modifier"); ok {
		t.Fatalf("prepare tool schema exposed rule.modifier: %#v", rule)
	}

	result, err := definitions[0].Tool.Run(context.Background(), `{
		"action":"pick the lock",
		"intent":"enter the warehouse",
		"challenge":"open it before the patrol arrives",
		"cost":"failure reveals the intrusion",
		"state":"The protagonist has simple tools.",
		"difficulty":"hard",
		"outcomes":{
			"critical_success":{"result":"Open it silently."},
			"success":{"result":"Open it."},
			"failure":{"result":"The lock stays shut."},
			"critical_failure":{"result":"The tool breaks."}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agent.ToolResultSuccess {
		t.Fatalf("prepare result status = %q", result.Status)
	}
	for _, hidden := range []string{"target", "total", "rolls", "bonus_total", "difficulty_shift"} {
		if strings.Contains(result.ModelContent, `"`+hidden+`"`) {
			t.Fatalf("model result exposed %s: %s", hidden, result.ModelContent)
		}
	}
	if !strings.Contains(result.ModelContent, `"outcome": "critical_success"`) ||
		!strings.Contains(result.ModelContent, `"result": "The lock opens silently."`) {
		t.Fatalf("model result lost the ruling: %s", result.ModelContent)
	}
	if !strings.Contains(result.DisplayContent, `"target": 18`) ||
		!strings.Contains(result.DisplayContent, `"bonus_total": 10`) {
		t.Fatalf("display result lost creator audit data: %s", result.DisplayContent)
	}
	if !json.Valid(result.Details) {
		t.Fatalf("prepare result details are not valid JSON: %s", result.Details)
	}
}
