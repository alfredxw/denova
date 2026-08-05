package app

import (
	"context"
	"denova/config"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestAppTaskReplayAdmissionIsSharedAcrossProducts(t *testing.T) {
	const retainedByteLimit = 128
	products := []struct {
		name string
		root bool
	}{
		{name: "Writing"},
		{name: "Game"},
		{name: "Lore"},
		{name: "Config"},
		{name: "Automation", root: true},
	}
	charge := 2 * retainedByteLimit

	tests := []struct {
		name       string
		countLimit int
		byteLimit  int
	}{
		{name: "aggregate count", countLimit: len(products), byteLimit: (len(products) + 1) * charge},
		{name: "aggregate bytes", countLimit: len(products) + 1, byteLimit: len(products) * charge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := &App{workspace: "/workspace-a"}
			application.activeTaskReplay.Configure(apptask.ReplayAdmissionLimits{MaxActive: test.countLimit, MaxBytes: test.byteLimit})
			tasks := make([]*apptask.Task, 0, len(products)+1)
			t.Cleanup(func() {
				for _, task := range tasks {
					task.RejectStart(errors.New("test cleanup"))
					application.unregisterWorkspaceTask(task)
				}
			})

			for _, product := range products {
				task, err := registerReplayProductTask(application, product.name, product.root, retainedByteLimit)
				if err != nil {
					t.Fatalf("register %s Task: %v", product.name, err)
				}
				tasks = append(tasks, task)
			}
			assertAppReplayRegistrationCount(t, application, len(products), len(products)*charge)

			if task, err := registerReplayProductTask(application, "Writing-overflow", false, retainedByteLimit); task != nil || !errors.Is(err, ErrAgentReplayCapacity) {
				t.Fatalf("overflow registration = task:%p err:%v, want ErrAgentReplayCapacity", task, err)
			}
			assertAppReplayRegistrationCount(t, application, len(products), len(products)*charge)

			// A rejected registration must not consume a hidden reservation. Once
			// one real owner exits, a different product can immediately take it.
			application.unregisterWorkspaceTask(tasks[0])
			replacement, err := registerReplayProductTask(application, "Game-replacement", false, retainedByteLimit)
			if err != nil {
				t.Fatalf("register after releasing Writing Task: %v", err)
			}
			tasks = append(tasks, replacement)
			assertAppReplayRegistrationCount(t, application, len(products), len(products)*charge)
		})
	}
}

func TestDefaultAppTaskReplayAdmissionCapsMixedProductsAtEightAnd512MiB(t *testing.T) {
	application := &App{workspace: "/workspace-a"}
	registrations := []struct {
		name string
		root bool
	}{
		{name: "Writing-1"},
		{name: "Game-1"},
		{name: "Lore-1"},
		{name: "Config-1"},
		{name: "Automation-1", root: true},
		{name: "Writing-2"},
		{name: "Game-2"},
		{name: "Automation-2", root: true},
	}
	if len(registrations) != apptask.DefaultMaxActiveReplayTasks || len(registrations)*2*apptask.DefaultRetainedByteLimit != apptask.DefaultActiveReplayByteLimit {
		t.Fatal("mixed product fixture no longer represents the default 8 Task / 512 MiB budget")
	}
	tasks := make([]*apptask.Task, 0, len(registrations)+1)
	t.Cleanup(func() {
		for _, task := range tasks {
			task.RejectStart(errors.New("test cleanup"))
			application.unregisterWorkspaceTask(task)
		}
	})
	for _, registration := range registrations {
		task, err := registerReplayProductTask(application, registration.name, registration.root, apptask.DefaultRetainedByteLimit)
		if err != nil {
			t.Fatalf("register %s Task: %v", registration.name, err)
		}
		tasks = append(tasks, task)
	}
	assertAppReplayRegistrationCount(t, application, apptask.DefaultMaxActiveReplayTasks, apptask.DefaultActiveReplayByteLimit)

	if task, err := registerReplayProductTask(application, "Lore-overflow", false, apptask.DefaultRetainedByteLimit); task != nil || !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("ninth mixed Task = task:%p err:%v, want ErrAgentReplayCapacity", task, err)
	}
	application.unregisterWorkspaceTask(tasks[0])
	replacement, err := registerReplayProductTask(application, "Config-replacement", false, apptask.DefaultRetainedByteLimit)
	if err != nil {
		t.Fatalf("register after releasing one mixed Task: %v", err)
	}
	tasks = append(tasks, replacement)
	assertAppReplayRegistrationCount(t, application, apptask.DefaultMaxActiveReplayTasks, apptask.DefaultActiveReplayByteLimit)
}

func TestFailedReplayRegistrationRollsBackWorkspaceLease(t *testing.T) {
	application := &App{workspace: "/workspace-a"}
	application.activeTaskReplay.Configure(apptask.ReplayAdmissionLimits{MaxActive: 1})
	first, err := registerReplayProductTask(application, "Writing", false, 128)
	if err != nil {
		t.Fatal(err)
	}
	if rejected, err := registerReplayProductTask(application, "Config", false, 128); rejected != nil || !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("rejected registration = task:%p err:%v", rejected, err)
	}

	application.mu.RLock()
	scope := application.workspaceScopes[lifecycleWorkspaceKey(application.workspace)]
	application.mu.RUnlock()
	first.RejectStart(errors.New("test cleanup"))
	application.unregisterWorkspaceTask(first)
	assertAppReplayRegistrationCount(t, application, 0, 0)

	// The failed Config registration acquired its workspace lease before replay
	// admission. Closing must drain after the sole published Writing Task exits.
	scope.BeginClose()
	waitCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := scope.Wait(waitCtx); err != nil {
		t.Fatalf("failed registration leaked workspace lifecycle lease: %v", err)
	}
}

func TestFailedTaskSettlementReleasesReplayRegistration(t *testing.T) {
	application := &App{workspace: "/workspace-a"}
	task, err := apptask.NewRegistered(func(task *apptask.Task) error {
		task.ConfigureRetention(0, 128)
		application.mu.Lock()
		defer application.mu.Unlock()
		return application.registerWorkspaceTaskLocked(task, application.workspace, true)
	}, func(_ context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		defer application.unregisterWorkspaceTask(task)
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": "settlement failed"}})
	})
	if err != nil {
		t.Fatal(err)
	}
	<-task.Done()
	if task.Status() != apptask.Failed {
		t.Fatalf("settled Task status = %s, want %s", task.Status(), apptask.Failed)
	}
	assertAppReplayRegistrationCount(t, application, 0, 0)
}

func registerReplayProductTask(application *App, product string, root bool, retainedByteLimit int) (*apptask.Task, error) {
	return apptask.NewDeferred(func(task *apptask.Task) error {
		task.ConfigureRetention(0, retainedByteLimit)
		application.mu.Lock()
		defer application.mu.Unlock()
		if root {
			if err := application.initializeLifecycleLocked(); err != nil {
				return err
			}
			return application.registerOwnedTaskLocked(task, "", application.rootScope)
		}
		if product == "" {
			return fmt.Errorf("test product is required")
		}
		return application.registerWorkspaceTaskLocked(task, application.workspace, true)
	})
}

func assertAppReplayRegistrationCount(t *testing.T, application *App, wantCount, wantBytes int) {
	t.Helper()
	application.mu.RLock()
	registered := len(application.workspaceTasks)
	leasing := len(application.workspaceTaskLeases)
	stopping := len(application.workspaceTaskStops)
	replaying := len(application.workspaceTaskReplayReservations)
	application.mu.RUnlock()
	stats := application.activeTaskReplay.Stats()
	active := stats.ActiveTasks
	activeBytes := stats.ActiveBytes
	if registered != wantCount || leasing != wantCount || stopping != wantCount || replaying != wantCount || active != wantCount || activeBytes != wantBytes {
		t.Fatalf("Task registry count mismatch registered=%d leases=%d stops=%d replay_records=%d active=%d bytes=%d, want count=%d bytes=%d", registered, leasing, stopping, replaying, active, activeBytes, wantCount, wantBytes)
	}
}

func TestWorkspaceTransitionFencesStartsAndWaitsForAdmittedTaskExit(t *testing.T) {
	application := &App{workspace: "/workspace-a"}
	cancelSeen := make(chan struct{})
	release := make(chan struct{})
	task, err := apptask.NewRegistered(func(task *apptask.Task) error {
		application.mu.Lock()
		defer application.mu.Unlock()
		if err := application.registerWorkspaceTaskLocked(task, application.workspace, true); err != nil {
			return err
		}
		application.activeTask = task
		return nil
	}, func(ctx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer application.unregisterWorkspaceTask(task)
		<-ctx.Done()
		close(cancelSeen)
		<-release
	})
	if err != nil {
		t.Fatal(err)
	}

	tasks, workspace, err := application.beginWorkspaceTransition()
	if err != nil {
		t.Fatal(err)
	}
	if workspace != "/workspace-a" || len(tasks) != 1 || tasks[0] != task {
		t.Fatalf("transition snapshot = workspace %q tasks %#v", workspace, tasks)
	}
	waited := make(chan error, 1)
	runAppErrorTestGoroutine(waited, "workspace transition task drain", func() error {
		return abortAndWaitTasks(context.Background(), tasks, workspace)
	})
	<-cancelSeen

	ran := make(chan struct{})
	_, err = apptask.NewRegistered(func(candidate *apptask.Task) error {
		application.mu.Lock()
		defer application.mu.Unlock()
		return application.registerWorkspaceTaskLocked(candidate, workspace, true)
	}, func(context.Context, *apptask.Task, func(agentrun.Event)) { close(ran) })
	if !errors.Is(err, ErrWorkspaceTransition) {
		t.Fatalf("start during transition error = %v, want ErrWorkspaceTransition", err)
	}
	select {
	case <-ran:
		t.Fatal("rejected task goroutine ran")
	default:
	}
	select {
	case err := <-waited:
		t.Fatalf("transition stopped waiting before task exit: %v", err)
	default:
	}

	close(release)
	if err := <-waited; err != nil {
		t.Fatal(err)
	}
	application.endWorkspaceTransition()
	if !task.Finished() {
		t.Fatal("transition returned before task fully finished")
	}
}

func TestLoreHostUnregisterReleasesProjectGenerationLease(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	requestContext, cancelRequest := context.WithCancel(context.Background())
	requestOperation, err := application.AcquireProjectOperation(requestContext, application.ProjectID())
	if err != nil {
		t.Fatal(err)
	}
	task, err := apptask.NewDeferredWithContext(requestOperation.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer task.RejectStart(errors.New("test cleanup"))
	projectID := application.ProjectID()
	_, layout, err := application.resolveProject(projectID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.registerProjectTask(task, projectID, layout.ContentRoot, layout.StateRoot); err != nil {
		t.Fatal(err)
	}
	application.mu.RLock()
	scope := application.projectScopes[projectID]
	registration := application.projectTasks[task]
	application.mu.RUnlock()
	if registration == nil {
		t.Fatal("Lore task was not registered against its Project")
	}
	if registration.operation == nil || registration.operation.lease == nil {
		t.Fatal("detached Lore task borrowed the request lease instead of owning a Project lease")
	}
	cancelRequest()
	requestOperation.Release()
	if err := task.Context().Err(); err != nil {
		t.Fatalf("request cancellation leaked into detached Lore task: %v", err)
	}

	(loreHost{app: application}).UnregisterLoreTask(task)
	application.mu.RLock()
	_, registered := application.projectTasks[task]
	application.mu.RUnlock()
	if registered {
		t.Fatal("Lore task cleanup left Project registry state")
	}

	scope.BeginClose()
	if err := scope.Wait(context.Background()); err != nil {
		t.Fatalf("released workspace generation did not drain: %v", err)
	}
}

func TestInactiveWorkspaceScopeDoesNotInvalidateCurrentStructuralFence(t *testing.T) {
	application := &App{workspace: "/workspace-a", workspaceGeneration: 7}

	op, err := application.acquireWorkspaceOperation(context.Background(), "/workspace-b", false)
	if err != nil {
		t.Fatal(err)
	}
	op.Release()

	application.mu.RLock()
	generation := application.workspaceGeneration
	application.mu.RUnlock()
	if generation != 7 {
		t.Fatalf("inactive workspace admission changed current structural generation: got %d want 7", generation)
	}
}
