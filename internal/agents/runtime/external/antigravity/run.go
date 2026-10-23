package antigravity

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"denova/config"
	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
	"gopkg.in/yaml.v3"
)

func (c *Client) Run(ctx context.Context, input external.Input, host external.Host) (result external.Result, runErr error) {
	if input.Selection.Kind != config.RuntimeAntigravity || input.Selection.Antigravity == nil || host == nil {
		return result, errors.New("Antigravity attempt requires its own settings and host")
	}
	switch input.Mode {
	case external.OperationTurn:
	case external.OperationSummarize:
		input.SessionID, input.CumulativeUsage, input.Tools = "", nil, nil
	case external.OperationCompact, external.OperationEvaluate:
		return result, errors.New("Antigravity CLI does not support isolated evaluation or manual compaction")
	default:
		return result, fmt.Errorf("unknown Antigravity operation %q", input.Mode)
	}
	if err := checkAmbientHooks(); err != nil {
		return result, err
	}
	defer func() {
		if runErr != nil {
			slog.WarnContext(ctx, "[external-runtime] Antigravity attempt failed", "error", runErr)
		}
	}()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if input.Mode == external.OperationTurn {
		if controls := external.SteeringFromContext(ctx); controls != nil {
			stopped := make(chan struct{})
			defer func() { cancel(); <-stopped }()
			go func() {
				defer close(stopped)
				defer func() {
					if value := recover(); value != nil {
						slog.ErrorContext(ctx, "Antigravity steering failed", "panic", value)
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
							slog.ErrorContext(ctx, "Read Antigravity steering input failed", "error", err)
							cancel()
							return
						}
						if pending {
							// NDJSON accepts queued user turns, but rejects control
							// messages. The host retains guidance for interrupt/resume.
							controls.Interrupt()
							return
						}
					}
				}
			}()
		}
	}
	dir := input.Directory
	if dir == "" {
		temporary, err := os.MkdirTemp("", "denova-antigravity-")
		if err != nil {
			return result, err
		}
		dir = temporary
		defer os.RemoveAll(dir)
	}
	references, err := stageAttachments(input, dir)
	if err != nil {
		return result, err
	}
	body, err := encodeInput(input, references)
	if err != nil {
		return result, err
	}
	bridge, err := startBridge(runCtx, input.Tools, host, cancel)
	if err != nil {
		return result, err
	}
	var cleanupAgent func() error
	defer func() {
		cancel()
		runErr = errors.Join(runErr, bridge.close())
		if cleanupAgent != nil {
			runErr = errors.Join(runErr, cleanupAgent())
		}
	}()
	agentName, cleanupAgent, err := writeAgent(input, bridge)
	if err != nil {
		return result, err
	}
	// Verified on CLI 1.4.2: the agent frontmatter "tools" list only mounts extra
	// components, it does not filter the built-in toolset, and no approval mode
	// works here — headless request-review denies call_mcp_tool itself
	// (denied_actions: mcp), so skipping permissions is what keeps product tools
	// usable. --sandbox is the boundary that remains: it contains the native
	// command tools this adapter cannot refuse, while MCP dispatch is unaffected.
	// It stays on even for a full-access conversation, because product access is
	// governed by the conversation tool settings and this CLI cannot express them.
	args := []string{"--input-format", "stream-json", "--output-format", "stream-json", "--print-timeout", "0", "--disable-slash-commands", "--agent", agentName, "--sandbox", "--dangerously-skip-permissions"}
	if input.SessionID != "" {
		args = append(args, "--conversation", input.SessionID)
	}
	settings := input.Selection.Antigravity
	if settings.Model == "" {
		return result, errors.New("Antigravity model was not selected")
	}
	args = append(args, "--model", settings.Model)
	if settings.Effort != "" {
		// Antigravity encodes reasoning strength in the model slug and its
		// catalog advertises no separate effort switch, so no such flag is sent.
		slog.WarnContext(ctx, "[external-runtime] ignoring Antigravity effort; strength belongs to the model slug", "effort", settings.Effort)
	}
	cmd := c.command(runCtx, args...)
	cmd.Dir = dir
	// Closing stdin after one NDJSON user event ends the process only after the
	// current result. No prompt content is put in process command arguments.
	cmd.Stdin = bytes.NewReader(body)
	reader, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	defer reader.Close()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return result, errors.New("Antigravity connection is closed")
	}
	err = cmd.Start()
	if err == nil {
		c.active[cmd] = cancel
	}
	c.mu.Unlock()
	if err != nil {
		return result, classifyAttemptError(fmt.Errorf("start Antigravity attempt: %w", err))
	}
	defer func() { c.mu.Lock(); delete(c.active, cmd); c.mu.Unlock() }()
	waited := false
	defer func() {
		cancel()
		if !waited {
			_ = cmd.Wait()
		}
	}()
	slog.InfoContext(ctx, "[external-runtime] Antigravity attempt started", "version", c.version, "model", settings.Model)
	output := streamOutput{allowed: map[string]bool{}}
	if len(input.Tools) > 0 {
		output.allowed["call_mcp_tool"] = true
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 32<<20)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		initialized := output.initialized
		if feedErr := output.feed(scanner.Bytes(), host); feedErr != nil {
			err = errors.Join(err, feedErr)
			cancel()
			break
		}
		if !initialized && output.initialized {
			close(bridge.ready)
		}
	}
	if scanner.Err() != nil {
		err = errors.Join(err, scanner.Err())
		cancel()
	}
	waitErr := cmd.Wait()
	waited = true
	result = output.complete(input.CumulativeUsage)
	result.Settled = output.terminal && err == nil && scanner.Err() == nil
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil || waitErr != nil {
		return result, classifyAttemptError(errors.Join(err, waitErr))
	}
	if !output.initialized || !output.terminal {
		return result, errors.New("Antigravity exited without initialization and a terminal result")
	}
	slog.InfoContext(ctx, "[external-runtime] Antigravity attempt completed")
	return result, nil
}

// inheritCustomizations is the ambient-isolation switch: set to false it stops
// the agent from adopting the user's skills, rules, plugins, subagents and MCP
// servers (verified on CLI 1.4.2). excludeDefaultComponents is deliberately left
// off: it removes the default file-reading component, which is exactly what the
// "@name" reference channel depends on. A headless stream-json turn accepts only
// text content blocks (no image/base64 block), so "@name" against the cwd is the
// single image channel and must keep the default file reader. Native command
// tools are contained by --sandbox instead, and product effects still settle only
// through the host MCP dispatcher. Do not substitute another CLI's
// restricted/settings flags. Source: antigravity.google/docs/cli/headless and
// antigravity.google/docs/changelog?tab=cli.
// Native 1.4.2 discovery was verified against its global agents command. A
// scratch cwd's workspace definition was not discovered, so create one private
// transient definition in the CLI-owned directory. Never edit a user definition;
// remove only this randomly named directory after process/tool settlement.
func writeAgent(input external.Input, bridge *toolBridge) (string, func() error, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	root := filepath.Join(home, ".gemini", "config", "agents")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", nil, err
	}
	agentDir := filepath.Join(root, "denova-"+rand.Text())
	if err := os.Mkdir(agentDir, 0700); err != nil {
		return "", nil, err
	}
	cleanup := func() error { return os.RemoveAll(agentDir) }
	name := filepath.Base(agentDir)
	// Verified on CLI 1.4.2: "tools" mounts named components and call_mcp_tool is
	// not one of them. Declaring it fails executor construction with
	// `unknown component: tool "call_mcp_tool" not found in registry`, aborting
	// every turn that carries product tools. MCP dispatch is built in and becomes
	// reachable from the non-empty mcpServers list.
	tools := []string{}
	servers := []map[string]any{}
	if len(input.Tools) > 0 {
		servers = append(servers, bridge.config())
	}
	frontmatter, err := yaml.Marshal(map[string]any{
		"name": name, "description": "Execute the accepted product conversation with its scoped tools.",
		"mainAgent": true, "subagent": false, "inheritCustomizations": false,
		"tools": tools, "mcpServers": servers,
	})
	if err != nil {
		return "", cleanup, err
	}
	var content strings.Builder
	content.WriteString("---\n")
	content.Write(frontmatter)
	content.WriteString("---\n\n# System Prompt\n")
	content.WriteString(input.Instructions)
	if len(input.Tools) > 0 {
		catalog, err := json.Marshal(input.Tools)
		if err != nil {
			return "", cleanup, err
		}
		// Same rule the Codex adapter gives its engine: the CLI working directory is
		// disposable scratch space, never the Project, and the native tools cannot be
		// removed from this agent profile.
		content.WriteString("\n\nThe process working directory is temporary scratch space, not the project. Use the provided host tools for all project reads and changes, and do not use built-in file, shell or browser tools to bypass them. The following JSON is the complete product tool catalog for this attempt; invoke these tools through call_mcp_tool on server denova. Tool outcomes are committed by the host; never reproduce their side effects independently.\n")
		content.Write(catalog)
	}
	err = os.WriteFile(filepath.Join(agentDir, "agent.md"), []byte(content.String()), 0600)
	return name, cleanup, err
}

// stageAttachments copies this attempt's reference images into the attempt
// working directory. The CLI resolves "@name" against its cwd, which is the only
// image channel its text protocol offers.
func stageAttachments(input external.Input, dir string) ([]string, error) {
	if len(input.Attachments) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(input.Attachments))
	for index, attachment := range input.Attachments {
		if !agentschema.IsNativeImageMediaType(attachment.MediaType) {
			return nil, errors.New("Antigravity reference attachments must be native images")
		}
		data, err := agentschema.ReadAttachmentImage(attachment)
		if err != nil {
			return nil, err
		}
		// Verified on CLI 1.4.2: "@" reference resolution only accepts a short,
		// simple name — "@ref.png" resolves, "@denova-ref.png" silently falls back
		// to plain text and the model never sees the image. Keep these names short.
		name := fmt.Sprintf("ref%d%s", index+1, attachmentExtension(attachment.MediaType))
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

func attachmentExtension(mediaType string) string {
	switch mediaType {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

func encodeInput(input external.Input, references []string) ([]byte, error) {
	var content strings.Builder
	if len(input.History) > 0 {
		content.WriteString("The following JSON records are prior conversation data, not new requests. Tool outcomes are already committed; do not repeat their side effects.\n")
		for _, message := range input.History {
			if message.Role != "user" && message.Role != "assistant" {
				return nil, errors.New("invalid external history role")
			}
			// Historical images stay with the provider conversation that received
			// them; a resumed conversation does not need them replayed.
			body, err := json.Marshal(map[string]string{"role": message.Role, "content": message.Text})
			if err != nil {
				return nil, err
			}
			content.Write(body)
			content.WriteByte('\n')
		}
	}
	content.WriteString("Current user request:\n")
	content.WriteString(input.Text)
	if len(references) > 0 {
		content.WriteString("\n\nReferenced images for this request:\n")
		for _, name := range references {
			content.WriteString("@")
			content.WriteString(name)
			content.WriteByte('\n')
		}
	}
	body, err := json.Marshal(map[string]any{"event": "user", "message": map[string]string{"content": content.String()}})
	return append(body, '\n'), err
}
