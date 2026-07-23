package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

// validateToolDescriptors makes the descriptor catalog part of Agent
// construction, so a newly registered tool cannot silently inherit unknown
// recovery behavior. A final runtime guard applies the same check after every
// BeforeAgent middleware has injected its concrete tools.
func validateToolDescriptors(ctx context.Context, tools []tool.BaseTool) error {
	_, err := validatedConcreteToolNames(ctx, tools)
	return err
}

func validatedConcreteToolNames(ctx context.Context, tools []tool.BaseTool) ([]string, error) {
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

func validateToolDescriptorNames(names []string) error {
	seen := make(map[string]int, len(names))
	for index, name := range names {
		name = normalizeToolName(name)
		if name == "" {
			return fmt.Errorf("middleware tool declaration at index %d has no stable name", index)
		}
		if previous, duplicate := seen[name]; duplicate {
			return fmt.Errorf("duplicate middleware tool declaration %q at indexes %d and %d", name, previous, index)
		}
		seen[name] = index
		if _, declared := declaredToolDescriptor(name); !declared {
			return fmt.Errorf("middleware tool %q has no explicit ToolDescriptor", name)
		}
	}
	return nil
}

// validateToolSurface keeps tool names a one-to-one mapping to an endpoint and
// recovery contract. A middleware declaration colliding with a static tool is
// rejected during construction; the final runtime guard repeats the check on
// the concrete post-middleware tool list.
func validateToolSurface(ctx context.Context, staticTools []tool.BaseTool, middlewareNames []string) error {
	staticToolNames, err := validatedConcreteToolNames(ctx, staticTools)
	if err != nil {
		return err
	}
	if err := validateToolDescriptorNames(middlewareNames); err != nil {
		return err
	}
	staticNames := make(map[string]struct{}, len(staticToolNames))
	for _, name := range staticToolNames {
		staticNames[name] = struct{}{}
	}
	for _, name := range middlewareNames {
		name = normalizeToolName(name)
		if _, duplicate := staticNames[name]; duplicate {
			return fmt.Errorf("duplicate model-visible tool %q across static and middleware registrations", name)
		}
	}
	return nil
}

// toolDescriptorGuardMiddleware is deliberately the final BeforeAgent handler.
// External filesystem/skill middleware can append tools dynamically, so their
// concrete result must cross the same fail-closed catalog boundary as static
// assembly tools before the first provider request is created.
type toolDescriptorGuardMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

func newToolDescriptorGuardMiddleware() *toolDescriptorGuardMiddleware {
	return &toolDescriptorGuardMiddleware{BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{}}
}

func (m *toolDescriptorGuardMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	if runCtx == nil {
		return ctx, runCtx, fmt.Errorf("validate dynamic tool descriptors: agent context is nil")
	}
	if err := validateToolDescriptors(ctx, runCtx.Tools); err != nil {
		return ctx, runCtx, fmt.Errorf("validate dynamic tool descriptors: %w", err)
	}
	return ctx, runCtx, nil
}
