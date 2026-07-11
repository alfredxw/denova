package interactive

import "testing"

func TestParsePlanDecisionNormalizesStructuredAndLegacyOutput(t *testing.T) {
	decision := ParsePlanDecision(`{"mode":"replan","triggers":["major_deviation"],"deviation":{"level":"major","invalidated_plan_refs":["beat-2"]},"reason":"关键前提失效"}`)
	if decision.Mode != PlanDecisionReplan || decision.Deviation.Level != "major" || len(decision.Deviation.InvalidatedPlanRefs) != 1 {
		t.Fatalf("unexpected structured decision: %#v", decision)
	}
	legacy := ParsePlanDecision("已更新最近分支安排。")
	if legacy.Mode != PlanDecisionPatch || legacy.Reason == "" {
		t.Fatalf("legacy output should degrade to patch: %#v", legacy)
	}
}
