package app

import (
	"context"
	"testing"

	"denova/config"
	"denova/internal/agent"
)

func TestAppCloseStopsRegisteredBackgroundTasks(t *testing.T) {
	agentStarted := make(chan struct{})
	interactiveStarted := make(chan struct{})
	novelImportStarted := make(chan struct{})
	configManagerStarted := make(chan struct{})
	agentTask := blockingLifecycleTask(agentStarted)
	interactiveTask := blockingLifecycleTask(interactiveStarted)
	novelImportTask := blockingLifecycleTask(novelImportStarted)
	configManagerTask := blockingLifecycleTask(configManagerStarted)
	t.Cleanup(func() {
		agentTask.Abort()
		interactiveTask.Abort()
		novelImportTask.Abort()
		configManagerTask.Abort()
	})

	application := &App{
		cfg: &config.Config{},
		agentTaskRuns: map[string]*agentTaskRun{
			"agent": {task: agentTask},
		},
		interactiveTaskRuns: map[string]*interactiveTaskRun{
			"story": {task: interactiveTask},
		},
		novelImportTasks: map[string]*novelImportTaskState{
			"novel": {task: novelImportTask},
		},
		configManagerTaskRuns: map[string]*configManagerTaskRun{
			"config": {task: configManagerTask},
		},
	}
	<-agentStarted
	<-interactiveStarted
	<-novelImportStarted
	<-configManagerStarted

	application.Close()

	if agentTask.Status() != TaskAborted || !agentTask.Finished() {
		t.Fatalf("Agent task remained active after App.Close: status=%s finished=%t", agentTask.Status(), agentTask.Finished())
	}
	if interactiveTask.Status() != TaskAborted || !interactiveTask.Finished() {
		t.Fatalf("interactive task remained active after App.Close: status=%s finished=%t", interactiveTask.Status(), interactiveTask.Finished())
	}
	if novelImportTask.Status() != TaskAborted || !novelImportTask.Finished() {
		t.Fatalf("novel import task remained active after App.Close: status=%s finished=%t", novelImportTask.Status(), novelImportTask.Finished())
	}
	if configManagerTask.Status() != TaskAborted || !configManagerTask.Finished() {
		t.Fatalf("config-manager task remained active after App.Close: status=%s finished=%t", configManagerTask.Status(), configManagerTask.Finished())
	}
}

func blockingLifecycleTask(started chan<- struct{}) *Task {
	return NewTask(func(ctx context.Context, _ *Task, _ func(agent.Event)) {
		close(started)
		<-ctx.Done()
	})
}
