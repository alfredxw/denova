package app

import (
	"context"
	"log"
	"sync"
)

// workspaceDirectorTaskGroup owns background Director work for exactly one
// workspace runtime. HTTP disconnects do not cancel it; replacing or closing
// the runtime does.
type workspaceDirectorTaskGroup struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
	tails  map[string]chan struct{}
}

func newWorkspaceDirectorTaskGroup() *workspaceDirectorTaskGroup {
	ctx, cancel := context.WithCancel(context.Background())
	return &workspaceDirectorTaskGroup{ctx: ctx, cancel: cancel, tails: map[string]chan struct{}{}}
}

func (g *workspaceDirectorTaskGroup) Go(run func(context.Context)) (<-chan struct{}, bool) {
	return g.GoKeyed("", run)
}

// GoKeyed serializes work sharing the same story/branch key while allowing
// unrelated branches to continue independently.
func (g *workspaceDirectorTaskGroup) GoKeyed(key string, run func(context.Context)) (<-chan struct{}, bool) {
	done := make(chan struct{})
	if g == nil || run == nil {
		close(done)
		return done, false
	}
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		close(done)
		return done, false
	}
	var previous chan struct{}
	if key != "" {
		previous = g.tails[key]
		g.tails[key] = done
	}
	g.wg.Add(1)
	g.mu.Unlock()

	go func() {
		defer g.wg.Done()
		defer func() {
			close(done)
			if key != "" {
				g.mu.Lock()
				if g.tails[key] == done {
					delete(g.tails, key)
				}
				g.mu.Unlock()
			}
		}()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[interactive-director-agent] workspace task panic recovered err=%v", recovered)
			}
		}()
		if previous != nil {
			select {
			case <-previous:
			case <-g.ctx.Done():
				return
			}
		}
		run(g.ctx)
	}()
	return done, true
}

// WaitKey waits until all work already queued for a story branch finishes.
// Work queued after this call starts belongs to a later turn and is not joined.
func (g *workspaceDirectorTaskGroup) WaitKey(ctx context.Context, key string) error {
	if g == nil || key == "" {
		return nil
	}
	g.mu.Lock()
	tail := g.tails[key]
	closed := g.closed
	g.mu.Unlock()
	if tail == nil {
		if closed {
			return context.Canceled
		}
		return nil
	}
	select {
	case <-tail:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-g.ctx.Done():
		return g.ctx.Err()
	}
}

func (g *workspaceDirectorTaskGroup) Close() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if !g.closed {
		g.closed = true
		g.cancel()
	}
	g.mu.Unlock()
	g.wg.Wait()
}
