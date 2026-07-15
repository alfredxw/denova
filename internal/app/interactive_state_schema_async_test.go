package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestOpeningTurnStaysVisibleWhileStateSchemaInitializesBeforeMaintenance(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStoreWithNovaDir(workspace, t.TempDir())
	stateSystem := interactive.StoryDirectorActorStateSystem{
		Templates:     []interactive.ActorStateTemplate{{ID: "protagonist", Name: "主角", Fields: []interactive.ActorStateField{{Name: "状态", Type: "string", Default: "平静"}}}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:      "异步开局",
		ActorState: &stateSystem,
		StateSchemaInitialization: &interactive.StateSchemaInitializationStatus{
			Mode: interactive.StateSchemaAdaptationModeAfterOpening, Status: interactive.StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "推门", Narrative: "门外是正在燃烧的长街。"})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var tasks []string
	minPressure, maxPressure := 0.0, 10.0
	generator := func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		tasks = append(tasks, toolContext.MaintenanceTask)
		if toolContext.MaintenanceTask == "state_schema_initialization" {
			once.Do(func() { close(started) })
			<-release
			if toolContext.SubmitStateSchemaBatch == nil {
				t.Fatal("state schema Director must receive the Batch submit callback")
			}
			result, err := toolContext.SubmitStateSchemaBatch(context.Background(), stateSchemaBatchFromProposal("crisis-pressure", interactive.ActorStateSchemaProposal{
				Summary: "补充危机压力",
				Requirements: []interactive.ActorStateSchemaRequirementReview{{
					Source:       interactive.ActorStateSchemaRequirementSource{Kind: "opening", ID: turn.ID},
					Requirement:  "燃烧长街形成可持续的危机压力",
					EvidenceKind: "confirmed",
					ExpectedType: "number",
					Decision:     "add",
					TemplateID:   "protagonist",
					FieldID:      "危机压力",
					ValuePolicy:  interactive.ActorStateSchemaValuePolicySchemaOnly,
					Reason:       "首轮明确建立持续危机",
				}},
				Adaptation: interactive.ActorStateSchemaAdaptation{TemplateOps: []interactive.ActorStateTemplateSchemaOp{{
					Op: "fields", TemplateID: "protagonist", FieldOps: []interactive.ActorStateFieldSchemaOp{{
						Op: "add", Field: interactive.ActorStateField{Name: "危机压力", Type: "number", Default: 1, Min: &minPressure, Max: &maxPressure, Visibility: "visible"}, Reason: "首轮出现燃烧街道",
					}},
				}}},
			}))
			if err != nil || !result.Finalized {
				t.Fatalf("submit state schema Batch: result=%#v err=%v", result, err)
			}
			return "状态结构提案已提交。", nil
		}
		return "maintenance complete", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{}).bindDirectorRuntime(newWorkspaceDirectorTaskGroup(), generator)
	done := startInteractiveDirectorMaintenanceTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil, false)
	<-started
	running, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if running.CurrentTurn == nil || running.CurrentTurn.Narrative != turn.Narrative || running.StateSchemaInitialization == nil || running.StateSchemaInitialization.Status != interactive.StateSchemaInitializationRunning {
		t.Fatalf("opening must stay visible while schema adapts: %#v", running)
	}
	if _, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID}); err == nil {
		t.Fatal("branch creation must wait until state schema migration completes")
	}
	close(release)
	<-done
	completed, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if completed.ActorStateSchema == nil || completed.ActorStateSchema.Revision != 2 || completed.StateSchemaInitialization == nil || completed.StateSchemaInitialization.Status != interactive.StateSchemaInitializationReady {
		t.Fatalf("schema initialization did not complete: %#v", completed.StateSchemaInitialization)
	}
	if len(tasks) == 0 || tasks[0] != "state_schema_initialization" {
		t.Fatalf("state schema must run before other maintenance: %#v", tasks)
	}
}

func TestStateSchemaInitializationRejectsLoreRevisionChangedDuringDirectorReview(t *testing.T) {
	workspace := t.TempDir()
	lore := book.NewLoreStore(workspace)
	if _, err := lore.Create(book.LoreItemInput{
		ID:       "state-rule",
		Type:     "rule",
		Name:     "状态规则",
		LoadMode: book.LoreLoadModeResident,
		Content:  "主角状态需要长期追踪。",
	}); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStoreWithNovaDir(workspace, t.TempDir())
	stateSystem := interactive.StoryDirectorActorStateSystem{
		Templates:     []interactive.ActorStateTemplate{{ID: "protagonist", Name: "主角", Fields: []interactive.ActorStateField{{Name: "状态", Type: "string", Default: "平静"}}}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:      "资料并发变更",
		ActorState: &stateSystem,
		StateSchemaInitialization: &interactive.StateSchemaInitializationStatus{
			Mode: interactive.StateSchemaAdaptationModeAfterOpening, Status: interactive.StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "醒来", Narrative: "主角从梦中醒来。"})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	generator := func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		if toolContext.MaintenanceTask != "state_schema_initialization" {
			return "maintenance complete", nil
		}
		close(started)
		<-release
		result, err := toolContext.SubmitStateSchemaBatch(context.Background(), stateSchemaBatchFromProposal("covered-state", interactive.ActorStateSchemaProposal{
			Summary: "现有状态字段已覆盖规则",
			Requirements: []interactive.ActorStateSchemaRequirementReview{{
				Source:       interactive.ActorStateSchemaRequirementSource{Kind: "opening", ID: turn.ID},
				Requirement:  "长期追踪主角状态",
				EvidenceKind: "confirmed",
				ExpectedType: "string",
				Decision:     "covered",
				TemplateID:   "protagonist",
				FieldID:      "状态",
				ValuePolicy:  interactive.ActorStateSchemaValuePolicySchemaOnly,
			}},
			Adaptation: interactive.ActorStateSchemaAdaptation{},
		}))
		if err != nil || !result.Finalized {
			return "", fmt.Errorf("状态结构 Batch 未完成: result=%#v err=%v", result, err)
		}
		return "状态结构提案已提交。", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{}).bindDirectorRuntime(newWorkspaceDirectorTaskGroup(), generator)
	done := startInteractiveDirectorMaintenanceTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil, false)
	<-started
	if _, err := lore.Create(book.LoreItemInput{ID: "new-rule", Type: "rule", Name: "新增规则", LoadMode: book.LoreLoadModeResident, Content: "新增规则正文"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	status := snapshot.StateSchemaInitialization
	if status == nil || status.Status != interactive.StateSchemaInitializationFailed || !strings.Contains(status.Error, "资料库") {
		t.Fatalf("stale lore review should fail instead of applying: %#v", status)
	}
	if snapshot.ActorStateSchema == nil || snapshot.ActorStateSchema.Revision != 1 {
		t.Fatalf("stale review must not advance schema revision: %#v", snapshot.ActorStateSchema)
	}
}

func TestStateSchemaInitializationRejectsLoreRequirementThatWasNotRead(t *testing.T) {
	workspace := t.TempDir()
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{
		ID: "numeric-rule", Type: "rule", Name: "数值规则", LoadMode: book.LoreLoadModeAuto, Content: "生命值范围为 0 到 100。",
	}); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStoreWithNovaDir(workspace, t.TempDir())
	stateSystem := interactive.StoryDirectorActorStateSystem{
		Templates:     []interactive.ActorStateTemplate{{ID: "protagonist", Name: "主角", Fields: []interactive.ActorStateField{{Name: "状态", Type: "string", Default: "平静"}}}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:      "未读资料审查",
		ActorState: &stateSystem,
		StateSchemaInitialization: &interactive.StateSchemaInitializationStatus{
			Mode: interactive.StateSchemaAdaptationModeAfterOpening, Status: interactive.StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "醒来", Narrative: "主角醒来。"})
	if err != nil {
		t.Fatal(err)
	}
	generator := func(callCtx context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		if toolContext.MaintenanceTask != "state_schema_initialization" {
			return "maintenance complete", nil
		}
		result, err := toolContext.SubmitStateSchemaBatch(callCtx, stateSchemaBatchFromProposal("unread-lore", interactive.ActorStateSchemaProposal{
			Summary: "声称资料已覆盖",
			Requirements: []interactive.ActorStateSchemaRequirementReview{{
				Source: interactive.ActorStateSchemaRequirementSource{Kind: "lore", ID: "numeric-rule"}, Requirement: "长期状态规则", EvidenceKind: "confirmed", ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: "状态", ValuePolicy: interactive.ActorStateSchemaValuePolicySchemaOnly,
			}},
			Adaptation: interactive.ActorStateSchemaAdaptation{},
		}))
		if err != nil || !result.Finalized {
			return "", fmt.Errorf("状态结构 Batch 未完成: result=%#v err=%v", result, err)
		}
		return "状态结构提案已提交。", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{}).bindDirectorRuntime(newWorkspaceDirectorTaskGroup(), generator)
	<-startInteractiveDirectorMaintenanceTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil, false)

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	status := snapshot.StateSchemaInitialization
	if status == nil || status.Status != interactive.StateSchemaInitializationFailed || !strings.Contains(status.Error, "lore_not_reviewed") {
		t.Fatalf("unread lore requirement should fail review: %#v", status)
	}
}

func TestStateSchemaInitializationKeepsFinalizedProposalAfterLaterSubmitFails(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStoreWithNovaDir(workspace, t.TempDir())
	stateSystem := interactive.StoryDirectorActorStateSystem{
		Templates:     []interactive.ActorStateTemplate{{ID: "protagonist", Name: "主角", Fields: []interactive.ActorStateField{{Name: "状态", Type: "string", Default: "平静"}}}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:      "后续无效提案",
		ActorState: &stateSystem,
		StateSchemaInitialization: &interactive.StateSchemaInitializationStatus{
			Mode: interactive.StateSchemaAdaptationModeAfterOpening, Status: interactive.StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "醒来", Narrative: "主角醒来。"})
	if err != nil {
		t.Fatal(err)
	}
	generator := func(callCtx context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		if toolContext.MaintenanceTask != "state_schema_initialization" {
			return "maintenance complete", nil
		}
		first, err := toolContext.SubmitStateSchemaBatch(callCtx, stateSchemaBatchFromProposal("covered-state", interactive.ActorStateSchemaProposal{
			Summary: "现有字段已覆盖",
			Requirements: []interactive.ActorStateSchemaRequirementReview{{
				Source: interactive.ActorStateSchemaRequirementSource{Kind: "opening", ID: turn.ID}, Requirement: "长期追踪主角状态", EvidenceKind: "confirmed", ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: "状态", ValuePolicy: interactive.ActorStateSchemaValuePolicySchemaOnly,
			}},
			Adaptation: interactive.ActorStateSchemaAdaptation{},
		}))
		if err != nil || !first.Finalized {
			return "", fmt.Errorf("有效 Batch 未完成: result=%#v err=%v", first, err)
		}
		later, err := toolContext.SubmitStateSchemaBatch(callCtx, stateSchemaBatchFromProposal("missing-review", interactive.ActorStateSchemaProposal{Summary: "缺少覆盖审查"}))
		if err == nil && len(later.Rejected) == 0 && len(later.Blocked) == 0 {
			return "", fmt.Errorf("expected the later proposal to fail validation")
		}
		return "后续提案未通过。", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{}).bindDirectorRuntime(newWorkspaceDirectorTaskGroup(), generator)
	<-startInteractiveDirectorMaintenanceTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil, false)

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.StateSchemaInitialization == nil || snapshot.StateSchemaInitialization.Status != interactive.StateSchemaInitializationReady {
		t.Fatalf("later failed submit must preserve the finalized proposal: %#v", snapshot.StateSchemaInitialization)
	}
	if snapshot.ActorStateSchema == nil || snapshot.ActorStateSchema.Revision != 1 {
		t.Fatalf("older proposal must not be applied after a later submit failure: %#v", snapshot.ActorStateSchema)
	}
}

func stateSchemaBatchFromProposal(itemID string, proposal interactive.ActorStateSchemaProposal) interactive.ActorStateSchemaBatch {
	return interactive.ActorStateSchemaBatch{
		Summary: proposal.Summary,
		Items: []interactive.ActorStateSchemaBatchItem{{
			ItemID:       itemID,
			Summary:      proposal.Summary,
			Requirements: proposal.Requirements,
			Adaptation:   proposal.Adaptation,
		}},
		Finalize: true,
	}
}
