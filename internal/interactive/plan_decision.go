package interactive

import (
	"encoding/json"
	"fmt"
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
	EventDecision   *EventDecision         `json:"event_decision,omitempty"`
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

// ParsePlanDecisionJSON extracts and validates one structured PlanDecision from
// model output. Surrounding narration is tolerated for recovery, but callers
// should persist only the returned normalized decision.
func ParsePlanDecisionJSON(output string) (PlanDecision, error) {
	valid := make([]PlanDecision, 0, 1)
	for _, candidate := range topLevelJSONObjectCandidates(output) {
		decoder := json.NewDecoder(strings.NewReader(candidate))
		decoder.DisallowUnknownFields()
		var decision PlanDecision
		if err := decoder.Decode(&decision); err != nil {
			continue
		}
		decision.Mode = strings.TrimSpace(decision.Mode)
		switch decision.Mode {
		case PlanDecisionKeep, PlanDecisionPatch, PlanDecisionReplan:
			valid = append(valid, normalizePlanDecision(decision))
		}
	}
	if len(valid) == 0 {
		return PlanDecision{}, fmt.Errorf("PlanDecision JSON object not found")
	}
	if len(valid) != 1 {
		return PlanDecision{}, fmt.Errorf("multiple valid PlanDecision JSON objects found: %d", len(valid))
	}
	return valid[0], nil
}

// topLevelJSONObjectCandidates finds complete outer objects while ignoring
// braces inside JSON strings. Nested objects are deliberately not returned as
// independent decisions.
func topLevelJSONObjectCandidates(output string) []string {
	candidates := make([]string, 0, 1)
	start := -1
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(output); i++ {
		ch := output[i]
		if depth > 0 && inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			if depth > 0 {
				inString = true
			}
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				candidates = append(candidates, output[start:i+1])
				start = -1
			}
		}
	}
	return candidates
}

func normalizePlanDecision(decision PlanDecision) PlanDecision {
	decision.Mode = normalizeEnum(decision.Mode, PlanDecisionKeep, PlanDecisionPatch, PlanDecisionReplan)
	decision.Triggers = normalizeStringListLimit(decision.Triggers, maxInteractiveListItems)
	decision.SceneTransition.Kind = normalizeEnum(decision.SceneTransition.Kind, "none", "exit", "enter", "replace")
	decision.SceneTransition.From = trimBytes(decision.SceneTransition.From, 256)
	decision.SceneTransition.To = trimBytes(decision.SceneTransition.To, 256)
	decision.SceneTransition.Evidence = normalizeStringListLimit(decision.SceneTransition.Evidence, maxInteractiveListItems)
	decision.Deviation.Level = normalizeEnum(decision.Deviation.Level, "none", "minor", "major")
	decision.Deviation.InvalidatedPlanRefs = normalizeStringListLimit(decision.Deviation.InvalidatedPlanRefs, maxInteractiveListItems)
	decision.Deviation.Reason = trimBytes(decision.Deviation.Reason, maxInteractiveTextBytes)
	decision.Reason = trimBytes(decision.Reason, maxInteractiveTextBytes)
	decision.BaseRevision = trimBytes(decision.BaseRevision, 128)
	if decision.EventDecision != nil {
		normalized := normalizeEventDecision(*decision.EventDecision)
		decision.EventDecision = &normalized
	}
	return decision
}
