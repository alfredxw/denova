package tools

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

const defaultToolResultMaxBytes = config.DefaultAgentToolResultLimitKB * 1024

const (
	ToolSourceLore    = agent.ToolSourceLore
	ToolSourceHistory = agent.ToolSourceHistory
	ToolSourceWeb     = agent.ToolSourceWeb
	ToolSourceImage   = agent.ToolSourceImage
)

// validateToolDescriptors makes the descriptor catalog part of Agent
// construction, so a newly registered tool cannot silently inherit unknown
// recovery behavior.
func validateToolDescriptors(ctx context.Context, tools []agent.ToolDefinition) error {
	_, err := agent.NewRegistry(ctx, tools...)
	return err
}

// Validate checks that every model-visible tool has a unique stable name and
// an explicit execution/recovery descriptor.
func Validate(ctx context.Context, concrete []agent.ToolDefinition) error {
	return validateToolDescriptors(ctx, concrete)
}

func validatedConcreteToolNames(ctx context.Context, tools []agent.ToolDefinition) ([]string, error) {
	seen := make(map[string]int, len(tools))
	names := make([]string, 0, len(tools))
	for index, candidate := range tools {
		if candidate.Tool == nil {
			return nil, fmt.Errorf("tool at index %d is nil", index)
		}
		info, err := candidate.Tool.Info(ctx)
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
		if err := candidate.Descriptor.Validate(); err != nil {
			return nil, fmt.Errorf("tool %q has invalid ToolDescriptor: %w", info.Name, err)
		}
		names = append(names, name)
	}
	return names, nil
}

// validateToolSurface keeps tool names a one-to-one mapping to an endpoint and
// recovery contract before Agent construction.
func validateToolSurface(ctx context.Context, tools []agent.ToolDefinition) error {
	return validateToolDescriptors(ctx, tools)
}

func defineTool(tool agent.Tool, descriptor agent.ToolDescriptor) (agent.ToolDefinition, error) {
	definition := agent.ToolDefinition{Tool: tool, Descriptor: descriptor}
	if err := definition.Validate(context.Background()); err != nil {
		return agent.ToolDefinition{}, err
	}
	return definition, nil
}

// Define attaches a descriptor to a concrete tool after validating it.
func Define(tool agent.Tool, descriptor agent.ToolDescriptor) (agent.ToolDefinition, error) {
	return defineTool(tool, descriptor)
}

func boundedReadDescriptor(source agent.ToolSource, capability string, recoveryKinds ...agent.ToolResultRecoveryKind) agent.ToolDescriptor {
	var resultRecovery agent.ToolResultRecoveryKind
	if len(recoveryKinds) > 0 {
		resultRecovery = recoveryKinds[0]
	}
	return agent.ToolDescriptor{
		Source: source, Capability: capability,
		Execution:          agent.ToolExecutionParallelRead,
		MutationScope:      agent.ToolMutationNone,
		PostCheck:          agent.ToolPostCheckNone,
		Recovery:           agent.ToolRecoveryReadOnly,
		ResultRecoveryKind: resultRecovery,
		ResultProjection:   agent.ToolResultBoundedModelContext,
		ResultRetention:    agent.ToolResultDeferred,
		Steering:           agent.SteeringFinishCurrent,
		MaxResultBytes:     defaultToolResultMaxBytes,
	}
}

// BoundedReadDescriptor declares a parallel, read-only tool whose result may
// enter bounded model context.
func BoundedReadDescriptor(source agent.ToolSource, capability string) agent.ToolDescriptor {
	return boundedReadDescriptor(source, capability)
}

// BoundedRecoverableReadDescriptor additionally declares the exact ordinary
// operation that can reconstruct a pressure-cleaned result.
func BoundedRecoverableReadDescriptor(source agent.ToolSource, capability string, recoveryKind agent.ToolResultRecoveryKind) agent.ToolDescriptor {
	return boundedReadDescriptor(source, capability, recoveryKind)
}

func workspaceWriteDescriptor(source agent.ToolSource, capability string, recovery agent.ToolRecoveryClass) agent.ToolDescriptor {
	return agent.ToolDescriptor{
		Source: source, Capability: capability,
		Execution:        agent.ToolExecutionWorkspaceExclusive,
		MutationScope:    agent.ToolMutationWorkspace,
		PostCheck:        agent.ToolPostCheckWorkspaceChange,
		Recovery:         recovery,
		ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention:  agent.ToolResultProtected,
		Steering:         agent.SteeringFinishCurrent,
		MaxResultBytes:   defaultToolResultMaxBytes,
	}
}

// interactiveStoryWorkflowDescriptor classifies the game-owned turn and state
// transaction separately from generic workspace file mutation. These tools may
// persist the current story session through their domain commit boundary, but
// they do not grant arbitrary workspace write access.
func interactiveStoryWorkflowDescriptor() agent.ToolDescriptor {
	return agent.ToolDescriptor{
		Source:           ToolSourceHistory,
		Execution:        agent.ToolExecutionSessionExclusive,
		MutationScope:    agent.ToolMutationSession,
		PostCheck:        agent.ToolPostCheckSessionState,
		Recovery:         agent.ToolRecoveryReconcilable,
		ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention:  agent.ToolResultProtected,
		Steering:         agent.SteeringFinishCurrent,
		MaxResultBytes:   defaultToolResultMaxBytes,
	}
}

// WorkspaceWriteDescriptor declares an exclusive workspace mutation that
// requires post-run verification.
func WorkspaceWriteDescriptor(source agent.ToolSource, capability string, recovery agent.ToolRecoveryClass) agent.ToolDescriptor {
	return workspaceWriteDescriptor(source, capability, recovery)
}

func normalizeToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
