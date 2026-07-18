package automation

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestUserCatalogListsTasksFromEveryWorkspaceWithExplicitTargets(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaceA := filepath.Join(root, "book-a")
	workspaceB := filepath.Join(root, "book-b")

	taskA, err := NewStore(userDir, workspaceA).Create(Task{
		Scope:    ScopeWorkspace,
		Name:     "Review A",
		Template: TemplateReview,
	})
	if err != nil {
		t.Fatalf("create workspace A task: %v", err)
	}
	taskB, err := NewStore(userDir, workspaceB).Create(Task{
		Scope:    ScopeWorkspace,
		Name:     "Review B",
		Template: TemplateReview,
	})
	if err != nil {
		t.Fatalf("create workspace B task: %v", err)
	}

	catalog := NewStore(userDir, "").WithWorkspaces(workspaceA, workspaceB)
	tasks, err := catalog.List()
	if err != nil {
		t.Fatalf("list user automation catalog: %v", err)
	}

	catalogIDs := map[string]bool{}
	for _, task := range tasks {
		if task.CatalogID == "" {
			t.Fatalf("task %s has no catalog id", task.ID)
		}
		if catalogIDs[task.CatalogID] {
			t.Fatalf("duplicate catalog id %q", task.CatalogID)
		}
		catalogIDs[task.CatalogID] = true
	}

	wantTargets := map[string]string{
		taskA.ID: canonicalStoreRoot(workspaceA),
		taskB.ID: canonicalStoreRoot(workspaceB),
	}
	for _, task := range tasks {
		wantWorkspace, ok := wantTargets[task.ID]
		if !ok {
			continue
		}
		if task.Target.Kind != TargetKindWorkspace {
			t.Fatalf("task %s target kind = %q, want workspace", task.ID, task.Target.Kind)
		}
		if got := canonicalStoreRoot(task.Target.Workspace); got != wantWorkspace {
			t.Fatalf("task %s target workspace = %q, want %q", task.ID, got, wantWorkspace)
		}
		delete(wantTargets, task.ID)
	}
	if len(wantTargets) != 0 {
		t.Fatalf("catalog omitted workspace tasks: %#v", wantTargets)
	}
}

func TestUserCatalogSelectsOnlyTasksForOneExecutionTarget(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaceA := filepath.Join(root, "book-a")
	workspaceB := filepath.Join(root, "book-b")
	storeA := NewStore(userDir, workspaceA)
	if _, err := storeA.Create(Task{Scope: ScopeWorkspace, Name: "A", Template: TemplateReview}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(userDir, workspaceB).Create(Task{Scope: ScopeWorkspace, Name: "B", Template: TemplateReview}); err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.Create(Task{Scope: ScopeUser, Target: ExecutionTarget{Kind: TargetKindUser}, Name: "Global", Template: TemplateCustomPrompt}); err != nil {
		t.Fatal(err)
	}

	catalog := NewStore(userDir, "").WithWorkspaces(workspaceA, workspaceB)
	tasks, err := catalog.ListForTarget(ExecutionTarget{Kind: TargetKindWorkspace, Workspace: workspaceB})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Target.Kind != TargetKindWorkspace || canonicalStoreRoot(task.Target.Workspace) != canonicalStoreRoot(workspaceB) {
			t.Fatalf("workspace B target included foreign task: %#v", task)
		}
	}
	if !hasTaskNamed(tasks, "B") || hasTaskNamed(tasks, "A") || hasTaskNamed(tasks, "Global") {
		t.Fatalf("workspace target tasks = %#v", tasks)
	}

	global, err := catalog.ListForTarget(ExecutionTarget{Kind: TargetKindUser})
	if err != nil {
		t.Fatal(err)
	}
	if len(global) != 1 || global[0].Name != "Global" {
		t.Fatalf("global target tasks = %#v", global)
	}
}

func hasTaskNamed(tasks []Task, name string) bool {
	for _, task := range tasks {
		if task.Name == name {
			return true
		}
	}
	return false
}

func TestUserCatalogListsInboxItemsAcrossWorkspaces(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaceA := filepath.Join(root, "book-a")
	workspaceB := filepath.Join(root, "book-b")
	for index, workspace := range []string{workspaceA, workspaceB} {
		_, err := NewStore(userDir, workspace).CreateInboxItem(TriggerInboxItem{
			TaskID:       "task",
			TriggerID:    "trigger",
			Scope:        ScopeWorkspace,
			Workspace:    workspace,
			ActionPolicy: ActionPolicyConfirm,
			NotifyPolicy: NotifyPolicyInbox,
			Title:        string(rune('A' + index)),
			Fingerprint:  string(rune('a' + index)),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	items, err := NewStore(userDir, "").WithWorkspaces(workspaceA, workspaceB).ListInbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("global inbox item count = %d, want 2: %#v", len(items), items)
	}
}

func TestGlobalAutomationRejectsWorkspaceContentTriggers(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "user"), "")
	_, err := store.Create(Task{
		Target:   ExecutionTarget{Kind: TargetKindUser},
		Name:     "Global research",
		Template: TemplateCustomPrompt,
		Triggers: []TriggerDefinition{{Type: TriggerTypeSemantic, Enabled: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "global automation") {
		t.Fatalf("Create error = %v, want global automation trigger validation", err)
	}
}
