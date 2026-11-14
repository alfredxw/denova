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
	"runtime"
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
	restoreSandbox := verifyOSSandbox
	verifyOSSandbox = func() error { return nil }
	t.Cleanup(func() { verifyOSSandbox = restoreSandbox })
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

// A terminal result is authoritative for the product task. CLI 1.4.7 was
// observed emitting it and then staying alive with stdout still open, which left
// the turn "thinking" forever: no output, no error, and only a manual cancel.
func TestRunSettlesTerminalResultWhenCLIStaysAlive(t *testing.T) {
	restore := exitGrace
	exitGrace = 200 * time.Millisecond
	t.Cleanup(func() { exitGrace = restore })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	started := time.Now()
	result, err := fixtureClient(t, "hang").Run(ctx, fixtureInput(t), &testHost{})
	if err != nil || !result.Settled || result.Text != "Task completed." || result.SessionID != "conversation-1" {
		t.Fatalf("hung attempt result: %#v %v", result, err)
	}
	if result.Usage == nil || result.Usage.TotalTokens != 30 {
		t.Fatalf("hung attempt usage: %#v", result.Usage)
	}
	if elapsed := time.Since(started); elapsed >= exitGrace*10 {
		t.Fatalf("hung attempt waited %s, want a bounded reap", elapsed)
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

// A disposable attempt must not leave its conversation store behind: the CLI
// prunes neither summaries nor unbound attempts, so every maintenance call would
// otherwise add another store.
func TestRunRemovesDisposableConversation(t *testing.T) {
	client := fixtureClient(t, "tool")
	store := filepath.Join(os.Getenv("HOME"), ".gemini", "antigravity-cli", "conversations")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(store, "conversation-1.db")
	if err := os.WriteFile(leftover, []byte("store"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := fixtureInput(t)
	input.Directory = ""
	if _, err := client.Run(t.Context(), input, &testHost{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disposable conversation survived: %v", err)
	}
}

// A resume that opens another conversation was not continued. Nothing has run at
// the initialization frame, so the attempt is rebuilt from canonical history
// instead of silently appending to a fresh conversation.
func TestRunRejectsResumeUnderAnotherConversation(t *testing.T) {
	input := fixtureInput(t)
	input.SessionID = "conversation-1"
	if _, err := fixtureClient(t, "resume-mismatch").Run(t.Context(), input, &testHost{}); !errors.Is(err, external.ErrSessionUnavailable) {
		t.Fatalf("resume mismatch was accepted: %v", err)
	}
}

// A provider verdict is the product-visible failure. The teardown this adapter
// triggers right after reading it is an expected side effect; appending its own
// exit status is what turned a policy rejection into "signal: killed".
func TestRunProviderVerdictExcludesProcessTeardown(t *testing.T) {
	result, err := fixtureClient(t, "error").Run(t.Context(), fixtureInput(t), &testHost{})
	if err == nil || !strings.Contains(err.Error(), "fixture failure") {
		t.Fatalf("provider verdict missing: %#v %v", result, err)
	}
	for _, noise := range []string{"exit status", "signal: killed"} {
		if strings.Contains(err.Error(), noise) {
			t.Fatalf("process teardown leaked into the product error: %v", err)
		}
	}
	if result.Text != "Task " {
		t.Fatalf("streamed text was dropped: %#v", result)
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
	// A historical image that cannot be read must surface a media failure
	// instead of being silently dropped from the rebuilt context.
	unreadable := external.Input{History: []external.Message{{Role: "user", Attachments: []agentschema.Attachment{{Name: "missing.png", MediaType: "image/png"}}}}}
	if _, err := stageAttachments(unreadable, t.TempDir()); !external.IsMediaUnsupported(err) {
		t.Fatalf("unreadable historical image was not marked: %v", err)
	}
	input := external.Input{History: []external.Message{{Role: "user", Text: "Historical request"}}, Text: "Current request"}
	body, err := encodeInput(input, stagedMedia{Request: []string{"ref1.png"}})
	var event struct {
		Event   string                   `json:"event"`
		Message struct{ Content string } `json:"message"`
	}
	if err != nil || json.Unmarshal(body, &event) != nil || event.Event != "user" || !strings.Contains(event.Message.Content, "prior conversation data") || !strings.Contains(event.Message.Content, "Current request") || !strings.Contains(event.Message.Content, "@ref1.png") {
		t.Fatalf("encoded input: %s %v", body, err)
	}
}

// A rebuilt conversation has no provider-side attachments, so historical user
// and tool images must be replayed through "@name" references beside their own
// records instead of being lost with the old provider session.
func TestEncodeRebuiltHistoryCarriesImageReferences(t *testing.T) {
	image := referenceAttachment(t)
	dir := t.TempDir()
	media, err := stageAttachments(external.Input{
		History: []external.Message{
			{Role: "user", Text: "Use this reference", Attachments: []agentschema.Attachment{image}, ToolImages: []agentschema.Attachment{image}},
			{Role: "assistant", Text: "Done."},
		},
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(media.History) != 2 || !slices.Equal(media.History[0], []string{"hist1.png", "hist2.png"}) || len(media.History[1]) != 0 {
		t.Fatalf("history references: %#v", media.History)
	}
	if !strings.Contains(media.History[0][0], "hist") {
		t.Fatal("historical files must use short hist names for @ resolution")
	}
	input := external.Input{History: []external.Message{
		{Role: "user", Text: "Use this reference"},
		{Role: "assistant", Text: "Done."},
	}, Text: "Continue"}
	body, err := encodeInput(input, media)
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Event   string                   `json:"event"`
		Message struct{ Content string } `json:"message"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatal(err)
	}
	content := event.Message.Content
	if !strings.Contains(content, "@hist1.png") || !strings.Contains(content, "@hist2.png") {
		t.Fatalf("historical references missing: %s", content)
	}
	if strings.Count(content, "Media for the record above") != 1 {
		t.Fatalf("media lines must sit beside the record that owns them: %s", content)
	}
}

// The CLI payload for a rebuilt conversation must carry the staged historical
// images: the fixture process verifies both the NDJSON references and the file.
func TestRunRebuildCarriesHistoryImagesToCLI(t *testing.T) {
	image := referenceAttachment(t)
	input := fixtureInput(t)
	input.History = []external.Message{{Role: "user", Text: "Earlier reference", Attachments: []agentschema.Attachment{image}}}
	result, err := fixtureClient(t, "media").Run(t.Context(), input, &testHost{})
	if err != nil || !result.Settled {
		t.Fatalf("result: %#v %v", result, err)
	}
}

// A summarize attempt is a fresh disposable conversation, but the checkpoint
// relies on seeing the historical images to preserve visual details: the
// fixture verifies the summary payload still carries their references.
func TestRunSummarizeCarriesHistoryImagesToCLI(t *testing.T) {
	image := referenceAttachment(t)
	input := fixtureInput(t)
	input.Mode = external.OperationSummarize
	input.History = []external.Message{{Role: "user", Text: "Earlier reference", Attachments: []agentschema.Attachment{image}}}
	result, err := fixtureClient(t, "summary-media").Run(t.Context(), input, &testHost{})
	if err != nil || !result.Settled {
		t.Fatalf("summary: %#v %v", result, err)
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
		if mode == "old-windows" {
			fmt.Println("1.4.2")
		} else {
			fmt.Println("1.5.8")
		}
		os.Exit(0)
	}
	if slices.Contains(args, "--help") {
		if mode == "missing-settings" {
			fmt.Println("Usage of agy:\n  --sandbox  Run in a sandbox")
		} else {
			fmt.Println("Usage of agy:\n  --sandbox   Run in a sandbox\n  --settings  Settings for this session only")
		}
		os.Exit(0)
	}
	if slices.Contains(args, "--dangerously-skip-permissions") {
		panic("adapter must never pass --dangerously-skip-permissions")
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
	settingsIndex := slices.Index(args, "--settings")
	if settingsIndex < 0 || settingsIndex+1 >= len(args) {
		panic("attempt must pass --settings isolation policy")
	}
	var parsedSettings sessionSettings
	if err := json.Unmarshal([]byte(args[settingsIndex+1]), &parsedSettings); err != nil {
		panic(err)
	}
	if parsedSettings.AgentMode != "" || parsedSettings.ToolPermission != "request-review" || !parsedSettings.EnableTerminalSandbox || parsedSettings.AllowNonWorkspaceAccess {
		panic(fmt.Sprintf("unexpected isolation settings: %#v", parsedSettings))
	}
	for _, denied := range []string{"write_file(*)", "command(*)", "execute_url(*)", "read_url(*)"} {
		if !slices.Contains(parsedSettings.Permissions.Deny, denied) {
			panic("missing required deny rule " + denied)
		}
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() || !json.Valid(scanner.Bytes()) || !strings.Contains(string(scanner.Bytes()), `"event":"user"`) {
		panic("expected one native NDJSON user event")
	}
	if mode == "media" {
		stdin := string(scanner.Bytes())
		if !strings.Contains(stdin, "@hist1.png") || !strings.Contains(stdin, "Media for the record above") {
			panic("historical image references missing from the CLI payload")
		}
		cwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		if _, err := os.Stat(filepath.Join(cwd, "hist1.png")); err != nil {
			panic("staged historical image missing: " + err.Error())
		}
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
		Tools                  []string         `yaml:"tools"`
		Exclude                bool             `yaml:"excludeDefaultComponents"`
		Inherit                bool             `yaml:"inheritCustomizations"`
		Subagent               bool             `yaml:"subagent"`
		CommandExecutionPolicy string           `yaml:"commandExecutionPolicy"`
		MCPServers             []map[string]any `yaml:"mcpServers"`
	}
	// inheritCustomizations must stay off (ambient isolation), but
	// excludeDefaultComponents must stay off too: it removes the default file
	// reader that the "@name" image channel depends on.
	if len(frontmatter) != 3 || yaml.Unmarshal([]byte(frontmatter[1]), &agent) != nil || agent.Exclude || agent.Inherit || agent.Subagent || agent.CommandExecutionPolicy != "off" {
		panic("agent did not isolate product capabilities")
	}
	if mode == "summary" {
		if len(agent.Tools) != 0 || len(agent.MCPServers) != 0 || slices.Contains(args, "--conversation") || len(parsedSettings.Permissions.Allow) != 0 || !slices.Contains(parsedSettings.Permissions.Deny, "mcp(*)") {
			panic("summary reused provider conversation or tools")
		}
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":[],"permission_mode":"request-review"}}`)
	} else if mode == "summary-media" {
		if len(agent.Tools) != 0 || len(agent.MCPServers) != 0 || slices.Contains(args, "--conversation") || len(parsedSettings.Permissions.Allow) != 0 || !slices.Contains(parsedSettings.Permissions.Deny, "mcp(*)") {
			panic("summary reused provider conversation or tools")
		}
		// The checkpoint instruction inspects supplied images, so a summarize
		// attempt must receive the historical references even though it is a
		// fresh disposable conversation.
		if !strings.Contains(string(scanner.Bytes()), "@hist1.png") {
			panic("summary lost its historical image references")
		}
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":[],"permission_mode":"request-review"}}`)
	} else if len(agent.Tools) != 0 || len(agent.MCPServers) != 1 || !slices.Equal(parsedSettings.Permissions.Allow, []string{"mcp(denova/*)"}) {
		panic("unexpected product tool configuration")
	} else if mode == "resume-mismatch" {
		if !slices.Contains(args, "--conversation") {
			panic("resume mismatch fixture needs a resumed attempt")
		}
		fmt.Println(`{"event":"init","conversation_id":"conversation-other","init":{"tools":["call_mcp_tool"],"permission_mode":"request-review"}}`)
	} else if mode == "unisolated" {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["call_mcp_tool"],"permission_mode":"always-proceed"}}`)
	} else if mode == "unsafe" {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["call_mcp_tool","run_command"],"permission_mode":"request-review"}}`)
	} else if mode == "approval" {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["ask_custom_permission","call_mcp_tool"],"permission_mode":"request-review"}}`)
	} else if mode == "browser" {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["browser_click_element","browser_navigate","call_mcp_tool"],"permission_mode":"request-review"}}`)
	} else {
		fmt.Println(`{"event":"init","conversation_id":"conversation-1","init":{"tools":["call_mcp_tool"],"permission_mode":"request-review"}}`)
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
	if mode == "hang" {
		// Settled turn whose process never exits, like CLI 1.4.7 keeping its
		// quota refreshers alive and stdout open after the terminal result.
		time.Sleep(10 * time.Minute)
	}
	os.Exit(0)
}

type bearerTransport struct{ value string }

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", transport.value)
	return http.DefaultTransport.RoundTrip(request)
}

func TestValidateWindowsSandboxBuild(t *testing.T) {
	for _, tc := range []struct {
		name      string
		build     uint64
		ubr       uint64
		wantError bool
	}{
		{"win10", 19045, 5000, true},
		{"win11-23h2", 22631, 4500, true},
		{"win11-24h2-pre-aug2026", 26100, 8655, true},
		{"win11-24h2-aug2026", 26100, 9168, false},
		{"win11-25h2-aug2026", 26200, 9168, false},
		{"win11-26h2", 26300, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWindowsSandboxBuild(tc.build, tc.ubr)
			if (err != nil) != tc.wantError {
				t.Fatalf("build %d.%d: err=%v wantError=%v", tc.build, tc.ubr, err, tc.wantError)
			}
		})
	}
}

func TestIsolationFailuresRefuseStartupAndProbe(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("missing-settings-flag", func(t *testing.T) {
		t.Setenv("DENOVA_ANTIGRAVITY_FIXTURE", "missing-settings")
		if _, err := Connect(t.Context(), ProcessOptions{Executable: executable, Args: []string{"-test.run=TestAntigravityFixtureProcess", "--"}}); !errors.Is(err, ErrVersionUnsupported) {
			t.Fatalf("CLI lacking --settings was accepted: %v", err)
		}
	})
	if runtime.GOOS == "windows" {
		t.Run("old-windows-cli", func(t *testing.T) {
			t.Setenv("DENOVA_ANTIGRAVITY_FIXTURE", "old-windows")
			if _, err := Connect(t.Context(), ProcessOptions{Executable: executable, Args: []string{"-test.run=TestAntigravityFixtureProcess", "--"}}); !errors.Is(err, ErrVersionUnsupported) {
				t.Fatalf("1.4.2 CLI on Windows was accepted: %v", err)
			}
		})
	}
	t.Run("os-sandbox-unavailable", func(t *testing.T) {
		client := fixtureClient(t, "tool")
		verifyOSSandbox = func() error { return errors.New("sandbox disabled on host") }
		state, err := client.Check(t.Context())
		if !errors.Is(err, ErrUnsafeCustomization) || state.Status != "unavailable" || state.ReasonKey != "agentRuntime.antigravityIsolationUnsupported" {
			t.Fatalf("sandbox unavailable check: %#v %v", state, err)
		}
		if _, err := client.Run(t.Context(), fixtureInput(t), &testHost{}); !errors.Is(err, ErrUnsafeCustomization) {
			t.Fatalf("Run started without verified OS sandbox: %v", err)
		}
	})
	t.Run("unisolated-init-permission-mode", func(t *testing.T) {
		client := fixtureClient(t, "unisolated")
		if _, err := client.Run(t.Context(), fixtureInput(t), &testHost{}); err == nil || !strings.Contains(err.Error(), "not isolated") {
			t.Fatalf("non-isolated permission_mode was accepted: %v", err)
		}
	})
	t.Run("workspace-active-hook", func(t *testing.T) {
		client := fixtureClient(t, "tool")
		input := fixtureInput(t)
		hookFile := filepath.Join(input.Directory, ".agents", "hooks.json")
		if err := os.MkdirAll(filepath.Dir(hookFile), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(hookFile, []byte(`{"ws-hook":{"SessionStart":[{"command":"unsafe"}]}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Run(t.Context(), input, &testHost{}); !errors.Is(err, ErrUnsafeCustomization) {
			t.Fatalf("Run started with active workspace hook: %v", err)
		}
	})
}
