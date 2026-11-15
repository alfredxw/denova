package antigravity

import (
	"context"
	"fmt"
	"os/exec"
)

func detectOSSandbox() error {
	path, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return fmt.Errorf("macOS sandbox-exec is unavailable: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), infrastructureTimeout)
	defer cancel()
	if err := exec.CommandContext(ctx, path, "-p", "(version 1)(allow default)", "/usr/bin/true").Run(); err != nil {
		return fmt.Errorf("macOS sandbox-exec cannot enforce terminal sandbox: %w", err)
	}
	return nil
}
