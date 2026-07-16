package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/schema"

	"denova/internal/workspacechange"
)

type agentStreamingShell struct {
	goos      string
	lookPath  func(string) (string, error)
	workspace string
	changes   *workspacechange.Service
}

func newAgentStreamingShell(workspaces ...string) filesystem.StreamingShell {
	shell := &agentStreamingShell{
		goos:     runtime.GOOS,
		lookPath: exec.LookPath,
	}
	if len(workspaces) > 0 {
		shell.workspace = strings.TrimSpace(workspaces[0])
		if shell.workspace != "" {
			shell.workspace = filepath.Clean(shell.workspace)
			shell.changes, _ = workspacechange.ForWorkspace(shell.workspace)
		}
	}
	return shell
}

func (s *agentStreamingShell) ExecuteStreaming(ctx context.Context, input *filesystem.ExecuteRequest) (*schema.StreamReader[*filesystem.ExecuteResponse], error) {
	if input == nil {
		return nil, fmt.Errorf("execute request is nil")
	}
	if input.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if input.RunInBackendGround {
		return nil, fmt.Errorf("background shell execution is disabled because its lifetime cannot be coordinated with workspace writes; run the command in the foreground")
	}

	cmd, stdout, stderr, err := s.initCommand(ctx, input.Command)
	if err != nil {
		return nil, err
	}

	if s.workspace != "" {
		cmd.Dir = s.workspace
	}

	sr, w := schema.Pipe[*filesystem.ExecuteResponse](100)
	go s.runForeground(ctx, cmd, stdout, stderr, w)
	return sr, nil
}

func (s *agentStreamingShell) runForeground(
	ctx context.Context,
	cmd *exec.Cmd,
	stdout, stderr io.ReadCloser,
	w *schema.StreamWriter[*filesystem.ExecuteResponse],
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			sendShellErrorAndClose(w, fmt.Errorf("shell execution panic: %v\n%s", recovered, string(debug.Stack())))
		}
	}()
	run := func() error {
		if err := cmd.Start(); err != nil {
			_ = stdout.Close()
			_ = stderr.Close()
			return fmt.Errorf("failed to start command: %w", err)
		}
		streamShellOutput(ctx, cmd, stdout, stderr, w)
		return nil
	}
	var err error
	if s.changes != nil {
		err = s.changes.WithExclusiveWorkspace(ctx, run)
	} else {
		err = run()
	}
	if err != nil {
		sendShellErrorAndClose(w, err)
	}
}

func (s *agentStreamingShell) initCommand(ctx context.Context, command string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	name, args := shellCommandArgs(s.goos, s.lookPath, command)
	cmd := exec.CommandContext(ctx, name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	return cmd, stdout, stderr, nil
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

func streamShellOutput(ctx context.Context, cmd *exec.Cmd, stdout, stderr io.ReadCloser, w *schema.StreamWriter[*filesystem.ExecuteResponse]) {
	defer func() {
		if pe := recover(); pe != nil {
			w.Send(nil, fmt.Errorf("panic: %v,\n stack: %s", pe, string(debug.Stack())))
		}
		w.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	stderrData, stderrErr := readShellStderrAsync(stderr)
	hasOutput, err := streamShellStdout(ctx, cmd, stdout, w)
	if err != nil {
		w.Send(nil, err)
		return
	}

	if err := <-stderrErr; err != nil {
		w.Send(nil, err)
		return
	}
	handleShellCompletion(cmd, stderrData, hasOutput, w)
}

func readShellStderrAsync(stderr io.Reader) (*[]byte, <-chan error) {
	stderrData := new([]byte)
	stderrErr := make(chan error, 1)
	go func() {
		defer func() {
			if pe := recover(); pe != nil {
				stderrErr <- fmt.Errorf("panic: %v,\n stack: %s", pe, string(debug.Stack()))
				return
			}
			close(stderrErr)
		}()
		var err error
		*stderrData, err = io.ReadAll(stderr)
		if err != nil {
			stderrErr <- fmt.Errorf("failed to read stderr: %w", err)
		}
	}()
	return stderrData, stderrErr
}

func streamShellStdout(ctx context.Context, cmd *exec.Cmd, stdout io.Reader, w *schema.StreamWriter[*filesystem.ExecuteResponse]) (bool, error) {
	reader := bufio.NewReader(stdout)
	hasOutput := false
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			hasOutput = true
			select {
			case <-ctx.Done():
				killShellProcess(cmd)
				return hasOutput, ctx.Err()
			default:
				w.Send(&filesystem.ExecuteResponse{Output: line}, nil)
			}
		}
		if err != nil {
			if err != io.EOF {
				return hasOutput, fmt.Errorf("error reading stdout: %w", err)
			}
			break
		}
	}
	return hasOutput, nil
}

func handleShellCompletion(cmd *exec.Cmd, stderrData *[]byte, hasOutput bool, w *schema.StreamWriter[*filesystem.ExecuteResponse]) {
	if err := cmd.Wait(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode := exitError.ExitCode()
			parts := []string{fmt.Sprintf("command exited with non-zero code %d", exitCode)}
			if stderrStr := string(*stderrData); stderrStr != "" {
				parts = append(parts, "[stderr]:\n"+stderrStr)
			}
			w.Send(&filesystem.ExecuteResponse{
				Output:   strings.Join(parts, "\n"),
				ExitCode: &exitCode,
			}, nil)
			return
		}
		w.Send(nil, fmt.Errorf("command failed: %w", err))
		return
	}

	if !hasOutput {
		exitCode := 0
		w.Send(&filesystem.ExecuteResponse{ExitCode: &exitCode}, nil)
	}
}

func sendShellErrorAndClose(w *schema.StreamWriter[*filesystem.ExecuteResponse], err error) {
	defer func() {
		if pe := recover(); pe != nil {
			log.Printf("[agent shell] send error panic: %v\n%s", pe, string(debug.Stack()))
		}
		w.Close()
	}()
	w.Send(nil, err)
}

func killShellProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
