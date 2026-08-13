package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agenttool "denova/internal/agents/tool"
)

const defaultToolResultMaxBytes = config.DefaultAgentToolResultLimitKB * 1024

const (
	ToolSourceLore    = agenttool.ToolSourceLore
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

// ValidateAgainstManifest proves that every product-owned capability assembled
// for an Agent matches the generated settings catalog. Application extensions
// with unknown capability names remain outside this product policy seam.
func ValidateAgainstManifest(ctx context.Context, concrete []agent.ToolDefinition, manifest []config.ResolvedAgentToolCapability) error {
	if err := Validate(ctx, concrete); err != nil {
		return err
	}
	known := make(map[string]config.AgentToolCapabilityCatalogEntry)
	for _, entry := range config.AgentToolCapabilityCatalogForGOOS("") {
		known[entry.Capability] = entry
	}
	resolved := make(map[string]config.ResolvedAgentToolCapability, len(manifest))
	for _, entry := range manifest {
		resolved[entry.Capability] = entry
	}
	for index, definition := range concrete {
		capability := strings.TrimSpace(definition.Descriptor.Capability)
		if _, productOwned := known[capability]; !productOwned {
			continue
		}
		entry, present := resolved[capability]
		if !present {
			return fmt.Errorf("tool capability %q at index %d is outside the selected Agent manifest", capability, index)
		}
		if !entry.Allowed || entry.Availability == config.AgentToolAvailabilityUnavailable {
			return fmt.Errorf("tool capability %q at index %d was assembled while disabled by the selected Agent manifest", capability, index)
		}
		info, err := definition.Tool.Info(ctx)
		if err != nil {
			return fmt.Errorf("read tool capability %q at index %d: %w", capability, index, err)
		}
		if info == nil || !containsToolName(entry.ToolNames, info.Name) {
			return fmt.Errorf("tool capability %q assembled undeclared tool name %q; declared names are %v", capability, toolInfoName(info), entry.ToolNames)
		}
		expected, declared := descriptorSummaryForTool(entry.ToolDescriptors, info.Name)
		if !declared {
			return fmt.Errorf("tool capability %q has no generated descriptor for declared tool %q", capability, info.Name)
		}
		summary, err := config.SummarizeAgentToolDescriptor(definition.Descriptor)
		if err != nil {
			return fmt.Errorf("summarize tool %q capability %q: %w", info.Name, capability, err)
		}
		if summary != expected {
			return fmt.Errorf("tool %q capability %q descriptor drift: runtime=%+v catalog=%+v", info.Name, capability, summary, expected)
		}
	}
	return nil
}

// ManifestValidator returns an identified validation seam suitable for a
// late-bound Toolset such as delegation. Its policy identity participates in
// the owning Toolset's recovery fingerprint.
func ManifestValidator(manifest []config.ResolvedAgentToolCapability) (agent.CapabilityIdentity, func(context.Context, []agent.ToolDefinition) error, error) {
	cloned := append([]config.ResolvedAgentToolCapability(nil), manifest...)
	for index := range cloned {
		cloned[index].ToolNames = append([]string(nil), cloned[index].ToolNames...)
		cloned[index].ToolDescriptors = cloneDescriptorSummaries(cloned[index].ToolDescriptors)
	}
	encoded, err := json.Marshal(manifestValidatorIdentityProjection(cloned))
	if err != nil {
		return agent.CapabilityIdentity{}, nil, fmt.Errorf("encode tool manifest validator identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	identity := agent.CapabilityIdentity{
		Kind: "denova.tools.manifest_validator", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
	}
	return identity, func(ctx context.Context, definitions []agent.ToolDefinition) error {
		return ValidateAgainstManifest(ctx, definitions, cloned)
	}, nil
}

// manifestValidatorIdentityProjection excludes catalog copy and presentation
// fields that never affect model context or execution recovery. The validator
// still checks presentation at materialization time, but display-only changes
// must not rotate the owning Toolset's durable identity or cache prefix.
type manifestValidatorIdentityEntry struct {
	Capability      string                                 `json:"capability"`
	ToolNames       []string                               `json:"tool_names"`
	Allowed         bool                                   `json:"allowed"`
	Availability    config.AgentToolAvailability           `json:"availability"`
	ToolDescriptors map[string]manifestValidatorDescriptor `json:"tool_descriptors"`
}

type manifestValidatorDescriptor struct {
	Source             agent.ToolSource              `json:"source"`
	Execution          agent.ToolExecutionClass      `json:"execution"`
	MutationScope      agent.ToolMutationScope       `json:"mutation_scope"`
	PostCheck          agent.ToolPostCheckPolicy     `json:"post_check"`
	Recovery           agent.ToolRecoveryClass       `json:"recovery"`
	ResultRecoveryKind agent.ToolResultRecoveryKind  `json:"result_recovery_kind,omitempty"`
	ResultProjection   agent.ToolResultProjection    `json:"result_projection"`
	ResultRetention    agent.ToolResultRetentionMode `json:"result_retention"`
	Steering           agent.SteeringPolicy          `json:"steering"`
	MaxResultBytes     int                           `json:"max_result_bytes"`
}

func manifestValidatorIdentityProjection(manifest []config.ResolvedAgentToolCapability) []manifestValidatorIdentityEntry {
	projected := make([]manifestValidatorIdentityEntry, len(manifest))
	for index, entry := range manifest {
		projected[index] = manifestValidatorIdentityEntry{
			Capability: entry.Capability, ToolNames: append([]string(nil), entry.ToolNames...),
			Allowed: entry.Allowed, Availability: entry.Availability,
			ToolDescriptors: manifestValidatorDescriptors(entry.ToolDescriptors),
		}
	}
	return projected
}

func manifestValidatorDescriptors(summaries map[string]config.AgentToolDescriptorSummary) map[string]manifestValidatorDescriptor {
	result := make(map[string]manifestValidatorDescriptor, len(summaries))
	for name, summary := range summaries {
		result[name] = manifestValidatorDescriptor{
			Source: summary.Source, Execution: summary.Execution, MutationScope: summary.MutationScope,
			PostCheck: summary.PostCheck, Recovery: summary.Recovery, ResultRecoveryKind: summary.ResultRecoveryKind,
			ResultProjection: summary.ResultProjection, ResultRetention: summary.ResultRetention,
			Steering: summary.Steering, MaxResultBytes: summary.MaxResultBytes,
		}
	}
	return result
}

func descriptorSummaryForTool(summaries map[string]config.AgentToolDescriptorSummary, name string) (config.AgentToolDescriptorSummary, bool) {
	normalized := normalizeToolName(name)
	for candidate, summary := range summaries {
		if normalizeToolName(candidate) == normalized {
			return summary, true
		}
	}
	return config.AgentToolDescriptorSummary{}, false
}

func cloneDescriptorSummaries(summaries map[string]config.AgentToolDescriptorSummary) map[string]config.AgentToolDescriptorSummary {
	result := make(map[string]config.AgentToolDescriptorSummary, len(summaries))
	for name, summary := range summaries {
		result[name] = summary
	}
	return result
}

func containsToolName(names []string, candidate string) bool {
	candidate = normalizeToolName(candidate)
	for _, name := range names {
		if normalizeToolName(name) == candidate {
			return true
		}
	}
	return false
}

func toolInfoName(info *agent.ToolInfo) string {
	if info == nil {
		return ""
	}
	return info.Name
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
	descriptor := agent.ToolDescriptor{
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
	switch source {
	case ToolSourceWeb:
		descriptor.Presentation = agent.UniformToolPresentation(agent.ToolPresentationWeb)
	default:
		descriptor.Presentation = agent.UniformToolPresentation(agent.ToolPresentationGeneric)
	}
	return descriptor
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
	descriptor := agent.ToolDescriptor{
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
	if source == ToolSourceImage {
		descriptor.Presentation = agent.UniformToolPresentation(agent.ToolPresentationImage)
	} else {
		descriptor.Presentation = agent.UniformToolPresentation(agent.ToolPresentationFile)
	}
	return descriptor
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
