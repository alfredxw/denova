package automation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectStoreKeepsAutomationStateOutsideContentDirectory(t *testing.T) {
	userDir := t.TempDir()
	workspace := t.TempDir()
	stateRoot := filepath.Join(userDir, "stores", "project-one")
	store := NewProjectStore(userDir, "project-one", workspace, stateRoot)
	task, err := store.Create(TaskDefinition{
		Scope: ScopeWorkspace, Name: "Project task", Template: TemplateCustomPrompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Target.ProjectID != "project-one" || task.Target.Workspace != canonicalStoreRoot(workspace) {
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
	stateRoot := filepath.Join(userDir, "stores", "project-relinked")
	created, err := NewProjectStore(userDir, "project-relinked", firstWorkspace, stateRoot).Create(TaskDefinition{
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
	if tasks[0].Target.Workspace != canonicalStoreRoot(secondWorkspace) || tasks[0].Target.ProjectID != "project-relinked" {
		t.Fatalf("task did not follow relinked workspace: %#v", tasks[0].Target)
	}
}

func TestProjectStoreMigratesReleasedPathOnlyInboxRecordsOnRead(t *testing.T) {
	userDir := t.TempDir()
	oldWorkspace := t.TempDir()
	relinkedWorkspace := t.TempDir()
	stateRoot := filepath.Join(userDir, "stores", "project-inbox-migration")
	inboxPath := filepath.Join(stateRoot, "automations", "inbox.json")
	if err := os.MkdirAll(filepath.Dir(inboxPath), 0o755); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	legacy := inboxFile{Items: []TriggerInboxItem{{
		ID:        "legacy-path-only",
		TaskID:    "task-one",
		TriggerID: "schedule",
		Scope:     ScopeWorkspace,
		Workspace: oldWorkspace,
		Status:    InboxStatusPending,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}}}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inboxPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewProjectStore(userDir, "project-inbox-migration", relinkedWorkspace, stateRoot)
	items, err := store.ListInbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one migrated inbox item, got %#v", items)
	}
	if items[0].ProjectID != "project-inbox-migration" || items[0].Workspace != canonicalStoreRoot(relinkedWorkspace) {
		t.Fatalf("released inbox item did not bind to the stable Project: %#v", items[0])
	}

	if _, err := store.MarkInboxItemRead(items[0].ID); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatal(err)
	}
	var migrated inboxFile
	if err := json.Unmarshal(persisted, &migrated); err != nil {
		t.Fatal(err)
	}
	if len(migrated.Items) != 1 || migrated.Items[0].ProjectID != "project-inbox-migration" || migrated.Items[0].Workspace != "" {
		t.Fatalf("mutated inbox record retained host routing instead of only Project identity: %#v", migrated.Items)
	}
}
