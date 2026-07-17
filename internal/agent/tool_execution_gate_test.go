package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

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
	endpoint := func(ctx context.Context, args string, _ ...tool.Option) (string, error) {
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
	endpoint := func(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
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

func TestToolExecutionGateHoldsStreamLockUntilResultEOF(t *testing.T) {
	gate := &toolExecutionGate{}
	middleware := &toolOrchestratorMiddleware{
		toolSettings:        config.ResolvedAgentToolSettings{FileWrite: true, ShellExecute: true},
		enforceToolSettings: true,
		executionGate:       gate,
	}
	sourceReader, sourceWriter := schema.Pipe[string](1)
	stream, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			return sourceReader, nil
		},
		&adk.ToolContext{Name: "execute"},
	)
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := stream(context.Background(), `{"command":"touch a.md"}`)
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan string, 1)
	releaseWrite := make(chan struct{})
	edit := mustWrapGateTestEndpoint(t, middleware, "edit_file", func(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
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

	if closed := sourceWriter.Send("done", nil); closed {
		t.Fatal("source stream closed before test result was sent")
	}
	sourceWriter.Close()
	if _, err := filtered.Recv(); err != nil {
		t.Fatal(err)
	}
	if _, err := filtered.Recv(); err != io.EOF {
		t.Fatalf("filtered stream EOF = %v", err)
	}
	if got := waitForGateTestEntry(t, entered); got != "edit" {
		t.Fatalf("entry after stream = %q", got)
	}
	close(releaseWrite)
	waitForGateTestResults(t, errs, 1)
}

func TestToolExecutionGateKeepsStreamLockAfterContextCancelUntilReaderEnds(t *testing.T) {
	gate := &toolExecutionGate{}
	middleware := &toolOrchestratorMiddleware{
		toolSettings:        config.ResolvedAgentToolSettings{FileWrite: true, ShellExecute: true},
		enforceToolSettings: true,
		executionGate:       gate,
	}
	sourceReader, sourceWriter := schema.Pipe[string](1)
	stream, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			return sourceReader, nil
		},
		&adk.ToolContext{Name: "execute"},
	)
	if err != nil {
		t.Fatal(err)
	}
	streamCtx, cancelStream := context.WithCancel(context.Background())
	filtered, err := stream(streamCtx, `{"command":"long-running"}`)
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan string, 1)
	releaseWrite := make(chan struct{})
	edit := mustWrapGateTestEndpoint(t, middleware, "edit_file", func(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
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
	cancelStream()
	assertNoGateTestEntry(t, entered)

	// Cancellation asks the endpoint to stop, but must not drop the safety lease
	// while a non-cooperative stream can still mutate the workspace.
	sourceWriter.Close()
	if _, err := filtered.Recv(); err != nil {
		t.Fatal(err)
	}
	if _, err := filtered.Recv(); err != io.EOF {
		t.Fatalf("filtered stream EOF = %v", err)
	}
	if got := waitForGateTestEntry(t, entered); got != "edit" {
		t.Fatalf("entry after source stream ended = %q", got)
	}
	close(releaseWrite)
	waitForGateTestResults(t, errs, 1)
}

func mustWrapGateTestEndpoint(t *testing.T, middleware *toolOrchestratorMiddleware, name string, endpoint adk.InvokableToolCallEndpoint) adk.InvokableToolCallEndpoint {
	t.Helper()
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), endpoint, &adk.ToolContext{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func invokeGateTestEndpoint(endpoint adk.InvokableToolCallEndpoint, args string, results chan<- error) {
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
