package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/internal/workspacechange"
)

const shellStderrDiagnosticMaxBytes = 1024 * 1024

type agentStreamingShell struct {
	goos      string
	lookPath  func(string) (string, error)
	workspace string
	changes   *workspacechange.Service
}

func newAgentStreamingShell(workspace string) (*agentStreamingShell, error) {
	localWorkspace, err := agenttools.OpenWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	changes, err := workspacechange.ForWorkspace(localWorkspace.Root())
	if err != nil {
		return nil, fmt.Errorf("coordinate shell workspace: %w", err)
	}
	return &agentStreamingShell{
		goos:      runtime.GOOS,
		lookPath:  exec.LookPath,
		workspace: localWorkspace.Root(),
		changes:   changes,
	}, nil
}

func (s *agentStreamingShell) ExecuteStreaming(ctx context.Context, command string) (*agent.StreamReader[string], error) {
	if s == nil {
		return nil, fmt.Errorf("streaming shell is nil")
	}
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	if strings.TrimSpace(s.workspace) == "" {
		return nil, fmt.Errorf("shell workspace is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	reader, writer := agent.Pipe[string](100)
	go s.runForeground(ctx, command, writer)
	return reader, nil
}

func (s *agentStreamingShell) Run(ctx context.Context, command string) (*agent.StreamReader[string], error) {
	return s.ExecuteStreaming(ctx, command)
}

func (s *agentStreamingShell) runForeground(ctx context.Context, command string, writer *agent.StreamWriter[string]) {
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = writer.Send("", fmt.Errorf("shell execution panic: %v\n%s", recovered, string(debug.Stack())))
		}
		writer.Close()
	}()

	run := func() error {
		name, args := shellCommandArgs(s.goos, s.lookPath, command)
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = s.workspace
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("create command stdout pipe: %w", err)
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			_ = stdout.Close()
			return fmt.Errorf("create command stderr pipe: %w", err)
		}
		if err := cmd.Start(); err != nil {
			_ = stdout.Close()
			_ = stderr.Close()
			return fmt.Errorf("start command: %w", err)
		}

		stderrResult := make(chan shellStderrResult, 1)
		go readShellStderr(stderr, stderrResult)
		hasOutput, stdoutErr := streamShellStdout(ctx, cmd, stdout, writer)
		diagnostic := <-stderrResult
		waitErr := cmd.Wait()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if stdoutErr != nil {
			return stdoutErr
		}
		if diagnostic.err != nil {
			return diagnostic.err
		}
		if waitErr != nil {
			var exitError *exec.ExitError
			if !errors.As(waitErr, &exitError) {
				return fmt.Errorf("command failed: %w", waitErr)
			}
			parts := []string{fmt.Sprintf("[Command failed with exit code %d]", exitError.ExitCode())}
			if diagnostic.content != "" {
				parts = append(parts, "[stderr]:\n"+diagnostic.content)
			}
			if diagnostic.truncated {
				parts = append(parts, "[stderr was truncated due to size limits]")
			}
			_ = writer.Send("\n"+strings.Join(parts, "\n"), nil)
			return nil
		}
		if !hasOutput {
			_ = writer.Send("[Command executed successfully with no output]", nil)
		}
		return nil
	}

	var err error
	if s.changes != nil {
		err = s.changes.WithExclusiveWorkspace(ctx, run)
	} else {
		err = run()
	}
	if err != nil {
		_ = writer.Send("", err)
	}
}

type shellStderrResult struct {
	content   string
	truncated bool
	err       error
}

func readShellStderr(stderr io.ReadCloser, result chan<- shellStderrResult) {
	var output shellStderrResult
	defer func() {
		if recovered := recover(); recovered != nil {
			output.err = fmt.Errorf("read command stderr panic: %v\n%s", recovered, string(debug.Stack()))
		}
		_ = stderr.Close()
		result <- output
	}()

	limited := &io.LimitedReader{R: stderr, N: shellStderrDiagnosticMaxBytes + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		output.err = fmt.Errorf("read command stderr: %w", err)
		return
	}
	if len(data) > shellStderrDiagnosticMaxBytes {
		data = data[:shellStderrDiagnosticMaxBytes]
		output.truncated = true
	}
	if _, err := io.Copy(io.Discard, stderr); err != nil && output.err == nil {
		output.err = fmt.Errorf("drain command stderr: %w", err)
	}
	output.content = string(data)
}

func streamShellStdout(ctx context.Context, cmd *exec.Cmd, stdout io.ReadCloser, writer *agent.StreamWriter[string]) (bool, error) {
	defer stdout.Close()
	buffer := make([]byte, 32*1024)
	hasOutput := false
	for {
		count, err := stdout.Read(buffer)
		if count > 0 {
			hasOutput = true
			if writer.Send(string(buffer[:count]), nil) {
				killShellProcess(cmd)
				return hasOutput, agent.ErrRecvAfterClosed
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return hasOutput, nil
			}
			return hasOutput, fmt.Errorf("read command stdout: %w", err)
		}
		select {
		case <-ctx.Done():
			killShellProcess(cmd)
			return hasOutput, ctx.Err()
		default:
		}
	}
}

func shellCommandArgs(goos string, lookPath func(string) (string, error), command string) (string, []string) {
	if goos != "windows" {
		return "/bin/sh", []string{"-c", command}
	}

	shell := lookupShell(lookPath, "pwsh")
	if shell == "" {
		shell = lookupShell(lookPath, "powershell.exe")
	}
	if shell == "" {
		shell = "powershell.exe"
	}

	args := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}
	if isWindowsPowerShell(shell) {
		args = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command}
	}
	return shell, args
}

func lookupShell(lookPath func(string) (string, error), name string) string {
	if lookPath == nil {
		return ""
	}
	path, err := lookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func isWindowsPowerShell(shell string) bool {
	base := strings.ToLower(filepath.Base(shell))
	return base == "powershell.exe" || base == "powershell"
}

func killShellProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
