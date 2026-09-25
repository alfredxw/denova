package conversationapp

import (
	"context"
	"encoding/json"
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
	agents "denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	"denova/internal/book"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
	agent "github.com/alfredxw/denova/agent"
)

// Exercise the same preparation/acceptance/settlement used by Writing and
// AgentChat with the real executable. Only model responses are local fixtures;
// tools, Ask, project stores, receipts and callbacks are production paths.
func TestInstalledExternalProductExecution(t *testing.T) {
	for _, engine := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude} {
		t.Run(string(engine), func(t *testing.T) {
			executable := os.Getenv("DENOVA_TEST_" + strings.ToUpper(string(engine)) + "_EXE")
			if executable == "" {
				t.Skip("set the corresponding DENOVA_TEST runtime executable to exercise real CLI execution")
			}
			for _, source := range []string{"cli", "api"} {
				t.Run(source, func(t *testing.T) { testInstalledProductExecution(t, engine, executable, source) })
			}
		})
	}
}

func testInstalledProductExecution(t *testing.T, engine config.RuntimeID, executable, source string) {
	for _, scenario := range []struct{ name, kind, contract string }{
		{"writing", config.AgentKindIDE, ""}, {"general", config.AgentKindGeneral, ""},
		{"custom-writing", config.AgentKindIDE, "writing.primary.v1"}, {"custom-general", config.AgentKindGeneral, "project.general.v1"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			var mu sync.Mutex
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/compact") {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"output":[{"type":"compaction","encrypted_content":"fixture-compacted-state"}],"usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150}}`)
					return
				}
				if strings.Contains(r.URL.Path, "count_tokens") {
					fmt.Fprint(w, `{"input_tokens":100}`)
					return
				}
				if !strings.HasPrefix(r.URL.Path, "/v1/messages") && r.URL.Path != "/v1/responses" {
					http.NotFound(w, r)
					return
				}
				var raw json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				mu.Lock()
				requests = append(requests, string(raw))
				ordinal := len(requests)
				mu.Unlock()
				if source == "api" {
					var body struct {
						Model string `json:"model"`
					}
					if err := json.Unmarshal(raw, &body); err != nil || body.Model != "gateway-model" || r.Header.Get("X-Tenant") != "product-fixture" {
						t.Errorf("API model or tenant routing lost: model=%q", body.Model)
					}
				}
				var item map[string]any
				switch ordinal {
				case 1:
					item = map[string]any{"type": "function_call", "id": "fc_question", "call_id": "call_question", "name": "ask", "arguments": `{"questions":[{"id":"tone","prompt":"Which tone?"}]}`, "status": "completed"}
				case 2:
					item = map[string]any{"type": "function_call", "id": "fc_write", "call_id": "call_write", "name": "write", "arguments": `{"path":"draft.md","content":"The harbor was still.\n"}`, "status": "completed"}
				default:
					item = map[string]any{"type": "message", "id": "msg_final", "role": "assistant", "status": "completed", "content": []map[string]any{{"type": "output_text", "text": "The restrained draft is saved.", "annotations": []any{}}}}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				emit := func(event map[string]any) {
					body, _ := json.Marshal(event)
					_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], body)
				}
				if engine == config.RuntimeClaude {
					emitClaudeProductFixture(emit, ordinal, item)
					return
				}
				id := fmt.Sprintf("response_%d", ordinal)
				emit(map[string]any{"type": "response.created", "response": map[string]any{"id": id, "object": "response", "status": "in_progress", "output": []any{}}})
				emit(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
				emit(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
				emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "object": "response", "status": "completed", "output": []any{item}, "usage": map[string]any{"input_tokens": 100, "output_tokens": 50, "total_tokens": 150, "input_tokens_details": map[string]int{"cached_tokens": 20}, "output_tokens_details": map[string]int{"reasoning_tokens": 10}}}})
			}))
			defer server.Close()
			hostRoot := t.TempDir()
			for _, key := range []string{"LOCALAPPDATA", "APPDATA", "XDG_CONFIG_HOME", "HOME", "USERPROFILE"} {
				t.Setenv(key, hostRoot)
			}
			t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
			if engine == config.RuntimeClaude {
				t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(hostRoot, "claude"))
				t.Setenv("ANTHROPIC_API_KEY", "fixture-only")
				t.Setenv("ANTHROPIC_BASE_URL", server.URL)
				for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
					t.Setenv(key, "")
				}
			} else {
				home := filepath.Join(hostRoot, ".codex")
				t.Setenv("CODEX_HOME", home)
				if err := os.MkdirAll(home, 0o700); err != nil {
					t.Fatal(err)
				}
				configuration := fmt.Sprintf("model = \"gpt-5.5\"\nmodel_provider = \"fixture\"\n[model_providers.fixture]\nname = \"Local fixture\"\nbase_url = %q\nwire_api = \"responses\"\nexperimental_bearer_token = \"fixture-only\"\n[features]\nenable_request_compression = false\n", server.URL+"/v1")
				if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(configuration), 0o600); err != nil {
					t.Fatal(err)
				}

			}
			root := t.TempDir()
			workspace := filepath.Join(root, "project")
			if err := os.MkdirAll(workspace, 0o700); err != nil {
				t.Fatal(err)
			}
			preference := &config.RuntimePreferences{Selected: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "gpt-5.5"}}
			if engine == config.RuntimeClaude {
				preference = &config.RuntimePreferences{Selected: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "sonnet"}}
			}
			cfg := config.Config{DenovaDir: root, Workspace: workspace, ProjectID: "product-fixture", ProjectStoreDir: filepath.Join(root, "store"), AgentRuntimes: config.AgentRuntimeSettings{IDE: preference, General: preference}}
			if source == "api" {
				protocol := "openai-responses"
				preference.Codex = &config.CodexRuntimeSettings{ProfileID: "api-profile"}
				if engine == config.RuntimeClaude {
					protocol = "anthropic-messages"
					preference.Codex = nil
					preference.Claude = &config.ClaudeRuntimeSettings{ProfileID: "api-profile"}
					t.Setenv("ANTHROPIC_BASE_URL", "http://127.0.0.1:1")
				}
				cfg.ModelEndpoints = []config.ModelEndpointSettings{{ID: "api-endpoint", Provider: "openai-compatible", Protocol: protocol, BaseURL: server.URL + "/v1", APIKey: "api-fixture-only", Headers: map[string]string{"X-Tenant": "product-fixture"}}}
				cfg.ModelProfiles = []config.ModelProfileSettings{{ID: "api-profile", EndpointID: "api-endpoint", Model: "gateway-model"}}
			}
			customID := ""
			if scenario.contract != "" {
				customID = "fixture-custom"
				cfg.CustomAgents = []config.CustomAgentConfig{{ID: customID, Name: "Fixture author", Contract: scenario.contract, Runtime: preference, Instructions: "Keep the opening restrained. CUSTOM_ROLE_SENTINEL"}}
			}
			selection, err := conversationconfig.DefaultWithCustomAgent(&cfg, scenario.kind, customID)
			if err != nil {
				t.Fatal(err)
			}
			store, err := session.NewStore(filepath.Join(cfg.ProjectStoreDir, "sessions"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			sess, err := store.GetOrCreateWithRuntimeConfig("session-fixture", selection)
			if err != nil {
				t.Fatal(err)
			}
			runtime := Runtime{ProjectID: cfg.ProjectID, ProjectStore: cfg.ProjectStoreDir, ProjectType: projectdomain.TypeGeneral, AgentKind: scenario.kind, Session: sess, Config: cfg, Workspace: workspace, BookService: book.NewService(workspace)}
			runtime.ExecutionRuntime = agentexecution.NewEphemeralRuntime()
			defer runtime.ExecutionRuntime.Close(context.Background())
			bindingOptions := agentrun.Options{ProjectID: cfg.ProjectID, StateRoot: cfg.ProjectStoreDir, Workspace: workspace, SessionID: sess.ID, AgentKind: scenario.kind, Mode: "agent_chat"}
			if scenario.kind == config.AgentKindIDE {
				runtime.ProjectType, runtime.State = projectdomain.TypeBook, book.NewState(workspace)
				if err := runtime.State.InitWorkspace(); err != nil {
					t.Fatal(err)
				}
			}
			request := agentchat.ChatRequest{CommandID: "product-command", Message: "Ask for a tone, then save an opening in draft.md."}
			runtime, request, err = Prepare(ctx, runtime, request)
			if err != nil {
				t.Fatal(err)
			}
			engines := agentruntime.NewEngines()
			defer engines.Close()
			built, err := BuildExecution(ctx, runtime, agents.AgentHostCapabilities{}, engines, "")
			if err != nil {
				t.Fatal(err)
			}
			answered, verified, inputCommitted := false, false, false
			var usage map[string]any
			var toolRunID string
			var eventError error
			emit := func(event agentrun.Event) {
				if event.Type == "tool_result" {
					toolRunID = event.DataString("run_id")
				}
				if event.Type == "token_usage" {
					usage, _ = event.Data.(map[string]any)
					return
				}
				if event.Type != "ask_pending" {
					return
				}
				data, _ := json.Marshal(event.Data)
				var pending session.AskInteraction
				_ = json.Unmarshal(data, &pending)
				id := pending.ID
				_, _, err := engines.Operations.ResolveAsk(ctx, runtime.ProjectID, runtime.Session, id, session.AskAnswered, []agentconversation.HostAskAnswer{{QuestionID: "tone", CustomInput: "Restrained"}}, "")
				answered, eventError = err == nil, err
				if err != nil {
					cancel()
				}
			}
			options := agentrun.Options{InputCommitEffect: agentrun.InputCommitEffectFuncs{ApplyFunc: func(context.Context, agentrun.InputCommitEffectRequest) error { inputCommitted = true; return nil }}, OnMutationsVerified: func(_ context.Context, mutations []agenttool.Mutation, _ agenttool.Verification) {
				verified = len(mutations) > 0
			}}
			options.ProjectID, options.StateRoot, options.Workspace = bindingOptions.ProjectID, bindingOptions.StateRoot, bindingOptions.Workspace
			options.SessionID, options.AgentKind, options.Mode = bindingOptions.SessionID, bindingOptions.AgentKind, bindingOptions.Mode
			op, err := built.Start(ctx, request, ProjectConversation(runtime, request), options, emit)
			if err != nil {
				t.Fatal(err)
			}
			outcome := op.Wait(ctx)
			if outcome.Status != agentrun.OutcomeCompleted || eventError != nil {
				t.Fatalf("product execution: %s, %v, %v", outcome.Status, outcome.Error, eventError)
			}
			if !answered || !verified || !inputCommitted || !op.OutputCommitted() {
				t.Fatalf("missing shared effects: answer=%v verification=%v input=%v output=%v", answered, verified, inputCommitted, op.OutputCommitted())
			}
			if usage["total_tokens"] != float64(450) || usage["model_calls"] != nil {
				t.Fatalf("incorrect product usage: %+v", usage)
			}
			body, err := os.ReadFile(filepath.Join(workspace, "draft.md"))
			if err != nil || string(body) != "The harbor was still.\n" {
				t.Fatalf("domain write: %q, %v", body, err)
			}
			changes, err := workspacechange.ForWorkspaceAt(workspace, runtime.ProjectStore)
			if err != nil {
				t.Fatal(err)
			}
			group, err := changes.GetGroup(ctx, toolRunID)
			if err != nil || len(group.ChangeSets) != 1 || group.RunID != toolRunID {
				t.Fatalf("missing original domain receipt: %+v, %v", group, err)
			}
			history, err := external.PrepareHistory(ctx, sess)
			if err != nil {
				t.Fatal(err)
			}
			messages, err := history.Messages(ctx)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(messages)
			if !strings.Contains(string(encoded), "Restrained") || !strings.Contains(string(encoded), "The restrained draft is saved.") {
				t.Fatal("canonical history lost question answer or output")
			}
			canonical, err := sess.ReadCanonicalMessages(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(canonical) != 6 || len(canonical[1].ToolCalls) != 1 || canonical[2].ToolCallID != canonical[1].ToolCalls[0].ID || canonical[2].ToolName != "ask" || canonical[4].ToolName != "write" {
				body, _ := json.Marshal(canonical)
				t.Fatalf("Native projection lost completed external observations: %s", body)
			}
			compacted, err := built.Compact(ctx, "manual-compaction", options)
			if err != nil || !compacted.Triggered || !compacted.RuntimeManaged {
				t.Fatalf("manual product compaction: %+v %v", compacted, err)
			}
			mu.Lock()
			requestsAfterCompaction := len(requests)
			mu.Unlock()
			if replay, err := built.Compact(ctx, "manual-compaction", options); err != nil || replay != compacted {
				t.Fatalf("manual compaction retry: %+v %v", replay, err)
			}
			mu.Lock()
			if len(requests) != requestsAfterCompaction {
				t.Error("manual compaction retry ran the provider again")
			}
			mu.Unlock()
			// Switch this exact logical Session and execute the real Native loop.
			// Only its model is a fixture; preparation and canonical synchronization
			// remain the production implementation.
			nativeRuntime := agentexecution.NewEphemeralRuntime()
			defer nativeRuntime.Close(context.Background())
			runtime.ExecutionRuntime = nativeRuntime
			current, _ := sess.RuntimeConfig()
			next := current.Config
			next.Runtime = &config.RuntimeSelection{Kind: config.RuntimeNative}
			next.ProfileID, next.ThinkingLevel = "fixture", "off"
			opts := agentrun.Options{ProjectID: cfg.ProjectID, StateRoot: cfg.ProjectStoreDir, Workspace: workspace, SessionID: sess.ID, AgentKind: scenario.kind, Mode: "agent_chat"}
			if _, err := engines.ApplyEngineSelection(ctx, nativeRuntime, sess, opts, next, current.Revision, config.Config{}); err != nil {
				t.Fatal(err)
			}
			nativeSettings := `[[model_endpoints]]
id = "fixture"
provider = "openai-compatible"
protocol = "openai-chat-completions"
api_key = "fixture-only"
base_url = "http://127.0.0.1:1/v1"
[[model_profiles]]
id = "fixture"
endpoint_id = "fixture"
model = "fixture"
context_window_tokens = 100000
`
			if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(nativeSettings), 0600); err != nil {
				t.Fatal(err)
			}
			nativeRequest := agentchat.ChatRequest{CommandID: "native-continuation", Message: "Continue from the confirmed saved draft."}
			runtime, nativeRequest, err = Prepare(ctx, runtime, nativeRequest)
			if err != nil {
				t.Fatal(err)
			}
			nativeBuilt, err := BuildExecution(ctx, runtime, agents.AgentHostCapabilities{}, engines, "")
			if err != nil {
				t.Fatal(err)
			}
			model := &continuationFixtureModel{}
			nativeBuilt.native.Definition.Model = model
			nativeOp, err := nativeBuilt.Start(ctx, nativeRequest, ProjectConversation(runtime, nativeRequest), opts, nil)
			if err != nil {
				t.Fatal(err)
			}
			if outcome := nativeOp.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("Native continuation: %+v", outcome)
			}
			modelInput, _ := json.Marshal(model.input)
			if !strings.Contains(string(modelInput), "Restrained") || !strings.Contains(string(modelInput), "draft.md") || !strings.Contains(string(modelInput), "The restrained draft is saved.") {
				t.Fatalf("Native model lost confirmed external history: %s", modelInput)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(requests) != requestsAfterCompaction || !strings.Contains(requests[1], "Restrained") {
				t.Fatalf("engine did not continue with answer, requests=%d", len(requests))
			}
			if customID != "" && !strings.Contains(requests[0], "CUSTOM_ROLE_SENTINEL") {
				t.Fatal("custom Agent instructions were lost")
			}
		})
	}
}

type continuationFixtureModel struct{ input []*agent.Message }

func (m *continuationFixtureModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	m.input = input
	return &agent.Message{Role: agent.Assistant, Content: "Native continuation preserved the draft.", ResponseMeta: &agent.ResponseMeta{FinishReason: "stop"}}, nil
}
func (m *continuationFixtureModel) Stream(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := m.Generate(ctx, input, options...)
	return agent.StreamReaderFromArray([]*agent.Message{message}), err
}

// Emit the actual Anthropic streaming protocol consumed by the CLI, including
// incremental tool JSON. Returning whole input only on block_start is not a
// valid substitute: the CLI assembles its tool arguments from deltas.
func emitClaudeProductFixture(emit func(map[string]any), ordinal int, item map[string]any) {
	id := fmt.Sprintf("msg_%d", ordinal)
	emit(map[string]any{"type": "message_start", "message": map[string]any{"id": id, "type": "message", "role": "assistant", "model": "claude-sonnet-5", "content": []any{}, "stop_reason": nil, "usage": map[string]int{"input_tokens": 100, "output_tokens": 0}}})
	var block, delta map[string]any
	stop := "end_turn"
	if item["type"] == "function_call" {
		stop = "tool_use"
		block = map[string]any{"type": "tool_use", "id": item["call_id"], "name": "mcp__denova__" + item["name"].(string), "input": map[string]any{}}
		delta = map[string]any{"type": "input_json_delta", "partial_json": item["arguments"]}
	} else {
		block = map[string]any{"type": "text", "text": ""}
		text, _ := item["fixture_text"].(string)
		if text == "" {
			text = "The restrained draft is saved."
		}
		delta = map[string]any{"type": "text_delta", "text": text}
	}
	emit(map[string]any{"type": "content_block_start", "index": 0, "content_block": block})
	emit(map[string]any{"type": "content_block_delta", "index": 0, "delta": delta})
	emit(map[string]any{"type": "content_block_stop", "index": 0})
	emit(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop}, "usage": map[string]int{"output_tokens": 50}})
	emit(map[string]any{"type": "message_stop"})
}
