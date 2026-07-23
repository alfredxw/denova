package app

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"denova/internal/keyedlock"
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
	processOverride func(context.Context, *AutomationAppService, *automationWorkspaceSnapshot, string) error
}

type automationTriggerRequest struct {
	service   *AutomationAppService
	snapshot  *automationWorkspaceSnapshot
	operation *appOperation
	sources   map[string]struct{}
	targets   map[string]struct{}
	dirty     bool
	complete  []func(error)
}

func newAutomationTriggerCoordinator() *automationTriggerCoordinator {
	ctx, cancel := context.WithCancel(context.Background())
	return &automationTriggerCoordinator{
		ctx:     ctx,
		cancel:  cancel,
		entries: make(map[string]*automationTriggerRequest),
	}
}

func (c *automationTriggerCoordinator) Enqueue(service *AutomationAppService, snapshot *automationWorkspaceSnapshot, source string, targets []string) bool {
	return c.enqueue(service, snapshot, source, targets, nil)
}

// EnqueueWithCompletion keeps durable outbox owners pending until the trigger
// pass itself has completed. The callback is process-local; a crash simply
// leaves the persisted outbox pending for the next startup scan.
func (c *automationTriggerCoordinator) EnqueueWithCompletion(
	service *AutomationAppService,
	snapshot *automationWorkspaceSnapshot,
	source string,
	targets []string,
	complete func(error),
) bool {
	return c.enqueue(service, snapshot, source, targets, complete)
}

func (c *automationTriggerCoordinator) enqueue(service *AutomationAppService, snapshot *automationWorkspaceSnapshot, source string, targets []string, complete func(error)) bool {
	if c == nil || service == nil || service.app == nil || snapshot == nil {
		return false
	}
	workspace := canonicalAutomationWorkspace(snapshot.workspace)
	if workspace == "" {
		return false
	}
	operation, err := service.app.acquireWorkspaceOperation(c.ctx, workspace, false)
	if err != nil {
		return false
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		operation.Release()
		return false
	}
	request := c.entries[workspace]
	if request == nil {
		request = &automationTriggerRequest{
			service:   service,
			snapshot:  snapshot,
			operation: operation,
			sources:   make(map[string]struct{}),
			targets:   make(map[string]struct{}),
		}
		c.entries[workspace] = request
		c.wg.Add(1)
		go c.run(workspace, request)
	} else {
		// A later immutable snapshot for the same canonical workspace is safe
		// to prefer and avoids retaining superseded runtime references.
		request.service = service
		request.snapshot = snapshot
		operation.Release()
	}
	mergeAutomationTriggerRequest(request, source, targets)
	if complete != nil {
		request.complete = append(request.complete, complete)
	}
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
	defer request.operation.Release()
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
		snapshot := request.snapshot
		source := joinedAutomationTriggerValues(request.sources)
		targets := sortedAutomationTriggerValues(request.targets)
		completions := append([]func(error){}, request.complete...)
		request.sources = make(map[string]struct{})
		request.targets = make(map[string]struct{})
		request.complete = nil
		request.dirty = false
		c.mu.Unlock()

		var itemsCount, runsCount int
		var err error
		if processOverride := c.processOverride; processOverride != nil {
			err = processOverride(request.operation.Context(), service, snapshot, source)
		} else {
			items, runs, processErr := service.processContentTriggers(request.operation.Context(), snapshot, time.Now().UTC(), source)
			itemsCount, runsCount, err = len(items), len(runs), processErr
		}
		if err != nil {
			log.Printf("[automation-trigger] mutation check failed source=%s workspace=%q targets=%q err=%v", source, workspace, targets, err)
		} else if itemsCount > 0 || runsCount > 0 {
			log.Printf("[automation-trigger] mutation check completed source=%s workspace=%q targets=%q inbox=%d runs=%d", source, workspace, targets, itemsCount, runsCount)
		}
		for _, complete := range completions {
			runAutomationTriggerCompletion(workspace, complete, err)
		}
		if c.afterRun != nil {
			c.afterRun(workspace)
		}
		if c.ctx.Err() != nil {
			return
		}
	}
}

func runAutomationTriggerCompletion(workspace string, complete func(error), processErr error) {
	if complete == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[automation-trigger] completion callback panic recovered workspace=%q err=%v", workspace, recovered)
		}
	}()
	complete(processErr)
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

var triggerExecutionLocks = keyedlock.New(canonicalAutomationWorkspace)
