package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk/filesystem"
)

func TestAgentFilesystemBackendDefaultsReadWindow(t *testing.T) {
	inner := &capturingReadBackend{}
	backend := newAgentFilesystemBackend(inner)
	req := &filesystem.ReadRequest{FilePath: "/tmp/story.md"}

	content, err := backend.Read(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if content.Content != "ok" {
		t.Fatalf("unexpected read content: %q", content.Content)
	}
	if inner.lastRead == nil {
		t.Fatalf("expected underlying backend to receive read request")
	}
	if inner.lastRead.Offset != 1 {
		t.Fatalf("default read offset = %d, want 1", inner.lastRead.Offset)
	}
	if inner.lastRead.Limit != agentFileReadDefaultLimitLines {
		t.Fatalf("default read limit = %d, want %d", inner.lastRead.Limit, agentFileReadDefaultLimitLines)
	}
	if req.Offset != 0 || req.Limit != 0 {
		t.Fatalf("wrapper should not mutate caller request, got offset=%d limit=%d", req.Offset, req.Limit)
	}
}

func TestAgentFilesystemBackendPreservesExplicitReadWindow(t *testing.T) {
	inner := &capturingReadBackend{}
	backend := newAgentFilesystemBackend(inner)

	_, err := backend.Read(context.Background(), &filesystem.ReadRequest{
		FilePath: "/tmp/story.md",
		Offset:   2001,
		Limit:    agentFileReadDefaultLimitLines + 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inner.lastRead == nil {
		t.Fatalf("expected underlying backend to receive read request")
	}
	if inner.lastRead.Offset != 2001 {
		t.Fatalf("explicit read offset = %d, want 2001", inner.lastRead.Offset)
	}
	if inner.lastRead.Limit != agentFileReadDefaultLimitLines+400 {
		t.Fatalf("explicit read limit = %d, want %d", inner.lastRead.Limit, agentFileReadDefaultLimitLines+400)
	}
}

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

func newTestAgentFilesystemBackend(t *testing.T, workspaces ...string) filesystem.Backend {
	t.Helper()
	inner, err := localbk.NewBackend(context.Background(), &localbk.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return newAgentFilesystemBackend(inner, workspaces...)
}

type capturingReadBackend struct {
	filesystem.Backend
	lastRead *filesystem.ReadRequest
}

func (b *capturingReadBackend) Read(_ context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	if req == nil {
		return nil, fmt.Errorf("read request is nil")
	}
	next := *req
	b.lastRead = &next
	return &filesystem.FileContent{Content: "ok"}, nil
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
