package agent

import "context"

type inspectionContextKey struct{}

// IsInspection reports whether Agent is preparing a read-only Session preview.
// Source, Toolset, Context, Goal, and Middleware implementations may use it to
// suppress optional telemetry, but must return the same model-visible result as
// normal preparation for the supplied snapshot.
func IsInspection(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	inspecting, _ := ctx.Value(inspectionContextKey{}).(bool)
	return inspecting
}

func contextWithInspection(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, inspectionContextKey{}, true)
}

// ModelRequestInspection is a detached, non-executable view of the exact
// provider-neutral request assembled by Session.Inspect. It intentionally does
// not expose the concrete model adapter or a Generate/Stream method.
type ModelRequestInspection struct {
	Messages             []*Message
	Options              Options
	Streaming            bool
	StablePrefixMessages int
}

// Inspection is a read-only preview of one prospective model step. The
// Definition identities and maintenance states identify the exact composition
// used to build ModelRequest without exposing Runtime or journal types.
type Inspection struct {
	Session SessionView
	// Run is an inspection-only identity used while materializing dynamic
	// capabilities. It is never accepted by Runtime and grants no authority.
	Run                     RunView
	DefinitionKey           string
	RestoreKey              string
	MaterializedFingerprint string
	PrefixFingerprint       string
	ModelIdentity           CapabilityIdentity
	Cleanup                 *CleanupState
	Compaction              *CompactionState
	// ContextFragments is the exact bounded provenance materialized by the
	// selected Definition before model middleware. ModelRequest remains the
	// sole provider-visible payload; diagnostics use these fragments to explain
	// which source contributed bytes without rebuilding host context.
	ContextFragments []ContextFragment
	ModelRequest     ModelRequestInspection
}

func modelRequestInspection(snapshot *ModelRequestSnapshot) ModelRequestInspection {
	if snapshot == nil {
		return ModelRequestInspection{}
	}
	options := snapshot.ResolvedOptions()
	return ModelRequestInspection{
		Messages:             snapshot.Messages(),
		Options:              *options,
		Streaming:            snapshot.Streaming(),
		StablePrefixMessages: snapshot.StablePrefixMessages(),
	}
}
