package director

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	DecisionKeep   = "keep"
	DecisionPatch  = "patch"
	DecisionReplan = "replan"
)

type Decision struct {
	Mode            string             `json:"mode"`
	Triggers        []string           `json:"triggers,omitempty"`
	SceneTransition DecisionTransition `json:"scene_transition,omitempty"`
	Deviation       DecisionDeviation  `json:"deviation,omitempty"`
	Reason          string             `json:"reason,omitempty"`
	BaseRevision    string             `json:"base_revision,omitempty"`
	EventDecision   *EventDecision     `json:"event_decision,omitempty"`
}

type DecisionTransition struct {
	Kind     string   `json:"kind,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type DecisionDeviation struct {
	Level               string   `json:"level,omitempty"`
	InvalidatedPlanRefs []string `json:"invalidated_plan_refs,omitempty"`
	Reason              string   `json:"reason,omitempty"`
}

// ParseDecisionJSON extracts and validates one structured PlanDecision from
// model output. Surrounding narration is tolerated for recovery, but callers
// should persist only the returned normalized decision.
func ParseDecisionJSON(output string) (Decision, error) {
	valid := make([]Decision, 0, 1)
	for _, candidate := range topLevelJSONObjectCandidates(output) {
		decoder := json.NewDecoder(strings.NewReader(candidate))
		decoder.DisallowUnknownFields()
		var decision Decision
		if err := decoder.Decode(&decision); err != nil {
			continue
		}
		decision.Mode = strings.TrimSpace(decision.Mode)
		switch decision.Mode {
		case DecisionKeep, DecisionPatch, DecisionReplan:
			valid = append(valid, NormalizeDecision(decision))
		}
	}
	if len(valid) == 0 {
		return Decision{}, fmt.Errorf("PlanDecision JSON object not found")
	}
	if len(valid) != 1 {
		return Decision{}, fmt.Errorf("multiple valid PlanDecision JSON objects found: %d", len(valid))
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

// NormalizeDecision canonicalizes a Director decision before validation or persistence.
func NormalizeDecision(decision Decision) Decision {
	decision.Mode = normalizeEnum(decision.Mode, DecisionKeep, DecisionPatch, DecisionReplan)
	decision.Triggers = normalizeStringList(decision.Triggers)
	decision.SceneTransition.Kind = normalizeEnum(decision.SceneTransition.Kind, "none", "exit", "enter", "replace")
	decision.SceneTransition.From = trimBytes(decision.SceneTransition.From, 256)
	decision.SceneTransition.To = trimBytes(decision.SceneTransition.To, 256)
	decision.SceneTransition.Evidence = normalizeStringList(decision.SceneTransition.Evidence)
	decision.Deviation.Level = normalizeEnum(decision.Deviation.Level, "none", "minor", "major")
	decision.Deviation.InvalidatedPlanRefs = normalizeStringList(decision.Deviation.InvalidatedPlanRefs)
	decision.Deviation.Reason = trimBytes(decision.Deviation.Reason, maxTextBytes)
	decision.Reason = trimBytes(decision.Reason, maxTextBytes)
	decision.BaseRevision = trimBytes(decision.BaseRevision, 128)
	if decision.EventDecision != nil {
		normalized := NormalizeEventDecision(*decision.EventDecision)
		decision.EventDecision = &normalized
	}
	return decision
}
