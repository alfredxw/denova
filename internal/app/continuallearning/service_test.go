package continuallearning

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/session"
	"denova/internal/agents/trajectory"
)

type testHost struct{ runtime Runtime }

func (host testHost) Runtime() Runtime { return host.runtime }

func (testHost) AcquireRootOperation(context.Context) (Operation, error) {
	return nil, errors.New("unexpected root operation")
}

func (testHost) TrajectorySources(context.Context) ([]trajectory.Source, error) { return nil, nil }

func (testHost) ResolveAsk(context.Context, *session.Session, string, string, []agentconversation.HostAskAnswer, string) (agentconversation.HostAskResolution, error) {
	return agentconversation.HostAskResolution{}, errors.New("unexpected Ask resolution")
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
