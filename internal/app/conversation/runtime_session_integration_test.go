package conversationapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/runtime/external/claude"
	"denova/internal/agents/runtime/external/codex"
	"denova/internal/hostruntime"
)

type runtimeProtocolTrace struct {
	adapter external.Adapter
	inputs  *[]external.Input
	results *[]external.Result
}

func (trace runtimeProtocolTrace) Version() string { return trace.adapter.Version() }
func (trace runtimeProtocolTrace) Run(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
	*trace.inputs = append(*trace.inputs, input)
	result, err := trace.adapter.Run(ctx, input, host)
	*trace.results = append(*trace.results, result)
	return result, err
}

type runtimeObservationHost struct{ events []agentrun.Event }

func (host *runtimeObservationHost) Emit(event agentrun.Event) error {
	host.events = append(host.events, event)
	return nil
}
func (*runtimeObservationHost) CallTool(context.Context, external.ToolCall) (external.ToolResult, error) {
	return external.ToolResult{}, fmt.Errorf("protocol fixture did not authorize tools")
}

// Use actual supported CLIs with local model fixtures. This checks persistence
// across process restarts, fork isolation, native compaction, cumulative usage
// and cold reconstruction without consuming account credentials or model calls.
func TestInstalledRuntimeSessionContinuity(t *testing.T) {
	for _, kind := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude} {
		t.Run(string(kind), func(t *testing.T) {
			executable := os.Getenv("DENOVA_TEST_" + strings.ToUpper(string(kind)) + "_EXE")
			if executable == "" {
				t.Skip("set DENOVA_TEST runtime executable")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			var mu sync.Mutex
			var requests []string
			var requestPause context.CancelCauseFunc
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "count_tokens") {
					fmt.Fprint(w, `{"input_tokens":100}`)
					return
				}
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				mu.Lock()
				requests = append(requests, string(body))
				ordinal := len(requests)
				pause := requestPause
				if pause != nil && strings.Contains(string(body), "WAIT_FOR_RUNTIME_PAUSE") {
					requestPause = nil
					mu.Unlock()
					pause(external.ErrSuspended)
					select {
					case <-r.Context().Done():
					case <-ctx.Done():
					}
					return
				}
				mu.Unlock()
				if strings.HasSuffix(r.URL.Path, "/compact") {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"output":[{"type":"compaction","encrypted_content":"fixture-compacted-state"}],"usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Connection", "close")
				emit := func(event map[string]any) {
					encoded, _ := json.Marshal(event)
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], encoded)
				}
				text := fmt.Sprintf("Recorded response %d.", ordinal)
				item := map[string]any{"type": "message", "id": fmt.Sprintf("msg_%d", ordinal), "role": "assistant", "status": "completed", "fixture_text": text,
					"content": []map[string]any{{"type": "output_text", "text": text, "annotations": []any{}}}}
				if kind == config.RuntimeClaude {
					emitClaudeProductFixture(emit, ordinal, item)
					return
				}
				delete(item, "fixture_text")
				id := fmt.Sprintf("response_%d", ordinal)
				emit(map[string]any{"type": "response.created", "response": map[string]any{"id": id, "object": "response", "status": "in_progress", "output": []any{}}})
				emit(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
				emit(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
				emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "object": "response", "status": "completed", "output": []any{item}, "usage": map[string]int{"input_tokens": 100, "output_tokens": 50, "total_tokens": 150}}})
			}))
			defer server.Close()
			personal := t.TempDir()
			for _, key := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "XDG_CONFIG_HOME", "CLAUDE_CONFIG_DIR"} {
				t.Setenv(key, personal)
			}
			for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
				t.Setenv(key, "")
			}
			if err := os.WriteFile(filepath.Join(personal, "config.toml"), []byte("[features]\nenable_request_compression=false\n"), 0600); err != nil {
				t.Fatal(err)
			}
			selection := config.RuntimeSelection{Kind: kind}
			if kind == config.RuntimeCodex {
				selection.Codex = &config.CodexRuntimeSettings{ProfileID: "fixture"}
			} else {
				selection.Claude = &config.ClaudeRuntimeSettings{ProfileID: "fixture"}
			}
			model := config.ResolvedModelSettings{ProfileID: "fixture", Model: "fixture-model", APIKey: "fixture-only", BaseURL: server.URL + "/v1"}
			var inputs []external.Input
			var results []external.Result
			runtime := &external.Runtime{Selection: selection, CacheRoot: t.TempDir(), Acquire: func(ctx context.Context) (external.Adapter, func(), error) {
				var connection external.Connection
				var err error
				if kind == config.RuntimeCodex {
					connection, err = codex.Connect(ctx, codex.ProcessOptions{Executable: executable, Home: personal, API: &model})
				} else {
					connection, err = claude.Connect(ctx, claude.ProcessOptions{Launch: hostruntime.ClaudeLaunch{Executable: executable}, API: &model})
				}
				if err != nil {
					return nil, nil, err
				}
				return runtimeProtocolTrace{connection, &inputs, &results}, func() { _ = connection.Close() }, nil
			}}
			host := &runtimeObservationHost{}
			request := external.SessionRequest{Key: "continuity", Boundary: "0", Input: external.Input{Instructions: "Use the provided conversation context.", Text: "Remember the harbor."}}
			first, err := runtime.Run(ctx, request, host)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Session.Close()
			if err := first.Session.Accept(ctx, "1"); err != nil {
				t.Fatal(err)
			}
			if _, err := first.Session.Evaluate(ctx, "PRIVATE_EVALUATOR_ONLY: assess the work without tools."); err != nil {
				t.Fatal(err)
			}
			if results[0].SessionID == "" || results[0].SessionID == results[1].SessionID {
				t.Fatal("evaluation did not fork the primary provider session")
			}
			_ = first.Session.Close()
			request.Boundary = "1"
			request.Input.History = []external.Message{{Role: "user", Text: request.Input.Text}, {Role: "assistant", Text: first.Text}}
			request.Input.Text = "Continue at the harbor."
			second, err := runtime.Run(ctx, request, host)
			if second.Session != nil {
				defer second.Session.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := second.Session.Accept(ctx, "2"); err != nil {
				t.Fatal(err)
			}
			_ = second.Session.Close()
			if inputs[2].SessionID != results[0].SessionID || len(inputs[2].History) != 0 || results[2].SessionID != results[0].SessionID {
				t.Fatal("second turn rebuilt an aligned provider session")
			}
			if second.Usage == nil || second.Usage.TotalTokens != 150 {
				t.Fatalf("usage counted history again: %+v", second.Usage)
			}
			mu.Lock()
			last := requests[len(requests)-1]
			mu.Unlock()
			if strings.Contains(last, "PRIVATE_EVALUATOR_ONLY") {
				t.Fatal("evaluation contaminated the primary provider context")
			}
			request.Input.History = append(request.Input.History, external.Message{Role: "user", Text: request.Input.Text}, external.Message{Role: "assistant", Text: second.Text})
			request.Boundary, request.Input.Text = "2", ""
			if _, err := runtime.Compact(ctx, request, host); err != nil {
				t.Fatal(err)
			}
			completed := false
			for _, event := range host.events {
				if event.Type == "context_compaction" && event.DataString("status") == "completed" {
					completed = true
				}
			}
			if !completed {
				t.Fatal("native compaction emitted no lifecycle event")
			}
			bindingFile := filepath.Join(inputs[0].Directory, "binding.json")
			data, err := os.ReadFile(bindingFile)
			if err != nil {
				t.Fatal(err)
			}
			var binding map[string]any
			if err := json.Unmarshal(data, &binding); err != nil {
				t.Fatal(err)
			}
			binding["session_id"] = "00000000-0000-4000-8000-000000000001"
			data, _ = json.Marshal(binding)
			if err := os.WriteFile(bindingFile, data, 0600); err != nil {
				t.Fatal(err)
			}
			request.Input.Text = "Recover the missing private transcript."
			rebuilt, err := runtime.Run(ctx, request, host)
			if rebuilt.Session != nil {
				defer rebuilt.Session.Close()
			}
			if err != nil {
				t.Fatalf("missing transcript was not reconstructed: %v", err)
			}
			if err := rebuilt.Session.Accept(ctx, "2"); err != nil {
				t.Fatal(err)
			}
			_ = rebuilt.Session.Close()
			if err := os.Remove(filepath.Join(inputs[0].Directory, "binding.json")); err != nil {
				t.Fatal(err)
			}
			request.Input.Text = "Recover the harbor context."
			third, err := runtime.Run(ctx, request, host)
			if err != nil {
				t.Fatal(err)
			}
			defer third.Session.Close()
			cold := inputs[len(inputs)-1]
			if cold.SessionID != "" || len(cold.History) != 4 {
				t.Fatalf("lost cache did not reconstruct full history: %+v", cold)
			}
			if err := third.Session.Accept(ctx, "2"); err != nil {
				t.Fatal(err)
			}
			_ = third.Session.Close()
			pauseCtx, cancelPause := context.WithCancelCause(ctx)
			defer cancelPause(context.Canceled)
			mu.Lock()
			requestPause = cancelPause
			mu.Unlock()
			request.Input.Text = "WAIT_FOR_RUNTIME_PAUSE"
			paused, err := runtime.Run(pauseCtx, request, host)
			if paused.Session != nil {
				defer paused.Session.Close()
			}
			if !errors.Is(err, context.Canceled) || !paused.Settled {
				t.Fatalf("provider did not acknowledge interruption before returning: result=%+v err=%v", paused.Result, err)
			}
			if err := paused.Session.Accept(ctx, "2"); err != nil {
				t.Fatal(err)
			}
			_ = paused.Session.Close()
			request.Input.Text = "Continue after the explicit pause."
			continued, err := runtime.Run(ctx, request, host)
			if continued.Session != nil {
				defer continued.Session.Close()
			}
			if err != nil || continued.SessionID != paused.SessionID || len(inputs[len(inputs)-1].History) != 0 {
				t.Fatalf("pause lost the aligned provider context: result=%+v err=%v", continued.Result, err)
			}
		})
	}
}
