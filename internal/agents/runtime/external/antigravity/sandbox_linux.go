package antigravity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func detectOSSandbox() error {
	if raw, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone"); err == nil {
		if strings.TrimSpace(string(raw)) == "0" {
			return errors.New("Linux unprivileged user namespaces are disabled (unprivileged_userns_clone=0)")
		}
	}
	helper := ""
	for _, candidate := range []string{"/bin/true", "/usr/bin/true", "true"} {
		if path, err := exec.LookPath(candidate); err == nil {
			helper = path
			break
		}
	}
	if helper == "" {
		return errors.New("cannot probe Linux user namespaces: true binary not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), infrastructureTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper)
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWUSER}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Linux unprivileged user namespaces are unavailable for terminal sandbox: %w", err)
	}
	return nil
}
