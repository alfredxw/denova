package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/runtime/external/antigravity"
	"denova/internal/agents/runtime/external/claude"
	"denova/internal/agents/runtime/external/codex"
	"denova/internal/agents/runtime/external/grok"
	"denova/internal/hostruntime"
)

var (
	ErrEngineNotFound         = errors.New("agent runtime is not registered")
	ErrEngineNotInstalled     = errors.New("agent runtime is not installed")
	ErrEngineNotReady         = errors.New("agent runtime is not ready")
	ErrEngineModelUnavailable = errors.New("agent runtime model or effort is unavailable")
)

// VersionUnsupportedReasonKey maps adapter version errors to localized product guidance.
// API handlers must not import a provider implementation to classify failures.
func VersionUnsupportedReasonKey(err error) string {
	switch {
	case errors.Is(err, codex.ErrVersionUnsupported):
		return "agentRuntime.incompatibleVersion"
	case errors.Is(err, claude.ErrVersionUnsupported):
		return "agentRuntime.claudeIncompatibleVersion"
	case errors.Is(err, antigravity.ErrVersionUnsupported):
		return "agentRuntime.antigravityIncompatibleVersion"
	case errors.Is(err, grok.ErrVersionUnsupported):
		return "agentRuntime.grokIncompatibleVersion"
	default:
		return ""
	}
}

// Engines owns host-local connections and their active-use leases. Creating or
// listing it has no filesystem, process, account, or network side effects.
// Native execution never calls Acquire and retains its existing runtime.
type Engines struct {
	controlsMu sync.Mutex
	controls   map[string]*ExternalController
	// Admissions may prepare concurrently. A selection change excludes their
	// final accept step across Writing and AgentChat, which share one journal.
	admission      sync.RWMutex
	entries        map[config.RuntimeID]*engineEntry
	Operations     external.Service
	apiMu          sync.Mutex
	apiConnections map[external.Connection]struct{}
	apiClosed      bool
	apiFactory     func(context.Context, config.RuntimeID, config.ResolvedModelSettings) (external.Connection, error)
}

// engineEntry holds one host-local engine's connection and its last known
// readiness. probeMu serializes connection replacement and every probe, so
// concurrent menu reads and admissions share one CLI invocation. mu only guards
// the fields below, so listing engines or reading status never waits for a
// process to start.
type engineEntry struct {
	probeMu      sync.Mutex
	mu           sync.Mutex
	id           config.RuntimeID
	factory      func(context.Context) (external.Connection, error)
	connection   external.Connection
	state        external.ConnectionState
	models       external.Models
	modelsExpiry time.Time
	retryAfter   time.Time
	users        int
	closed       bool
}

// engineModelsTTL bounds repeated CLI model discovery across menu opens and
// admissions. Only an explicit check bypasses it.
const engineModelsTTL = 5 * time.Minute

// engineRetryDelay bounds retries after a negative probe. Containers restart
// often and the model menu is reopened constantly; without a backoff a missing
// or signed-out CLI is spawned again on every read.
const engineRetryDelay = 30 * time.Second

func NewEngines() *Engines {
	return &Engines{apiConnections: make(map[external.Connection]struct{}), apiFactory: connectRuntimeAPI, entries: map[config.RuntimeID]*engineEntry{config.RuntimeCodex: {
		id: config.RuntimeCodex, state: external.ConnectionState{Status: "unchecked"}, factory: connectCodex,
	}, config.RuntimeClaude: {id: config.RuntimeClaude, state: external.ConnectionState{Status: "unchecked"}, factory: connectClaude},
		config.RuntimeGrok:        {id: config.RuntimeGrok, state: external.ConnectionState{Status: "unchecked"}, factory: connectGrok},
		config.RuntimeAntigravity: {id: config.RuntimeAntigravity, state: external.ConnectionState{Status: "unchecked"}, factory: connectAntigravity}}}
}

func connectGrok(ctx context.Context) (external.Connection, error) {
	executable := hostruntime.DiscoverGrok(os.Environ())
	if executable == "" {
		return nil, ErrEngineNotInstalled
	}
	return grok.Connect(ctx, grok.ProcessOptions{Executable: executable})
}

func connectAntigravity(ctx context.Context) (external.Connection, error) {
	executable := hostruntime.DiscoverAntigravity(os.Environ())
	if executable == "" {
		return nil, ErrEngineNotInstalled
	}
	return antigravity.Connect(ctx, antigravity.ProcessOptions{Executable: executable})
}

func connectClaude(ctx context.Context) (external.Connection, error) {
	launch := hostruntime.DiscoverClaude(os.Environ())
	if launch.Executable == "" {
		return nil, ErrEngineNotInstalled
	}
	return claude.Connect(ctx, claude.ProcessOptions{Launch: launch})
}

func connectCodex(ctx context.Context) (external.Connection, error) {
	executable := hostruntime.DiscoverCodex(os.Environ())
	if executable == "" {
		return nil, ErrEngineNotInstalled
	}
	home, err := hostruntime.CodexHome(os.Environ())
	if err != nil {
		return nil, err
	}
	return codex.Connect(ctx, codex.ProcessOptions{Executable: executable, Home: home})
}

func (engines *Engines) Catalog() []EngineDescriptor {
	items := []EngineDescriptor{engineDescriptor(config.RuntimeNative)}
	for _, id := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude, config.RuntimeGrok, config.RuntimeAntigravity} {
		entry := engines.entries[id]
		items = append(items, entry.descriptor())
	}
	return items
}

func (entry *engineEntry) descriptor() EngineDescriptor {
	_, state := entry.status()
	item := engineDescriptor(entry.id)
	item.Status, item.ReasonKey = state.Status, state.ReasonKey
	return item
}

// status reports readiness together with the connection it was read from. A live
// connection owns the current state; the cached state only covers the interval
// before the first connection exists.
func (entry *engineEntry) status() (external.Connection, external.ConnectionState) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	connection := entry.connection
	state := entry.state
	if connection != nil {
		state = connection.Status()
	}
	return connection, state
}

func (engines *Engines) entry(id config.RuntimeID) (*engineEntry, error) {
	entry := engines.entries[id]
	if entry == nil {
		if id != config.RuntimeNative {
			return nil, ErrEngineNotFound
		}
		return nil, conversationconfig.ErrRuntimeCapabilityUnsupported
	}
	return entry, nil
}

// Check is the only path that bypasses the catalog TTL and the retry backoff.
// It reloads CLI credentials and configuration, and reports the outcome through
// the descriptor instead of an error so the UI can render a status.
func (engines *Engines) Check(ctx context.Context, id config.RuntimeID) (EngineDescriptor, error) {
	if id == config.RuntimeNative {
		return engineDescriptor(id), nil
	}
	entry, err := engines.entry(id)
	if err != nil {
		return EngineDescriptor{}, err
	}
	entry.probeMu.Lock()
	_, _ = entry.catalog(ctx, true)
	entry.probeMu.Unlock()
	return entry.descriptor(), nil
}

// Models reports the engine catalog from cache and probes only when the cached
// readiness or catalog is stale. This is what the model menu reads.
func (engines *Engines) Models(ctx context.Context, id config.RuntimeID) (external.Models, error) {
	entry, err := engines.entry(id)
	if err != nil {
		return external.Models{}, err
	}
	entry.probeMu.Lock()
	defer entry.probeMu.Unlock()
	return entry.catalog(ctx, false)
}

// ValidateSelection admits a selection against the cached catalog. It starts a
// CLI process only when the catalog is cold, so changing model or effort inside a
// ready engine costs nothing.
func (engines *Engines) ValidateSelection(ctx context.Context, selection config.RuntimeSelection) error {
	entry, err := engines.entry(selection.Kind)
	if err != nil {
		return err
	}
	if err := selection.Validate(config.AgentKindGeneral); err != nil {
		return err
	}
	if selection.ModelProfileID() != "" {
		// API profiles resolve from settings; no CLI catalog is involved.
		return nil
	}
	entry.probeMu.Lock()
	defer entry.probeMu.Unlock()
	models, err := entry.catalog(ctx, false)
	if err != nil {
		return err
	}
	return validateEngineModel(models, selection)
}

// catalog resolves readiness and the model catalog. It is the only place that
// starts an engine process. Callers hold probeMu.
func (entry *engineEntry) catalog(ctx context.Context, force bool) (external.Models, error) {
	connection, state := entry.status()
	entry.mu.Lock()
	closed, users := entry.closed, entry.users
	expiry, cached, retryAfter := entry.modelsExpiry, entry.models, entry.retryAfter
	entry.mu.Unlock()
	if closed {
		return external.Models{}, ErrEngineNotReady
	}
	if !force {
		if state.Status == "ready" && time.Now().Before(expiry) {
			return cached, nil
		}
		if time.Now().Before(retryAfter) {
			// A ready engine that only failed a catalog read keeps serving its
			// last known list instead of failing the menu.
			if state.Status == "ready" && len(cached.Items) > 0 {
				return cached, nil
			}
			return external.Models{}, ErrEngineNotReady
		}
	}
	// Replacing a connection is only safe while nothing holds a lease on it.
	if connection != nil && ((force && users == 0) || state.Status == "unavailable") {
		if users != 0 {
			return external.Models{}, ErrOperationActive
		}
		_ = connection.Close()
		connection = nil
		entry.mu.Lock()
		entry.connection = nil
		entry.models, entry.modelsExpiry = external.Models{}, time.Time{}
		entry.mu.Unlock()
	}
	if connection == nil {
		created, err := entry.factory(ctx)
		if err != nil {
			entry.record(engineFailureState(entry.id, err), nil)
			return external.Models{}, err
		}
		connection = created
		entry.mu.Lock()
		entry.connection = created
		entry.mu.Unlock()
	}
	// A new connection is unchecked, so the first read also establishes
	// readiness. A ready connection is never re-probed: that spawned a CLI
	// process on every menu open and let one transient failure turn a working
	// runtime into an unavailable one.
	state = connection.Status()
	var probed *external.Models
	if state.Status == "unchecked" || (force && users != 0) {
		state, probed = probeReadiness(ctx, connection)
	}
	if state.Status != "ready" {
		entry.record(state, probed)
		return external.Models{}, ErrEngineNotReady
	}
	if probed != nil {
		// The probe already answered both questions, so its catalog is the
		// cached one. Without this the next read would run the same CLI command
		// again, and a transient failure there surfaced as a broken connection
		// right after a successful check.
		entry.record(state, probed)
		return *probed, nil
	}
	models, err := connection.Models(ctx)
	if err != nil {
		slog.WarnContext(ctx, "[agent-runtime] catalog read failed", "runtime", entry.id, "error", err)
		// A failed catalog read does not prove the engine signed out. Keep the
		// last known readiness and only delay the next attempt.
		entry.backoff()
		if state.Status == "ready" && len(cached.Items) > 0 {
			return cached, nil
		}
		return external.Models{}, err
	}
	entry.record(external.ConnectionState{Status: "ready"}, &models)
	return models, nil
}

// probeReadiness reads readiness once. Adapters whose readiness check is itself a
// catalog read return both, so Grok and Antigravity are not asked twice.
func probeReadiness(ctx context.Context, connection external.Connection) (external.ConnectionState, *external.Models) {
	if probe, ok := connection.(external.CatalogProbe); ok {
		state, models, err := probe.Probe(ctx)
		state = probeState(state, err)
		if state.Status != "ready" {
			return state, nil
		}
		return state, &models
	}
	state, err := connection.Check(ctx)
	return probeState(state, err), nil
}

func probeState(state external.ConnectionState, err error) external.ConnectionState {
	if state.Status == "" {
		state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionFailed"}
	}
	if key := VersionUnsupportedReasonKey(err); key != "" {
		state = external.ConnectionState{Status: "incompatible", ReasonKey: key}
	}
	return state
}

// record stores one probe outcome. Negative results are remembered for a short
// backoff so repeated reads do not respawn a missing or signed-out CLI.
func (entry *engineEntry) record(state external.ConnectionState, models *external.Models) {
	if state.Status == "" {
		state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionFailed"}
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.state = state
	if state.Status != "ready" {
		entry.models, entry.modelsExpiry = external.Models{}, time.Time{}
		entry.retryAfter = time.Now().Add(engineRetryDelay)
		return
	}
	if models != nil {
		entry.models = *models
	}
	entry.modelsExpiry, entry.retryAfter = time.Now().Add(engineModelsTTL), time.Time{}
}

func (entry *engineEntry) backoff() {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.retryAfter = time.Now().Add(engineRetryDelay)
}

func engineFailureState(id config.RuntimeID, err error) external.ConnectionState {
	state := external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionFailed"}
	if errors.Is(err, ErrEngineNotInstalled) {
		state = external.ConnectionState{Status: "not_installed", ReasonKey: "agentRuntime.notInstalled"}
	}
	if key := VersionUnsupportedReasonKey(err); key != "" {
		state = external.ConnectionState{Status: "incompatible", ReasonKey: key}
	}
	if state.Status == "not_installed" {
		switch id {
		case config.RuntimeClaude:
			state.ReasonKey = "agentRuntime.claudeNotInstalled"
		case config.RuntimeGrok:
			state.ReasonKey = "agentRuntime.grokNotInstalled"
		case config.RuntimeAntigravity:
			state.ReasonKey = "agentRuntime.antigravityNotInstalled"
		}
	}
	return state
}

// Acquire pins the connection from preflight through durable settlement. It
// validates against the cached catalog and only probes a cold engine, so
// switching model or effort inside a ready engine does not start a process.
// The caller releases on acceptance failure or after Wait, including cancellation.
func (engines *Engines) Acquire(ctx context.Context, selection config.RuntimeSelection, cfg config.Config) (external.Adapter, func(), error) {
	entry, err := engines.entry(selection.Kind)
	if err != nil {
		return nil, nil, err
	}
	if err := selection.Validate(config.AgentKindGeneral); err != nil {
		return nil, nil, err
	}
	if selection.ModelProfileID() != "" {
		return engines.acquireAPI(ctx, selection, cfg)
	}
	// Hold probeMu until the lease is pinned so an explicit check cannot replace
	// the connection between validation and the first turn.
	entry.probeMu.Lock()
	models, err := entry.catalog(ctx, false)
	if err != nil {
		entry.probeMu.Unlock()
		return nil, nil, err
	}
	if err := validateEngineModel(models, selection); err != nil {
		entry.probeMu.Unlock()
		return nil, nil, err
	}
	entry.mu.Lock()
	connection := entry.connection
	if connection == nil {
		entry.mu.Unlock()
		entry.probeMu.Unlock()
		return nil, nil, ErrEngineNotReady
	}
	entry.users++
	entry.mu.Unlock()
	entry.probeMu.Unlock()
	var once sync.Once
	return connection, func() { once.Do(func() { entry.mu.Lock(); entry.users--; entry.mu.Unlock() }) }, nil
}

// validateEngineModel reports whether the selection is served by the catalog the
// engine actually reports, including effort support.
func validateEngineModel(models external.Models, selection config.RuntimeSelection) error {
	var modelID, effort string
	switch selection.Kind {
	case config.RuntimeCodex:
		modelID, effort = selection.Codex.Model, selection.Codex.Effort
	case config.RuntimeClaude:
		modelID, effort = selection.Claude.Model, selection.Claude.Effort
	case config.RuntimeGrok:
		modelID, effort = selection.Grok.Model, selection.Grok.Effort
	case config.RuntimeAntigravity:
		modelID, effort = selection.Antigravity.Model, selection.Antigravity.Effort
	default:
		return ErrEngineNotFound
	}
	for _, model := range models.Items {
		if model.ID == modelID && (effort == "" || slices.Contains(model.Efforts, effort)) {
			return nil
		}
	}
	return ErrEngineModelUnavailable
}

func (engines *Engines) Close() error {
	var failures []error
	engines.apiMu.Lock()
	engines.apiClosed = true
	for connection := range engines.apiConnections {
		failures = append(failures, connection.Close())
		delete(engines.apiConnections, connection)
	}
	engines.apiMu.Unlock()
	for _, entry := range engines.entries {
		entry.probeMu.Lock()
		entry.mu.Lock()
		entry.closed = true
		entry.models, entry.modelsExpiry = external.Models{}, time.Time{}
		connection := entry.connection
		entry.mu.Unlock()
		if connection != nil {
			if err := connection.Close(); err != nil {
				failures = append(failures, fmt.Errorf("close %s: %w", entry.id, err))
			}
			entry.mu.Lock()
			entry.state = external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionLost"}
			entry.mu.Unlock()
		}
		entry.probeMu.Unlock()
	}
	return errors.Join(failures...)
}
