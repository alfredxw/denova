package external

import (
	"context"
	"errors"

	agentchat "denova/internal/agents/chat"
)

// Steering connects one live provider attempt to the application's durable
// input queue. It holds no queue or recovery state. Adapters choose their native
// delivery mechanism; product hosts prepare and commit accepted input.
type Steering struct {
	Changed   <-chan struct{}
	Next      func(context.Context) (Guidance, bool, error)
	Delivered func(Guidance)
	Interrupt func()
}

type Guidance struct {
	Request agentchat.ChatRequest
	Count   int
}

// PreparedSteer keeps product context and persistence outside provider adapters.
// Commit runs only after native acceptance, before subsequent tool callbacks.
type PreparedSteer struct {
	Input  Input
	Commit func(context.Context) error
}

type SteeringHost interface {
	PrepareSteer(context.Context, Guidance) (PreparedSteer, error)
}

type steeringContextKey struct{}

// WithSteering binds controls to the task context, never an HTTP request.
func WithSteering(ctx context.Context, controls *Steering) context.Context {
	return context.WithValue(ctx, steeringContextKey{}, controls)
}

func SteeringFromContext(ctx context.Context) *Steering {
	controls, _ := ctx.Value(steeringContextKey{}).(*Steering)
	return controls
}

// ErrSteerUnavailable means the targeted native turn already ended. The
// application retains the undelivered command for the next product cycle.
var ErrSteerUnavailable = errors.New("native turn is no longer available for steering")

func (controls *Steering) Deliver(ctx context.Context, host Host, send func(context.Context, Input) error) error {
	for {
		guidance, present, err := controls.Next(ctx)
		if err != nil || !present {
			return err
		}
		product, ok := host.(SteeringHost)
		if !ok {
			return errors.New("product does not support native steering")
		}
		prepared, err := product.PrepareSteer(ctx, guidance)
		if err != nil {
			return err
		}
		if err := send(ctx, prepared.Input); errors.Is(err, ErrSteerUnavailable) {
			return nil
		} else if err != nil {
			return err
		}
		if err := prepared.Commit(context.WithoutCancel(ctx)); err != nil {
			return err
		}
		controls.Delivered(guidance)
	}
}
