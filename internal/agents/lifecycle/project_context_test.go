package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/book"

	agent "github.com/alfredxw/denova/agent"
)

func TestProjectInstructionContextSourceIsEarlyVerbatimAndLive(t *testing.T) {
	workspace := t.TempDir()
	state := book.NewState(workspace)
	source, err := NewProjectInstructionContextSource(&config.Config{Workspace: workspace}, config.AgentKindIDE, state)
	if err != nil {
		t.Fatal(err)
	}
	fragments, err := source.Materialize(context.Background(), agent.ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 0 {
		t.Fatalf("missing CREATOR.md produced fragments: %#v", fragments)
	}

	if err := os.WriteFile(filepath.Join(workspace, projectInstructionsResource), []byte("Use close third-person narration."), 0o644); err != nil {
		t.Fatal(err)
	}
	fragments, err = source.Materialize(context.Background(), agent.ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 1 {
		t.Fatalf("fragment count = %d", len(fragments))
	}
	fragment := fragments[0]
	if fragment.Source != "workspace CREATOR.md" || fragment.Resource != projectInstructionsResource || fragment.Revision == "" ||
		fragment.Placement != agent.ContextLeadingMessage || fragment.Rendering != agent.ContextRenderVerbatim || fragment.Role != agent.User {
		t.Fatalf("unexpected project instruction fragment: %#v", fragment)
	}
	if !strings.HasPrefix(fragment.Content, "# Project instructions\n\nUse close third-person narration.") || strings.Contains(fragment.Content, "Source:") {
		t.Fatalf("unexpected model-visible project instruction: %q", fragment.Content)
	}

	if err := os.WriteFile(filepath.Join(workspace, projectInstructionsResource), []byte("Use first-person narration."), 0o644); err != nil {
		t.Fatal(err)
	}
	next, err := source.Materialize(context.Background(), agent.ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if next[0].Revision == fragment.Revision || !strings.Contains(next[0].Content, "first-person") {
		t.Fatalf("live project instructions were not refreshed: %#v", next[0])
	}
	lowerLimit := 64 * 1024
	limited, err := NewProjectInstructionContextSource(&config.Config{AgentContexts: config.AgentContextSettings{
		IDE: config.AgentContextOverride{MaxFragmentBytes: &lowerLimit},
	}}, config.AgentKindIDE, state)
	if err != nil {
		t.Fatal(err)
	}
	if limited.Identity() == source.Identity() {
		t.Fatal("project instruction identity must change with its configured hard limit")
	}
}

func TestProjectInstructionContextSourceRejectsOversizeContent(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, projectInstructionsResource), []byte(strings.Repeat("x", 1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	limit := 512
	cfg := &config.Config{Workspace: workspace, AgentContexts: config.AgentContextSettings{
		IDE: config.AgentContextOverride{MaxFragmentBytes: &limit},
	}}
	source, err := NewProjectInstructionContextSource(cfg, config.AgentKindIDE, book.NewState(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Materialize(context.Background(), agent.ContextRequest{}); err == nil || !strings.Contains(err.Error(), "project instruction limit") {
		t.Fatalf("oversize error = %v", err)
	}
}
