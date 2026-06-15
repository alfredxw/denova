package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk/filesystem"
)

func TestAgentFilesystemBackendNormalizesTrailingWhitespaceForUniqueEditMatch(t *testing.T) {
	filePath := writeTempFile(t, "alpha   \nbeta\t\nomega   \n")
	backend := newTestAgentFilesystemBackend(t)

	err := backend.Edit(context.Background(), &filesystem.EditRequest{
		FilePath:   filePath,
		OldString:  "alpha\nbeta\n",
		NewString:  "ALPHA\nBETA\n",
		ReplaceAll: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filePath)
	want := "ALPHA\nBETA\nomega   \n"
	if got != want {
		t.Fatalf("edited content mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestAgentFilesystemBackendRejectsAmbiguousNormalizedEditMatch(t *testing.T) {
	content := "target   \nkeep\ntarget\t\n"
	filePath := writeTempFile(t, content)
	backend := newTestAgentFilesystemBackend(t)

	err := backend.Edit(context.Background(), &filesystem.EditRequest{
		FilePath:   filePath,
		OldString:  "target\n",
		NewString:  "changed\n",
		ReplaceAll: false,
	})
	if err == nil || !strings.Contains(err.Error(), "appears 2 times") {
		t.Fatalf("expected ambiguous normalized match error, got %v", err)
	}
	if got := readFile(t, filePath); got != content {
		t.Fatalf("ambiguous edit should not change file\ngot:  %q\nwant: %q", got, content)
	}
}

func TestAgentFilesystemBackendDoesNotUsePartialPrefixMatch(t *testing.T) {
	content := "alpha\nbeta\n"
	filePath := writeTempFile(t, content)
	backend := newTestAgentFilesystemBackend(t)

	err := backend.Edit(context.Background(), &filesystem.EditRequest{
		FilePath:   filePath,
		OldString:  "alpha\nchanged\n",
		NewString:  "ALPHA\nchanged\n",
		ReplaceAll: false,
	})
	if err == nil || !strings.Contains(err.Error(), "string not found") {
		t.Fatalf("expected original string not found error, got %v", err)
	}
	if got := readFile(t, filePath); got != content {
		t.Fatalf("failed edit should not change file\ngot:  %q\nwant: %q", got, content)
	}
}

func newTestAgentFilesystemBackend(t *testing.T) filesystem.Backend {
	t.Helper()
	inner, err := localbk.NewBackend(context.Background(), &localbk.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return newAgentFilesystemBackend(inner)
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return filePath
}

func readFile(t *testing.T, filePath string) string {
	t.Helper()
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
