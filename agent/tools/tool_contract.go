package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

const (
	defaultReadLines         = 2000
	defaultDirectoryDepth    = 1
	defaultDirectoryItems    = 200
	defaultMaxDirectoryDepth = 64
	defaultResultBytes       = 128 * 1024
	defaultResultEntries     = 10_000
	maxConfiguredResultBytes = 64 * 1024 * 1024
	maxConfiguredEntries     = 100_000
	maxFilesystemScanEntries = 100_000
	maxFilesystemIgnoreBytes = 1024 * 1024
	maxFilesystemIgnoreRules = 10_000
	maxMutationFileBytes     = 16 * 1024 * 1024
	maxMutationEdits         = 256
	resultTruncatedMarker    = "[filesystem result truncated at the configured safety limit; use pagination or narrow the path or pattern]"
	processTruncatedMarker   = "[process output truncated; inspect a narrower command]"
)

// DefinitionOption customizes a general tool's runtime contract without
// changing its model-visible Interface.
type DefinitionOption func(*agent.ToolDescriptor)

// WithCapability associates a product-defined authorization capability.
func WithCapability(capability string) DefinitionOption {
	return func(descriptor *agent.ToolDescriptor) { descriptor.Capability = capability }
}

// WithMaxResultBytes overrides the model-context result ceiling.
func WithMaxResultBytes(limit int) DefinitionOption {
	return func(descriptor *agent.ToolDescriptor) { descriptor.MaxResultBytes = limit }
}

// WithPresentation selects the existing UI renderer for a composed tool.
func WithPresentation(presentation agent.ToolPresentationKind) DefinitionOption {
	return func(descriptor *agent.ToolDescriptor) {
		descriptor.Presentation = agent.UniformToolPresentation(presentation)
	}
}

// WithResultRecoveryKind declares the exact ordinary capability used to
// reconstruct a successful result after context-pressure cleanup.
func WithResultRecoveryKind(kind agent.ToolResultRecoveryKind) DefinitionOption {
	return func(descriptor *agent.ToolDescriptor) { descriptor.ResultRecoveryKind = kind }
}

func applyDefinitionOptions(descriptor agent.ToolDescriptor, options []DefinitionOption) agent.ToolDescriptor {
	for _, option := range options {
		if option != nil {
			option(&descriptor)
		}
	}
	return descriptor
}

func readDescriptor(options ...DefinitionOption) agent.ToolDescriptor {
	return applyDefinitionOptions(agent.ToolDescriptor{
		Source:             agent.ToolSourceRead,
		Execution:          agent.ToolExecutionParallelRead,
		MutationScope:      agent.ToolMutationNone,
		PostCheck:          agent.ToolPostCheckNone,
		Recovery:           agent.ToolRecoveryReadOnly,
		ResultRecoveryKind: agent.ToolResultRecoveryRead,
		ResultProjection:   agent.ToolResultBoundedModelContext,
		ResultRetention:    agent.ToolResultDeferred,
		Steering:           agent.SteeringFinishCurrent,
		MaxResultBytes:     defaultResultBytes,
	}, options)
}

func writeDescriptor(options ...DefinitionOption) agent.ToolDescriptor {
	return applyDefinitionOptions(agent.ToolDescriptor{
		Source:           agent.ToolSourceWrite,
		Execution:        agent.ToolExecutionWorkspaceExclusive,
		MutationScope:    agent.ToolMutationWorkspace,
		PostCheck:        agent.ToolPostCheckWorkspaceChange,
		Recovery:         agent.ToolRecoveryReconcilable,
		ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention:  agent.ToolResultProtected,
		Steering:         agent.SteeringFinishCurrent,
		MaxResultBytes:   defaultResultBytes,
		Presentation:     agent.UniformToolPresentation(agent.ToolPresentationFile),
	}, options)
}

func shellDescriptor(options ...DefinitionOption) agent.ToolDescriptor {
	return applyDefinitionOptions(agent.ToolDescriptor{
		Source:    agent.ToolSourceShell,
		Execution: agent.ToolExecutionWorkspaceExclusive,
		// A workspace cwd is not an OS sandbox: a command can change host state,
		// access the network, and spawn descendants outside the workspace. Keep
		// the workspace-exclusive execution lane for editor coordination, but do
		// not misrepresent the command's mutation boundary or recovery receipt.
		MutationScope:    agent.ToolMutationExternal,
		PostCheck:        agent.ToolPostCheckExternalReceipt,
		Recovery:         agent.ToolRecoveryNonIdempotent,
		ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention:  agent.ToolResultProtected,
		Steering:         agent.SteeringFinishCurrent,
		MaxResultBytes:   defaultResultBytes,
		Presentation:     agent.UniformToolPresentation(agent.ToolPresentationTerminal),
	}, options)
}

func normalizeAndDecode[T any](arguments string) (T, error) {
	var input T
	info, err := agent.GoStruct2ToolInfo[T]("arguments", "")
	if err != nil {
		return input, err
	}
	normalized, err := agent.NormalizeToolArguments(info, arguments)
	if err != nil {
		return input, err
	}
	if err := json.Unmarshal([]byte(normalized), &input); err != nil {
		return input, err
	}
	return input, nil
}

// JSONResult constructs a successful bounded structured Tool result.
func JSONResult(value any) (agent.ToolResult, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode tool result: %w", err)
	}
	return agent.TextToolResult(string(encoded)), nil
}

func lineNumbers(content string, start int) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var result strings.Builder
	for index, line := range lines {
		fmt.Fprintf(&result, "%d\t%s", start+index, line)
		if index < len(lines)-1 || strings.HasSuffix(content, "\n") {
			result.WriteByte('\n')
		}
	}
	return result.String()
}

func joinedResult(entries []string, empty string) string {
	if len(entries) == 0 {
		return empty
	}
	return boundedString(strings.Join(entries, "\n"), defaultResultBytes)
}

func boundedString(content string, limit int) string {
	if len(content) <= limit {
		return content
	}
	truncated, _ := truncateUTF8WithMarker(content, "\n"+resultTruncatedMarker, limit)
	return truncated
}

func truncateUTF8(content string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(content) <= limit {
		return content
	}
	end := limit
	for end > 0 && !utf8.RuneStart(content[end]) {
		end--
	}
	return content[:end]
}

func truncateUTF8WithMarker(content, marker string, limit int) (string, bool) {
	if len(content) <= limit {
		return content, false
	}
	if limit <= 0 {
		return "", true
	}
	if len(marker) >= limit {
		return truncateUTF8(marker, limit), true
	}
	prefixLimit := limit - len(marker)
	prefix := truncateUTF8(content, prefixLimit)
	return prefix + marker, true
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
