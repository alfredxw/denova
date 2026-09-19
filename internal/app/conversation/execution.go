package conversationapp

import (
	"context"
	"errors"
	"sync"

	"denova/config"
	agents "denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/external"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
)

// Execution keeps the product preparation and task lifecycle common while
// selecting exactly one executor. Only foreground conversation entries may
// supply an external engine; automation must use the Native lane.
type Execution struct {
	Composition prompts.SystemPromptComposition
	runtime     Runtime
	native      appagentruntime.BuiltAgent
	external    *agents.ExternalAssembly
	engines     *appagentruntime.Engines
}

func BuildExecution(ctx context.Context, runtime Runtime, host agents.AgentHostCapabilities, engines *appagentruntime.Engines, origin string) (Execution, error) {
	execution := Execution{runtime: runtime, engines: engines}
	if runtime.Config.ActiveAgentRuntime == nil || runtime.Config.ActiveAgentRuntime.Kind == config.RuntimeNative {
		built, err := appagentruntime.BuildConversationAgent(ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind, host)
		execution.native, execution.Composition = built, built.Composition
		return execution, err
	}
	if origin != "" || engines == nil {
		return Execution{}, conversationconfig.ErrRuntimeCapabilityUnsupported
	}
	host.Interactive = true
	assembly, err := agents.BuildExternalConversationAssembly(ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind, host)
	execution.external, execution.Composition = &assembly, assembly.Composition
	return execution, err
}

type acceptedOperation interface {
	Receipt() agentrun.CommandReceipt
	Wait(context.Context) agentrun.Outcome
}

// Operation owns the engine lease through settlement. OutputCommitted follows
// the canonical receipt, so late transport errors cannot roll back content.
type Operation struct {
	accepted     acceptedOperation
	conversation *agentconversation.SessionConversation
	external     bool
	release      func()
	once         sync.Once
	outcome      agentrun.Outcome
}

func (operation *Operation) Receipt() agentrun.CommandReceipt { return operation.accepted.Receipt() }
func (operation *Operation) IsExternal() bool                 { return operation.external }
func (operation *Operation) Wait(ctx context.Context) agentrun.Outcome {
	operation.once.Do(func() {
		if operation.release != nil {
			defer operation.release()
		}
		operation.outcome = operation.accepted.Wait(ctx)
	})
	return operation.outcome
}
func (operation *Operation) OutputCommitted() bool {
	if operation.external {
		return operation.outcome.Status == agentrun.OutcomeCompleted
	}
	_, committed := operation.conversation.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput)
	return committed
}

func (execution Execution) Start(ctx context.Context, request agentchat.ChatRequest, conversation *agentconversation.SessionConversation, options agentrun.Options, emit func(agentrun.Event)) (*Operation, error) {
	if execution.engines != nil && execution.runtime.Session != nil {
		release, err := execution.engines.AdmitExecution(ctx, execution.runtime.Session, execution.runtime.Config.ActiveAgentRuntime)
		if err != nil {
			return nil, err
		}
		defer release()
	}
	operation := &Operation{conversation: conversation}
	if execution.external == nil {
		accepted, err := execution.runtime.ExecutionRuntime.Start(ctx, agentexecution.StartRequest{
			Cycle: agentexecution.Cycle{Definition: execution.native.Definition, Conversation: conversation,
				BookService: execution.runtime.BookService, Request: request, Options: options}, Emit: emit,
		})
		operation.accepted = accepted
		return operation, err
	}
	if execution.runtime.Session == nil {
		return nil, errors.New("external execution requires a Session")
	}
	if request.PlanMode {
		return nil, conversationconfig.ErrRuntimeCapabilityUnsupported
	}
	if err := execution.engines.Operations.Recover(ctx, execution.runtime.ProjectID, execution.runtime.Session); err != nil {
		return nil, err
	}
	if err := external.Reconcile(ctx, execution.runtime.Session, execution.runtime.Workspace, execution.runtime.ProjectStore); err != nil {
		return nil, err
	}
	prepared, err := prepareExternal(ctx, execution.runtime, request, conversation, *execution.external, options, emit)
	if err != nil {
		return nil, err
	}
	adapter, release, err := execution.engines.Acquire(ctx, prepared.Input.Selection, execution.runtime.Config)
	if err != nil {
		return nil, err
	}
	prepared.Adapter = adapter
	accepted, err := execution.engines.Operations.Start(ctx, prepared)
	if err != nil {
		release()
		return nil, err
	}
	operation.accepted, operation.external, operation.release = accepted, true, release
	return operation, nil
}

var _ acceptedOperation = (*external.Operation)(nil)
