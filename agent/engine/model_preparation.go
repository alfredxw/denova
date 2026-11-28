package engine

import (
	"context"
	"errors"
	"fmt"

	agentmiddleware "github.com/alfredxw/denova/agent/engine/middleware"
	agentmodel "github.com/alfredxw/denova/agent/model"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

// modelContext keeps execution callbacks and trusted prefix provenance out of
// the public middleware contract. Middleware sees only ModelContext.
type modelStepContext struct {
	agentmiddleware.ModelContext
	// prepareCompaction prepares a replacement in the active loop, including
	// retry feedback. Lifecycle validates and publishes it before execution.
	prepareCompaction   func([]*agentschema.Message, int) (*preparedModelCall, error)
	instruction         string
	maintenanceMessages []*agentschema.Message
	// stablePrefixSeed is lifecycle-owned provenance. It is deliberately kept
	// outside Message.Extra so model output and caller middleware cannot label
	// arbitrary body messages as cache-stable context.
	stablePrefixSeed  []*agentschema.Message
	activeUserMessage *agentschema.Message
}

// Maintenance can call a summary model before primary model I/O begins.
// Bind that work to the same Abort/Suspend control, restoring the surrounding
// retry binding afterward. A prepared replacement owns its live parent context,
// so closing this side fork does not cancel the accepted provider call.
func (agent *modelToolLoop) applyModelCallGate(ctx context.Context, call *modelCall, metadata *modelStepContext, cancel *cancelControl) (*preparedModelCall, error) {
	gateCtx, stop := context.WithCancel(ctx)
	previous := cancel.bindModel(stop)
	defer func() { cancel.bindModel(previous); stop() }()
	return agent.modelCallGate(gateCtx, call, metadata)
}

// prepareModelStep is shared by normal execution and the candidate validated by
// automatic Compaction. It stays in the live invocation: BeforeAgent is not
// repeated, and middleware contexts and wrappers survive until the provider call.
func (agent *modelToolLoop) prepareModelStep(ctx context.Context, state *agentmiddleware.RunState, modelContext *modelStepContext, streaming bool) (*preparedModelCall, error) {
	entryCtx := ctx
	entryTools := agentschema.CloneToolInfos(state.ToolInfos)
	entryExtra := agentschema.CloneStringAnyMap(state.Extra)
	var err error
	for _, middleware := range agent.middlewares {
		ctx, state, err = middleware.BeforeModelRewriteState(ctx, state, &modelContext.ModelContext)
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
	modelContext.Tools = agentschema.CloneToolInfos(state.ToolInfos)
	model, err := agent.modelForCall(ctx, modelContext)
	if err != nil {
		return nil, err
	}
	options := []agentmodel.ModelOption{agentmodel.WithTools(state.ToolInfos)}
	if sessionKey, ok := agentmodel.SessionKeyFromContext(ctx); ok {
		options = append(options, agentmodel.WithSessionKey(sessionKey))
	}
	call := &modelCall{Model: model, Messages: agentschema.CloneMessages(state.Messages), Options: options, Streaming: streaming}
	modelContext.maintenanceMessages = agentschema.CloneMessages(call.Messages)
	ctx, call, err = agent.beforeModelCall(ctx, call, modelContext)
	if err != nil {
		return nil, err
	}
	modelContext.prepareCompaction = func(messages []*agentschema.Message, stable int) (*preparedModelCall, error) {
		if modelContext.instruction != "" {
			messages = append([]*agentschema.Message{agentschema.SystemMessage(modelContext.instruction)}, messages...)
			stable++
		}
		nextContext := &modelStepContext{
			ModelContext:      agentmiddleware.ModelContext{Tools: agentschema.CloneToolInfos(entryTools), Iteration: modelContext.Iteration},
			stablePrefixSeed:  agentschema.CloneMessages(messages[:min(stable, len(messages))]),
			activeUserMessage: agentschema.CloneMessage(modelContext.activeUserMessage),
			instruction:       modelContext.instruction,
		}
		next, err := agent.prepareModelStep(agentmiddleware.ContextWithMaintenanceCommitted(entryCtx), &agentmiddleware.RunState{
			Messages: agentschema.CloneMessages(messages), ToolInfos: agentschema.CloneToolInfos(entryTools), Extra: agentschema.CloneStringAnyMap(entryExtra),
		}, nextContext, streaming)
		if err != nil {
			return nil, err
		}
		return agent.freezeCompactionCall(next)
	}
	return &preparedModelCall{ctx: ctx, call: call, modelContext: modelContext, state: state}, nil
}

func (agent *modelToolLoop) beforeModelCall(ctx context.Context, call *modelCall, modelContext *modelStepContext) (context.Context, *modelCall, error) {
	var err error
	for _, middleware := range agent.middlewares {
		var rewritten *agentmodel.ModelCall
		request := call.request()
		ctx, rewritten, err = middleware.BeforeModelCall(ctx, &request, &modelContext.ModelContext)
		if err != nil {
			return ctx, nil, fmt.Errorf("before model call middleware: %w", err)
		}
		if ctx == nil {
			return nil, nil, errors.New("before model call middleware returned nil Go context")
		}
		if rewritten == nil || rewritten.Model == nil {
			return ctx, nil, errors.New("before model call middleware returned nil model call")
		}
		call.Model, call.Messages, call.Options, call.Streaming = rewritten.Model, rewritten.Messages, rewritten.Options, rewritten.Streaming
	}
	call.modelIdentity = agent.modelIdentity
	call.inputEstimator = agentmodel.InputEstimatorForModel(agent.model)
	if model, ok := call.Model.(agentmodel.DefinitionModel); ok {
		call.modelIdentity = model.ModelIdentity()
	}
	if model, ok := call.Model.(agentmodel.ModelInputEstimator); ok {
		call.inputEstimator = model.InputEstimator()
	}
	call.stablePrefixMessages = authenticatedStablePrefixMessages(call.Messages, modelContext.stablePrefixSeed)
	cachePrefix := resolvedCachePrefixMessages(call.Messages, call.stablePrefixMessages, modelContext.activeUserMessage)
	call.Options = append(call.Options, agentmodel.WithCachePrefixMessages(cachePrefix))
	return ctx, call, nil
}

// resolvedCachePrefixMessages returns the leading message count that stays
// byte-identical across turns. Messages strictly before the transient active
// user message (system, leading context, and committed prior turns) remain
// stable even after the active turn's wrapper is replaced with raw user text
// on the next turn.
func resolvedCachePrefixMessages(messages []*agentschema.Message, stablePrefix int, activeUser *agentschema.Message) int {
	prefix := min(max(0, stablePrefix), len(messages))
	if activeUser == nil {
		return prefix
	}
	for index := len(messages) - 1; index >= prefix; index-- {
		message := messages[index]
		if message == nil || message.Role != agentschema.User {
			continue
		}
		if equal, err := canonicalMessagesEqual(message, activeUser); err == nil && equal {
			return index
		}
	}
	return prefix
}

func (agent *modelToolLoop) freezeCompactionCall(step *preparedModelCall) (*preparedModelCall, error) {
	messages, err := projectToolArtifactPaths(step.ctx, agent.artifacts, step.call.Messages)
	if err != nil {
		return nil, err
	}
	step.call.providerMessages = messages
	return step, nil
}

// modelCall owns execution-only provenance independently of middleware's request.
type modelCall struct {
	Model                agentmodel.BaseChatModel
	Messages             []*agentschema.Message
	Options              []agentmodel.ModelOption
	Streaming            bool
	modelIdentity        agentschema.CapabilityIdentity
	inputEstimator       agentmodel.InputEstimator
	stablePrefixMessages int
	providerMessages     []*agentschema.Message
}

func (call *modelCall) Snapshot() *agentmodel.ModelRequestSnapshot {
	if call == nil {
		return nil
	}
	request := call.request()
	if call.providerMessages != nil {
		request.Messages = call.providerMessages
	}
	return agentmodel.CaptureModelRequest(&request, agentmodel.SnapshotMetadata{
		ModelIdentity: call.modelIdentity, InputEstimator: call.inputEstimator,
		StablePrefixMessages: call.stablePrefixMessages,
	})
}

func (call *modelCall) request() agentmodel.ModelCall {
	return agentmodel.ModelCall{Model: call.Model, Messages: call.Messages, Options: call.Options, Streaming: call.Streaming}
}
