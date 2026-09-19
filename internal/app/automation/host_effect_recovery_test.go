package automationapp

import (
	"encoding/json"
	"path/filepath"
	"testing"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/automation"
)

type effectRecoveryHost struct {
	Host
	catalog Catalog
}

func (host effectRecoveryHost) Catalog() (Catalog, error) { return host.catalog, nil }

func TestReleasedEffectTransferKeepsOneDurableOwner(t *testing.T) {
	root := t.TempDir()
	location := automation.ProjectLocation{ProjectID: "project-one", Workspace: filepath.Join(root, "workspace"), StateRoot: filepath.Join(root, "store")}
	service := NewService(effectRecoveryHost{catalog: Catalog{DataDir: root, CurrentWorkspace: location.Workspace, Projects: []automation.ProjectLocation{location}}})
	store := service.storeAllWorkspaces()
	task, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Target: automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, ProjectID: location.ProjectID, Workspace: location.Workspace}, Name: "Legacy trigger", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	legacy := automation.RunRecord{ID: "legacy-run", TaskID: task.ID, Scope: task.Scope, ProjectID: location.ProjectID, Trigger: automation.TriggerManual,
		Status: automation.RunStatusRunning, CompletionEffectsPending: true, CompletionEffectsOperationID: "operation-one",
		CompletionMutationPaths: []string{"chapters/one.md"}, CompletionMutationEffectIDs: []string{"effect-one"}}
	if _, err := store.AppendRun(task.CatalogID, legacy); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(admittedToolMutationPayload{Version: admittedToolMutationVersion, RuntimeOperation: "operation-one", ToolCallID: "write-one",
		Origin:   agenttoolruntime.ToolMutationOrigin{AutomationTaskID: task.ID, TaskID: legacy.ID},
		Mutation: agenttool.Mutation{ToolName: "write", ToolCallID: "write-one", Target: "chapters/one.md"}})
	if err != nil {
		t.Fatal(err)
	}
	effect, err := store.AdmitHostEffect(t.Context(), automation.HostEffectObligation{ID: "effect-one", Kind: agentrun.HostEffectToolMutationCommitted, ProjectID: location.ProjectID, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	// Repeating reconciliation after the old transfer must only acknowledge the
	// global outbox. Runtime admission and trigger routing are deliberately absent.
	for range 2 {
		if acknowledged, err := service.reconcilePersistedHostEffect(t.Context(), effect); err != nil || !acknowledged {
			t.Fatalf("legacy transfer acknowledgement = %v, error = %v", acknowledged, err)
		}
	}
	remaining, err := store.ListHostEffects()
	if err != nil || len(remaining) != 0 {
		t.Fatalf("global obligations = %+v, error = %v", remaining, err)
	}
	_, retained, err := store.GetRunByID(legacy.ID)
	if err != nil || !retained.CompletionEffectsPending || len(retained.CompletionMutationPaths) != 1 {
		t.Fatalf("legacy owner lost its obligation: %+v, error = %v", retained, err)
	}
}
