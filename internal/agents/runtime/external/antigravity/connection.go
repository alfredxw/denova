// Package antigravity drives Antigravity CLI's native NDJSON headless protocol.
// Product history and committed tool effects remain owned by external.Host.
package antigravity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"denova/internal/agents/runtime/external"
	"denova/internal/hostruntime"
	"github.com/Masterminds/semver/v3"
)

// MinimumVersion is the version verified for unlimited headless NDJSON turns.
const MinimumVersion = "1.4.2"
const infrastructureTimeout = 15 * time.Second

// catalogTimeout covers the `models` command, which resolves the catalog over
// the network and needs 5-11s on a container behind a proxy. The control-plane
// timeout killed it mid-flight and surfaced as a failed connection.
const catalogTimeout = 40 * time.Second

var ErrVersionUnsupported = errors.New("unsupported Antigravity CLI version")
var errAuthenticationRequired = errors.New("Antigravity authentication required")

// ProcessOptions identifies an already discovered host-local CLI invocation.
type ProcessOptions struct {
	Executable string
	Args       []string
}

type Client struct {
	options ProcessOptions
	env     []string
	version string
	mu      sync.Mutex
	state   external.ConnectionState
	closed  bool
	active  map[*exec.Cmd]context.CancelFunc
}

func Connect(ctx context.Context, options ProcessOptions) (*Client, error) {
	if !filepath.IsAbs(options.Executable) {
		return nil, errors.New("Antigravity executable must be absolute")
	}
	c := &Client{options: options, env: hostruntime.WithSystemProxy(ctx, os.Environ()),
		state: external.ConnectionState{Status: "unchecked"}, active: map[*exec.Cmd]context.CancelFunc{}}
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	body, err := c.command(ctx, "--version").Output()
	if err != nil {
		return nil, fmt.Errorf("read Antigravity CLI version: %w", err)
	}
	version, err := semver.StrictNewVersion(strings.TrimSpace(string(body)))
	if err != nil || version.LessThan(semver.MustParse(MinimumVersion)) {
		return nil, ErrVersionUnsupported
	}
	c.version = version.String()
	slog.InfoContext(ctx, "[external-runtime] detected Antigravity CLI", "version", c.version)
	return c, nil
}

func (c *Client) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.options.Executable, append(append([]string{}, c.options.Args...), args...)...)
	cmd.Env, cmd.Stderr, cmd.WaitDelay = append([]string{}, c.env...), io.Discard, infrastructureTimeout
	configureProcess(cmd)
	return cmd
}

func (c *Client) Version() string { return c.version }
func (c *Client) Status() external.ConnectionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Client) Check(ctx context.Context) (external.ConnectionState, error) {
	state, _, err := c.Probe(ctx)
	return state, err
}

// Probe answers readiness and the catalog from the same `models` invocation. The
// CLI exposes no separate account handshake, so probing twice would spawn the
// process twice for one answer.
func (c *Client) Probe(ctx context.Context) (state external.ConnectionState, models external.Models, err error) {
	state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionFailed"}
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionLost"}
			err = errors.New("Antigravity connection is closed")
			models = external.Models{}
			c.state = state
			return
		}
		// Interrupted discovery retains the last confirmed readiness, while
		// authentication, isolation and catalog errors replace it.
		if state.Status == "unavailable" && c.state.Status == "ready" &&
			(errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
			state = c.state
			models = external.Models{}
			return
		}
		c.state = state
	}()
	if err := checkAmbientHooks(); err != nil {
		return external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.antigravityIsolationUnsupported"}, external.Models{}, err
	}
	models, err = c.Models(ctx)
	if errors.Is(err, errAuthenticationRequired) {
		return external.ConnectionState{Status: "auth_required"}, external.Models{}, nil
	}
	if err != nil {
		return state, external.Models{}, err
	}
	return external.ConnectionState{Status: "ready"}, models, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionLost"}
	for _, cancel := range c.active {
		cancel()
	}
	return nil
}

// Models uses the CLI's read-only catalog command, which currently exposes text
// columns rather than JSON. It never sends a prompt or starts a model turn.
func (c *Client) Models(ctx context.Context) (external.Models, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "denova-antigravity-models-")
	if err != nil {
		return external.Models{}, err
	}
	defer os.RemoveAll(dir)
	cmd := c.command(ctx, "models")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("")
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	body, err := cmd.Output()
	models, decodeErr := decodeModels(string(body))
	// Native 1.4.2 writes its unauthenticated catalog diagnostic to stderr
	// and exits 1; supported fixture/CLI variants also report it on stdout.
	if errors.Is(decodeErr, errAuthenticationRequired) || strings.Contains(diagnostics.String(), "Please sign in") || strings.Contains(diagnostics.String(), "authentication required") {
		return external.Models{}, errAuthenticationRequired
	}
	if err != nil {
		return external.Models{}, fmt.Errorf("read Antigravity models: %w", errors.Join(err, ctx.Err()))
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return decodeModels(diagnostics.String())
	}
	return models, decodeErr
}

func decodeModels(body string) (external.Models, error) {
	result := external.Models{Items: []external.Model{}}
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "Fetching available models..." {
			continue
		}
		if strings.Contains(line, "Please sign in") || strings.Contains(line, "authentication required") {
			return result, errAuthenticationRequired
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.Contains(fields[0], ":") || !strings.Contains(fields[0], "-") || seen[fields[0]] {
			return result, errors.New("Antigravity returned an invalid model catalog")
		}
		seen[fields[0]] = true
		result.Items = append(result.Items, external.Model{ID: fields[0], DisplayName: strings.Join(fields[1:], " "), Efforts: []string{}})
	}
	if len(result.Items) == 0 {
		return result, errors.New("Antigravity returned an empty model catalog")
	}
	// Preserve runtime-owned slugs (including their effort suffix). The CLI does
	// not mark a default in this catalog, so the first entry is only a UI choice.
	result.DefaultID = result.Items[0].ID
	return result, nil
}
