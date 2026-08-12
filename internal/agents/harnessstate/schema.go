// Package harnessstate defines Denova's user-managed Harness State schema and
// materializes validated snapshots into runtime Agent contributions.
package harnessstate

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
	agentstate "github.com/alfredxw/denova/agent/state"
)

type runContextKey struct{}

// WithRunID binds Agent construction to one durable public Run snapshot.
func WithRunID(ctx context.Context, runID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runContextKey{}, strings.TrimSpace(runID))
}

func RunID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(runContextKey{}).(string)
	return strings.TrimSpace(value)
}

const (
	stateDirectoryName = "state"
	toolsFilePath      = "tools.toml"
)

// Harness is the complete, immutable contribution produced from one State
// snapshot. Maps and slices are private so callers cannot mutate a Run's
// effective policy after it has been admitted.
type Harness struct {
	revision         string
	token            string
	prompts          map[string]string
	contexts         []ContextFragment
	toolDescriptions map[string]string
	subAgents        []config.SubAgentConfig
}

type ContextFragment struct {
	ID        string
	Purpose   string
	Agents    []string
	Placement agent.ContextPlacement
	Content   string
	Resource  string
}

func (h Harness) Revision() string { return h.revision }

func (h Harness) Token() string { return h.token }

func (h Harness) Prompt(agentKind string) string {
	return h.prompts[strings.TrimSpace(agentKind)]
}

func (h Harness) SubAgents() []config.SubAgentConfig {
	result := make([]config.SubAgentConfig, len(h.subAgents))
	for index, subAgent := range h.subAgents {
		result[index] = subAgent
		result[index].Parents = append([]string(nil), subAgent.Parents...)
		result[index].Tools = make(config.AgentToolOverride, len(subAgent.Tools))
		for capability, enabled := range subAgent.Tools {
			result[index].Tools[capability] = enabled
		}
		if subAgent.Enabled != nil {
			enabled := *subAgent.Enabled
			result[index].Enabled = &enabled
		}
		if subAgent.Model.Temperature != nil {
			temperature := *subAgent.Model.Temperature
			result[index].Model.Temperature = &temperature
		}
	}
	return result
}

func (h Harness) ToolDescriptions() map[string]string {
	result := make(map[string]string, len(h.toolDescriptions))
	for name, description := range h.toolDescriptions {
		result[name] = description
	}
	return result
}

// Manager owns Harness-specific validation and runtime materialization on top
// of the provider-neutral agent/state store. Product history is deliberately
// outside this package and is composed by the Denova application layer.
type Manager struct {
	store        *agentstate.Store
	configSource ConfigSource
}

// ConfigSource resolves the current application configuration used for
// cross-reference and injection-budget validation. Long-lived hosts should
// return a fresh immutable snapshot so State never validates against stale
// model profiles or limits after settings change.
type ConfigSource func() *config.Config

func Open(cfg *config.Config) (*Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("open Harness State: config is nil")
	}
	return OpenWithConfigSource(func() *config.Config { return cfg })
}

func OpenWithConfigSource(source ConfigSource) (*Manager, error) {
	if source == nil {
		return nil, fmt.Errorf("open Harness State: config source is nil")
	}
	cfg := source()
	if cfg == nil {
		return nil, fmt.Errorf("open Harness State: config source returned nil")
	}
	dataDir := strings.TrimSpace(cfg.DataDir())
	if dataDir == "" {
		return nil, fmt.Errorf("open Harness State: Denova data directory is required")
	}
	manager := &Manager{configSource: source}
	store, err := agentstate.Open(agentstate.Options{
		Root:        filepath.Join(dataDir, stateDirectoryName),
		RuntimeRoot: filepath.Join(dataDir, "runtime", "harness-state"),
		Validator:   agentstate.ValidatorFunc(manager.validate),
	})
	if err != nil {
		return nil, fmt.Errorf("open Harness State: %w", err)
	}
	manager.store = store
	return manager, nil
}

func (m *Manager) Root() string {
	if m == nil || m.store == nil {
		return ""
	}
	return m.store.Root()
}

func (m *Manager) Store() *agentstate.Store {
	if m == nil {
		return nil
	}
	return m.store
}

func (m *Manager) Current(ctx context.Context) (Harness, error) {
	if m == nil || m.store == nil {
		return Harness{}, fmt.Errorf("Harness State manager is unavailable")
	}
	snapshot, err := m.store.Current(ctx)
	if err != nil {
		return Harness{}, err
	}
	return parse(snapshot, m.config())
}

// ForRun pins the current State to a stable public Agent Run identity. The
// same identity restores the exact content snapshot after a cold restart.
func (m *Manager) ForRun(ctx context.Context, runID string) (Harness, error) {
	if m == nil || m.store == nil {
		return Harness{}, fmt.Errorf("Harness State manager is unavailable")
	}
	snapshot, err := m.store.ForRun(ctx, runID)
	if err != nil {
		return Harness{}, err
	}
	return parse(snapshot, m.config())
}

// Load opens and validates the user State, pinning it when WithRunID is
// present. Build-time callers without a durable identity receive Current.
func Load(ctx context.Context, cfg *config.Config) (Harness, error) {
	if cfg == nil {
		return Harness{}, fmt.Errorf("load Harness State: config is nil")
	}
	if !cfg.Labs.ContinualLearning {
		return Harness{prompts: map[string]string{}, toolDescriptions: map[string]string{}}, nil
	}
	if strings.TrimSpace(cfg.DenovaDir) == "" && strings.TrimSpace(cfg.NovaDir) == "" {
		return Harness{prompts: map[string]string{}, toolDescriptions: map[string]string{}}, nil
	}
	manager, err := Open(cfg)
	if err != nil {
		return Harness{}, err
	}
	if runID := RunID(ctx); runID != "" {
		return manager.ForRun(ctx, runID)
	}
	return manager.Current(ctx)
}

func (m *Manager) validate(ctx context.Context, snapshot agentstate.Snapshot) []agentstate.Diagnostic {
	_, diagnostics := parseAll(ctx, snapshot, m.config())
	return diagnostics
}

func (m *Manager) config() *config.Config {
	if m == nil || m.configSource == nil {
		return nil
	}
	return m.configSource()
}

func parse(snapshot agentstate.Snapshot, cfg *config.Config) (Harness, error) {
	harness, diagnostics := parseAll(context.Background(), snapshot, cfg)
	if len(diagnostics) != 0 {
		return Harness{}, &agentstate.ValidationError{Diagnostics: diagnostics}
	}
	return harness, nil
}

func sortedAgentKinds() []string {
	definitions := config.AgentKindDefinitions()
	kinds := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		kinds = append(kinds, definition.Kind)
	}
	sort.Strings(kinds)
	return kinds
}
