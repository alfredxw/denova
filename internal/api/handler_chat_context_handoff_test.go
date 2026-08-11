package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agent"
	runtimeapp "denova/internal/app"
)

func TestChatContextAnalysisPreservesExplicitHandoffMetadata(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, "/api/chat/context-analysis", map[string]any{
		"message": "Review this selection.",
		"selections": []map[string]any{{
			"source":     "editor_selection",
			"purpose":    "ask_agent",
			"version":    "revision-7",
			"size_bytes": 1,
			"file_name":  "chapters/ch01.md",
			"start_line": 2,
			"end_line":   3,
			"content":    "hello",
		}},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("context analysis status=%d body=%s", response.Code, response.Body.String())
	}

	var analysis agent.ContextAnalysis
	decodeResponse(t, response.Body.Bytes(), &analysis)
	var handoff *agent.ContextAnalysisPart
	for i := range analysis.ContextParts {
		if analysis.ContextParts[i].Source == "editor_selection" {
			handoff = &analysis.ContextParts[i]
			break
		}
	}
	if handoff == nil {
		t.Fatalf("explicit handoff missing from context parts: %#v", analysis.ContextParts)
	}
	if handoff.Title != "chapters/ch01.md:L2-L3" || handoff.Purpose != "ask_agent" || handoff.Version != "revision-7" || handoff.Bytes != 5 {
		t.Fatalf("unexpected handoff metadata: %#v", *handoff)
	}

	if len(analysis.ContextMessages) == 0 {
		t.Fatal("final model-visible messages are empty")
	}
	finalMessage := analysis.ContextMessages[len(analysis.ContextMessages)-1].Content
	for _, expected := range []string{
		`"source":"editor_selection"`,
		`"purpose":"ask_agent"`,
		`"version":"revision-7"`,
		`"size_bytes":5`,
		"hello",
	} {
		if !strings.Contains(finalMessage, expected) {
			t.Fatalf("final model message missing %q: %s", expected, finalMessage)
		}
	}
}

func TestChatContextAnalysisRejectsHandoffsOverTheServerLimit(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, "/api/chat/context-analysis", map[string]any{
		"message": "Review this selection.",
		"selections": []map[string]any{{
			"source":     "editor_selection",
			"purpose":    "ask_agent",
			"version":    "revision-7",
			"size_bytes": 1,
			"file_name":  "chapters/ch01.md",
			"start_line": 2,
			"end_line":   3,
			"content":    strings.Repeat("界", 90_000),
		}},
	})
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized handoff status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestChatContextAnalysisUsesConfiguredHandoffLimit(t *testing.T) {
	root := t.TempDir()
	limitKB := 1
	if err := config.WriteSettingsFile(config.UserConfigPath(root), config.Settings{
		AgentContextHandoffLimitKB: &limitKB,
	}); err != nil {
		t.Fatal(err)
	}
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:         "test-model",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, "/api/chat/context-analysis", map[string]any{
		"message": "Review this selection.",
		"selections": []map[string]any{{
			"source":    "editor_selection",
			"purpose":   "ask_agent",
			"version":   "revision-7",
			"file_name": "chapters/ch01.md",
			"content":   strings.Repeat("x", 1_100),
		}},
	})
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("configured handoff limit status=%d body=%s", response.Code, response.Body.String())
	}
}
