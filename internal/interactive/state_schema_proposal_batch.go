package interactive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	actorStateSchemaBatchSourceIDPrefix = "state_schema_batch:"
	// StateSchemaBatchMaxItems bounds one tool response while allowing the
	// run-local draft to accumulate several batches.
	StateSchemaBatchMaxItems             = 16
	maxActorStateSchemaBatchDependencies = 16
	maxActorStateSchemaBatchItemIDBytes  = 128 - len(actorStateSchemaBatchSourceIDPrefix)
)

// ActorStateSchemaBatch incrementally adds independently retryable proposal
// items to one run-local draft. Story state is not modified by this type.
type ActorStateSchemaBatch struct {
	Summary  string                      `json:"summary,omitempty" jsonschema:"description=本次审查的简短摘要；后续批次可更新"`
	Items    []ActorStateSchemaBatchItem `json:"items" jsonschema:"description=本次新增的独立提案项；已 accepted 的 item_id 不要重传，单批最多 16 项"`
	Finalize bool                        `json:"finalize" jsonschema:"description=是否在接收本批成功项后完成草稿；失败或阻塞项存在时不会 finalize"`
}

// ActorStateSchemaBatchItem groups one sourced requirement decision with the
// minimal schema or Actor operations needed to satisfy it.
type ActorStateSchemaBatchItem struct {
	ItemID       string                              `json:"item_id" jsonschema:"description=稳定且唯一的幂等 ID，仅使用字母、数字、点、下划线、冒号或短横线"`
	DependsOn    []string                            `json:"depends_on,omitempty" jsonschema:"description=本项依赖的其他 item_id；依赖项未 accepted 时本项返回 blocked"`
	Summary      string                              `json:"summary,omitempty"`
	Requirements []ActorStateSchemaRequirementReview `json:"requirements" jsonschema:"description=本项自包含的来源化需求审查；每项必须填写 evidence_kind"`
	Adaptation   ActorStateSchemaAdaptation          `json:"adaptation" jsonschema:"description=仅包含满足本项 requirements 所需的最小 diff"`
}

// ActorStateSchemaBatchAudit is supplied by the backend. Model input cannot
// claim Lore reads or choose the revision recorded in the final audit.
type ActorStateSchemaBatchAudit struct {
	ReviewedLoreIDs     []string
	OpeningSourceIDs    []string
	TurnResultSourceIDs []string
	TRPGSourceIDs       []string
	SourceLoreRevision  string
}

// ActorStateSchemaBatchAccepted describes one item accepted in this call.
type ActorStateSchemaBatchAccepted struct {
	ItemID          string                          `json:"item_id"`
	AlreadyAccepted bool                            `json:"already_accepted,omitempty"`
	Preview         ActorStateSchemaProposalPreview `json:"preview"`
}

// ActorStateSchemaBatchIssue gives the model a stable code and precise input
// path so a retry can contain only the failed item.
type ActorStateSchemaBatchIssue struct {
	ItemID    string   `json:"item_id,omitempty"`
	Code      string   `json:"code"`
	Path      string   `json:"path"`
	Message   string   `json:"message"`
	Retryable bool     `json:"retryable"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// ActorStateSchemaBatchResult reports partial progress without converting
// item-level validation failures into an opaque tool execution failure.
type ActorStateSchemaBatchResult struct {
	Accepted           []ActorStateSchemaBatchAccepted  `json:"accepted"`
	Rejected           []ActorStateSchemaBatchIssue     `json:"rejected"`
	Blocked            []ActorStateSchemaBatchIssue     `json:"blocked"`
	DraftAcceptedItems int                              `json:"draft_accepted_items"`
	Finalized          bool                             `json:"finalized"`
	Preview            *ActorStateSchemaProposalPreview `json:"preview,omitempty"`
}

type actorStateSchemaDraftItem struct {
	proposal    ActorStateSchemaProposal
	fingerprint string
}

// ActorStateSchemaBatchDraft owns accepted items for one Agent run. Callers
// should serialize access; the app already shares its Lore-read audit mutex.
type ActorStateSchemaBatchDraft struct {
	base      StoryDirectorActorStateSystem
	trpg      StoryDirectorTRPGSystem
	summary   string
	order     []string
	items     map[string]actorStateSchemaDraftItem
	finalized *ActorStateSchemaProposal
}

func NewActorStateSchemaBatchDraft(base StoryDirectorActorStateSystem, trpg StoryDirectorTRPGSystem) *ActorStateSchemaBatchDraft {
	return &ActorStateSchemaBatchDraft{
		base:  base,
		trpg:  trpg,
		items: map[string]actorStateSchemaDraftItem{},
	}
}

// Submit accepts every valid item independently and optionally finalizes the
// accumulated proposal. A failed later call never removes accepted items.
func (d *ActorStateSchemaBatchDraft) Submit(batch ActorStateSchemaBatch, audit ActorStateSchemaBatchAudit) ActorStateSchemaBatchResult {
	result := ActorStateSchemaBatchResult{
		Accepted: []ActorStateSchemaBatchAccepted{},
		Rejected: []ActorStateSchemaBatchIssue{},
		Blocked:  []ActorStateSchemaBatchIssue{},
	}
	if d == nil {
		result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue("", "draft_unavailable", "", "状态结构 Batch 草稿不可用"))
		return result
	}
	if len(batch.Items) > StateSchemaBatchMaxItems {
		result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue("", "batch_too_large", "items", fmt.Sprintf("单批 items 过多: %d > %d", len(batch.Items), StateSchemaBatchMaxItems)))
		result.DraftAcceptedItems = len(d.order)
		result.Finalized = d.finalized != nil
		return result
	}
	requestedSummary := trimBytes(batch.Summary, maxTurnBriefTextBytes)
	if d.finalized != nil {
		for index, item := range batch.Items {
			result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue(strings.TrimSpace(item.ItemID), "draft_finalized", fmt.Sprintf("items[%d]", index), "状态结构草稿已经 finalize，不能再增加或替换 item"))
		}
		result.DraftAcceptedItems = len(d.order)
		result.Finalized = true
		preview := actorStateSchemaProposalPreview(*d.finalized)
		result.Preview = &preview
		return result
	}

	type pendingItem struct {
		index       int
		item        ActorStateSchemaBatchItem
		fingerprint string
	}
	pending := make([]pendingItem, 0, len(batch.Items))
	itemIDCounts := make(map[string]int, len(batch.Items))
	for _, item := range batch.Items {
		itemIDCounts[strings.TrimSpace(item.ItemID)]++
	}
	for index := range batch.Items {
		item := batch.Items[index]
		item.ItemID = strings.TrimSpace(item.ItemID)
		item.DependsOn = append([]string(nil), item.DependsOn...)
		path := fmt.Sprintf("items[%d]", index)
		if err := validateActorStateSchemaBatchItemID(item.ItemID); err != nil {
			result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue(item.ItemID, "invalid_item_id", path+".item_id", err.Error()))
			continue
		}
		if itemIDCounts[item.ItemID] > 1 {
			result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue(item.ItemID, "duplicate_item_id", path+".item_id", "同一 Batch 中 item_id 重复"))
			continue
		}
		if len(item.DependsOn) > maxActorStateSchemaBatchDependencies {
			result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue(item.ItemID, "too_many_dependencies", path+".depends_on", fmt.Sprintf("depends_on 过多: %d > %d", len(item.DependsOn), maxActorStateSchemaBatchDependencies)))
			continue
		}
		dependenciesValid := true
		for depIndex := range item.DependsOn {
			item.DependsOn[depIndex] = strings.TrimSpace(item.DependsOn[depIndex])
			if err := validateActorStateSchemaBatchItemID(item.DependsOn[depIndex]); err != nil || item.DependsOn[depIndex] == item.ItemID {
				result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue(item.ItemID, "invalid_dependency", fmt.Sprintf("%s.depends_on[%d]", path, depIndex), "depends_on 必须引用另一个合法 item_id"))
				dependenciesValid = false
				break
			}
		}
		if !dependenciesValid {
			continue
		}
		fingerprint := actorStateSchemaBatchItemFingerprint(item)
		if stored, ok := d.items[item.ItemID]; ok {
			if stored.fingerprint != fingerprint {
				result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue(item.ItemID, "item_id_conflict", path, "已 accepted 的 item_id 不能用于不同内容"))
				continue
			}
			result.Accepted = append(result.Accepted, ActorStateSchemaBatchAccepted{ItemID: item.ItemID, AlreadyAccepted: true, Preview: actorStateSchemaProposalPreview(stored.proposal)})
			continue
		}
		pending = append(pending, pendingItem{index: index, item: item, fingerprint: fingerprint})
	}

	for len(pending) > 0 {
		madeProgress := false
		remaining := pending[:0]
		for _, candidate := range pending {
			missingDependencies := d.missingDependencies(candidate.item.DependsOn)
			if len(missingDependencies) > 0 {
				remaining = append(remaining, candidate)
				continue
			}
			path := fmt.Sprintf("items[%d]", candidate.index)
			if issue, blocked := validateActorStateSchemaBatchItemInput(candidate.item, path, audit); issue != nil {
				if blocked {
					result.Blocked = append(result.Blocked, *issue)
				} else {
					result.Rejected = append(result.Rejected, *issue)
				}
				madeProgress = true
				continue
			}
			if issue := d.actorStateSchemaBatchTargetConflict(candidate.item, path); issue != nil {
				result.Rejected = append(result.Rejected, *issue)
				madeProgress = true
				continue
			}
			before := d.mergedProposal(nil, audit)
			itemProposal := actorStateSchemaProposalFromBatchItem(candidate.item)
			combined := mergeActorStateSchemaProposals(before, itemProposal)
			normalized, _, err := ValidateActorStateSchemaProposal(d.base, d.trpg, combined)
			if err != nil {
				issue := actorStateSchemaBatchIssue(candidate.item.ItemID, actorStateSchemaBatchValidationCode(err), actorStateSchemaBatchValidationPath(path, candidate.item, err), err.Error())
				result.Rejected = append(result.Rejected, issue)
				madeProgress = true
				continue
			}
			normalizedItem := actorStateSchemaProposalTail(normalized, before, candidate.item.Summary)
			targetSystem, _, targetErr := ApplyActorStateSchemaAdaptation(d.base, d.trpg, normalized.Adaptation)
			if targetErr != nil {
				result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue(candidate.item.ItemID, "validation_failed", path+".adaptation", targetErr.Error()))
				madeProgress = true
				continue
			}
			if issue := validateActorStateSchemaBatchActorValueVisibility(candidate.item.ItemID, normalizedItem.Adaptation, targetSystem, path); issue != nil {
				result.Rejected = append(result.Rejected, *issue)
				madeProgress = true
				continue
			}
			if issue := validateActorStateSchemaBatchActorValueSources(candidate.item.ItemID, normalizedItem, path); issue != nil {
				result.Rejected = append(result.Rejected, *issue)
				madeProgress = true
				continue
			}
			d.items[candidate.item.ItemID] = actorStateSchemaDraftItem{proposal: normalizedItem, fingerprint: candidate.fingerprint}
			d.order = append(d.order, candidate.item.ItemID)
			result.Accepted = append(result.Accepted, ActorStateSchemaBatchAccepted{ItemID: candidate.item.ItemID, Preview: actorStateSchemaProposalPreview(normalizedItem)})
			madeProgress = true
		}
		pending = remaining
		if !madeProgress {
			break
		}
	}
	for _, candidate := range pending {
		missing := d.missingDependencies(candidate.item.DependsOn)
		result.Blocked = append(result.Blocked, ActorStateSchemaBatchIssue{
			ItemID: candidate.item.ItemID, Code: "dependency_not_accepted", Path: fmt.Sprintf("items[%d].depends_on", candidate.index),
			Message: "依赖项尚未 accepted", Retryable: true, DependsOn: missing,
		})
	}
	if strings.TrimSpace(requestedSummary) != "" && (len(result.Accepted) > 0 || len(batch.Items) == 0) {
		d.summary = requestedSummary
	}

	result.DraftAcceptedItems = len(d.order)
	if !batch.Finalize || len(result.Rejected) > 0 || len(result.Blocked) > 0 {
		return result
	}
	if len(d.order) == 0 {
		result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue("", "empty_draft", "items", "状态结构草稿至少需要一个有来源的审查 item"))
		return result
	}
	proposal := d.mergedProposal(nil, audit)
	normalized, preview, err := ValidateActorStateSchemaProposal(d.base, d.trpg, proposal)
	if err != nil {
		result.Rejected = append(result.Rejected, actorStateSchemaBatchIssue("", "finalize_validation_failed", "items", err.Error()))
		return result
	}
	d.finalized = &normalized
	result.Finalized = true
	result.Preview = &preview
	return result
}

// FinalProposal returns a deep copy only after a successful finalize.
func (d *ActorStateSchemaBatchDraft) FinalProposal() (ActorStateSchemaProposal, bool) {
	if d == nil || d.finalized == nil {
		return ActorStateSchemaProposal{}, false
	}
	return cloneActorStateSchemaProposal(*d.finalized), true
}

func (d *ActorStateSchemaBatchDraft) missingDependencies(dependencies []string) []string {
	missing := make([]string, 0, len(dependencies))
	for _, id := range dependencies {
		if _, ok := d.items[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing
}

func (d *ActorStateSchemaBatchDraft) mergedProposal(extra *ActorStateSchemaProposal, audit ActorStateSchemaBatchAudit) ActorStateSchemaProposal {
	proposal := ActorStateSchemaProposal{Summary: d.summary}
	for _, id := range d.order {
		proposal = mergeActorStateSchemaProposals(proposal, d.items[id].proposal)
	}
	if extra != nil {
		proposal = mergeActorStateSchemaProposals(proposal, *extra)
	}
	if strings.TrimSpace(d.summary) != "" {
		proposal.Summary = d.summary
	}
	proposal.ReviewedLoreIDs = normalizedActorStateSchemaAuditLoreIDs(audit.ReviewedLoreIDs)
	proposal.SourceLoreRevision = strings.TrimSpace(audit.SourceLoreRevision)
	return proposal
}

func actorStateSchemaProposalFromBatchItem(item ActorStateSchemaBatchItem) ActorStateSchemaProposal {
	summary := trimBytes(item.Summary, maxTurnBriefTextBytes)
	adaptation := item.Adaptation
	adaptation.InitialActorOps = append([]ActorStateInitialActorSchemaOp(nil), item.Adaptation.InitialActorOps...)
	adaptation.ActorOps = append([]ActorStateRuntimeSchemaOp(nil), item.Adaptation.ActorOps...)
	if strings.TrimSpace(adaptation.Summary) == "" {
		adaptation.Summary = summary
	}
	requirements := append([]ActorStateSchemaRequirementReview(nil), item.Requirements...)
	for index := range requirements {
		requirements[index].ItemID = item.ItemID
	}
	valueSource, hasValueSource := actorStateSchemaBatchItemValueSource(item)
	for index := range adaptation.InitialActorOps {
		adaptation.InitialActorOps[index].ValueSource = nil
		if hasValueSource {
			source := valueSource
			adaptation.InitialActorOps[index].ValueSource = &source
		}
	}
	for index := range adaptation.ActorOps {
		adaptation.ActorOps[index].ValueSource = nil
		if hasValueSource {
			source := valueSource
			adaptation.ActorOps[index].ValueSource = &source
		}
	}
	return ActorStateSchemaProposal{Summary: summary, Requirements: requirements, Adaptation: adaptation}
}

func mergeActorStateSchemaProposals(base, addition ActorStateSchemaProposal) ActorStateSchemaProposal {
	merged := base
	if strings.TrimSpace(addition.Summary) != "" {
		merged.Summary = trimBytes(addition.Summary, maxTurnBriefTextBytes)
	}
	if strings.TrimSpace(addition.Adaptation.Summary) != "" {
		merged.Adaptation.Summary = trimBytes(addition.Adaptation.Summary, maxTurnBriefTextBytes)
	}
	merged.Requirements = append(merged.Requirements, addition.Requirements...)
	merged.Adaptation.TemplateOps = append(merged.Adaptation.TemplateOps, addition.Adaptation.TemplateOps...)
	merged.Adaptation.InitialActorOps = append(merged.Adaptation.InitialActorOps, addition.Adaptation.InitialActorOps...)
	merged.Adaptation.ActorOps = append(merged.Adaptation.ActorOps, addition.Adaptation.ActorOps...)
	return merged
}

func actorStateSchemaProposalTail(normalized, before ActorStateSchemaProposal, itemSummary string) ActorStateSchemaProposal {
	proposal := ActorStateSchemaProposal{
		Summary:      trimBytes(itemSummary, maxTurnBriefTextBytes),
		Requirements: append([]ActorStateSchemaRequirementReview(nil), normalized.Requirements[len(before.Requirements):]...),
		Adaptation: ActorStateSchemaAdaptation{
			TemplateOps:     append([]ActorStateTemplateSchemaOp(nil), normalized.Adaptation.TemplateOps[len(before.Adaptation.TemplateOps):]...),
			InitialActorOps: append([]ActorStateInitialActorSchemaOp(nil), normalized.Adaptation.InitialActorOps[len(before.Adaptation.InitialActorOps):]...),
			ActorOps:        append([]ActorStateRuntimeSchemaOp(nil), normalized.Adaptation.ActorOps[len(before.Adaptation.ActorOps):]...),
		},
	}
	if strings.TrimSpace(proposal.Summary) == "" {
		proposal.Summary = normalized.Summary
	}
	proposal.Adaptation.Summary = proposal.Summary
	return cloneActorStateSchemaProposal(proposal)
}

func actorStateSchemaProposalPreview(proposal ActorStateSchemaProposal) ActorStateSchemaProposalPreview {
	fieldOps := 0
	for _, op := range proposal.Adaptation.TemplateOps {
		fieldOps += len(op.FieldOps)
	}
	return ActorStateSchemaProposalPreview{
		Summary:     firstNonEmptyString(proposal.Summary, proposal.Adaptation.Summary),
		TemplateOps: len(proposal.Adaptation.TemplateOps), FieldOps: fieldOps,
		InitialActorOps: len(proposal.Adaptation.InitialActorOps), ActorOps: len(proposal.Adaptation.ActorOps),
	}
}

func actorStateSchemaBatchIssue(itemID, code, path, message string) ActorStateSchemaBatchIssue {
	return ActorStateSchemaBatchIssue{ItemID: itemID, Code: code, Path: path, Message: message, Retryable: true}
}

func validateActorStateSchemaBatchItemID(id string) error {
	if id == "" {
		return fmt.Errorf("item_id 不能为空")
	}
	if len(id) > maxActorStateSchemaBatchItemIDBytes {
		return fmt.Errorf("item_id 过长: %d > %d bytes", len(id), maxActorStateSchemaBatchItemIDBytes)
	}
	for index, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (index > 0 && (r == '.' || r == '_' || r == ':' || r == '-')) {
			continue
		}
		return fmt.Errorf("item_id 只能使用字母、数字、点、下划线、冒号或短横线，且必须以字母或数字开头")
	}
	return nil
}

func actorStateSchemaBatchItemFingerprint(item ActorStateSchemaBatchItem) string {
	data, _ := json.Marshal(item)
	return string(data)
}

func actorStateSchemaBatchValidationCode(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "expected_type"):
		return "invalid_expected_type"
	case strings.Contains(message, "来源"):
		return "invalid_source"
	case strings.Contains(message, "覆盖字段不存在"):
		return "target_field_not_found"
	case strings.Contains(message, "schema 操作"):
		return "missing_schema_operation"
	case strings.Contains(message, "操作过多"):
		return "too_many_operations"
	default:
		return "validation_failed"
	}
}

func actorStateSchemaBatchValidationPath(basePath string, item ActorStateSchemaBatchItem, err error) string {
	message := err.Error()
	for index, requirement := range item.Requirements {
		if strings.Contains(message, "source="+strings.TrimSpace(requirement.Source.ID)) {
			return fmt.Sprintf("%s.requirements[%d]", basePath, index)
		}
	}
	if strings.Contains(message, "状态需求") || strings.Contains(message, "覆盖") || strings.Contains(message, "expected_type") {
		return basePath + ".requirements"
	}
	return basePath + ".adaptation"
}

func normalizedActorStateSchemaAuditLoreIDs(ids []string) []string {
	seen := map[string]bool{}
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			seen[id] = true
		}
	}
	normalized := make([]string, 0, len(seen))
	for id := range seen {
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
	return normalized
}

func cloneActorStateSchemaProposal(proposal ActorStateSchemaProposal) ActorStateSchemaProposal {
	data, err := json.Marshal(proposal)
	if err != nil {
		return proposal
	}
	var cloned ActorStateSchemaProposal
	if err := json.Unmarshal(data, &cloned); err != nil {
		return proposal
	}
	cloned.ReviewedLoreIDs = append([]string(nil), proposal.ReviewedLoreIDs...)
	cloned.SourceLoreRevision = proposal.SourceLoreRevision
	return cloned
}
