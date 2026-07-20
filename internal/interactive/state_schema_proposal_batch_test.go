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

func TestOpeningActorStateSchemaBatchReturnsExactInitializationGuide(t *testing.T) {
	base := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{ID: DefaultActorID, Fields: []ActorStateField{
				{Name: "身份", Type: "string", Visibility: "visible"},
				{Name: "氧气", Type: "number", Default: 45, Visibility: "visible"},
				{Name: "幕后风险", Type: "string", Visibility: "hidden"},
			}},
			{ID: ActorStateStoryContextTemplateID, Fields: []ActorStateField{{Name: storyContextCurrentEventField, Type: "string", Visibility: "visible"}}},
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, TemplateID: DefaultActorID},
			{ID: DefaultStoryContextActorID, TemplateID: ActorStateStoryContextTemplateID},
		},
	}
	draft := NewOpeningActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
	result := draft.SubmitStructureOnly(ActorStateSchemaBatch{
		Items: []ActorStateSchemaBatchItem{{
			ItemID: "identity-covered",
			Requirements: []ActorStateSchemaRequirementReview{{
				Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-draft"}, Requirement: "主角身份需要长期记录",
				EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicySchemaOnly,
				ExpectedType: "string", Decision: "covered", TemplateID: DefaultActorID, FieldID: "身份",
			}},
			Adaptation: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{}},
		}},
		Finalize: true,
	}, ActorStateSchemaBatchAudit{OpeningSourceIDs: []string{"opening-draft"}})
	if !result.Finalized || result.InitializationGuide == nil {
		t.Fatalf("a finalized opening draft must return its initialization guide: %#v", result)
	}
	guide := result.InitializationGuide
	if guide.TotalWritableFields != 3 || guide.AutoInitializedFields != 1 || len(guide.RequiredStateChanges) != 2 {
		t.Fatalf("unexpected initialization coverage: %#v", guide)
	}
	if first, second := guide.RequiredStateChanges[0], guide.RequiredStateChanges[1]; first.ActorID != DefaultActorID || first.FieldID != "身份" || second.ActorID != DefaultStoryContextActorID || second.FieldID != storyContextCurrentEventField {
		t.Fatalf("the guide must use exact stable actor and field IDs: %#v", guide.RequiredStateChanges)
	}
}

func TestOpeningActorStateSchemaBatchResolvesInitialActorIDAsTemplateAlias(t *testing.T) {
	base := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{ID: DefaultActorID, Fields: []ActorStateField{{Name: "身份", Type: "string", Visibility: "visible"}}},
			{ID: ActorStateStoryContextTemplateID, Fields: []ActorStateField{{Name: storyContextCurrentEventField, Type: "string", Visibility: "visible"}}},
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, TemplateID: DefaultActorID},
			{ID: DefaultStoryContextActorID, TemplateID: ActorStateStoryContextTemplateID},
		},
	}
	draft := NewOpeningActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
	result := draft.SubmitStructureOnly(ActorStateSchemaBatch{
		Items: []ActorStateSchemaBatchItem{{
			ItemID: "story-integrity",
			Requirements: []ActorStateSchemaRequirementReview{{
				Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-draft"}, Requirement: "空间站完整度需要独立变化",
				EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicySchemaOnly,
				ExpectedType: "number", Decision: "add", TemplateID: DefaultStoryContextActorID, FieldID: "站体完整度",
			}},
			Adaptation: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
				Op: "fields", TemplateID: DefaultStoryContextActorID,
				FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "站体完整度", Type: "number", Visibility: "visible"}}},
			}}},
		}},
		Finalize: true,
	}, ActorStateSchemaBatchAudit{OpeningSourceIDs: []string{"opening-draft"}})
	if !result.Finalized || len(result.Rejected) != 0 || len(result.Accepted) != 1 {
		t.Fatalf("an unambiguous initial Actor ID should resolve to its template in the opening contract: %#v", result)
	}
	proposal, ok := draft.FinalProposal()
	if !ok || proposal.Requirements[0].TemplateID != ActorStateStoryContextTemplateID || proposal.Adaptation.TemplateOps[0].TemplateID != ActorStateStoryContextTemplateID {
		t.Fatalf("the finalized proposal must persist canonical template IDs: %#v", proposal)
	}
}

func TestActorStateSchemaBatchDraftRequiresDeclaredActorValueInitialization(t *testing.T) {
	base := batchTestActorStateSystem()
	base.InitialActors = append(base.InitialActors, ActorStateInitialActor{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID})
	draft := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
	item := ActorStateSchemaBatchItem{
		ItemID: "protagonist-realm",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "开局明确主角境界为筑基初期",
			EvidenceKind: "confirmed", ValuePolicy: "initialize", ActorID: DefaultActorID,
			ExpectedType: "string", Decision: "add", TemplateID: DefaultActorID, FieldID: "境界",
		}},
		Adaptation: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "fields", TemplateID: DefaultActorID,
			FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "境界", Type: "string", Visibility: "visible"}}},
		}}},
	}

	audit := batchTestAudit()
	audit.CurrentState = map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{"id": DefaultActorID, "template_id": DefaultActorID, "state": map[string]any{"状态": "平静"}},
	}}
	rejected := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, audit)
	if len(rejected.Rejected) != 1 || rejected.Rejected[0].Code != "missing_actor_value_initialization" || rejected.Rejected[0].Path != "items[0].adaptation.actor_ops" {
		t.Fatalf("a declared concrete value must not finalize without a field-level Actor operation: %#v", rejected)
	}
	if rejected.DraftAcceptedItems != 0 || rejected.Finalized {
		t.Fatalf("a rejected initialization must not enter or finalize the draft: %#v", rejected)
	}

	item.Adaptation.ActorOps = []ActorStateRuntimeSchemaOp{{
		Op: "set", ActorID: DefaultActorID, FieldID: "境界", Value: "筑基初期", Reason: "开局正文明确",
	}}
	accepted := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, audit)
	if !accepted.Finalized || len(accepted.Rejected) != 0 || len(accepted.Accepted) != 1 {
		t.Fatalf("the corrected field-level initialization should finalize independently: %#v", accepted)
	}
	proposal, ok := draft.FinalProposal()
	if !ok || len(proposal.Adaptation.ActorOps) != 1 || proposal.Adaptation.ActorOps[0].ValueSource == nil {
		t.Fatalf("the finalized field initialization must retain backend provenance: %#v", proposal)
	}
}

func TestActorStateSchemaBatchDraftPreservesRenamedActorField(t *testing.T) {
	base := batchTestActorStateSystem()
	base.InitialActors = append(base.InitialActors, ActorStateInitialActor{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID})
	audit := batchTestAudit()
	audit.CurrentState = map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{"id": DefaultActorID, "template_id": DefaultActorID, "state": map[string]any{"状态": "灵基未稳"}},
	}}
	item := ActorStateSchemaBatchItem{
		ItemID: "rename-status",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "用更准确的名称承接现有状态",
			EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicyPreserve, ActorID: DefaultActorID,
			ExpectedType: "string", Decision: "replace", TemplateID: DefaultActorID, FieldID: "当前状态",
		}},
		Adaptation: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "fields", TemplateID: DefaultActorID, FieldOps: []ActorStateFieldSchemaOp{{
				Op: "replace", FieldID: "状态", Field: ActorStateField{Name: "当前状态", Type: "string", Visibility: "visible"},
			}},
		}}},
	}
	result := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{}).Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, audit)
	if !result.Finalized || len(result.Rejected) != 0 {
		t.Fatalf("preserve should validate the source field of a rename migration: %#v", result)
	}
}

func TestActorStateSchemaBatchDraftRejectsActorTemplateMismatch(t *testing.T) {
	audit := batchTestAudit()
	audit.CurrentState = map[string]any{actorStateRoot: map[string]any{
		"guide": map[string]any{"id": "guide", "template_id": "npc", "state": map[string]any{"态度": "中立"}},
	}}
	item := ActorStateSchemaBatchItem{
		ItemID: "wrong-template",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "初始化主角状态",
			EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicyInitialize, ActorID: "guide",
			ExpectedType: "string", Decision: "covered", TemplateID: DefaultActorID, FieldID: "状态",
		}},
		Adaptation: ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{Op: "set", ActorID: "guide", FieldID: "状态", Value: "清醒"}}},
	}
	result := NewActorStateSchemaBatchDraft(batchTestActorStateSystem(), StoryDirectorTRPGSystem{}).Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, audit)
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "actor_template_mismatch" || result.Rejected[0].Path != "items[0].requirements[0].template_id" {
		t.Fatalf("Actor values must not be initialized through a mismatched template requirement: %#v", result)
	}
}

func TestActorStateSchemaBatchDraftDistinguishesPreservedAndDeferredActorValues(t *testing.T) {
	base := batchTestActorStateSystem()
	base.InitialActors = append(base.InitialActors, ActorStateInitialActor{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID})
	currentState := map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{
			"id": DefaultActorID, "template_id": DefaultActorID,
			"state": map[string]any{"状态": "灵基未稳"},
		},
	}}
	audit := batchTestAudit()
	audit.CurrentState = currentState

	preserve := batchTestCoveredItem("preserve-status", "confirmed")
	preserve.Requirements[0].ActorID = DefaultActorID
	preserve.Requirements[0].ValuePolicy = "preserve"
	draft := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{preserve}, Finalize: true}, audit)
	if !result.Finalized {
		t.Fatalf("an existing current value should satisfy preserve: %#v", result)
	}

	missing := batchTestCoveredItem("preserve-missing", "confirmed")
	missing.Requirements[0].FieldID = "心境"
	missing.Requirements[0].ActorID = DefaultActorID
	missing.Requirements[0].ValuePolicy = "preserve"
	missingDraft := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
	result = missingDraft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{missing}, Finalize: true}, audit)
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "actor_value_not_initialized" || result.Rejected[0].Path != "items[0].requirements[0].value_policy" {
		t.Fatalf("preserve must point at a materialized current value: %#v", result)
	}

	deferred := batchTestCoveredItem("deferred-mood", "default")
	deferred.Requirements[0].FieldID = "心境"
	deferred.Requirements[0].ActorID = DefaultActorID
	deferred.Requirements[0].ValuePolicy = "defer"
	deferred.Requirements[0].Reason = "规则没有给出可靠初值"
	deferredDraft := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
	result = deferredDraft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{deferred}, Finalize: true}, audit)
	if !result.Finalized {
		t.Fatalf("an explicitly deferred unknown value should be auditable without inventing a value: %#v", result)
	}
}

func TestActorStateSchemaBatchDraftRejectsBlankActorInitialization(t *testing.T) {
	base := batchTestActorStateSystem()
	audit := batchTestAudit()
	audit.CurrentState = map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{"id": DefaultActorID, "template_id": DefaultActorID, "state": map[string]any{"状态": "平静"}},
	}}
	item := batchTestCoveredItem("blank-status", "confirmed")
	item.Requirements[0].ValuePolicy = ActorStateSchemaValuePolicyInitialize
	item.Requirements[0].ActorID = DefaultActorID
	item.Adaptation.ActorOps = []ActorStateRuntimeSchemaOp{{Op: "set", ActorID: DefaultActorID, FieldID: "状态", Value: "  "}}
	result := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{}).Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, audit)
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "invalid_actor_value" || result.Rejected[0].Path != "items[0].adaptation.actor_ops[0].value" {
		t.Fatalf("blank Actor values must not be accepted as initialization: %#v", result)
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
		ValuePolicy:  ActorStateSchemaValuePolicySchemaOnly,
		ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: "状态",
	})
	audit := batchTestAudit()
	audit.TRPGSourceIDs = []string{"rule-1"}
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}}, audit)
	if len(result.Rejected) != 1 || result.Rejected[0].Code != "ambiguous_actor_value_source" || result.Rejected[0].Path != "items[0].requirements" {
		t.Fatalf("actor values with mixed sources must be split into items: %#v", result)
	}
}

func TestActorStateSchemaBatchDraftAssignsSourcePerFieldSet(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "状态", Type: "string"}, {Name: "心境", Type: "string"}},
	}}}
	draft := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
	item := ActorStateSchemaBatchItem{
		ItemID: "multi-source-fields",
		Requirements: []ActorStateSchemaRequirementReview{
			{Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "承接当前状态", EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicyInitialize, ActorID: "protagonist", ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: "状态"},
			{Source: ActorStateSchemaRequirementSource{Kind: "trpg", ID: "rule-1"}, Requirement: "初始化规则心境", EvidenceKind: "default", ValuePolicy: ActorStateSchemaValuePolicyInitialize, ActorID: "protagonist", ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: "心境"},
		},
		Adaptation: ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{
			{Op: "set", ActorID: "protagonist", FieldID: "状态", Value: "警觉"},
			{Op: "set", ActorID: "protagonist", FieldID: "心境", Value: "稳定"},
		}},
	}
	audit := ActorStateSchemaBatchAudit{
		OpeningSourceIDs: []string{"opening-turn"}, TRPGSourceIDs: []string{"rule-1"},
		CurrentState: map[string]any{"actors": map[string]any{"protagonist": map[string]any{
			"template_id": "protagonist", "state": map[string]any{},
		}}},
	}
	result := draft.Submit(ActorStateSchemaBatch{Items: []ActorStateSchemaBatchItem{item}, Finalize: true}, audit)
	if !result.Finalized || len(result.Rejected) != 0 {
		t.Fatalf("independent field sources should be accepted in one item: %#v", result)
	}
	proposal, ok := draft.FinalProposal()
	if !ok || len(proposal.Adaptation.ActorOps) != 2 {
		t.Fatalf("missing finalized field operations: %#v", proposal)
	}
	if first, second := proposal.Adaptation.ActorOps[0].ValueSource, proposal.Adaptation.ActorOps[1].ValueSource; first == nil || second == nil || first.Source.Kind != "opening" || second.Source.Kind != "trpg" {
		t.Fatalf("backend should bind provenance per field operation: %#v", proposal.Adaptation.ActorOps)
	}
}

func TestActorStateSchemaBatchDraftRejectsInferredSecretActorValues(t *testing.T) {
	makeItem := func(evidenceKind string) ActorStateSchemaBatchItem {
		item := batchTestItemWithAdaptation("secret-"+evidenceKind, ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{
			Op: "replace", ActorID: "guide",
			Actor: ActorStateInitialActor{ID: "guide", TemplateID: "protagonist", State: map[string]any{"秘密": "真实身份"}},
		}}}, evidenceKind)
		item.Requirements[0].FieldID = "秘密"
		item.Requirements[0].ActorID = "guide"
		item.Requirements[0].ValuePolicy = ActorStateSchemaValuePolicyInitialize
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
	for _, op := range adaptation.ActorOps {
		if op.Op == "set" && normalizeActorStateFieldName(op.FieldID) == normalizeActorStateFieldName(item.Requirements[0].FieldID) {
			item.Requirements[0].ValuePolicy = ActorStateSchemaValuePolicyInitialize
			item.Requirements[0].ActorID = firstNonEmptyString(op.ActorID, op.Actor.ID)
			break
		}
		if _, exists := op.Actor.State[item.Requirements[0].FieldID]; exists {
			item.Requirements[0].ValuePolicy = ActorStateSchemaValuePolicyInitialize
			item.Requirements[0].ActorID = firstNonEmptyString(op.ActorID, op.Actor.ID)
			break
		}
	}
	return item
}

func batchTestSourcedAdaptationItem(itemID string, adaptation ActorStateSchemaAdaptation, evidenceKind string) ActorStateSchemaBatchItem {
	item := ActorStateSchemaBatchItem{ItemID: itemID, Adaptation: adaptation}
	for _, templateOp := range adaptation.TemplateOps {
		templateID := firstNonEmptyString(templateOp.TemplateID, templateOp.Template.ID)
		if templateOp.Op == "remove" {
			item.Requirements = append(item.Requirements, ActorStateSchemaRequirementReview{
				Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "该模板不应长期追踪",
				EvidenceKind: evidenceKind, ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, Decision: "ignored", TemplateID: templateID, Reason: "开局确认不需要该模板",
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
				EvidenceKind: evidenceKind, ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, Decision: decision, TemplateID: templateID, FieldID: fieldID,
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
	appendActorStateRequirements := func(actorID string, actor ActorStateInitialActor) {
		for fieldID := range actor.State {
			item.Requirements = append(item.Requirements, ActorStateSchemaRequirementReview{
				Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "该 Actor 值需要长期承接",
				EvidenceKind: evidenceKind, ValuePolicy: ActorStateSchemaValuePolicyInitialize, ActorID: firstNonEmptyString(actorID, actor.ID), ExpectedType: "string", Decision: "covered", TemplateID: actor.TemplateID, FieldID: fieldID,
			})
		}
	}
	for _, actorOp := range adaptation.InitialActorOps {
		appendActorStateRequirements(actorOp.ActorID, actorOp.Actor)
	}
	for _, actorOp := range adaptation.ActorOps {
		appendActorStateRequirements(actorOp.ActorID, actorOp.Actor)
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
			EvidenceKind: evidenceKind, ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: fieldID,
		}},
	}
}
