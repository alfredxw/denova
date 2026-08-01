package harness

import (
	"fmt"

	"denova/internal/agents/chat"
	"denova/internal/agents/run"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func newTestExecutor(policy agentrun.LoopPolicy) *chat.Executor {
	return chat.NewExecutor(policy)
}

func runOutcomeTestGoroutine(destination chan<- agentrun.Outcome, scope string, run func() agentrun.Outcome) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- agentrun.Outcome{
					Status: agentrun.OutcomeFailed,
					Error:  fmt.Errorf("%s panic: %v", scope, recovered),
				}
			}
		}()
		destination <- run()
	}()
}

type harnessEngineTestResult struct {
	result runstate.EngineResult
	err    error
}

func runHarnessEngineTestGoroutine(
	destination chan<- harnessEngineTestResult,
	scope string,
	run func() (runstate.EngineResult, error),
) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- harnessEngineTestResult{err: fmt.Errorf("%s panic: %v", scope, recovered)}
			}
		}()
		result, err := run()
		destination <- harnessEngineTestResult{result: result, err: err}
	}()
}

func runErrorTestGoroutine(destination chan<- error, scope string, run func() error) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- fmt.Errorf("%s panic: %v", scope, recovered)
			}
		}()
		destination <- run()
	}()
}

func countEventType(events []agentrun.Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func mustRuntimeBinding(binding agentrun.RuntimeBinding) runstate.BindingRef {
	ref, err := binding.Ref()
	if err != nil {
		panic(err)
	}
	return ref
}
