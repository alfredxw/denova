package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentstructural "denova/internal/agents/context/structural"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	"fmt"
	"strconv"
	"strings"

	"denova/config"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
	"denova/internal/interactive"
)

func (a *App) prepareWritingProfileCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
	binding agentrun.RuntimeBinding,
) (agentexecution.Cycle, error) {
	var activeRuntime ideChatRuntime
	var task *apptask.Task
	if strings.TrimSpace(request.Options.TaskID) != "" {
		var err error
		activeRuntime, task, err = a.chat().activeCommandRuntime()
		if err != nil {
			return agentexecution.Cycle{}, err
		}
		if task.ID() != strings.TrimSpace(request.Options.TaskID) || activeRuntime.workspace != binding.Workspace || activeRuntime.sess == nil || activeRuntime.sess.ID != binding.SessionID {
			return agentexecution.Cycle{}, ErrAgentContextChanged
		}
	}
	cycle, runtime, err := a.chat().prepareWritingCycle(ctx, request.Request, request.Options.TaskID)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if strings.TrimSpace(runtime.workspace) != binding.Workspace || runtime.sess == nil || runtime.sess.ID != binding.SessionID {
		return agentexecution.Cycle{}, fmt.Errorf(
			"%w: prepared writing runtime does not match durable binding",
			agentexecution.ErrCyclePreparationUnavailable,
		)
	}
	if task != nil {
		if runtime.workspace != activeRuntime.workspace || runtime.sess != activeRuntime.sess || runtime.executionRuntime != activeRuntime.executionRuntime {
			return agentexecution.Cycle{}, ErrAgentContextChanged
		}
		if err := a.chat().confirmActiveCommandRuntime(activeRuntime, task); err != nil {
			return agentexecution.Cycle{}, err
		}
	}
	cycle.Successor = func(ctx context.Context, parent agentrun.OperationID, outcome agentrun.Outcome) error {
		runtime, task, err := a.chat().activeCommandRuntime()
		if err != nil {
			return err
		}
		if runtime.sess == nil || runtime.sess.ID != binding.SessionID || runtime.workspace != binding.Workspace {
			return ErrAgentContextChanged
		}
		return a.chat().writingGoalSuccessor(runtime, task, request.Request.Locale)(ctx, parent, outcome)
	}
	return cycle, nil
}

func (a *App) prepareInteractiveProfileCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
	binding agentrun.RuntimeBinding,
) (agentexecution.Cycle, error) {
	var target interactiveAgentCommandTarget
	if strings.TrimSpace(request.Options.TaskID) != "" {
		var err error
		target, err = a.interactiveService().activeAgentCommandTarget(binding.StoryID, binding.BranchID)
		if err != nil {
			return agentexecution.Cycle{}, err
		}
		if target.task.ID() != strings.TrimSpace(request.Options.TaskID) {
			return agentexecution.Cycle{}, ErrAgentContextChanged
		}
	}
	cycle, err := a.interactiveService().prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
		StoryID: binding.StoryID, BranchID: binding.BranchID,
		Message: request.Request.Message, StyleScenes: request.Request.StyleScenes,
		Locale: request.Request.Locale,
	})
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if cycle.workspace != binding.Workspace || cycle.storyID != binding.StoryID || cycle.branchID != binding.BranchID {
		return agentexecution.Cycle{}, fmt.Errorf(
			"%w: prepared game runtime does not match durable binding",
			agentexecution.ErrCyclePreparationUnavailable,
		)
	}
	if target.task != nil {
		if cycle.executionRuntime != target.executionRuntime {
			return agentexecution.Cycle{}, ErrAgentContextChanged
		}
		if err := a.interactiveService().confirmActiveAgentCommandTarget(target); err != nil {
			return agentexecution.Cycle{}, err
		}
	}
	cycle.bindCommit(request.Emit)
	return agentexecution.Cycle{
		Runner: cycle.runner, Conversation: cycle.conversation,
		BookService: cycle.bookService, Request: cycle.request,
		Options: cycle.options(request.Options.TaskID),
	}, nil
}

func (s *ChatAppService) prepareWritingCycle(
	ctx context.Context,
	request agentchat.ChatRequest,
	taskID string,
) (agentexecution.Cycle, ideChatRuntime, error) {
	runtime, resolved, err := s.prepareIDEChatRuntime(ctx, request)
	if err != nil {
		return agentexecution.Cycle{}, ideChatRuntime{}, err
	}
	goalTools, err := appagentruntime.GoalTools(ctx, runtime.sess)
	if err != nil {
		return agentexecution.Cycle{}, ideChatRuntime{}, err
	}
	runner, systemPrompt, err := appagentruntime.BuildConversation(
		ctx, &runtime.cfg, runtime.state, runtime.ideTeller, agentrun.AgentKindIDE,
		goalTools...,
	)
	if err != nil {
		return agentexecution.Cycle{}, ideChatRuntime{}, err
	}
	runtimeContexts := prompts.IDEWorkspaceRuntimeContextsForContext(runtime.state, resolved.IDEContext)
	conversation := agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	).WithInputVisibility(resolved.InputVisibility)
	options := s.bindReviewFeedbackInputCommit(agentrun.Options{
		AgentKind:          agentrun.AgentKindIDE,
		StateRoot:          runtime.projectState,
		TaskID:             strings.TrimSpace(taskID),
		SessionID:          runtime.sess.ID,
		Workspace:          runtime.workspace,
		Mode:               "ide",
		IdleTimeout:        appagentruntime.IdleTimeout(runtime.cfg),
		ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(runtime.cfg),
		SystemPromptLog:    systemPrompt,
		OnMutationsVerified: s.app.writingMutationCallback(
			strings.TrimSpace(taskID), conversation,
		),
	}, runtime, resolved)
	return agentexecution.Cycle{
		Runner: runner, Conversation: conversation, BookService: runtime.bookService, Request: resolved,
		Options: options,
	}, runtime, nil
}

type executionDomainCommitter interface {
	reconcileDomainCommit(context.Context, agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error)
}

type inputExecutionDomain interface {
	executionDomainCommitter
	planInput(context.Context, agentexecution.InputMaterializationRequest) (agentrun.InputMaterializationPlan, error)
	materializeInput(context.Context, agentexecution.InputMaterializationRequest, agentrun.InputMaterializationPlan) (agentrun.InputMaterializationReceipt, error)
}

type applicationExecutionProfile struct {
	id     agentexecution.ProfileID
	app    *App
	domain executionDomainCommitter
}

func (profile *applicationExecutionProfile) ID() agentexecution.ProfileID {
	if profile == nil {
		return ""
	}
	return profile.id
}

func (profile *applicationExecutionProfile) ReconcileDomainCommit(
	ctx context.Context,
	request agentrun.DomainCommitReconcileRequest,
) (agentrun.DomainCommitReconcileResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return agentrun.DomainCommitReconcileResult{}, err
		}
	}
	if err := validateDomainCommitReconcileRequest(request); err != nil {
		return agentrun.DomainCommitReconcileResult{}, err
	}
	if request.Structural != nil {
		if profile == nil || profile.app == nil {
			return agentrun.DomainCommitReconcileResult{}, agentexecution.ErrStructuralRestoreUnavailable
		}
		return profile.app.reconcileStructuralDomainCommit(ctx, request)
	}
	if profile == nil || profile.domain == nil {
		return agentrun.DomainCommitReconcileResult{}, fmt.Errorf("%w: profile %q has no domain adapter", agentexecution.ErrProfileInvalid, profile.ID())
	}
	return profile.domain.reconcileDomainCommit(ctx, request)
}

type inputExecutionProfile struct {
	*applicationExecutionProfile
	input inputExecutionDomain
}

func (profile *inputExecutionProfile) PlanInput(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
) (agentrun.InputMaterializationPlan, error) {
	if profile == nil {
		return agentrun.InputMaterializationPlan{}, fmt.Errorf("%w: profile has no input adapter", agentexecution.ErrProfileInvalid)
	}
	if profile.input == nil {
		return agentrun.InputMaterializationPlan{}, fmt.Errorf("%w: profile %q has no input adapter", agentexecution.ErrProfileInvalid, profile.ID())
	}
	return profile.input.planInput(ctx, request)
}

func (profile *inputExecutionProfile) MaterializeInput(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
	plan agentrun.InputMaterializationPlan,
) (agentrun.InputMaterializationReceipt, error) {
	if profile == nil {
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("%w: profile has no input adapter", agentexecution.ErrProfileInvalid)
	}
	if profile.input == nil {
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("%w: profile %q has no input adapter", agentexecution.ErrProfileInvalid, profile.ID())
	}
	return profile.input.materializeInput(ctx, request, plan)
}

type queuedInputExecutionProfile struct {
	*inputExecutionProfile
	prepare func(context.Context, agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error)
}

func (profile *queuedInputExecutionProfile) PrepareCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
) (agentexecution.Cycle, error) {
	return profile.prepare(ctx, request)
}

type structuralExecutionCapability struct{ app *App }

func (capability structuralExecutionCapability) RestoreStructural(
	ctx context.Context,
	request agentexecution.StructuralRestoreRequest,
) (agentstructural.Spec, error) {
	if capability.app == nil {
		return agentstructural.Spec{}, agentexecution.ErrStructuralRestoreUnavailable
	}
	return capability.app.restoreContextStructuralOperation(ctx, request)
}

type structuralQueuedInputExecutionProfile struct {
	*queuedInputExecutionProfile
	structuralExecutionCapability
}

type structuralInputExecutionProfile struct {
	*inputExecutionProfile
	structuralExecutionCapability
}

type structuralDomainExecutionProfile struct {
	*applicationExecutionProfile
	structuralExecutionCapability
}

type sessionExecutionDomain struct{ app *App }

func (domain sessionExecutionDomain) planInput(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
) (agentrun.InputMaterializationPlan, error) {
	intent, err := domain.app.sessionAcceptedInputIntent(ctx, request)
	if err != nil {
		return agentrun.InputMaterializationPlan{}, err
	}
	return agentrun.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
}

func (domain sessionExecutionDomain) materializeInput(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
	plan agentrun.InputMaterializationPlan,
) (agentrun.InputMaterializationReceipt, error) {
	if !plan.Required || plan.Hash == "" {
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("accepted Session input materialization requires an exact semantic hash")
	}
	intent, err := domain.app.sessionAcceptedInputIntent(ctx, request)
	if err != nil {
		return agentrun.InputMaterializationReceipt{}, err
	}
	if intent.Hash != plan.Hash {
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted Session input changed after planning", session.ErrDomainCommitIdentityConflict)
	}
	receipt, err := domain.app.commitSessionAcceptedInput(ctx, request.Binding, intent)
	if err != nil {
		return agentrun.InputMaterializationReceipt{}, err
	}
	return agentrun.InputMaterializationReceipt{Revision: strconv.FormatUint(receipt.ContextRevision, 10)}, nil
}

func (domain sessionExecutionDomain) reconcileDomainCommit(
	_ context.Context,
	request agentrun.DomainCommitReconcileRequest,
) (agentrun.DomainCommitReconcileResult, error) {
	return domain.app.reconcileSessionDomainCommit(request)
}

type gameExecutionDomain struct{ app *App }

func (domain gameExecutionDomain) planInput(
	_ context.Context,
	request agentexecution.InputMaterializationRequest,
) (agentrun.InputMaterializationPlan, error) {
	intent, err := gameAcceptedInputIntent(request)
	if err != nil {
		return agentrun.InputMaterializationPlan{}, err
	}
	return agentrun.InputMaterializationPlan{Required: true, Hash: intent.Hash}, nil
}

func (domain gameExecutionDomain) materializeInput(
	_ context.Context,
	request agentexecution.InputMaterializationRequest,
	plan agentrun.InputMaterializationPlan,
) (agentrun.InputMaterializationReceipt, error) {
	if !plan.Required || plan.Hash == "" {
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("accepted Game input materialization requires an exact semantic hash")
	}
	intent, err := gameAcceptedInputIntent(request)
	if err != nil {
		return agentrun.InputMaterializationReceipt{}, err
	}
	if intent.Hash != plan.Hash {
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("%w: accepted player input changed after planning", interactive.ErrPlayerInputIdentityConflict)
	}
	receipt, err := interactive.NewStore(request.Binding.Workspace).CommitPlayerInput(request.Binding.StoryID, intent)
	if err != nil {
		return agentrun.InputMaterializationReceipt{}, err
	}
	return agentrun.InputMaterializationReceipt{Revision: receipt.Revision}, nil
}

func (domain gameExecutionDomain) reconcileDomainCommit(
	_ context.Context,
	request agentrun.DomainCommitReconcileRequest,
) (agentrun.DomainCommitReconcileResult, error) {
	return reconcileGameDomainCommit(request)
}

type directorExecutionDomain struct{}

func (directorExecutionDomain) reconcileDomainCommit(
	_ context.Context,
	request agentrun.DomainCommitReconcileRequest,
) (agentrun.DomainCommitReconcileResult, error) {
	return reconcileDirectorDomainCommit(request)
}

func (app *App) executionProfiles() []agentexecution.Profile {
	sessionDomain := sessionExecutionDomain{app: app}
	gameDomain := gameExecutionDomain{app: app}
	directorDomain := directorExecutionDomain{}
	base := func(
		id agentexecution.ProfileID,
		domain executionDomainCommitter,
	) *applicationExecutionProfile {
		return &applicationExecutionProfile{id: id, app: app, domain: domain}
	}
	input := func(id agentexecution.ProfileID, domain inputExecutionDomain) *inputExecutionProfile {
		return &inputExecutionProfile{applicationExecutionProfile: base(id, domain), input: domain}
	}
	queuedInput := func(
		id agentexecution.ProfileID,
		domain inputExecutionDomain,
		prepare func(context.Context, agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error),
	) *queuedInputExecutionProfile {
		return &queuedInputExecutionProfile{inputExecutionProfile: input(id, domain), prepare: prepare}
	}
	structural := structuralExecutionCapability{app: app}
	writing := queuedInput(agentexecution.ProfileWriting, sessionDomain, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
		return app.prepareWritingProfileCycle(ctx, request, request.Binding)
	})
	agentChat := queuedInput(agentexecution.ProfileAgentChat, sessionDomain, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
		return app.AgentChat().PrepareCycle(ctx, request, request.Binding)
	})
	game := queuedInput(agentexecution.ProfileGame, gameDomain, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
		return app.prepareInteractiveProfileCycle(ctx, request, request.Binding)
	})
	configManager := input(agentexecution.ProfileConfigManager, sessionDomain)
	image := input(agentexecution.ProfileImage, sessionDomain)
	director := base(agentexecution.ProfileDirector, directorDomain)
	automation := input(agentexecution.ProfileAutomation, sessionDomain)
	return []agentexecution.Profile{
		&structuralQueuedInputExecutionProfile{queuedInputExecutionProfile: writing, structuralExecutionCapability: structural},
		&structuralQueuedInputExecutionProfile{queuedInputExecutionProfile: agentChat, structuralExecutionCapability: structural},
		&structuralQueuedInputExecutionProfile{queuedInputExecutionProfile: game, structuralExecutionCapability: structural},
		&structuralInputExecutionProfile{inputExecutionProfile: configManager, structuralExecutionCapability: structural},
		&structuralInputExecutionProfile{inputExecutionProfile: image, structuralExecutionCapability: structural},
		&structuralDomainExecutionProfile{applicationExecutionProfile: director, structuralExecutionCapability: structural},
		automation,
	}
}
