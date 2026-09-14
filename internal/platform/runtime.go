package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"denova/internal/portablepath"
)

// RuntimeConfiguration keeps mutable extension preferences separate from frozen
// game setup. Each activation gets a snapshot; saved preferences apply next start.
type RuntimeConfiguration struct {
	Settings map[string]any `json:"settings"`
	Setup    map[string]any `json:"setup,omitempty"`
	// Frozen host-Agent settings bypass later mutable extension preferences.
	frozenSettings map[string]map[string]any
}

type RuntimeContext struct {
	Source      ReleaseRef `json:"source"`
	Scope       Scope      `json:"scope"`
	Locale      string     `json:"locale"`
	Theme       string     `json:"theme"`
	Environment string     `json:"environment"`
	RuntimeConfiguration
}

// Connection is an ephemeral bearer connection delivered only to the trusted
// manager, a validated view handshake, or the owning backend's stdin pipe.
type Connection struct {
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token"`
}

type RuntimeSnapshot struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"`
	ViewURL    string         `json:"viewUrl,omitempty"`
	Connection Connection     `json:"connection"`
	Context    RuntimeContext `json:"context"`
}

type OpenOptions struct {
	// hostOnly runtimes have no browser consumer or view handshake.
	hostOnly     bool
	ParentOrigin string `json:"-"`
	Locale       string `json:"locale"`
	Theme        string `json:"theme"`
}

type activation struct {
	release    Release
	context    RuntimeContext
	connection Connection
	grants     []string
	process    *backendProcess
	dataDir    string
}

// Runtime is one game instance or explicitly selected plugin target. Multiple
// views share this runtime; dependency processes never cross instance scopes.
type Runtime struct {
	hostOnly     bool
	manager      *Manager
	id           string
	ctx          context.Context
	cancel       context.CancelFunc
	server       *http.Server
	baseURL      string
	parentOrigin string
	owner        *activation
	providers    map[string]*activation
	models       map[string]string
	dataMu       sync.Mutex
	mu           sync.RWMutex
	closeMu      sync.Mutex
	status       string
}

func (m *Manager) OpenInstance(ctx context.Context, id string, options OpenOptions) (RuntimeSnapshot, error) {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	if current := m.runtimes[id]; current != nil && current.ctx.Err() == nil {
		return current.snapshot(), nil
	}
	if err := m.stopLocked(ctx, id); err != nil {
		return RuntimeSnapshot{}, err
	}
	instance, err := m.Instance(id)
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Game, ID: instance.GameID}, ReleaseID: instance.ReleaseID})
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	// Re-resolve against the exact stored pins; never select an updated plugin
	// when opening an existing save, even if a newer compatible version exists.
	pins, err := m.resolveDependencies(release.Manifest, instance.Dependencies)
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	if !slices.Equal(pins, instance.Dependencies) {
		return RuntimeSnapshot{}, failure("DEPENDENCY_UNAVAILABLE", "Saved dependency binding is incomplete")
	}
	runtime, err := m.startRuntime(id, release, Scope{Kind: "game-instance", InstanceID: id, ProjectID: instance.ProjectID, StoryID: instance.StoryID}, pins, RuntimeConfiguration{Setup: instance.Setup}, instance.Models, instance.Preview, options)
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	m.runtimes[id] = runtime
	return runtime.snapshot(), nil
}

type ActivatePlugin struct {
	PluginID  string            `json:"pluginId"`
	ReleaseID string            `json:"releaseId"`
	Scope     Scope             `json:"scope"`
	Settings  map[string]any    `json:"settings"`
	Models    map[string]string `json:"models"`
	Preview   bool              `json:"preview"`
	OpenOptions
}

func (m *Manager) ActivatePlugin(ctx context.Context, request ActivatePlugin) (RuntimeSnapshot, error) {
	if request.Preview != strings.HasPrefix(request.ReleaseID, "preview-") {
		return RuntimeSnapshot{}, failure("INVALID_ARGUMENT", "Preview releases require isolated targets")
	}
	if request.Scope.Kind != "project" && request.Scope.Kind != "session" {
		return RuntimeSnapshot{}, failure("INVALID_ARGUMENT", "Plugin test target must be an explicit Project or Session")
	}
	if _, _, err := m.registry.Resolve(request.Scope.ProjectID, true); err != nil {
		return RuntimeSnapshot{}, err
	}
	if request.Scope.Kind == "session" && request.Scope.SessionID == "" {
		return RuntimeSnapshot{}, failure("INVALID_ARGUMENT", "Session target is required")
	}
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	id := stableID("plugin", request.PluginID, request.Scope.Kind, request.Scope.ProjectID, request.Scope.SessionID)
	if request.Preview {
		id = "preview-" + stableID(id, request.ReleaseID)
		request.Scope = Scope{Kind: "project", ProjectID: request.Scope.ProjectID, SessionID: id}
	}
	if current := m.runtimes[id]; current != nil && current.ctx.Err() == nil {
		configuration, err := validateConfiguration(&ConfigurationForm{Schema: map[string]any{"type": "object"}, Defaults: current.owner.context.Settings}, request.Settings)
		if err != nil {
			return RuntimeSnapshot{}, err
		}
		requested, _ := json.Marshal([]any{request.ReleaseID, configuration, request.Models})
		existing, _ := json.Marshal([]any{current.owner.release.Ref.ReleaseID, current.owner.context.Settings, current.models})
		if string(requested) != string(existing) {
			return RuntimeSnapshot{}, failure("DOCUMENT_CONFLICT", "Stop the active runtime before changing its release, models or configuration")
		}
		return current.snapshot(), nil
	}
	if err := m.stopLocked(ctx, id); err != nil {
		return RuntimeSnapshot{}, err
	}
	release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: request.PluginID}, ReleaseID: request.ReleaseID})
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	pins, err := m.resolveDependencies(release.Manifest, nil)
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	environment := "installed"
	if request.Preview {
		environment = "preview"
	}
	configuration, err := m.settingsValues(release, environment, request.Settings)
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	if err := m.validateModels(release, pins, request.Scope.ProjectID, request.Models); err != nil {
		return RuntimeSnapshot{}, err
	}
	runtime, err := m.startRuntime(id, release, request.Scope, pins, RuntimeConfiguration{Settings: configuration}, request.Models, request.Preview, request.OpenOptions)
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	m.runtimes[id] = runtime
	return runtime.snapshot(), nil
}

func (m *Manager) startRuntime(id string, owner Release, scope Scope, pins []DependencyPin, configuration RuntimeConfiguration, models map[string]string, preview bool, options OpenOptions) (*Runtime, error) {
	parent, err := url.Parse(options.ParentOrigin)
	if !options.hostOnly && (err != nil || parent.Host == "" || (parent.Scheme != "http" && parent.Scheme != "https") || parent.Path != "") {
		return nil, failure("INVALID_ARGUMENT", "A trusted parent origin is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &Runtime{manager: m, id: id, ctx: ctx, cancel: cancel, baseURL: "http://" + listener.Addr().String(), parentOrigin: options.ParentOrigin, providers: map[string]*activation{}, models: models, status: "starting", hostOnly: options.hostOnly}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = listener.Close()
			_ = runtime.close(context.Background())
		}
	}()
	releases := []Release{owner}
	for _, pin := range pins {
		release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: pin.PluginID}, ReleaseID: pin.ReleaseID})
		if err != nil {
			return nil, err
		}
		releases = append(releases, release)
	}
	for index, release := range releases {
		_, installed, err := m.release(release.Ref)
		if err != nil {
			return nil, err
		}
		if (!installed.Enabled && !options.hostOnly) || installed.Removed {
			return nil, failure("DEPENDENCY_UNAVAILABLE", "Package %s is disabled", release.Manifest.ID)
		}
		if release.Manifest.APIMajor != APIMajor {
			return nil, failure("API_INCOMPATIBLE", "Package %s requires API %d", release.Manifest.ID, release.Manifest.APIMajor)
		}
		if err := portablepath.PreflightTree(m.releasePath(release.Ref)); err != nil {
			return nil, err
		}
		environment := "installed"
		if preview {
			environment = "preview"
		}
		supplied := map[string]any{}
		if index == 0 {
			supplied = configuration.Settings
		}
		var settings map[string]any
		if configuration.frozenSettings != nil {
			var ok bool
			settings, ok = configuration.frozenSettings[release.Manifest.ID]
			if !ok {
				return nil, failure("INVALID_CONFIGURATION", "Missing frozen plugin settings")
			}
		} else {
			settings, err = m.settingsValues(release, environment, supplied)
		}
		if err != nil {
			return nil, err
		}
		locale := options.Locale
		if locale != "zh-CN" {
			locale = "en-US"
		}
		theme := options.Theme
		if theme != "light" {
			theme = "dark"
		}
		activation := &activation{release: release, context: RuntimeContext{Source: release.Ref, Scope: scope, Locale: locale, Theme: theme, Environment: environment, RuntimeConfiguration: RuntimeConfiguration{Settings: settings}}, grants: slices.Clone(release.Grants), connection: Connection{BaseURL: runtime.baseURL + "/api/platform/v1", Token: randomToken()}}
		if release.Ref.Package.Kind == Game {
			activation.context.Setup, err = m.gameSetup(release, configuration.Setup)
			if err != nil {
				return nil, err
			}
			activation.dataDir = filepath.Join(m.instancePath(release.Manifest.ID, scope.InstanceID), "data")
			if scope.StoryID != "" {
				_, layout, err := m.registry.Resolve(scope.ProjectID, true)
				if err != nil {
					return nil, err
				}
				activation.dataDir = filepath.Join(layout.StoreRoot, "extensions", release.Manifest.ID, scope.StoryID, "data")
			}
		} else {
			activation.dataDir = filepath.Join(m.packagePath(release.Ref.Package), "data", stableID(scope.Kind, scope.ProjectID, scope.InstanceID, scope.SessionID, scope.StoryID, scope.BranchID))
		}
		if err := os.MkdirAll(activation.dataDir, 0o700); err != nil {
			return nil, err
		}
		key := release.Manifest.ID
		if release.Ref.Package.Kind == Game {
			key = "local:"
		}
		runtime.providers[key] = activation
		if index == 0 {
			runtime.owner = activation
		}
	}
	runtime.server = &http.Server{Handler: runtime, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 32 << 10}
	launch("platform_http_runtime", cancel, func() {
		if err := runtime.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("platform_http_runtime_failed", "runtime", id, "error", err)
			cancel()
		}
	})
	for _, provider := range runtime.providers {
		if provider.release.Manifest.backend() != nil {
			process, err := startBackend(runtime, provider)
			if err != nil {
				return nil, err
			}
			provider.process = process
		}
	}
	runtime.mu.Lock()
	runtime.status = "running"
	runtime.mu.Unlock()
	succeeded = true
	launch("platform_runtime_cleanup", cancel, func() {
		<-ctx.Done()
		if err := runtime.close(context.Background()); err != nil {
			slog.Error("platform_runtime_cleanup_failed", "runtime", id, "error", err)
		}
	})
	slog.Info("platform_runtime_started", "runtime", id, "package", owner.Manifest.ID, "scope", scope.Kind)
	return runtime, nil
}

func (r *Runtime) snapshot() RuntimeSnapshot {
	r.mu.RLock()
	status := r.status
	r.mu.RUnlock()
	if r.ctx.Err() != nil && status != "stopped" {
		status = "failed"
	}
	viewURL := ""
	if r.owner.release.Manifest.Game != nil {
		viewURL = r.baseURL + "/views/" + r.owner.release.Manifest.Game.ViewID + "/"
	}
	return RuntimeSnapshot{ID: r.id, Status: status, ViewURL: viewURL, Connection: r.owner.connection, Context: r.owner.context}
}

func (r *Runtime) uses(kind Kind, id string) bool {
	for _, provider := range r.providers {
		if provider.release.Ref.Package == (PackageRef{Kind: kind, ID: id}) {
			return true
		}
	}
	return false
}

func (m *Manager) RuntimeSnapshots() []RuntimeSnapshot {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	items := []RuntimeSnapshot{}
	for _, runtime := range m.runtimes {
		item := runtime.snapshot()
		item.Connection = Connection{}
		items = append(items, item)
	}
	return items
}

func (m *Manager) Stop(ctx context.Context, id string) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	return m.stopLocked(ctx, id)
}
func (m *Manager) stopLocked(ctx context.Context, id string) error {
	runtime := m.runtimes[id]
	if runtime == nil {
		return nil
	}
	if err := runtime.close(ctx); err != nil {
		return err
	}
	delete(m.runtimes, id)
	return nil
}
func (m *Manager) Close(ctx context.Context) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	for id := range m.runtimes {
		if err := m.stopLocked(ctx, id); err != nil {
			return err
		}
	}
	if m.agents != nil {
		return m.agents.Close(ctx)
	}
	return nil
}

func (r *Runtime) close(ctx context.Context) error {
	r.closeMu.Lock()
	defer r.closeMu.Unlock()
	r.mu.Lock()
	if r.status == "stopped" {
		r.mu.Unlock()
		return nil
	}
	r.status = "stopping"
	r.mu.Unlock()
	r.cancel()
	if r.manager.stories != nil && r.owner != nil && r.owner.context.Scope.StoryID != "" {
		if err := r.manager.stories.StopRuntime(ctx, r.owner.context.Scope); err != nil {
			return err
		}
	}
	if r.manager.resources != nil {
		if err := r.manager.resources.StopRuntime(ctx, r); err != nil {
			return err
		}
	}
	if r.manager.agents != nil {
		if err := r.manager.agents.StopRuntime(ctx, r); err != nil {
			return err
		}
	}
	for _, provider := range r.providers {
		if provider.process != nil {
			provider.process.close()
		}
	}
	if r.server != nil {
		if err := r.server.Shutdown(ctx); err != nil {
			_ = r.server.Close()
			return err
		}
	}
	r.mu.Lock()
	r.status = "stopped"
	r.mu.Unlock()
	slog.Info("platform_runtime_stopped", "runtime", r.id)
	return nil
}

func randomToken() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Errorf("create runtime credential: %w", err))
	}
	return hex.EncodeToString(raw[:])
}

// launch gives every platform-owned goroutine a panic boundary and explicit
// failure cleanup. LLM tasks use the same boundary without a time limit.
func launch(operation string, failed func(), work func()) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("platform_worker_panicked", "operation", operation, "panic", recovered)
				failed()
			}
		}()
		work()
	}()
}
