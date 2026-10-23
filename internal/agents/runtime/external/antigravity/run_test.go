package antigravity

import (
	"bufio"
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
	agentschema "github.com/alfredxw/denova/agent/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

type testHost struct {
	events []agentrun.Event
	calls  []external.ToolCall
	onEmit func(agentrun.Event)
	onCall func(context.Context, external.ToolCall) (external.ToolResult, error)
}

func (host *testHost) Emit(event agentrun.Event) error {
	host.events = append(host.events, event)
	if host.onEmit != nil {
		host.onEmit(event)
	}
	return nil
}
func (host *testHost) CallTool(ctx context.Context, call external.ToolCall) (external.ToolResult, error) {
	host.calls = append(host.calls, call)
	if host.onCall != nil {
		return host.onCall(ctx, call)
	}
	return external.ToolResult{Text: "Committed product outcome", Success: true}, nil
}

func fixtureClient(t *testing.T, mode string) *Client {
	t.Helper()
	t.Setenv("DENOVA_ANTIGRAVITY_FIXTURE", mode)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client, err := Connect(t.Context(), ProcessOptions{Executable: executable, Args: []string{"-test.run=TestAntigravityFixtureProcess", "--"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(func() {
		entries, err := os.ReadDir(filepath.Join(os.Getenv("HOME"), ".gemini", "config", "agents"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Error(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "denova-") {
				t.Errorf("transient agent was not removed: %s", entry.Name())
			}
		}
	})
	return client
}

func fixtureInput(t *testing.T) external.Input {
	t.Helper()
	return external.Input{Selection: config.RuntimeSelection{Kind: config.RuntimeAntigravity, Antigravity: &config.AntigravityRuntimeSettings{Model: "gemini-fixture-high"}},
		Directory: t.TempDir(), Instructions: "Use the accepted product tools.", Text: "Perform the scoped task.",
		Tools: []external.Tool{{Name: "record", Description: "Record a confirmed product change.", Schema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}}}`)}}}
}

func TestRunNativeNDJSONMCPAndResume(t *testing.T) {
	client := fixtureClient(t, "tool")
	state, err := client.Check(t.Context())
	if err != nil || state.Status != "ready" {
		t.Fatalf("check: %#v %v", state, err)
	}
	input, host := fixtureInput(t), &testHost{}
	first, err := client.Run(t.Context(), input, host)
	if err != nil || !first.Settled || first.Text != "Task completed." || first.SessionID != "conversation-1" {
		t.Fatalf("first result: %#v %v", first, err)
	}
	if len(host.calls) != 1 || host.calls[0].Name != "record" || string(host.calls[0].Arguments) != `{"value":"fixture"}` || host.calls[0].ID == "" {
		t.Fatalf("host calls: %#v", host.calls)
	}
	if len(host.events) != 2 || first.Usage.TotalTokens != 30 {
		t.Fatalf("stream/usage: %#v %#v", host.events, first.Usage)
	}
	input.SessionID, input.CumulativeUsage = first.SessionID, first.CumulativeUsage
	second, err := client.Run(t.Context(), input, host)
	if err != nil || !second.Settled || second.Usage.TotalTokens != 30 || second.CumulativeUsage.TotalTokens != 60 {
		t.Fatalf("resumed result: %#v %v", second, err)
	}
}

func TestRunCancellationPreservesPartialResponse(t *testing.T) {
	client := fixtureClient(t, "cancel")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	host := &testHost{onEmit: func(event agentrun.Event) {
		if event.Type == "chunk" {
			cancel()
		}
	}}
	result, err := client.Run(ctx, fixtureInput(t), host)
	if !errors.Is(err, context.Canceled) || result.Text != "Task " || result.Settled {
		t.Fatalf("cancelled result: %#v %v", result, err)
	}
}

// The CLI advertises provider tools the frontmatter never declared: approval
// prompts in every build, browser session tools in newer ones, and whatever the
// next release adds. Their names are not knowable ahead of time, so an unlisted
// one is recorded and the turn continues; product effects still only settle
// through Host.
func TestRunToleratesProviderToolsOutsideScope(t *testing.T) {
	for _, mode := range []string{"approval", "browser", "unsafe"} {
		t.Run(mode, func(t *testing.T) {
			host := &testHost{}
			result, err := fixtureClient(t, mode).Run(t.Context(), fixtureInput(t), host)
			if err != nil || !result.Settled || result.Text != "Task completed." {
				t.Fatalf("%s result: %#v %v", mode, result, err)
			}
			if len(host.calls) != 1 || host.calls[0].Name != "record" {
				t.Fatalf("%s host calls: %#v", mode, host.calls)
			}
		})
	}
}

func TestRunRejectsIncompleteAndFailedTurns(t *testing.T) {
	for _, mode := range []string{"incomplete", "error"} {
		t.Run(mode, func(t *testing.T) {
			client := fixtureClient(t, mode)
			result, err := client.Run(t.Context(), fixtureInput(t), &testHost{})
			if err == nil || result.Settled || result.Text != "Task " {
				t.Fatalf("failed result: %#v %v", result, err)
			}
		})
	}
}

func TestCheckAuthenticationAndCatalog(t *testing.T) {
	for _, mode := range []string{"auth", "auth-stderr"} {
		t.Run(mode, func(t *testing.T) {
			client := fixtureClient(t, mode)
			state, err := client.Check(t.Context())
			if err != nil || state.Status != "auth_required" || client.Status().Status != "auth_required" {
				t.Fatalf("auth check: %#v %v", state, err)
			}
		})
	}
	models, err := decodeModels("Fetching available models...\ngemini-3.8-flash-high   Gemini 3.8 Flash (High)\nclaude-opus-high  Claude Opus (High)\n")
	if err != nil || len(models.Items) != 2 || models.Items[1].ID != "claude-opus-high" || len(models.Items[0].Efforts) != 0 {
		t.Fatalf("catalog: %#v %v", models, err)
	}
	for _, invalid := range []string{"", "Error: no models available", "gemini-high Name\ngemini-high Duplicate"} {
		if _, err := decodeModels(invalid); err == nil {
			t.Fatalf("invalid catalog accepted: %q", invalid)
		}
	}
}

func TestRejectsUnsupportedInputAndOperations(t *testing.T) {
	client := fixtureClient(t, "tool")
	for _, mode := range []external.OperationMode{external.OperationEvaluate, external.OperationCompact} {
		input := fixtureInput(t)
		input.Mode = mode
		if _, err := client.Run(t.Context(), input, &testHost{}); err == nil {
			t.Fatalf("unsupported operation %q accepted", mode)
		}
	}
	// Historical images belong to the provider conversation that received them;
	// replaying the text without them must not fail the turn.
	for _, history := range []external.Input{
		{History: []external.Message{{Role: "user", Attachments: []agentschema.Attachment{{MediaType: "image/png"}}}}},
		{History: []external.Message{{Role: "assistant", ToolImages: []agentschema.Attachment{{MediaType: "image/png"}}}}},
	} {
		if _, err := encodeInput(history, nil); err != nil {
			t.Fatalf("historical image rejected: %v", err)
		}
	}
	input := external.Input{History: []external.Message{{Role: "user", Text: "Historical request"}}, Text: "Current request"}
	body, err := encodeInput(input, []string{"ref1.png"})
	var event struct {
		Event   string                   `json:"event"`
		Message struct{ Content string } `json:"message"`
	}
	if err != nil || json.Unmarshal(body, &event) != nil || event.Event != "user" || !strings.Contains(event.Message.Content, "prior conversation data") || !strings.Contains(event.Message.Content, "Current request") || !strings.Contains(event.Message.Content, "@ref1.png") {
		t.Fatalf("encoded input: %s %v", body, err)
	}
}

// The process fixture uses Antigravity's documented init/step_update/result
// envelopes (antigravity.google/docs/cli/headless), never Claude control frames.
// It validates generated agent scope and performs a real local MCP roundtrip.
func TestAntigravityFixtureProcess(t *testing.T) {
	mode := os.Getenv("DENOVA_ANTIGRAVITY_FIXTURE")
	if mode == "" {
		return
	}
	defer func() {
		if value := recover(); value != nil {
			fmt.Fprintln(os.Stderr, value)
			os.Exit(2)
		}
	}()
	args := os.Args[slices.Index(os.Args, "--")+1:]
	if slices.Contains(args, "--version") {
		fmt.Println(MinimumVersion)
		os.Exit(0)
	}
	if slices.Contains(args, "models") {
		if mode == "auth-stderr" {
			fmt.Fprintln(os.Stderr, "Fetching available models...\nError: Please sign in to view available models. Launch the CLI without arguments to sign in.")
			os.Exit(1)
		}
		fmt.Println("Fetching available models...")
		if mode == "auth" {
			fmt.Println("Error: Please sign in to view available models. Launch the CLI without arguments to sign in.")
		} else {
			fmt.Println("gemini-fixture-high  Fixture Model (High)")
		}
		os.Exit(0)
	}
	for key, expected := range map[string]string{"--input-format": "stream-json", "--output-format": "stream-json", "--print-timeout": "0", "--model": "gemini-fixture-high"} {
		index := slices.Index(args, key)
		if index < 0 || index+1 >= len(args) || args[index+1] != expected {
			panic("missing native flag " + key)
		}
	}
	if !slices.Contains(args, "--sandbox") {
		panic("attempt must keep native tools inside the sandbox")
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() || !json.Valid(scanner.Bytes()) || !strings.Contains(string(scanner.Bytes()), `"event":"user"`) {
		panic("expected one native NDJSON user event")
	}
	agentIndex := slices.Index(args, "--agent")
	if agentIndex < 0 || agentIndex+1 >= len(args) || !strings.HasPrefix(args[agentIndex+1], "denova-") {
		panic("missing transient agent identity")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".gemini", "config", "agents", args[agentIndex+1], "agent.md"))
	if err != nil {
		panic(err)
	}
	frontmatter := strings.SplitN(string(data), "---", 3)
	var agent struct {
		Tools      []string         `yaml:"tools"`
		Exclude    bool             `yaml:"excludeDefaultComponents"`
		Inherit    bool             `yaml:"inheritCustomizations"`
		Subagent   bool             `yaml:"subagent"`
		MCPServers []map[string]any `yaml:"mcpServers"`
	}
	// inheritCustomizations must stay off (ambient isolation), but
	// excludeDefaultComponents must stay off too: it removes the default file
	// reader that the "@name" image channel depends on.
	if len(frontmatter) != 3 || yaml.Unmarshal([]byte(frontmatter[1]), &agent) != nil || agent.Exclude || agent.Inherit || agent.Subagent {
		panic("agent did not isolate product capabilities")
	}
	if mode == "summary" {
		if len(agent.Tools) != 0 || len(agent.MCPServers) != 0 || slices.Contains(args, "--conversation") {
			panic("summary reused provider conversation or tools")
		}
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":[]}}`)
	} else if len(agent.Tools) != 0 || len(agent.MCPServers) != 1 {
		panic("unexpected product tool configuration")
	} else if mode == "unsafe" {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["call_mcp_tool","run_command"]}}`)
	} else if mode == "approval" {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["ask_custom_permission","call_mcp_tool"]}}`)
	} else if mode == "browser" {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["browser_click_element","browser_navigate","call_mcp_tool"]}}`)
	} else {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["call_mcp_tool"]}}`)
	}
	if mode == "tool" || mode == "unsafe" || mode == "concurrent" || mode == "approval" || mode == "browser" {
		server := agent.MCPServers[0]
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client := mcp.NewClient(&mcp.Implementation{Name: "agy-fixture", Version: "1"}, nil)
		headers := server["headers"].(map[string]any)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server["serverUrl"].(string), HTTPClient: &http.Client{Transport: bearerTransport{headers["Authorization"].(string)}}}, nil)
		if err != nil {
			panic(err)
		}
		tools, err := session.ListTools(ctx, nil)
		if err != nil || len(tools.Tools) != 1 || tools.Tools[0].Name != "record" {
			panic("unexpected host tool catalog")
		}
		answer, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "record", Arguments: map[string]any{"value": "fixture"}})
		if err != nil || answer.IsError || answer.Content[0].(*mcp.TextContent).Text != "Committed product outcome" {
			panic("host tool result did not reach the CLI")
		}
		_ = session.Close()
	}
	fmt.Println(`{"event":"step_update","step_update":{"conversation_id":"conversation-1","step_index":2,"state":"ACTIVE","step_type":"agent_response","text_delta":"Task "}}`)
	if mode == "cancel" {
		time.Sleep(time.Hour)
	}
	if mode == "incomplete" {
		os.Exit(0)
	}
	if mode == "error" {
		fmt.Println(`{"event":"result","result":{"conversation_id":"conversation-1","status":"ERROR","error":"fixture failure"}}`)
		os.Exit(1)
	}
	fmt.Println(`{"event":"step_update","step_update":{"conversation_id":"conversation-1","step_index":2,"state":"DONE","step_type":"agent_response","text_delta":"completed."}}`)
	total := 30
	if index := slices.Index(args, "--conversation"); index >= 0 {
		if args[index+1] != "conversation-1" {
			panic("incorrect resumed conversation")
		}
		total = 60
	}
	fmt.Printf("{\"event\":\"result\",\"result\":{\"conversation_id\":\"conversation-1\",\"status\":\"SUCCESS\",\"response\":\"Task completed.\",\"usage\":{\"input_tokens\":%d,\"output_tokens\":%d,\"total_tokens\":%d}}}\n", total*2/3, total/3, total)
	os.Exit(0)
}

type bearerTransport struct{ value string }

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", transport.value)
	return http.DefaultTransport.RoundTrip(request)
}
