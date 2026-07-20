package interactive

import (
	"fmt"
	"strings"
)

const (
	DefaultRuleStateConsumptionMode      = RuleStateConsumptionModeHybridAuto
	RuleStateConsumptionModeHybridAuto   = "hybrid_auto"
	RuleStateConsumptionModeDirectorOnly = "director_only"

	StateOpSourceRuleResolution = "rule_resolution"
)

type RuleStateConsumption struct {
	Status          string                        `json:"status"`
	Mode            string                        `json:"mode,omitempty"`
	AppliedOps      []StateOp                     `json:"applied_ops,omitempty"`
	AppliedActorOps []ActorStateOp                `json:"applied_actor_ops,omitempty"`
	Warnings        []RuleStateConsumptionWarning `json:"warnings,omitempty"`
}

type RuleStateConsumptionWarning struct {
	ActorID string `json:"actor_id,omitempty"`
	FieldID string `json:"field_id,omitempty"`
	Reason  string `json:"reason"`
}

func applyRuleStateConsumptionV2(state map[string]any, system StoryDirectorActorStateSystem, turnID string, resolution *RuleResolution, mode string) ([]StateOp, []ActorStateOp) {
	if resolution == nil {
		return nil, nil
	}
	mode = normalizeRuleStateConsumptionMode(mode)
	changes := normalizeTurnStateChanges(resolution.Result.StateChanges)
	if len(changes) == 0 {
		resolution.StateConsumption = &RuleStateConsumption{Status: "none", Mode: mode}
		return nil, nil
	}
	if mode == RuleStateConsumptionModeDirectorOnly {
		resolution.StateConsumption = &RuleStateConsumption{
			Status: "disabled",
			Mode:   mode,
			Warnings: []RuleStateConsumptionWarning{{
				Reason: "规则状态自动消费已关闭；该检定结果将由后台导演按叙事上下文处理。",
			}},
		}
		return nil, nil
	}
	system = normalizeActorStateSystem(system)
	actorOps := make([]ActorStateOp, 0, len(changes))
	warnings := make([]RuleStateConsumptionWarning, 0)
	for _, change := range changes {
		op, ok, warning := ruleStateChangeToActorOp(state, system, turnID, *resolution, change)
		if !ok {
			warnings = append(warnings, warning)
			continue
		}
		actorOps = append(actorOps, op)
		applyActorStateOp(state, op)
	}
	status := "applied"
	switch {
	case len(actorOps) == 0:
		status = "skipped"
	case len(warnings) > 0:
		status = "partial"
	}
	resolution.StateConsumption = normalizeRuleStateConsumptionPointer(&RuleStateConsumption{
		Status:          status,
		Mode:            mode,
		AppliedActorOps: actorOps,
		Warnings:        warnings,
	})
	return nil, actorOps
}

func ruleStateChangeToActorOp(state map[string]any, system StoryDirectorActorStateSystem, turnID string, resolution RuleResolution, change TurnStateChange) (ActorStateOp, bool, RuleStateConsumptionWarning) {
	normalized := normalizeTurnStateChanges([]TurnStateChange{change})
	if len(normalized) == 0 || normalized[0].ActorID == "" || normalized[0].FieldID == "" {
		return ActorStateOp{}, false, RuleStateConsumptionWarning{Reason: "状态引用必须提供 actor_id 和 field_id"}
	}
	change = normalized[0]
	actorID := change.ActorID
	fieldRef := change.FieldID
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !found {
		return ActorStateOp{}, false, RuleStateConsumptionWarning{ActorID: actorID, FieldID: fieldRef, Reason: "目标 Actor 不存在或缺少状态模板"}
	}
	field, found := actorStateFieldByID(actorStateTemplateByID(system, templateID), fieldRef)
	if !found {
		return ActorStateOp{}, false, RuleStateConsumptionWarning{ActorID: actorID, FieldID: fieldRef, Reason: fmt.Sprintf("字段不在状态系统中: %s", fieldRef)}
	}
	if field.Type != "number" {
		return ActorStateOp{}, false, RuleStateConsumptionWarning{ActorID: actorID, FieldID: fieldRef, Reason: fmt.Sprintf("字段不是 number 类型: %s", fieldRef)}
	}
	current, ok := actorStateNumber(actorStateFieldValue(state, actorID, actorStateFieldID(field)))
	if !ok {
		if defaultValue, defaultOK := actorStateNumber(field.Default); defaultOK {
			current = defaultValue
		}
	}
	next := current + change.Change
	if field.Min != nil && next < *field.Min {
		next = *field.Min
	}
	if field.Max != nil && next > *field.Max {
		next = *field.Max
	}
	reason := firstNonEmptyString(change.Reason, resolution.Result.Result, resolution.Request.Cost, resolution.Request.Challenge)
	return ActorStateOp{Op: "set", ActorID: actorID, FieldID: actorStateFieldID(field), Value: next, Reason: reason, SourceTurnID: turnID, SourceKind: StateOpSourceRuleResolution, SourceID: resolution.ID}, true, RuleStateConsumptionWarning{}
}

func actorTemplateIDFromStateOrSystem(state map[string]any, system StoryDirectorActorStateSystem, actorID string) (string, bool) {
	if raw := getPath(state, actorStateActorPath(actorID, "template_id")); raw != nil {
		if value := normalizeActorStateID(fmt.Sprint(raw)); value != "" {
			return value, true
		}
	}
	for _, actor := range normalizeActorStateSystem(system).InitialActors {
		if actor.ID == actorID && strings.TrimSpace(actor.TemplateID) != "" {
			return normalizeActorStateID(actor.TemplateID), true
		}
	}
	if actorID == DefaultActorID {
		return "protagonist", true
	}
	return "", false
}

func actorStateFieldByPath(template ActorStateTemplate, path string) (ActorStateField, bool) {
	return actorStateFieldByID(template, path)
}

func removeRuleResolutionStateOps(ops []StateOp, resolutionID string) []StateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]StateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceRuleResolution && (resolutionID == "" || op.SourceID == resolutionID) {
			continue
		}
		out = append(out, op)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func removeRuleResolutionActorOps(ops []ActorStateOp, resolutionID string) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceRuleResolution && (resolutionID == "" || op.SourceID == resolutionID) {
			continue
		}
		out = append(out, op)
	}
	return normalizeActorStateOps(out)
}

func normalizeRuleStateConsumptionMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", RuleStateConsumptionModeHybridAuto:
		return RuleStateConsumptionModeHybridAuto
	case RuleStateConsumptionModeDirectorOnly:
		return RuleStateConsumptionModeDirectorOnly
	default:
		return RuleStateConsumptionModeHybridAuto
	}
}

func normalizeRuleStateConsumptionPointer(value *RuleStateConsumption) *RuleStateConsumption {
	if value == nil {
		return nil
	}
	next := *value
	next.Status = normalizeRuleStateConsumptionStatus(next.Status)
	next.Mode = normalizeRuleStateConsumptionMode(next.Mode)
	next.AppliedOps = normalizeStateOps(next.AppliedOps)
	next.AppliedActorOps = normalizeActorStateOps(next.AppliedActorOps)
	if len(next.Warnings) > maxInteractiveListItems {
		next.Warnings = next.Warnings[:maxInteractiveListItems]
	}
	warnings := make([]RuleStateConsumptionWarning, 0, len(next.Warnings))
	for _, warning := range next.Warnings {
		warning.ActorID = normalizeStatePanelActorID(warning.ActorID)
		warning.FieldID = normalizeActorStateFieldName(warning.FieldID)
		warning.Reason = trimBytes(warning.Reason, 1024)
		if warning.ActorID == "" && warning.FieldID == "" && warning.Reason == "" {
			continue
		}
		warnings = append(warnings, warning)
	}
	next.Warnings = warnings
	return &next
}

func normalizeRuleStateConsumptionStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "none", "disabled", "applied", "partial", "skipped":
		return strings.TrimSpace(status)
	default:
		return "none"
	}
}
