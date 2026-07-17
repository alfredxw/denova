package app

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// automationTriggerCoordinator owns mutation-trigger evaluation for the App
// lifetime. One worker is active per canonical workspace; saves that arrive
// while evaluation is running are coalesced into at most one follow-up pass.
type automationTriggerCoordinator struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	closed  bool
	wg      sync.WaitGroup
	entries map[string]*automationTriggerRequest

	// Test barriers stay private so lifecycle races can be exercised without
	// timing-dependent sleeps or production API surface.
	afterRun        func(string)
	afterIdleDetach func(string)
}

type automationTriggerRequest struct {
	service *AutomationAppService
	sources map[string]struct{}
	targets map[string]struct{}
	dirty   bool
}

func newAutomationTriggerCoordinator() *automationTriggerCoordinator {
	ctx, cancel := context.WithCancel(context.Background())
	return &automationTriggerCoordinator{
		ctx:     ctx,
		cancel:  cancel,
		entries: make(map[string]*automationTriggerRequest),
	}
}

func (c *automationTriggerCoordinator) Enqueue(service *AutomationAppService, source string, targets []string) bool {
	if c == nil || service == nil {
		return false
	}
	workspace := canonicalAutomationWorkspace(service.workspace())
	if workspace == "" {
		return false
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	request := c.entries[workspace]
	if request == nil {
		request = &automationTriggerRequest{
			service: service,
			sources: make(map[string]struct{}),
			targets: make(map[string]struct{}),
		}
		c.entries[workspace] = request
		c.wg.Add(1)
		go c.run(workspace, request)
	} else {
		// A later immutable snapshot for the same canonical workspace is safe
		// to prefer and avoids retaining superseded runtime references.
		request.service = service
	}
	mergeAutomationTriggerRequest(request, source, targets)
	request.dirty = true
	c.mu.Unlock()
	return true
}

func mergeAutomationTriggerRequest(request *automationTriggerRequest, source string, targets []string) {
	if source = strings.TrimSpace(source); source != "" {
		request.sources[source] = struct{}{}
	}
	for _, target := range targets {
		if target = strings.TrimSpace(target); target != "" {
			request.targets[target] = struct{}{}
		}
	}
}

func (c *automationTriggerCoordinator) run(workspace string, request *automationTriggerRequest) {
	defer c.wg.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[automation-trigger] coordinator panic recovered workspace=%q err=%v", workspace, recovered)
		}
		c.mu.Lock()
		if c.entries[workspace] == request {
			delete(c.entries, workspace)
		}
		c.mu.Unlock()
	}()
	for {
		c.mu.Lock()
		if c.closed || !request.dirty {
			if c.entries[workspace] == request {
				delete(c.entries, workspace)
			}
			afterIdleDetach := c.afterIdleDetach
			c.mu.Unlock()
			if afterIdleDetach != nil {
				afterIdleDetach(workspace)
			}
			return
		}
		service := request.service
		source := joinedAutomationTriggerValues(request.sources)
		targets := sortedAutomationTriggerValues(request.targets)
		request.sources = make(map[string]struct{})
		request.targets = make(map[string]struct{})
		request.dirty = false
		c.mu.Unlock()

		items, runs, err := service.processContentTriggers(c.ctx, time.Now().UTC(), source)
		if err != nil {
			log.Printf("[automation-trigger] mutation check failed source=%s workspace=%q targets=%q err=%v", source, workspace, targets, err)
		} else if len(items) > 0 || len(runs) > 0 {
			log.Printf("[automation-trigger] mutation check completed source=%s workspace=%q targets=%q inbox=%d runs=%d", source, workspace, targets, len(items), len(runs))
		}
		if c.afterRun != nil {
			c.afterRun(workspace)
		}
		if c.ctx.Err() != nil {
			return
		}
	}
}

func joinedAutomationTriggerValues(values map[string]struct{}) string {
	items := sortedAutomationTriggerValues(values)
	if len(items) == 0 {
		return "workspace_mutation"
	}
	return strings.Join(items, ",")
}

func sortedAutomationTriggerValues(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}

func (c *automationTriggerCoordinator) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.cancel()
	}
	c.mu.Unlock()
	c.wg.Wait()
}

type automationTriggerExecutionLocks struct {
	mu      sync.Mutex
	entries map[string]*automationTriggerExecutionLock
}

type automationTriggerExecutionLock struct {
	mu   sync.Mutex
	refs int
}

var triggerExecutionLocks = &automationTriggerExecutionLocks{
	entries: make(map[string]*automationTriggerExecutionLock),
}

func (l *automationTriggerExecutionLocks) lock(workspace string) func() {
	key := canonicalAutomationWorkspace(workspace)
	l.mu.Lock()
	entry := l.entries[key]
	if entry == nil {
		entry = &automationTriggerExecutionLock{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}
