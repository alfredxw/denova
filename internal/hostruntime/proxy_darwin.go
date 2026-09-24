package hostruntime

import (
	"context"
	"os/exec"
)

func systemProxyEnvironment(ctx context.Context) ([]string, error) {
	output, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return nil, err
	}
	return macProxyEnvironment(string(output)), nil
}
