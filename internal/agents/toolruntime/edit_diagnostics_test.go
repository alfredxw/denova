package toolruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
	agenttoolresult "github.com/alfredxw/denova/agent/toolresult"
)

func TestEditDiagnosticsReachModelThroughPublicAgent(t *testing.T) {
	for _, kind := range []string{agentrun.AgentKindIDE, "general-purpose"} {
		t.Run(kind, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "state.md")
			const original = "## First\n\nsame\n## Second\n\nsame\nexisting"
			if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			definitions, err := NewCatalog(&config.Config{Workspace: workspace}).Workspace(config.ResolvedAgentToolSettings{
				config.AgentToolWorkspaceWrite: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			toolset, err := agent.StaticTools(definitions...)
			if err != nil {
				t.Fatal(err)
			}
			model := &editDiagnosticsModel{}
			definition := agent.Definition{
				Name: kind, Instructions: "Execute the requested edit.", Model: model, Tools: toolset,
				Middlewares: []agent.Middleware{NewOrchestratorMiddleware(OrchestratorConfig{
					AgentKind: kind, PolicyKind: agentrun.AgentKindIDE, Workspace: workspace,
				})},
				ResultProcessor: agenttoolresult.Standard(agenttoolresult.Policy{MaxBytes: 4096}),
			}
			result, err, projections := runPublicToolLifecycle(t, definition, agentrun.NewObserver(nil, ""))
			if err != nil || result.Status != agent.ResultCompleted {
				t.Fatalf("public Agent failed: result=%+v err=%v", result, err)
			}
			var feedback *agent.Message
			for _, message := range model.messages {
				if message.Role == "tool" && message.ToolName == "edit" {
					feedback = message
				}
			}
			if feedback == nil {
				t.Fatal("model did not receive the edit result")
			}
			if display := projections["edit"].DisplayContent; display != feedback.Content {
				t.Fatalf("display lost structured edit diagnostics: display=%q model=%q", display, feedback.Content)
			}
			var receipt struct {
				WorkspaceMutated bool `json:"workspace_mutated"`
				Details          struct {
					Issues []struct {
						Index   int            `json:"edit_index"`
						Code    string         `json:"code"`
						Details map[string]any `json:"details"`
					} `json:"issues"`
				} `json:"details"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(feedback.Content, "[tool error]\n")), &receipt); err != nil {
				t.Fatalf("model received invalid feedback: %q err=%v", feedback.Content, err)
			}
			issues := receipt.Details.Issues
			if receipt.WorkspaceMutated || len(issues) != 2 || issues[0].Index != 1 || issues[0].Code != "not_unique" || issues[1].Index != 2 || issues[1].Code != "not_found" {
				t.Fatalf("model lost batch failure details: %s", feedback.Content)
			}
			wantMatches := []any{
				map[string]any{"line": float64(3), "column": float64(1), "context_start_line": float64(1), "context": original, "context_truncated": false},
				map[string]any{"line": float64(6), "column": float64(1), "context_start_line": float64(1), "context": original, "context_truncated": false},
			}
			if !reflect.DeepEqual(issues[0].Details["matches"], wantMatches) ||
				!strings.Contains(feedback.Content, "expand old_string") || !strings.Contains(feedback.Content, "Read the target file again") {
				t.Fatalf("model lost recovery guidance: %s", feedback.Content)
			}
			content, err := os.ReadFile(path)
			if err != nil || string(content) != original {
				t.Fatalf("failed batch changed the file: %q err=%v", content, err)
			}
		})
	}
}

type editDiagnosticsModel struct{ messages []*agent.Message }

func (model *editDiagnosticsModel) Generate(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	first := model.messages == nil
	model.messages = messages
	if first {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: "edit-diagnostics", Type: "function",
			Function: agent.FunctionCall{
				Name: "edit", Arguments: `{"path":"state.md","edits":[{"old_string":"existing","new_string":"updated"},{"old_string":"same","new_string":"unique"},{"old_string":"missing","new_string":"found"}]}`,
			},
		}}), nil
	}
	return agent.AssistantMessage("done", nil), nil
}

func (model *editDiagnosticsModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}
