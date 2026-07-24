//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package filelease

import "context"

// The process lease remains the portability fallback where syscall.Flock is
// unavailable. Supported macOS/Linux production builds take both leases.
func acquireOS(ctx context.Context, _ string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return func() error { return nil }, nil
}
