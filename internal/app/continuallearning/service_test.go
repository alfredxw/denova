package continuallearning

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/agents/harnessstate"
	"denova/internal/agents/trajectory"
	apptask "denova/internal/app/task"

	agentstate "github.com/alfredxw/denova/agent/state"
)

type testHost struct{ cfg config.Config }

func (host *testHost) Runtime() Runtime { return Runtime{Config: host.cfg} }

func (*testHost) TrajectorySources(context.Context) ([]trajectory.Source, error) { return nil, nil }

func (*testHost) StartHarnessTurn(context.Context, HarnessTurnRequest) (*apptask.Task, error) {
	return nil, errors.New("unexpected Harness turn")
}

func TestDraftDebugAndPublishAreSeparate(t *testing.T) {
	ctx := context.Background()
	host := &testHost{cfg: config.Config{
		DenovaDir: t.TempDir(), Labs: config.ResolvedLabs{DeveloperMode: true, HarnessStateEnabled: true},
	}}
	service := NewService(host)
	initial, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateState(ctx, StateUpdateRequest{
		BaseRevision: initial.Revision, Summary: "Tune Writing and Game Agents",
		Changes: []StateChange{
			{Path: "prompts/ide.md", Content: "Prefer concise, evidence-backed edits."},
			{Path: "prompts/interactive_story.md", Content: "Keep player choices consequential."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision == initial.PublishedRevision {
		t.Fatal("Draft write unexpectedly changed Published revision")
	}
	production, err := harnessstate.Load(ctx, &host.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := production.Prompt(config.AgentKindIDE); got != "" {
		t.Fatalf("unpublished Draft reached production: %q", got)
	}
	debugged, err := service.Debug(ctx, config.AgentKindIDE, updated.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if debugged.PromptResource != "prompts/ide.md" || debugged.Revision != updated.Revision {
		t.Fatalf("unexpected Draft debug projection: %#v", debugged)
	}
	gameDebug, err := service.Debug(ctx, config.AgentKindInteractiveStory, updated.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if gameDebug.PromptResource != "prompts/interactive_story.md" {
		t.Fatalf("unexpected Game Draft debug projection: %#v", gameDebug)
	}
	generalDebug, err := service.Debug(ctx, config.AgentKindGeneral, updated.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if generalDebug.PromptResource != "" {
		t.Fatalf("Writing/Game Draft leaked into General debug projection: %#v", generalDebug)
	}
	if _, err := service.Debug(ctx, config.AgentKindIDE, initial.Revision); !errors.Is(err, agentstate.ErrConflict) {
		t.Fatalf("stale Draft debug revision error = %v", err)
	}
	published, err := service.Publish(ctx, StatePublishRequest{
		DraftRevision: updated.Revision, PublishedRevision: initial.PublishedRevision,
		Summary: "Publish Writing Agent tuning",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !published.Changed || published.PublishedRevision != updated.Revision {
		t.Fatalf("unexpected publish result: %#v", published)
	}
	production, err = harnessstate.Load(ctx, &host.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := production.Prompt(config.AgentKindIDE); got != "Prefer concise, evidence-backed edits." {
		t.Fatalf("published prompt = %q", got)
	}
	if got := production.Prompt(config.AgentKindInteractiveStory); got != "Keep player choices consequential." {
		t.Fatalf("globally published Game prompt = %q", got)
	}
}

func TestPublishedStateCanBeGloballyDisabledWithoutDeletingIt(t *testing.T) {
	ctx := context.Background()
	host := &testHost{cfg: config.Config{
		DenovaDir: t.TempDir(), Labs: config.ResolvedLabs{DeveloperMode: true, HarnessStateEnabled: true},
	}}
	service := NewService(host)
	initial, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateState(ctx, StateUpdateRequest{
		BaseRevision: initial.Revision, Summary: "Add shared behavior",
		Changes: []StateChange{{Path: "prompts/general.md", Content: "Keep decisions explicit."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Publish(ctx, StatePublishRequest{
		DraftRevision: updated.Revision, PublishedRevision: initial.PublishedRevision,
	}); err != nil {
		t.Fatal(err)
	}

	enabled, err := harnessstate.Load(ctx, &host.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := enabled.Prompt(config.AgentKindGeneral); got != "Keep decisions explicit." {
		t.Fatalf("enabled prompt = %q", got)
	}
	enabledCapabilities, err := service.AgentHostCapabilities(ctx, &host.cfg, config.AgentKindGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabledCapabilities.ReadAdapters) != 1 {
		t.Fatalf("enabled Harness State adapters = %d, want 1", len(enabledCapabilities.ReadAdapters))
	}

	host.cfg.Labs.HarnessStateEnabled = false
	disabled, err := harnessstate.Load(ctx, &host.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := disabled.Prompt(config.AgentKindGeneral); got != "" {
		t.Fatalf("disabled Harness State still contributed prompt %q", got)
	}
	disabledCapabilities, err := service.AgentHostCapabilities(ctx, &host.cfg, config.AgentKindGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if len(disabledCapabilities.ReadAdapters) != 0 {
		t.Fatalf("disabled Harness State exposed %d read adapters", len(disabledCapabilities.ReadAdapters))
	}

	host.cfg.Labs.HarnessStateEnabled = true
	restored, err := harnessstate.Load(ctx, &host.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.Prompt(config.AgentKindGeneral); got != "Keep decisions explicit." {
		t.Fatalf("re-enabled prompt = %q", got)
	}
}

func TestInvalidDraftCannotReplacePublishedState(t *testing.T) {
	ctx := context.Background()
	host := &testHost{cfg: config.Config{
		DenovaDir: t.TempDir(), Labs: config.ResolvedLabs{DeveloperMode: true, HarnessStateEnabled: true},
	}}
	service := NewService(host)
	initial, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := service.UpdateState(ctx, StateUpdateRequest{
		BaseRevision: initial.Revision, Summary: "Add General prompt",
		Changes: []StateChange{{Path: "prompts/general.md", Content: "Keep durable behavior explicit."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstRelease, err := service.Publish(ctx, StatePublishRequest{
		DraftRevision: valid.Revision, PublishedRevision: initial.PublishedRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := service.UpdateState(ctx, StateUpdateRequest{
		BaseRevision: valid.Revision, Summary: "Invalid Draft",
		Changes: []StateChange{{Path: "prompts/not-an-agent.md", Content: "invalid"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Publish(ctx, StatePublishRequest{
		DraftRevision: invalid.Revision, PublishedRevision: firstRelease.PublishedRevision,
	})
	var validation *agentstate.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("invalid Draft publish error = %v", err)
	}
	production, err := harnessstate.Load(ctx, &host.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := production.Prompt(config.AgentKindGeneral); got != "Keep durable behavior explicit." {
		t.Fatalf("invalid Draft replaced Published prompt: %q", got)
	}
}

func TestReleasedStateBecomesInitialPublishedSnapshot(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	promptPath := filepath.Join(dataDir, "state", "prompts", "general.md")
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptPath, []byte("Preserve the released user preference."), 0o600); err != nil {
		t.Fatal(err)
	}
	host := &testHost{cfg: config.Config{
		DenovaDir: dataDir, Labs: config.ResolvedLabs{DeveloperMode: true, HarnessStateEnabled: true},
	}}
	service := NewService(host)
	state, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Changed || state.Revision != state.PublishedRevision {
		t.Fatalf("released State was not initialized as Published: %#v", state)
	}
	production, err := harnessstate.Load(ctx, &host.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := production.Prompt(config.AgentKindGeneral); got != "Preserve the released user preference." {
		t.Fatalf("migrated Published prompt = %q", got)
	}
}
