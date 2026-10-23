package grok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pelletier/go-toml/v2"
)

type recordingHost struct {
	calls  []external.ToolCall
	events []agentrun.Event
}

func (h *recordingHost) Emit(event agentrun.Event) error {
	h.events = append(h.events, event)
	return nil
}

func (h *recordingHost) CallTool(_ context.Context, call external.ToolCall) (external.ToolResult, error) {
	if call.Name != "save" {
		return external.ToolResult{}, errors.New("unexpected host tool")
	}
	h.calls = append(h.calls, call)
	return external.ToolResult{Text: "Committed product outcome", Success: true}, nil
}

func fixtureClient(t *testing.T, mode string) *Client {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client, err := Connect(t.Context(), ProcessOptions{Executable: executable, Args: []string{"-test.run=TestGrokHeadlessFixture", "--", "--fixture-mode=" + mode}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func fixtureInput(t *testing.T) external.Input {
	t.Helper()
	return external.Input{
		Selection:    config.RuntimeSelection{Kind: config.RuntimeGrok, Grok: &config.GrokRuntimeSettings{Model: "grok-fixture", ContextWindow: 500000}},
		Instructions: "Reply briefly.",
		Text:         "Save the story.",
		Tools:        []external.Tool{{Name: "save", Description: "Save the story", Schema: json.RawMessage(`{"type":"object"}`)}},
		Directory:    t.TempDir(),
	}
}

func TestHeadlessTurnSettlesWithPlanAndUsage(t *testing.T) {
	host := &recordingHost{}
	result, err := fixtureClient(t, "ok").Run(t.Context(), fixtureInput(t), host)
	if err != nil || !result.Settled {
		t.Fatalf("turn result: %+v %v", result, err)
	}
	if result.Text != "Story saved." || result.SessionID != "session-1" {
		t.Fatalf("turn payload: %+v", result)
	}
	if result.Plan == nil || len(result.Plan.Items) != 1 || result.Plan.Items[0].Status != "completed" {
		t.Fatalf("plan: %#v", result.Plan)
	}
	if result.Usage == nil || result.Usage.PromptTokens != 120 || result.Usage.CompletionTokens != 25 {
		t.Fatalf("usage: %#v", result.Usage)
	}
	if len(host.events) < 2 {
		t.Fatalf("streamed chunks were not emitted: %#v", host.events)
	}
}

// Product tools must reach Host over the private MCP server; a native tool call
// mirror must never settle them.
func TestHeadlessRoutesHostToolThroughMCP(t *testing.T) {
	host := &recordingHost{}
	result, err := fixtureClient(t, "tool").Run(t.Context(), fixtureInput(t), host)
	if err != nil || !result.Settled {
		t.Fatalf("tool result: %+v %v", result, err)
	}
	if len(host.calls) != 1 || host.calls[0].Name != "save" {
		t.Fatalf("host calls: %#v", host.calls)
	}
	if !strings.Contains(string(host.calls[0].Arguments), "Story") {
		t.Fatalf("tool arguments: %s", host.calls[0].Arguments)
	}
}

// A resumed provider session already owns the prior turns: replaying the
// canonical history would duplicate it on every turn.
func TestHeadlessResumeSkipsHistoryReplay(t *testing.T) {
	input := fixtureInput(t)
	input.SessionID = "session-1"
	input.History = []external.Message{{Role: "user", Text: "Old context"}, {Role: "assistant", Text: "Old answer"}}
	result, err := fixtureClient(t, "resume").Run(t.Context(), input, &recordingHost{})
	if err != nil || !result.Settled || result.SessionID != "session-1" {
		t.Fatalf("resume: %+v %v", result, err)
	}
}

func TestHeadlessWithoutTerminalEventFails(t *testing.T) {
	result, err := fixtureClient(t, "empty").Run(t.Context(), fixtureInput(t), &recordingHost{})
	if err == nil || result.Settled {
		t.Fatalf("missing terminal event accepted: %+v %v", result, err)
	}
}

// A new event type is an observation, not a failure: the terminal `end` still
// decides settlement, so a CLI upgrade cannot break every turn.
func TestHeadlessIgnoresUnknownEvents(t *testing.T) {
	result, err := fixtureClient(t, "unknown").Run(t.Context(), fixtureInput(t), &recordingHost{})
	if err != nil || !result.Settled || result.Text != "Story saved." {
		t.Fatalf("unknown event broke the turn: %+v %v", result, err)
	}
}

func TestAttemptConfigCarriesContextWindowAndServer(t *testing.T) {
	body, err := attemptConfig(nil, "grok-4.7", 500000, map[string]any{"url": "http://127.0.0.1:1/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Model map[string]struct {
			ContextWindow int `toml:"context_window"`
		} `toml:"model"`
		Servers map[string]struct {
			URL string `toml:"url"`
		} `toml:"mcp_servers"`
		Permission struct {
			Deny []string `toml:"deny"`
		} `toml:"permission"`
	}
	if err := toml.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Model["grok-4.7"].ContextWindow != 500000 {
		t.Fatalf("context window: %#v", parsed.Model)
	}
	if parsed.Servers["denova"].URL != "http://127.0.0.1:1/mcp" {
		t.Fatalf("mcp server: %#v", parsed.Servers)
	}
	if !slices.Contains(parsed.Permission.Deny, "Read(**)") {
		t.Fatalf("isolation defaults: %#v", parsed.Permission)
	}
}

// Zero keeps the model default instead of pinning a stale size.
func TestAttemptConfigOmitsZeroContextWindow(t *testing.T) {
	body, err := attemptConfig(nil, "grok-4.7", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "context_window") {
		t.Fatalf("zero context window was written:\n%s", body)
	}
}

func TestDecodeModelsReadsCatalog(t *testing.T) {
	catalog, err := decodeModels("You are not authenticated.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n  - grok-4.7-build-fast\n  - grok-4.6\n")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.DefaultID != "grok-4.7" || len(catalog.Items) != 3 {
		t.Fatalf("catalog: %#v", catalog)
	}
	if catalog.Items[1].ID != "grok-4.7-build-fast" {
		t.Fatalf("catalog order: %#v", catalog.Items)
	}
}

// TestGrokHeadlessFixture stands in for the installed CLI. It only runs when the
// parent test passes --fixture-mode, so a normal test run skips it.
func TestGrokHeadlessFixture(t *testing.T) {
	mode := ""
	version := false
	for _, arg := range os.Args {
		if arg == "--version" {
			version = true
		}
		if strings.HasPrefix(arg, "--fixture-mode=") {
			mode = strings.TrimPrefix(arg, "--fixture-mode=")
		}
	}
	if version {
		fmt.Println("grok 1.0.62")
		os.Exit(0)
	}
	if mode == "models" {
		fmt.Println("Default model: grok-4.7")
		fmt.Println("Available models:")
		fmt.Println("  * grok-4.7 (default)")
		fmt.Println("  - grok-4.7-build-fast")
		os.Exit(0)
	}
	if mode == "signed-out" {
		fmt.Fprintln(os.Stderr, "You are not authenticated.")
		os.Exit(1)
	}
	if mode == "" {
		return
	}
	runFixture(mode)
	os.Exit(0)
}

func runFixture(mode string) {
	promptFile := ""
	resumed := false
	for index, arg := range os.Args {
		if arg == "--prompt-file" && index+1 < len(os.Args) {
			promptFile = os.Args[index+1]
		}
		if arg == "-r" && index+1 < len(os.Args) {
			resumed = os.Args[index+1] != ""
		}
	}
	prompt, err := os.ReadFile(promptFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture could not read prompt:", err)
		os.Exit(2)
	}
	if mode == "resume" {
		if !resumed {
			os.Exit(5)
		}
		if strings.Contains(string(prompt), "prior conversation") {
			os.Exit(6)
		}
	}
	emit := func(value any) {
		body, _ := json.Marshal(value)
		fmt.Println(string(body))
	}
	emit(map[string]any{"type": "available_commands", "tools": []string{"todo_write", "search_tool", "use_tool"}})
	switch mode {
	case "unknown":
		emit(map[string]any{"type": "mystery"})
		emit(map[string]any{"type": "thought", "data": "private"})
	case "empty":
		return
	}
	if mode == "tool" {
		callHostTool()
	}
	emit(map[string]any{"type": "text", "data": "Story "})
	emit(map[string]any{"type": "text", "data": "saved."})
	emit(map[string]any{"type": "plan", "entries": []map[string]string{{"content": "Save story", "status": "completed", "priority": "medium"}}})
	emit(map[string]any{"type": "usage", "usage": map[string]int{"input_tokens": 100, "output_tokens": 20, "cache_read_input_tokens": 40, "reasoning_tokens": 5}})
	emit(map[string]any{"type": "end", "stopReason": "end_turn", "sessionId": "session-1", "usage": map[string]int{"input_tokens": 120, "output_tokens": 25, "cache_read_input_tokens": 40, "reasoning_tokens": 5, "total_tokens": 145}})
}

type bearerTransport struct{ token string }

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request.Header.Set("Authorization", transport.token)
	return http.DefaultTransport.RoundTrip(request)
}

func callHostTool() {
	body, err := os.ReadFile(filepath.Join(os.Getenv("GROK_HOME"), "config.toml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture could not read config:", err)
		os.Exit(3)
	}
	var parsed struct {
		Servers map[string]struct {
			URL     string            `toml:"url"`
			Headers map[string]string `toml:"headers"`
		} `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(body, &parsed); err != nil {
		fmt.Fprintln(os.Stderr, "fixture could not parse config:", err)
		os.Exit(4)
	}
	server := parsed.Servers["denova"]
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "grok-fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearerTransport{server.Headers["Authorization"]}}}, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture could not reach host tools:", err)
		os.Exit(7)
	}
	defer session.Close()
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "save", Arguments: map[string]any{"text": "Story"}}); err != nil {
		fmt.Fprintln(os.Stderr, "fixture host tool failed:", err)
		os.Exit(8)
	}
}
