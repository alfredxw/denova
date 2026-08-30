// Package delegation carries Denova's immutable child-Agent catalog between
// the builder and execution adapter. The builder owns child composition; the
// execution adapter supplies the durable Agent opener for the parent Session.
package delegation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"
)

// Child is one fully composed delegated Agent. Identity describes the exact
// selector semantics and participates in the root Definition behavior key.
type Child struct {
	Name        string
	Description string
	Definition  agent.Definition
	Identity    agent.CapabilityIdentity
}

// Config contains the Denova policy projection for the common delegation tools.
// These values affect the model-visible tool contract and are fingerprinted.
type Config struct {
	Capability         string
	Description        string
	MaxResultBytes     int
	Parallelism        int
	ValidationIdentity agent.CapabilityIdentity
	Validate           func(context.Context, []agent.ToolDefinition) error `json:"-"`
}

// Catalog is deliberately unusable until an execution owner binds a durable
// TaskExecutor. This prevents a Definition from silently advertising
// delegation without a public Session/Run owner.
type Catalog struct {
	base     agent.Toolset
	children []Child
	config   Config
	identity agent.CapabilityIdentity
}

func NewCatalog(base agent.Toolset, config Config, children ...Child) (*Catalog, error) {
	if base == nil {
		var err error
		base, err = agent.StaticTools()
		if err != nil {
			return nil, fmt.Errorf("construct empty delegation base Toolset: %w", err)
		}
	}
	config.Capability = strings.TrimSpace(config.Capability)
	config.Description = strings.TrimSpace(config.Description)
	if config.Capability == "" || config.MaxResultBytes <= 0 || config.Parallelism <= 0 || config.Validate == nil ||
		config.ValidationIdentity.Kind == "" || config.ValidationIdentity.Version == 0 {
		return nil, errors.New("delegation Catalog requires capability, positive result limit, positive parallelism, and identified manifest validation")
	}
	resolved := make([]Child, len(children))
	seen := make(map[string]struct{}, len(children))
	for index, child := range children {
		child.Name = strings.TrimSpace(child.Name)
		child.Description = strings.TrimSpace(child.Description)
		if child.Name == "" || child.Definition.Model == nil || child.Identity.Kind == "" || child.Identity.Version == 0 {
			return nil, fmt.Errorf("delegation child %d is incomplete", index)
		}
		if _, duplicate := seen[child.Name]; duplicate {
			return nil, fmt.Errorf("delegation child %q is duplicated", child.Name)
		}
		seen[child.Name] = struct{}{}
		resolved[index] = child
	}
	encoded, _ := json.Marshal(struct {
		Base               agent.CapabilityIdentity
		Capability         string
		Description        string
		MaxResultBytes     int
		Parallelism        int
		ValidationIdentity agent.CapabilityIdentity
		Children           []struct {
			Name, Description string
			Identity          agent.CapabilityIdentity
		}
	}{
		Base: base.Identity(), Capability: config.Capability, Description: config.Description,
		MaxResultBytes: config.MaxResultBytes, Parallelism: config.Parallelism, ValidationIdentity: config.ValidationIdentity,
		Children: childIdentities(resolved),
	})
	digest := sha256.Sum256(encoded)
	return &Catalog{
		base: base, children: resolved, config: config,
		identity: agent.CapabilityIdentity{
			Kind: "denova.tools.tasks", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
		},
	}, nil
}

func childIdentities(children []Child) []struct {
	Name, Description string
	Identity          agent.CapabilityIdentity
} {
	result := make([]struct {
		Name, Description string
		Identity          agent.CapabilityIdentity
	}, len(children))
	for index, child := range children {
		result[index] = struct {
			Name, Description string
			Identity          agent.CapabilityIdentity
		}{child.Name, child.Description, child.Identity}
	}
	return result
}

func (catalog *Catalog) Identity() agent.CapabilityIdentity {
	if catalog == nil {
		return agent.CapabilityIdentity{}
	}
	return catalog.identity
}

func (catalog *Catalog) PrepareTools(context.Context, agent.ToolRequest) ([]agent.ToolDefinition, error) {
	return nil, errors.New("Denova delegation Catalog is not bound to a durable Agent owner")
}

func (catalog *Catalog) Children() []Child {
	if catalog == nil {
		return nil
	}
	return append([]Child(nil), catalog.children...)
}

// Parallelism returns the maximum active child Runs for one parent Session.
func (catalog *Catalog) Parallelism() int {
	if catalog == nil {
		return 0
	}
	return catalog.config.Parallelism
}

// MaxResultBytes returns the shared bounded-context budget used for delegated
// tool results and asynchronous task-completion projections.
func (catalog *Catalog) MaxResultBytes() int {
	if catalog == nil {
		return 0
	}
	return catalog.config.MaxResultBytes
}

func (catalog *Catalog) Bind(executor publictools.TaskExecutor) (agent.Toolset, error) {
	if catalog == nil || executor == nil {
		return nil, errors.New("bind Denova delegation: Catalog and TaskExecutor are required")
	}
	tasks := publictools.Tasks(executor)
	return &boundCatalog{catalog: catalog, tasks: tasks}, nil
}

type boundCatalog struct {
	catalog *Catalog
	tasks   agent.Toolset
}

func (bound *boundCatalog) InitializeDefinition(ctx context.Context) error {
	if bound == nil || bound.tasks == nil {
		return errors.New("bound Denova delegation Catalog is incomplete")
	}
	if initializer, ok := bound.tasks.(agent.DefinitionInitializer); ok {
		return initializer.InitializeDefinition(ctx)
	}
	return nil
}

func (bound *boundCatalog) Identity() agent.CapabilityIdentity {
	if bound == nil || bound.catalog == nil {
		return agent.CapabilityIdentity{}
	}
	return bound.catalog.Identity()
}

func (bound *boundCatalog) PrepareTools(ctx context.Context, request agent.ToolRequest) ([]agent.ToolDefinition, error) {
	if bound == nil || bound.catalog == nil || bound.tasks == nil {
		return nil, errors.New("Denova delegation Catalog binding is incomplete")
	}
	base, err := bound.catalog.base.PrepareTools(ctx, request)
	if err != nil {
		return nil, err
	}
	tasks, err := bound.tasks.PrepareTools(ctx, request)
	if err != nil {
		return nil, err
	}
	for index := range tasks {
		if tasks[index].Tool == nil {
			return nil, fmt.Errorf("delegated Agent tool %d is nil", index)
		}
		tasks[index].Descriptor.Capability = bound.catalog.config.Capability
		tasks[index].Descriptor.MaxResultBytes = bound.catalog.config.MaxResultBytes
		info, infoErr := tasks[index].Tool.Info(ctx)
		if infoErr != nil {
			return nil, fmt.Errorf("inspect delegated Agent tool %d: %w", index, infoErr)
		}
		if bound.catalog.config.Description != "" && info != nil && info.Name == "task" {
			tasks[index].Tool = describedTaskTool{Tool: tasks[index].Tool, description: bound.catalog.config.Description}
		}
	}
	definitions := append(base, tasks...)
	if err := bound.catalog.config.Validate(ctx, definitions); err != nil {
		return nil, fmt.Errorf("validate delegated Agent tool manifest: %w", err)
	}
	return definitions, nil
}

type describedTaskTool struct {
	agent.Tool
	description string
}

func (tool describedTaskTool) Info(ctx context.Context) (*agent.ToolInfo, error) {
	info, err := tool.Tool.Info(ctx)
	if err != nil || info == nil {
		return info, err
	}
	cloned := *info
	cloned.Desc = tool.description
	return &cloned, nil
}

func AsCatalog(toolset agent.Toolset) (*Catalog, bool) {
	catalog, ok := toolset.(*Catalog)
	return catalog, ok && catalog != nil
}

// ChildDefinition selects one immutable child from a freshly rebuilt root
// Definition. It is the only product-facing extraction seam used by cold task
// recovery.
func ChildDefinition(definition agent.Definition, name string) (agent.Definition, error) {
	catalog, ok := AsCatalog(definition.Tools)
	if !ok {
		return agent.Definition{}, errors.New("Agent Definition has no delegated child catalog")
	}
	name = strings.TrimSpace(name)
	for _, child := range catalog.children {
		if child.Name == name {
			return child.Definition, nil
		}
	}
	return agent.Definition{}, fmt.Errorf("delegated Agent %q was not found", name)
}
