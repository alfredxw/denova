package conversationapp

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"

	"denova/config"
	agents "denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	externaljournal "denova/internal/agents/runtime/external/journal"
	"denova/internal/agents/session"
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
	engines     *agentruntime.Engines
}

func BuildExecution(ctx context.Context, runtime Runtime, host agents.AgentHostCapabilities, engines *agentruntime.Engines, origin string) (Execution, error) {
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
	state, err := agentruntime.SessionState(options, execution.runtime.Session)
	if err != nil {
		return nil, err
	}
	control, err := execution.engines.ExternalControl(options, state)
	if err != nil {
		return nil, err
	}
	accepted, err := control.Start(ctx, agentruntime.ExternalCycleInput{Request: request}, execution.ExternalFactory(request, conversation, options), emit)
	if err != nil {
		return nil, err
	}
	operation.accepted, operation.external = accepted, true
	return operation, nil
}

// ExternalFactory rebuilds product context at each accepted input boundary;
// queue, pause and Goal orchestration remain outside the Native runtime.
func (execution Execution) ExternalFactory(initial agentchat.ChatRequest, conversation *agentconversation.SessionConversation, options agentrun.Options) agentruntime.ExternalCycleFactory {
	return func(ctx context.Context, input agentruntime.ExternalCycleInput, emit func(agentrun.Event), after func(context.Context, *external.RuntimeSession) error) (agentruntime.ExternalCycle, error) {
		request := input.Request
		cycleOptions := options
		if err := execution.engines.Operations.Recover(ctx, execution.runtime.ProjectID, execution.runtime.Session); err != nil {
			return nil, err
		}
		var previous *externaljournal.Operation
		if input.Resume {
			if err := execution.runtime.Session.ReadExternal(ctx, func(state session.ExternalState) error {
				for _, operation := range state.Projection.Operations {
					if (operation.CommandID == request.CommandID || operation.InputCommandID == request.CommandID) && (previous == nil || previous.Accepted.Cursor < operation.Accepted.Cursor) {
						copy := *operation
						previous = &copy
					}
				}
				return nil
			}); err != nil {
				return nil, err
			}
		}
		resumeAccepted := previous != nil && previous.Status != externaljournal.Completed
		if request.CommandID != initial.CommandID || resumeAccepted {
			cycleOptions.InputCommitEffect = nil
		}
		if resumeAccepted {
			request = agentchat.CallerView(input.Request).Request()
			request.CommandID = "resume-" + rand.Text()
			request.Message = "Continue the accepted task from the confirmed conversation and tool outcomes. Do not repeat effects that already succeeded."
			request.InputVisibility = agentrun.InputModelOnly
			request.AttachedFiles = input.Request.AttachedFiles
			request.DisplayMessage = ""
		}
		if previous == nil || resumeAccepted {
			consumed := 0
			if previous != nil {
				consumed = previous.GuidanceCount
			}
			var guidance []string
			for _, additional := range input.Guidance[consumed:] {
				guidance = append(guidance, additional.Message)
				request.AttachedFiles = append(request.AttachedFiles, additional.AttachedFiles...)
				request.AttachmentIDs = append(request.AttachmentIDs, additional.AttachmentIDs...)
			}
			if len(guidance) > 0 {
				request.Message += "\n\nAdditional user instructions:\n" + strings.Join(guidance, "\n\n")
				request.DisplayMessage = strings.Join(guidance, "\n\n")
				request.InputVisibility = agentrun.InputVisible
			}
		}
		request = agentchat.CaptureChatRequestCallerInput(request)
		if err := external.Reconcile(ctx, execution.runtime.Session, execution.runtime.Workspace, execution.runtime.ProjectStore); err != nil {
			return nil, err
		}
		runtime, resolved, err := Prepare(ctx, execution.runtime, request)
		if err != nil {
			return nil, err
		}
		projected := conversation
		if input.Resume || request.CommandID != initial.CommandID {
			projected = ProjectConversation(runtime, resolved)
		}
		prepared, err := prepareExternal(ctx, runtime, resolved, projected, *execution.external, cycleOptions, emit)
		if err != nil {
			return nil, err
		}
		prepared.Runtime, prepared.AfterCommit = execution.engines.ExternalRuntime(runtime.Config), after
		prepared.InputCommandID, prepared.GuidanceCount = input.Request.CommandID, len(input.Guidance)
		prepared.PrepareGuidance = func(ctx context.Context, guidance agentchat.ChatRequest) (external.StartRequest, error) {
			current, resolved, err := Prepare(ctx, execution.runtime, guidance)
			if err != nil {
				return external.StartRequest{}, err
			}
			return prepareExternalInput(ctx, current, resolved, ProjectConversation(current, resolved), *execution.external, cycleOptions, emit)
		}
		if previous != nil && previous.Status == externaljournal.Completed {
			// Canonical completion wins over a lost controller acknowledgement.
			prepared.CommandID, prepared.Fingerprint = previous.CommandID, previous.Fingerprint
		}
		return execution.engines.Operations.Start(ctx, prepared)
	}
}

var _ acceptedOperation = (*external.Operation)(nil)
