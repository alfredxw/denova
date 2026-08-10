package automation

import (
	"path/filepath"
	"testing"
)

func TestUserCatalogListsTasksFromEveryWorkspaceWithExplicitTargets(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaceA := filepath.Join(root, "book-a")
	workspaceB := filepath.Join(root, "book-b")

	taskA, err := NewStore(userDir, workspaceA).Create(TaskDefinition{
		Scope:    ScopeWorkspace,
		Name:     "Review A",
		Template: TemplateReview,
	})
	if err != nil {
		t.Fatalf("create workspace A task: %v", err)
	}
	taskB, err := NewStore(userDir, workspaceB).Create(TaskDefinition{
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

func TestCatalogLocatorsDisambiguateImportedTaskIDsAcrossWorkspaces(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaceA := filepath.Join(root, "book-a")
	workspaceB := filepath.Join(root, "book-b")
	storeA := NewStore(userDir, workspaceA)
	storeB := NewStore(userDir, workspaceB)
	taskA, err := storeA.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "A", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeB.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "B", Template: TemplateReview}); err != nil {
		t.Fatal(err)
	}
	// Imported workspace data can legitimately preserve a task's compact ID.
	// Reproduce that collision without relying on random ID generation.
	tasksB, err := storeB.readScope(ScopeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	tasksB[0].ID = taskA.ID
	if err := storeB.writeScope(ScopeWorkspace, tasksB); err != nil {
		t.Fatal(err)
	}

	locatorA := CatalogTaskID(ScopeWorkspace, workspaceA, taskA.ID)
	locatorB := CatalogTaskID(ScopeWorkspace, workspaceB, taskA.ID)
	if locatorA == locatorB {
		t.Fatalf("workspace catalog locators collided: %q", locatorA)
	}
	catalog := NewStore(userDir, "").WithWorkspaces(workspaceA, workspaceB)
	resolvedA, err := catalog.Get(locatorA)
	if err != nil {
		t.Fatal(err)
	}
	resolvedB, err := catalog.Get(locatorB)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedA.Name != "A" || resolvedB.Name != "B" || resolvedA.ID != resolvedB.ID {
		t.Fatalf("catalog collision resolution A=%#v B=%#v", resolvedA, resolvedB)
	}
	if _, err := catalog.AppendRun(locatorA, RunRecord{ID: "run-a", TaskID: taskA.ID, Scope: ScopeWorkspace, Workspace: workspaceA, Trigger: TriggerManual, Status: RunStatusSuccess}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendRun(locatorB, RunRecord{ID: "run-b", TaskID: taskA.ID, Scope: ScopeWorkspace, Workspace: workspaceB, Trigger: TriggerManual, Status: RunStatusSuccess}); err != nil {
		t.Fatal(err)
	}
	latestA, err := storeA.Get(locatorA)
	if err != nil {
		t.Fatal(err)
	}
	latestB, err := storeB.Get(locatorB)
	if err != nil {
		t.Fatal(err)
	}
	if len(latestA.RecentRuns) != 1 || latestA.RecentRuns[0].ID != "run-a" ||
		len(latestB.RecentRuns) != 1 || latestB.RecentRuns[0].ID != "run-b" {
		t.Fatalf("catalog run writes crossed workspaces: A=%#v B=%#v", latestA.RecentRuns, latestB.RecentRuns)
	}
}

func TestUserCatalogSelectsOnlyTasksForOneExecutionTarget(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaceA := filepath.Join(root, "book-a")
	workspaceB := filepath.Join(root, "book-b")
	storeA := NewStore(userDir, workspaceA)
	if _, err := storeA.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "A", Template: TemplateReview}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(userDir, workspaceB).Create(TaskDefinition{Scope: ScopeWorkspace, Name: "B", Template: TemplateReview}); err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.Create(TaskDefinition{Scope: ScopeUser, Target: ExecutionTarget{Kind: TargetKindUser}, Name: "Global", Template: TemplateCustomPrompt}); err != nil {
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

func TestLegacyGlobalAutomationAllowsContentTriggers(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "user"), "")
	task, err := store.Create(TaskDefinition{
		Target:   ExecutionTarget{Kind: TargetKindUser},
		Name:     "Global research",
		Template: TemplateCustomPrompt,
		// Legacy user definitions remain readable so their trigger state can be
		// retired without corrupting the catalog.
		Triggers: []TriggerDefinition{{Type: TriggerTypeSemantic, Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Create error = %v, want user-scope content trigger accepted", err)
	}
	if task.Scope != ScopeUser {
		t.Fatalf("scope = %q, want user", task.Scope)
	}
	// The user-scope task is not part of any single workspace's exclusive list.
	workspace := filepath.Join(t.TempDir(), "book")
	wsStore := NewStore(store.userDir, workspace)
	if tasks, err := wsStore.ListForTarget(ExecutionTarget{Kind: TargetKindWorkspace, Workspace: workspace}); err != nil {
		t.Fatal(err)
	} else if hasTaskNamed(tasks, "Global research") {
		t.Fatalf("workspace target must not include user-scope task: %#v", tasks)
	}
	// ... but it IS included when evaluating triggers against a workspace.
	if tasks, err := wsStore.ListForTriggerEvaluation(ExecutionTarget{Kind: TargetKindWorkspace, Workspace: workspace}); err != nil {
		t.Fatal(err)
	} else if !hasTaskNamed(tasks, "Global research") {
		t.Fatalf("trigger evaluation must include user-scope task: %#v", tasks)
	}
}
