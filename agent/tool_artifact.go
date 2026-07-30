package agent

import (
	"context"
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

type toolArtifactStoreContextKey struct{}

// ContextWithToolArtifactStore binds run-scoped artifact persistence without
// coupling reusable tools to Denova's session storage implementation.
func ContextWithToolArtifactStore(ctx context.Context, store ToolArtifactStore) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		return ctx
	}
	return context.WithValue(ctx, toolArtifactStoreContextKey{}, store)
}

// ToolArtifactStoreFromContext returns the optional run-scoped artifact store.
func ToolArtifactStoreFromContext(ctx context.Context) ToolArtifactStore {
	if ctx == nil {
		return nil
	}
	store, _ := ctx.Value(toolArtifactStoreContextKey{}).(ToolArtifactStore)
	return store
}
