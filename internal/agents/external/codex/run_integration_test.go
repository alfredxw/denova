package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/attachment"
	"denova/internal/agents/external"
	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

// This opt-in fixture exercises the installed executable, without account
// credentials or remote model traffic. It tests the real protocol and sandbox,
// not the quality or capabilities of an actual model.
func TestInstalledAppServerHostBoundary(t *testing.T) {
	executable := os.Getenv("DENOVA_TEST_CODEX_EXE")
	if executable == "" {
		t.Skip("set DENOVA_TEST_CODEX_EXE to test the installed App Server")
	}
	versionOutput, err := exec.Command(executable, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	installedVersion := strings.TrimPrefix(strings.TrimSpace(string(versionOutput)), "codex-cli ")
	t.Logf("Installed App Server version: %s", installedVersion)
	for _, scenario := range []string{"ask", "patch", "cancel", "images", "api"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			project := t.TempDir()
			personal := t.TempDir()
			t.Setenv("HOME", personal)
			t.Setenv("USERPROFILE", personal)
			t.Setenv("DENOVA_FIXTURE_CODEX_KEY", "overridden-by-codex-dotenv")
			guard := filepath.Join(project, "chapter.txt")
			if err := os.WriteFile(guard, []byte("Original content.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var requests []json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/responses" {
					http.NotFound(w, r)
					return
				}
				if r.Header.Get("Authorization") != "Bearer fixture-only" {
					http.Error(w, "shared home credential was not loaded", http.StatusUnauthorized)
					return
				}
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				mu.Lock()
				if scenario == "api" && (!bytes.Contains(body, []byte(`"model":"fixture-api-model"`)) || r.Header.Get("X-Tenant") != "api-tenant" || r.Header.Get("X-Unexpected") != "") {
					t.Error("API model or custom header was not applied")
				}
				requests = append(requests, body)
				ordinal := len(requests)
				mu.Unlock()
				var item map[string]any
				if ordinal == 1 && scenario == "patch" {
					item = map[string]any{"type": "custom_tool_call", "id": "fc_patch", "call_id": "call_patch", "name": "apply_patch", "input": "*** Begin Patch\n*** Update File: " + filepath.ToSlash(guard) + "\n@@\n-Original content.\n+Unauthorized write.\n*** End Patch", "status": "completed"}
				} else if ordinal == 1 {
					item = map[string]any{"type": "function_call", "id": "fc_question", "call_id": "call_question", "name": "ask", "arguments": `{"questions":[{"id":"tone","prompt":"Which tone?"}]}`, "status": "completed"}
				} else {
					item = map[string]any{"type": "message", "id": "msg_final", "role": "assistant", "status": "completed", "content": []map[string]any{{"type": "output_text", "text": "The tool result was received.", "annotations": []any{}}}}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Connection", "close")
				emit := func(event map[string]any) {
					body, _ := json.Marshal(event)
					_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], body)
				}
				id := fmt.Sprintf("response_%d", ordinal)
				emit(map[string]any{"type": "response.created", "response": map[string]any{"id": id, "object": "response", "status": "in_progress", "output": []any{}}})
				emit(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
				if item["type"] == "message" {
					emit(map[string]any{"type": "response.output_text.delta", "item_id": item["id"], "output_index": 0, "content_index": 0, "delta": "The tool result was received."})
				}
				emit(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
				emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "object": "response", "status": "completed", "output": []any{item}, "usage": map[string]int{"input_tokens": 20, "output_tokens": 10, "total_tokens": 30}}})
			}))
			defer server.Close()
			home := t.TempDir()
			configuration := fmt.Sprintf("model = \"gpt-5.5\"\nmodel_provider = \"fixture\"\nmodel_context_window = 128000\n[model_providers.fixture]\nname = \"Local fixture\"\nbase_url = %q\nwire_api = \"responses\"\nenv_key = \"DENOVA_FIXTURE_CODEX_KEY\"\n[features]\nenable_request_compression = false\n", server.URL+"/v1")
			if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			// Let the actual CLI load .env from the selected home. Denova must
			// not duplicate its dotenv/config/provider resolution rules.
			if err := os.WriteFile(filepath.Join(home, ".env"), []byte("DENOVA_FIXTURE_CODEX_KEY=fixture-only\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			options := ProcessOptions{Executable: executable, Home: home}
			if scenario == "api" {
				options.API = &config.ResolvedModelSettings{BaseURL: server.URL + "/v1", APIKey: "fixture-only", Model: "fixture-api-model", Headers: map[string]string{"X-Tenant": "api-tenant"}}
				configuration = "model_provider = \"denova\"\n[model_providers.denova]\nname=\"Wrong\"\nbase_url=\"http://127.0.0.1:1\"\nhttp_headers={\"X-Unexpected\"=\"inherited-secret\"}\n[features]\nenable_request_compression=false\n"
				if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(configuration), 0600); err != nil {
					t.Fatal(err)
				}
			}
			client, err := Connect(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			defer func() {
				data, err := os.ReadFile(filepath.Join(home, "config.toml"))
				if err != nil || string(data) != configuration {
					t.Error("runtime changed CLI configuration")
				}
			}()
			if client.Version() != installedVersion {
				t.Fatalf("reported engine version = %q, want installed version %q", client.Version(), installedVersion)
			}
			host := &integrationHost{started: make(chan struct{}, 1), answer: make(chan struct{})}
			input := external.Input{
				Selection:    config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "gpt-5.5"}},
				Instructions: "Use the host tools to carry out the user request.", Text: "Ask which tone to use, then report the result.",
				History: []external.Message{{Role: "user", Text: "Previous user context."}, {Role: "assistant", Text: "Previous confirmed content."}},
				Tools:   []external.Tool{{Name: "ask", Description: "Ask the user for information.", Schema: json.RawMessage(`{"type":"object","properties":{"questions":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"prompt":{"type":"string"}},"required":["id","prompt"]}}},"required":["questions"]}`)}},
			}
			if scenario == "api" {
				input.Selection.Codex = &config.CodexRuntimeSettings{ProfileID: "api-profile"}
			}
			if scenario == "patch" {
				input.Selection.Codex.Sandbox = config.CodexReadOnly
			}
			if scenario == "images" {
				var picture bytes.Buffer
				if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
					t.Fatal(err)
				}
				files, err := attachment.Materialize(t.TempDir(), attachment.SessionScope("fixture"), "images", []attachment.Upload{{Name: "reference.png", DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(picture.Bytes())}})
				if err != nil {
					t.Fatal(err)
				}
				input.Attachments, input.History[0].Attachments, host.images = files, files, files
			}
			type outcome struct {
				result external.Result
				err    error
			}
			done := make(chan outcome, 1)
			attemptContext, abort := context.WithCancel(ctx)
			defer abort()
			go func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						done <- outcome{err: fmt.Errorf("fixture run panic: %v", recovered)}
					}
				}()
				result, err := client.Run(attemptContext, input, host)
				done <- outcome{result, err}
			}()
			if scenario != "patch" {
				select {
				case <-host.started:
				case result := <-done:
					t.Fatalf("attempt ended before question: %v", result.err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				// RPC traffic continues while the host awaits an answer. There is
				// no model/answer deadline in the runtime implementation.
				if err := client.call(ctx, "account/read", map[string]bool{"refreshToken": false}, nil); err != nil {
					t.Fatal(err)
				}
				if scenario == "cancel" {
					abort()
					select {
					case result := <-done:
						if !errors.Is(result.err, context.Canceled) {
							t.Fatalf("question did not cancel: %v", result.err)
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					close(host.answer)
					// Cancelling one thread leaves the shared connection usable.
					result, err := client.Run(ctx, input, host)
					if err != nil || result.Text != "The tool result was received." {
						t.Fatalf("connection failed after cancellation: %#v, %v", result, err)
					}
					mu.Lock()
					defer mu.Unlock()
					if len(requests) != 2 {
						t.Fatalf("cancelled attempt continued model execution, requests=%d", len(requests))
					}
					return
				}
				close(host.answer)
			}
			select {
			case result := <-done:
				if result.err != nil {
					t.Fatal(result.err)
				}
				if result.result.Text != "The tool result was received." || host.text.String() != result.result.Text {
					t.Fatalf("stream/final duplicated or lost: %#v / %q", result, host.text.String())
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			mu.Lock()
			defer mu.Unlock()
			if len(requests) != 2 || !strings.Contains(string(requests[0]), "Previous confirmed content.") {
				t.Fatalf("canonical history was not injected, requests=%d", len(requests))
			}
			if scenario == "images" {
				if strings.Count(string(requests[0]), "data:image/png;base64,") != 2 || strings.Count(string(requests[1]), "data:image/png;base64,") != 3 {
					t.Fatalf("App Server image payload counts: first png=%d all=%d; second png=%d all=%d", strings.Count(string(requests[0]), "data:image/png;base64,"), strings.Count(string(requests[0]), "data:image/"), strings.Count(string(requests[1]), "data:image/png;base64,"), strings.Count(string(requests[1]), "data:image/"))
				}
			}
			if scenario == "patch" {
				content, err := os.ReadFile(guard)
				if err != nil || string(content) != "Original content.\n" {
					t.Fatalf("built-in patch escaped host boundary: %q, %v", content, err)
				}
				if !strings.Contains(string(requests[1]), "read-only sandbox") {
					t.Fatal("patch denial was not returned to the model")
				}
			} else if !strings.Contains(string(requests[1]), "Restrained") {
				t.Fatal("saved host answer was not returned to the model")
			}
		})
	}
}

type integrationHost struct {
	started chan struct{}
	answer  chan struct{}
	text    strings.Builder
	images  []agent.Attachment
}

func (host *integrationHost) Emit(event agentrun.Event) error {
	if event.Type == "chunk" {
		host.text.WriteString(event.DataString("content"))
	}
	return nil
}

func (host *integrationHost) CallTool(ctx context.Context, call external.ToolCall) (external.ToolResult, error) {
	if call.Name != "ask" {
		return external.ToolResult{}, fmt.Errorf("unexpected host tool %q", call.Name)
	}
	host.started <- struct{}{}
	select {
	case <-host.answer:
		return external.ToolResult{Success: true, Text: `{"status":"answered","answers":[{"question_id":"tone","custom_input":"Restrained"}]}`, Images: host.images}, nil
	case <-ctx.Done():
		return external.ToolResult{}, ctx.Err()
	}
}
