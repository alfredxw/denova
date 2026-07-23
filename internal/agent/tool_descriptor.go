package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/alfredxw/denova/adk"
)

// validateToolDescriptors makes the descriptor catalog part of Agent
// construction, so a newly registered tool cannot silently inherit unknown
// recovery behavior. A final runtime guard applies the same check after every
// BeforeAgent middleware has injected its concrete tools.
func validateToolDescriptors(ctx context.Context, tools []adk.BaseTool) error {
	_, err := validatedConcreteToolNames(ctx, tools)
	return err
}

func validatedConcreteToolNames(ctx context.Context, tools []adk.BaseTool) ([]string, error) {
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
		if _, declared := declaredToolDescriptor(info.Name); !declared {
			return nil, fmt.Errorf("tool %q has no explicit ToolDescriptor", info.Name)
		}
		names = append(names, name)
	}
	return names, nil
}

// validateToolSurface keeps tool names a one-to-one mapping to an endpoint and
// recovery contract before Agent construction.
func validateToolSurface(ctx context.Context, tools []adk.BaseTool) error {
	return validateToolDescriptors(ctx, tools)
}

// toolDescriptorGuardMiddleware is deliberately the final BeforeAgent handler.
// Any host middleware that appends tools dynamically must cross the same
// fail-closed catalog boundary before the first provider request is created.
type toolDescriptorGuardMiddleware struct {
	*adk.BaseMiddleware
}

func newToolDescriptorGuardMiddleware() *toolDescriptorGuardMiddleware {
	return &toolDescriptorGuardMiddleware{BaseMiddleware: &adk.BaseMiddleware{}}
}

func (m *toolDescriptorGuardMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.RunContext,
) (context.Context, *adk.RunContext, error) {
	if runCtx == nil {
		return ctx, runCtx, fmt.Errorf("validate dynamic tool descriptors: agent context is nil")
	}
	if err := validateToolDescriptors(ctx, runCtx.Tools); err != nil {
		return ctx, runCtx, fmt.Errorf("validate dynamic tool descriptors: %w", err)
	}
	return ctx, runCtx, nil
}
