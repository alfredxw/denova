package adk

import (
	"context"
	"sync/atomic"
)

const (
	asyncCallActive uint32 = iota
	asyncCallAbandoned
	asyncCallCompleted
)

type asyncCallResult[T any] struct {
	value T
	err   error
}

// awaitContextCall isolates extension points that may ignore cancellation.
// Cancellation stops the Agent from waiting; Go cannot forcibly terminate the
// abandoned call, so the call retains ownership of any late result and cleans
// it up before its recovered goroutine exits.
func awaitContextCall[T any](
	ctx context.Context,
	call func() (T, error),
	onCancel func(),
	discard func(T),
) (T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	var state atomic.Uint32
	result := make(chan asyncCallResult[T], 1)

	discardValue := func(value T) {
		if discard == nil {
			return
		}
		func() {
			defer func() { _ = recover() }()
			discard(value)
		}()
	}
	publish := func(received asyncCallResult[T]) {
		if state.CompareAndSwap(asyncCallActive, asyncCallCompleted) {
			result <- received
			return
		}
		discardValue(received.value)
	}
	safeGo(func() {
		value, err := call()
		publish(asyncCallResult[T]{value: value, err: err})
	}, func(err error) {
		publish(asyncCallResult[T]{err: err})
	})

	notifyCancel := func() {
		if onCancel == nil {
			return
		}
		safeGo(onCancel, func(error) {})
	}
	select {
	case received := <-result:
		if err := ctx.Err(); err != nil {
			notifyCancel()
			discardValue(received.value)
			return zero, err
		}
		return received.value, received.err
	case <-ctx.Done():
		notifyCancel()
		if state.CompareAndSwap(asyncCallActive, asyncCallAbandoned) {
			return zero, ctx.Err()
		}
		// The call won the ownership race and is about to publish to the
		// buffered channel. Cancellation still wins at this boundary.
		received := <-result
		discardValue(received.value)
		return zero, ctx.Err()
	}
}
