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

func TestProjectInstructionsContextSourceIsOrderedVerbatimAndLive(t *testing.T) {
	workspace := t.TempDir()
	state := book.NewState(workspace)
	source, err := NewProjectInstructionsContextSource(&config.Config{Workspace: workspace}, config.AgentKindIDE, state)
	if err != nil {
		t.Fatal(err)
	}
	fragments, err := source.Materialize(context.Background(), agent.ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 0 {
		t.Fatalf("missing project instruction files produced fragments: %#v", fragments)
	}

	if err := os.WriteFile(filepath.Join(workspace, book.AgentInstructionsFileName), []byte("Run focused verification after changes."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, book.CreatorFileName), []byte("Use close third-person narration."), 0o644); err != nil {
		t.Fatal(err)
	}
	fragments, err = source.Materialize(context.Background(), agent.ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 2 {
		t.Fatalf("fragment count = %d", len(fragments))
	}
	wants := []struct {
		resource string
		source   string
		heading  string
		body     string
	}{
		{book.AgentInstructionsFileName, "workspace AGENTS.md", "# Project instructions", "Run focused verification after changes."},
		{book.CreatorFileName, "workspace CREATOR.md", "# Creative instructions", "Use close third-person narration."},
	}
	for index, want := range wants {
		fragment := fragments[index]
		if fragment.Source != want.source || fragment.Resource != want.resource || fragment.Revision == "" ||
			fragment.Placement != agent.ContextLeadingMessage || fragment.Rendering != agent.ContextRenderVerbatim || fragment.Role != agent.User {
			t.Fatalf("unexpected project instruction fragment %d: %#v", index, fragment)
		}
		if !strings.HasPrefix(fragment.Content, want.heading+"\n\n"+want.body) || strings.Contains(fragment.Content, "Source:") ||
			!strings.HasSuffix(fragment.Content, "A later explicit user request takes precedence.") {
			t.Fatalf("unexpected model-visible project instruction %d: %q", index, fragment.Content)
		}
	}

	if err := os.WriteFile(filepath.Join(workspace, book.CreatorFileName), []byte("Use first-person narration."), 0o644); err != nil {
		t.Fatal(err)
	}
	next, err := source.Materialize(context.Background(), agent.ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if next[0] != fragments[0] {
		t.Fatalf("unchanged AGENTS.md fragment should preserve exact cache bytes: before=%#v after=%#v", fragments[0], next[0])
	}
	if next[1].Revision == fragments[1].Revision || !strings.Contains(next[1].Content, "first-person") {
		t.Fatalf("live CREATOR.md instructions were not refreshed: %#v", next[1])
	}
	lowerLimit := 64 * 1024
	limited, err := NewProjectInstructionsContextSource(&config.Config{AgentContexts: config.AgentContextSettings{
		IDE: config.AgentContextOverride{MaxFragmentBytes: &lowerLimit},
	}}, config.AgentKindIDE, state)
	if err != nil {
		t.Fatal(err)
	}
	if limited.Identity() == source.Identity() {
		t.Fatal("project instruction identity must change with its configured hard limit")
	}
}

func TestProjectInstructionsContextSourceRejectsEachOversizeFile(t *testing.T) {
	for _, resource := range []string{book.AgentInstructionsFileName, book.CreatorFileName} {
		t.Run(resource, func(t *testing.T) {
			workspace := t.TempDir()
			if err := os.WriteFile(filepath.Join(workspace, resource), []byte(strings.Repeat("x", 1024)), 0o644); err != nil {
				t.Fatal(err)
			}
			limit := 512
			cfg := &config.Config{Workspace: workspace, AgentContexts: config.AgentContextSettings{
				IDE: config.AgentContextOverride{MaxFragmentBytes: &limit},
			}}
			source, err := NewProjectInstructionsContextSource(cfg, config.AgentKindIDE, book.NewState(workspace))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := source.Materialize(context.Background(), agent.ContextRequest{}); err == nil ||
				!strings.Contains(err.Error(), resource) || !strings.Contains(err.Error(), "project instruction limit") {
				t.Fatalf("oversize error = %v", err)
			}
		})
	}
}

func TestProjectInstructionsContextIdentityUsesProjectIDInsteadOfRuntimeRoot(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	first, err := NewProjectInstructionsContextSource(
		&config.Config{ProjectID: "project-portable", Workspace: firstRoot},
		config.AgentKindIDE,
		book.NewState(firstRoot),
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewProjectInstructionsContextSource(
		&config.Config{ProjectID: "project-portable", Workspace: secondRoot},
		config.AgentKindIDE,
		book.NewState(secondRoot),
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity() != second.Identity() {
		t.Fatalf("context identity changed after moving the same Project: first=%#v second=%#v", first.Identity(), second.Identity())
	}

	otherProject, err := NewProjectInstructionsContextSource(
		&config.Config{ProjectID: "project-other", Workspace: secondRoot},
		config.AgentKindIDE,
		book.NewState(secondRoot),
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity() == otherProject.Identity() {
		t.Fatal("context identity must distinguish different Projects")
	}
}
