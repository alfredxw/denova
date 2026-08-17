package agent

import (
	"context"
	"errors"
	"io"
)

// ToolArtifactRequest describes one immutable output stream stored outside
// model history. Extension is a safe filename hint, not a caller-controlled
// path.
type ToolArtifactRequest struct {
	ToolName string
	// Purpose declares whether this stream is the complete pre-projection model
	// output or an auxiliary attachment. Stores default an omitted purpose to the
	// conservative attachment contract.
	Purpose ToolArtifactPurpose
	// ToolCallID is the stable execution/call identity within the current
	// session. Stores use it for atomic idempotent publication.
	ToolCallID  string
	MIMEType    string
	Extension   string
	Description string
}

// ToolArtifactWriter is a transactional output stream. Commit makes the
// artifact visible and returns its stable reference; Abort removes a partial
// stream. Exactly one terminal method should be called.
type ToolArtifactWriter interface {
	io.Writer
	Commit() (ToolArtifactRef, error)
	Abort() error
}

// ToolArtifactStore starts immutable streams for complete tool outputs.
type ToolArtifactStore interface {
	BeginToolArtifact(context.Context, ToolArtifactRequest) (ToolArtifactWriter, error)
}

// ToolArtifactVerifier authenticates a published reference before Agent treats
// it as complete recoverable output. A reference supplied by a tool is never
// trusted solely because its fields look complete.
type ToolArtifactVerifier interface {
	VerifyToolArtifact(context.Context, ToolArtifactRef, ToolArtifactRequest) error
}

// ToolArtifactBackend is the host storage contract before Definition identity
// is attached. Keeping publication and verification in one contract prevents
// a store that can write artifacts from accidentally claiming that arbitrary
// tool-provided references are recoverable.
type ToolArtifactBackend interface {
	ToolArtifactStore
	ToolArtifactVerifier
}

// ToolArtifactStorage is the Definition-composable artifact capability. The
// stable identity describes storage semantics and scope derivation, not a raw
// filesystem path, credential, or process-local address.
type ToolArtifactStorage interface {
	ToolArtifactBackend
	Identity() CapabilityIdentity
}

type identifiedToolArtifactStorage struct {
	ToolArtifactBackend
	identity CapabilityIdentity
}

func (storage identifiedToolArtifactStorage) Identity() CapabilityIdentity { return storage.identity }

// IdentifyToolArtifactStorage binds a host store to a stable capability
// identity without forcing reusable storage implementations to own Definition
// concerns.
func IdentifyToolArtifactStorage(store ToolArtifactBackend, identity CapabilityIdentity) (ToolArtifactStorage, error) {
	if store == nil {
		return nil, errors.New("ToolArtifactStorage store is nil")
	}
	if err := identity.validate("ToolArtifactStorage"); err != nil {
		return nil, err
	}
	return identifiedToolArtifactStorage{ToolArtifactBackend: store, identity: identity}, nil
}

func identityOfToolArtifactStorage(storage ToolArtifactStorage) CapabilityIdentity {
	if storage == nil {
		return CapabilityIdentity{Kind: "tool_artifact_storage.none", Version: 1}
	}
	return storage.Identity()
}

type toolArtifactContextKey struct{}

type toolArtifactContext struct {
	store    ToolArtifactStore
	verifier ToolArtifactVerifier
}

func toolArtifactsFromContext(ctx context.Context) toolArtifactContext {
	if ctx == nil {
		return toolArtifactContext{}
	}
	access, _ := ctx.Value(toolArtifactContextKey{}).(toolArtifactContext)
	return access
}

// ContextWithToolArtifactStore binds run-scoped artifact persistence without
// coupling reusable tools to Denova's session storage implementation.
func ContextWithToolArtifactStore(ctx context.Context, store ToolArtifactStore) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	access := toolArtifactsFromContext(ctx)
	access.store = store
	return context.WithValue(ctx, toolArtifactContextKey{}, access)
}

// ContextWithToolArtifactVerifier explicitly binds reference authentication
// for direct ToolResultProcessor use. Definition-based runs bind their single
// ToolArtifactStorage authority automatically after BeforeAgent middleware.
func ContextWithToolArtifactVerifier(ctx context.Context, verifier ToolArtifactVerifier) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	access := toolArtifactsFromContext(ctx)
	access.verifier = verifier
	return context.WithValue(ctx, toolArtifactContextKey{}, access)
}

// ContextWithToolArtifactBackend binds publication and verification together
// for direct Tool/ToolResultProcessor execution outside a Definition. Normal
// Agent runs should configure Definition.Artifacts instead.
func ContextWithToolArtifactBackend(ctx context.Context, backend ToolArtifactBackend) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolArtifactContextKey{}, toolArtifactContext{
		store: backend, verifier: backend,
	})
}

func contextWithDefinitionToolArtifactStorage(ctx context.Context, storage ToolArtifactStorage) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return ContextWithToolArtifactBackend(ctx, storage)
}

// ToolArtifactStoreFromContext returns the optional run-scoped artifact store.
func ToolArtifactStoreFromContext(ctx context.Context) ToolArtifactStore {
	if ctx == nil {
		return nil
	}
	return toolArtifactsFromContext(ctx).store
}

// ToolArtifactVerifierFromContext returns the explicitly composed verifier.
// Result processors use this instead of guessing an optional method through a
// private type assertion.
func ToolArtifactVerifierFromContext(ctx context.Context) ToolArtifactVerifier {
	return toolArtifactsFromContext(ctx).verifier
}
