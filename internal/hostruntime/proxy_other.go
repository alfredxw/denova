//go:build !windows && !darwin

package hostruntime

import "context"

// Linux and WSL use the host's exported proxy environment.
func systemProxyEnvironment(context.Context) ([]string, error) { return nil, nil }
