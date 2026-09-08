package tools

import (
	"context"
	"errors"
	"runtime"

	agent "github.com/alfredxw/denova/agent"
)

// Terminal task readers share cached Session handles. Another reader may close
// that handle while releasing its Store lease. Repeat the read from the durable
// journal through the opener; never retry mutations or other storage failures.
func retryClosedTaskRead[T any](ctx context.Context, read func() (T, error)) (T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			var zero T
			return zero, err
		}
		value, err := read()
		if !errors.Is(err, agent.ErrSessionClosed) {
			return value, err
		}
		// Let Close finish removing the old handle from the owner's registry.
		runtime.Gosched()
	}
}
