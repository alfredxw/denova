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

	"denova/config"
	"denova/internal/agents/external"
	agent "github.com/alfredxw/denova/agent"
)

func (c *Client) Run(ctx context.Context, input external.Input, host external.Host) (result external.Result, runErr error) {
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
	dir, err := os.MkdirTemp("", "denova-claude-")
	if err != nil {
		return result, err
	}
	// dir is created here and never references a user Project or credential home.
	defer os.RemoveAll(dir)
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
	// Only scoped MCP tools can reach the Project. Restricted mode suppresses
	// local settings; explicit settings disable hooks and automatic memory.
	// Auth/provider environment stays host-local and is never copied to data.
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--include-partial-messages", "--no-session-persistence", "--restricted", "--tools", "", "--strict-mcp-config", "--mcp-config", filepath.Join(dir, "mcp.json"), "--system-prompt-file", filepath.Join(dir, "instructions.txt"), "--settings", `{"disableAllHooks":true,"autoMemoryEnabled":false}`, "--setting-sources", "", "--permission-mode", "dontAsk", "--allowedTools", "mcp__denova__*"}
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
	cmd.Dir = dir
	cmd.Env = runEnvironment(cmd.Env)
	cmd.Stdin = bytes.NewReader(body)
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
	waited := false
	defer func() {
		cancel()
		if !waited {
			_ = cmd.Wait()
		}
	}()
	slog.InfoContext(ctx, "[external-runtime] Claude attempt started", "version", c.version, "model", model)
	output := streamOutput{tools: map[string]bool{}}
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
		if err = output.feed(scanner.Bytes(), host); err != nil {
			cancel()
			break
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		err = errors.Join(err, scanErr)
		cancel()
	}
	waitErr := cmd.Wait()
	waited = true
	result = output.result()
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

func runEnvironment(env []string) []string {
	overrides := map[string]string{"CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT": "0", "CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS": "0", "CLAUDE_AUTO_BACKGROUND_TASKS": "0", "CLAUDE_CODE_DISABLE_BACKGROUND_TASKS": "1", "CLAUDE_CODE_DISABLE_AUTO_MEMORY": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
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
	if err := add("Current user request:\n"+input.Text, input.Attachments); err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
	return append(body, '\n'), err
}
