package interactive

import (
	"fmt"
	"math"
	"strings"
)

type RuleStateBinding struct {
	ID                  string                          `json:"id,omitempty"`
	Label               string                          `json:"label,omitempty"`
	Trigger             string                          `json:"trigger,omitempty"`
	ActorTemplateID     string                          `json:"actor_template_id,omitempty"`
	TargetTemplateID    string                          `json:"target_template_id,omitempty"`
	Modifiers           []RuleStateBindingModifier      `json:"modifiers,omitempty"`
	NarrativeStateRefs  []RuleNarrativeStateRef         `json:"narrative_state_refs,omitempty"`
	OutcomeStateChanges []RuleOutcomeStateChangeBinding `json:"outcome_state_changes,omitempty"`
}

type RuleStateBindingModifier struct {
	Source   string   `json:"source,omitempty"`
	FieldID  string   `json:"field_id,omitempty"`
	Effect   string   `json:"effect,omitempty"`
	Scale    float64  `json:"scale,omitempty"`
	Offset   float64  `json:"offset,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Rounding string   `json:"rounding,omitempty"`
	Required *bool    `json:"required,omitempty"`
}

type RuleNarrativeStateRef struct {
	Source   string `json:"source,omitempty"`
	FieldID  string `json:"field_id,omitempty"`
	Usage    string `json:"usage,omitempty"`
	Guidance string `json:"guidance,omitempty"`
}

type RuleOutcomeStateChangeBinding struct {
	Outcome      string                    `json:"outcome,omitempty"`
	StateChanges []RuleComputedStateChange `json:"state_changes,omitempty"`
}

type RuleComputedStateChange struct {
	Source        string                 `json:"source,omitempty"`
	FieldID       string                 `json:"field_id,omitempty"`
	ChangeFormula RuleStateChangeFormula `json:"change_formula,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
}

type RuleStateChangeFormula struct {
	Base     float64                `json:"base,omitempty"`
	Terms    []RuleStateFormulaTerm `json:"terms,omitempty"`
	Min      *float64               `json:"min,omitempty"`
	Max      *float64               `json:"max,omitempty"`
	Rounding string                 `json:"rounding,omitempty"`
}

type RuleStateFormulaTerm struct {
	Source  string  `json:"source,omitempty"`
	FieldID string  `json:"field_id,omitempty"`
	Scale   float64 `json:"scale,omitempty"`
	Offset  float64 `json:"offset,omitempty"`
}

type RuleStateBindingAudit struct {
	binding                     RuleStateBinding
	BindingID                   string                    `json:"binding_id,omitempty"`
	ActorID                     string                    `json:"actor_id,omitempty"`
	TargetActorID               string                    `json:"target_actor_id,omitempty"`
	StateInputs                 []RuleStateBindingInput   `json:"state_inputs,omitempty"`
	BindingBonusTotal           float64                   `json:"binding_bonus_total,omitempty"`
	BindingResistanceTotal      float64                   `json:"binding_resistance_total,omitempty"`
	ManualBonusTotal            float64                   `json:"manual_bonus_total,omitempty"`
	BonusDetails                []TurnCheckBonus          `json:"bonus_details,omitempty"`
	NarrativeStateRefs          []RuleNarrativeStateRef   `json:"narrative_state_refs,omitempty"`
	ConfiguredStateChangeInputs []RuleComputedStateChange `json:"configured_state_change_inputs,omitempty"`
	ComputedStateChanges        []TurnStateChange         `json:"computed_state_changes,omitempty"`
	ManualStateChanges          []TurnStateChange         `json:"manual_state_changes,omitempty"`
	Warnings                    []RuleStateBindingWarning `json:"warnings,omitempty"`
}

type RuleStateBindingInput struct {
	Source        string  `json:"source,omitempty"`
	ActorID       string  `json:"actor_id,omitempty"`
	TemplateID    string  `json:"template_id,omitempty"`
	FieldID       string  `json:"field_id,omitempty"`
	RawValue      float64 `json:"raw_value"`
	ComputedValue float64 `json:"computed_value"`
	Effect        string  `json:"effect,omitempty"`
}

type RuleStateBindingWarning struct {
	ActorID string `json:"actor_id,omitempty"`
	FieldID string `json:"field_id,omitempty"`
	Reason  string `json:"reason"`
}

func normalizeRuleStateBindings(values []RuleStateBinding) []RuleStateBinding {
	if len(values) > maxRuleStateBindings {
		values = values[:maxRuleStateBindings]
	}
	out := make([]RuleStateBinding, 0, len(values))
	for i, value := range values {
		value.ID = normalizeSlotID(value.ID)
		if value.ID == "" {
			value.ID = fmt.Sprintf("binding-%d", i+1)
		}
		value.Label = trimBytes(firstNonEmptyString(value.Label, value.ID), 256)
		value.Trigger = trimBytes(value.Trigger, maxInteractiveTextBytes)
		value.ActorTemplateID = normalizeActorStateID(value.ActorTemplateID)
		value.TargetTemplateID = normalizeActorStateID(value.TargetTemplateID)
		value.Modifiers = normalizeRuleStateBindingModifiers(value.Modifiers)
		value.NarrativeStateRefs = normalizeRuleNarrativeStateRefs(value.NarrativeStateRefs)
		value.OutcomeStateChanges = normalizeRuleOutcomeStateChangeBindings(value.OutcomeStateChanges)
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolveRuleStateFieldIDs(system StoryDirectorActorStateSystem, trpg StoryDirectorTRPGSystem) StoryDirectorTRPGSystem {
	system = normalizeActorStateSystem(system)
	trpg.RuleTemplates = normalizeRuleChecks(trpg.RuleTemplates)
	for checkIndex := range trpg.RuleTemplates {
		for bindingIndex := range trpg.RuleTemplates[checkIndex].StateBindings {
			binding := &trpg.RuleTemplates[checkIndex].StateBindings[bindingIndex]
			for index := range binding.Modifiers {
				templateID := binding.ActorTemplateID
				if binding.Modifiers[index].Source == "target" {
					templateID = binding.TargetTemplateID
				}
				binding.Modifiers[index].FieldID = resolvedRuleStateFieldID(system, templateID, binding.Modifiers[index].FieldID)
			}
			for index := range binding.NarrativeStateRefs {
				templateID := binding.ActorTemplateID
				if binding.NarrativeStateRefs[index].Source == "target" {
					templateID = binding.TargetTemplateID
				}
				if binding.NarrativeStateRefs[index].Source != "scene" {
					binding.NarrativeStateRefs[index].FieldID = resolvedRuleStateFieldID(system, templateID, binding.NarrativeStateRefs[index].FieldID)
				}
			}
			for groupIndex := range binding.OutcomeStateChanges {
				for changeIndex := range binding.OutcomeStateChanges[groupIndex].StateChanges {
					change := &binding.OutcomeStateChanges[groupIndex].StateChanges[changeIndex]
					templateID := binding.ActorTemplateID
					if change.Source == "target" {
						templateID = binding.TargetTemplateID
					}
					change.FieldID = resolvedRuleStateFieldID(system, templateID, change.FieldID)
					for termIndex := range change.ChangeFormula.Terms {
						term := &change.ChangeFormula.Terms[termIndex]
						termTemplateID := binding.ActorTemplateID
						if term.Source == "target" {
							termTemplateID = binding.TargetTemplateID
						}
						term.FieldID = resolvedRuleStateFieldID(system, termTemplateID, term.FieldID)
					}
				}
			}
		}
	}
	return trpg
}

func resolvedRuleStateFieldID(system StoryDirectorActorStateSystem, templateID, reference string) string {
	reference = normalizeActorStateFieldName(reference)
	if field, ok := actorStateFieldByID(actorStateTemplateByID(system, templateID), reference); ok {
		return actorStateFieldID(field)
	}
	return reference
}

func normalizeRuleStateBindingModifiers(values []RuleStateBindingModifier) []RuleStateBindingModifier {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]RuleStateBindingModifier, 0, len(values))
	for _, value := range values {
		value.Source = normalizeRuleBindingSource(value.Source)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		value.Effect = normalizeRuleBindingEffect(value.Effect)
		if value.Scale == 0 {
			value.Scale = 1
		}
		value.Rounding = normalizeRuleBindingRounding(value.Rounding)
		normalizeMinMaxPointers(&value.Min, &value.Max)
		if value.Source == "" || value.FieldID == "" || value.Effect == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRuleNarrativeStateRefs(values []RuleNarrativeStateRef) []RuleNarrativeStateRef {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]RuleNarrativeStateRef, 0, len(values))
	for _, value := range values {
		value.Source = normalizeRuleNarrativeSource(value.Source)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		value.Usage = normalizeRuleNarrativeUsage(value.Usage)
		value.Guidance = trimBytes(value.Guidance, maxInteractiveTextBytes)
		if value.Source == "" || (value.Source != "scene" && value.FieldID == "") {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRuleOutcomeStateChangeBindings(values []RuleOutcomeStateChangeBinding) []RuleOutcomeStateChangeBinding {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]RuleOutcomeStateChangeBinding, 0, len(values))
	for _, value := range values {
		value.Outcome = normalizeRuleOutcomeName(value.Outcome)
		value.StateChanges = normalizeRuleComputedStateChanges(value.StateChanges)
		if value.Outcome == "" || len(value.StateChanges) == 0 {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRuleComputedStateChanges(values []RuleComputedStateChange) []RuleComputedStateChange {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]RuleComputedStateChange, 0, len(values))
	for _, value := range values {
		value.Source = normalizeRuleBindingSource(value.Source)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		value.ChangeFormula = normalizeRuleStateChangeFormula(value.ChangeFormula)
		value.Reason = trimBytes(value.Reason, 512)
		if value.Source == "" || value.FieldID == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRuleStateChangeFormula(value RuleStateChangeFormula) RuleStateChangeFormula {
	if len(value.Terms) > maxInteractiveListItems {
		value.Terms = value.Terms[:maxInteractiveListItems]
	}
	terms := make([]RuleStateFormulaTerm, 0, len(value.Terms))
	for _, term := range value.Terms {
		term.Source = normalizeRuleBindingSource(term.Source)
		term.FieldID = normalizeActorStateFieldName(term.FieldID)
		if term.Scale == 0 {
			term.Scale = 1
		}
		if term.Source == "" || term.FieldID == "" {
			continue
		}
		terms = append(terms, term)
	}
	value.Terms = terms
	value.Rounding = normalizeRuleBindingRounding(value.Rounding)
	normalizeMinMaxPointers(&value.Min, &value.Max)
	return value
}

func validateRuleStateBindings(values []RuleStateBinding) error {
	seen := map[string]bool{}
	for _, binding := range values {
		if binding.ID == "" {
			return fmt.Errorf("state binding 缺少 id")
		}
		if seen[binding.ID] {
			return fmt.Errorf("state binding id 重复: %s", binding.ID)
		}
		seen[binding.ID] = true
		if binding.ActorTemplateID == "" {
			return fmt.Errorf("state binding %s 缺少 actor_template_id", binding.ID)
		}
		for _, modifier := range binding.Modifiers {
			if modifier.Source != "actor" && modifier.Source != "target" {
				return fmt.Errorf("state binding %s modifier source 无效: %s", binding.ID, modifier.Source)
			}
			if modifier.Effect != "advantage" && modifier.Effect != "resistance" {
				return fmt.Errorf("state binding %s modifier effect 无效: %s", binding.ID, modifier.Effect)
			}
			if modifier.Source == "target" && binding.TargetTemplateID == "" {
				return fmt.Errorf("state binding %s modifier 使用 target 但缺少 target_template_id", binding.ID)
			}
		}
		for _, group := range binding.OutcomeStateChanges {
			if group.Outcome == "" {
				return fmt.Errorf("state binding %s outcome_state_changes 缺少 outcome", binding.ID)
			}
			for _, change := range group.StateChanges {
				if change.Source == "target" && binding.TargetTemplateID == "" {
					return fmt.Errorf("state binding %s outcome_state_changes 使用 target 但缺少 target_template_id", binding.ID)
				}
			}
		}
	}
	return nil
}

func normalizeRuleStateBindingAuditPointer(value *RuleStateBindingAudit) *RuleStateBindingAudit {
	if value == nil {
		return nil
	}
	next := *value
	next.BindingID = normalizeSlotID(next.BindingID)
	next.ActorID = normalizeActorStateID(next.ActorID)
	next.TargetActorID = normalizeActorStateID(next.TargetActorID)
	next.NarrativeStateRefs = normalizeRuleNarrativeStateRefs(next.NarrativeStateRefs)
	next.ComputedStateChanges = normalizeTurnStateChanges(next.ComputedStateChanges)
	next.ManualStateChanges = normalizeTurnStateChanges(next.ManualStateChanges)
	next.ConfiguredStateChangeInputs = normalizeRuleComputedStateChanges(next.ConfiguredStateChangeInputs)
	next.BonusDetails = normalizeTurnCheckBonuses(next.BonusDetails)
	next.StateInputs = normalizeRuleStateBindingInputs(next.StateInputs)
	next.Warnings = normalizeRuleStateBindingWarnings(next.Warnings)
	return &next
}

func normalizeRuleStateBindingInputs(values []RuleStateBindingInput) []RuleStateBindingInput {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]RuleStateBindingInput, 0, len(values))
	for _, value := range values {
		value.Source = normalizeRuleBindingSource(value.Source)
		value.ActorID = normalizeActorStateID(value.ActorID)
		value.TemplateID = normalizeActorStateID(value.TemplateID)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		value.Effect = normalizeRuleBindingEffect(value.Effect)
		if value.Source == "" && value.FieldID == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRuleStateBindingWarnings(values []RuleStateBindingWarning) []RuleStateBindingWarning {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]RuleStateBindingWarning, 0, len(values))
	for _, value := range values {
		value.ActorID = normalizeActorStateID(value.ActorID)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		value.Reason = trimBytes(value.Reason, 512)
		if value.ActorID == "" && value.FieldID == "" && value.Reason == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ruleCheckHasStateBindings(check RuleCheck) bool {
	return len(normalizeRuleStateBindings(check.StateBindings)) > 0
}

func ruleChecksHaveStateBindings(checks []RuleCheck) bool {
	for _, check := range checks {
		if ruleCheckHasStateBindings(check) {
			return true
		}
	}
	return false
}

func resolveRuleStateBinding(state map[string]any, director StoryDirector, req TurnCheckRequest) (*RuleStateBindingAudit, error) {
	bindingID := normalizeSlotID(req.Rule.BindingID)
	if bindingID == "" {
		return nil, nil
	}
	director = normalizeStoryDirector(director)
	binding, ok := findRuleStateBinding(director.TRPGSystem.RuleTemplates, req.Rule.TemplateID, bindingID)
	if !ok {
		return nil, fmt.Errorf("prepare_interactive_turn rule.binding_id 不存在: %s", bindingID)
	}
	if actorStateEmpty(director.ActorState) {
		return nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s 需要状态系统", bindingID)
	}
	actorID := normalizeActorStateID(req.Rule.ActorID)
	if actorID == "" {
		return nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s 缺少 actor_id", bindingID)
	}
	actorTemplateID, err := validateBindingActor(state, director.ActorState, actorID, binding.ActorTemplateID, "actor")
	if err != nil {
		return nil, err
	}
	targetActorID := normalizeActorStateID(req.Rule.TargetActorID)
	if binding.TargetTemplateID != "" {
		if targetActorID == "" {
			return nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s 需要 target_actor_id", bindingID)
		}
		if _, err := validateBindingActor(state, director.ActorState, targetActorID, binding.TargetTemplateID, "target"); err != nil {
			return nil, err
		}
	} else {
		targetActorID = ""
	}
	audit := &RuleStateBindingAudit{
		binding:            binding,
		BindingID:          binding.ID,
		ActorID:            actorID,
		TargetActorID:      targetActorID,
		NarrativeStateRefs: append([]RuleNarrativeStateRef(nil), binding.NarrativeStateRefs...),
	}
	for _, modifier := range binding.Modifiers {
		value, fieldTemplateID, _, ok, err := readBindingNumber(state, director.ActorState, actorID, targetActorID, modifier.Source, modifier.FieldID)
		if err != nil {
			if ruleBindingRequired(modifier.Required) {
				return nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s modifier %s.%s: %w", bindingID, modifier.Source, modifier.FieldID, err)
			}
			audit.Warnings = append(audit.Warnings, RuleStateBindingWarning{Reason: err.Error()})
			continue
		}
		if !ok {
			if ruleBindingRequired(modifier.Required) {
				return nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s modifier %s.%s 缺少 number 值", bindingID, modifier.Source, modifier.FieldID)
			}
			warningActorID := actorID
			if modifier.Source == "target" {
				warningActorID = targetActorID
			}
			audit.Warnings = append(audit.Warnings, RuleStateBindingWarning{ActorID: warningActorID, FieldID: modifier.FieldID, Reason: "可选 modifier 缺少 number 值"})
			continue
		}
		computed := applyRuleBindingNumber(value*modifier.Scale+modifier.Offset, modifier.Rounding, modifier.Min, modifier.Max)
		inputActorID := actorID
		if modifier.Source == "target" {
			inputActorID = targetActorID
		}
		input := RuleStateBindingInput{
			Source:        modifier.Source,
			ActorID:       inputActorID,
			TemplateID:    firstNonEmptyString(fieldTemplateID, actorTemplateID),
			FieldID:       modifier.FieldID,
			RawValue:      value,
			ComputedValue: computed,
			Effect:        modifier.Effect,
		}
		audit.StateInputs = append(audit.StateInputs, input)
		switch modifier.Effect {
		case "advantage":
			audit.BindingBonusTotal += computed
			audit.BonusDetails = append(audit.BonusDetails, TurnCheckBonus{
				Kind:    "state_binding",
				ActorID: inputActorID,
				FieldID: modifier.FieldID,
				Reason:  fmt.Sprintf("%s: %s", firstNonEmptyString(binding.Label, binding.ID), modifier.FieldID),
				Value:   computed,
			})
		case "resistance":
			audit.BindingResistanceTotal += computed
		}
	}
	return audit, nil
}

func findRuleStateBinding(checks []RuleCheck, templateID, bindingID string) (RuleStateBinding, bool) {
	templateID = strings.TrimSpace(templateID)
	bindingID = normalizeSlotID(bindingID)
	normalized := normalizeRuleChecks(checks)
	for _, check := range normalized {
		if templateID != "" && check.ID != templateID {
			continue
		}
		for _, binding := range check.StateBindings {
			if binding.ID == bindingID {
				return binding, true
			}
		}
	}
	if templateID != "" {
		return RuleStateBinding{}, false
	}
	for _, check := range normalized {
		for _, binding := range check.StateBindings {
			if binding.ID == bindingID {
				return binding, true
			}
		}
	}
	return RuleStateBinding{}, false
}

func computeBindingOutcomeStateChanges(state map[string]any, system StoryDirectorActorStateSystem, audit *RuleStateBindingAudit, outcome string) ([]TurnStateChange, []RuleStateBindingWarning, error) {
	if audit == nil {
		return nil, nil, nil
	}
	bindingID := audit.BindingID
	directorBinding := audit.binding
	if directorBinding.ID == "" {
		return nil, nil, nil
	}
	outcome = normalizeRuleOutcomeName(outcome)
	var configured []RuleComputedStateChange
	for _, group := range directorBinding.OutcomeStateChanges {
		if group.Outcome == outcome {
			configured = append(configured, group.StateChanges...)
		}
	}
	if len(configured) == 0 {
		return nil, nil, nil
	}
	audit.ConfiguredStateChangeInputs = append([]RuleComputedStateChange(nil), configured...)
	changes := make([]TurnStateChange, 0, len(configured))
	warnings := make([]RuleStateBindingWarning, 0)
	for _, item := range configured {
		targetActorID := audit.ActorID
		if item.Source == "target" {
			targetActorID = audit.TargetActorID
		}
		if targetActorID == "" {
			return nil, nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s outcome_state_changes 缺少 target_actor_id", bindingID)
		}
		field, templateID, err := bindingNumberField(system, state, targetActorID, item.FieldID)
		if err != nil {
			return nil, nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s outcome_state_changes %s.%s: %w", bindingID, item.Source, item.FieldID, err)
		}
		if field.Type != "number" {
			return nil, nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s outcome_state_changes 字段不是 number: %s", bindingID, item.FieldID)
		}
		change, err := computeBindingFormula(state, system, audit.ActorID, audit.TargetActorID, item.ChangeFormula)
		if err != nil {
			return nil, nil, fmt.Errorf("prepare_interactive_turn rule.binding_id=%s outcome_state_changes %s.%s: %w", bindingID, item.Source, item.FieldID, err)
		}
		reason := firstNonEmptyString(item.Reason, fmt.Sprintf("state binding %s %s", bindingID, outcome))
		_ = templateID
		changes = append(changes, TurnStateChange{ActorID: targetActorID, FieldID: actorStateFieldID(field), Change: change, Reason: reason})
	}
	return normalizeTurnStateChanges(changes), warnings, nil
}

func computeBindingFormula(state map[string]any, system StoryDirectorActorStateSystem, actorID, targetActorID string, formula RuleStateChangeFormula) (float64, error) {
	total := formula.Base
	for _, term := range formula.Terms {
		value, _, _, ok, err := readBindingNumber(state, system, actorID, targetActorID, term.Source, term.FieldID)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, fmt.Errorf("字段缺少 number 值: %s.%s", term.Source, term.FieldID)
		}
		total += value*term.Scale + term.Offset
	}
	return applyRuleBindingNumber(total, formula.Rounding, formula.Min, formula.Max), nil
}

func validateBindingActor(state map[string]any, system StoryDirectorActorStateSystem, actorID, wantTemplateID, role string) (string, error) {
	templateID, ok := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !ok {
		return "", fmt.Errorf("prepare_interactive_turn rule.%s_id 不存在或缺少状态模板: %s", role, actorID)
	}
	templateID = normalizeActorStateID(templateID)
	if wantTemplateID != "" && templateID != wantTemplateID {
		return "", fmt.Errorf("prepare_interactive_turn rule.%s_id 模板不匹配: actor=%s got=%s want=%s", role, actorID, templateID, wantTemplateID)
	}
	if actorStateTemplateByID(system, templateID).ID == "" {
		return "", fmt.Errorf("prepare_interactive_turn rule.%s_id 状态模板不存在: %s", role, templateID)
	}
	return templateID, nil
}

func readBindingNumber(state map[string]any, system StoryDirectorActorStateSystem, actorID, targetActorID, source, fieldPath string) (float64, string, string, bool, error) {
	readActorID := actorID
	if source == "target" {
		readActorID = targetActorID
	}
	if readActorID == "" {
		return 0, "", "", false, fmt.Errorf("缺少 %s actor", source)
	}
	field, templateID, err := bindingNumberField(system, state, readActorID, fieldPath)
	if err != nil {
		return 0, "", "", false, err
	}
	if field.Type != "number" {
		return 0, templateID, actorStateFieldPath(readActorID, fieldPath), false, fmt.Errorf("字段不是 number 类型: %s", fieldPath)
	}
	path := actorStateFieldPath(readActorID, actorStateFieldID(field))
	value, ok := actorStateNumber(actorStateFieldValue(state, readActorID, actorStateFieldID(field)))
	if !ok && strings.TrimSpace(field.LegacyPath) != "" {
		value, ok = actorStateNumber(getPath(state, actorStateFieldPath(readActorID, field.LegacyPath)))
	}
	if ok {
		return value, templateID, path, true, nil
	}
	if defaultValue, defaultOK := actorStateNumber(field.Default); defaultOK {
		return defaultValue, templateID, path, true, nil
	}
	return 0, templateID, path, false, nil
}

func bindingNumberField(system StoryDirectorActorStateSystem, state map[string]any, actorID, fieldPath string) (ActorStateField, string, error) {
	templateID, ok := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !ok {
		return ActorStateField{}, "", fmt.Errorf("Actor 不存在或缺少状态模板: %s", actorID)
	}
	template := actorStateTemplateByID(system, templateID)
	if template.ID == "" {
		return ActorStateField{}, templateID, fmt.Errorf("Actor 状态模板不存在: %s", templateID)
	}
	field, ok := actorStateFieldByPath(template, fieldPath)
	if !ok {
		return ActorStateField{}, templateID, fmt.Errorf("字段不在状态系统中: %s", fieldPath)
	}
	return field, templateID, nil
}

func mergeBindingStateChanges(configured, manual []TurnStateChange) []TurnStateChange {
	out := make([]TurnStateChange, 0, len(configured)+len(manual))
	out = append(out, configured...)
	out = append(out, normalizeTurnStateChanges(manual)...)
	return normalizeTurnStateChanges(out)
}

func duplicateStateChangeWarnings(configured, manual []TurnStateChange) []RuleStateBindingWarning {
	if len(configured) == 0 || len(manual) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, change := range normalizeTurnStateChanges(configured) {
		seen[change.ActorID+"\x00"+actorStateFieldNameKey(change.FieldID)] = true
	}
	warnings := []RuleStateBindingWarning{}
	for _, change := range normalizeTurnStateChanges(manual) {
		key := change.ActorID + "\x00" + actorStateFieldNameKey(change.FieldID)
		if seen[key] {
			warnings = append(warnings, RuleStateBindingWarning{ActorID: change.ActorID, FieldID: change.FieldID, Reason: "binding 与 DM 临场 state_changes 修改同一状态字段，将按顺序执行"})
		}
	}
	return warnings
}

func applyRuleBindingNumber(value float64, rounding string, min, max *float64) float64 {
	switch normalizeRuleBindingRounding(rounding) {
	case "floor":
		value = math.Floor(value)
	case "ceil":
		value = math.Ceil(value)
	case "nearest":
		value = math.Round(value)
	}
	if min != nil && value < *min {
		value = *min
	}
	if max != nil && value > *max {
		value = *max
	}
	return value
}

func ruleBindingRequired(required *bool) bool {
	return required == nil || *required
}

func normalizeRuleBindingSource(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "actor", "target":
		return normalizeTurnCheckEnumToken(value)
	default:
		return ""
	}
}

func normalizeRuleNarrativeSource(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "actor", "target", "scene":
		return normalizeTurnCheckEnumToken(value)
	default:
		return ""
	}
}

func normalizeRuleBindingEffect(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "advantage", "resistance":
		return normalizeTurnCheckEnumToken(value)
	default:
		return ""
	}
}

func normalizeRuleBindingRounding(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "none", "floor", "ceil", "nearest":
		return normalizeTurnCheckEnumToken(value)
	default:
		return "nearest"
	}
}

func normalizeRuleNarrativeUsage(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "check_decision", "difficulty", "outcome_design", "prose":
		return normalizeTurnCheckEnumToken(value)
	default:
		return "outcome_design"
	}
}

func normalizeRuleOutcomeName(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "critical_success", "success", "failure", "critical_failure":
		return normalizeTurnCheckEnumToken(value)
	default:
		return ""
	}
}

func normalizeMinMaxPointers(min, max **float64) {
	if min == nil || max == nil || *min == nil || *max == nil {
		return
	}
	if **min > **max {
		*min, *max = *max, *min
	}
}
