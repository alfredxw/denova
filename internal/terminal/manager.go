package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	shlex "github.com/anmitsu/go-shlex"
)

const (
	// defaultScrollbackBytes caps the recent output retained per session. It backs the
	// screen restore on re-attach, so it needs a hard limit or a long-lived session eats memory.
	defaultScrollbackBytes = 256 * 1024
	// defaultSubscriberQueue caps the output backlog per WebSocket client, counted in chunks.
	// Beyond it the client is treated as lagging and dropped; it re-attaches and replays scrollback.
	defaultSubscriberQueue = 512

	defaultCols = 80
	defaultRows = 24
	minCols     = 2
	minRows     = 1
	maxCols     = 1000
	maxRows     = 500

	// DefaultMaxSessions is the default cap on concurrent terminal sessions.
	DefaultMaxSessions = 8
	// DefaultScrollbackKB is the default per-session scrollback size in KB.
	DefaultScrollbackKB = 256
	// MaxScrollbackKB bounds the configurable scrollback so a misconfiguration cannot blow up memory.
	MaxScrollbackKB = 4096
	// MaxSessionsLimit bounds the configurable number of concurrent sessions.
	MaxSessionsLimit = 64
)

// ErrDisabled reports that the terminal capability is switched off by configuration.
var ErrDisabled = errors.New("terminal disabled")

// ErrTooManySessions reports that the concurrent session limit is reached.
var ErrTooManySessions = errors.New("too many terminal sessions")

// ErrNotFound reports that the requested session does not exist.
var ErrNotFound = errors.New("terminal session not found")

// ErrInvalidProfile reports an unknown terminal launch profile.
var ErrInvalidProfile = errors.New("invalid terminal profile")

// ErrInvalidLaunchCommand reports a malformed or empty configured CLI command.
var ErrInvalidLaunchCommand = errors.New("invalid terminal launch command")

// ErrOwnerConflict reports an idempotency key reused by another Project.
var ErrOwnerConflict = errors.New("terminal owner belongs to another project")

// ErrProjectSessionsActive prevents Project relink/archive from leaving a
// live process bound to a superseded content directory.
var ErrProjectSessionsActive = errors.New("project has active terminal sessions")

// CommandProfile is one backend-authoritative CLI shortcut. Browser tabs keep
// only ID and display name; the executable command never enters local storage.
type CommandProfile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"-"`
	Enabled bool   `json:"-"`
}

// Config is the resolved terminal runtime configuration.
type Config struct {
	Enabled bool
	// An empty Shell is inferred from the platform ($SHELL / COMSPEC / platform default).
	Shell           string
	Commands        []CommandProfile
	MaxSessions     int
	ScrollbackBytes int
}

// DefaultConfig returns the built-in defaults.
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		Commands: []CommandProfile{
			{ID: "codex", Name: "Codex CLI", Command: "codex", Enabled: true},
			{ID: "claude", Name: "Claude Code", Command: "claude", Enabled: true},
		},
		MaxSessions:     DefaultMaxSessions,
		ScrollbackBytes: DefaultScrollbackKB * 1024,
	}
}

func (c Config) normalized() Config {
	out := c
	if out.MaxSessions <= 0 {
		out.MaxSessions = DefaultMaxSessions
	}
	if out.MaxSessions > MaxSessionsLimit {
		out.MaxSessions = MaxSessionsLimit
	}
	if out.ScrollbackBytes <= 0 {
		out.ScrollbackBytes = DefaultScrollbackKB * 1024
	}
	if out.ScrollbackBytes > MaxScrollbackKB*1024 {
		out.ScrollbackBytes = MaxScrollbackKB * 1024
	}
	out.Shell = strings.TrimSpace(out.Shell)
	if out.Commands == nil {
		out.Commands = DefaultConfig().Commands
	} else {
		out.Commands = append([]CommandProfile(nil), out.Commands...)
	}
	for index := range out.Commands {
		out.Commands[index].ID = strings.TrimSpace(out.Commands[index].ID)
		out.Commands[index].Name = strings.TrimSpace(out.Commands[index].Name)
		out.Commands[index].Command = strings.TrimSpace(out.Commands[index].Command)
	}
	return out
}

// Manager owns every live terminal session. Sessions exist independently of HTTP
// connections, so a browser reload or a tab switch can re-attach to a running pty.
type Manager struct {
	mu              sync.Mutex
	cfg             Config
	sessions        map[string]*Session
	sessionsByOwner map[string]string
}

// NewManager builds the terminal session manager.
func NewManager(cfg Config) *Manager {
	return &Manager{
		cfg:             cfg.normalized(),
		sessions:        map[string]*Session{},
		sessionsByOwner: map[string]string{},
	}
}

// SetConfig refreshes runtime limits after a configuration change (workspace switch or
// saved settings). Existing sessions are untouched; disabling the terminal closes them all.
func (m *Manager) SetConfig(cfg Config) {
	normalized := cfg.normalized()
	m.mu.Lock()
	previouslyEnabled := m.cfg.Enabled
	m.cfg = normalized
	m.mu.Unlock()
	if previouslyEnabled && !normalized.Enabled {
		slog.InfoContext(context.Background(), "[terminal/manager.go] terminal disabled by config, closing all sessions")
		m.CloseAll()
	}
}

// Config returns a snapshot of the current configuration.
func (m *Manager) Config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := m.cfg
	snapshot.Commands = append([]CommandProfile(nil), m.cfg.Commands...)
	return snapshot
}

// ResolveShell returns the default login shell: the configured one, else the platform default.
func (m *Manager) ResolveShell() string {
	m.mu.Lock()
	configured := m.cfg.Shell
	m.mu.Unlock()
	return resolveShell(configured)
}

// ResolveStartupCommand turns a stable profile ID into the configured command
// entered into the user's interactive shell. Executable command lines remain
// backend-authoritative and never cross the browser API.
func (m *Manager) ResolveStartupCommand(profileID string) (string, error) {
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()

	profileID = strings.TrimSpace(profileID)
	switch profileID {
	case "shell":
		return "", nil
	}
	for _, profile := range cfg.Commands {
		if profile.Enabled && profile.ID == profileID {
			if err := validateLaunchCommand(profile.Command); err != nil {
				return "", err
			}
			return profile.Command, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidProfile, profileID)
}

// AvailableCommands returns enabled menu metadata in configured order. It
// deliberately omits executable command lines from the API-facing value.
func (m *Manager) AvailableCommands() []CommandProfile {
	m.mu.Lock()
	commands := append([]CommandProfile(nil), m.cfg.Commands...)
	m.mu.Unlock()
	result := make([]CommandProfile, 0, len(commands))
	seen := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		if !command.Enabled || command.ID == "" || command.Name == "" || command.Command == "" {
			continue
		}
		if _, duplicate := seen[command.ID]; duplicate {
			continue
		}
		seen[command.ID] = struct{}{}
		result = append(result, CommandProfile{ID: command.ID, Name: command.Name})
	}
	return result
}

func validateLaunchCommand(commandLine string) error {
	parts, err := shlex.Split(strings.TrimSpace(commandLine), true)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLaunchCommand, err)
	}
	if len(parts) == 0 {
		return fmt.Errorf("%w: command is empty", ErrInvalidLaunchCommand)
	}
	return nil
}

func resolveShell(configured string) string {
	if configured != "" {
		return configured
	}
	return platformDefaultShell()
}

// Create starts a terminal session. A non-empty OwnerTabID is an idempotency key: concurrent
// effect runs and retried HTTP requests for one frontend tab all receive the same pty.
func (m *Manager) Create(spec Spec) (*Session, error) {
	m.mu.Lock()
	cfg := m.cfg
	if !cfg.Enabled {
		m.mu.Unlock()
		return nil, ErrDisabled
	}
	spec.OwnerTabID = strings.TrimSpace(spec.OwnerTabID)
	if spec.OwnerTabID != "" {
		if id, ok := m.sessionsByOwner[spec.OwnerTabID]; ok {
			if session, exists := m.sessions[id]; exists {
				if existingProjectID := strings.TrimSpace(session.Info().ProjectID); existingProjectID != strings.TrimSpace(spec.ProjectID) {
					m.mu.Unlock()
					return nil, fmt.Errorf("%w: owner_tab_id=%s", ErrOwnerConflict, spec.OwnerTabID)
				}
				m.mu.Unlock()
				slog.InfoContext(context.Background(), fmt.Sprintf("[terminal/manager.go] reused session id=%s owner_tab_id=%s", id, spec.OwnerTabID))
				return session, nil
			}
			delete(m.sessionsByOwner, spec.OwnerTabID)
		}
	}
	m.evictExitedLocked(cfg.MaxSessions)
	if len(m.sessions) >= cfg.MaxSessions {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: limit=%d", ErrTooManySessions, cfg.MaxSessions)
	}

	if strings.TrimSpace(spec.Command) == "" {
		spec.Command = resolveShell(cfg.Shell)
	}
	if strings.TrimSpace(spec.Title) == "" {
		spec.Title = describeCommand(spec.Command, spec.Args)
	}
	spec.Env = append(baseEnv(spec), spec.Env...)

	// Keep creation under the manager lock. Process startup is short, and serialising this small
	// critical section guarantees both the session cap and OwnerTabID idempotency under races.
	id := newID()
	token := newID()
	session, err := newSession(id, token, spec, cfg.ScrollbackBytes, m.handleSessionExit)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}

	m.sessions[id] = session
	if spec.OwnerTabID != "" {
		m.sessionsByOwner[spec.OwnerTabID] = id
	}
	total := len(m.sessions)
	m.mu.Unlock()
	slog.InfoContext(context.Background(), fmt.Sprintf("[terminal/manager.go] session created id=%s owner_tab_id=%s total=%d", id, spec.OwnerTabID, total))
	return session, nil
}

// Get returns the requested session.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return session, nil
}

// List returns session snapshots ordered by creation time.
func (m *Manager) List() []Info {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.Unlock()

	out := make([]Info, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, session.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// Close terminates and removes one session.
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		m.removeSessionLocked(id, session)
	}
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	session.Close()
	return nil
}

// CloseProject removes exited sessions owned by one Project and rejects the
// transition while any owned process is still running. Callers fence new
// Project-scoped creation before invoking this method.
func (m *Manager) CloseProject(projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return errors.New("terminal Project ID is required")
	}
	m.mu.Lock()
	for _, session := range m.sessions {
		info := session.Info()
		if info.ProjectID == projectID && !info.Exited {
			m.mu.Unlock()
			return fmt.Errorf("%w: project_id=%s session_id=%s", ErrProjectSessionsActive, projectID, info.ID)
		}
	}
	exited := make([]*Session, 0)
	for id, session := range m.sessions {
		info := session.Info()
		if info.ProjectID != projectID {
			continue
		}
		m.removeSessionLocked(id, session)
		exited = append(exited, session)
	}
	m.mu.Unlock()
	for _, session := range exited {
		session.Close()
	}
	return nil
}

// CloseAll terminates every session; called on process shutdown.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = map[string]*Session{}
	m.sessionsByOwner = map[string]string{}
	m.mu.Unlock()
	for _, session := range sessions {
		session.Close()
	}
	if len(sessions) > 0 {
		slog.InfoContext(context.Background(), fmt.Sprintf("[terminal/manager.go] closed all sessions count=%d", len(sessions)))
	}
}

// handleSessionExit keeps the session object when the process exits on its own (the frontend
// still reads the final output) and only logs. Reclaiming happens when the user closes the
// tab or when a new session triggers eviction.
func (m *Manager) handleSessionExit(session *Session) {
	code, message := session.ExitStatus()
	slog.InfoContext(context.Background(), fmt.Sprintf("[terminal/manager.go] session process exited id=%s code=%d err=%q", session.ID(), code, message))
}

// evictExitedLocked reclaims the oldest exited session with no attached client once the limit
// is hit, so "the process finished but the tab is still open" cannot hold a slot forever.
func (m *Manager) evictExitedLocked(limit int) {
	if len(m.sessions) < limit {
		return
	}
	type candidate struct {
		id   string
		info Info
	}
	candidates := make([]candidate, 0, len(m.sessions))
	for id, session := range m.sessions {
		info := session.Info()
		if info.Exited && info.Attached == 0 {
			candidates = append(candidates, candidate{id: id, info: info})
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].info.CreatedAt.Before(candidates[j].info.CreatedAt) })
	for _, item := range candidates {
		if len(m.sessions) < limit {
			return
		}
		session := m.sessions[item.id]
		m.removeSessionLocked(item.id, session)
		session.Close()
		slog.InfoContext(context.Background(), fmt.Sprintf("[terminal/manager.go] evicted exited session id=%s", item.id))
	}
}

func (m *Manager) removeSessionLocked(id string, session *Session) {
	delete(m.sessions, id)
	ownerTabID := session.OwnerTabID()
	if ownerTabID != "" && m.sessionsByOwner[ownerTabID] == id {
		delete(m.sessionsByOwner, ownerTabID)
	}
}

func newID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand does not fail on supported platforms; panicking beats a predictable id.
		panic(fmt.Sprintf("terminal: read random id: %v", err))
	}
	return hex.EncodeToString(buf)
}
