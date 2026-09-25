package external

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	agentrun "denova/internal/agents/run"
)

func TestCodexHostPermissionsControlActualWrites(t *testing.T) {
	for _, mode := range []config.CodexSandbox{config.CodexReadOnly, config.CodexWorkspaceWrite, config.CodexFullAccess, ""} {
		t.Run(string(mode), func(t *testing.T) {
			service, request, workspace := operationFixture(t)
			selection := *request.Input.Selection.Codex
			selection.Sandbox = mode
			base, _ := request.Session.RuntimeConfig()
			next, err := conversationconfig.Merge(&config.Config{}, base.Config, conversationconfig.Patch{Codex: &selection})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := request.Session.SetRuntimeConfig(next, base.Revision)
			if err != nil {
				t.Fatal(err)
			}
			request.Revision = snapshot.Revision
			request.Input.Selection = snapshot.Engine()
			history, err := PrepareHistory(t.Context(), request.Session)
			if err != nil {
				t.Fatal(err)
			}
			request.PreparedCursor = history.Cursor
			request.Adapter = adapterFunc(func(ctx context.Context, _ Input, host Host) (Result, error) {
				result, err := host.CallTool(ctx, ToolCall{ID: "write", Name: "write", Arguments: json.RawMessage(`{"path":"draft.md","content":"Saved draft."}`)})
				if err != nil {
					return Result{}, err
				}
				if result.Success != (mode != config.CodexReadOnly) {
					return Result{}, fmt.Errorf("mode %q returned %#v", mode, result)
				}
				return Result{Text: result.Text}, nil
			})
			op, err := service.Start(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if outcome := op.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("outcome: %#v", outcome)
			}
			content, err := os.ReadFile(filepath.Join(workspace, "draft.md"))
			if mode == config.CodexReadOnly {
				if !os.IsNotExist(err) {
					t.Fatalf("read-only mode wrote: %q, %v", content, err)
				}
			} else if err != nil || string(content) != "Saved draft." {
				t.Fatalf("write failed: %q, %v", content, err)
			}
		})
	}
}
