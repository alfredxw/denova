package interactiveapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/i18n"
	"denova/internal/interactive"
)

func TestExternalGamePersistsToolNotInvokedResult(t *testing.T) {
	for _, locale := range []string{"en-US", "zh-CN"} {
		t.Run(locale, func(t *testing.T) {
			store, story, cfg := externalGameFixture(t)
			cfg.Language = locale
			cfg.ActiveAgentRuntime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}}
			args := `{"action":"Open the gate","intent":"Reach the path","challenge":"The gate is stuck","cost":"Making noise","state":"Standing by the gate","difficulty":"normal","outcomes":{"critical_success":{"result":"The gate opens"},"success":{"result":"The gate opens"},"failure":{"result":"The gate opens with noise"},"critical_failure":{"result":"The gate opens with loud noise"}}}`
			turn := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "cancel-tool", Message: "Open the gate"}, gameAdapterFunc(func(ctx context.Context, _ external.Input, host external.Host) (external.Result, error) {
				callCtx, cancel := context.WithCancel(ctx)
				cancel()
				result, err := host.CallTool(callCtx, external.ToolCall{ID: "rule", Name: "prepare_interactive_turn", Arguments: []byte(args)})
				if !errors.Is(err, context.Canceled) || result.Success || !strings.Contains(result.Text, "did not start") {
					t.Fatalf("lost nonexecution result: %+v err=%v", result, err)
				}
				return external.Result{}, err
			}))
			var results []agentrun.Event
			turn.config.Emit = func(event agentrun.Event) {
				if event.Type == "tool_result" {
					results = append(results, event)
				}
			}
			if outcome := turn.Wait(t.Context()); !errors.Is(outcome.Error, context.Canceled) {
				t.Fatalf("lost cancellation: %+v", outcome)
			}
			if len(results) != 1 || results[0].DataString("status") != "error" || results[0].DataString("content") != i18n.New(locale).T("agentRuntime.toolNotExecuted") {
				t.Fatalf("tool display did not settle in the selected language: %+v", results)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := interactive.NewStore(store.Root())
			defer reopened.Close()
			snapshot, err := reopened.Snapshot(story, "main")
			if err != nil {
				t.Fatal(err)
			}
			observations := 0
			for _, batch := range snapshot.PendingModelContextBatches {
				for _, message := range batch.Messages {
					if message.Role == "tool" && message.ToolName == "prepare_interactive_turn" && strings.Contains(message.Content, "did not start") {
						observations++
					}
				}
			}
			if observations != 1 {
				t.Fatalf("cold recovery lost the failed tool observation: %d", observations)
			}
		})
	}
}
