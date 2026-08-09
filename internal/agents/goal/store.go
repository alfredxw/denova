package goal

import "context"

// Store is the narrow persistence boundary used by the model-visible finish
// tool. Session implements it without exposing journal internals.
type Store interface {
	Goal(context.Context) (State, bool, error)
	FinishGoal(context.Context, string, uint64, Status, string) (State, error)
}
