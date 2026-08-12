package continuallearning

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/agents/trajectory"
)

type testHost struct {
	runtime Runtime
	sources []trajectory.Source
}

func (host testHost) Runtime() Runtime { return host.runtime }

func (testHost) AcquireRootOperation(context.Context) (Operation, error) {
	return nil, errors.New("unexpected root operation")
}

func (host testHost) TrajectorySources(context.Context) ([]trajectory.Source, error) {
	return append([]trajectory.Source(nil), host.sources...), nil
}

func TestDisabledServiceDoesNotInitializeState(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	service := NewService(testHost{runtime: Runtime{Config: config.Config{DenovaDir: dataDir}}})
	status, err := service.ScheduleStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled {
		t.Fatalf("disabled schedule status = %#v", status)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"enabled":false,"interval_hours":0}` {
		t.Fatalf("zero schedule timestamps leaked into the API: %s", encoded)
	}
	if _, err := service.State(context.Background()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("State error = %v, want ErrDisabled", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled Lab initialized State directory: %v", err)
	}
}

func TestStateUpdateAndApplicationVersionHistory(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	cfg := config.Config{DenovaDir: dataDir}
	cfg.Labs.ContinualLearning = true
	service := NewService(testHost{runtime: Runtime{Config: cfg}})
	current, err := service.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Source != StateSourceBuiltin {
		t.Fatalf("empty State source = %q, want %q", current.Source, StateSourceBuiltin)
	}
	updated, err := service.UpdateState(context.Background(), StateUpdateRequest{
		BaseRevision: current.Revision,
		Summary:      "Add durable response preference",
		Changes:      []StateChange{{Path: "prompts/general.md", Content: "Lead with the result."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Changed || updated.Version == nil {
		t.Fatalf("unexpected update %#v", updated)
	}
	firstVersionID := updated.Version.ID
	rawVersionID := StateVersionID(strings.TrimPrefix(string(firstVersionID), stateVersionIDPrefix))
	if _, err := service.Restore(context.Background(), rawVersionID); !errors.Is(err, ErrStateVersionNotFound) {
		t.Fatalf("raw Git hash escaped the opaque version contract: %v", err)
	}
	snapshot, err := service.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 1 || snapshot.Files[0].Path != "prompts/general.md" || snapshot.Files[0].Content != "Lead with the result." {
		t.Fatalf("unexpected State snapshot %#v", snapshot)
	}
	if snapshot.Source != StateSourceUser {
		t.Fatalf("non-empty State source = %q, want %q", snapshot.Source, StateSourceUser)
	}
	updated, err = service.UpdateState(context.Background(), StateUpdateRequest{
		BaseRevision: snapshot.Revision,
		Summary:      "Refine durable response preference",
		Changes:      []StateChange{{Path: "prompts/general.md", Content: "Lead with concise evidence."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Changed || updated.Version == nil || updated.Version.ID == firstVersionID {
		t.Fatalf("unexpected second update %#v", updated)
	}
	versions, err := service.Versions(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Summary != "Refine durable response preference" || versions[1].ID != firstVersionID {
		t.Fatalf("unexpected application versions %#v", versions)
	}
	diff, err := service.Diff(context.Background(), versions[1].ID, versions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Patch == "" {
		t.Fatal("expected a non-empty application State diff")
	}
	restored, err := service.Restore(context.Background(), firstVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Changed || restored.Version == nil {
		t.Fatalf("unexpected restore %#v", restored)
	}
	snapshot, err = service.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 1 || snapshot.Files[0].Content != "Lead with the result." {
		t.Fatalf("restored State snapshot = %#v", snapshot)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state", ".git")); err != nil {
		t.Fatalf("application layer did not create State version repository: %v", err)
	}
}

func TestTrajectoryCatalogListsAndReadsRecentSessionEvidence(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, ".denova")
	projectState := filepath.Join(dataDir, "project-state", "project-1")
	store, err := session.NewStore(filepath.Join(projectState, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create("Opening revision")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DenovaDir: dataDir}
	cfg.Labs.ContinualLearning = true
	service := NewService(testHost{
		runtime: Runtime{Config: cfg},
		sources: []trajectory.Source{{
			ProjectID: "project-1", Name: "First Book", Workspace: filepath.Join(root, "book"), StateRoot: projectState,
		}},
	})

	result, err := service.Trajectories(context.Background(), time.Now().UTC().Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Kind != trajectoryKindSession || result.Items[0].Title != "Opening revision" {
		t.Fatalf("unexpected trajectory list %#v", result)
	}
	if !strings.Contains(result.Items[0].URI, created.ID) {
		t.Fatalf("trajectory URI %q does not contain Session %q", result.Items[0].URI, created.ID)
	}
	detail, err := service.Trajectory(context.Background(), result.Items[0].URI)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Kind != "trajectory_session" || !strings.Contains(detail.Content, `"schema": "denova.trajectory.session.v1"`) {
		t.Fatalf("unexpected trajectory detail %#v", detail)
	}
}

func TestStateHistoryRejectsPrivateGitSymlink(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	stateRoot := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(stateRoot, ".git")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cfg := config.Config{DenovaDir: dataDir}
	cfg.Labs.ContinualLearning = true
	service := NewService(testHost{runtime: Runtime{Config: cfg}})
	if _, err := service.State(context.Background()); err == nil || !strings.Contains(err.Error(), ".git must be a private directory") {
		t.Fatalf("Harness State history followed an unsafe .git entry: %v", err)
	}
}

func TestCommittedHistoryCanRestoreInvalidLiveState(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	cfg := config.Config{DenovaDir: dataDir}
	cfg.Labs.ContinualLearning = true
	service := NewService(testHost{runtime: Runtime{Config: cfg}})
	current, err := service.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateState(context.Background(), StateUpdateRequest{
		BaseRevision: current.Revision, Summary: "Add valid State",
		Changes: []StateChange{{Path: "prompts/general.md", Content: "Lead with evidence."}},
	})
	if err != nil || updated.Version == nil {
		t.Fatalf("create valid State: result=%#v err=%v", updated, err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "state", "unsupported.txt"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.State(context.Background()); err != nil {
		t.Fatalf("management read hid invalid live files: %v", err)
	}
	versions, err := service.Versions(context.Background(), 10)
	if err != nil || len(versions) != 1 {
		t.Fatalf("committed history unavailable while live State is invalid: versions=%#v err=%v", versions, err)
	}
	if _, err := service.Restore(context.Background(), updated.Version.ID); err != nil {
		t.Fatalf("restore could not repair invalid live State: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state", "unsupported.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore retained invalid live file: %v", err)
	}
}

func TestServiceRetriesInitializationAfterStorageIsRepaired(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	stateRoot := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	gitPath := filepath.Join(stateRoot, ".git")
	if err := os.Symlink(t.TempDir(), gitPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cfg := config.Config{DenovaDir: dataDir}
	cfg.Labs.ContinualLearning = true
	service := NewService(testHost{runtime: Runtime{Config: cfg}})
	if _, err := service.State(context.Background()); err == nil {
		t.Fatal("unsafe storage unexpectedly initialized")
	}
	if err := os.Remove(gitPath); err != nil {
		t.Fatal(err)
	}
	if _, err := service.State(context.Background()); err != nil {
		t.Fatalf("repaired storage remained permanently poisoned: %v", err)
	}
}

func TestStateVersionSummaryTruncatesOnUTF8Boundary(t *testing.T) {
	summary := normalizeStateVersionSummary(strings.Repeat("界", 100))
	if !utf8.ValidString(summary) || len(summary) > 240 {
		t.Fatalf("version summary truncation produced invalid UTF-8: bytes=%d value=%q", len(summary), summary)
	}
}
