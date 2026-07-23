package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"denova/internal/agent"
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
			application.activeTaskReplay.countLimit = test.countLimit
			application.activeTaskReplay.byteLimit = test.byteLimit
			tasks := make([]*Task, 0, len(products)+1)
			t.Cleanup(func() {
				for _, task := range tasks {
					task.failBeforeStart(errors.New("test cleanup"))
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

func TestWorkspaceTaskUnregisterWakesDurableEffectReconciliation(t *testing.T) {
	application := &App{
		workspace:            "/workspace-a",
		automationEffectWake: make(chan struct{}, 1),
	}
	task, err := registerReplayProductTask(application, "Writing", false, 128)
	if err != nil {
		t.Fatalf("register Writing Task: %v", err)
	}

	task.failBeforeStart(errors.New("settled test Task"))
	application.unregisterWorkspaceTask(task)
	select {
	case <-application.automationEffectWake:
	default:
		t.Fatal("Task settlement did not wake durable HostEffect reconciliation")
	}
	application.unregisterWorkspaceTask(task)
	select {
	case <-application.automationEffectWake:
		t.Fatal("duplicate Task unregister emitted another reconciliation wake")
	default:
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
	if len(registrations) != maxActiveReplayTasks || len(registrations)*2*defaultTaskRetainedByteLimit != defaultActiveReplayByteLimit {
		t.Fatal("mixed product fixture no longer represents the default 8 Task / 512 MiB budget")
	}
	tasks := make([]*Task, 0, len(registrations)+1)
	t.Cleanup(func() {
		for _, task := range tasks {
			task.failBeforeStart(errors.New("test cleanup"))
			application.unregisterWorkspaceTask(task)
		}
	})
	for _, registration := range registrations {
		task, err := registerReplayProductTask(application, registration.name, registration.root, defaultTaskRetainedByteLimit)
		if err != nil {
			t.Fatalf("register %s Task: %v", registration.name, err)
		}
		tasks = append(tasks, task)
	}
	assertAppReplayRegistrationCount(t, application, maxActiveReplayTasks, defaultActiveReplayByteLimit)

	if task, err := registerReplayProductTask(application, "Lore-overflow", false, defaultTaskRetainedByteLimit); task != nil || !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("ninth mixed Task = task:%p err:%v, want ErrAgentReplayCapacity", task, err)
	}
	application.unregisterWorkspaceTask(tasks[0])
	replacement, err := registerReplayProductTask(application, "Config-replacement", false, defaultTaskRetainedByteLimit)
	if err != nil {
		t.Fatalf("register after releasing one mixed Task: %v", err)
	}
	tasks = append(tasks, replacement)
	assertAppReplayRegistrationCount(t, application, maxActiveReplayTasks, defaultActiveReplayByteLimit)
}

func TestFailedReplayRegistrationRollsBackWorkspaceLease(t *testing.T) {
	application := &App{workspace: "/workspace-a"}
	application.activeTaskReplay.countLimit = 1
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
	first.failBeforeStart(errors.New("test cleanup"))
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
	task, err := NewRegisteredTask(func(task *Task) error {
		task.retainedByteLimit = 128
		application.mu.Lock()
		defer application.mu.Unlock()
		return application.registerWorkspaceTaskLocked(task, application.workspace, true)
	}, func(_ context.Context, task *Task, emit func(agent.Event)) {
		defer application.unregisterWorkspaceTask(task)
		emit(agent.Event{Type: "error", Data: map[string]string{"message": "settlement failed"}})
	})
	if err != nil {
		t.Fatal(err)
	}
	<-task.Done()
	if task.Status() != TaskError {
		t.Fatalf("settled Task status = %s, want %s", task.Status(), TaskError)
	}
	assertAppReplayRegistrationCount(t, application, 0, 0)
}

func registerReplayProductTask(application *App, product string, root bool, retainedByteLimit int) (*Task, error) {
	return NewDeferredRegisteredTask(func(task *Task) error {
		task.retainedByteLimit = retainedByteLimit
		application.mu.Lock()
		defer application.mu.Unlock()
		if root {
			return (&AutomationAppService{app: application}).registerAutomationTaskLocked(task, "")
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
	application.activeTaskReplay.mu.Lock()
	active := len(application.activeTaskReplay.active)
	activeBytes := application.activeTaskReplay.activeBytesLocked()
	application.activeTaskReplay.mu.Unlock()
	if registered != wantCount || leasing != wantCount || stopping != wantCount || replaying != wantCount || active != wantCount || activeBytes != wantBytes {
		t.Fatalf("Task registry count mismatch registered=%d leases=%d stops=%d replay_records=%d active=%d bytes=%d, want count=%d bytes=%d", registered, leasing, stopping, replaying, active, activeBytes, wantCount, wantBytes)
	}
}

func TestWorkspaceTransitionFencesStartsAndWaitsForAdmittedTaskExit(t *testing.T) {
	application := &App{workspace: "/workspace-a"}
	cancelSeen := make(chan struct{})
	release := make(chan struct{})
	task, err := NewRegisteredTask(func(task *Task) error {
		application.mu.Lock()
		defer application.mu.Unlock()
		if err := application.registerWorkspaceTaskLocked(task, application.workspace, true); err != nil {
			return err
		}
		application.activeTask = task
		return nil
	}, func(ctx context.Context, task *Task, _ func(agent.Event)) {
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
	_, err = NewRegisteredTask(func(candidate *Task) error {
		application.mu.Lock()
		defer application.mu.Unlock()
		return application.registerWorkspaceTaskLocked(candidate, workspace, true)
	}, func(context.Context, *Task, func(agent.Event)) { close(ran) })
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

func TestClearLoreImageTaskReleasesWorkspaceGenerationLease(t *testing.T) {
	application := &App{workspace: "/workspace-a"}
	task := &Task{}
	application.mu.Lock()
	if err := application.registerWorkspaceTaskLocked(task, application.workspace, true); err != nil {
		application.mu.Unlock()
		t.Fatal(err)
	}
	application.activeLoreImageTask = task
	scope := application.workspaceScopes[lifecycleWorkspaceKey(application.workspace)]
	application.mu.Unlock()

	(&LoreAppService{app: application}).clearLoreImageTask(task)
	application.mu.RLock()
	_, registered := application.workspaceTasks[task]
	_, hasLease := application.workspaceTaskLeases[task]
	_, hasStop := application.workspaceTaskStops[task]
	_, hasReplay := application.workspaceTaskReplayReservations[task]
	active := application.activeLoreImageTask
	application.mu.RUnlock()
	if registered || hasLease || hasStop || hasReplay || active != nil {
		t.Fatalf("lore task cleanup left registry state: registered=%t lease=%t stop=%t replay=%t active=%p", registered, hasLease, hasStop, hasReplay, active)
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
