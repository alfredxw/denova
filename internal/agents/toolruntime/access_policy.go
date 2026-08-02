package toolruntime

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

// ToolAccessMode is a turn-scoped restriction layered on top of the Agent's
// configured capabilities. Capability settings decide what an Agent may ever
// use; an access mode narrows that already-authorized surface for one run.
type ToolAccessMode string

const (
	ToolAccessModeDefault      ToolAccessMode = ""
	ToolAccessModePlanReadOnly ToolAccessMode = "plan_read_only"
)

type toolAccessModeContextKey struct{}

// ContextWithToolAccessMode binds one access policy to the current invocation.
// Child invocations inherit the value, so delegation cannot widen the root
// turn's authority.
func ContextWithToolAccessMode(ctx context.Context, mode ToolAccessMode) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if mode == ToolAccessModeDefault {
		return ctx
	}
	return context.WithValue(ctx, toolAccessModeContextKey{}, mode)
}

func toolAccessModeFromContext(ctx context.Context) ToolAccessMode {
	if ctx == nil {
		return ToolAccessModeDefault
	}
	mode, _ := ctx.Value(toolAccessModeContextKey{}).(ToolAccessMode)
	return mode
}

// BeforeAgent removes disallowed definitions before the provider sees tool
// schemas. WrapToolCall applies the same policy again as a fail-closed defense.
func (m *OrchestratorMiddleware) BeforeAgent(
	ctx context.Context,
	run *agent.RunContext,
) (context.Context, *agent.RunContext, error) {
	if run == nil {
		return ctx, run, nil
	}
	mode := toolAccessModeFromContext(ctx)
	if mode == ToolAccessModeDefault {
		return ctx, run, nil
	}

	filtered := make([]agent.ToolDefinition, 0, len(run.Tools))
	for _, definition := range run.Tools {
		if toolAllowedByAccessMode(mode, definition.Descriptor) {
			filtered = append(filtered, definition)
		}
	}
	next := *run
	next.Tools = filtered
	return ctx, &next, nil
}

func toolAllowedByAccessMode(mode ToolAccessMode, descriptor agent.ToolDescriptor) bool {
	switch mode {
	case ToolAccessModeDefault:
		return true
	case ToolAccessModePlanReadOnly:
		if descriptor.MutationScope == agent.ToolMutationNone {
			return true
		}
		// Only planning controls may update session state. A scope-only rule
		// would accidentally admit game/domain commits if Plan Mode is reused by
		// another Agent kind in the future.
		if descriptor.MutationScope == agent.ToolMutationSession {
			switch descriptor.Capability {
			case config.AgentToolAsk, config.AgentToolTodo, config.AgentToolContextRewind:
				return true
			}
		}
		return false
	default:
		return false
	}
}

func toolAccessModeBlockedMessage(mode ToolAccessMode, name string, descriptor agent.ToolDescriptor) string {
	if mode != ToolAccessModePlanReadOnly {
		return fmt.Sprintf(
			"[tool error] Runtime access mode %q blocked tool %q (capability %q, mutation scope %s). / 运行时访问模式 %q 已阻止工具 %q（能力 %q，变更范围 %s）。",
			mode, name, descriptor.Capability, descriptor.MutationScope,
			mode, name, descriptor.Capability, descriptor.MutationScope,
		)
	}
	return fmt.Sprintf(
		"[tool error] Plan Mode is read-only and blocked tool %q (capability %q, mutation scope %s). Approve the plan or switch to Chat Mode before using it. / 规划模式为只读，工具 %q（能力 %q，变更范围 %s）已被阻止；请先确认计划或切换到 Chat Mode。",
		name, descriptor.Capability, descriptor.MutationScope,
		name, descriptor.Capability, descriptor.MutationScope,
	)
}
