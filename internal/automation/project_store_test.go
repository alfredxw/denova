package automation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectStoreKeepsAutomationStateOutsideContentDirectory(t *testing.T) {
	userDir := t.TempDir()
	workspace := t.TempDir()
	stateRoot := filepath.Join(userDir, "project-state", "project-one")
	store := NewProjectStore(userDir, "project-one", workspace, stateRoot)
	task, err := store.Create(Task{
		Scope: ScopeWorkspace, Name: "Project task", Template: TemplateCustomPrompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Target.WorkspaceID != "project-one" || task.Target.Workspace != canonicalStoreRoot(workspace) {
		t.Fatalf("automation target should use stable Project identity: %#v", task.Target)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "automations", "tasks.json")); err != nil {
		t.Fatalf("central Project task file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".denova")); !os.IsNotExist(err) {
		t.Fatalf("automation state leaked into content directory: %v", err)
	}

	inbox, err := store.CreateInboxItem(TriggerInboxItem{
		ID: "inbox-one", TaskID: task.ID, TriggerID: "manual", Scope: ScopeWorkspace, Workspace: workspace,
		Status: InboxStatusPending, Purpose: InboxPurposeTrigger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Workspace != canonicalStoreRoot(workspace) {
		t.Fatalf("inbox execution target changed: %#v", inbox)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "automations", "inbox.json")); err != nil {
		t.Fatalf("central Project inbox missing: %v", err)
	}
}

func TestProjectStoreFollowsRelinkWithoutChangingTaskIdentity(t *testing.T) {
	userDir := t.TempDir()
	firstWorkspace := t.TempDir()
	secondWorkspace := t.TempDir()
	stateRoot := filepath.Join(userDir, "project-state", "project-relinked")
	created, err := NewProjectStore(userDir, "project-relinked", firstWorkspace, stateRoot).Create(Task{
		Scope: ScopeWorkspace, Name: "Relinked task", Template: TemplateCustomPrompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := NewProjectStore(userDir, "project-relinked", secondWorkspace, stateRoot).ListInScope(ScopeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task after relink, got %#v", tasks)
	}
	if tasks[0].ID != created.ID || tasks[0].CatalogID != created.CatalogID {
		t.Fatalf("task identity changed after relink: before=%#v after=%#v", created, tasks[0])
	}
	if tasks[0].Target.Workspace != canonicalStoreRoot(secondWorkspace) || tasks[0].Target.WorkspaceID != "project-relinked" {
		t.Fatalf("task did not follow relinked workspace: %#v", tasks[0].Target)
	}
}
