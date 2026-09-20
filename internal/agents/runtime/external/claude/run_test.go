package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/hostruntime"
	agent "github.com/alfredxw/denova/agent"
)

type testHost struct {
	mu     sync.Mutex
	text   string
	calls  []external.ToolCall
	events []agentrun.Event
	call   func(context.Context, external.ToolCall) (external.ToolResult, error)
}

func (h *testHost) Emit(event agentrun.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	if event.Type == "chunk" {
		h.text += event.Data.(map[string]any)["content"].(string)
	}
	return nil
}
func (h *testHost) CallTool(ctx context.Context, call external.ToolCall) (external.ToolResult, error) {
	h.mu.Lock()
	h.calls = append(h.calls, call)
	h.mu.Unlock()
	if h.call != nil {
		return h.call(ctx, call)
	}
	return external.ToolResult{Text: "Saved.", Success: true}, nil
}

func TestStreamMergesWrappersAndDeltas(t *testing.T) {
	start := `{"type":"stream_event","event":{"type":"message_start","message":{"id":"m1"}}}`
	delta := `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}`
	wrapper := `{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"Hello"}]}}`
	for _, frames := range [][]string{{start, delta, wrapper}, {start, wrapper, delta}, {wrapper}} {
		var output streamOutput
		host := &testHost{}
		for _, frame := range frames {
			if err := output.feed([]byte(frame), host); err != nil {
				t.Fatal(err)
			}
		}
		if output.terminal {
			t.Fatal("assistant message completed the attempt")
		}
		if err := output.feed([]byte(`{"type":"result","subtype":"success","parent_tool_use_id":"child"}`), host); err != nil {
			t.Fatal(err)
		}
		if output.terminal {
			t.Fatal("child result completed parent")
		}
		if err := output.feed([]byte(`{"type":"result","subtype":"success","usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":20,"cache_creation_input_tokens":5}}`), host); err != nil {
			t.Fatal(err)
		}
		if output.result().Text != "Hello" || host.text != "Hello" || output.usage.TotalTokens != 39 {
			t.Fatalf("unexpected output: %#v, %s", output.result(), host.text)
		}
	}
}

func TestStreamRejectsFailedResult(t *testing.T) {
	var output streamOutput
	if err := output.feed([]byte(`{"type":"result","subtype":"error_during_execution","is_error":true}`), &testHost{}); err == nil {
		t.Fatal("error result succeeded")
	}
}

func TestInputHistoryIsOneTurn(t *testing.T) {
	body, err := encodeInput(external.Input{History: []external.Message{{Role: "user", Text: "Old request"}, {Role: "assistant", Text: "Old answer"}}, Text: "New request"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "\n") != 1 || !strings.Contains(string(body), "Old answer") || !strings.Contains(string(body), "New request") {
		t.Fatalf("invalid input: %s", body)
	}
}

func TestInstalledClaudeToolLoop(t *testing.T) {
	for _, source := range []string{"cli", "api"} {
		t.Run(source, func(t *testing.T) { testInstalledClaudeToolLoop(t, source) })
	}
}

func testInstalledClaudeToolLoop(t *testing.T, source string) {
	exe := os.Getenv("DENOVA_TEST_CLAUDE_EXE")
	if exe == "" {
		t.Skip("set DENOVA_TEST_CLAUDE_EXE to exercise the installed CLI against a local model fixture")
	}
	personal := t.TempDir()
	t.Setenv("HOME", personal)
	t.Setenv("USERPROFILE", personal)
	ambientSkill := filepath.Join(personal, ".agents", "skills", "ambient-disabled")
	if err := os.MkdirAll(ambientSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ambientSkill, "SKILL.md"), []byte("---\nname: ambient-disabled\ndescription: ambient-skill-must-not-bypass-library\n---\nAmbient body.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var mu sync.Mutex
	requests := 0
	imageSeen := false
	var modelTools []string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if source == "api" && strings.Contains(r.URL.Path, "messages") && (r.Header.Get("X-Tenant") != "api-fixture" || r.Header.Get("X-Api-Key") != "fixture-only") {
			t.Error("API headers were not applied")
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/messages") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "count_tokens") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"input_tokens":100}`)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if source == "api" && request["model"] != "gateway-model" {
			t.Error("API model was not applied")
		}
		mu.Lock()
		requests++
		raw, _ := json.Marshal(request)
		if strings.Contains(string(raw), "ambient-skill-must-not-bypass-library") {
			t.Error("CLI injected an ambient Skill outside the Denova library")
		}
		if strings.Contains(string(raw), `"media_type":"image/png"`) {
			imageSeen = true
		}
		if tools, ok := request["tools"].([]any); ok {
			for _, tool := range tools {
				modelTools = append(modelTools, tool.(map[string]any)["name"].(string))
			}
		}
		n := requests
		mu.Unlock()
		var block map[string]any
		stop := "tool_use"
		switch n {
		case 1:
			block = map[string]any{"type": "tool_use", "id": "ask_1", "name": "mcp__denova__ask", "input": map[string]any{"questions": []any{map[string]any{"id": "tone", "prompt": "Which tone?"}}}}
		case 2:
			block = map[string]any{"type": "tool_use", "id": "write_1", "name": "mcp__denova__write", "input": map[string]any{"path": "draft.md", "content": "Saved draft"}}
		case 3:
			block = map[string]any{"type": "tool_use", "id": "plan_1", "name": "TaskCreate", "input": map[string]any{"subject": "Verify the draft", "description": "Check the saved draft"}}
		case 4:
			block = map[string]any{"type": "tool_use", "id": "plan_2", "name": "TaskUpdate", "input": map[string]any{"taskId": "1", "status": "completed"}}
		default:
			block = map[string]any{"type": "text", "text": "Draft saved."}
			stop = "end_turn"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v map[string]any) {
			body, _ := json.Marshal(v)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", v["type"], body)
		}
		emit(map[string]any{"type": "message_start", "message": map[string]any{"id": fmt.Sprintf("msg_%d", n), "type": "message", "role": "assistant", "model": "claude-sonnet-4-5", "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 100, "output_tokens": 0}}})
		initial := map[string]any{"type": block["type"]}
		if block["type"] == "tool_use" {
			initial["id"], initial["name"], initial["input"] = block["id"], block["name"], map[string]any{}
		} else {
			initial["text"] = ""
		}
		emit(map[string]any{"type": "content_block_start", "index": 0, "content_block": initial})
		delta := map[string]any{"type": "text_delta", "text": block["text"]}
		if block["type"] == "tool_use" {
			raw, _ := json.Marshal(block["input"])
			delta = map[string]any{"type": "input_json_delta", "partial_json": string(raw)}
		}
		emit(map[string]any{"type": "content_block_delta", "index": 0, "delta": delta})
		emit(map[string]any{"type": "content_block_stop", "index": 0})
		emit(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 10}})
		emit(map[string]any{"type": "message_stop"})
	}))
	defer model.Close()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "fixture-only")
	t.Setenv("ANTHROPIC_BASE_URL", model.URL)
	for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
		t.Setenv(key, "")
	}
	options := ProcessOptions{Launch: hostruntime.ClaudeLaunch{Executable: exe}}
	if source == "api" {
		t.Setenv("ANTHROPIC_BASE_URL", "http://127.0.0.1:1")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "wrong-token")
		t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
		options.API = &config.ResolvedModelSettings{BaseURL: model.URL + "/v1", APIKey: "fixture-only", Model: "gateway-model", Headers: map[string]string{"X-Tenant": "api-fixture"}}
	}
	c, err := Connect(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if state, err := c.Check(ctx); err != nil || state.Status != "ready" {
		t.Fatalf("authentication: %#v %v", state, err)
	}
	host := &testHost{call: func(ctx context.Context, call external.ToolCall) (external.ToolResult, error) {
		if call.Name == "ask" {
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				return external.ToolResult{}, ctx.Err()
			}
		}
		return external.ToolResult{Text: "Confirmed", Success: true}, nil
	}}
	input := external.Input{Selection: config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "sonnet"}}, Instructions: "Use supplied tools to answer the current request.", Text: "Ask then save a draft.", Tools: []external.Tool{{Name: "ask", Description: "Ask the user", Schema: json.RawMessage(`{"type":"object","properties":{"questions":{"type":"array"}}}`)}, {Name: "write", Description: "Write a draft", Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}}}`)}}}
	if source == "api" {
		input.Selection.Claude = &config.ClaudeRuntimeSettings{ProfileID: "api-profile"}
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(t.TempDir(), "fixture.png")
	if err := os.WriteFile(imagePath, picture.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(picture.Bytes())
	input.Attachments = []agent.Attachment{{ID: "image-1", Name: "fixture.png", MediaType: "image/png", Size: int64(picture.Len()), Path: "fixture.png", RuntimePath: imagePath, SHA256: hex.EncodeToString(digest[:])}}
	result, err := c.Run(ctx, input, host)
	if err != nil {
		t.Fatalf("run: %v; requests=%d; calls=%#v; result=%#v", err, requests, host.calls, result)
	}
	if result.Text != "Draft saved." || len(host.calls) != 2 || host.calls[0].Name != "ask" || host.calls[0].ID != "ask_1" || host.calls[1].Name != "write" || host.calls[1].ID != "write_1" {
		t.Fatalf("unexpected tool loop: %#v calls=%#v", result, host.calls)
	}
	if result.Usage == nil || result.Usage.TotalTokens == 0 {
		t.Fatal("missing usage")
	}
	if !imageSeen {
		t.Fatal("CLI model request lost image input")
	}
	var plan []agent.TodoItem
	for _, event := range host.events {
		if event.Type == "todo_updated" {
			plan, err = external.PlanItems(event)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(plan) != 1 || plan[0].Text != "Verify the draft" || plan[0].Status != agent.TodoCompleted {
		t.Fatalf("native plan=%+v", plan)
	}
	for _, tool := range modelTools {
		if !strings.HasPrefix(tool, "mcp__denova__") && !slices.Contains([]string{"TaskCreate", "TaskGet", "TaskList", "TaskUpdate"}, tool) {
			t.Fatalf("unscoped model tool: %s", tool)
		}
	}
	for _, steering := range []bool{false, true} {
		mu.Lock()
		requests = 0
		mu.Unlock()
		stopCtx, stop := context.WithCancelCause(ctx)
		defer stop(context.Canceled)
		changed := make(chan struct{}, 1)
		if steering {
			stopCtx = external.WithSteering(stopCtx, &external.Steering{
				Changed: changed,
				Next: func(context.Context) (external.Guidance, bool, error) {
					return external.Guidance{Request: agentchat.ChatRequest{CommandID: "guidance", Message: "Keep the ending"}, Count: 1}, true, nil
				},
				Interrupt: func() { stop(external.ErrSteered) },
			})
		}
		cancelledHost := &testHost{call: func(callCtx context.Context, call external.ToolCall) (external.ToolResult, error) {
			if steering {
				changed <- struct{}{}
			} else {
				stop(context.Canceled)
			}
			<-callCtx.Done()
			return external.ToolResult{}, callCtx.Err()
		}}
		result, err := c.Run(stopCtx, input, cancelledHost)
		if !errors.Is(err, context.Canceled) || (steering && (!errors.Is(context.Cause(stopCtx), external.ErrSteered) || !result.Settled)) {
			t.Fatalf("native interruption: steering=%t settled=%t error=%v", steering, result.Settled, err)
		}
		c.mu.Lock()
		active := len(c.active)
		c.mu.Unlock()
		if active != 0 || len(cancelledHost.calls) != 1 {
			t.Fatal("interrupt left an active process or ran another tool")
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal(ctx.Err())
	}
}
