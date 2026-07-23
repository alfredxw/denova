package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestFileReadCacheHitReturnsCachedContent(t *testing.T) {
	path := writeTempFile(t, "line one\nline two\nline three\n")
	cache := newFileReadCache(fileReadCacheDefaultMaxBytes)
	cache.set(path, "line one\nline two\nline three\n")

	got, ok := cache.get(path)
	if !ok {
		t.Fatal("cache should return cached content")
	}
	if got != "line one\nline two\nline three\n" {
		t.Fatalf("cached content = %q, want %q", got, "line one\nline two\nline three\n")
	}
}

func TestFileReadCacheMissesWhenFileModified(t *testing.T) {
	path := writeTempFile(t, "original\n")
	cache := newFileReadCache(fileReadCacheDefaultMaxBytes)
	cache.set(path, "original\n")

	// Modify the file on disk
	time.Sleep(10 * time.Millisecond) // ensure mtime changes on fast filesystems
	if err := os.WriteFile(path, []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, ok := cache.get(path); ok {
		t.Fatal("cache should miss when file mtime changed")
	}
}

func TestFileReadCacheEvictsLRUWhenFull(t *testing.T) {
	dir := t.TempDir()
	cache := newFileReadCache(20) // tiny cache, only ~20 bytes

	// Write two files, each ~15 bytes of content (total ~30 > 20)
	path1 := filepath.Join(dir, "a.txt")
	path2 := filepath.Join(dir, "b.txt")
	writeFile(t, path1, "aaaaaaaaaaaaaaa\n")
	writeFile(t, path2, "bbbbbbbbbbbbbbb\n")

	cache.set(path1, "aaaaaaaaaaaaaaa\n")
	cache.set(path2, "bbbbbbbbbbbbbbb\n")

	// The first entry should have been evicted
	if _, ok := cache.get(path1); ok {
		t.Fatal("oldest entry should be evicted when cache overflows")
	}
	// The second entry should still be present
	if _, ok := cache.get(path2); !ok {
		t.Fatal("newer entry should remain in cache")
	}
}

func TestApplyFileWindowSlicesByOffsetAndLimit(t *testing.T) {
	full := "line1\nline2\nline3\nline4\nline5"
	got, err := applyFileWindow(full, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "line2\nline3" {
		t.Fatalf("window [2,2] = %q, want %q", got, "line2\nline3")
	}
}

func TestApplyFileWindowClampsToEnd(t *testing.T) {
	full := "a\nb\nc"
	got, err := applyFileWindow(full, 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != "b\nc" {
		t.Fatalf("window beyond end = %q, want %q", got, "b\nc")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
