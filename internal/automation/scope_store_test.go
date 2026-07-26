package automation

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreScopedUpdateAndDeleteShareAtomicRevisionCheck(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	created, err := store.Create(Task{
		Scope: ScopeWorkspace, Target: ExecutionTarget{Kind: TargetKindWorkspace},
		Name: "Concurrent definition", Template: TemplateCustomPrompt, Prompt: "before",
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make([]error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	run := func(index int, operation func() error) {
		defer wait.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				errs[index] = fmt.Errorf("panic: %v", recovered)
			}
		}()
		<-start
		errs[index] = operation()
	}
	go run(0, func() error {
		_, updateErr := store.UpdateInScopeIfRevision(ScopeWorkspace, created.CatalogID, Task{
			Name: "Updated definition", Prompt: "after",
		}, created.Revision)
		return updateErr
	})
	go run(1, func() error {
		return store.DeleteInScopeIfRevision(ScopeWorkspace, created.CatalogID, created.Revision)
	})
	close(start)
	wait.Wait()

	succeeded := 0
	conflicted := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrRevisionConflict), errors.Is(err, ErrTaskArchived):
			conflicted++
		default:
			t.Fatalf("unexpected concurrent mutation error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("atomic scoped mutation results: succeeded=%d conflicted=%d errors=%v", succeeded, conflicted, errs)
	}
}
