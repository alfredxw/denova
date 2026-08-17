// Package director owns reusable Director decisions and scheduling policy.
package director

import "strings"

const (
	EventDecisionNone    = "none"
	EventDecisionSeed    = "seed"
	EventDecisionAdvance = "advance"
	EventDecisionPayoff  = "payoff"
	EventDecisionResolve = "resolve"
	EventDecisionAbandon = "abandon"
)

// EventDecision records how one planning decision changes an event thread.
type EventDecision struct {
	Mode            string   `json:"mode"`
	EventRef        string   `json:"event_ref,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	Evidence        []string `json:"evidence,omitempty"`
	EvidenceTurnIDs []string `json:"evidence_turn_ids,omitempty"`
}

// NormalizeEventDecision bounds and canonicalizes an event decision before it
// enters durable Director metadata.
func NormalizeEventDecision(decision EventDecision) EventDecision {
	decision.Mode = normalizeEnum(decision.Mode, EventDecisionNone, EventDecisionSeed, EventDecisionAdvance, EventDecisionPayoff, EventDecisionResolve, EventDecisionAbandon)
	decision.EventRef = trimBytes(strings.TrimSpace(decision.EventRef), 256)
	decision.Summary = trimBytes(strings.TrimSpace(decision.Summary), maxTextBytes)
	decision.Reason = trimBytes(strings.TrimSpace(decision.Reason), maxTextBytes)
	decision.Evidence = normalizeStringList(decision.Evidence)
	decision.EvidenceTurnIDs = normalizeStringList(decision.EvidenceTurnIDs)
	return decision
}
