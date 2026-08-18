package agents

import (
	"context"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"denova/config"
)

func TestWorkspaceToolsFactoryBuildsOnlyEnabledReadSurface(t *testing.T) {
	workspace := t.TempDir()
	tools, err := agenttoolruntime.NewCatalog(&config.Config{Workspace: workspace}).Workspace(config.ResolvedAgentToolSettings{config.AgentToolFilesystemRead: true})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, candidate := range tools {
		info, err := candidate.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, info.Name)
	}
	sort.Strings(names)
	want := []string{"glob", "grep", "read"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("filesystem tool names = %v, want %v", names, want)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".denova")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only tool assembly must not initialize mutation storage, stat err=%v", err)
	}
}
