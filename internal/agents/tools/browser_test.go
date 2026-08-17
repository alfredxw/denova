package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	browserruntime "denova/internal/browser"
)

type browserControllerStub struct {
	open        browserruntime.OpenRequest
	run         browserruntime.RunRequest
	close       browserruntime.CloseRequest
	shutdown    int
	shutdownErr error
}

func (controller *browserControllerStub) Open(_ context.Context, request browserruntime.OpenRequest) (browserruntime.Result, error) {
	controller.open = request
	return browserStubResult("open", request.Tab, ""), nil
}

func (controller *browserControllerStub) Run(_ context.Context, request browserruntime.RunRequest) (browserruntime.Result, error) {
	controller.run = request
	return browserStubResult("run", request.Tab, request.Command), nil
}

func (controller *browserControllerStub) Close(_ context.Context, request browserruntime.CloseRequest) (browserruntime.Result, error) {
	controller.close = request
	return browserStubResult("close", request.Tab, ""), nil
}

func (controller *browserControllerStub) Shutdown(context.Context) error {
	controller.shutdown++
	return controller.shutdownErr
}

func browserStubResult(action, tab, command string) browserruntime.Result {
	return browserruntime.Result{
		Schema: "browser.result.v1", Status: "completed", Action: action, Tab: tab, Command: command,
		Receipt: browserruntime.ExternalReceipt{
			Schema: "external_effect.receipt.v1", Boundary: "browser", Operation: action, Target: tab, Status: "completed",
		},
	}
}

func TestBrowserToolPublishesActionUnionAndExternalContract(t *testing.T) {
	controller := &browserControllerStub{}
	definition, err := NewBrowser(controller)
	if err != nil {
		t.Fatal(err)
	}
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "browser" || len(schema.AnyOf) != 3 {
		t.Fatalf("browser schema = %#v", info)
	}
	descriptor := definition.Descriptor
	if descriptor.Capability != config.AgentToolBrowser ||
		descriptor.Execution != agent.ToolExecutionSessionExclusive ||
		descriptor.MutationScope != agent.ToolMutationExternal ||
		descriptor.PostCheck != agent.ToolPostCheckExternalReceipt ||
		descriptor.Recovery != agent.ToolRecoveryNonIdempotent {
		t.Fatalf("browser descriptor = %#v", descriptor)
	}

	for _, arguments := range []string{
		`{"action":"open","tab":"docs","url":"https://example.com"}`,
		`{"action":"run","tab":"docs","command":"fill","selector":"#query","text":"Denova"}`,
		`{"action":"close","tab":"docs"}`,
	} {
		result, err := definition.Tool.Run(context.Background(), arguments)
		if err != nil {
			t.Fatalf("browser run %s: %v", arguments, err)
		}
		if !strings.Contains(result.ModelContent, `"boundary":"browser"`) || !json.Valid(result.Details) ||
			!strings.Contains(string(result.Details), `"schema":"browser.tool_receipt.v1"`) ||
			!strings.Contains(string(result.Details), `"schema":"external_effect.receipt.v1"`) {
			t.Fatalf("browser result = %#v", result)
		}
	}
	if controller.open.Tab != "docs" || controller.run.Command != "fill" || controller.run.Text != "Denova" || controller.close.Tab != "docs" {
		t.Fatalf("browser routes = open %#v run %#v close %#v", controller.open, controller.run, controller.close)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"action":"run","tab":"docs","command":"wait","text":"ready","timeout_seconds":9}`); err != nil {
		t.Fatal(err)
	}
	if controller.run.Command != browserruntime.CommandWait || controller.run.Text != "ready" || controller.run.TimeoutSeconds != 9 {
		t.Fatalf("browser wait route = %#v", controller.run)
	}
	for _, arguments := range []string{
		`{"action":"open","tab":"docs","command":"observe"}`,
		`{"action":"close","tab":"docs","mystery":true}`,
	} {
		if _, err := definition.Tool.Run(context.Background(), arguments); err != nil {
			t.Fatalf("browser rejected harmless extra action field %s: %v", arguments, err)
		}
	}
	if _, err := definition.Tool.Run(context.Background(), `{"action":"run","tab":"docs","command":"unknown"}`); err == nil {
		t.Fatal("browser accepted an unknown command enum")
	}
}

func TestCatalogBrowserRegistersOnlyEnabledAvailableRuntime(t *testing.T) {
	originalProbe := probeRuntimeBrowser
	originalCreate := createRuntimeBrowserController
	defer func() {
		probeRuntimeBrowser = originalProbe
		createRuntimeBrowserController = originalCreate
	}()
	controller := &browserControllerStub{}
	probed := 0
	created := 0
	probeRuntimeBrowser = func(context.Context) (bool, error) {
		probed++
		return true, nil
	}
	createRuntimeBrowserController = func(context.Context) (runtimeBrowserController, error) {
		created++
		return controller, nil
	}
	catalog := NewCatalog(&config.Config{AgentToolResultLimitKB: 64}, nil, RuntimeExecutables{})

	disabled, err := catalog.Browser(context.Background(), config.ResolvedAgentToolSettings{})
	if err != nil || len(disabled) != 0 || probed != 0 || created != 0 {
		t.Fatalf("disabled browser = definitions %d probed %d created %d err %v", len(disabled), probed, created, err)
	}
	enabled, err := catalog.Browser(context.Background(), config.ResolvedAgentToolSettings{config.AgentToolBrowser: true})
	if err != nil || len(enabled) != 1 || probed != 1 || created != 0 {
		t.Fatalf("enabled browser = definitions %d probed %d created %d err %v", len(enabled), probed, created, err)
	}
	if enabled[0].Descriptor.MaxResultBytes != 64*1024 {
		t.Fatalf("browser max result bytes = %d", enabled[0].Descriptor.MaxResultBytes)
	}
	invocationCtx, finishInvocation, err := agent.BeginChildInvocation(context.Background(), "browser-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enabled[0].Tool.Run(invocationCtx, `{"action":"open","tab":"docs"}`); err != nil {
		t.Fatal(err)
	}
	if err := finishInvocation(); err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("browser controller creations = %d", created)
	}

	probeRuntimeBrowser = func(context.Context) (bool, error) {
		return false, nil
	}
	unavailable, err := catalog.Browser(context.Background(), config.ResolvedAgentToolSettings{config.AgentToolBrowser: true})
	if err != nil || len(unavailable) != 0 {
		t.Fatalf("unavailable browser = definitions %d err %v", len(unavailable), err)
	}
}

func TestRuntimeBrowserSessionUsesFirstAgentCallLifetime(t *testing.T) {
	originalProbe := probeRuntimeBrowser
	originalCreate := createRuntimeBrowserController
	defer func() {
		probeRuntimeBrowser = originalProbe
		createRuntimeBrowserController = originalCreate
	}()
	probeRuntimeBrowser = func(context.Context) (bool, error) { return true, nil }
	controller := &browserControllerStub{}
	created := 0
	lifetimeDone := make(chan struct{})
	createRuntimeBrowserController = func(ctx context.Context) (runtimeBrowserController, error) {
		created++
		go func() {
			<-ctx.Done()
			close(lifetimeDone)
		}()
		return controller, nil
	}

	catalogCtx, cancelCatalog := context.WithCancel(context.Background())
	definition, available, err := newRuntimeBrowserTool(catalogCtx)
	if err != nil || !available {
		t.Fatalf("build runtime browser available=%t err=%v", available, err)
	}
	cancelCatalog()
	if created != 0 {
		t.Fatalf("catalog construction eagerly created %d browser controllers", created)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	invocationCtx, finishInvocation, err := agent.BeginChildInvocation(runCtx, "browser-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definition.Tool.Run(invocationCtx, `{"action":"open","tab":"docs"}`); err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("first tool call created %d browser controllers", created)
	}
	select {
	case <-lifetimeDone:
		t.Fatal("browser lifetime ended with the catalog context")
	default:
	}
	cancelRun()
	select {
	case <-lifetimeDone:
	case <-time.After(time.Second):
		t.Fatal("browser lifetime did not end with the Agent run context")
	}
	if err := finishInvocation(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeBrowserControllerIsOwnedByEachInvocation(t *testing.T) {
	originalProbe := probeRuntimeBrowser
	originalCreate := createRuntimeBrowserController
	defer func() {
		probeRuntimeBrowser = originalProbe
		createRuntimeBrowserController = originalCreate
	}()
	probeRuntimeBrowser = func(context.Context) (bool, error) { return true, nil }
	var controllers []*browserControllerStub
	createRuntimeBrowserController = func(context.Context) (runtimeBrowserController, error) {
		controller := &browserControllerStub{}
		controllers = append(controllers, controller)
		return controller, nil
	}
	definition, available, err := newRuntimeBrowserTool(context.Background())
	if err != nil || !available {
		t.Fatalf("build runtime browser available=%t err=%v", available, err)
	}
	for index := 0; index < 2; index++ {
		ctx, finish, err := agent.BeginChildInvocation(context.Background(), "browser-test")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := definition.Tool.Run(ctx, `{"action":"open","tab":"docs"}`); err != nil {
			t.Fatal(err)
		}
		if err := finish(); err != nil {
			t.Fatal(err)
		}
	}
	if len(controllers) != 2 || controllers[0] == controllers[1] || controllers[0].shutdown != 1 || controllers[1].shutdown != 1 {
		t.Fatalf("invocation-owned controllers = %#v", controllers)
	}
}

func TestRuntimeBrowserInvocationCleanupSurfacesShutdownErrorAfterCancellation(t *testing.T) {
	originalProbe := probeRuntimeBrowser
	originalCreate := createRuntimeBrowserController
	defer func() {
		probeRuntimeBrowser = originalProbe
		createRuntimeBrowserController = originalCreate
	}()
	probeRuntimeBrowser = func(context.Context) (bool, error) { return true, nil }
	shutdownErr := errors.New("browser shutdown failed")
	controller := &browserControllerStub{shutdownErr: shutdownErr}
	createRuntimeBrowserController = func(context.Context) (runtimeBrowserController, error) {
		return controller, nil
	}
	definition, available, err := newRuntimeBrowserTool(context.Background())
	if err != nil || !available {
		t.Fatalf("build runtime browser available=%t err=%v", available, err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	invocationCtx, finishInvocation, err := agent.BeginChildInvocation(runCtx, "browser-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definition.Tool.Run(invocationCtx, `{"action":"open","tab":"docs"}`); err != nil {
		t.Fatal(err)
	}
	cancelRun()
	if err := finishInvocation(); !errors.Is(err, shutdownErr) {
		t.Fatalf("invocation cleanup error = %v, want %v", err, shutdownErr)
	}
	if controller.shutdown != 1 {
		t.Fatalf("controller shutdown count = %d, want 1", controller.shutdown)
	}
}
