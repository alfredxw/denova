// Package harnessstate defines Denova's user-managed Harness State schema and
// materializes validated snapshots into runtime Agent contributions.
package harnessstate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
	agentscript "github.com/alfredxw/denova/agent/script"
	agentstate "github.com/alfredxw/denova/agent/state"
	publictools "github.com/alfredxw/denova/agent/tools"
	"github.com/invopop/jsonschema"
)

const (
	stateDirectoryName = "state"
	toolsFilePath      = "tools.toml"
)

// Harness is the complete contribution produced from the live State
// directory. Maps and slices are private so callers cannot mutate one Agent
// build after it has been admitted.
type Harness struct {
	prompts          map[string]string
	contexts         []ContextFragment
	toolDescriptions map[string]string
	subAgents        []config.SubAgentConfig
	scriptTools      []ScriptTool
}

// ScriptTool is one validated Harness file and its immutable compiled Program.
// Fields remain private so an admitted Harness cannot be mutated by callers.
type ScriptTool struct {
	name        string
	description string
	agents      []string
	enabled     bool
	resource    string
	inputSchema *jsonschema.Schema
	program     agentscript.Program
}

// ScriptToolMetadata is the read-only management projection used by the UI.
type ScriptToolMetadata struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Agents      []string        `json:"agents"`
	Enabled     bool            `json:"enabled"`
	Resource    string          `json:"resource"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func (h Harness) ScriptToolMetadata() []ScriptToolMetadata {
	result := make([]ScriptToolMetadata, len(h.scriptTools))
	for index, tool := range h.scriptTools {
		schema, _ := json.Marshal(tool.inputSchema)
		result[index] = ScriptToolMetadata{
			Name: tool.name, Description: tool.description, Agents: append([]string(nil), tool.agents...),
			Enabled: tool.enabled, Resource: tool.resource, InputSchema: schema,
		}
	}
	return result
}

// ScriptToolDefinitions materializes target-matched saved scripts as ordinary
// ToolDefinitions in stable name order.
func (h Harness) ScriptToolDefinitions(
	agentKind string,
	config publictools.ScriptConfig,
) ([]agent.ToolDefinition, error) {
	definitions := make([]agent.ToolDefinition, 0, len(h.scriptTools))
	for _, tool := range h.scriptTools {
		if !tool.enabled || !contains(tool.agents, agentKind) {
			continue
		}
		definition, err := publictools.SavedScriptTool(config, publictools.SavedScriptToolSpec{
			Name: tool.name, Description: tool.description,
			InputSchema: tool.inputSchema, Program: tool.program,
		})
		if err != nil {
			return nil, fmt.Errorf("materialize Script Tool %q: %w", tool.name, err)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

type ContextFragment struct {
	ID        string
	Purpose   string
	Agents    []string
	Placement agent.ContextPlacement
	Content   string
	Resource  string
}

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

// Inspection exposes raw immutable files together with every schema
// diagnostic. Management surfaces use it to repair an invalid current State
// without weakening Current, which remains fail-closed for Agent assembly.
type Inspection struct {
	Snapshot    agentstate.Snapshot
	Harness     Harness
	Diagnostics []agentstate.Diagnostic
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
	_, harness, err := m.readValidated(ctx)
	return harness, err
}

// Inspect returns the current State with every validation diagnostic. Invalid
// content remains visible so users and Agents can repair it atomically.
func (m *Manager) Inspect(ctx context.Context) (Inspection, error) {
	if m == nil || m.store == nil {
		return Inspection{}, fmt.Errorf("Harness State manager is unavailable")
	}
	snapshot, err := m.store.Current(ctx)
	if err != nil {
		return Inspection{}, err
	}
	harness, diagnostics := parseAll(ctx, snapshot, m.config())
	return Inspection{
		Snapshot: snapshot, Harness: harness,
		Diagnostics: append([]agentstate.Diagnostic(nil), diagnostics...),
	}, nil
}

// ValidatedSnapshot returns the exact live files after full schema and budget
// validation. Management callers use it when a history record must match the
// same validated contents.
func (m *Manager) ValidatedSnapshot(ctx context.Context) (agentstate.Snapshot, error) {
	snapshot, _, err := m.readValidated(ctx)
	return snapshot, err
}

func (m *Manager) readValidated(ctx context.Context) (agentstate.Snapshot, Harness, error) {
	if m == nil || m.store == nil {
		return agentstate.Snapshot{}, Harness{}, fmt.Errorf("Harness State manager is unavailable")
	}
	snapshot, err := m.store.Current(ctx)
	if err != nil {
		return agentstate.Snapshot{}, Harness{}, err
	}
	harness, err := parse(ctx, snapshot, m.config())
	if err != nil {
		return agentstate.Snapshot{}, Harness{}, err
	}
	return snapshot, harness, nil
}

// Validate checks the complete live directory. It is used by management tools
// after direct file edits and does not mutate files or record Git history.
func (m *Manager) Validate(ctx context.Context) error {
	_, err := m.Current(ctx)
	return err
}

// Load opens the live user State directory and applies it only when the whole
// snapshot is valid. Invalid user State is rejected as one contribution so a
// malformed edit cannot prevent the base Agent from being built.
func Load(ctx context.Context, cfg *config.Config) (Harness, error) {
	if cfg == nil {
		return Harness{}, fmt.Errorf("load Harness State: config is nil")
	}
	if !cfg.Labs.DeveloperMode {
		return emptyHarness(), nil
	}
	if strings.TrimSpace(cfg.DenovaDir) == "" && strings.TrimSpace(cfg.NovaDir) == "" {
		return emptyHarness(), nil
	}
	manager, err := Open(cfg)
	if err != nil {
		return Harness{}, err
	}
	inspection, err := manager.Inspect(ctx)
	if err != nil {
		return Harness{}, err
	}
	if len(inspection.Diagnostics) != 0 {
		slog.WarnContext(ctx, "[harness-state] rejecting invalid user Harness contribution",
			"revision", inspection.Snapshot.Revision, "diagnostics", len(inspection.Diagnostics))
		return emptyHarness(), nil
	}
	return inspection.Harness, nil
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

func parse(ctx context.Context, snapshot agentstate.Snapshot, cfg *config.Config) (Harness, error) {
	harness, diagnostics := parseAll(ctx, snapshot, cfg)
	if len(diagnostics) != 0 {
		return Harness{}, &agentstate.ValidationError{Diagnostics: diagnostics}
	}
	return harness, nil
}

func emptyHarness() Harness {
	return Harness{
		prompts: map[string]string{}, toolDescriptions: map[string]string{},
	}
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
