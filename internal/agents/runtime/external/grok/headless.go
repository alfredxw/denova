package grok

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"denova/config"
	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/image/draw"

	_ "image/gif"
	_ "image/png"
)

// maxPromptArgumentBytes keeps the JSON prompt inside the kernel's per-argument
// limit (128 KiB on Linux), which is the only channel that carries images: the
// CLI has no stdin input format on this channel.
const maxPromptArgumentBytes = 120 << 10

// hostTools are the only built-in tools a product turn keeps. Everything that
// can change the project must travel through use_tool to the host MCP server;
// the rest of the CLI toolset would bypass the product journal.
var hostTools = []string{"todo_write", "search_tool", "use_tool"}

// isolationConfig is the private base configuration every attempt starts from.
// It closes the ambient surfaces that would otherwise inject user MCP servers,
// hooks, plugins or skills into a product turn.
func isolationConfig() map[string]any {
	compat := func(agents, mcps, rules bool) map[string]any {
		return map[string]any{"agents": agents, "hooks": false, "mcps": mcps, "rules": rules, "skills": false}
	}
	return map[string]any{
		"cli":          map[string]any{"auto_update": false},
		"session":      map[string]any{"load_envrc": false},
		"managed_mcps": map[string]any{"enabled": false, "gateway_tools_enabled": false},
		"compat": map[string]any{
			"claude": compat(false, false, false),
			"cursor": compat(false, false, false),
			"codex":  map[string]any{"hooks": false, "skills": false},
		},
		"permission": map[string]any{"deny": []any{"Read(**)"}},
	}
}

// attemptConfig merges the filtered user model routing with the attempt-owned
// sections: the host MCP server and the conversation context window. Both only
// exist in this attempt's private home, never in the user's CLI configuration.
func attemptConfig(models []byte, model string, contextWindow int, server map[string]any) ([]byte, error) {
	source := isolationConfig()
	if len(models) > 0 {
		var user map[string]any
		if err := toml.Unmarshal(models, &user); err != nil {
			return nil, err
		}
		for key, value := range user {
			source[key] = value
		}
	}
	if server != nil {
		source["mcp_servers"] = map[string]any{"denova": server}
	}
	if contextWindow > 0 {
		section, _ := source["model"].(map[string]any)
		if section == nil {
			section = map[string]any{}
		}
		entry, _ := section[model].(map[string]any)
		if entry == nil {
			entry = map[string]any{}
		}
		entry["context_window"] = contextWindow
		section[model] = entry
		source["model"] = section
	}
	return toml.Marshal(source)
}

type headlessAttempt struct {
	cmd        *exec.Cmd
	out        io.ReadCloser
	home       string
	promptPath string
	removeDir  string
	bridge     *toolBridge
	diagnostic *boundedBuffer
}

// boundedBuffer keeps the last slice of CLI diagnostics without letting a
// chatty failure grow without bound. The text is never shown verbatim; it backs
// a protocol error whose cause would otherwise be invisible.
type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.Buffer.Write(p)
	if b.Len() > 8<<10 {
		trimmed := b.Bytes()[b.Len()-8<<10:]
		b.Reset()
		b.Write(trimmed)
	}
	return len(p), nil
}

func (a *headlessAttempt) close() error {
	if a.out != nil {
		_ = a.out.Close()
	}
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
		_ = a.cmd.Wait()
	}
	var err error
	if a.bridge != nil {
		err = a.bridge.close()
	}
	_ = os.Remove(a.promptPath)
	if a.removeDir != "" {
		_ = os.RemoveAll(a.removeDir)
	}
	return err
}

// startHeadless launches one product turn on the headless channel. The agent
// stdio channel exposes only the model set its built-in profile knows, while
// this channel resolves the chat proxy catalog where newer models live.
func (c *Client) startHeadless(ctx context.Context, input external.Input, host external.Host, settings config.GrokRuntimeSettings, instructions, prompt string) (_ *headlessAttempt, err error) {
	dir := input.Directory
	if dir == "" {
		dir, err = os.MkdirTemp("", "denova-grok-headless-")
		if err != nil {
			return nil, err
		}
	}
	attempt := &headlessAttempt{home: filepath.Join(dir, "grok-headless-home"), removeDir: ""}
	if input.Directory == "" {
		attempt.removeDir = dir
	}
	defer func() {
		if err != nil {
			_ = attempt.close()
		}
	}()
	if err := os.MkdirAll(attempt.home, 0o700); err != nil {
		return nil, err
	}
	if len(input.Tools) > 0 {
		bridge, err := startBridge(ctx, input.Tools, host)
		if err != nil {
			return nil, err
		}
		attempt.bridge = bridge
	}
	var server map[string]any
	if attempt.bridge != nil {
		server = attempt.bridge.config()
	}
	models, _, err := modelConfiguration(c.env)
	if err != nil {
		return nil, fmt.Errorf("read Grok model configuration: %w", err)
	}
	body, err := attemptConfig(models, settings.Model, settings.ContextWindow, server)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(attempt.home, "config.toml"), body, 0o600); err != nil {
		return nil, err
	}
	// An image-bearing turn must travel as one JSON argument: this channel has no
	// stdin input format. A text-only turn keeps the prompt out of the process
	// arguments entirely.
	args := append([]string{}, c.options.Args...)
	if len(input.Attachments) > 0 {
		encoded, err := promptArgument(prompt, input.Attachments)
		if err != nil {
			return nil, err
		}
		args = append(args, "--prompt-json", encoded)
	} else {
		attempt.promptPath = filepath.Join(attempt.home, "prompt.txt")
		if err := os.WriteFile(attempt.promptPath, []byte(prompt), 0o600); err != nil {
			return nil, err
		}
		args = append(args, "--prompt-file", attempt.promptPath)
	}
	args = append(args,
		"--output-format", "streaming-json",
		"--model", settings.Model,
		"--always-approve",
		"--tools", strings.Join(hostTools, ","),
	)
	if instructions != "" {
		args = append(args, "--system-prompt-override", instructions)
	}
	if input.SessionID != "" {
		args = append(args, "-r", input.SessionID)
	}
	cmd := exec.CommandContext(ctx, c.options.Executable, args...)
	cmd.Dir = filepath.Join(dir, "cwd")
	if err := os.MkdirAll(cmd.Dir, 0o700); err != nil {
		return nil, err
	}
	// The private home has no credentials of its own: keep the CLI's existing auth
	// file in place instead of copying a secret into an attempt directory.
	auth := environmentValue(c.env, "GROK_AUTH_PATH")
	if auth == "" {
		auth = filepath.Join(originalHome(c.env), "auth.json")
	}
	cmd.Env = isolatedEnv(c.env, attempt.home, auth)
	attempt.diagnostic = &boundedBuffer{}
	cmd.Stderr = attempt.diagnostic
	configureProcess(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	attempt.out, attempt.cmd = out, cmd
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Grok headless attempt: %w", err)
	}
	return attempt, nil
}

// promptArgument encodes the turn for --prompt-json. The headless channel has no
// stdin input format, so images travel as one JSON argument; each one is
// re-encoded to fit the per-argument limit instead of being dropped or failing
// the turn.
func promptArgument(prompt string, attachments []agentschema.Attachment) (string, error) {
	available := maxPromptArgumentBytes - len(prompt) - 1024
	if available < 8<<10 {
		return "", errors.New("Grok headless prompt is too long to carry images")
	}
	budget := available / len(attachments)
	blocks := make([]map[string]any, 0, len(attachments)+1)
	blocks = append(blocks, map[string]any{"type": "text", "text": prompt})
	for _, attachment := range attachments {
		if !agentschema.IsNativeImageMediaType(attachment.MediaType) {
			return "", errors.New("Grok headless prompt accepts image attachments only")
		}
		data, err := encodeAttachment(attachment, budget)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, map[string]any{"type": "image", "data": data, "mimeType": "image/jpeg"})
	}
	body, err := json.Marshal(blocks)
	if err != nil {
		return "", err
	}
	if len(body) > maxPromptArgumentBytes {
		return "", errors.New("Grok headless prompt exceeded its argument limit")
	}
	return string(body), nil
}

func encodeAttachment(attachment agentschema.Attachment, budget int) (string, error) {
	raw, err := agentschema.ReadAttachmentImage(attachment)
	if err != nil {
		return "", err
	}
	source, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", errors.New("attachment is not a decodable image")
	}
	for _, size := range []int{1024, 768, 512, 384, 256} {
		scaled := scaleImage(source, size)
		for _, quality := range []int{80, 70, 60, 50, 40} {
			encoded, err := encodeJPEG(scaled, quality)
			if err != nil {
				return "", err
			}
			data := base64.StdEncoding.EncodeToString(encoded)
			if len(data) <= budget {
				return data, nil
			}
		}
	}
	return "", errors.New("attachment cannot be compressed into the Grok headless prompt")
}

func scaleImage(source image.Image, max int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= max && height <= max {
		return source
	}
	if width >= height {
		height, width = height*max/width, max
	} else {
		width, height = width*max/height, max
	}
	target := image.NewRGBA(image.Rect(0, 0, maxInt(width, 1), maxInt(height, 1)))
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	return target
}

// encodeJPEG flattens transparency onto white: JPEG has no alpha channel, and a
// reference image must not turn black where the source was transparent.
func encodeJPEG(source image.Image, quality int) ([]byte, error) {
	bounds := source.Bounds()
	flat := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), source, bounds.Min, draw.Over)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, flat, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

// isolatedEnv pins this attempt to its private home while inheriting the host
// environment (credentials, proxy) exactly as the CLI expects.
func isolatedEnv(environment []string, home, authPath string) []string {
	result := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		// GROK_HOME selects the private tree; GROK_CONFIG would let an ambient
		// value point the CLI at a configuration this attempt never inspected.
		if strings.EqualFold(key, "GROK_HOME") || strings.EqualFold(key, "GROK_CONFIG") {
			continue
		}
		result = append(result, entry)
	}
	result = append(result, "GROK_HOME="+home, "GROK_CONFIG=")
	if authPath != "" {
		result = append(result, "GROK_AUTH_PATH="+authPath)
	}
	return result
}
