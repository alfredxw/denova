package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSkillRootRevisionIncludesSupportingFiles(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}
	writeSkillFile(t, user, "research", "research", "Research sources.")
	refPath := filepath.Join(user, "research", "references", "sources.md")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refPath, []byte("# Original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := ReadDocument(ctx, dirs, ScopeUser, "research")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refPath, []byte("# Changed externally\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := ReadDocument(ctx, dirs, ScopeUser, "research")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision == root.Revision {
		t.Fatal("supporting-file change did not advance the Skill root revision")
	}

	if _, err := SaveDocumentIfRevision(ctx, dirs, ScopeUser, "research", DefaultContent("research", "stale root edit"), root.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale root save error = %v, want ErrRevisionConflict", err)
	}
	if _, err := DeleteDocumentIfRevision(ctx, dirs, ScopeUser, "research", root.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale root delete error = %v, want ErrRevisionConflict", err)
	}
	if _, err := os.Stat(refPath); err != nil {
		t.Fatalf("stale root delete removed the changed Skill: %v", err)
	}
	deleted, err := DeleteDocumentIfRevision(ctx, dirs, ScopeUser, "research", changed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Revision != changed.Revision {
		t.Fatalf("delete receipt revision = %q, want %q", deleted.Revision, changed.Revision)
	}
	if _, err := os.Stat(filepath.Join(user, "research")); !os.IsNotExist(err) {
		t.Fatalf("current root delete left the Skill directory behind: %v", err)
	}
}

func TestConcurrentSkillRootDeleteAndReferenceUpdateHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}

	for iteration := 0; iteration < 12; iteration++ {
		name := fmt.Sprintf("research-%d", iteration)
		writeSkillFile(t, user, name, name, "Research sources.")
		created, err := CreateSkillFile(ctx, dirs, ScopeUser, name, "references/sources.md", "# Original\n")
		if err != nil {
			t.Fatal(err)
		}
		root, err := ReadDocument(ctx, dirs, ScopeUser, name)
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		errorsByOperation := make(chan error, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		go func() {
			ready.Done()
			<-start
			_, deleteErr := DeleteDocumentIfRevision(ctx, dirs, ScopeUser, name, root.Revision)
			errorsByOperation <- deleteErr
		}()
		go func() {
			ready.Done()
			<-start
			_, updateErr := SaveSkillFileIfRevision(ctx, dirs, ScopeUser, name, "references/sources.md", "# Updated\n", created.Revision)
			errorsByOperation <- updateErr
		}()
		ready.Wait()
		close(start)

		successes := 0
		for count := 0; count < 2; count++ {
			if err := <-errorsByOperation; err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("iteration %d committed %d concurrent root/reference mutations, want exactly 1", iteration, successes)
		}
	}
}
