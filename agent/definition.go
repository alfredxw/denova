package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// CapabilityIdentity is stable across process restarts. ConfigHash contains
// only canonical behavior configuration; credentials and process addresses
// must never be included.
type CapabilityIdentity struct {
	Kind       string `json:"kind"`
	Version    uint16 `json:"version"`
	ConfigHash string `json:"config_hash,omitempty"`
}

func (identity CapabilityIdentity) validate(name string) error {
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return fmt.Errorf("%s capability identity is incomplete", name)
	}
	return nil
}

// Toolset materializes the immutable tool registry for one model cycle.
type Toolset interface {
	Identity() CapabilityIdentity
	PrepareTools(context.Context, ToolRequest) ([]ToolDefinition, error)
}

type ToolRequest struct {
	Session SessionView
	Run     RunView
	Input   Input
}

// ContextSource returns accountable model-visible fragments for one cycle.
type ContextSource interface {
	Identity() CapabilityIdentity
	Materialize(context.Context, ContextRequest) ([]ContextFragment, error)
}

type ContextRequest struct {
	Session SessionView
	Run     RunView
	Input   Input
	// Compaction is the current Agent-owned checkpoint. ContextSource may use
	// its opaque ContextData to project a bounded host-owned context.
	Compaction *CompactionState
}

type ContextPlacement string

const (
	ContextLeadingMessage ContextPlacement = "leading_message"
	// ContextStateMessage appends a durable, model-visible state update to the
	// raw transcript. Agent emits a new message only when the named state
	// changes, is removed, or must be restored after Compaction.
	ContextStateMessage         ContextPlacement = "state_message"
	ContextCompactionCheckpoint ContextPlacement = "compaction_checkpoint"
	ContextFinalUserPrefix      ContextPlacement = "final_user_prefix"
	// ContextFinalUserMessage replaces the model-visible user message for this
	// cycle. The raw Input remains durable and is still passed to Canonical.
	// This placement lets hosts preserve localized, audited context renderers
	// without moving their presentation policy into the Agent module.
	ContextFinalUserMessage ContextPlacement = "final_user_message"
	ContextAuditOnly        ContextPlacement = "audit_only"
)

// ContextStability defines the lifecycle of a model-visible fragment
// independently from where it is rendered. Keeping this contract explicit
// prevents mutable state from accidentally invalidating a stable cache prefix.
type ContextStability string

const (
	ContextStablePrefix ContextStability = "stable_prefix"
	ContextSessionState ContextStability = "session_state"
	ContextTurn         ContextStability = "turn"
	ContextCheckpoint   ContextStability = "checkpoint"
	ContextAudit        ContextStability = "audit"
)

// ContextRendering controls only the model-visible wrapper. Provenance and
// bounds remain mandatory for both modes.
type ContextRendering string

const (
	ContextRenderAttributed ContextRendering = "attributed"
	ContextRenderVerbatim   ContextRendering = "verbatim"
)

// ContextFragment makes the provenance and hard bound of every injected byte
// explicit. Content over HardLimit is rejected instead of silently truncated.
type ContextFragment struct {
	Source   string
	Purpose  string
	Resource string
	Revision string
	// StateID is required for session_state fragments and must remain stable
	// across revisions of the same logical state section.
	StateID   string
	Stability ContextStability
	Placement ContextPlacement
	Rendering ContextRendering
	// Role applies to leading messages. Empty selects System; User is useful
	// for hosts whose stable cache prefix is intentionally represented as a
	// contextual user message.
	Role      RoleType
	Content   string
	HardLimit int
}

type TurnReason string

const (
	TurnReasonStart        TurnReason = "start"
	TurnReasonSteer        TurnReason = "steer"
	TurnReasonFollowUp     TurnReason = "follow_up"
	TurnReasonNextTurn     TurnReason = "next_turn"
	TurnReasonInteraction  TurnReason = "interaction"
	TurnReasonGoalMutation TurnReason = "goal_mutation"
	TurnReasonStructural   TurnReason = "structural"
)

// TurnDelivery identifies how the current input entered a Run. Reason describes
// why Source is being called, including interaction and structural cycles that
// do not admit a new input.
type TurnDelivery string

const (
	TurnDeliveryStart    TurnDelivery = "start"
	TurnDeliverySteer    TurnDelivery = "steer"
	TurnDeliveryFollowUp TurnDelivery = "follow_up"
	TurnDeliveryNextTurn TurnDelivery = "next_turn"
)

type SessionView struct {
	Key      agentsession.Key
	Revision uint64
}

type RunView struct {
	ID        string
	CommandID string
	Cycle     int
	// StartedAt is captured once when this cycle starts. Context sources should
	// use it instead of reading the wall clock while assembling one request.
	StartedAt  time.Time
	Delivery   TurnDelivery
	Autonomous bool
}

type PrepareRequest struct {
	Session SessionView
	Run     RunView
	Input   Input
	Reason  TurnReason

	DefinitionKey string
	BehaviorKey   string
	HostData      *HostData
	Compaction    *CompactionState
	Cleanup       *CleanupState
}

// Source prepares a complete immutable Definition for one cycle. CanonicalInput
// is the deliberately narrow, provider-free admission phase: Agent invokes it
// after accepting the Input and before any model/context/tool preparation.
// Implementations must resolve only the CanonicalAdapter needed to persist the
// accepted user input; it must not assemble model context or call a provider.
type Source interface {
	Prepare(context.Context, PrepareRequest) (Definition, error)
	CanonicalInput(context.Context, PrepareRequest) (CanonicalAdapter, error)
}

type SourceFunc func(context.Context, PrepareRequest) (Definition, error)

func (prepare SourceFunc) Prepare(ctx context.Context, request PrepareRequest) (Definition, error) {
	if prepare == nil {
		return Definition{}, errors.New("agent Definition Source is nil")
	}
	return prepare(ctx, request)
}

// CanonicalInput makes SourceFunc the simple no-product-canonical composition
// helper. Dynamic hosts that return Definition.Canonical must implement Source
// directly so accepted input can cross the canonical barrier before Prepare.
func (SourceFunc) CanonicalInput(context.Context, PrepareRequest) (CanonicalAdapter, error) {
	return nil, nil
}

// DefinitionInitializer is implemented by declarative capabilities whose
// construction can fail. Agent initializes every such capability before it
// accepts a static Definition; dynamic Sources receive the same guarantee when
// their Definition is prepared. Implementations must be idempotent and safe to
// call more than once.
type DefinitionInitializer interface {
	InitializeDefinition(context.Context) error
}

// Definition is the complete composition root for one Agent cycle.
type Definition struct {
	// Key lets a dynamic Source find the same business configuration again.
	// Behavior and prefix identities are always derived by Agent.
	Key string

	Name          string
	Description   string
	Model         BaseChatModel
	ModelIdentity CapabilityIdentity
	Instructions  string
	// AttachmentRoot is the current host's absolute owner root for durable
	// slash-relative Attachment paths. It is runtime routing, not behavior
	// identity, and is therefore excluded from Definition fingerprints.
	AttachmentRoot string

	Tools Toolset
	// ResultProcessor is the single fixed post-tool projection authority. It
	// runs outside host middleware so every tool path receives identical
	// materialization, result projection, cleanup, and transcript semantics.
	ResultProcessor ToolResultProcessor
	// Artifacts is used both by streaming tools and ResultProcessor. Its stable
	// identity participates in behavior validation, never the model prefix fingerprint.
	Artifacts   ToolArtifactStorage
	Context     ContextSource
	Goal        GoalManager
	Cleanup     CleanupManager
	Compaction  CompactionManager
	Permission  PermissionPolicy
	Interaction InteractionPolicy
	Canonical   CanonicalAdapter

	Middlewares []Middleware
	Execution   ExecutionPolicy
}

// IdentifiedMiddleware gives behavior-changing middleware a stable identity
// for Definition validation and traceability.
type IdentifiedMiddleware interface {
	Middleware
	Identity() CapabilityIdentity
}

type identifiedMiddleware struct {
	Middleware
	identity CapabilityIdentity
}

func (middleware identifiedMiddleware) Identity() CapabilityIdentity { return middleware.identity }

func (middleware identifiedMiddleware) unwrap() Middleware { return middleware.Middleware }

// IdentifyMiddleware associates a stable behavior identity with an existing
// Middleware. It is useful for host-owned middleware whose concrete type is
// intentionally unaware of Agent Sessions.
func IdentifyMiddleware(middleware Middleware, identity CapabilityIdentity) IdentifiedMiddleware {
	if middleware == nil {
		return nil
	}
	return identifiedMiddleware{Middleware: middleware, identity: identity}
}

// MiddlewareImplementation returns the concrete host middleware beneath an
// identity decorator. It is intended for code that still needs concrete-type
// diagnostics; Agent execution itself never unwraps middleware.
func MiddlewareImplementation(middleware Middleware) Middleware {
	for middleware != nil {
		wrapped, ok := middleware.(interface{ unwrap() Middleware })
		if !ok {
			return middleware
		}
		next := wrapped.unwrap()
		if next == middleware {
			return middleware
		}
		middleware = next
	}
	return nil
}

func (definition Definition) Prepare(context.Context, PrepareRequest) (Definition, error) {
	return definition, nil
}

func (definition Definition) CanonicalInput(context.Context, PrepareRequest) (CanonicalAdapter, error) {
	return definition.Canonical, nil
}

type ExecutionPolicy struct {
	Retry *RetryConfig
	// RetryIdentity gives retry behavior a stable, inspectable identity when
	// Retry is non-nil. Function and closure addresses are not identities.
	RetryIdentity   CapabilityIdentity
	ToolParallelism int
	MaxIterations   int
	// IdleTimeout limits only a continuous period with no model chunk, tool
	// lifecycle/progress event, or Interaction request. Zero means unlimited;
	// it is never interpreted as a total run deadline.
	IdleTimeout time.Duration
	// MaxAutomaticCompactionFailures opens the Session failure fuse for one
	// unchanged final model request. Zero uses the Agent default. A changed
	// request automatically gets a fresh attempt; explicit Compact calls are
	// never blocked by this policy.
	MaxAutomaticCompactionFailures int
}

func validateDefinition(definition Definition) error {
	if definition.Model == nil {
		return errors.New("agent Definition Model is required")
	}
	if definition.Execution.MaxIterations < 0 {
		return errors.New("agent Definition MaxIterations cannot be negative")
	}
	if definition.Execution.IdleTimeout < 0 {
		return errors.New("agent Definition IdleTimeout cannot be negative")
	}
	if definition.Execution.MaxAutomaticCompactionFailures < 0 {
		return errors.New("agent Definition MaxAutomaticCompactionFailures cannot be negative")
	}
	if strings.TrimSpace(definition.Key) != definition.Key {
		return errors.New("agent Definition Key cannot contain surrounding whitespace")
	}
	if definition.Compaction != nil && definition.Compaction.SummaryLimitBytes() <= 0 {
		return errors.New("agent Definition Compaction summary limit must be positive")
	}
	if definition.Cleanup != nil {
		if err := definition.Cleanup.Identity().validate("Cleanup"); err != nil {
			return err
		}
	}
	if definition.ResultProcessor != nil {
		if err := definition.ResultProcessor.Identity().validate("ToolResultProcessor"); err != nil {
			return err
		}
	}
	if definition.Artifacts != nil {
		if err := definition.Artifacts.Identity().validate("ToolArtifactStorage"); err != nil {
			return err
		}
	}
	return nil
}

func initializeDefinition(ctx context.Context, definition Definition) (Definition, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	type component struct {
		name  string
		value any
	}
	components := []component{
		{name: "Model", value: definition.Model},
		{name: "Tools", value: definition.Tools},
		{name: "ToolResultProcessor", value: definition.ResultProcessor},
		{name: "ToolArtifactStorage", value: definition.Artifacts},
		{name: "Context", value: definition.Context},
		{name: "Goal", value: definition.Goal},
		{name: "Cleanup", value: definition.Cleanup},
		{name: "Compaction", value: definition.Compaction},
		{name: "Permission", value: definition.Permission},
		{name: "Interaction", value: definition.Interaction},
		{name: "Canonical", value: definition.Canonical},
	}
	for index, middleware := range definition.Middlewares {
		components = append(components, component{
			name: fmt.Sprintf("Middleware[%d]", index), value: middleware,
		})
	}
	var initializationErrors []error
	for _, candidate := range components {
		initializer, ok := candidate.value.(DefinitionInitializer)
		if !ok {
			continue
		}
		if err := initializer.InitializeDefinition(ctx); err != nil {
			initializationErrors = append(initializationErrors, fmt.Errorf("%s: %w", candidate.name, err))
		}
	}
	if err := errors.Join(initializationErrors...); err != nil {
		return Definition{}, fmt.Errorf("initialize agent Definition: %w", err)
	}
	if model, ok := definition.Model.(DefinitionModel); ok {
		identity := model.ModelIdentity()
		if err := identity.validate("Model"); err != nil {
			return Definition{}, err
		}
		if definition.ModelIdentity == (CapabilityIdentity{}) {
			definition.ModelIdentity = identity
		} else if definition.ModelIdentity != identity {
			return Definition{}, errors.New("agent Definition ModelIdentity does not match declarative Model")
		}
	}
	if err := validateDefinition(definition); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

type preparedDefinition struct {
	definition              Definition
	tools                   []ToolDefinition
	toolSnapshots           []ToolDefinitionSnapshot
	fragments               []ContextFragment
	goalFragments           []ContextFragment
	goalReservedTokens      int
	definitionKey           string
	behaviorKey             string
	prefixFingerprint       string
	materializedFingerprint string
	definitionOperationID   string
	definitionCommandID     string
	definitionCycle         int
	preparationStage        enginePreparationStage
	hostData                *HostData
	clearRevision           uint64
	contextState            contextStateSnapshot
}

func prepareDefinition(
	ctx context.Context,
	source Source,
	request PrepareRequest,
) (preparedDefinition, error) {
	prepared, err := prepareDefinitionBase(ctx, source, request)
	if err != nil {
		return preparedDefinition{}, err
	}
	if err := materializeDefinitionCapabilities(ctx, request, &prepared); err != nil {
		return preparedDefinition{}, err
	}
	return prepared, nil
}

// prepareDefinitionBase resolves the immutable composition and its identity
// without materializing tool or context capabilities. Agent uses this phase to
// fence the exact Definition and canonicalize accepted input before any
// product ContextSource reads the resulting state.
func prepareDefinitionBase(
	ctx context.Context,
	source Source,
	request PrepareRequest,
) (preparedDefinition, error) {
	definition, err := source.Prepare(ctx, request)
	if err != nil {
		return preparedDefinition{}, fmt.Errorf("prepare agent Definition: %w", err)
	}
	definition, err = initializeDefinition(ctx, definition)
	if err != nil {
		return preparedDefinition{}, err
	}
	behaviorKey, err := definitionBehaviorIdentity(definition)
	if err != nil {
		return preparedDefinition{}, err
	}
	definitionKey := strings.TrimSpace(definition.Key)
	if definitionKey == "" {
		definitionKey = behaviorKey
	}
	return preparedDefinition{
		definition: definition, definitionKey: definitionKey, behaviorKey: behaviorKey,
	}, nil
}

// DefinitionBehaviorIdentity returns the stable behavior identity used by
// Agent before materializing tools or Context. Composition catalogs may embed
// it to fence a child Definition without duplicating the lifecycle's identity
// vocabulary. It hashes behavior only; credentials and process addresses must
// stay out of every CapabilityIdentity supplied by the caller.
func DefinitionBehaviorIdentity(definition Definition) (string, error) {
	initialized, err := initializeDefinition(context.Background(), definition)
	if err != nil {
		return "", err
	}
	return definitionBehaviorIdentity(initialized)
}

func definitionBehaviorIdentity(definition Definition) (string, error) {
	return hashCanonical(definitionIdentity{
		DefinitionKey: definition.Key,
		Name:          definition.Name, Model: definition.ModelIdentity,
		Instructions: definition.Instructions, Execution: identityOfExecution(definition.Execution),
		Toolset: identityOfToolset(definition.Tools), ResultProcessor: identityOfToolResultProcessor(definition.ResultProcessor),
		Artifacts: identityOfToolArtifactStorage(definition.Artifacts), Context: identityOfContext(definition.Context),
		Goal: identityOfGoal(definition.Goal), Cleanup: identityOfCleanup(definition.Cleanup), Compaction: identityOfCompaction(definition.Compaction),
		Permission: identityOfPermission(definition.Permission), Interaction: identityOfInteraction(definition.Interaction),
		Canonical:   identityOfCanonical(definition.Canonical),
		Middlewares: middlewareIdentities(definition.Middlewares),
	})
}

func materializeDefinitionCapabilities(
	ctx context.Context,
	request PrepareRequest,
	prepared *preparedDefinition,
) error {
	if prepared == nil {
		return errors.New("materialize agent Definition capabilities: prepared Definition is nil")
	}
	definition := prepared.definition
	var tools []ToolDefinition
	var err error
	if definition.Tools != nil {
		tools, err = definition.Tools.PrepareTools(ctx, ToolRequest{
			Session: request.Session, Run: request.Run, Input: request.Input,
		})
		if err != nil {
			return fmt.Errorf("prepare agent Toolset: %w", err)
		}
	}
	registry, err := NewRegistry(ctx, tools...)
	if err != nil {
		return fmt.Errorf("prepare agent Toolset: %w", err)
	}
	prepared.tools = registry.Definitions()
	prepared.toolSnapshots = registry.Snapshots()

	var fragments []ContextFragment
	if definition.Context != nil {
		fragments, err = definition.Context.Materialize(ctx, ContextRequest{
			Session: request.Session, Run: request.Run, Input: request.Input,
			Compaction: cloneCompactionState(request.Compaction),
		})
		if err != nil {
			return fmt.Errorf("materialize agent Context: %w", err)
		}
	}
	fragments = append(fragments, request.Input.Context...)
	if err := validateContextFragments(fragments); err != nil {
		return err
	}
	prepared.fragments = fragments
	prepared.goalFragments = nil
	prepared.goalReservedTokens = 0
	return updatePreparedPrefixFingerprint(prepared)
}

func rematerializeDefinitionContext(
	ctx context.Context,
	request PrepareRequest,
	prepared *preparedDefinition,
) error {
	if prepared == nil {
		return errors.New("rematerialize agent Context: prepared Definition is nil")
	}
	var fragments []ContextFragment
	var err error
	if prepared.definition.Context != nil {
		fragments, err = prepared.definition.Context.Materialize(ctx, ContextRequest{
			Session: request.Session, Run: request.Run, Input: request.Input,
			Compaction: cloneCompactionState(request.Compaction),
		})
		if err != nil {
			return fmt.Errorf("rematerialize agent Context: %w", err)
		}
	}
	fragments = append(fragments, request.Input.Context...)
	fragments = append(fragments, prepared.goalFragments...)
	if err := validateContextFragments(fragments); err != nil {
		return err
	}
	prepared.fragments = fragments
	return updatePreparedPrefixFingerprint(prepared)
}

func updatePreparedPrefixFingerprint(prepared *preparedDefinition) error {
	if prepared == nil {
		return errors.New("fingerprint prepared Definition: prepared Definition is nil")
	}
	stableFragments := make([]ContextFragment, 0, len(prepared.fragments))
	for _, fragment := range prepared.fragments {
		if fragment.Placement == ContextLeadingMessage {
			stableFragments = append(stableFragments, fragment)
		}
	}
	var err error
	prepared.prefixFingerprint, err = hashCanonical(struct {
		Model        CapabilityIdentity
		Instructions string
		Tools        []ToolDefinitionSnapshot
		Context      []contextFragmentIdentity
		Middlewares  []CapabilityIdentity
	}{
		prepared.definition.ModelIdentity, prepared.definition.Instructions,
		prepared.toolSnapshots, contextFragmentIdentities(stableFragments),
		middlewareIdentities(prepared.definition.Middlewares),
	})
	return err
}

type definitionIdentity struct {
	DefinitionKey   string
	Name            string
	Model           CapabilityIdentity
	Instructions    string
	Execution       executionPolicyIdentity
	Toolset         CapabilityIdentity
	ResultProcessor CapabilityIdentity
	Artifacts       CapabilityIdentity
	Context         CapabilityIdentity
	Goal            CapabilityIdentity
	Cleanup         CapabilityIdentity
	Compaction      CapabilityIdentity
	Permission      CapabilityIdentity
	Interaction     CapabilityIdentity
	Canonical       CapabilityIdentity
	Middlewares     []CapabilityIdentity
}

type executionPolicyIdentity struct {
	Retry                          CapabilityIdentity
	ToolParallelism                int
	MaxIterations                  int
	IdleTimeout                    time.Duration
	MaxAutomaticCompactionFailures int
}

func identityOfExecution(policy ExecutionPolicy) executionPolicyIdentity {
	retry := policy.RetryIdentity
	if policy.Retry == nil {
		retry = CapabilityIdentity{Kind: "retry.none", Version: 1}
	}
	return executionPolicyIdentity{
		Retry: retry, ToolParallelism: policy.ToolParallelism, MaxIterations: policy.MaxIterations,
		IdleTimeout:                    policy.IdleTimeout,
		MaxAutomaticCompactionFailures: normalizedAutomaticCompactionFailureLimit(policy),
	}
}

func middlewareIdentities(middlewares []Middleware) []CapabilityIdentity {
	identities := make([]CapabilityIdentity, len(middlewares))
	for index, middleware := range middlewares {
		if identified, ok := middleware.(IdentifiedMiddleware); ok {
			identities[index] = identified.Identity()
		}
	}
	return identities
}

type contextFragmentIdentity struct {
	Source, Purpose, Resource, Revision string
	StateID                             string
	Stability                           ContextStability
	Placement                           ContextPlacement
	Rendering                           ContextRendering
	Role                                RoleType
	ContentHash                         string
	Bytes                               int
}

func contextFragmentIdentities(fragments []ContextFragment) []contextFragmentIdentity {
	identities := make([]contextFragmentIdentity, 0, len(fragments))
	for _, fragment := range fragments {
		hash := sha256.Sum256([]byte(fragment.Content))
		identities = append(identities, contextFragmentIdentity{
			Source: fragment.Source, Purpose: fragment.Purpose, Resource: fragment.Resource,
			StateID: fragment.StateID, Stability: fragment.Stability,
			Revision: fragment.Revision, Placement: fragment.Placement, Rendering: effectiveContextRendering(fragment.Rendering),
			Role:        effectiveContextRole(fragment),
			ContentHash: hex.EncodeToString(hash[:]), Bytes: len(fragment.Content),
		})
	}
	return identities
}

func identityOfToolset(toolset Toolset) CapabilityIdentity {
	if toolset == nil {
		return CapabilityIdentity{Kind: "tools.none", Version: 1}
	}
	return toolset.Identity()
}

func identityOfContext(source ContextSource) CapabilityIdentity {
	if source == nil {
		return CapabilityIdentity{Kind: "context.none", Version: 1}
	}
	return source.Identity()
}

func identityOfCanonical(adapter CanonicalAdapter) CapabilityIdentity {
	if adapter == nil {
		return CapabilityIdentity{Kind: "canonical.none", Version: 1}
	}
	return adapter.Identity()
}

func identityOfGoal(manager GoalManager) CapabilityIdentity {
	if manager == nil {
		return CapabilityIdentity{Kind: "goal.none", Version: 1}
	}
	return manager.Identity()
}

func identityOfCompaction(manager CompactionManager) CapabilityIdentity {
	if manager == nil {
		return CapabilityIdentity{Kind: "compaction.none", Version: 1}
	}
	return manager.Identity()
}

func identityOfCleanup(manager CleanupManager) CapabilityIdentity {
	if manager == nil {
		return CapabilityIdentity{Kind: "cleanup.none", Version: 1}
	}
	return manager.Identity()
}

func identityOfPermission(policy PermissionPolicy) CapabilityIdentity {
	if policy == nil {
		return CapabilityIdentity{Kind: "permission.safe_default", Version: 1}
	}
	return policy.Identity()
}

func identityOfInteraction(policy InteractionPolicy) CapabilityIdentity {
	return effectiveInteractionPolicy(policy).Identity()
}

func hashCanonical(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode agent identity: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func validateContextFragments(fragments []ContextFragment) error {
	finalUserMessages := 0
	finalUserPrefixes := 0
	for index, fragment := range fragments {
		if strings.TrimSpace(fragment.Source) == "" || strings.TrimSpace(fragment.Purpose) == "" ||
			strings.TrimSpace(fragment.Resource) == "" || fragment.HardLimit <= 0 {
			return fmt.Errorf("agent Context fragment %d requires source, purpose, resource, and HardLimit", index)
		}
		if len(fragment.Content) > fragment.HardLimit {
			return fmt.Errorf("agent Context fragment %d exceeds its %d-byte hard limit", index, fragment.HardLimit)
		}
		switch fragment.Placement {
		case ContextLeadingMessage, ContextStateMessage, ContextCompactionCheckpoint, ContextAuditOnly:
		case ContextFinalUserPrefix:
			finalUserPrefixes++
		case ContextFinalUserMessage:
			finalUserMessages++
		default:
			return fmt.Errorf("agent Context fragment %d has invalid placement %q", index, fragment.Placement)
		}
		switch fragment.Stability {
		case ContextStablePrefix:
			if fragment.Placement != ContextLeadingMessage {
				return fmt.Errorf("agent Context fragment %d stable_prefix requires leading_message placement", index)
			}
		case ContextSessionState:
			if fragment.Placement != ContextStateMessage || strings.TrimSpace(fragment.StateID) == "" {
				return fmt.Errorf("agent Context fragment %d session_state requires state_message placement and StateID", index)
			}
		case ContextTurn:
			if fragment.Placement != ContextFinalUserPrefix && fragment.Placement != ContextFinalUserMessage {
				return fmt.Errorf("agent Context fragment %d turn stability requires final user placement", index)
			}
		case ContextCheckpoint:
			if fragment.Placement != ContextCompactionCheckpoint {
				return fmt.Errorf("agent Context fragment %d checkpoint stability requires compaction_checkpoint placement", index)
			}
		case ContextAudit:
			if fragment.Placement != ContextAuditOnly {
				return fmt.Errorf("agent Context fragment %d audit stability requires audit_only placement", index)
			}
		default:
			return fmt.Errorf("agent Context fragment %d requires an explicit stability", index)
		}
		switch effectiveContextRendering(fragment.Rendering) {
		case ContextRenderAttributed, ContextRenderVerbatim:
		default:
			return fmt.Errorf("agent Context fragment %d has invalid rendering %q", index, fragment.Rendering)
		}
		role := effectiveContextRole(fragment)
		if fragment.Placement == ContextLeadingMessage {
			if role != System && role != User {
				return fmt.Errorf("agent Context fragment %d has invalid leading message role %q", index, fragment.Role)
			}
		} else if fragment.Role != "" {
			return fmt.Errorf("agent Context fragment %d sets a role outside leading_message placement", index)
		}
	}
	stateIDs := make(map[string]int)
	for index, fragment := range fragments {
		if fragment.Placement != ContextStateMessage {
			continue
		}
		if previous, exists := stateIDs[fragment.StateID]; exists {
			return fmt.Errorf("agent Context fragments %d and %d reuse StateID %q", previous, index, fragment.StateID)
		}
		stateIDs[fragment.StateID] = index
	}
	if finalUserMessages > 1 {
		return errors.New("agent Context permits at most one final user message")
	}
	if finalUserMessages > 0 && finalUserPrefixes > 0 {
		return errors.New("agent Context final user message cannot be combined with final user prefixes")
	}
	return nil
}

func effectiveContextRole(fragment ContextFragment) RoleType {
	if fragment.Role == "" {
		return System
	}
	return fragment.Role
}

func effectiveContextRendering(rendering ContextRendering) ContextRendering {
	if rendering == "" {
		return ContextRenderAttributed
	}
	return rendering
}
