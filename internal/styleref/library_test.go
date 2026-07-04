package styleref

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryWriteListAndDelete(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	ref, err := lib.Write(WriteRequest{
		Name:        "克制细腻",
		Description: "动作、对白和停顿承载情绪",
		Filename:    "restraint.txt",
		Content:     "# 克制细腻\n\n动作、对白和停顿承载情绪。\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.DisplayPath != ".denova/styles/restraint.md" {
		t.Fatalf("display path = %s", ref.DisplayPath)
	}
	if filepath.Ext(ref.Path) != ".md" {
		t.Fatalf("path should be md: %s", ref.Path)
	}
	refs, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Name != "克制细腻" || refs[0].Description == "" {
		t.Fatalf("refs = %#v", refs)
	}
	if err := lib.Delete(ref.DisplayPath); err != nil {
		t.Fatal(err)
	}
	refs, err = lib.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("refs after delete = %#v", refs)
	}
}

func TestNormalizeStoragePathUsesStylesDisplayDir(t *testing.T) {
	for _, input := range []string{
		"style.md",
		".denova/styles/style.md",
		"/tmp/.denova/styles/style.md",
		"../style.md",
	} {
		if got := NormalizeStoragePath(input); got != ".denova/styles/style.md" {
			t.Fatalf("NormalizeStoragePath(%q) = %q", input, got)
		}
	}
}

func TestLibraryWriteAddsHeaderForDirectSourceContent(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	ref, err := lib.Write(WriteRequest{
		Name:        "雨夜对白",
		Description: "用停顿和环境压住情绪",
		Filename:    "rain-dialogue.md",
		Content:     "窗外的雨声很轻。\n他没有立刻回答。",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "# 雨夜对白\n\n> 用停顿和环境压住情绪\n\n") {
		t.Fatalf("unexpected content:\n%s", text)
	}
	refs, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Name != "雨夜对白" || refs[0].Description != "用停顿和环境压住情绪" {
		t.Fatalf("refs = %#v", refs)
	}
}

func TestLibraryTrimsLargeContentByBytes(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	ref, err := lib.Write(WriteRequest{
		Name:     "长文风",
		Filename: "long.md",
		Content:  strings.Repeat("风", MaxContentBytes),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxContentBytes+1 {
		t.Fatalf("content bytes = %d, want <= %d", len(data), MaxContentBytes+1)
	}
}

func TestLibraryReadAndUpdateFileDocument(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	ref, err := lib.Write(WriteRequest{
		Name:     "旧文风",
		Filename: "restraint.md",
		Content:  "# 旧文风\n\n动作承载情绪。\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := lib.Read(ref.DisplayPath)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Reference.DisplayPath != ref.DisplayPath || !strings.Contains(doc.Content, "动作承载情绪") || doc.Revision == "" {
		t.Fatalf("unexpected document: %#v", doc)
	}

	updated, err := lib.Update(UpdateRequest{
		Path:         ref.DisplayPath,
		Content:      "# 新文风\n\n对白更锋利，句子更短，停顿更多。",
		BaseRevision: doc.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Reference.Name != "新文风" || updated.Reference.Description != "对白更锋利，句子更短，停顿更多。" {
		t.Fatalf("updated reference mismatch: %#v", updated.Reference)
	}
	if updated.Content != "# 新文风\n\n对白更锋利，句子更短，停顿更多。\n" {
		t.Fatalf("updated content should preserve edited markdown without injected header, got:\n%s", updated.Content)
	}
	if updated.Revision == "" || updated.Revision == doc.Revision {
		t.Fatalf("revision should change after update: before=%q after=%q", doc.Revision, updated.Revision)
	}
}

func TestLibraryUpdateRejectsStaleRevision(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	ref, err := lib.Write(WriteRequest{
		Name:     "旧文风",
		Filename: "stale.md",
		Content:  "# 旧文风\n\n旧内容。\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := lib.Read(ref.DisplayPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ref.Path, []byte("# 外部更新\n\n外部内容明显更长。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := lib.Update(UpdateRequest{
		Path:         ref.DisplayPath,
		Content:      "# 前端旧内容\n\n旧编辑器内容。",
		BaseRevision: doc.Revision,
	}); !errors.Is(err, ErrReferenceRevisionConflict) {
		t.Fatalf("expected stale revision conflict, got %v", err)
	}
	got, err := os.ReadFile(ref.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "外部更新") {
		t.Fatalf("stale update should keep external content, got:\n%s", string(got))
	}
}
