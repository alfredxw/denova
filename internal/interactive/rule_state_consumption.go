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
	Status     string                        `json:"status"`
	Mode       string                        `json:"mode,omitempty"`
	AppliedOps []StateOp                     `json:"applied_ops,omitempty"`
	Warnings   []RuleStateConsumptionWarning `json:"warnings,omitempty"`
}

type RuleStateConsumptionWarning struct {
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason"`
}

func applyRuleStateConsumption(state map[string]any, system StoryDirectorActorStateSystem, turnID string, resolution *RuleResolution, mode string) []StateOp {
	if resolution == nil {
		return nil
	}
	mode = normalizeRuleStateConsumptionMode(mode)
	changes := normalizeTurnStateChanges(resolution.Result.StateChanges)
	if len(changes) == 0 {
		resolution.StateConsumption = &RuleStateConsumption{Status: "none", Mode: mode}
		return nil
	}
	if mode == RuleStateConsumptionModeDirectorOnly {
		resolution.StateConsumption = &RuleStateConsumption{
			Status: "disabled",
			Mode:   mode,
			Warnings: []RuleStateConsumptionWarning{{
				Reason: "规则状态自动消费已关闭；该检定结果将由后台导演按叙事上下文处理。",
			}},
		}
		return nil
	}
	system = normalizeActorStateSystem(system)
	ops := make([]StateOp, 0, len(changes))
	warnings := make([]RuleStateConsumptionWarning, 0)
	for _, change := range changes {
		op, ok, warning := ruleStateChangeToOp(state, system, turnID, *resolution, change)
		if !ok {
			warnings = append(warnings, warning)
			continue
		}
		ops = append(ops, op)
		applyStateOp(state, op)
	}
	status := "applied"
	switch {
	case len(ops) == 0:
		status = "skipped"
	case len(warnings) > 0:
		status = "partial"
	}
	resolution.StateConsumption = normalizeRuleStateConsumptionPointer(&RuleStateConsumption{
		Status:     status,
		Mode:       mode,
		AppliedOps: ops,
		Warnings:   warnings,
	})
	return ops
}

func ruleStateChangeToOp(state map[string]any, system StoryDirectorActorStateSystem, turnID string, resolution RuleResolution, change TurnStateChange) (StateOp, bool, RuleStateConsumptionWarning) {
	path := canonicalStatePath(change.Path)
	if !validStatePathSyntax(path) {
		return StateOp{}, false, RuleStateConsumptionWarning{Path: strings.TrimSpace(change.Path), Reason: "状态路径无效"}
	}
	actorID, fieldPath, ok := parseActorStateFieldPath(path)
	if !ok {
		return StateOp{}, false, RuleStateConsumptionWarning{Path: path, Reason: "状态路径不属于 Actor State 字段"}
	}
	templateID, ok := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !ok {
		return StateOp{}, false, RuleStateConsumptionWarning{Path: path, Reason: "目标 Actor 不存在或缺少状态模板"}
	}
	template := actorStateTemplateByID(system, templateID)
	if template.ID == "" {
		return StateOp{}, false, RuleStateConsumptionWarning{Path: path, Reason: fmt.Sprintf("Actor 状态模板不存在: %s", templateID)}
	}
	field, ok := actorStateFieldByPath(template, fieldPath)
	if !ok {
		return StateOp{}, false, RuleStateConsumptionWarning{Path: path, Reason: fmt.Sprintf("字段不在状态系统中: %s", fieldPath)}
	}
	if field.Type != "number" {
		return StateOp{}, false, RuleStateConsumptionWarning{Path: path, Reason: fmt.Sprintf("字段不是 number 类型: %s", fieldPath)}
	}
	current, ok := actorStateNumber(getPath(state, path))
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
	return StateOp{
		Op:           "set",
		Path:         path,
		Value:        next,
		Reason:       reason,
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceRuleResolution,
		SourceID:     resolution.ID,
	}, true, RuleStateConsumptionWarning{}
}

func parseActorStateFieldPath(path string) (string, string, bool) {
	path = canonicalStatePath(path)
	parts := strings.Split(path, ".")
	if len(parts) < 4 || parts[0] != actorStateRoot || parts[2] != "state" {
		return "", "", false
	}
	actorID := normalizeActorStateID(parts[1])
	fieldPath := strings.Join(parts[3:], ".")
	return actorID, fieldPath, actorID != "" && fieldPath != ""
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
	for _, field := range normalizeActorStateFields(template.Fields) {
		if field.Path == path {
			return field, true
		}
	}
	return ActorStateField{}, false
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
	if len(next.Warnings) > maxTurnBriefListItems {
		next.Warnings = next.Warnings[:maxTurnBriefListItems]
	}
	warnings := make([]RuleStateConsumptionWarning, 0, len(next.Warnings))
	for _, warning := range next.Warnings {
		warning.Path = trimBytes(warning.Path, 512)
		warning.Reason = trimBytes(warning.Reason, 1024)
		if warning.Path == "" && warning.Reason == "" {
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
