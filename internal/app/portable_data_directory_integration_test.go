package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/attachment"
	"denova/internal/automation"
	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
)

func TestManagedDataDirectoryRunsAfterCopyingToAnotherRoot(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	firstRoot := filepath.Join(parent, "denova-first")
	secondRoot := filepath.Join(parent, "denova-second")
	firstApp, err := New(ctx, &config.Config{
		OpenAIModel: "test-model", DenovaDir: firstRoot, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := firstApp.CreateBook(ctx, firstRoot, "Portable Book", "Writer", "Move test")
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	projectID := created.ProjectID
	record, err := firstApp.projectRegistry.Get(projectID)
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	firstLayout, err := firstApp.projectRegistry.Layout(record)
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}

	projectRuntime, err := firstApp.AgentChat().ProjectRuntime(ctx, projectID)
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	sess, err := projectRuntime.SessionStore.GetOrCreate("portable-session")
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	attachments, err := attachment.Materialize(
		firstLayout.StoreRoot,
		attachment.SessionScope(sess.ID),
		"portable-command",
		[]attachment.Upload{{
			Name: "note.txt", MediaType: "text/plain",
			DataURL: "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("portable attachment")),
		}},
	)
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessageWithAttachments("Keep this conversation", attachments)); err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	createdTask, err := automation.NewProjectStore(
		firstRoot, projectID, firstLayout.ContentRoot, firstLayout.StoreRoot,
	).Create(automation.TaskDefinition{
		Scope: automation.ScopeWorkspace, Name: "Portable task", Template: automation.TemplateCustomPrompt,
	})
	if err != nil {
		firstApp.Close()
		t.Fatal(err)
	}
	firstApp.Close()

	// A byte-for-byte copy models a fully stopped process moving the tree. On
	// Windows, in-process filesystem watchers can retain OS handles briefly even
	// after Close; a real process exit releases them before the user moves it.
	if err := copyPortableTestTree(firstRoot, secondRoot); err != nil {
		t.Fatal(err)
	}
	secondApp, err := New(ctx, &config.Config{
		OpenAIModel: "test-model", DenovaDir: secondRoot, ResumeLastWorkspace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondApp.Close)
	if secondApp.ProjectID() != projectID {
		t.Fatalf("Project identity changed after move: got=%q want=%q", secondApp.ProjectID(), projectID)
	}
	wantWorkspace := filepath.Join(secondRoot, "projects", "Portable Book")
	canonicalWorkspace, err := filepath.EvalSymlinks(wantWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if secondApp.Workspace() != canonicalWorkspace {
		t.Fatalf("runtime Project root=%q want=%q", secondApp.Workspace(), wantWorkspace)
	}

	secondRecord, err := secondApp.projectRegistry.Get(projectID)
	if err != nil {
		t.Fatal(err)
	}
	secondLayout, err := secondApp.projectRegistry.Layout(secondRecord)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := secondApp.bookMetaStore.Read(secondLayout.ContentRoot, secondLayout.StoreRoot)
	if err != nil || meta.Title != "Portable Book" || meta.Author != "Writer" {
		t.Fatalf("moved Book metadata=%#v err=%v", meta, err)
	}
	secondRuntime, err := secondApp.AgentChat().ProjectRuntime(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := secondRuntime.SessionStore.Get("portable-session")
	if err != nil {
		t.Fatal(err)
	}
	history := reopened.History()
	if len(history) != 1 || history[0].Content != "Keep this conversation" || len(history[0].Attachments) != 1 {
		t.Fatalf("moved Session history=%#v", history)
	}
	attachmentPath := filepath.Join(secondLayout.StoreRoot, filepath.FromSlash(history[0].Attachments[0].Path))
	if content, err := os.ReadFile(attachmentPath); err != nil || string(content) != "portable attachment" {
		t.Fatalf("moved attachment=%q err=%v", content, err)
	}
	tasks, err := automation.NewProjectStore(
		secondRoot, projectID, secondLayout.ContentRoot, secondLayout.StoreRoot,
	).ListInScope(automation.ScopeWorkspace)
	if err != nil || len(tasks) != 1 || tasks[0].ID != createdTask.ID || tasks[0].Target.Workspace != canonicalWorkspace {
		t.Fatalf("moved automation tasks=%#v err=%v", tasks, err)
	}
	storyStore := interactive.NewStore(secondLayout.ContentRoot)
	t.Cleanup(func() { _ = storyStore.Close() })
	storyIndex, err := storyStore.Index()
	if err != nil || len(storyIndex.Stories) != 1 {
		t.Fatalf("moved Game state=%#v err=%v", storyIndex, err)
	}

	assertTreeOmitsRuntimeRoot(t, secondRoot, firstRoot)
}

func copyPortableTestTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode().Perm())
	})
}

func assertTreeOmitsRuntimeRoot(t *testing.T, treeRoot, obsoleteRoot string) {
	t.Helper()
	needles := [][]byte{
		[]byte(obsoleteRoot),
		[]byte(filepath.ToSlash(obsoleteRoot)),
		[]byte(strings.ReplaceAll(obsoleteRoot, `\`, `\\`)),
	}
	err := filepath.WalkDir(treeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, needle := range needles {
			if len(needle) > 0 && bytes.Contains(content, needle) {
				relative, _ := filepath.Rel(treeRoot, path)
				return &obsoleteRootError{path: relative, root: string(needle)}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type obsoleteRootError struct {
	path string
	root string
}

func (err *obsoleteRootError) Error() string {
	return "moved Denova file " + err.path + " retained obsolete runtime root " + err.root
}
