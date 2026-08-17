package app

import "fmt"

// runAppErrorTestGoroutine mirrors the App's production panic boundary and
// always reports exactly one result so the coordinating test cannot hang.
func runAppErrorTestGoroutine(destination chan<- error, scope string, run func() error) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- fmt.Errorf("%s panic: %v", scope, recovered)
			}
		}()
		destination <- run()
	}()
}
