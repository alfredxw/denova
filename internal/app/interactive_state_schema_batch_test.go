package app

import (
	"context"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestStateSchemaBatchAccumulatesWithoutMutatingStoryUntilFinalizeRunCompletes(t *testing.T) {
	workspace := t.TempDir()
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{
		ID: "life-rule", Type: "rule", Name: "生命规则", LoadMode: book.LoreLoadModeResident, Content: "生命必须使用 0 到 100 的独立数值字段。",
	}); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStoreWithNovaDir(workspace, t.TempDir())
	stateSystem := interactive.StoryDirectorActorStateSystem{
		Templates:     []interactive.ActorStateTemplate{{ID: "protagonist", Name: "主角", Fields: []interactive.ActorStateField{{Name: "状态", Type: "string", Default: "平静"}}}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "Batch 初始化", ActorState: &stateSystem,
		StateSchemaInitialization: &interactive.StateSchemaInitializationStatus{Mode: interactive.StateSchemaAdaptationModeAfterOpening, Status: interactive.StateSchemaInitializationWaitingOpening, BaseRevision: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "醒来", Narrative: "主角带伤醒来。"})
	if err != nil {
		t.Fatal(err)
	}
	minLife, maxLife := 0.0, 100.0
	generator := func(callCtx context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, instruction string) (string, error) {
		if toolContext.MaintenanceTask != "state_schema_initialization" {
			return "maintenance complete", nil
		}
		if toolContext.SubmitStateSchemaBatch == nil {
			t.Fatal("state schema Director must receive the Batch callback")
		}
		if !strings.Contains(toolContext.StableContextTitle, "complete=true") || !strings.Contains(toolContext.StableContext, "生命必须使用 0 到 100") || toolContext.StableContextMaxBytes < len(toolContext.StableContext) {
			t.Fatalf("resident Lore must be supplied as a bounded stable prefix: title=%q max=%d body=%q", toolContext.StableContextTitle, toolContext.StableContextMaxBytes, toolContext.StableContext)
		}
		if strings.Contains(instruction, "生命必须使用 0 到 100") {
			t.Fatal("changing task JSON must not duplicate the stable resident Lore body")
		}
		first, err := toolContext.SubmitStateSchemaBatch(callCtx, interactive.ActorStateSchemaBatch{
			Summary: "逐项审查",
			Items: []interactive.ActorStateSchemaBatchItem{{
				ItemID: "existing-status",
				Requirements: []interactive.ActorStateSchemaRequirementReview{{
					Source: interactive.ActorStateSchemaRequirementSource{Kind: "opening", ID: turn.ID}, Requirement: "承接主角当前状态",
					EvidenceKind: "confirmed", ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: "状态",
					ValuePolicy: interactive.ActorStateSchemaValuePolicyPreserve, ActorID: "protagonist",
				}},
			}},
		})
		if err != nil || len(first.Accepted) != 1 || first.Finalized {
			t.Fatalf("first Batch should only stage its accepted item: result=%#v err=%v", first, err)
		}
		assertStateSchemaRevision(t, store, story.ID, 1, interactive.StateSchemaInitializationRunning)

		second, err := toolContext.SubmitStateSchemaBatch(callCtx, interactive.ActorStateSchemaBatch{
			Items: []interactive.ActorStateSchemaBatchItem{{
				ItemID: "protagonist-life",
				Requirements: []interactive.ActorStateSchemaRequirementReview{{
					Source: interactive.ActorStateSchemaRequirementSource{Kind: "lore", ID: "life-rule"}, Requirement: "生命独立参与结算",
					EvidenceKind: "confirmed", ExpectedType: "number", Min: &minLife, Max: &maxLife, Decision: "add", TemplateID: "protagonist", FieldID: "生命",
					ValuePolicy: interactive.ActorStateSchemaValuePolicyInitialize, ActorID: "protagonist",
				}},
				Adaptation: interactive.ActorStateSchemaAdaptation{
					TemplateOps: []interactive.ActorStateTemplateSchemaOp{{
						Op: "fields", TemplateID: "protagonist", FieldOps: []interactive.ActorStateFieldSchemaOp{{
							Op: "add", Field: interactive.ActorStateField{Name: "生命", Type: "number", Min: &minLife, Max: &maxLife, Visibility: "visible"}, Reason: "常驻生命规则要求独立结算",
						}},
					}},
					ActorOps: []interactive.ActorStateRuntimeSchemaOp{{Op: "set", ActorID: "protagonist", FieldID: "生命", Value: 73}},
				},
			}},
			Finalize: true,
		})
		if err != nil || !second.Finalized || second.DraftAcceptedItems != 2 {
			t.Fatalf("second Batch should finalize the accumulated draft: result=%#v err=%v", second, err)
		}
		// finalize only seals the run-local draft; Store mutation happens after
		// the Director returns successfully.
		assertStateSchemaRevision(t, store, story.ID, 1, interactive.StateSchemaInitializationRunning)
		return "状态结构 Batch 已完成。", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{}).bindDirectorRuntime(newWorkspaceDirectorTaskGroup(), generator)
	<-startInteractiveDirectorMaintenanceTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil, false)
	assertStateSchemaRevision(t, store, story.ID, 2, interactive.StateSchemaInitializationReady)
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	actors, _ := snapshot.State["actors"].(map[string]any)
	actor, _ := actors["protagonist"].(map[string]any)
	state, _ := actor["state"].(map[string]any)
	if state["生命"] != float64(73) {
		t.Fatalf("schema migration must initialize the declared actor field atomically: %#v", state)
	}
}

func assertStateSchemaRevision(t *testing.T, store *interactive.Store, storyID string, revision int, status string) {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil || snapshot.ActorStateSchema.Revision != revision || snapshot.StateSchemaInitialization == nil || snapshot.StateSchemaInitialization.Status != status {
		t.Fatalf("unexpected state schema snapshot: revision=%#v status=%#v", snapshot.ActorStateSchema, snapshot.StateSchemaInitialization)
	}
}
