package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestToolExecutionGateAllowsReadOnlyCallsInParallel(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{
		toolSettings:        config.ResolvedAgentToolSettings{FileRead: true},
		enforceToolSettings: true,
		executionGate:       &toolExecutionGate{},
	}
	entered := make(chan string, 2)
	release := make(chan struct{})
	endpoint := func(ctx context.Context, args string, _ ...agent.ToolOption) (string, error) {
		entered <- args
		select {
		case <-release:
			return "ok", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	readFile := mustWrapGateTestEndpoint(t, middleware, "read_file", endpoint)
	grep := mustWrapGateTestEndpoint(t, middleware, "grep", endpoint)

	errs := make(chan error, 2)
	go invokeGateTestEndpoint(readFile, `{"file_path":"a.md"}`, errs)
	go invokeGateTestEndpoint(grep, `{"pattern":"x"}`, errs)
	waitForGateTestEntries(t, entered, 2)
	close(release)
	waitForGateTestResults(t, errs, 2)
}

func TestToolExecutionGateSerializesWritesAcrossMiddlewareInstances(t *testing.T) {
	workspace := t.TempDir()
	gateA := sharedToolExecutionGate(workspace)
	gateB := sharedToolExecutionGate(workspace)
	if gateA != gateB {
		t.Fatal("same workspace should reuse one execution gate")
	}
	settings := config.ResolvedAgentToolSettings{FileWrite: true}
	firstMiddleware := &toolOrchestratorMiddleware{toolSettings: settings, enforceToolSettings: true, executionGate: gateA}
	secondMiddleware := &toolOrchestratorMiddleware{toolSettings: settings, enforceToolSettings: true, executionGate: gateB}

	entered := make(chan string, 2)
	firstRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	var callMu sync.Mutex
	call := 0
	endpoint := func(ctx context.Context, _ string, _ ...agent.ToolOption) (string, error) {
		callMu.Lock()
		call++
		index := call
		callMu.Unlock()
		entered <- fmt.Sprintf("call-%d", index)
		wait := firstRelease
		if index == 2 {
			wait = secondRelease
		}
		select {
		case <-wait:
			return "ok", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	edit := mustWrapGateTestEndpoint(t, firstMiddleware, "edit_file", endpoint)
	write := mustWrapGateTestEndpoint(t, secondMiddleware, "write_file", endpoint)

	errs := make(chan error, 2)
	go invokeGateTestEndpoint(edit, `{"file_path":"a.md","edits":[]}`, errs)
	if got := waitForGateTestEntry(t, entered); got != "call-1" {
		t.Fatalf("first entry = %q", got)
	}
	go invokeGateTestEndpoint(write, `{"file_path":"b.md","content":"b"}`, errs)
	assertNoGateTestEntry(t, entered)
	close(firstRelease)
	if got := waitForGateTestEntry(t, entered); got != "call-2" {
		t.Fatalf("second entry = %q", got)
	}
	close(secondRelease)
	waitForGateTestResults(t, errs, 2)
}

func TestToolExecutionGateCanceledWaiterDoesNotStartEndpoint(t *testing.T) {
	tests := []struct {
		name string
		mode toolExecutionMode
	}{
		{name: "parallel read", mode: toolExecutionParallelRead},
		{name: "exclusive", mode: toolExecutionExclusive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := &toolExecutionGate{}
			releaseBlocker, err := gate.acquire(context.Background(), toolExecutionExclusive)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseBlocker()

			ctx, cancel := context.WithCancel(context.Background())
			acquireObserved := make(chan struct{})
			waitingCtx := &gateTestObservedContext{Context: ctx, doneObserved: acquireObserved}
			endpointStarted := make(chan struct{}, 1)
			result := make(chan error, 1)
			go func() {
				release, err := gate.acquire(waitingCtx, tt.mode)
				if err != nil {
					result <- err
					return
				}
				defer release()
				endpointStarted <- struct{}{}
				result <- nil
			}()

			<-acquireObserved
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("queued acquisition error = %v, want context.Canceled", err)
				}
			case <-time.After(250 * time.Millisecond):
				t.Fatal("canceled acquisition did not return while exclusive admission was held")
			}

			releaseBlocker()
			select {
			case <-endpointStarted:
				t.Fatal("endpoint started after its queued acquisition was canceled")
			default:
			}
		})
	}
}

func TestToolExecutionGateCanonicalizesWorkspaceSymlink(t *testing.T) {
	workspace := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(workspace, link); err != nil {
		t.Fatal(err)
	}
	if direct, throughLink := sharedToolExecutionGate(workspace), sharedToolExecutionGate(link); direct != throughLink {
		t.Fatal("one physical workspace received multiple execution gates")
	}
}

func TestToolExecutionGateHoldsExclusiveLockUntilToolRunReturns(t *testing.T) {
	gate := &toolExecutionGate{}
	middleware := &toolOrchestratorMiddleware{
		toolSettings:        config.ResolvedAgentToolSettings{FileWrite: true, ShellExecute: true},
		enforceToolSettings: true,
		executionGate:       gate,
	}
	entered := make(chan string, 2)
	releaseExecute := make(chan struct{})
	execute := mustWrapGateTestEndpoint(t, middleware, "execute", func(context.Context, string, ...agent.ToolOption) (string, error) {
		entered <- "execute"
		<-releaseExecute
		return "done", nil
	})
	releaseWrite := make(chan struct{})
	edit := mustWrapGateTestEndpoint(t, middleware, "edit_file", func(ctx context.Context, _ string, _ ...agent.ToolOption) (string, error) {
		entered <- "edit"
		select {
		case <-releaseWrite:
			return "ok", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	errs := make(chan error, 2)
	go invokeGateTestEndpoint(execute, `{"command":"touch a.md"}`, errs)
	if got := waitForGateTestEntry(t, entered); got != "execute" {
		t.Fatalf("first entry = %q", got)
	}
	go invokeGateTestEndpoint(edit, `{"file_path":"a.md","edits":[]}`, errs)
	assertNoGateTestEntry(t, entered)
	close(releaseExecute)
	if got := waitForGateTestEntry(t, entered); got != "edit" {
		t.Fatalf("entry after execute returned = %q", got)
	}
	close(releaseWrite)
	waitForGateTestResults(t, errs, 2)
}

func TestToolExecutionGateKeepsLockAfterCancelUntilNonCooperativeToolReturns(t *testing.T) {
	gate := &toolExecutionGate{}
	middleware := &toolOrchestratorMiddleware{
		toolSettings:        config.ResolvedAgentToolSettings{FileWrite: true, ShellExecute: true},
		enforceToolSettings: true,
		executionGate:       gate,
	}
	entered := make(chan string, 2)
	releaseExecute := make(chan struct{})
	execute := mustWrapGateTestEndpoint(t, middleware, "execute", func(context.Context, string, ...agent.ToolOption) (string, error) {
		entered <- "execute"
		<-releaseExecute
		return "done", nil
	})
	executeCtx, cancelExecute := context.WithCancel(context.Background())
	executeResult := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				executeResult <- fmt.Errorf("panic: %v", recovered)
			}
		}()
		_, runErr := execute(executeCtx, `{"command":"long-running"}`)
		executeResult <- runErr
	}()
	if got := waitForGateTestEntry(t, entered); got != "execute" {
		t.Fatalf("first entry = %q", got)
	}
	releaseWrite := make(chan struct{})
	edit := mustWrapGateTestEndpoint(t, middleware, "edit_file", func(ctx context.Context, _ string, _ ...agent.ToolOption) (string, error) {
		entered <- "edit"
		select {
		case <-releaseWrite:
			return "ok", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	errs := make(chan error, 1)
	go invokeGateTestEndpoint(edit, `{"file_path":"a.md","edits":[]}`, errs)
	assertNoGateTestEntry(t, entered)
	cancelExecute()
	assertNoGateTestEntry(t, entered)

	// Cancellation asks the tool to stop, but a non-cooperative mutation keeps
	// the safety lease until its Run method actually returns.
	close(releaseExecute)
	if err := <-executeResult; err != nil {
		t.Fatal(err)
	}
	if got := waitForGateTestEntry(t, entered); got != "edit" {
		t.Fatalf("entry after non-cooperative tool returned = %q", got)
	}
	close(releaseWrite)
	waitForGateTestResults(t, errs, 1)
}

func mustWrapGateTestEndpoint(t *testing.T, middleware *toolOrchestratorMiddleware, name string, endpoint testTextToolEndpoint) testTextToolEndpoint {
	t.Helper()
	wrapped, err := wrapTextToolCallForTest(middleware, endpoint, testToolContext(name, ""))
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func invokeGateTestEndpoint(endpoint testTextToolEndpoint, args string, results chan<- error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			results <- fmt.Errorf("panic: %v", recovered)
		}
	}()
	_, err := endpoint(context.Background(), args)
	results <- err
}

func waitForGateTestEntries(t *testing.T, entered <-chan string, count int) {
	t.Helper()
	for range count {
		_ = waitForGateTestEntry(t, entered)
	}
}

func waitForGateTestEntry(t *testing.T, entered <-chan string) string {
	t.Helper()
	select {
	case value := <-entered:
		return value
	case <-time.After(250 * time.Millisecond):
		t.Fatal("tool endpoint did not enter before test deadline")
		return ""
	}
}

func assertNoGateTestEntry(t *testing.T, entered <-chan string) {
	t.Helper()
	select {
	case value := <-entered:
		t.Fatalf("exclusive tool overlapped another writer: %s", value)
	case <-time.After(25 * time.Millisecond):
	}
}

func waitForGateTestResults(t *testing.T, results <-chan error, count int) {
	t.Helper()
	for range count {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(250 * time.Millisecond):
			t.Fatal("tool endpoint did not finish before test deadline")
		}
	}
}

type gateTestObservedContext struct {
	context.Context
	doneObserved chan struct{}
	once         sync.Once
}

func (c *gateTestObservedContext) Done() <-chan struct{} {
	c.once.Do(func() {
		close(c.doneObserved)
	})
	return c.Context.Done()
}
