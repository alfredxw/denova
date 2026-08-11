package app

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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
		importSubtree("denova/internal/agents"),
		importSubtree("github.com/alfredxw/denova/agent"),
	)
	assertNoProductionImports(t, filepath.Join(repository, "internal", "app"), nil,
		importSubtree("denova/internal/api"),
		importSubtree("github.com/alfredxw/denova/agent/context"),
		importSubtree("github.com/alfredxw/denova/agent/providers"),
		importSubtree("github.com/alfredxw/denova/agent/session"),
		importSubtree("github.com/alfredxw/denova/agent/tools"),
	)
	assertNoProductionImports(t, filepath.Join(repository, "internal", "agents"), nil,
		importSubtree("denova/internal/app"),
		importSubtree("denova/internal/api"),
		importSubtree("github.com/alfredxw/denova/agent/providers/protocols"),
	)
	assertNoProductionImports(t, filepath.Join(repository, "agent"), map[string]bool{"providers": true},
		importSubtree("denova"),
		importSubtree("github.com/openai/openai-go"),
	)
	assertNoProductionImports(t, filepath.Join(repository, "agent", "providers"), nil,
		importSubtree("denova"),
	)
	// Provider catalog and registry code stay SDK-free. Only protocol adapters
	// own wire types; builtin provider definitions merely compose registrations.
	assertNoProductionImports(t, filepath.Join(repository, "agent", "providers"), map[string]bool{"protocols": true},
		importSubtree("github.com/openai/openai-go"),
	)
	// Providers are packages in the public agent module. A nested go.mod would
	// silently remove that provider from `cd agent && go test ./...` again.
	providersRoot := filepath.Join(repository, "agent", "providers")
	if err := filepath.WalkDir(providersRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "go.mod" {
			relative, _ := filepath.Rel(repository, path)
			t.Errorf("provider must share the agent module, found nested %s", filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		t.Fatalf("inspect provider module boundaries: %v", err)
	}

	// Eino is not an adapter in the new architecture. Scan every production Go
	// package and every module manifest so it cannot return through an unrelated
	// layer or an indirect nested-module declaration.
	eino := []importRule{
		importSubtree("github.com/cloudwego/eino"),
		importSubtree("github.com/cloudwego/eino-ext"),
	}
	assertNoProductionImports(t, repository, map[string]bool{".git": true}, eino...)
	assertNoModuleDependency(t, repository, "github.com/cloudwego/eino")
}

type importRule struct {
	path    string
	subtree bool
}

func importExact(path string) importRule {
	return importRule{path: strings.TrimSuffix(path, "/")}
}

func importSubtree(path string) importRule {
	return importRule{path: strings.TrimSuffix(path, "/"), subtree: true}
}

func (rule importRule) rejects(path string) bool {
	if path == rule.path {
		return true
	}
	return rule.subtree && strings.HasPrefix(path, rule.path+"/")
}

func assertNoProductionImports(t *testing.T, root string, skippedTopLevel map[string]bool, forbidden ...importRule) {
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
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
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
			for _, rule := range forbidden {
				if rule.rejects(name) {
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

func assertNoModuleDependency(t *testing.T, repository, dependency string) {
	t.Helper()
	err := filepath.WalkDir(repository, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "go.mod" && entry.Name() != "go.sum" {
			return nil
		}
		content, err := fs.ReadFile(os.DirFS(filepath.Dir(path)), entry.Name())
		if err != nil {
			return err
		}
		if strings.Contains(string(content), dependency) {
			relative, _ := filepath.Rel(repository, path)
			t.Errorf("%s declares removed dependency %q", filepath.ToSlash(relative), dependency)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect module dependencies: %v", err)
	}
}
