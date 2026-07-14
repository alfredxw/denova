package interactive

import (
	"strings"
	"testing"
)

func TestActorStateSchemaBatchDraftAcceptsValidItemsAndRetriesOnlyFailures(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	result := draft.Submit(ActorStateSchemaBatch{
		Summary: "增量审查",
		Items: []ActorStateSchemaBatchItem{
			batchTestCoveredItem("status", "confirmed"),
			batchTestCoveredItem("mood", ""),
		},
		Finalize: true,
	}, batchTestAudit())
	if len(result.Accepted) != 1 || result.Accepted[0].ItemID != "status" {
		t.Fatalf("valid item should be accepted independently: %#v", result)
	}
	if len(result.Rejected) != 1 || result.Rejected[0].ItemID != "mood" || result.Rejected[0].Code != "missing_evidence_kind" || result.Rejected[0].Path != "items[1].requirements[0].evidence_kind" {
		t.Fatalf("invalid item should return a precise retry path: %#v", result)
	}
	if result.Finalized || result.DraftAcceptedItems != 1 {
		t.Fatalf("partial failure must keep the accepted draft without finalizing: %#v", result)
	}
	if _, ok := draft.FinalProposal(); ok {
		t.Fatal("draft must not expose a proposal before successful finalize")
	}

	retry := draft.Submit(ActorStateSchemaBatch{
		Items:    []ActorStateSchemaBatchItem{batchTestCoveredItem("mood", "inferred")},
		Finalize: true,
	}, batchTestAudit())
	if len(retry.Accepted) != 1 || retry.Accepted[0].ItemID != "mood" || len(retry.Rejected) != 0 || !retry.Finalized || retry.DraftAcceptedItems != 2 {
		t.Fatalf("retry should send and accept only the failed item: %#v", retry)
	}
	proposal, ok := draft.FinalProposal()
	if !ok || proposal.Summary != "增量审查" || len(proposal.Requirements) != 2 || proposal.Requirements[1].EvidenceKind != "inferred" {
		t.Fatalf("final proposal should preserve accumulated normalized items: %#v", proposal)
	}
}

func TestActorStateSchemaBatchDraftReportsDependenciesAsBlocked(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	dependent := batchTestCoveredItem("mood", "confirmed")
	dependent.DependsOn = []string{"status"}
	blocked := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{dependent}, Finalize: true}, batchTestAudit())
	if len(blocked.Blocked) != 1 || blocked.Blocked[0].Code != "dependency_not_accepted" || len(blocked.Blocked[0].DependsOn) != 1 || blocked.Blocked[0].DependsOn[0] != "status" {
		t.Fatalf("missing dependency should be structured as blocked: %#v", blocked)
	}

	accepted := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{batchTestCoveredItem("status", "confirmed")}}, batchTestAudit())
	if len(accepted.Accepted) != 1 {
		t.Fatalf("dependency should be accepted separately: %#v", accepted)
	}
	retried := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{dependent}, Finalize: true}, batchTestAudit())
	if !retried.Finalized || len(retried.Accepted) != 1 || retried.Accepted[0].ItemID != "mood" {
		t.Fatalf("blocked item should finalize after its dependency succeeds: %#v", retried)
	}
}

func TestActorStateSchemaBatchDraftUsesBackendLoreAudit(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	item := batchTestCoveredItem("lore-status", "confirmed")
	item.Requirements[0].Source = ActorStateSchemaRequirementSource{Kind: "lore", ID: "status-rule"}
	blocked := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, ActorStateSchemaBatchAudit{})
	if len(blocked.Blocked) != 1 || blocked.Blocked[0].Code != "lore_not_reviewed" {
		t.Fatalf("model-declared lore source must wait for backend read audit: %#v", blocked)
	}

	finalized := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, ActorStateSchemaBatchAudit{
		ReviewedLoreIDs: []string{"status-rule"}, SourceLoreRevision: "backend-revision",
	})
	if !finalized.Finalized {
		t.Fatalf("backend-reviewed lore should allow finalize: %#v", finalized)
	}
	proposal, ok := draft.FinalProposal()
	if !ok || proposal.SourceLoreRevision != "backend-revision" || len(proposal.ReviewedLoreIDs) != 1 || proposal.ReviewedLoreIDs[0] != "status-rule" {
		t.Fatalf("final proposal must use backend-owned lore audit: %#v", proposal)
	}
}

func TestActorStateSchemaBatchDraftKeepsFinalizedProposalAfterLaterFailure(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	first := draft.Submit(ActorStateSchemaBatch{Summary: "有效审查", Items: []ActorStateSchemaBatchItem{batchTestCoveredItem("status", "confirmed")}, Finalize: true}, batchTestAudit())
	if !first.Finalized {
		t.Fatalf("first batch should finalize: %#v", first)
	}
	later := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{{ItemID: "broken"}}, Finalize: true}, batchTestAudit())
	if !later.Finalized || len(later.Rejected) != 1 || later.Rejected[0].Code != "draft_finalized" {
		t.Fatalf("later failure should report an error without clearing finalized state: %#v", later)
	}
	proposal, ok := draft.FinalProposal()
	if !ok || proposal.Summary != "有效审查" || len(proposal.Requirements) != 1 {
		t.Fatalf("finalized proposal must remain intact: %#v", proposal)
	}
}

func TestActorStateSchemaBatchDraftIsIdempotentByStableItemID(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	item := batchTestCoveredItem("status", "confirmed")
	first := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}}, batchTestAudit())
	second := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}}, batchTestAudit())
	if len(first.Accepted) != 1 || len(second.Accepted) != 1 || !second.Accepted[0].AlreadyAccepted || second.DraftAcceptedItems != 1 {
		t.Fatalf("repeating the same item must be an idempotent success: first=%#v second=%#v", first, second)
	}
	changed := item
	changed.Summary = "different content"
	conflict := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{changed}}, batchTestAudit())
	if len(conflict.Rejected) != 1 || conflict.Rejected[0].Code != "item_id_conflict" || conflict.DraftAcceptedItems != 1 {
		t.Fatalf("a stable ID must not silently replace accepted content: %#v", conflict)
	}
}

func TestActorStateSchemaBatchDraftRejectsConflictingTargetsAcrossAndWithinItems(t *testing.T) {
	base := batchTestActorStateSystem()
	tests := []struct {
		name         string
		first        ActorStateSchemaAdaptation
		second       ActorStateSchemaAdaptation
		expectedPath string
	}{
		{
			name: "remove then add field",
			first: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
				Op: "fields", TemplateID: "protagonist", FieldOps: []ActorStateFieldSchemaOp{{Op: "remove", FieldID: "状态"}},
			}}},
			second: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
				Op: "fields", TemplateID: "protagonist", FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "状态", Type: "string"}}},
			}}},
			expectedPath: "items[0].adaptation.template_ops[0].field_ops[0]",
		},
		{
			name: "replace then replace field",
			first: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
				Op: "fields", TemplateID: "protagonist", FieldOps: []ActorStateFieldSchemaOp{{Op: "replace", FieldID: "状态", Field: ActorStateField{Name: "状态", Type: "string"}}},
			}}},
			second: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
				Op: "fields", TemplateID: "protagonist", FieldOps: []ActorStateFieldSchemaOp{{Op: "replace", FieldID: "状态", Field: ActorStateField{Name: "状态", Type: "string", Default: "警觉"}}},
			}}},
			expectedPath: "items[0].adaptation.template_ops[0].field_ops[0]",
		},
		{
			name: "initial and runtime actor share a target",
			first: ActorStateSchemaAdaptation{InitialActorOps: []ActorStateInitialActorSchemaOp{{
				Op: "replace", ActorID: "guide", Actor: ActorStateInitialActor{ID: "guide", Name: "向导", TemplateID: "npc", State: map[string]any{"态度": "友善"}},
			}}},
			second:       ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{Op: "remove", ActorID: "guide"}}},
			expectedPath: "items[0].adaptation.actor_ops[0]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
			first := batchTestSourcedAdaptationItem("first", test.first, "confirmed")
			accepted := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{first}}, batchTestAudit())
			if len(accepted.Accepted) != 1 {
				t.Fatalf("first target should be accepted: %#v", accepted)
			}
			second := batchTestSourcedAdaptationItem("second", test.second, "confirmed")
			result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{second}}, batchTestAudit())
			if len(result.Rejected) != 1 || result.Rejected[0].Code != "target_conflict" || result.Rejected[0].Path != test.expectedPath {
				t.Fatalf("second target should be rejected precisely: %#v", result)
			}
		})
	}

	withinItem := []struct {
		name         string
		adaptation   ActorStateSchemaAdaptation
		expectedPath string
	}{
		{
			name: "whole template and field",
			adaptation: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{
				{Op: "remove", TemplateID: "npc"},
				{Op: "fields", TemplateID: "npc", FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "立场", Type: "string"}}}},
			}},
			expectedPath: "items[0].adaptation.template_ops[1].field_ops[0]",
		},
		{
			name: "initial and runtime actor",
			adaptation: ActorStateSchemaAdaptation{
				InitialActorOps: []ActorStateInitialActorSchemaOp{{Op: "replace", ActorID: "guide", Actor: ActorStateInitialActor{ID: "guide", TemplateID: "npc"}}},
				ActorOps:        []ActorStateRuntimeSchemaOp{{Op: "remove", ActorID: "guide"}},
			},
			expectedPath: "items[0].adaptation.actor_ops[0]",
		},
	}
	for _, test := range withinItem {
		t.Run("within item "+test.name, func(t *testing.T) {
			draft := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
			item := batchTestSourcedAdaptationItem("duplicate", test.adaptation, "confirmed")
			result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}}, batchTestAudit())
			if len(result.Rejected) != 1 || result.Rejected[0].Code != "target_conflict" || result.Rejected[0].Path != test.expectedPath {
				t.Fatalf("same-item conflict should be rejected precisely: %#v", result)
			}
		})
	}
}

func TestActorStateSchemaBatchDraftRejectsUnavailableBackendSourceIDs(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		code  string
		audit ActorStateSchemaBatchAudit
	}{
		{name: "opening", kind: "opening", code: "opening_source_not_available", audit: ActorStateSchemaBatchAudit{OpeningSourceIDs: []string{"real-opening"}}},
		{name: "turn result", kind: "turn_result", code: "turn_result_source_not_available", audit: ActorStateSchemaBatchAudit{TurnResultSourceIDs: []string{"real-turn"}}},
		{name: "trpg", kind: "trpg", code: "trpg_source_not_available", audit: ActorStateSchemaBatchAudit{TRPGSourceIDs: []string{"real-rule"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
			item := batchTestCoveredItem("source", "confirmed")
			item.Requirements[0].Source = ActorStateSchemaRequirementSource{Kind: test.kind, ID: "model-invented-id"}
			result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}}, test.audit)
			if len(result.Rejected) != 1 || result.Rejected[0].Code != test.code || result.Rejected[0].Path != "items[0].requirements[0].source.id" {
				t.Fatalf("unavailable source should be rejected precisely: %#v", result)
			}
		})
	}
}

func TestActorStateSchemaBatchDraftRejectsDuplicateIDsWithoutPartialAcceptance(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{
		batchTestCoveredItem("duplicate", "confirmed"),
		batchTestCoveredItem("duplicate", "confirmed"),
	}}, batchTestAudit())
	if len(result.Accepted) != 0 || len(result.Rejected) != 2 || result.DraftAcceptedItems != 0 {
		t.Fatalf("a duplicated ID group must not be both accepted and rejected: %#v", result)
	}
	for _, issue := range result.Rejected {
		if issue.Code != "duplicate_item_id" {
			t.Fatalf("duplicate group returned an unexpected issue: %#v", result)
		}
	}
}

func TestActorStateSchemaBatchItemIDKeepsMigrationSourceIDUnique(t *testing.T) {
	prefix := strings.Repeat("a", maxActorStateSchemaBatchItemIDBytes-1)
	first := batchTestCoveredItem(prefix+"1", "confirmed")
	second := batchTestCoveredItem(prefix+"2", "confirmed")
	firstSource, firstOK := actorStateSchemaBatchItemValueSource(first)
	secondSource, secondOK := actorStateSchemaBatchItemValueSource(second)
	if !firstOK || !secondOK || firstSource.SourceID == secondSource.SourceID || len(firstSource.SourceID) > 128 || len(secondSource.SourceID) > 128 {
		t.Fatalf("maximum-length item IDs must retain unique migration source IDs: first=%#v second=%#v", firstSource, secondSource)
	}
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	tooLong := batchTestCoveredItem(strings.Repeat("b", maxActorStateSchemaBatchItemIDBytes+1), "confirmed")
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{tooLong}}, batchTestAudit())
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "invalid_item_id" {
		t.Fatalf("an item ID that would truncate provenance must be rejected: %#v", result)
	}
}

func TestActorStateSchemaBatchDraftRejectsUnsourcedAndInferredTemplateDefaults(t *testing.T) {
	field := ActorStateField{Name: "隐藏身份", Type: "string", Default: "宗门少主", Visibility: "hidden"}
	unsourced := batchTestCoveredItem("unsourced", "inferred")
	unsourced.Adaptation = ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
		Op: "fields", TemplateID: "protagonist", FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: field}},
	}}}
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{unsourced}}, batchTestAudit())
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "unsourced_adaptation_op" || result.Rejected[0].Path != "items[0].adaptation.template_ops[0].field_ops[0]" {
		t.Fatalf("an extra hidden field must not be smuggled beside another requirement: %#v", result)
	}

	sourced := batchTestSourcedAdaptationItem("inferred-default", unsourced.Adaptation, "inferred")
	sourcedDraft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	result = sourcedDraft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{sourced}}, batchTestAudit())
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "inferred_template_default" || result.Rejected[0].Path != "items[0].adaptation.template_ops[0].field_ops[0].field.default" {
		t.Fatalf("an inferred concrete value must use an Actor op, not a template default: %#v", result)
	}
}

func TestActorStateSchemaBatchDraftInjectsActorValueProvenance(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	item := batchTestItemWithAdaptation("realm", ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{
		Op: "replace", ActorID: "guide",
		Actor:       ActorStateInitialActor{ID: "guide", Name: "向导", TemplateID: "protagonist", State: map[string]any{"状态": "筑基中期"}},
		ValueSource: &ActorStateSchemaActorValueSource{SourceID: "model-fake", ItemID: "fake", EvidenceKind: "default"},
	}}}, "inferred")
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, batchTestAudit())
	if !result.Finalized {
		t.Fatalf("actor value item should finalize: %#v", result)
	}
	proposal, ok := draft.FinalProposal()
	if !ok || len(proposal.Adaptation.ActorOps) != 1 || len(proposal.Requirements) != 1 {
		t.Fatalf("final proposal should contain actor provenance: %#v", proposal)
	}
	source := proposal.Adaptation.ActorOps[0].ValueSource
	if source == nil || source.SourceID != "state_schema_batch:realm" || source.ItemID != "realm" || source.Source.Kind != "opening" || source.Source.ID != "opening-turn" || source.EvidenceKind != "inferred" {
		t.Fatalf("backend must overwrite actor provenance from the accepted requirement: %#v", source)
	}
	if proposal.Requirements[0].ItemID != "realm" {
		t.Fatalf("requirement must be linked to its accepted item: %#v", proposal.Requirements[0])
	}
}

func TestActorStateSchemaBatchDraftRequiresOneActorValueSource(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	item := batchTestItemWithAdaptation("ambiguous", ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{
		Op: "replace", ActorID: "guide", Actor: ActorStateInitialActor{ID: "guide", TemplateID: "protagonist"},
	}}}, "confirmed")
	item.Requirements = append(item.Requirements, ActorStateSchemaRequirementReview{
		Source: ActorStateSchemaRequirementSource{Kind: "trpg", ID: "rule-1"}, Requirement: "规则默认值", EvidenceKind: "default",
		ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: "状态",
	})
	audit := batchTestAudit()
	audit.TRPGSourceIDs = []string{"rule-1"}
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}}, audit)
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "ambiguous_actor_value_source" || result.Rejected[0].Path != "items[0].requirements" {
		t.Fatalf("actor values with mixed sources must be split into items: %#v", result)
	}
}

func TestActorStateSchemaBatchDraftRejectsInferredSecretActorValues(t *testing.T) {
	makeItem := func(evidenceKind string) ActorStateSchemaBatchItem {
		item := batchTestItemWithAdaptation("secret-"+evidenceKind, ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{
			Op: "replace", ActorID: "guide",
			Actor: ActorStateInitialActor{ID: "guide", TemplateID: "protagonist", State: map[string]any{"秘密": "真实身份"}},
		}}}, evidenceKind)
		if evidenceKind == "confirmed" {
			item.Requirements[0].FieldID = "秘密"
		}
		return item
	}
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	rejected := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{makeItem("inferred")}}, batchTestAudit())
	if len(rejected.Rejected) != 1 || rejected.Rejected[0].Code != "inferred_secret_value" || rejected.Rejected[0].Path != "items[0].adaptation.actor_ops[0].actor.state.秘密" {
		t.Fatalf("inferred secret values must be rejected precisely: %#v", rejected)
	}

	confirmedDraft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	confirmed := confirmedDraft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{makeItem("confirmed")}, Finalize: true}, batchTestAudit())
	if !confirmed.Finalized {
		t.Fatalf("confirmed secret values may be accepted: %#v", confirmed)
	}
}

func TestActorStateSchemaBatchDraftRejectsUnsourcedVisibleActorValue(t *testing.T) {
	draft := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{})
	item := batchTestItemWithAdaptation("extra-actor-value", ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{
		Op: "replace", ActorID: "guide",
		Actor: ActorStateInitialActor{ID: "guide", TemplateID: "protagonist", State: map[string]any{"心境": "紧张"}},
	}}}, "confirmed")
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}}, batchTestAudit())
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "unsourced_actor_value" || result.Rejected[0].Path != "items[0].adaptation.actor_ops[0].actor.state.心境" {
		t.Fatalf("an Actor value must be covered by an exact requirement: %#v", result)
	}
}

func batchTestActorStateSystem() StoryDirectorActorStateSystem {
	return StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{ID: "protagonist", Name: "主角", Fields: []ActorStateField{
				{Name: "状态", Type: "string", Default: "平静"},
				{Name: "心境", Type: "string", Default: "稳定"},
				{Name: "秘密", Type: "string", Visibility: "hidden"},
			}},
			{ID: "npc", Name: "NPC", Fields: []ActorStateField{{Name: "态度", Type: "string", Default: "中立"}}},
		},
		InitialActors: []ActorStateInitialActor{{ID: "guide", Name: "向导", TemplateID: "npc"}},
	}
}

func batchTestAudit() ActorStateSchemaBatchAudit {
	return ActorStateSchemaBatchAudit{OpeningSourceIDs: []string{"opening-turn"}}
}

func batchTestItemWithAdaptation(itemID string, adaptation ActorStateSchemaAdaptation, evidenceKind string) ActorStateSchemaBatchItem {
	item := batchTestCoveredItem(itemID, evidenceKind)
	item.Adaptation = adaptation
	return item
}

func batchTestSourcedAdaptationItem(itemID string, adaptation ActorStateSchemaAdaptation, evidenceKind string) ActorStateSchemaBatchItem {
	item := ActorStateSchemaBatchItem{ItemID: itemID, Adaptation: adaptation}
	for _, templateOp := range adaptation.TemplateOps {
		templateID := firstNonEmptyString(templateOp.TemplateID, templateOp.Template.ID)
		if templateOp.Op == "remove" {
			item.Requirements = append(item.Requirements, ActorStateSchemaRequirementReview{
				Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "该模板不应长期追踪",
				EvidenceKind: evidenceKind, Decision: "ignored", TemplateID: templateID, Reason: "开局确认不需要该模板",
			})
			continue
		}
		fieldOps := templateOp.FieldOps
		if templateOp.Op == "add" {
			fieldOps = make([]ActorStateFieldSchemaOp, 0, len(templateOp.Template.Fields))
			for _, field := range templateOp.Template.Fields {
				fieldOps = append(fieldOps, ActorStateFieldSchemaOp{Op: "add", Field: field})
			}
		}
		for _, fieldOp := range fieldOps {
			decision := fieldOp.Op
			fieldID := firstNonEmptyString(fieldOp.Field.Name, fieldOp.FieldID)
			review := ActorStateSchemaRequirementReview{
				Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "该字段需要长期审查",
				EvidenceKind: evidenceKind, Decision: decision, TemplateID: templateID, FieldID: fieldID,
			}
			if fieldOp.Op == "remove" {
				review.Decision = "ignored"
				review.Reason = "开局确认不需要该字段"
			} else {
				review.ExpectedType = fieldOp.Field.Type
			}
			item.Requirements = append(item.Requirements, review)
		}
	}
	appendActorStateRequirements := func(actor ActorStateInitialActor) {
		for fieldID := range actor.State {
			item.Requirements = append(item.Requirements, ActorStateSchemaRequirementReview{
				Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "该 Actor 值需要长期承接",
				EvidenceKind: evidenceKind, ExpectedType: "string", Decision: "covered", TemplateID: actor.TemplateID, FieldID: fieldID,
			})
		}
	}
	for _, actorOp := range adaptation.InitialActorOps {
		appendActorStateRequirements(actorOp.Actor)
	}
	for _, actorOp := range adaptation.ActorOps {
		appendActorStateRequirements(actorOp.Actor)
	}
	if len(item.Requirements) == 0 {
		item.Requirements = batchTestCoveredItem(itemID, evidenceKind).Requirements
	}
	return item
}

func batchTestCoveredItem(itemID, evidenceKind string) ActorStateSchemaBatchItem {
	fieldID := "状态"
	if itemID == "mood" {
		fieldID = "心境"
	}
	return ActorStateSchemaBatchItem{
		ItemID: itemID,
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "长期承接" + fieldID,
			EvidenceKind: evidenceKind, ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: fieldID,
		}},
	}
}
