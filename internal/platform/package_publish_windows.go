package platform

import (
	"errors"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// A freshly written release can still be held by another Windows process.
// Keep one atomic rename and retry only transient lock/permission errors while
// its destination remains absent. This infrastructure wait never copies over
// an existing release or retries the surrounding package installation.
func publishReleaseDirectory(staging, destination string) error {
	deadline := time.Now().Add(time.Second)
	for attempt := 0; ; attempt++ {
		err := os.Rename(staging, destination)
		if err == nil {
			if attempt > 0 {
				slog.Info("platform_release_publish_recovered", "destination", destination, "retries", attempt)
			}
			return nil
		}
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return err
		}
		// A competing publisher is not a transient handle: leave its bytes intact
		// and let the caller reconcile the failed installation explicitly.
		if _, statErr := os.Lstat(destination); !os.IsNotExist(statErr) {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			slog.Warn("platform_release_publish_retry_exhausted", "destination", destination, "error", err)
			return err
		}
		if attempt == 0 {
			slog.Warn("platform_release_publish_retry", "destination", destination, "error", err)
		}
		time.Sleep(min(50*time.Millisecond, remaining))
	}
}
