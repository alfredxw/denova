package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// InvocationScope identifies one root or delegated Agent invocation. Provider
// tool-call IDs are only unique inside one model response; ToolNamespace gives
// durable hosts a stable namespace for nested calls while ID separates
// run-owned resources across repeated executions of the same Agent object.
type InvocationScope struct {
	ID            string
	Depth         int
	RunPath       []string
	ToolNamespace string
	OperationID   string
	Cycle         int
}

// InvocationIdentity is the caller-owned durable identity of one root model
// cycle. Scope identifies the durable runtime binding; OperationID and Cycle
// identify the exact recoverable cycle inside that binding. RunID is used only
// by hosts that do not have a durable operation coordinator.
type InvocationIdentity struct {
	Scope       string
	OperationID string
	Cycle       int
	RunID       string
}

type invocationContextValue struct {
	scope InvocationScope
	state *invocationResourceState
}

type invocationResource struct {
	value   any
	cleanup func(context.Context) error
}

type invocationResourceState struct {
	mu        sync.Mutex
	resources map[string]invocationResource
	order     []string
	closed    bool
	children  uint64
	responses uint64
}

type invocationScopeKey struct{}
type invocationIdentityKey struct{}

var invocationSequence atomic.Uint64

// BeginChildInvocation derives an isolated child scope from the current task
// tool call. The returned finish function must be called after the child event
// stream is drained so run-owned resources are released deterministically.
func BeginChildInvocation(ctx context.Context, childName string) (context.Context, func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	childName = strings.TrimSpace(childName)
	if childName == "" {
		return nil, nil, errors.New("child invocation requires an Agent name")
	}
	parent, ok := invocationValueFromContext(ctx)
	depth := 1
	runPath := []string{childName}
	parentNamespace := ""
	operationID := ""
	cycle := 0
	childOrdinal := uint64(1)
	if ok {
		depth = parent.scope.Depth + 1
		runPath = append(append([]string(nil), parent.scope.RunPath...), childName)
		parentNamespace = parent.scope.ToolNamespace
		operationID = parent.scope.OperationID
		cycle = parent.scope.Cycle
		parent.state.mu.Lock()
		parent.state.children++
		childOrdinal = parent.state.children
		parent.state.mu.Unlock()
	}
	delegationExecutionID := strings.TrimSpace(CurrentToolExecutionID(ctx))
	if delegationExecutionID == "" {
		delegationExecutionID = ToolExecutionID(ctx, ToolCallID(ctx))
	}
	namespace := childToolNamespace(parentNamespace, delegationExecutionID, childName, childOrdinal)
	return beginInvocation(ctx, InvocationScope{
		ID:            nextInvocationID(),
		Depth:         depth,
		RunPath:       runPath,
		ToolNamespace: namespace,
		OperationID:   operationID,
		Cycle:         cycle,
	})
}

func beginRootInvocation(ctx context.Context, agentName string) (context.Context, func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := invocationValueFromContext(ctx); ok {
		return ctx, func() error { return nil }, nil
	}
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		agentName = "agent"
	}
	identity, _ := InvocationIdentityFromContext(ctx)
	namespace := rootToolNamespace(identity, agentName)
	return beginInvocation(ctx, InvocationScope{
		ID: namespace, Depth: 0, RunPath: []string{agentName},
		ToolNamespace: namespace, OperationID: identity.OperationID, Cycle: identity.Cycle,
	})
}

func beginInvocation(ctx context.Context, scope InvocationScope) (context.Context, func() error, error) {
	state := &invocationResourceState{resources: make(map[string]invocationResource)}
	value := invocationContextValue{scope: cloneInvocationScope(scope), state: state}
	invocationCtx, cancel := context.WithCancel(context.WithValue(ctx, invocationScopeKey{}, value))
	var once sync.Once
	var closeErr error
	finish := func() error {
		once.Do(func() {
			cancel()
			closeErr = state.close(context.Background())
		})
		return closeErr
	}
	return invocationCtx, finish, nil
}

// InvocationScopeFromContext returns a caller-owned copy of the active scope.
func InvocationScopeFromContext(ctx context.Context) (InvocationScope, bool) {
	value, ok := invocationValueFromContext(ctx)
	if !ok {
		return InvocationScope{}, false
	}
	return cloneInvocationScope(value.scope), true
}

// IsRootInvocation reports whether host-owned transcript controls may run.
// Contexts without an explicit scope retain root behavior for direct callers.
func IsRootInvocation(ctx context.Context) bool {
	scope, ok := InvocationScopeFromContext(ctx)
	return !ok || scope.Depth == 0
}

// ContextWithInvocationIdentity binds the stable host identity used to derive
// tool execution IDs. Replaying the same durable cycle must bind the same
// identity; a different operation or cycle must bind a different identity.
func ContextWithInvocationIdentity(ctx context.Context, identity InvocationIdentity) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	identity.Scope = strings.TrimSpace(identity.Scope)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	identity.RunID = strings.TrimSpace(identity.RunID)
	if identity.Cycle < 0 {
		identity.Cycle = 0
	}
	return context.WithValue(ctx, invocationIdentityKey{}, identity)
}

// InvocationIdentityFromContext returns the caller-owned root identity.
func InvocationIdentityFromContext(ctx context.Context) (InvocationIdentity, bool) {
	if ctx == nil {
		return InvocationIdentity{}, false
	}
	identity, ok := ctx.Value(invocationIdentityKey{}).(InvocationIdentity)
	if !ok {
		return InvocationIdentity{}, false
	}
	return identity, identity.Scope != "" || identity.OperationID != "" || identity.RunID != ""
}

// ToolExecutionID returns the current durable execution ID when called inside
// a native tool. The provider call ID is accepted only to support direct tool
// callers and older middleware; provider IDs never participate in native ID
// generation because providers may reuse them across model responses.
func ToolExecutionID(ctx context.Context, providerCallID string) string {
	providerCallID = strings.TrimSpace(providerCallID)
	if metadata, ok := toolCallMetadata(ctx); ok && metadata.executionID != "" {
		if providerCallID == "" || metadata.providerCallID == "" || providerCallID == metadata.providerCallID {
			return metadata.executionID
		}
	}
	if providerCallID == "" {
		return ""
	}
	scope, ok := InvocationScopeFromContext(ctx)
	if !ok || strings.TrimSpace(scope.ToolNamespace) == "" {
		return providerCallID
	}
	// Direct callers do not provide a model-response/tool ordinal. Keep this
	// compatibility path isolated from native execution identities.
	return hashedExecutionID("direct", scope.ToolNamespace, providerCallID)
}

// ToolExecutionIDForOrdinal derives one provider-independent execution ID.
// modelResponseOrdinal and toolOrdinal are owned by the native loop, making
// calls unique even when a provider reuses the same call ID.
func ToolExecutionIDForOrdinal(ctx context.Context, modelResponseOrdinal, toolOrdinal int) string {
	scope, ok := InvocationScopeFromContext(ctx)
	if !ok || strings.TrimSpace(scope.ToolNamespace) == "" || modelResponseOrdinal <= 0 || toolOrdinal < 0 {
		return ""
	}
	return executionIDForNamespace(scope.ToolNamespace, modelResponseOrdinal, toolOrdinal)
}

// CurrentToolExecutionID returns the durable ID of the active native call.
func CurrentToolExecutionID(ctx context.Context) string {
	metadata, _ := toolCallMetadata(ctx)
	return metadata.executionID
}

// nextModelResponseOrdinal is intentionally invocation-local. Durable hosts
// may replay an exact cycle from ordinal one, but must never resume midway
// without restoring the ordinal; Denova's runtime pauses crashed cycles and
// continues user input in cycle+1.
func nextModelResponseOrdinal(ctx context.Context) int {
	value, ok := invocationValueFromContext(ctx)
	if !ok || value.state == nil {
		return 0
	}
	value.state.mu.Lock()
	defer value.state.mu.Unlock()
	value.state.responses++
	return int(value.state.responses)
}

// InvocationResource returns one resource per invocation and key. Creation is
// serialized so stateful tools cannot accidentally publish two owners during
// concurrent calls. cleanup runs once when the invocation finishes.
func InvocationResource[T any](
	ctx context.Context,
	key string,
	create func(context.Context) (T, func(context.Context) error, error),
) (T, error) {
	var zero T
	key = strings.TrimSpace(key)
	if key == "" {
		return zero, errors.New("invocation resource key is required")
	}
	if create == nil {
		return zero, errors.New("invocation resource factory is required")
	}
	value, ok := invocationValueFromContext(ctx)
	if !ok || value.state == nil {
		return zero, errors.New("invocation resource requires an active Agent invocation")
	}
	value.state.mu.Lock()
	defer value.state.mu.Unlock()
	if value.state.closed {
		return zero, errors.New("Agent invocation is already closed")
	}
	if existing, exists := value.state.resources[key]; exists {
		resource, valid := existing.value.(T)
		if !valid {
			return zero, fmt.Errorf("invocation resource %q has an incompatible type", key)
		}
		return resource, nil
	}
	resource, cleanup, err := create(ctx)
	if err != nil {
		return zero, err
	}
	value.state.resources[key] = invocationResource{value: resource, cleanup: cleanup}
	value.state.order = append(value.state.order, key)
	return resource, nil
}

func (state *invocationResourceState) close(ctx context.Context) error {
	if state == nil {
		return nil
	}
	state.mu.Lock()
	if state.closed {
		state.mu.Unlock()
		return nil
	}
	state.closed = true
	resources := state.resources
	order := append([]string(nil), state.order...)
	state.resources = nil
	state.order = nil
	state.mu.Unlock()

	var closeErrors []error
	for index := len(order) - 1; index >= 0; index-- {
		resource := resources[order[index]]
		if resource.cleanup == nil {
			continue
		}
		if err := resource.cleanup(ctx); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close invocation resource %q: %w", order[index], err))
		}
	}
	return errors.Join(closeErrors...)
}

func invocationValueFromContext(ctx context.Context) (invocationContextValue, bool) {
	if ctx == nil {
		return invocationContextValue{}, false
	}
	value, ok := ctx.Value(invocationScopeKey{}).(invocationContextValue)
	return value, ok && value.state != nil
}

func nextInvocationID() string {
	return fmt.Sprintf("inv-%016x", invocationSequence.Add(1))
}

func rootToolNamespace(identity InvocationIdentity, agentName string) string {
	operationID := strings.TrimSpace(identity.OperationID)
	cycle := identity.Cycle
	if operationID != "" && cycle > 0 {
		return hashedExecutionID("inv", identity.Scope, operationID, fmt.Sprintf("cycle:%d", cycle), agentName)
	}
	if runID := strings.TrimSpace(identity.RunID); runID != "" {
		return hashedExecutionID("inv", identity.Scope, "run:"+runID, agentName)
	}
	// Standalone Agent consumers have no durable host operation. A monotonic
	// process identity still prevents collisions without introducing randomness;
	// durable hosts must bind InvocationIdentity above.
	return hashedExecutionID("inv", "standalone", nextInvocationID(), agentName)
}

func childToolNamespace(parentNamespace, delegationExecutionID, childName string, ordinal uint64) string {
	return hashedExecutionID("inv", strings.TrimSpace(parentNamespace),
		strings.TrimSpace(delegationExecutionID), strings.TrimSpace(childName), fmt.Sprintf("child:%d", ordinal))
}

func executionIDForNamespace(namespace string, modelResponseOrdinal, toolOrdinal int) string {
	return hashedExecutionID("tool", strings.TrimSpace(namespace),
		fmt.Sprintf("response:%d", modelResponseOrdinal), fmt.Sprintf("tool:%d", toolOrdinal))
}

func hashedExecutionID(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + "-" + hex.EncodeToString(digest[:16])
}

func cloneInvocationScope(scope InvocationScope) InvocationScope {
	scope.RunPath = append([]string(nil), scope.RunPath...)
	return scope
}
