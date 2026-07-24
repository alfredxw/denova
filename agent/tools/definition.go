// Package tools provides complete, provider-neutral tool definitions and a
// reusable workspace toolset. A Definition keeps execution and recovery
// semantics beside the concrete Tool; callers do not maintain a second name
// catalog.
package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const descriptorExtraKey = "github.com/alfredxw/denova/agent/tools.descriptor.v1"

// Source describes where a tool obtains or changes state.
type Source string

const (
	SourceOther Source = "other"
	SourceRead  Source = "read"
	SourceWrite Source = "write"
	SourceShell Source = "shell"
)

// ExecutionClass declares which coordination boundary a caller should hold.
type ExecutionClass string

const (
	ExecutionParallelRead       ExecutionClass = "parallel_read"
	ExecutionWorkspaceExclusive ExecutionClass = "workspace_exclusive"
	ExecutionChild              ExecutionClass = "child"
)

// RecoveryClass declares what is safe after an observed start without a
// matching completion.
type RecoveryClass string

const (
	RecoveryReadOnly      RecoveryClass = "read_only"
	RecoveryIdempotent    RecoveryClass = "idempotent"
	RecoveryReconcilable  RecoveryClass = "reconcilable"
	RecoveryNonIdempotent RecoveryClass = "non_idempotent"
)

// ResultProjection describes how a result may enter the next model context.
type ResultProjection string

const ResultBoundedModelContext ResultProjection = "bounded_model_context"

// Descriptor is the runtime contract attached to one model-visible Tool.
// Name intentionally does not appear here: Tool.Info is its only source.
type Descriptor struct {
	Source            Source           `json:"source"`
	Capability        string           `json:"capability,omitempty"`
	Execution         ExecutionClass   `json:"execution"`
	Recovery          RecoveryClass    `json:"recovery"`
	ResultProjection  ResultProjection `json:"result_projection"`
	MutatesWorkspace  bool             `json:"mutates_workspace"`
	MaxResultBytes    int              `json:"max_result_bytes"`
	RequiresPostCheck bool             `json:"requires_post_check"`
}

// Definition is the complete registration unit for one Tool.
type Definition struct {
	Tool       agent.BaseTool
	Descriptor Descriptor
}

// Validate checks the concrete tool schema and its runtime contract.
func (definition Definition) Validate(ctx context.Context) error {
	if definition.Tool == nil {
		return errors.New("tool definition has nil Tool")
	}
	info, err := definition.Tool.Info(ctx)
	if err != nil {
		return fmt.Errorf("read tool info: %w", err)
	}
	if info == nil || strings.TrimSpace(info.Name) == "" {
		return errors.New("tool definition has no stable name")
	}
	if info.Name != strings.TrimSpace(info.Name) {
		return fmt.Errorf("tool %q has leading or trailing whitespace", info.Name)
	}
	if definition.Descriptor.Execution == "" {
		return fmt.Errorf("tool %q has no execution class", info.Name)
	}
	if definition.Descriptor.Recovery == "" {
		return fmt.Errorf("tool %q has no recovery class", info.Name)
	}
	if definition.Descriptor.ResultProjection == "" {
		return fmt.Errorf("tool %q has no result projection", info.Name)
	}
	if definition.Descriptor.MaxResultBytes <= 0 {
		return fmt.Errorf("tool %q has no positive result limit", info.Name)
	}
	return nil
}

// ToolInfo returns the model schema with its descriptor attached as opaque
// metadata. Providers still see the same name, description, and JSON Schema.
func (definition Definition) ToolInfo(ctx context.Context) (*agent.ToolInfo, error) {
	if err := definition.Validate(ctx); err != nil {
		return nil, err
	}
	info, err := definition.Tool.Info(ctx)
	if err != nil {
		return nil, err
	}
	clone := *info
	clone.Extra = cloneExtra(info.Extra)
	clone.Extra[descriptorExtraKey] = definition.Descriptor
	return &clone, nil
}

// DescriptorFromInfo returns the contract carried by a registered tool.
func DescriptorFromInfo(info *agent.ToolInfo) (Descriptor, bool) {
	if info == nil || info.Extra == nil {
		return Descriptor{}, false
	}
	value, ok := info.Extra[descriptorExtraKey]
	if !ok {
		return Descriptor{}, false
	}
	switch descriptor := value.(type) {
	case Descriptor:
		return descriptor, true
	case *Descriptor:
		if descriptor != nil {
			return *descriptor, true
		}
	case map[string]any:
		return descriptorFromMap(descriptor)
	}
	return Descriptor{}, false
}

func descriptorFromMap(value map[string]any) (Descriptor, bool) {
	result := Descriptor{
		Source:            Source(stringValue(value["source"])),
		Capability:        stringValue(value["capability"]),
		Execution:         ExecutionClass(stringValue(value["execution"])),
		Recovery:          RecoveryClass(stringValue(value["recovery"])),
		ResultProjection:  ResultProjection(stringValue(value["result_projection"])),
		MutatesWorkspace:  boolValue(value["mutates_workspace"]),
		MaxResultBytes:    intValue(value["max_result_bytes"]),
		RequiresPostCheck: boolValue(value["requires_post_check"]),
	}
	return result, result.Execution != "" && result.Recovery != ""
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func intValue(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case float64:
		return int(number)
	default:
		return 0
	}
}

func cloneExtra(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

// describedTool replaces Info while preserving the concrete invocation
// capabilities of its underlying tool. Separate wrappers avoid accidentally
// making an invokable tool streamable (or the reverse).
type describedTool struct {
	definition Definition
}

func (tool describedTool) toolDefinition() Definition { return tool.definition }

func (tool describedTool) Info(ctx context.Context) (*agent.ToolInfo, error) {
	return tool.definition.ToolInfo(ctx)
}

type describedInvokableTool struct{ describedTool }

func (tool describedInvokableTool) InvokableRun(ctx context.Context, arguments string, options ...agent.ToolOption) (string, error) {
	return tool.definition.Tool.(agent.InvokableTool).InvokableRun(ctx, arguments, options...)
}

type describedStreamableTool struct{ describedTool }

func (tool describedStreamableTool) StreamableRun(ctx context.Context, arguments string, options ...agent.ToolOption) (*agent.StreamReader[string], error) {
	return tool.definition.Tool.(agent.StreamableTool).StreamableRun(ctx, arguments, options...)
}

type describedHybridTool struct{ describedTool }

func (tool describedHybridTool) InvokableRun(ctx context.Context, arguments string, options ...agent.ToolOption) (string, error) {
	return tool.definition.Tool.(agent.InvokableTool).InvokableRun(ctx, arguments, options...)
}

func (tool describedHybridTool) StreamableRun(ctx context.Context, arguments string, options ...agent.ToolOption) (*agent.StreamReader[string], error) {
	return tool.definition.Tool.(agent.StreamableTool).StreamableRun(ctx, arguments, options...)
}

func described(definition Definition) agent.BaseTool {
	base := describedTool{definition: definition}
	_, invokable := definition.Tool.(agent.InvokableTool)
	_, streamable := definition.Tool.(agent.StreamableTool)
	switch {
	case invokable && streamable:
		return describedHybridTool{describedTool: base}
	case invokable:
		return describedInvokableTool{describedTool: base}
	case streamable:
		return describedStreamableTool{describedTool: base}
	default:
		return base
	}
}

type definitionCarrier interface {
	toolDefinition() Definition
}

// Bind attaches a Definition to its concrete Tool while preserving whether it
// is invokable, streamable, or both. It is useful at legacy composition seams
// that still accept BaseTool.
func Bind(definition Definition) agent.BaseTool {
	return described(definition)
}

// FromTool recovers the complete Definition carried by a bound tool. Raw tools
// are rejected so assembly cannot silently invent execution policy by name.
func FromTool(ctx context.Context, tool agent.BaseTool) (Definition, error) {
	if carrier, ok := tool.(definitionCarrier); ok {
		definition := carrier.toolDefinition()
		if err := definition.Validate(ctx); err != nil {
			return Definition{}, err
		}
		return definition, nil
	}
	if tool == nil {
		return Definition{}, errors.New("tool is nil")
	}
	info, err := tool.Info(ctx)
	if err != nil {
		return Definition{}, err
	}
	descriptor, ok := DescriptorFromInfo(info)
	if !ok {
		return Definition{}, fmt.Errorf("tool %q has no attached Descriptor", info.Name)
	}
	return Definition{Tool: tool, Descriptor: descriptor}, nil
}
