package agentrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLedgerOmitsRuntimeFilesystemRoots(t *testing.T) {
	container := t.TempDir()
	workspace := filepath.Join(container, "projects", "portable")
	stateRoot := filepath.Join(container, "project-state", "portable")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	ledger, err := NewLedgerWithOptions(workspace, DefaultLoopPolicy().RunLedger, Options{
		AgentKind: AgentKindIDE, ProjectID: "project-portable", StateRoot: stateRoot,
		Workspace: workspace, SessionID: "session-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ledger == nil {
		t.Fatal("run ledger was not created")
	}
	path := ledger.Path()
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, runtimeRoot := range []string{workspace, stateRoot, filepath.ToSlash(workspace), filepath.ToSlash(stateRoot)} {
		if strings.Contains(string(raw), runtimeRoot) {
			t.Fatalf("run ledger retained runtime root %q: %s", runtimeRoot, raw)
		}
	}
	if !strings.Contains(string(raw), `"project_id":"project-portable"`) {
		t.Fatalf("run ledger lost stable Project identity: %s", raw)
	}
}
