package harnessstate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"

	agentstate "github.com/alfredxw/denova/agent/state"
)

func TestLoadSkipsDisabledDeveloperModeState(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	harness, err := Load(context.Background(), &config.Config{DenovaDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if harness.Prompt(config.AgentKindGeneral) != "" || len(harness.SubAgents()) != 0 || len(harness.ToolDescriptions()) != 0 {
		t.Fatalf("disabled Harness State = %#v", harness)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled Harness State initialized storage: %v", err)
	}
}

func TestDisabledDeveloperModeRetainsButDoesNotLoadExistingState(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	enabled := &config.Config{DenovaDir: dataDir}
	enabled.Labs.DeveloperMode = true
	manager, err := OpenWithConfigSource(func() *config.Config { return enabled })
	if err != nil {
		t.Fatal(err)
	}
	current, err := manager.Store().Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes:      []agentstate.Change{{Path: "prompts/general.md", Content: []byte("Preserve this State.")}},
	}); err != nil {
		t.Fatal(err)
	}

	disabled, err := Load(context.Background(), &config.Config{DenovaDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Prompt(config.AgentKindGeneral) != "" {
		t.Fatalf("disabled Developer Mode loaded retained State: %#v", disabled)
	}
	if _, err := os.Stat(filepath.Join(manager.Root(), "prompts", "general.md")); err != nil {
		t.Fatalf("retained State was removed: %v", err)
	}
}

func TestManagerParsesCompleteHarnessAndReadsLiveEdits(t *testing.T) {
	manager := openTestManager(t)
	ctx := context.Background()
	current, err := manager.Store().Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Store().Update(ctx, agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes: []agentstate.Change{
			{Path: "prompts/general.md", Content: []byte("Prefer concrete answers.")},
			{Path: "context/review-style.md", Content: []byte(`---
id: review-style
purpose: Preserve concise review feedback.
agents: [general, ide]
placement: leading_message
enabled: true
---

Lead with actionable edits.`)},
			{Path: "tools.toml", Content: []byte("[tools.read]\ndescription = \"Read only the narrowest relevant source.\"\n")},
			{Path: "subagents/reviewer.md", Content: []byte(`---
id: reviewer
name: Reviewer
description: Review a bounded artifact.
parents: [general]
model_profile: default
tools: [workspace_read]
---

Return concrete findings with evidence.`)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatalf("unexpected update result %#v", result)
	}
	updated, err := manager.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.Prompt(config.AgentKindGeneral); got != "Prefer concrete answers." {
		t.Fatalf("general prompt = %q", got)
	}
	if got := updated.ToolDescriptions()["read"]; got == "" {
		t.Fatal("read tool description was not parsed")
	}
	if got := updated.SubAgents(); len(got) != 1 || got[0].ID != "reviewer" {
		t.Fatalf("unexpected subagents %#v", got)
	}
	exposed := updated.SubAgents()
	exposed[0].Parents[0] = config.AgentKindIDE
	exposed[0].Tools[config.AgentToolWorkspaceWrite] = true
	stable := updated.SubAgents()[0]
	if stable.Parents[0] != config.AgentKindGeneral || stable.Tools[config.AgentToolWorkspaceWrite] {
		t.Fatalf("caller mutated immutable Harness SubAgents: %#v", stable)
	}

	current, err = manager.Store().Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store().Update(ctx, agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes:      []agentstate.Change{{Path: "prompts/general.md", Content: []byte("Prefer short answers.")}},
	}); err != nil {
		t.Fatal(err)
	}
	live, err := manager.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if live.Prompt(config.AgentKindGeneral) != "Prefer short answers." {
		t.Fatalf("Agent build did not observe the live State prompt: %q", live.Prompt(config.AgentKindGeneral))
	}
}

func TestManagerReturnsAllSchemaDiagnostics(t *testing.T) {
	manager := openTestManager(t)
	current, err := manager.Store().Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes: []agentstate.Change{
			{Path: "prompts/unknown.md", Content: []byte("unknown")},
			{Path: "context/bad.md", Content: []byte("missing frontmatter")},
		},
	})
	validation, ok := err.(*agentstate.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T %v", err, err)
	}
	if len(validation.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want two independent failures", validation.Diagnostics)
	}
}

func TestManagerRejectsStateThatWouldBeTruncatedAtRuntime(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	fragmentLimit, metadataLimit := 64, 32
	cfg := &config.Config{
		DenovaDir: dataDir,
		AgentContexts: config.AgentContextSettings{Default: config.AgentContextOverride{
			MaxFragmentBytes: &fragmentLimit, MaxMetadataFieldBytes: &metadataLimit,
		}},
	}
	manager, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	current, err := manager.Store().Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes: []agentstate.Change{
			{Path: "tools.toml", Content: []byte("[tools.read]\ndescription = \"" + strings.Repeat("x", metadataLimit+1) + "\"\n")},
			{Path: "subagents/large.md", Content: []byte(`---
id: large
description: Review a bounded artifact.
parents: [general]
tools: [workspace_read]
---

` + strings.Repeat("x", fragmentLimit+1))},
		},
	})
	validation, ok := err.(*agentstate.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T %v", err, err)
	}
	codes := make(map[string]bool)
	for _, diagnostic := range validation.Diagnostics {
		codes[diagnostic.Code] = true
	}
	if !codes["fragment_too_large"] || !codes["metadata_too_large"] {
		t.Fatalf("runtime truncation risks were not rejected before update: %#v", validation.Diagnostics)
	}
}

func TestManagerRejectsMultipleFrontmatterDocuments(t *testing.T) {
	manager := openTestManager(t)
	current, err := manager.Store().Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes: []agentstate.Change{{Path: "context/invalid.md", Content: []byte(`---
id: invalid
purpose: one
agents: [general]
...
purpose: two
---

content`)}},
	})
	validation, ok := err.(*agentstate.ValidationError)
	if !ok || len(validation.Diagnostics) != 1 || validation.Diagnostics[0].Code != "invalid_frontmatter" {
		t.Fatalf("multiple YAML documents were not rejected: %#v err=%v", validation, err)
	}
}

func TestManagerUsesCurrentConfigurationForStateReferences(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".denova")
	currentConfig := &config.Config{DenovaDir: dataDir}
	manager, err := OpenWithConfigSource(func() *config.Config { return currentConfig })
	if err != nil {
		t.Fatal(err)
	}
	current, err := manager.Store().Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	change := agentstate.Change{Path: "subagents/future.md", Content: []byte(`---
id: future
description: Use a model profile added after the Lab was opened.
parents: [general]
model_profile: future
tools: [workspace_read]
---

Review the supplied artifact.`)}
	if _, err := manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: current.Revision, Changes: []agentstate.Change{change},
	}); err == nil {
		t.Fatal("unknown model profile unexpectedly passed validation")
	}
	currentConfig = &config.Config{
		DenovaDir:     dataDir,
		ModelProfiles: []config.ModelProfileSettings{{ID: "future", Model: "future-model"}},
	}
	result, err := manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: current.Revision, Changes: []agentstate.Change{change},
	})
	if err != nil || !result.Changed {
		t.Fatalf("long-lived Manager kept stale model profiles: result=%#v err=%v", result, err)
	}
}

func openTestManager(t *testing.T) *Manager {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".denova")
	manager, err := Open(&config.Config{DenovaDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
