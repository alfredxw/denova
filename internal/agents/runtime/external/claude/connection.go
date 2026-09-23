// Package claude drives disposable Claude Code CLI sessions. Canonical history,
// tool effects and questions belong to external.Host, never CLI session files.
package claude

import (
	"context"
	"encoding/json"
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

	"denova/config"
	"denova/internal/agents/runtime/external"
	"denova/internal/hostruntime"
	"github.com/Masterminds/semver/v3"
)

// MinimumVersion includes restricted execution and the recorded stream format.
const MinimumVersion = "2.1.259"
const infrastructureTimeout = 15 * time.Second

var ErrVersionUnsupported = errors.New("unsupported Claude Code version")

// Client retains discovery and authentication state, not a shared CLI session.
// Each Run owns its process and tool bridge, so concurrent conversations isolate.
type Client struct {
	launch   hostruntime.ClaudeLaunch
	env      []string
	version  string
	mu       sync.Mutex
	state    external.ConnectionState
	closed   bool
	active   map[*exec.Cmd]context.CancelFunc
	apiModel string
}

// ProcessOptions carries host-local launch information and optional API routing.
// API is never persisted; nil uses the CLI's existing credentials.
type ProcessOptions struct {
	Launch hostruntime.ClaudeLaunch
	API    *config.ResolvedModelSettings
}

func Connect(ctx context.Context, options ProcessOptions) (*Client, error) {
	launch := options.Launch
	if !filepath.IsAbs(launch.Executable) {
		return nil, errors.New("Claude executable must be absolute")
	}
	c := &Client{launch: launch, env: hostruntime.WithSystemProxy(ctx, os.Environ()), active: map[*exec.Cmd]context.CancelFunc{}, state: external.ConnectionState{Status: "unchecked"}}
	if options.API != nil {
		c.env = apiEnvironment(c.env, *options.API)
		c.apiModel = options.API.Model
	}
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	body, err := c.command(ctx, "--version").Output()
	if err != nil {
		return nil, fmt.Errorf("read Claude Code version: %w", err)
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return nil, ErrVersionUnsupported
	}
	v, err := semver.StrictNewVersion(fields[0])
	if err != nil || v.LessThan(semver.MustParse(MinimumVersion)) {
		return nil, ErrVersionUnsupported
	}
	c.version = v.String()
	slog.InfoContext(ctx, "[external-runtime] detected Claude Code", "version", c.version)
	return c, nil
}

func (c *Client) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.launch.Executable, append(append([]string{}, c.launch.Args...), args...)...)
	cmd.Env = append([]string{}, c.env...)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = infrastructureTimeout
	configureProcess(cmd)
	return cmd
}

func (c *Client) Version() string                  { return c.version }
func (c *Client) Status() external.ConnectionState { c.mu.Lock(); defer c.mu.Unlock(); return c.state }

func (c *Client) Check(ctx context.Context) (state external.ConnectionState, err error) {
	state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionFailed"}
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionLost"}
			err = errors.New("Claude connection is closed")
		}
		c.state = state
	}()
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	// An explicit API route does not require a CLI subscription login. Actual
	// endpoint authentication is checked by the request, never by auth status.
	if c.apiModel != "" {
		return external.ConnectionState{Status: "ready"}, nil
	}
	body, probeErr := c.command(ctx, "auth", "status").Output()
	var auth struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if decodeErr := json.Unmarshal(body, &auth); decodeErr != nil {
		return state, fmt.Errorf("read Claude authentication status: %w", errors.Join(probeErr, decodeErr))
	}
	if !auth.LoggedIn {
		return external.ConnectionState{Status: "auth_required"}, nil
	}
	if probeErr != nil {
		return state, probeErr
	}
	return external.ConnectionState{Status: "ready"}, nil
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
