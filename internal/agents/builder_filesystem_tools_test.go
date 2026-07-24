package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"denova/config"
)

func TestFilesystemToolsFactoryBuildsStableNativeSurfaceWithoutReadOnlyMutationStore(t *testing.T) {
	workspace := t.TempDir()
	tools, err := filesystemToolsFactory(workspace)(config.ResolvedAgentToolSettings{FileRead: true})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, candidate := range tools {
		info, err := candidate.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, info.Name)
	}
	sort.Strings(names)
	want := []string{"edit_file", "execute", "glob", "grep", "ls", "read_file", "write_file"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("filesystem tool names = %v, want %v", names, want)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".denova")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only tool assembly must not initialize mutation storage, stat err=%v", err)
	}
}
