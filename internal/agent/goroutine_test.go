package agent

import (
	"fmt"

	runstate "denova/internal/agent/runtime"
)

// runOutcomeTestGoroutine keeps asynchronous Agent tests subject to the same
// panic boundary as production goroutines while preserving their normal result
// channel. A recovered panic therefore fails the assertion path instead of
// crashing the whole test binary or leaving the caller blocked forever.
func runOutcomeTestGoroutine(destination chan<- RunOutcome, scope string, run func() RunOutcome) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- RunOutcome{
					Status: RunOutcomeFailed,
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
