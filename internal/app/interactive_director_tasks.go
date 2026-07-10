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
}

func newWorkspaceDirectorTaskGroup() *workspaceDirectorTaskGroup {
	ctx, cancel := context.WithCancel(context.Background())
	return &workspaceDirectorTaskGroup{ctx: ctx, cancel: cancel}
}

func (g *workspaceDirectorTaskGroup) Go(run func(context.Context)) (<-chan struct{}, bool) {
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
	g.wg.Add(1)
	g.mu.Unlock()

	go func() {
		defer g.wg.Done()
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[interactive-director-agent] workspace task panic recovered err=%v", recovered)
			}
		}()
		run(g.ctx)
	}()
	return done, true
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
