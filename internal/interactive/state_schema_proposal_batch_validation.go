package interactive

import (
	"fmt"
	"strings"
)

func validateActorStateSchemaBatchItemInput(item ActorStateSchemaBatchItem, path string, audit ActorStateSchemaBatchAudit) (*ActorStateSchemaBatchIssue, bool) {
	if len(item.Requirements) == 0 {
		issue := actorStateSchemaBatchIssue(item.ItemID, "missing_requirements", path+".requirements", "每个 item 必须包含至少一项来源化需求审查")
		return &issue, false
	}
	reviewedLore := map[string]bool{}
	for _, id := range audit.ReviewedLoreIDs {
		reviewedLore[strings.TrimSpace(id)] = true
	}
	for index, requirement := range item.Requirements {
		requirementPath := fmt.Sprintf("%s.requirements[%d]", path, index)
		switch strings.TrimSpace(requirement.EvidenceKind) {
		case "confirmed", "inferred", "default":
		case "":
			issue := actorStateSchemaBatchIssue(item.ItemID, "missing_evidence_kind", requirementPath+".evidence_kind", "evidence_kind 必须是 confirmed、inferred 或 default")
			return &issue, false
		default:
			issue := actorStateSchemaBatchIssue(item.ItemID, "invalid_evidence_kind", requirementPath+".evidence_kind", "evidence_kind 必须是 confirmed、inferred 或 default")
			return &issue, false
		}
		if issue := validateActorStateSchemaBatchValuePolicy(item, requirement, requirementPath, audit); issue != nil {
			return issue, false
		}
		sourceKind := strings.TrimSpace(requirement.Source.Kind)
		sourceID := strings.TrimSpace(requirement.Source.ID)
		switch sourceKind {
		case "lore":
			if !reviewedLore[sourceID] {
				issue := actorStateSchemaBatchIssue(item.ItemID, "lore_not_reviewed", requirementPath+".source.id", "状态需求引用的资料尚未由后端确认已读取")
				return &issue, true
			}
		case "opening":
			if !actorStateSchemaBatchSourceAllowed(audit.OpeningSourceIDs, sourceID) {
				issue := actorStateSchemaBatchIssue(item.ItemID, "opening_source_not_available", requirementPath+".source.id", "opening 来源 ID 不在后端提供的本次上下文中")
				return &issue, false
			}
		case "turn_result":
			if !actorStateSchemaBatchSourceAllowed(audit.TurnResultSourceIDs, sourceID) {
				issue := actorStateSchemaBatchIssue(item.ItemID, "turn_result_source_not_available", requirementPath+".source.id", "turn_result 来源 ID 不在后端提供的本次上下文中")
				return &issue, false
			}
		case "trpg":
			if !actorStateSchemaBatchSourceAllowed(audit.TRPGSourceIDs, sourceID) {
				issue := actorStateSchemaBatchIssue(item.ItemID, "trpg_source_not_available", requirementPath+".source.id", "trpg 来源 ID 不在后端提供的规则绑定中")
				return &issue, false
			}
		}
	}
	if actorStateSchemaBatchHasWholeActorOps(item.Adaptation) {
		if _, ok := actorStateSchemaBatchItemValueSource(item); !ok {
			issue := actorStateSchemaBatchIssue(item.ItemID, "ambiguous_actor_value_source", path+".requirements", "包含整体 Actor 操作的 item 必须只有一个一致的 source/evidence_kind；请拆成多个 item，或改用字段级 actor_ops set")
			return &issue, false
		}
	}
	for index, op := range item.Adaptation.ActorOps {
		if op.Op != "set" {
			continue
		}
		if _, ok := actorStateSchemaBatchActorFieldValueSource(item, op.ActorID, op.FieldID); !ok {
			issue := actorStateSchemaBatchIssue(item.ItemID, "ambiguous_actor_value_source", fmt.Sprintf("%s.adaptation.actor_ops[%d]", path, index), "字段级 set 必须能唯一对应同一 item 中 actor_id、field_id 和 value_policy=initialize 的 requirement")
			return &issue, false
		}
	}
	if issue := validateActorStateSchemaBatchTemplateOpSources(item, path); issue != nil {
		return issue, false
	}
	return nil, false
}

func validateActorStateSchemaBatchTemplateOpSources(item ActorStateSchemaBatchItem, basePath string) *ActorStateSchemaBatchIssue {
	findReview := func(decision, templateID, fieldID string) *ActorStateSchemaRequirementReview {
		templateID = normalizeActorStateID(templateID)
		fieldID = normalizeActorStateFieldName(fieldID)
		for index := range item.Requirements {
			review := &item.Requirements[index]
			if strings.TrimSpace(review.Decision) != decision || normalizeActorStateID(review.TemplateID) != templateID || normalizeActorStateFieldName(review.FieldID) != fieldID {
				continue
			}
			return review
		}
		return nil
	}
	validateField := func(op ActorStateFieldSchemaOp, templateID, decision, fieldID, path string) *ActorStateSchemaBatchIssue {
		review := findReview(decision, templateID, fieldID)
		if review == nil {
			issue := actorStateSchemaBatchIssue(item.ItemID, "unsourced_adaptation_op", path, fmt.Sprintf("字段操作缺少同一 item 中指向 template=%s field=%s 且 decision=%s 的 requirement", templateID, fieldID, decision))
			return &issue
		}
		if (op.Op == "add" || op.Op == "replace") && op.Field.Default != nil && strings.TrimSpace(review.EvidenceKind) == "inferred" {
			issue := actorStateSchemaBatchIssue(item.ItemID, "inferred_template_default", path+".field.default", "合理推测的具体 Actor 值不能写成整个模板的通用 default；请改用 initial_actor_ops 或 actor_ops")
			return &issue
		}
		return nil
	}
	for templateIndex, templateOp := range item.Adaptation.TemplateOps {
		templateID := normalizeActorStateID(firstNonEmptyString(templateOp.TemplateID, templateOp.Template.ID))
		templatePath := fmt.Sprintf("%s.adaptation.template_ops[%d]", basePath, templateIndex)
		switch templateOp.Op {
		case "add":
			if len(templateOp.Template.Fields) == 0 {
				issue := actorStateSchemaBatchIssue(item.ItemID, "unsourced_adaptation_op", templatePath, "新增空模板无法对应长期状态 requirement")
				return &issue
			}
			for fieldIndex, field := range templateOp.Template.Fields {
				fieldID := actorStateFieldID(field)
				op := ActorStateFieldSchemaOp{Op: "add", Field: field}
				if issue := validateField(op, templateID, "add", fieldID, fmt.Sprintf("%s.template.fields[%d]", templatePath, fieldIndex)); issue != nil {
					return issue
				}
			}
		case "remove":
			if findReview("ignored", templateID, "") == nil {
				issue := actorStateSchemaBatchIssue(item.ItemID, "unsourced_adaptation_op", templatePath, fmt.Sprintf("删除模板缺少同一 item 中指向 template=%s、field_id 为空且 decision=ignored 的 requirement", templateID))
				return &issue
			}
		case "fields":
			if len(templateOp.FieldOps) == 0 {
				issue := actorStateSchemaBatchIssue(item.ItemID, "unsourced_adaptation_op", templatePath+".field_ops", "fields 操作至少需要一个有来源的字段操作")
				return &issue
			}
			for fieldIndex, fieldOp := range templateOp.FieldOps {
				fieldPath := fmt.Sprintf("%s.field_ops[%d]", templatePath, fieldIndex)
				switch fieldOp.Op {
				case "add":
					if issue := validateField(fieldOp, templateID, "add", actorStateFieldID(fieldOp.Field), fieldPath); issue != nil {
						return issue
					}
				case "replace":
					if issue := validateField(fieldOp, templateID, "replace", actorStateFieldID(fieldOp.Field), fieldPath); issue != nil {
						return issue
					}
				case "remove":
					if issue := validateField(fieldOp, templateID, "ignored", fieldOp.FieldID, fieldPath); issue != nil {
						return issue
					}
				}
			}
		}
	}
	return nil
}

func actorStateSchemaBatchSourceAllowed(allowedIDs []string, sourceID string) bool {
	sourceID = strings.TrimSpace(sourceID)
	for _, allowedID := range allowedIDs {
		if strings.TrimSpace(allowedID) == sourceID && sourceID != "" {
			return true
		}
	}
	return false
}

func actorStateSchemaBatchItemValueSource(item ActorStateSchemaBatchItem) (ActorStateSchemaActorValueSource, bool) {
	var source ActorStateSchemaActorValueSource
	var sourceKey string
	for _, requirement := range item.Requirements {
		kind := strings.TrimSpace(requirement.Source.Kind)
		id := strings.TrimSpace(requirement.Source.ID)
		evidenceKind := strings.TrimSpace(requirement.EvidenceKind)
		key := kind + "\x00" + id + "\x00" + evidenceKind
		if sourceKey != "" && sourceKey != key {
			return ActorStateSchemaActorValueSource{}, false
		}
		sourceKey = key
		source = ActorStateSchemaActorValueSource{
			SourceID: actorStateSchemaBatchSourceIDPrefix + strings.TrimSpace(item.ItemID),
			ItemID:   strings.TrimSpace(item.ItemID), Source: ActorStateSchemaRequirementSource{Kind: kind, ID: id}, EvidenceKind: evidenceKind,
		}
	}
	return source, sourceKey != ""
}

func actorStateSchemaBatchHasWholeActorOps(adaptation ActorStateSchemaAdaptation) bool {
	if len(adaptation.InitialActorOps) > 0 {
		return true
	}
	for _, op := range adaptation.ActorOps {
		if op.Op != "set" {
			return true
		}
	}
	return false
}

func actorStateSchemaBatchActorFieldValueSource(item ActorStateSchemaBatchItem, actorID, fieldID string) (ActorStateSchemaActorValueSource, bool) {
	actorID = normalizeStatePanelActorID(actorID)
	fieldID = normalizeActorStateFieldName(fieldID)
	var source ActorStateSchemaActorValueSource
	key := ""
	for _, requirement := range item.Requirements {
		if strings.TrimSpace(requirement.ValuePolicy) != ActorStateSchemaValuePolicyInitialize || normalizeStatePanelActorID(requirement.ActorID) != actorID || normalizeActorStateFieldName(requirement.FieldID) != fieldID {
			continue
		}
		nextKey := strings.TrimSpace(requirement.Source.Kind) + "\x00" + strings.TrimSpace(requirement.Source.ID) + "\x00" + strings.TrimSpace(requirement.EvidenceKind)
		if key != "" && key != nextKey {
			return ActorStateSchemaActorValueSource{}, false
		}
		key = nextKey
		source = ActorStateSchemaActorValueSource{
			SourceID: actorStateSchemaBatchSourceIDPrefix + strings.TrimSpace(item.ItemID),
			ItemID:   strings.TrimSpace(item.ItemID),
			Source: ActorStateSchemaRequirementSource{
				Kind: strings.TrimSpace(requirement.Source.Kind), ID: strings.TrimSpace(requirement.Source.ID),
			},
			EvidenceKind: strings.TrimSpace(requirement.EvidenceKind),
		}
	}
	return source, key != ""
}

type actorStateSchemaBatchTargetClaim struct {
	key        string
	templateID string
	actorID    string
	whole      bool
	path       string
}

func (d *ActorStateSchemaBatchDraft) actorStateSchemaBatchTargetConflict(item ActorStateSchemaBatchItem, path string) *ActorStateSchemaBatchIssue {
	existingClaims := map[string]string{}
	existingTemplateClaims := map[string]string{}
	existingWholeActorClaims := map[string]string{}
	existingActorFieldClaims := map[string]string{}
	for _, itemID := range d.order {
		for _, claim := range actorStateSchemaBatchTargetClaims(d.items[itemID].proposal.Adaptation, "") {
			existingClaims[claim.key] = itemID
			if claim.templateID != "" && (claim.whole || existingTemplateClaims[claim.templateID] == "") {
				existingTemplateClaims[claim.templateID] = itemID
			}
			if claim.actorID != "" {
				if claim.whole {
					existingWholeActorClaims[claim.actorID] = itemID
				} else if existingActorFieldClaims[claim.actorID] == "" {
					existingActorFieldClaims[claim.actorID] = itemID
				}
			}
		}
	}
	for _, claim := range actorStateSchemaBatchTargetClaims(item.Adaptation, path+".adaptation") {
		conflictItemID := existingClaims[claim.key]
		if conflictItemID == "" && claim.templateID != "" {
			if claim.whole {
				conflictItemID = existingTemplateClaims[claim.templateID]
			} else {
				conflictItemID = existingClaims["template:"+claim.templateID]
			}
		}
		if conflictItemID == "" && claim.actorID != "" {
			if claim.whole {
				conflictItemID = firstNonEmptyString(existingWholeActorClaims[claim.actorID], existingActorFieldClaims[claim.actorID])
			} else {
				conflictItemID = existingWholeActorClaims[claim.actorID]
			}
		}
		if conflictItemID == "" {
			existingClaims[claim.key] = item.ItemID
			if claim.templateID != "" && (claim.whole || existingTemplateClaims[claim.templateID] == "") {
				existingTemplateClaims[claim.templateID] = item.ItemID
			}
			if claim.actorID != "" {
				if claim.whole {
					existingWholeActorClaims[claim.actorID] = item.ItemID
				} else if existingActorFieldClaims[claim.actorID] == "" {
					existingActorFieldClaims[claim.actorID] = item.ItemID
				}
			}
			continue
		}
		message := fmt.Sprintf("目标 %s 已由 accepted item %s 修改；请删除重复操作", claim.key, conflictItemID)
		if conflictItemID == item.ItemID {
			message = fmt.Sprintf("同一 item 重复修改目标 %s；每个目标只能有一个操作", claim.key)
		}
		issue := actorStateSchemaBatchIssue(item.ItemID, "target_conflict", claim.path, message)
		return &issue
	}
	return nil
}

func actorStateSchemaBatchTargetClaims(adaptation ActorStateSchemaAdaptation, basePath string) []actorStateSchemaBatchTargetClaim {
	claims := make([]actorStateSchemaBatchTargetClaim, 0, len(adaptation.TemplateOps)+len(adaptation.InitialActorOps)+len(adaptation.ActorOps))
	for templateIndex, templateOp := range adaptation.TemplateOps {
		templateID := normalizeActorStateID(firstNonEmptyString(templateOp.TemplateID, templateOp.Template.ID))
		templatePath := fmt.Sprintf("%s.template_ops[%d]", basePath, templateIndex)
		if templateOp.Op != "fields" {
			if templateID != "" {
				claims = append(claims, actorStateSchemaBatchTargetClaim{key: "template:" + templateID, templateID: templateID, whole: true, path: templatePath})
			}
			continue
		}
		for fieldIndex, fieldOp := range templateOp.FieldOps {
			fieldID := normalizeActorStateFieldName(firstNonEmptyString(fieldOp.FieldID, fieldOp.Field.Name))
			if templateID == "" || fieldID == "" {
				continue
			}
			claims = append(claims, actorStateSchemaBatchTargetClaim{
				key: "field:" + templateID + ":" + fieldID, templateID: templateID,
				path: fmt.Sprintf("%s.field_ops[%d]", templatePath, fieldIndex),
			})
		}
	}
	for index, op := range adaptation.InitialActorOps {
		if actorID := normalizeStatePanelActorID(firstNonEmptyString(op.ActorID, op.Actor.ID)); actorID != "" {
			claims = append(claims, actorStateSchemaBatchTargetClaim{key: "actor:" + actorID, actorID: actorID, whole: true, path: fmt.Sprintf("%s.initial_actor_ops[%d]", basePath, index)})
		}
	}
	for index, op := range adaptation.ActorOps {
		if actorID := normalizeStatePanelActorID(firstNonEmptyString(op.ActorID, op.Actor.ID)); actorID != "" {
			if op.Op == "set" {
				fieldID := normalizeActorStateFieldName(op.FieldID)
				claims = append(claims, actorStateSchemaBatchTargetClaim{key: "actor_field:" + actorID + ":" + fieldID, actorID: actorID, path: fmt.Sprintf("%s.actor_ops[%d]", basePath, index)})
				continue
			}
			claims = append(claims, actorStateSchemaBatchTargetClaim{key: "actor:" + actorID, actorID: actorID, whole: true, path: fmt.Sprintf("%s.actor_ops[%d]", basePath, index)})
		}
	}
	return claims
}

func validateActorStateSchemaBatchActorValueVisibility(itemID string, proposal ActorStateSchemaProposal, target StoryDirectorActorStateSystem, basePath string) *ActorStateSchemaBatchIssue {
	validate := func(source *ActorStateSchemaActorValueSource, actor ActorStateInitialActor, path string) *ActorStateSchemaBatchIssue {
		if len(actor.State) == 0 {
			return nil
		}
		template := actorStateTemplateByID(target, actor.TemplateID)
		fields := actorStateFieldsByReference(template)
		for rawFieldID, value := range actor.State {
			if !actorStateSchemaBatchValueInitialized(value) {
				issue := actorStateSchemaBatchIssue(itemID, "invalid_actor_value", path+".actor.state."+rawFieldID, "Actor 字段初始化值不能为 null、空字符串或纯空白字符串；无法确定时请使用 value_policy=defer")
				return &issue
			}
			if source == nil || source.EvidenceKind != "inferred" {
				continue
			}
			field, ok := fields[actorStateFieldNameKey(rawFieldID)]
			if !ok || (field.Visibility != "spoiler" && field.Visibility != "hidden") {
				continue
			}
			issue := actorStateSchemaBatchIssue(itemID, "inferred_secret_value", path+".actor.state."+rawFieldID, fmt.Sprintf("inferred Actor 值不能写入 %s 字段 %s", field.Visibility, actorStateFieldID(field)))
			return &issue
		}
		return nil
	}
	for index, op := range proposal.Adaptation.InitialActorOps {
		if issue := validate(op.ValueSource, op.Actor, fmt.Sprintf("%s.adaptation.initial_actor_ops[%d]", basePath, index)); issue != nil {
			return issue
		}
	}
	for index, op := range proposal.Adaptation.ActorOps {
		if op.Op == "set" {
			review, ok := actorStateSchemaBatchActorFieldReview(proposal.Requirements, op.ActorID, op.FieldID)
			if !ok {
				continue
			}
			template := actorStateTemplateByID(target, review.TemplateID)
			field, exists := actorStateFieldByID(template, review.FieldID)
			if !exists {
				issue := actorStateSchemaBatchIssue(itemID, "target_field_not_found", fmt.Sprintf("%s.adaptation.actor_ops[%d].field_id", basePath, index), fmt.Sprintf("Actor 字段初始化目标不存在: template=%s field=%s", review.TemplateID, review.FieldID))
				return &issue
			}
			if !actorStateSchemaBatchValueInitialized(op.Value) {
				issue := actorStateSchemaBatchIssue(itemID, "invalid_actor_value", fmt.Sprintf("%s.adaptation.actor_ops[%d].value", basePath, index), "Actor 字段初始化值不能为空")
				return &issue
			}
			if _, err := normalizeActorStateValue(field, op.Value); err != nil {
				issue := actorStateSchemaBatchIssue(itemID, "invalid_actor_value", fmt.Sprintf("%s.adaptation.actor_ops[%d].value", basePath, index), err.Error())
				return &issue
			}
			if op.ValueSource == nil || op.ValueSource.EvidenceKind != "inferred" {
				continue
			}
			if exists && (field.Visibility == "spoiler" || field.Visibility == "hidden") {
				issue := actorStateSchemaBatchIssue(itemID, "inferred_secret_value", fmt.Sprintf("%s.adaptation.actor_ops[%d].value", basePath, index), fmt.Sprintf("inferred Actor 值不能写入 %s 字段 %s", field.Visibility, actorStateFieldID(field)))
				return &issue
			}
			continue
		}
		if issue := validate(op.ValueSource, op.Actor, fmt.Sprintf("%s.adaptation.actor_ops[%d]", basePath, index)); issue != nil {
			return issue
		}
	}
	return nil
}

func validateActorStateSchemaBatchActorValueSources(itemID string, proposal ActorStateSchemaProposal, basePath string) *ActorStateSchemaBatchIssue {
	hasReview := func(actorID, templateID, fieldID string) bool {
		actorID = normalizeStatePanelActorID(actorID)
		templateID = normalizeActorStateID(templateID)
		fieldID = normalizeActorStateFieldName(fieldID)
		for _, review := range proposal.Requirements {
			if strings.TrimSpace(review.Decision) == "ignored" {
				continue
			}
			if strings.TrimSpace(review.ValuePolicy) == ActorStateSchemaValuePolicyInitialize && normalizeStatePanelActorID(review.ActorID) == actorID && normalizeActorStateID(review.TemplateID) == templateID && normalizeActorStateFieldName(review.FieldID) == fieldID {
				return true
			}
		}
		return false
	}
	validate := func(actorID string, actor ActorStateInitialActor, path string) *ActorStateSchemaBatchIssue {
		for rawFieldID := range actor.State {
			if hasReview(actorID, actor.TemplateID, rawFieldID) {
				continue
			}
			issue := actorStateSchemaBatchIssue(itemID, "unsourced_actor_value", path+".actor.state."+rawFieldID, fmt.Sprintf("Actor 值缺少同一 item 中指向 template=%s field=%s 的非 ignored requirement", actor.TemplateID, rawFieldID))
			return &issue
		}
		return nil
	}
	for index, op := range proposal.Adaptation.InitialActorOps {
		actorID := firstNonEmptyString(op.ActorID, op.Actor.ID)
		if issue := validate(actorID, op.Actor, fmt.Sprintf("%s.adaptation.initial_actor_ops[%d]", basePath, index)); issue != nil {
			return issue
		}
	}
	for index, op := range proposal.Adaptation.ActorOps {
		path := fmt.Sprintf("%s.adaptation.actor_ops[%d]", basePath, index)
		if op.Op == "set" {
			review, ok := actorStateSchemaBatchActorFieldReview(proposal.Requirements, op.ActorID, op.FieldID)
			if !ok || strings.TrimSpace(review.ValuePolicy) != ActorStateSchemaValuePolicyInitialize {
				issue := actorStateSchemaBatchIssue(itemID, "unsourced_actor_value", path+".value", fmt.Sprintf("Actor 字段值缺少同一 item 中 actor_id=%s field=%s 且 value_policy=initialize 的 requirement", op.ActorID, op.FieldID))
				return &issue
			}
			continue
		}
		actorID := firstNonEmptyString(op.ActorID, op.Actor.ID)
		if issue := validate(actorID, op.Actor, path); issue != nil {
			return issue
		}
	}
	return nil
}

func actorStateSchemaBatchActorFieldReview(reviews []ActorStateSchemaRequirementReview, actorID, fieldID string) (ActorStateSchemaRequirementReview, bool) {
	actorID = normalizeStatePanelActorID(actorID)
	fieldID = normalizeActorStateFieldName(fieldID)
	for _, review := range reviews {
		if strings.TrimSpace(review.Decision) == "ignored" || normalizeStatePanelActorID(review.ActorID) != actorID || normalizeActorStateFieldName(review.FieldID) != fieldID {
			continue
		}
		return review, true
	}
	return ActorStateSchemaRequirementReview{}, false
}
