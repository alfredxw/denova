// Package grok drives Grok Build's headless single-turn channel. The CLI
// transcript is a disposable cache; product tools execute on Host.
package grok

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

const infrastructureTimeout = 15 * time.Second

// MinimumVersion is the released version exercised by the headless channel.
const MinimumVersion = "1.0.62"

var ErrVersionUnsupported = errors.New("unsupported Grok Build version")

// errAuthenticationRequired lets the engine report a signed-out CLI as an
// actionable login state instead of a generic connection failure.
var errAuthenticationRequired = errors.New("Grok authentication required")

// ProcessOptions contains only host-local executable discovery, never journal data.
type ProcessOptions struct {
	Executable string
	Args       []string
}

type Client struct {
	options ProcessOptions
	env     []string
	version string
	mu      sync.Mutex
	closed  bool
	state   external.ConnectionState
}

func Connect(ctx context.Context, options ProcessOptions) (*Client, error) {
	if !filepath.IsAbs(options.Executable) {
		return nil, errors.New("Grok executable must be absolute")
	}
	c := &Client{options: options, env: hostruntime.WithSystemProxy(ctx, os.Environ()), state: external.ConnectionState{Status: "unchecked"}}
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	body, err := c.command(ctx, "--version").Output()
	if err != nil {
		return nil, fmt.Errorf("read Grok Build version: %w", err)
	}
	fields := strings.Fields(string(body))
	if len(fields) < 2 || fields[0] != "grok" {
		return nil, ErrVersionUnsupported
	}
	version, err := semver.StrictNewVersion(fields[1])
	if err != nil || version.LessThan(semver.MustParse(MinimumVersion)) {
		return nil, ErrVersionUnsupported
	}
	c.version = version.String()
	return c, nil
}

func (c *Client) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.options.Executable, append(append([]string{}, c.options.Args...), args...)...)
	cmd.Env, cmd.Stderr, cmd.WaitDelay = append([]string{}, c.env...), io.Discard, infrastructureTimeout
	configureProcess(cmd)
	return cmd
}

func (c *Client) Version() string                  { return "headless-v1/" + c.version }
func (c *Client) Status() external.ConnectionState { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionLost"}
	return nil
}

func (c *Client) Check(ctx context.Context) (external.ConnectionState, error) {
	state, _, err := c.Probe(ctx)
	return state, err
}

// Probe answers readiness and the catalog from the same `models` invocation. The
// headless channel has no separate account handshake, so probing twice would
// spawn the process twice for one answer.
func (c *Client) Probe(ctx context.Context) (state external.ConnectionState, models external.Models, err error) {
	state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionFailed"}
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionLost"}
			err = errors.New("Grok connection is closed")
			models = external.Models{}
			c.state = state
			return
		}
		// Only interrupted probes retain the last confirmed readiness. Invalid
		// configuration and protocol failures must replace it.
		if state.Status == "unavailable" && c.state.Status == "ready" &&
			(errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
			state = c.state
			models = external.Models{}
			return
		}
		if errors.Is(err, ErrVersionUnsupported) {
			state = external.ConnectionState{Status: "incompatible", ReasonKey: "agentRuntime.grokIncompatibleVersion"}
		}
		c.state = state
	}()
	models, err = c.Models(ctx)
	if errors.Is(err, errAuthenticationRequired) {
		return external.ConnectionState{Status: "auth_required"}, external.Models{}, nil
	}
	if err != nil {
		return state, external.Models{}, err
	}
	return external.ConnectionState{Status: "ready"}, models, nil
}

// Models reads the CLI's own catalog command. The headless channel resolves the
// chat proxy catalog, so this is the list a product turn can actually select —
// the agent channel reported a different, smaller set.
func (c *Client) Models(ctx context.Context) (external.Models, error) {
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	cmd := c.command(ctx, "models")
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	body, err := cmd.Output()
	models, decodeErr := decodeModels(string(body))
	// The CLI may print its sign-in notice and a catalog together, so only an
	// otherwise unusable catalog becomes an authentication state.
	if decodeErr != nil && (unauthenticated(string(body)) || unauthenticated(diagnostics.String())) {
		return external.Models{}, errAuthenticationRequired
	}
	if err != nil && len(bytes.TrimSpace(body)) == 0 {
		return external.Models{}, fmt.Errorf("read Grok model catalog: %w", err)
	}
	if decodeErr != nil {
		return external.Models{}, decodeErr
	}
	return models, nil
}

// unauthenticated classifies the CLI's sign-in notice, which appears instead of a
// catalog when the local account is missing.
func unauthenticated(diagnostics string) bool {
	for _, marker := range []string{"not authenticated", "not logged in", "Please sign in", "sign in to"} {
		if strings.Contains(diagnostics, marker) {
			return true
		}
	}
	return false
}

func decodeModels(body string) (external.Models, error) {
	result := external.Models{Items: []external.Model{}}
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "Available models:" || unauthenticated(line) {
			continue
		}
		if value, ok := strings.CutPrefix(line, "Default model:"); ok {
			result.DefaultID = strings.TrimSpace(value)
			continue
		}
		name := strings.TrimSpace(strings.TrimLeft(line, "*-\u2022"))
		if name == "" || strings.Contains(name, ":") {
			continue
		}
		fields := strings.Fields(name)
		if len(fields) == 0 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		result.Items = append(result.Items, external.Model{ID: fields[0], DisplayName: fields[0], Efforts: []string{}})
	}
	if len(result.Items) == 0 {
		return result, errors.New("Grok returned an empty model catalog")
	}
	if result.DefaultID == "" || !seen[result.DefaultID] {
		result.DefaultID = result.Items[0].ID
	}
	return result, nil
}
