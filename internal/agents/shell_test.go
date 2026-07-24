package agents

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellCommandArgsUsesUnixShellOutsideWindows(t *testing.T) {
	name, args := shellCommandArgs("darwin", nil, "pwd")
	if name != "/bin/sh" {
		t.Fatalf("expected /bin/sh, got %q", name)
	}
	if got := strings.Join(args, " "); got != "-c pwd" {
		t.Fatalf("unexpected args: %q", got)
	}
}

func TestShellCommandArgsPrefersPwshOnWindows(t *testing.T) {
	name, args := shellCommandArgs("windows", func(name string) (string, error) {
		if name == "pwsh" {
			return "C:/Program Files/PowerShell/7/pwsh.exe", nil
		}
		return "", exec.ErrNotFound
	}, "Get-Location")

	if !strings.HasSuffix(strings.ToLower(name), "pwsh.exe") {
		t.Fatalf("expected pwsh on Windows when available, got %q", name)
	}
	if strings.Contains(strings.Join(args, " "), "ExecutionPolicy") {
		t.Fatalf("pwsh args should not include Windows PowerShell execution policy: %#v", args)
	}
	if args[len(args)-2] != "-Command" || args[len(args)-1] != "Get-Location" {
		t.Fatalf("PowerShell command args not wired correctly: %#v", args)
	}
}

func TestShellCommandArgsFallsBackToWindowsPowerShell(t *testing.T) {
	name, args := shellCommandArgs("windows", func(name string) (string, error) {
		if name == "powershell.exe" {
			return "C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe", nil
		}
		return "", exec.ErrNotFound
	}, "Get-Location")

	if !strings.HasSuffix(strings.ToLower(name), "powershell.exe") {
		t.Fatalf("expected powershell.exe fallback, got %q", name)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-ExecutionPolicy Bypass") {
		t.Fatalf("Windows PowerShell should run with execution policy bypass: %#v", args)
	}
	if args[len(args)-2] != "-Command" || args[len(args)-1] != "Get-Location" {
		t.Fatalf("PowerShell command args not wired correctly: %#v", args)
	}
}

func TestAgentStreamingShellStreamsOutputFromWorkspace(t *testing.T) {
	workspace := t.TempDir()
	shell := newTestAgentStreamingShell(t, workspace)
	command := "printf 'nova-shell:%s\\n' \"$PWD\""
	if runtime.GOOS == "windows" {
		command = "Write-Output nova-shell:$PWD"
	}

	output, err := collectShellOutput(context.Background(), shell, command)
	if err != nil {
		t.Fatalf("execute streaming failed: %v", err)
	}
	if !strings.Contains(output, "nova-shell") || !strings.Contains(filepath.Clean(output), filepath.Clean(workspace)) {
		t.Fatalf("expected streamed workspace output, got %q", output)
	}
}

func TestAgentStreamingShellReportsExitCodeAndBoundedStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exit syntax differs on Windows")
	}
	shell := newTestAgentStreamingShell(t, t.TempDir())
	output, err := collectShellOutput(context.Background(), shell, "printf 'failure detail' >&2; exit 3")
	if err != nil {
		t.Fatalf("execute streaming failed: %v", err)
	}
	if !strings.Contains(output, "exit code 3") || !strings.Contains(output, "failure detail") {
		t.Fatalf("expected exit diagnostics, got %q", output)
	}
}

func TestAgentStreamingShellHonorsCanceledContext(t *testing.T) {
	shell := newTestAgentStreamingShell(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := collectShellOutput(ctx, shell, "echo should-not-run")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled execute should return context cancellation, got %v", err)
	}
}

func newTestAgentStreamingShell(t *testing.T, workspace string) *agentStreamingShell {
	t.Helper()
	shell, err := newAgentStreamingShell(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return shell
}

func collectShellOutput(ctx context.Context, shell *agentStreamingShell, command string) (string, error) {
	reader, err := shell.ExecuteStreaming(ctx, command)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	var output strings.Builder
	for {
		fragment, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			return output.String(), nil
		}
		if recvErr != nil {
			return output.String(), recvErr
		}
		output.WriteString(fragment)
	}
}
