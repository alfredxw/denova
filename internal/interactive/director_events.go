package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const maxDirectorEventDecisionHistory = 128

const (
	EventDecisionNone    = "none"
	EventDecisionSeed    = "seed"
	EventDecisionAdvance = "advance"
	EventDecisionPayoff  = "payoff"
	EventDecisionResolve = "resolve"
	EventDecisionAbandon = "abandon"
)

type EventDecision struct {
	Mode            string   `json:"mode"`
	EventRef        string   `json:"event_ref,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	Evidence        []string `json:"evidence,omitempty"`
	EvidenceTurnIDs []string `json:"evidence_turn_ids,omitempty"`
}

type EventOpportunity struct {
	Due              bool   `json:"due"`
	Kind             string `json:"kind"`
	Reason           string `json:"reason,omitempty"`
	TurnsSinceReview int    `json:"turns_since_review,omitempty"`
	ReviewInterval   int    `json:"review_interval,omitempty"`
	ActiveEventRef   string `json:"active_event_ref,omitempty"`
	Forced           bool   `json:"forced,omitempty"`
}

type DirectorEventThread struct {
	EventRef       string `json:"event_ref"`
	Summary        string `json:"summary,omitempty"`
	Stage          string `json:"stage,omitempty"`
	SeededTurnID   string `json:"seeded_turn_id,omitempty"`
	UpdatedTurnID  string `json:"updated_turn_id,omitempty"`
	LastDecisionID string `json:"last_decision_id,omitempty"`
}

type DirectorEventDecisionRecord struct {
	ID           string        `json:"id"`
	SourceTurnID string        `json:"source_turn_id"`
	Decision     EventDecision `json:"decision"`
}

type DirectorEventRuntime struct {
	Active                *DirectorEventThread          `json:"active,omitempty"`
	LastOpportunityTurnID string                        `json:"last_opportunity_turn_id,omitempty"`
	RecentDecisions       []DirectorEventDecisionRecord `json:"recent_decisions,omitempty"`
}

type DirectorEventCardIndex struct {
	EventRef  string   `json:"event_ref"`
	Name      string   `json:"name"`
	Category  string   `json:"category,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Intensity string   `json:"intensity,omitempty"`
}

func normalizeEventDecision(decision EventDecision) EventDecision {
	decision.Mode = normalizeEnum(decision.Mode, EventDecisionNone, EventDecisionSeed, EventDecisionAdvance, EventDecisionPayoff, EventDecisionResolve, EventDecisionAbandon)
	decision.EventRef = trimBytes(strings.TrimSpace(decision.EventRef), 256)
	decision.Summary = trimBytes(strings.TrimSpace(decision.Summary), maxInteractiveTextBytes)
	decision.Reason = trimBytes(strings.TrimSpace(decision.Reason), maxInteractiveTextBytes)
	decision.Evidence = normalizeStringListLimit(decision.Evidence, maxInteractiveListItems)
	decision.EvidenceTurnIDs = normalizeStringListLimit(decision.EvidenceTurnIDs, maxInteractiveListItems)
	return decision
}

func normalizeDirectorEventRuntime(runtime DirectorEventRuntime) DirectorEventRuntime {
	if len(runtime.RecentDecisions) > maxDirectorEventDecisionHistory {
		runtime.RecentDecisions = runtime.RecentDecisions[len(runtime.RecentDecisions)-maxDirectorEventDecisionHistory:]
	}
	for i := range runtime.RecentDecisions {
		runtime.RecentDecisions[i].Decision = normalizeEventDecision(runtime.RecentDecisions[i].Decision)
	}
	return runtime
}

func reconcileDirectorEventRuntime(runtime DirectorEventRuntime, turns []TurnEvent) DirectorEventRuntime {
	turnSet := make(map[string]bool, len(turns))
	for _, turn := range turns {
		turnSet[turn.ID] = true
	}
	filtered := make([]DirectorEventDecisionRecord, 0, len(runtime.RecentDecisions))
	for _, record := range runtime.RecentDecisions {
		if turnSet[record.SourceTurnID] {
			filtered = append(filtered, record)
		}
	}
	rebuilt := DirectorEventRuntime{RecentDecisions: filtered}
	for _, record := range filtered {
		rebuilt = replayDirectorEventDecision(rebuilt, record)
	}
	return normalizeDirectorEventRuntime(rebuilt)
}

func replayDirectorEventDecision(runtime DirectorEventRuntime, record DirectorEventDecisionRecord) DirectorEventRuntime {
	decision := normalizeEventDecision(record.Decision)
	switch decision.Mode {
	case EventDecisionNone:
		runtime.LastOpportunityTurnID = record.SourceTurnID
	case EventDecisionSeed:
		runtime.LastOpportunityTurnID = record.SourceTurnID
		runtime.Active = &DirectorEventThread{EventRef: decision.EventRef, Summary: decision.Summary, Stage: EventDecisionSeed, SeededTurnID: record.SourceTurnID, UpdatedTurnID: record.SourceTurnID, LastDecisionID: record.ID}
	case EventDecisionAdvance, EventDecisionPayoff:
		if runtime.Active != nil && runtime.Active.EventRef == decision.EventRef {
			runtime.Active.Summary = firstNonEmptyString(decision.Summary, runtime.Active.Summary)
			runtime.Active.Stage = decision.Mode
			runtime.Active.UpdatedTurnID = record.SourceTurnID
			runtime.Active.LastDecisionID = record.ID
		}
	case EventDecisionResolve, EventDecisionAbandon:
		if runtime.Active != nil && runtime.Active.EventRef == decision.EventRef {
			runtime.Active = nil
			runtime.LastOpportunityTurnID = record.SourceTurnID
		}
	}
	return runtime
}

func directorEventOpportunity(runtime DirectorEventRuntime, turns []TurnEvent, frequency string, hasCards, forced bool) EventOpportunity {
	runtime = reconcileDirectorEventRuntime(runtime, turns)
	if len(turns) == 0 {
		return EventOpportunity{Kind: "none", Reason: "opening_not_ready"}
	}
	currentTurnID := turns[len(turns)-1].ID
	for _, record := range runtime.RecentDecisions {
		if record.SourceTurnID == currentTurnID {
			return EventOpportunity{Kind: "none", Reason: "current_turn_already_evaluated"}
		}
	}
	if runtime.Active != nil {
		return EventOpportunity{Due: true, Kind: "active", Reason: "active_event_requires_review", ActiveEventRef: runtime.Active.EventRef, Forced: forced}
	}
	if !hasCards {
		return EventOpportunity{Kind: "none", Reason: "no_enabled_event_cards"}
	}
	interval := eventFrequencyInterval(frequency)
	if forced {
		return EventOpportunity{Due: true, Kind: "new", Reason: "manual_evaluation", ReviewInterval: interval, Forced: true}
	}
	if interval == 0 {
		return EventOpportunity{Kind: "none", Reason: "event_frequency_off"}
	}
	turnsSince := len(turns)
	if runtime.LastOpportunityTurnID != "" {
		for i, turn := range turns {
			if turn.ID == runtime.LastOpportunityTurnID {
				turnsSince = len(turns) - i - 1
				break
			}
		}
	}
	due := turnsSince >= interval
	reason := "cadence_not_due"
	if due {
		reason = "cadence_due"
	}
	return EventOpportunity{Due: due, Kind: map[bool]string{true: "new", false: "none"}[due], Reason: reason, TurnsSinceReview: turnsSince, ReviewInterval: interval}
}

func directorEventTurnsThrough(turns []TurnEvent, sourceTurnID string) []TurnEvent {
	sourceTurnID = strings.TrimSpace(sourceTurnID)
	if sourceTurnID == "" {
		return turns
	}
	for i := range turns {
		if turns[i].ID == sourceTurnID {
			return turns[:i+1]
		}
	}
	return turns
}

func eventFrequencyInterval(frequency string) int {
	switch normalizeEventFrequency(frequency) {
	case EventFrequencySparse:
		return 6
	case EventFrequencyBalanced:
		return 4
	case EventFrequencyFrequent:
		return 2
	default:
		return 0
	}
}

func applyDirectorEventDecision(runtime DirectorEventRuntime, decision *EventDecision, opportunity EventOpportunity, sourceTurnID string, turns []TurnEvent, catalog []DirectorEvent) (DirectorEventRuntime, error) {
	runtime = reconcileDirectorEventRuntime(runtime, turns)
	if decision == nil {
		if opportunity.Due && opportunity.Kind == "new" {
			return runtime, fmt.Errorf("本轮存在新事件机会，event_decision 不能为空")
		}
		return runtime, nil
	}
	normalized := normalizeEventDecision(*decision)
	if !opportunity.Due {
		return runtime, fmt.Errorf("本轮没有事件机会，不应提交 event_decision")
	}
	if opportunity.Kind == "new" && normalized.Mode != EventDecisionNone && normalized.Mode != EventDecisionSeed {
		return runtime, fmt.Errorf("新事件机会只能选择 none 或 seed")
	}
	if opportunity.Kind == "active" && (normalized.Mode == EventDecisionNone || normalized.Mode == EventDecisionSeed) {
		return runtime, fmt.Errorf("活跃事件只能 advance、payoff、resolve、abandon，未变化时省略 event_decision")
	}
	if normalized.Mode != EventDecisionNone && normalized.EventRef == "" {
		return runtime, fmt.Errorf("event_decision.event_ref 不能为空")
	}
	if normalized.Mode == EventDecisionSeed && !directorEventCatalogContains(catalog, normalized.EventRef) {
		return runtime, fmt.Errorf("事件卡不在当前故事导演显式选择的事件包中: %s", normalized.EventRef)
	}
	if opportunity.Kind == "active" && (runtime.Active == nil || normalized.EventRef != runtime.Active.EventRef) {
		return runtime, fmt.Errorf("event_decision.event_ref 必须匹配当前活跃事件")
	}
	if normalized.Mode == EventDecisionAdvance || normalized.Mode == EventDecisionPayoff || normalized.Mode == EventDecisionResolve {
		turnSet := map[string]bool{}
		for _, turn := range turns {
			turnSet[turn.ID] = true
		}
		validEvidence := false
		for _, turnID := range normalized.EvidenceTurnIDs {
			validEvidence = validEvidence || turnSet[turnID]
		}
		if !validEvidence {
			return runtime, fmt.Errorf("事件推进、回报或解决必须引用当前分支上的 evidence_turn_ids")
		}
	}
	record := DirectorEventDecisionRecord{SourceTurnID: sourceTurnID, Decision: normalized}
	record.ID = directorEventDecisionID(record)
	for _, existing := range runtime.RecentDecisions {
		if existing.ID == record.ID {
			return runtime, nil
		}
	}
	runtime.RecentDecisions = append(runtime.RecentDecisions, record)
	runtime = replayDirectorEventDecision(runtime, record)
	return normalizeDirectorEventRuntime(runtime), nil
}

func directorEventCatalogContains(catalog []DirectorEvent, ref string) bool {
	for _, event := range catalog {
		if event.Enabled && event.ID == ref {
			return true
		}
	}
	return false
}

func directorEventDecisionID(record DirectorEventDecisionRecord) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{record.SourceTurnID, record.Decision.Mode, record.Decision.EventRef}, "\x00")))
	return "event-decision-" + hex.EncodeToString(sum[:8])
}

func directorEventCardIndex(catalog []DirectorEvent) []DirectorEventCardIndex {
	result := make([]DirectorEventCardIndex, 0, len(catalog))
	for _, event := range catalog {
		if !event.Enabled {
			continue
		}
		result = append(result, DirectorEventCardIndex{EventRef: event.ID, Name: event.Name, Category: event.Category, Tags: event.CompatibleGenres, Intensity: event.Intensity})
	}
	return result
}

func (s *Store) DirectorEventContext(storyID, branchID, sourceTurnID string) (EventOpportunity, DirectorEventRuntime, []DirectorEventCardIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return EventOpportunity{}, DirectorEventRuntime{}, nil, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return EventOpportunity{}, DirectorEventRuntime{}, nil, err
	}
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return EventOpportunity{}, DirectorEventRuntime{}, nil, err
	}
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return EventOpportunity{}, DirectorEventRuntime{}, nil, err
	}
	director := s.storyDirectorForMeta(meta)
	catalog := DirectorEventCatalogFromStoryDirector(director)
	turns := directorEventTurnsThrough(snapshot.Turns, sourceTurnID)
	runtime := reconcileDirectorEventRuntime(plan.Metadata.EventRuntime, turns)
	opportunity := directorEventOpportunity(runtime, turns, director.Strategy.EventFrequency, len(catalog) > 0, false)
	if plan.Metadata.LastRun != nil && plan.Metadata.LastRun.SourceTurnID == sourceTurnID {
		opportunity = plan.Metadata.LastRun.EventOpportunity
	}
	var index []DirectorEventCardIndex
	if opportunity.Due && opportunity.Kind == "new" {
		index = directorEventCardIndex(catalog)
	}
	return opportunity, runtime, index, nil
}

func (s *Store) ReadDirectorEventCards(storyID string, eventRefs []string) ([]DirectorEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(eventRefs) > 8 {
		return nil, fmt.Errorf("一次最多读取 8 张事件卡")
	}
	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	catalog := DirectorEventCatalogFromStoryDirector(s.storyDirectorForMeta(meta))
	byRef := make(map[string]DirectorEvent, len(catalog))
	for _, event := range catalog {
		byRef[event.ID] = event
	}
	result := make([]DirectorEvent, 0, len(eventRefs))
	seen := map[string]bool{}
	for _, ref := range eventRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		event, ok := byRef[ref]
		if !ok {
			return nil, fmt.Errorf("事件卡不存在或不属于当前故事导演: %s", ref)
		}
		seen[ref] = true
		result = append(result, event)
	}
	return result, nil
}
