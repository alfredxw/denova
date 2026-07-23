package app

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestAgentPackageDependencyDirection(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve dependency test path")
	}
	repository := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))

	assertNoProductionImports(t, filepath.Join(repository, "internal", "api"), nil,
		"denova/internal/agent", "github.com/alfredxw/denova/adk")
	assertNoProductionImports(t, filepath.Join(repository, "internal", "app"), nil,
		"denova/internal/api", "github.com/alfredxw/denova/adk",
		"github.com/cloudwego/eino", "github.com/cloudwego/eino-ext")
	assertNoProductionImports(t, filepath.Join(repository, "internal", "agent"), nil,
		"denova/internal/app", "denova/internal/api",
		"github.com/cloudwego/eino", "github.com/cloudwego/eino-ext")
	assertNoProductionImports(t, filepath.Join(repository, "adk"), map[string]bool{"model": true},
		"denova/", "github.com/openai/openai-go",
		"github.com/cloudwego/eino", "github.com/cloudwego/eino-ext")
}

func assertNoProductionImports(t *testing.T, root string, skippedTopLevel map[string]bool, forbidden ...string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			first, _, _ := strings.Cut(filepath.ToSlash(relative), "/")
			if relative != "." && skippedTopLevel[first] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			for _, prefix := range forbidden {
				if name == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(name, prefix) {
					t.Errorf("%s imports forbidden dependency %q", filepath.ToSlash(relative), name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect production imports under %s: %v", root, err)
	}
}
