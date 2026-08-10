package agents

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const adkRuntimeImport = "github.com/alfredxw/denova/agent/runtime"

func TestADKRuntimeDependencyStopsAtAgentsLayer(t *testing.T) {
	for _, dir := range []string{"../app", "../api"} {
		assertGoFilesDoNotImport(t, dir, true, adkRuntimeImport)
	}
}

func TestConcreteToolsDoNotDependOnAgentOrchestration(t *testing.T) {
	assertGoFilesDoNotImport(t, "tools", false, adkRuntimeImport, "denova/internal/agents")
}

func TestReusableAgentPackagesDoNotDependOnCompositionRoot(t *testing.T) {
	for _, dir := range []string{
		"chat", "context", "conversation", "execution", "interactive",
		"modelio", "modeltask", "prompts", "run", "skills", "toolresult", "toolruntime",
	} {
		assertGoFilesDoNotImport(t, dir, false, "denova/internal/agents")
	}
}

func TestChatDoesNotDependOnProductExecution(t *testing.T) {
	assertGoFilesDoNotImport(t, "chat", false, "denova/internal/agents/execution")
}

func TestAgentsRootContainsOnlyCompositionResponsibilities(t *testing.T) {
	allowed := map[string]bool{
		"builder.go":  true,
		"director.go": true,
		"doc.go":      true,
		"message.go":  true,
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !allowed[name] {
			t.Errorf("agents root implementation %q must move to its owning package", name)
		}
	}
}

func assertGoFilesDoNotImport(t *testing.T, dir string, skipTests bool, forbidden ...string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || (skipTests && strings.HasSuffix(path, "_test.go")) {
			return nil
		}
		files := token.NewFileSet()
		parsed, err := parser.ParseFile(files, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			for _, denied := range forbidden {
				if importPath == denied {
					t.Errorf("%s:%d imports forbidden package %q", path, files.Position(imported.Pos()).Line, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect package boundary %s: %v", dir, err)
	}
}
