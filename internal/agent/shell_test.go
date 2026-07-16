package agent

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk/filesystem"
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

func TestAgentStreamingShellStreamsOutput(t *testing.T) {
	sh := &agentStreamingShell{goos: runtime.GOOS, lookPath: exec.LookPath}
	command := "printf 'nova-shell\\n'"
	if runtime.GOOS == "windows" {
		command = "Write-Output nova-shell"
	}

	output, err := collectShellOutput(context.Background(), sh, command)
	if err != nil {
		t.Fatalf("execute streaming failed: %v", err)
	}
	if !strings.Contains(output, "nova-shell") {
		t.Fatalf("expected streamed output, got %q", output)
	}
}

func TestAgentStreamingShellReportsExitCode(t *testing.T) {
	sh := &agentStreamingShell{goos: runtime.GOOS, lookPath: exec.LookPath}
	sr, err := sh.ExecuteStreaming(context.Background(), &filesystem.ExecuteRequest{Command: "exit 3"})
	if err != nil {
		t.Fatalf("execute streaming failed: %v", err)
	}

	var exitCode *int
	for {
		resp, recvErr := sr.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			t.Fatalf("receive failed: %v", recvErr)
		}
		if resp != nil && resp.ExitCode != nil {
			exitCode = resp.ExitCode
		}
	}
	if exitCode == nil || *exitCode != 3 {
		t.Fatalf("expected exit code 3, got %v", exitCode)
	}
}

func TestAgentStreamingShellRejectsBackgroundExecution(t *testing.T) {
	sh := &agentStreamingShell{goos: runtime.GOOS, lookPath: exec.LookPath}
	_, err := sh.ExecuteStreaming(context.Background(), &filesystem.ExecuteRequest{
		Command:            "echo unsafe",
		RunInBackendGround: true,
	})
	if err == nil || !strings.Contains(err.Error(), "background shell execution is disabled") {
		t.Fatalf("background execution should be rejected, got %v", err)
	}
}

func collectShellOutput(ctx context.Context, sh *agentStreamingShell, command string) (string, error) {
	sr, err := sh.ExecuteStreaming(ctx, &filesystem.ExecuteRequest{Command: command})
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for {
		resp, recvErr := sr.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return output.String(), recvErr
		}
		if resp != nil {
			output.WriteString(resp.Output)
		}
	}
	return output.String(), nil
}
