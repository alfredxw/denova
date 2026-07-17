package interactive

import (
	"strings"
	"testing"
)

func TestParsePlanDecisionJSONNormalizesStructuredOutput(t *testing.T) {
	decision, err := ParsePlanDecisionJSON(`{"mode":"replan","triggers":["major_deviation"],"deviation":{"level":"major","invalidated_plan_refs":["beat-2"]},"reason":"关键前提失效"}`)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Mode != PlanDecisionReplan || decision.Deviation.Level != "major" || len(decision.Deviation.InvalidatedPlanRefs) != 1 {
		t.Fatalf("unexpected structured decision: %#v", decision)
	}
}

func TestParsePlanDecisionJSONExtractsOneObjectAndRejectsNarrationOnly(t *testing.T) {
	decision, err := ParsePlanDecisionJSON("I reviewed the plan.\n```json\n{\"mode\":\"replan\",\"reason\":\"opening plan required\"}\n```")
	if err != nil {
		t.Fatalf("parse fenced decision: %v", err)
	}
	if decision.Mode != PlanDecisionReplan || decision.Reason != "opening plan required" {
		t.Fatalf("unexpected normalized decision: %#v", decision)
	}

	if _, err := ParsePlanDecisionJSON("I updated the plan files."); err == nil || !strings.Contains(err.Error(), "PlanDecision") {
		t.Fatalf("narration-only output must fail with a useful error: %v", err)
	}
}

func TestParsePlanDecisionJSONRejectsMultipleValidObjects(t *testing.T) {
	output := "Example: {\"mode\":\"keep\",\"reason\":\"example\"}\nFinal: {\"mode\":\"replan\",\"reason\":\"actual\"}"
	if _, err := ParsePlanDecisionJSON(output); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("multiple valid decisions must be rejected instead of choosing the first: %v", err)
	}
}

func TestParsePlanDecisionJSONDoesNotPromoteNestedObject(t *testing.T) {
	output := `{ "example": { "mode": "keep", "reason": "nested only" } }`
	if _, err := ParsePlanDecisionJSON(output); err == nil {
		t.Fatal("a nested decision must not be accepted as the top-level PlanDecision")
	}
}

func TestParsePlanDecisionJSONHandlesBracesInsideStrings(t *testing.T) {
	decision, err := ParsePlanDecisionJSON(`prefix {"mode":"patch","reason":"preserve {this} text"} suffix`)
	if err != nil {
		t.Fatalf("parse decision containing braces in a string: %v", err)
	}
	if decision.Mode != PlanDecisionPatch || decision.Reason != "preserve {this} text" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}
