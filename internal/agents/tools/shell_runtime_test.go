package tools

import (
	"context"
	"runtime"
	"strings"
	"testing"

	agenttools "github.com/alfredxw/denova/agent/tools"
)

func TestAgentCommandRunnerExecutesFromWorkspaceThroughReusableProcessModule(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash assertion is Unix-specific")
	}
	workspace, err := agenttools.OpenWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := newAgentCommandRunner(workspace, agenttools.ShellBash, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), agenttools.CommandRequest{Command: "printf 'nova-shell:%s' \"$PWD\""}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Output, "nova-shell") || !strings.Contains(result.Output, workspace.Root()) {
		t.Fatalf("command result = %#v", result)
	}
}

func TestAgentCommandRunnerHonorsCanceledContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash assertion is Unix-specific")
	}
	workspace, err := agenttools.OpenWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := newAgentCommandRunner(workspace, agenttools.ShellBash, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runner.Run(ctx, agenttools.CommandRequest{Command: "echo should-not-run"}, nil)
	if err != nil || result.Status != agenttools.ProcessStatusCancelled {
		t.Fatalf("canceled command result = %#v error = %v", result, err)
	}
}

func TestAgentCommandRunnerRejectsMissingWorkspace(t *testing.T) {
	if _, err := newAgentCommandRunner(nil, agenttools.ShellBash, "", nil, ""); err == nil {
		t.Fatal("nil workspace was accepted")
	}
}
