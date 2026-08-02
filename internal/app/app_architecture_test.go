package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAppPackageArchitecture(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve App architecture test path")
	}
	root := filepath.Dir(currentFile)

	// Child application services depend on explicit contracts and domain
	// packages; importing the root composition type would recreate a cycle and
	// erase the boundaries established by this refactor.
	assertNoProductionImports(t, root, nil, importExact("denova/internal/app"))

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	productionFiles := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		productionFiles = append(productionFiles, entry.Name())
		content, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		lines := bytes.Count(content, []byte{'\n'}) + 1
		if lines > 800 {
			t.Errorf("root App file %s has %d lines; split the responsibility before extending it", entry.Name(), lines)
		}
	}
	if len(productionFiles) > 48 {
		t.Errorf("root App package has %d production files; keep domain services in subpackages (files=%v)", len(productionFiles), productionFiles)
	}

	for _, name := range productionFiles {
		if extractedRootFilename(name) {
			t.Errorf("%s recreates a responsibility already owned by an App subpackage", name)
		}
	}
}

func extractedRootFilename(name string) bool {
	if name == "automation_host.go" || name == "agentchat_host.go" || name == "configmanager_host.go" ||
		name == "image_host.go" || name == "lore_host.go" || name == "settings_host.go" {
		return false
	}
	for _, prefix := range []string{
		"agent_chat_", "automation_", "book_", "config_manager_", "context_compaction_",
		"image_", "lore_", "messages_", "review_feedback_", "settings_app_", "skills_",
		"style_", "task_replay_", "interactive_resource_",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
