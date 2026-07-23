package runtime_test

import "fmt"

func runExternalTestGoroutine[T any](destination chan<- T, panicValue func(any) T, run func() T) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- panicValue(recovered)
			}
		}()
		destination <- run()
	}()
}

func runExternalErrorTestGoroutine(destination chan<- error, scope string, run func() error) {
	runExternalTestGoroutine(destination, func(recovered any) error {
		return fmt.Errorf("%s panic: %v", scope, recovered)
	}, run)
}
