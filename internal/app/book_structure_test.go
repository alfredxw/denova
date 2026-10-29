package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/book"
)

func TestBookStructureRejectsStalePreviewAndRestoresWholeBook(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	application, err := New(ctx, &config.Config{OpenAIModel: "test-model", DenovaDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	created, err := application.CreateBook(ctx, root, "Structure Book", "", "", book.StructureVolumes)
	if err != nil {
		t.Fatal(err)
	}
	files := book.NewService(created.Workspace)
	path := "chapters/v00001-Volume/ch00001-Chapter.md"
	if err := files.Create(path, "file", "Original prose"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(created.Workspace, "chapters/v00002-Empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := files.SetChapterConfirmed(path, true); err != nil {
		t.Fatal(err)
	}
	titles := book.StructureTitles{Volume: "Volume 1", Section: "Section 1"}
	preview, err := application.BookStructure(ctx, created.ProjectID, book.StructureSections, "", titles, "Before chapter structure change")
	if err != nil {
		t.Fatal(err)
	}
	if err := files.WriteFile(path, "Edited prose"); err != nil {
		t.Fatal(err)
	}
	if _, err := application.BookStructure(ctx, created.ProjectID, book.StructureSections, preview.Revision, titles, "Before chapter structure change"); !errors.Is(err, book.ErrStructureConflict) {
		t.Fatalf("stale preview error=%v", err)
	}
	preview, err = application.BookStructure(ctx, created.ProjectID, book.StructureSections, "", titles, "Before chapter structure change")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.BookStructure(ctx, created.ProjectID, book.StructureSections, preview.Revision, titles, "Before chapter structure change"); err != nil {
		t.Fatal(err)
	}
	history, err := application.ProjectFiles().VersionHistory(created.ProjectID, 10)
	if err != nil {
		t.Fatal(err)
	}
	backup := ""
	for _, version := range history {
		if version.Message == "Before chapter structure change" {
			backup = version.ID
			break
		}
	}
	if backup == "" {
		t.Fatal("missing explicit structure backup")
	}
	if _, err := application.RestoreProjectVersion(ctx, created.ProjectID, backup, nil); err != nil {
		t.Fatal(err)
	}
	summary, err := files.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Structure != book.StructureVolumes || len(summary.Chapters) != 1 || summary.Chapters[0].Path != path || !summary.Chapters[0].Confirmed {
		t.Fatalf("restored summary=%+v", summary)
	}
	content, err := files.ReadFile(path)
	if err != nil || content != "Edited prose" {
		t.Fatalf("restored content=%q err=%v", content, err)
	}
	if len(summary.Manuscript) != 2 || len(summary.Manuscript[1].Children) != 0 {
		t.Fatalf("empty volume not restored: %+v", summary.Manuscript)
	}
}
