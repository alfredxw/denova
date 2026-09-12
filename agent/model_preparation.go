package agent

import (
	"context"
	"errors"
	"fmt"
)

// Maintenance can call a summary model before primary model I/O begins.
// Bind that work to the same Abort/Suspend control, restoring the surrounding
// retry binding afterward. A prepared replacement owns its live parent context,
// so closing this side fork does not cancel the accepted provider call.
func (agent *modelToolLoop) applyModelCallGate(ctx context.Context, call *ModelCall, metadata *ModelContext, cancel *cancelControl) (*preparedModelCall, error) {
	gateCtx, stop := context.WithCancel(ctx)
	previous := cancel.bindModel(stop)
	defer func() { cancel.bindModel(previous); stop() }()
	return agent.modelCallGate(gateCtx, call, metadata)
}

// prepareModelStep is shared by normal execution and the candidate validated by
// automatic Compaction. It stays in the live invocation: BeforeAgent is not
// repeated, and middleware contexts and wrappers survive until the provider call.
func (agent *modelToolLoop) prepareModelStep(ctx context.Context, state *RunState, modelContext *ModelContext, streaming bool) (*preparedModelCall, error) {
	entryCtx := ctx
	entryTools := cloneToolInfos(state.ToolInfos)
	entryExtra := cloneStringAnyMap(state.Extra)
	var err error
	for _, middleware := range agent.middlewares {
		ctx, state, err = middleware.BeforeModelRewriteState(ctx, state, modelContext)
		if err != nil {
			return nil, fmt.Errorf("before model middleware: %w", err)
		}
		if ctx == nil {
			return nil, errors.New("before model middleware returned nil Go context")
		}
		if state == nil {
			return nil, errors.New("before model middleware returned nil state")
		}
	}
	modelContext.Tools = cloneToolInfos(state.ToolInfos)
	model, err := agent.modelForCall(ctx, modelContext)
	if err != nil {
		return nil, err
	}
	options := []ModelOption{WithTools(state.ToolInfos)}
	if sessionKey, ok := SessionKeyFromContext(ctx); ok {
		options = append(options, WithSessionKey(sessionKey))
	}
	call := &ModelCall{Model: model, Messages: cloneMessages(state.Messages), Options: options, Streaming: streaming}
	modelContext.maintenanceMessages = cloneMessages(call.Messages)
	ctx, call, err = agent.beforeModelCall(ctx, call, modelContext)
	if err != nil {
		return nil, err
	}
	modelContext.prepareCompaction = func(messages []*Message, stable int) (*preparedModelCall, error) {
		if modelContext.instruction != "" {
			messages = append([]*Message{SystemMessage(modelContext.instruction)}, messages...)
			stable++
		}
		nextContext := &ModelContext{
			Tools: cloneToolInfos(entryTools), Iteration: modelContext.Iteration,
			stablePrefixSeed: cloneMessages(messages[:min(stable, len(messages))]), instruction: modelContext.instruction,
		}
		next, err := agent.prepareModelStep(contextWithMaintenanceCommitted(entryCtx), &RunState{
			Messages: cloneMessages(messages), ToolInfos: cloneToolInfos(entryTools), Extra: cloneStringAnyMap(entryExtra),
		}, nextContext, streaming)
		if err != nil {
			return nil, err
		}
		return agent.freezeCompactionCall(next)
	}
	return &preparedModelCall{ctx: ctx, call: call, modelContext: modelContext, state: state}, nil
}

func (agent *modelToolLoop) beforeModelCall(ctx context.Context, call *ModelCall, modelContext *ModelContext) (context.Context, *ModelCall, error) {
	var err error
	for _, middleware := range agent.middlewares {
		ctx, call, err = middleware.BeforeModelCall(ctx, call, modelContext)
		if err != nil {
			return ctx, nil, fmt.Errorf("before model call middleware: %w", err)
		}
		if ctx == nil {
			return nil, nil, errors.New("before model call middleware returned nil Go context")
		}
		if call == nil || call.Model == nil {
			return ctx, nil, errors.New("before model call middleware returned nil model call")
		}
	}
	call.modelIdentity = agent.modelIdentity
	if model, ok := call.Model.(DefinitionModel); ok {
		call.modelIdentity = model.ModelIdentity()
	}
	call.stablePrefixMessages = authenticatedStablePrefixMessages(call.Messages, modelContext.stablePrefixSeed)
	return ctx, call, nil
}

func (agent *modelToolLoop) freezeCompactionCall(step *preparedModelCall) (*preparedModelCall, error) {
	messages, err := projectToolArtifactPaths(step.ctx, agent.artifacts, step.call.Messages)
	if err != nil {
		return nil, err
	}
	step.call.providerMessages = messages
	return step, nil
}
