package browser

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

type fakeDriver struct {
	mu       sync.Mutex
	pages    []*fakePage
	pageURL  string
	closeErr error
	closes   int
}

func newFakeDriver() *fakeDriver { return &fakeDriver{} }

func (driver *fakeDriver) Available(context.Context) error { return nil }

func (driver *fakeDriver) NewPage(context.Context) (Page, error) {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	page := &fakePage{url: driver.pageURL}
	driver.pages = append(driver.pages, page)
	return page, nil
}

func (driver *fakeDriver) Close(context.Context) error {
	driver.mu.Lock()
	driver.closes++
	driver.mu.Unlock()
	return driver.closeErr
}

func (driver *fakeDriver) pageCount() int {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	return len(driver.pages)
}

func (driver *fakeDriver) closeCount() int {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	return driver.closes
}

type fakePage struct {
	mu         sync.Mutex
	url        string
	title      string
	text       string
	operations []string
	closed     bool
	waitCancel func()
}

func (page *fakePage) Navigate(ctx context.Context, target string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	page.mu.Lock()
	defer page.mu.Unlock()
	page.url = target
	page.title = "Test page"
	page.text = string(body)
	page.operations = append(page.operations, "goto")
	return nil
}

func (page *fakePage) Observe(context.Context) (Observation, error) {
	page.mu.Lock()
	defer page.mu.Unlock()
	return Observation{
		URL: page.url, Title: page.title, Text: page.text,
		Elements: []ElementSummary{{Ref: "e1", Role: "button", Name: "Save", Selector: "#save"}},
	}, nil
}

func (page *fakePage) Wait(ctx context.Context, condition WaitCondition) error {
	page.record("wait:" + condition.Selector + ":" + condition.Text)
	page.mu.Lock()
	cancel := page.waitCancel
	page.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-ctx.Done()
	return ctx.Err()
}

func (page *fakePage) Click(_ context.Context, selector string) error {
	page.record("click:" + selector)
	return nil
}

func (page *fakePage) Fill(_ context.Context, selector, text string) error {
	page.record("fill:" + selector + ":" + text)
	return nil
}

func (page *fakePage) Type(_ context.Context, selector, text string) error {
	page.record("type:" + selector + ":" + text)
	return nil
}

func (page *fakePage) Press(_ context.Context, selector, key string) error {
	page.record("press:" + selector + ":" + key)
	return nil
}

func (page *fakePage) Select(_ context.Context, selector string, values []string) error {
	page.record("select:" + selector + ":" + strings.Join(values, ","))
	return nil
}

func (page *fakePage) Evaluate(_ context.Context, expression string) (json.RawMessage, error) {
	page.record("evaluate:" + expression)
	return json.RawMessage(`{"answer":42}`), nil
}

func (page *fakePage) Screenshot(_ context.Context, full bool) ([]byte, error) {
	page.record("screenshot")
	return []byte("\x89PNG\r\n\x1a\nimage"), nil
}

func (page *fakePage) Close(context.Context) error {
	page.mu.Lock()
	defer page.mu.Unlock()
	page.closed = true
	return nil
}

func (page *fakePage) record(operation string) {
	page.mu.Lock()
	defer page.mu.Unlock()
	page.operations = append(page.operations, operation)
}

func TestSessionManagesNamedTabsAndAllFirstContractCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "hello from test page")
	}))
	defer server.Close()
	driver := newFakeDriver()
	session, err := NewSession(context.Background(), driver, Options{
		ArtifactRoot: t.TempDir(),
		ValidateURL:  func(_ context.Context, value string) (string, error) { return strings.TrimSpace(value), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Shutdown(context.Background()) }()

	opened, err := session.Open(context.Background(), OpenRequest{Tab: "research", URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Observation == nil || opened.Observation.Text != "hello from test page" || opened.Receipt.Boundary != "browser" {
		t.Fatalf("open result = %#v", opened)
	}
	if _, err := session.Open(context.Background(), OpenRequest{Tab: "research"}); err != nil {
		t.Fatal(err)
	}
	if driver.pageCount() != 1 {
		t.Fatalf("reopening a named tab created %d pages", driver.pageCount())
	}

	commands := []RunRequest{
		{Tab: "research", Command: CommandObserve},
		{Tab: "research", Command: CommandGoto, URL: server.URL + "/next"},
		{Tab: "research", Command: CommandWait, Selector: "#save", Text: "hello"},
		{Tab: "research", Command: CommandClick, Selector: "#save"},
		{Tab: "research", Command: CommandFill, Selector: "#title", Text: "draft"},
		{Tab: "research", Command: CommandType, Selector: "#title", Text: " more"},
		{Tab: "research", Command: CommandPress, Selector: "#title", Key: "Enter"},
		{Tab: "research", Command: CommandSelect, Selector: "#genre", Values: []string{"fantasy"}},
	}
	for _, request := range commands {
		result, runErr := session.Run(context.Background(), request)
		if runErr != nil {
			t.Fatalf("run %s: %v", request.Command, runErr)
		}
		if result.Command != request.Command || result.Receipt.Operation != request.Command {
			t.Fatalf("run result = %#v", result)
		}
	}
	evaluated, err := session.Run(context.Background(), RunRequest{Tab: "research", Command: CommandEvaluate, Expression: "({answer: 42})"})
	if err != nil || string(evaluated.Value) != `{"answer":42}` {
		t.Fatalf("evaluate result = %#v err=%v", evaluated, err)
	}
	screenshot, err := session.Run(context.Background(), RunRequest{Tab: "research", Command: CommandScreenshot, FullPage: true})
	if err != nil {
		t.Fatal(err)
	}
	if screenshot.Screenshot == nil || screenshot.Screenshot.Bytes == 0 || screenshot.Screenshot.SHA256 == "" {
		t.Fatalf("screenshot result = %#v", screenshot)
	}
	artifactPath := screenshot.Screenshot.Path
	if screenshot.Receipt.Target != artifactPath {
		t.Fatalf("screenshot receipt target = %q, want %q", screenshot.Receipt.Target, artifactPath)
	}
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("screenshot artifact: %v", err)
	}
	closed, err := session.Close(context.Background(), CloseRequest{Tab: "research"})
	if err != nil {
		t.Fatal(err)
	}
	if len(closed.Tabs) != 0 {
		t.Fatalf("tabs after close = %#v", closed.Tabs)
	}
	if _, err := os.Stat(artifactPath); !os.IsNotExist(err) {
		t.Fatalf("closed tab retained screenshot artifact: %v", err)
	}
}

func TestSessionWaitUsesCallerCancellationWithoutImplicitDeadline(t *testing.T) {
	driver := newFakeDriver()
	session, err := NewSession(context.Background(), driver, Options{
		ValidateURL: func(_ context.Context, value string) (string, error) { return value, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Shutdown(context.Background()) }()
	if _, err := session.Open(context.Background(), OpenRequest{Tab: "async"}); err != nil {
		t.Fatal(err)
	}

	waitCtx, cancel := context.WithCancel(context.Background())
	driver.mu.Lock()
	page := driver.pages[0]
	driver.mu.Unlock()
	page.mu.Lock()
	page.waitCancel = cancel
	page.mu.Unlock()
	_, err = session.Run(waitCtx, RunRequest{Tab: "async", Command: CommandWait, Text: "ready"})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("cancelled browser wait error = %v", err)
	}
	page.mu.Lock()
	operations := strings.Join(page.operations, "|")
	page.mu.Unlock()
	if !strings.Contains(operations, "wait::ready") {
		t.Fatalf("wait adapter was not invoked: %s", operations)
	}
	for _, request := range []RunRequest{
		{Tab: "async", Command: CommandWait},
		{Tab: "async", Command: CommandWait, Text: "ready", TimeoutSeconds: -1},
		{Tab: "async", Command: CommandObserve, TimeoutSeconds: 1},
	} {
		if _, err := session.Run(context.Background(), request); err == nil {
			t.Fatalf("invalid wait request was accepted: %#v", request)
		}
	}
}

func TestSessionCancellationAndExplicitShutdownHaveOneObservableOwner(t *testing.T) {
	lifetime, cancel := context.WithCancel(context.Background())
	driver := newFakeDriver()
	shutdownErr := errors.New("driver cleanup failed")
	driver.closeErr = shutdownErr
	session, err := NewSession(lifetime, driver, Options{
		ValidateURL: func(_ context.Context, value string) (string, error) { return value, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Open(context.Background(), OpenRequest{Tab: "one"}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	finishErr := make(chan error, 1)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		cancel()
	}()
	go func() {
		defer wait.Done()
		<-start
		finishErr <- session.Shutdown(context.Background())
	}()
	close(start)
	wait.Wait()
	if err := <-finishErr; !errors.Is(err, shutdownErr) {
		t.Fatalf("shutdown error = %v, want %v", err, shutdownErr)
	}
	if err := session.Shutdown(context.Background()); !errors.Is(err, shutdownErr) {
		t.Fatalf("repeated shutdown error = %v, want retained %v", err, shutdownErr)
	}
	if driver.closeCount() != 1 {
		t.Fatalf("driver close count = %d, want 1", driver.closeCount())
	}
	if _, err := session.Open(context.Background(), OpenRequest{Tab: "two"}); err == nil {
		t.Fatal("shutdown browser session reopened")
	}
}

func TestSessionRevalidatesAdapterObservationURL(t *testing.T) {
	driver := newFakeDriver()
	session, err := NewSession(context.Background(), driver, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Shutdown(context.Background()) }()

	if _, err := session.Open(context.Background(), OpenRequest{Tab: "boundary"}); err != nil {
		t.Fatal(err)
	}
	driver.mu.Lock()
	page := driver.pages[0]
	driver.mu.Unlock()
	page.mu.Lock()
	page.url = "file:///etc/passwd"
	page.mu.Unlock()
	if _, err := session.Run(context.Background(), RunRequest{Tab: "boundary", Command: CommandObserve}); err == nil ||
		!strings.Contains(err.Error(), "left the admitted HTTP(S) boundary") {
		t.Fatalf("unsafe adapter observation error = %v", err)
	}
}

func TestSessionDiscardsNewTabWhenInitialObservationIsUnsafe(t *testing.T) {
	driver := newFakeDriver()
	driver.pageURL = "file:///etc/passwd"
	session, err := NewSession(context.Background(), driver, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Shutdown(context.Background()) }()

	if _, err := session.Open(context.Background(), OpenRequest{Tab: "unsafe"}); err == nil {
		t.Fatal("unsafe initial observation was accepted")
	}
	if len(session.tabs) != 0 || driver.pageCount() != 1 {
		t.Fatalf("failed initial tab was retained: tabs=%d pages=%d", len(session.tabs), driver.pageCount())
	}
	driver.mu.Lock()
	page := driver.pages[0]
	driver.mu.Unlock()
	page.mu.Lock()
	closed := page.closed
	page.mu.Unlock()
	if !closed {
		t.Fatal("failed initial tab page was not closed")
	}
}

func TestValidatePublicURLRejectsHTTPTestServerAndUnsafeForms(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	for _, value := range []string{server.URL, "file:///tmp/a", "https://user:secret@example.com", "http://127.0.0.1"} {
		if _, err := ValidatePublicURL(context.Background(), value); err == nil {
			t.Fatalf("unsafe browser URL accepted: %s", value)
		}
	}
}
