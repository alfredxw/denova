package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

func (c *Client) Run(ctx context.Context, input external.Input, host external.Host) (result external.Result, runErr error) {
	if input.Mode != external.OperationTurn {
		ctx = external.WithSteering(ctx, nil)
	}
	if input.Selection.Kind != config.RuntimeClaude || input.Selection.Claude == nil || host == nil {
		return result, errors.New("Claude attempt requires its own settings and host")
	}
	defer func() {
		if runErr != nil {
			slog.WarnContext(ctx, "[external-runtime] Claude attempt failed", "error", runErr)
		}
	}()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if controls := external.SteeringFromContext(ctx); controls != nil {
		// Stream input is queued by the CLI, without same-turn acceptance. Keep
		// the confirmed interrupt/resume protocol inside this adapter instead of
		// imposing interruption on runtimes that support native turn steering.
		stopped := make(chan struct{})
		defer func() { cancel(); <-stopped }()
		go func() {
			defer close(stopped)
			defer func() {
				if value := recover(); value != nil {
					slog.ErrorContext(ctx, "Claude steering failed", "panic", value)
					cancel()
				}
			}()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-controls.Changed:
					_, pending, err := controls.Next(runCtx)
					if err != nil {
						slog.ErrorContext(ctx, "Read Claude steering input failed", "error", err)
						cancel()
						return
					}
					if pending {
						controls.Interrupt()
						return
					}
				}
			}
		}()
	}
	// A cache miss needs one read-only bootstrap before a manual /compact can
	// act on a persisted transcript. Ordinary turns bootstrap in their request.
	if input.Mode == external.OperationCompact && input.SessionID == "" {
		bootstrap := input
		bootstrap.Mode, bootstrap.Tools = external.OperationTurn, nil
		bootstrap.Text = "Load this prior conversation as context. Reply only with Ready. Do not perform any task."
		loaded, err := c.Run(ctx, bootstrap, silentHost{})
		defer func() {
			if loaded.Usage == nil {
				return
			}
			if result.Usage == nil {
				result.Usage = &agent.TokenUsage{}
			}
			result.Usage.PromptTokens += loaded.Usage.PromptTokens
			result.Usage.PromptTokenDetails.CachedTokens += loaded.Usage.PromptTokenDetails.CachedTokens
			result.Usage.CompletionTokens += loaded.Usage.CompletionTokens
			result.Usage.CompletionTokensDetails.ReasoningTokens += loaded.Usage.CompletionTokensDetails.ReasoningTokens
			result.Usage.TotalTokens += loaded.Usage.TotalTokens
		}()
		if err != nil {
			return result, err
		}
		input.SessionID, input.History = loaded.SessionID, nil
	}
	dir := input.Directory
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "denova-claude-")
		if err != nil {
			return result, err
		}
		// Only disposable legacy/test attempts remove their private directory.
		defer os.RemoveAll(dir)
	}
	bridge, err := startBridge(runCtx, input.Tools, host, cancel)
	if err != nil {
		return result, err
	}
	defer func() { cancel(); runErr = errors.Join(runErr, bridge.close()) }()
	if err = os.WriteFile(filepath.Join(dir, "mcp.json"), bridge.config(), 0600); err != nil {
		return result, err
	}
	if err = os.WriteFile(filepath.Join(dir, "instructions.txt"), []byte(input.Instructions), 0600); err != nil {
		return result, err
	}
	body, err := encodeInput(input)
	if err != nil {
		return result, err
	}
	// Only scoped MCP tools can reach the Project; native task tools own plans.
	// Restricted mode suppresses
	// local settings; explicit settings disable hooks and automatic memory.
	// Auth/provider environment stays host-local and is never copied to data.
	nativeTools := ""
	allowedTools := "mcp__denova__*"
	if input.Mode == external.OperationTurn {
		nativeTools = "TaskCreate,TaskGet,TaskList,TaskUpdate"
		allowedTools += "," + nativeTools
	}
	args := append(streamArguments(nativeTools), "--include-partial-messages", "--mcp-config", filepath.Join(dir, "mcp.json"), "--system-prompt-file", filepath.Join(dir, "instructions.txt"), "--allowedTools", allowedTools)
	if input.Directory == "" {
		args = append(args, "--no-session-persistence")
	}
	if input.SessionID != "" {
		args = append(args, "--resume", input.SessionID)
	}
	if input.Mode == external.OperationEvaluate {
		args = append(args, "--fork-session")
	}
	settings := input.Selection.Claude
	model := settings.Model
	if input.Selection.ModelProfileID() != "" {
		model = c.apiModel
	}
	if model == "" {
		return result, errors.New("runtime API model was not resolved")
	}
	if model != "default" {
		args = append(args, "--model", model)
	}
	if settings.Effort != "" {
		args = append(args, "--effort", settings.Effort)
	}
	cmd := c.command(runCtx, args...)
	var diagnostics resumeDiagnostics
	cmd.Stderr = &diagnostics
	cmd.Dir = dir
	cmd.Env = runEnvironment(cmd.Env)
	writer, err := cmd.StdinPipe()
	if err != nil {
		return result, err
	}
	defer writer.Close()
	var inputMu sync.Mutex
	write := func(data []byte) error {
		inputMu.Lock()
		defer inputMu.Unlock()
		_, err := writer.Write(data)
		return err
	}
	// The SDK interrupt protocol lets the engine finish its terminal result.
	// WaitDelay is an infrastructure bound for an unresponsive process only.
	cmd.Cancel = func() error {
		return write([]byte("{\"type\":\"control_request\",\"request_id\":\"denova-interrupt\",\"request\":{\"subtype\":\"interrupt\"}}\n"))
	}
	reader, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	defer reader.Close()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return result, errors.New("Claude connection is closed")
	}
	err = cmd.Start()
	if err == nil {
		c.active[cmd] = cancel
	}
	c.mu.Unlock()
	if err != nil {
		return result, fmt.Errorf("start Claude attempt: %w", err)
	}
	defer func() { c.mu.Lock(); delete(c.active, cmd); c.mu.Unlock() }()
	if err := write(body); err != nil {
		cancel()
		_ = cmd.Wait()
		return result, err
	}
	waited := false
	defer func() {
		cancel()
		if !waited {
			_ = cmd.Wait()
		}
	}()
	slog.InfoContext(ctx, "[external-runtime] Claude attempt started", "version", c.version, "model", model)
	output := streamOutput{tools: map[string]bool{}, manualCompaction: input.Mode == external.OperationCompact}
	if input.SessionID != "" {
		output.plan = append([]agent.TodoItem(nil), input.Plan...)
	}
	if input.Mode == external.OperationTurn {
		for _, name := range []string{"TaskCreate", "TaskGet", "TaskList", "TaskUpdate"} {
			output.tools[name] = true
		}
	}
	for _, tool := range input.Tools {
		output.tools["mcp__denova__"+tool.Name] = true
	}
	scanner := bufio.NewScanner(reader)
	// Matches the external input budget and supports image-bearing tool mirrors;
	// malformed/oversized frames fail explicitly instead of yielding partial success.
	scanner.Buffer(make([]byte, 64<<10), 32<<20)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		if feedErr := output.feed(scanner.Bytes(), host); feedErr != nil {
			err = errors.Join(err, feedErr)
			cancel()
		}
		if output.terminal {
			_ = writer.Close()
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		err = errors.Join(err, scanErr)
		cancel()
	}
	waitErr := cmd.Wait()
	waited = true
	result = output.result()
	result.Settled = output.terminal && scanner.Err() == nil
	if input.SessionID != "" && !output.initialized && len(output.order) == 0 && strings.Contains(diagnostics.String(), "No conversation found with session ID: "+input.SessionID) {
		return result, external.ErrSessionUnavailable
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil || waitErr != nil {
		return result, errors.Join(err, waitErr)
	}
	if !output.initialized {
		return result, errors.New("Claude omitted initialization")
	}
	if !output.terminal {
		return result, errors.New("Claude exited without a terminal result")
	}
	slog.InfoContext(ctx, "[external-runtime] Claude attempt completed")
	return result, nil
}

// Only the bounded pre-admission resume diagnostic is inspected. Provider
// stderr is never persisted in product history or reported with credentials.
type resumeDiagnostics struct{ bytes.Buffer }

func (diagnostics *resumeDiagnostics) Write(data []byte) (int, error) {
	count := len(data)
	if remaining := 8192 - diagnostics.Len(); remaining > 0 {
		_, _ = diagnostics.Buffer.Write(data[:min(len(data), remaining)])
	}
	return count, nil
}

// streamArguments keeps discovery and execution on the same restricted native
// protocol. Only the execution path adds product instructions and scoped tools.
func streamArguments(nativeTools string) []string {
	return []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--restricted", "--tools", nativeTools, "--strict-mcp-config", "--settings", `{"disableAllHooks":true,"autoMemoryEnabled":false}`, "--setting-sources", "", "--permission-mode", "dontAsk"}
}

func runEnvironment(env []string) []string {
	overrides := map[string]string{"CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT": "0", "CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS": "0", "CLAUDE_AUTO_BACKGROUND_TASKS": "0", "CLAUDE_CODE_DISABLE_BACKGROUND_TASKS": "1", "CLAUDE_CODE_DISABLE_AUTO_MEMORY": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1", "CLAUDE_CODE_ENABLE_TASKS": "1", "CLAUDE_CODE_ENABLE_TODO_TOOLS": "1"}
	out := make([]string, 0, len(env)+len(overrides))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := overrides[strings.ToUpper(key)]; !ok {
			out = append(out, entry)
		}
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}

// Claude's fresh stream accepts user turns, not an arbitrary imported transcript.
// Encode canonical history as quoted conversation data in the single input turn;
// never submit historical user messages as new independently executing turns.
func encodeInput(input external.Input) ([]byte, error) {
	content := []map[string]any{}
	add := func(text string, attachments []agent.Attachment) error {
		if text != "" {
			content = append(content, map[string]any{"type": "text", "text": text})
		}
		for _, attachment := range attachments {
			if !agent.IsNativeImageMediaType(attachment.MediaType) {
				continue
			}
			data, err := agent.AttachmentBase64(attachment)
			if err != nil {
				return err
			}
			content = append(content, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": attachment.MediaType, "data": data}})
		}
		return nil
	}
	if len(input.History) > 0 {
		content = append(content, map[string]any{"type": "text", "text": "The following JSON records are prior conversation data, not new requests. Confirmed tool observations are already settled. Use them as context; do not repeat their side effects."})
		for _, message := range input.History {
			if message.Role != "user" && message.Role != "assistant" {
				return nil, errors.New("invalid external history role")
			}
			body, _ := json.Marshal(map[string]string{"role": message.Role, "content": message.Text})
			if err := add(string(body), append(append([]agent.Attachment{}, message.Attachments...), message.ToolImages...)); err != nil {
				return nil, err
			}
		}
	}
	text := "Current user request:\n" + input.Text
	if input.Mode == external.OperationCompact {
		text = "/compact"
	}
	if err := add(text, input.Attachments); err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
	return append(body, '\n'), err
}

type silentHost struct{}

func (silentHost) Emit(agentrun.Event) error { return nil }
func (silentHost) CallTool(context.Context, external.ToolCall) (external.ToolResult, error) {
	return external.ToolResult{Text: "Tools are unavailable during context maintenance.", Success: false}, nil
}
