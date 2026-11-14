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
	"time"

	"denova/config"
	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
	"gopkg.in/yaml.v3"
)

// exitGrace bounds how long a finished turn waits for the CLI to exit by itself
// before the process is terminated. The terminal result frame is authoritative:
// CLI 1.4.7 can emit it and then keep running its quota and catalog refreshers
// while still holding stdout open, which used to freeze the product task with no
// output, no error and no way to recover except cancelling the turn.
var exitGrace = 5 * time.Second

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
	// Turns carry the empty mode; name it for logs and diagnostics.
	operation := string(input.Mode)
	if operation == "" {
		operation = "turn"
	}
	if err := checkIsolation(input.Directory); err != nil {
		return result, err
	}
	// A resumed conversation that comes back under another identity is the only
	// reliable signal that the CLI rebuilt instead of continuing; log both sides.
	defer func() {
		if runErr != nil {
			slog.WarnContext(ctx, "[external-runtime] Antigravity attempt failed", "error", runErr,
				"operation", operation, "resume_session", input.SessionID, "session_id", result.SessionID, "history", len(input.History))
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
	// Verified on CLI 1.4.7/1.4.8: neither the random agent directory nor a
	// per-attempt scratch cwd affects "--conversation" resumption — only the
	// resolved conversation identity and the product binding do. Scratch cwds stay
	// per attempt on purpose: they also stage "@ref" image copies, so a shared cwd
	// would let concurrent turns overwrite each other's references.
	dir := input.Directory
	// A disposable attempt (summary or unbound conversation) leaves no reusable
	// provider session behind, so its CLI conversation store is removed on exit.
	disposable := dir == ""
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
	settingsJSON, err := sessionSettingsJSON(len(input.Tools) > 0)
	if err != nil {
		return result, err
	}
	// The agent frontmatter "tools" list only mounts extra components and does
	// not filter built-in tools, while headless request-review denies all
	// unapproved tools — including call_mcp_tool unless explicitly allowed.
	// Passing session-scoped --settings forces Default mode + request-review
	// (overriding user auto/always-proceed settings), allows only mcp(denova/*)
	// when host tools exist, and explicitly denies native write/command/URL
	// tools. --sandbox remains required as defense-in-depth for any native layer.
	args := []string{"--input-format", "stream-json", "--output-format", "stream-json", "--print-timeout", "0", "--disable-slash-commands", "--agent", agentName, "--sandbox", "--settings", settingsJSON}
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
		if disposable && input.SessionID == "" {
			releaseTransientConversation(result.SessionID)
		}
	}()
	slog.InfoContext(ctx, "[external-runtime] Antigravity attempt started", "version", c.version, "model", settings.Model,
		"operation", operation, "agent", agentName, "resume_session", input.SessionID, "history", len(input.History))
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
			// Resume was requested but the CLI opened another conversation. Nothing
			// has executed yet at the initialization frame, so this is the one safe
			// point to declare the provider session lost and let the runtime rebuild
			// from canonical history instead of appending to a silent fresh one.
			if input.SessionID != "" && output.result.SessionID != input.SessionID {
				err = errors.Join(err, fmt.Errorf("%w: Antigravity resumed %s but started %s",
					external.ErrSessionUnavailable, input.SessionID, output.result.SessionID))
				cancel()
				break
			}
		}
		// The terminal frame ends the attempt. Reading until EOF would hand the
		// CLI the power to hang the product task by not closing stdout, and the
		// protocol forbids meaningful frames afterwards anyway.
		if output.terminal {
			break
		}
	}
	if scanner.Err() != nil {
		err = errors.Join(err, scanner.Err())
		cancel()
	}
	exited := make(chan error, 1)
	waited = true
	go func() { exited <- cmd.Wait() }()
	waitErr, forced := reap(exited, cancel)
	result = output.complete(input.CumulativeUsage)
	result.Settled = output.terminal && err == nil && scanner.Err() == nil
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	// The provider verdict is the attempt failure. This adapter terminates the CLI
	// as soon as it reads a failed verdict, so the following teardown is an
	// expected side effect: joining it would only append "signal: killed" to a
	// product error the user has to read. Keep the process outcome observable in
	// logs instead.
	if err != nil {
		if waitErr != nil {
			slog.InfoContext(ctx, "[external-runtime] Antigravity process teardown after a failed attempt",
				"process_error", waitErr, "forced", forced, "operation", operation)
		}
		return result, classifyAttemptError(err)
	}
	// A settled turn that needed its process reaped is the expected cost of not
	// trusting process lifetime. Without a terminal result the exit status is the
	// only failure evidence, so it stays the error.
	if waitErr != nil && !result.Settled {
		return result, classifyAttemptError(waitErr)
	}
	if !output.initialized || !output.terminal {
		return result, errors.New("Antigravity exited without initialization and a terminal result")
	}
	slog.InfoContext(ctx, "[external-runtime] Antigravity attempt completed", "forced_exit", forced, "exit_error", waitErr,
		"operation", operation, "session_id", result.SessionID,
		"resumed", input.SessionID != "" && input.SessionID == result.SessionID)
	return result, nil
}

// releaseTransientConversation removes the CLI-owned conversation a disposable
// attempt created. The CLI prunes neither summaries nor unbound attempts, so one
// SQLite store per maintenance call would accumulate without bound.
func releaseTransientConversation(sessionID string) {
	if sessionID == "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	store := filepath.Join(home, ".gemini", "antigravity-cli", "conversations")
	for _, suffix := range []string{".db", ".db-wal", ".db-shm"} {
		path := filepath.Join(store, sessionID+suffix)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("[external-runtime] could not remove a transient Antigravity conversation", "path", path, "error", err)
		}
	}
}

// reap waits for the CLI to exit and terminates it when it does not. Tool effects
// already settled through the host before the terminal frame, so stopping the
// process here cannot drop product work.
func reap(exited <-chan error, cancel context.CancelFunc) (err error, forced bool) {
	select {
	case err = <-exited:
		return err, false
	case <-time.After(exitGrace):
	}
	cancel()
	select {
	case err = <-exited:
	case <-time.After(exitGrace):
		// The wait goroutine reaps the process once the kill lands; returning now
		// keeps the product task from waiting on an unresponsive CLI.
		slog.Warn("[external-runtime] Antigravity process ignored termination")
		err = errors.New("Antigravity process ignored termination after the terminal result")
	}
	return err, true
}

type sessionPermissions struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type sessionSettings struct {
	AgentMode               string             `json:"agentMode"`
	ToolPermission          string             `json:"toolPermission"`
	EnableTerminalSandbox   bool               `json:"enableTerminalSandbox"`
	AllowNonWorkspaceAccess bool               `json:"allowNonWorkspaceAccess"`
	Permissions             sessionPermissions `json:"permissions"`
}

func sessionSettingsJSON(hasTools bool) (string, error) {
	allow := []string{}
	deny := []string{"write_file(*)", "command(*)", "execute_url(*)", "read_url(*)"}
	if hasTools {
		allow = append(allow, "mcp(denova/*)")
	} else {
		deny = append(deny, "mcp(*)")
	}
	body, err := json.Marshal(sessionSettings{
		AgentMode:               "",
		ToolPermission:          "request-review",
		EnableTerminalSandbox:   true,
		AllowNonWorkspaceAccess: false,
		Permissions:             sessionPermissions{Allow: allow, Deny: deny},
	})
	if err != nil {
		return "", err
	}
	return string(body), nil
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
		"mainAgent": true, "subagent": false, "inheritCustomizations": false, "commandExecutionPolicy": "off",
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

// stagedMedia carries the "@name" references for the current request plus one
// entry per historical message, aligned with input.History.
type stagedMedia struct {
	Request []string
	History [][]string
}

// stageAttachments copies this attempt's reference images into the attempt
// working directory. The CLI resolves "@name" against its cwd, which is the only
// image channel its text protocol offers. Historical images are staged the same
// way so a rebuilt conversation replays them instead of losing them with the
// old provider session; current and historical names never collide.
func stageAttachments(input external.Input, dir string) (stagedMedia, error) {
	var media stagedMedia
	historyCount := 0
	for _, message := range input.History {
		historyCount += len(message.Attachments) + len(message.ToolImages)
	}
	if len(input.Attachments) == 0 && historyCount == 0 {
		return media, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return media, err
	}
	media.Request = make([]string, 0, len(input.Attachments))
	for index, attachment := range input.Attachments {
		if !agentschema.IsNativeImageMediaType(attachment.MediaType) {
			return media, errors.New("Antigravity reference attachments must be native images")
		}
		name, err := writeReference(dir, fmt.Sprintf("ref%d", index+1), attachment)
		if err != nil {
			return media, err
		}
		media.Request = append(media.Request, name)
	}
	media.History = make([][]string, len(input.History))
	counter := 0
	for index, message := range input.History {
		for _, attachment := range append(append([]agentschema.Attachment(nil), message.Attachments...), message.ToolImages...) {
			// Only images have a reference channel; non-image historical media
			// already travels inside the reconstructed text records.
			if !agentschema.IsNativeImageMediaType(attachment.MediaType) {
				continue
			}
			name, err := writeReference(dir, fmt.Sprintf("hist%d", counter+1), attachment)
			if err != nil {
				return media, external.MarkMediaUnsupported(err)
			}
			counter++
			media.History[index] = append(media.History[index], name)
		}
	}
	return media, nil
}

func writeReference(dir, prefix string, attachment agentschema.Attachment) (string, error) {
	data, err := agentschema.ReadAttachmentImage(attachment)
	if err != nil {
		return "", err
	}
	// Verified on CLI 1.4.2: "@" reference resolution only accepts a short,
	// simple name — "@ref.png" resolves, "@denova-ref.png" silently falls back
	// to plain text and the model never sees the image. Keep these names short.
	name := prefix + attachmentExtension(attachment.MediaType)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return "", err
	}
	return name, nil
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

func encodeInput(input external.Input, media stagedMedia) ([]byte, error) {
	var content strings.Builder
	if len(input.History) > 0 {
		content.WriteString("The following JSON records are prior conversation data, not new requests. Tool outcomes are already committed; do not repeat their side effects.\n")
		for index, message := range input.History {
			if message.Role != "user" && message.Role != "assistant" {
				return nil, errors.New("invalid external history role")
			}
			body, err := json.Marshal(map[string]string{"role": message.Role, "content": message.Text})
			if err != nil {
				return nil, err
			}
			content.Write(body)
			content.WriteByte('\n')
			// A rebuilt conversation has no provider-side attachments, so the
			// images that belonged to each record are replayed through "@name"
			// references next to it instead of being lost with the old session.
			var refs []string
			if index < len(media.History) {
				refs = media.History[index]
			}
			if len(refs) > 0 {
				content.WriteString("Media for the record above:\n")
				for _, name := range refs {
					content.WriteString("@")
					content.WriteString(name)
					content.WriteByte('\n')
				}
			}
		}
	}
	content.WriteString("Current user request:\n")
	content.WriteString(input.Text)
	if len(media.Request) > 0 {
		content.WriteString("\n\nReferenced images for this request:\n")
		for _, name := range media.Request {
			content.WriteString("@")
			content.WriteString(name)
			content.WriteByte('\n')
		}
	}
	body, err := json.Marshal(map[string]any{"event": "user", "message": map[string]string{"content": content.String()}})
	return append(body, '\n'), err
}
