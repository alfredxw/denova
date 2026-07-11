package interactive

import (
	"encoding/json"
	"strings"
)

const (
	PlanDecisionKeep   = "keep"
	PlanDecisionPatch  = "patch"
	PlanDecisionReplan = "replan"
)

type PlanDecision struct {
	Mode            string                 `json:"mode"`
	Triggers        []string               `json:"triggers,omitempty"`
	SceneTransition PlanDecisionTransition `json:"scene_transition,omitempty"`
	Deviation       PlanDecisionDeviation  `json:"deviation,omitempty"`
	Reason          string                 `json:"reason,omitempty"`
	BaseRevision    string                 `json:"base_revision,omitempty"`
}

type PlanDecisionTransition struct {
	Kind     string   `json:"kind,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type PlanDecisionDeviation struct {
	Level               string   `json:"level,omitempty"`
	InvalidatedPlanRefs []string `json:"invalidated_plan_refs,omitempty"`
	Reason              string   `json:"reason,omitempty"`
}

func ParsePlanDecision(output string) PlanDecision {
	trimmed := strings.TrimSpace(output)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	var decision PlanDecision
	if err := json.Unmarshal([]byte(trimmed), &decision); err != nil {
		return normalizePlanDecision(PlanDecision{Mode: PlanDecisionPatch, Reason: trimmed})
	}
	return normalizePlanDecision(decision)
}

func normalizePlanDecision(decision PlanDecision) PlanDecision {
	decision.Mode = normalizeEnum(decision.Mode, PlanDecisionKeep, PlanDecisionPatch, PlanDecisionReplan)
	decision.Triggers = normalizeStringListLimit(decision.Triggers, maxTurnBriefListItems)
	decision.SceneTransition.Kind = normalizeEnum(decision.SceneTransition.Kind, "none", "exit", "enter", "replace")
	decision.SceneTransition.From = trimBytes(decision.SceneTransition.From, 256)
	decision.SceneTransition.To = trimBytes(decision.SceneTransition.To, 256)
	decision.SceneTransition.Evidence = normalizeStringListLimit(decision.SceneTransition.Evidence, maxTurnBriefListItems)
	decision.Deviation.Level = normalizeEnum(decision.Deviation.Level, "none", "minor", "major")
	decision.Deviation.InvalidatedPlanRefs = normalizeStringListLimit(decision.Deviation.InvalidatedPlanRefs, maxTurnBriefListItems)
	decision.Deviation.Reason = trimBytes(decision.Deviation.Reason, maxTurnBriefTextBytes)
	decision.Reason = trimBytes(decision.Reason, maxTurnBriefTextBytes)
	decision.BaseRevision = trimBytes(decision.BaseRevision, 128)
	return decision
}
