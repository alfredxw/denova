package statistics

import (
	"context"
	"log/slog"
)

type projectKey struct{}

// WithProject scopes numerical capture to the immutable Project Store. The
// host path stays in memory; it is never written into statistics records.
func WithProject(ctx context.Context, root string) context.Context {
	if root == "" {
		return ctx
	}
	return context.WithValue(ctx, projectKey{}, root)
}

func ProjectRoot(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	root, _ := ctx.Value(projectKey{}).(string)
	return root
}

// RecordCall is best effort: a statistics failure must not fail paid model
// work or a canonical product commit. Failures remain diagnosable in logs.
func RecordCall(root string, call Call) {
	if root == "" {
		return
	}
	if err := AppendCall(root, call); err != nil {
		slog.Error("Record model statistics failed", "call_id", call.ID, "error", err)
	}
}
