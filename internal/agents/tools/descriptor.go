package tools

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
)

const defaultToolResultMaxBytes = config.DefaultAgentToolResultLimitKB * 1024

const (
	ToolSourceLore    = agenttools.Source("lore")
	ToolSourceHistory = agenttools.Source("history")
	ToolSourceWeb     = agenttools.Source("web")
	ToolSourceImage   = agenttools.Source("image")
)

// validateToolDescriptors makes the descriptor catalog part of Agent
// construction, so a newly registered tool cannot silently inherit unknown
// recovery behavior. A final runtime guard applies the same check after every
// BeforeAgent middleware has injected its concrete tools.
func validateToolDescriptors(ctx context.Context, tools []agent.BaseTool) error {
	_, err := validatedConcreteToolNames(ctx, tools)
	return err
}

// Validate checks that every model-visible tool has a unique stable name and
// an explicit execution/recovery descriptor.
func Validate(ctx context.Context, concrete []agent.BaseTool) error {
	return validateToolDescriptors(ctx, concrete)
}

func validatedConcreteToolNames(ctx context.Context, tools []agent.BaseTool) ([]string, error) {
	seen := make(map[string]int, len(tools))
	names := make([]string, 0, len(tools))
	for index, candidate := range tools {
		if candidate == nil {
			continue
		}
		info, err := candidate.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("read tool descriptor at index %d: %w", index, err)
		}
		if info == nil || strings.TrimSpace(info.Name) == "" {
			return nil, fmt.Errorf("tool at index %d has no stable name", index)
		}
		name := normalizeToolName(info.Name)
		if previous, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate model-visible tool %q at indexes %d and %d", name, previous, index)
		}
		seen[name] = index
		_, declared := agenttools.DescriptorFromInfo(info)
		if !declared {
			return nil, fmt.Errorf("tool %q has no explicit ToolDescriptor", info.Name)
		}
		names = append(names, name)
	}
	return names, nil
}

// validateToolSurface keeps tool names a one-to-one mapping to an endpoint and
// recovery contract before Agent construction.
func validateToolSurface(ctx context.Context, tools []agent.BaseTool) error {
	return validateToolDescriptors(ctx, tools)
}

func bindToolDefinition(definition agenttools.Definition) (agent.BaseTool, error) {
	ctx := context.Background()
	if err := definition.Validate(ctx); err != nil {
		return nil, err
	}
	return agenttools.Bind(definition), nil
}

// Bind validates and binds one ADK tool definition.
func Bind(definition agenttools.Definition) (agent.BaseTool, error) {
	return bindToolDefinition(definition)
}

func defineTool(tool agent.BaseTool, descriptor agenttools.Descriptor) (agent.BaseTool, error) {
	return bindToolDefinition(agenttools.Definition{Tool: tool, Descriptor: descriptor})
}

// Define attaches a descriptor to a concrete tool after validating it.
func Define(tool agent.BaseTool, descriptor agenttools.Descriptor) (agent.BaseTool, error) {
	return defineTool(tool, descriptor)
}

func boundedReadDescriptor(source agenttools.Source, capability string) agenttools.Descriptor {
	return agenttools.Descriptor{
		Source: source, Capability: capability,
		Execution:        agenttools.ExecutionParallelRead,
		Recovery:         agenttools.RecoveryReadOnly,
		ResultProjection: agenttools.ResultBoundedModelContext,
		MaxResultBytes:   defaultToolResultMaxBytes,
	}
}

// BoundedReadDescriptor declares a parallel, read-only tool whose result may
// enter bounded model context.
func BoundedReadDescriptor(source agenttools.Source, capability string) agenttools.Descriptor {
	return boundedReadDescriptor(source, capability)
}

func workspaceWriteDescriptor(source agenttools.Source, capability string, recovery agenttools.RecoveryClass) agenttools.Descriptor {
	return agenttools.Descriptor{
		Source: source, Capability: capability,
		Execution:        agenttools.ExecutionWorkspaceExclusive,
		Recovery:         recovery,
		ResultProjection: agenttools.ResultBoundedModelContext,
		MutatesWorkspace: true, RequiresPostCheck: true,
		MaxResultBytes: defaultToolResultMaxBytes,
	}
}

// WorkspaceWriteDescriptor declares an exclusive workspace mutation that
// requires post-run verification.
func WorkspaceWriteDescriptor(source agenttools.Source, capability string, recovery agenttools.RecoveryClass) agenttools.Descriptor {
	return workspaceWriteDescriptor(source, capability, recovery)
}

// toolDescriptorGuardMiddleware is deliberately the final BeforeAgent handler.
// Any host middleware that appends tools dynamically must cross the same
// fail-closed catalog boundary before the first provider request is created.
type toolDescriptorGuardMiddleware struct {
	*agent.BaseMiddleware
}

func newToolDescriptorGuardMiddleware() *toolDescriptorGuardMiddleware {
	return &toolDescriptorGuardMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
}

// NewDescriptorGuardMiddleware validates the final dynamic tool surface just
// before a model run starts.
func NewDescriptorGuardMiddleware() agent.Middleware {
	return newToolDescriptorGuardMiddleware()
}

func (m *toolDescriptorGuardMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *agent.RunContext,
) (context.Context, *agent.RunContext, error) {
	if runCtx == nil {
		return ctx, runCtx, fmt.Errorf("validate dynamic tool descriptors: agent context is nil")
	}
	if err := validateToolDescriptors(ctx, runCtx.Tools); err != nil {
		return ctx, runCtx, fmt.Errorf("validate dynamic tool descriptors: %w", err)
	}
	return ctx, runCtx, nil
}

func normalizeToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
