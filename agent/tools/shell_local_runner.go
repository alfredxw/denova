package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
	"github.com/charmbracelet/x/xpty"
)

const (
	maxCommandBytes              = 1024 * 1024
	maxCommandEnvironmentEntries = 128
	maxCommandEnvironmentBytes   = 64 * 1024
	defaultPTYWidth              = 120
	defaultPTYHeight             = 40
)

var portableEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LocalCommandRunner runs one foreground Bash or PowerShell command.
type LocalCommandRunner struct {
	workspace       *LocalWorkspace
	shell           ShellKind
	executable      string
	engine          string
	version         string
	baseEnvironment []string
	guard           CommandRunGuard
}

func (runner *LocalCommandRunner) Identity() agent.CapabilityIdentity {
	if runner == nil {
		return agent.CapabilityIdentity{}
	}
	root := ""
	if runner.workspace != nil {
		root = runner.workspace.Root()
	}
	return toolsetIdentity("tools.process.local", struct {
		Root       string
		Shell      ShellKind
		Executable string
		Engine     string
	}{root, runner.shell, runner.executable, runner.engine})
}

// NewLocalCommandRunner resolves the requested shell and binds it to a
// canonical workspace. It never substitutes /bin/sh for Bash.
func NewLocalCommandRunner(options CommandRunnerOptions) (*LocalCommandRunner, error) {
	if options.Workspace == nil || options.Workspace.Root() == "" {
		return nil, errors.New("command workspace is required")
	}
	executable, engine, err := resolveShellExecutable(options.Shell, options.Executable, exec.LookPath)
	if err != nil {
		return nil, err
	}
	return &LocalCommandRunner{
		workspace: options.Workspace, shell: options.Shell,
		executable: executable, engine: engine,
		version:         detectShellVersion(options.Shell, executable),
		baseEnvironment: normalizeBaseEnvironment(options.BaseEnvironment),
		guard:           options.Guard,
	}, nil
}

// Run executes a command while holding the configured product guard.
func (runner *LocalCommandRunner) Run(ctx context.Context, request CommandRequest, progress func(string)) (CommandResult, error) {
	if runner == nil || runner.workspace == nil || runner.executable == "" {
		return CommandResult{}, errors.New("command runner is not configured")
	}
	if strings.TrimSpace(request.Command) == "" {
		return CommandResult{}, errors.New("command is required")
	}
	if len(request.Command) > maxCommandBytes {
		return CommandResult{}, fmt.Errorf("command exceeds the %d-byte limit", maxCommandBytes)
	}
	if request.TimeoutSeconds < 0 {
		return CommandResult{}, errors.New("timeout_seconds cannot be negative")
	}
	if int64(request.TimeoutSeconds) > math.MaxInt64/int64(time.Second) {
		return CommandResult{}, errors.New("timeout_seconds is too large")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	relativeCwd, absoluteCwd, err := runner.resolveCommandDirectory(request.Cwd)
	if err != nil {
		return CommandResult{}, err
	}
	environment, err := commandEnvironment(runner.baseEnvironment, request.Env, request.PTY, absoluteCwd)
	if err != nil {
		return CommandResult{}, err
	}
	var result CommandResult
	run := func() error {
		var runErr error
		result, runErr = runner.runProcess(ctx, request, relativeCwd, absoluteCwd, environment, progress)
		return runErr
	}
	if runner.guard != nil {
		if err := runner.guard(ctx, run); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return runner.interruptedResult(request, relativeCwd, err), nil
			}
			return CommandResult{}, err
		}
	} else if err := run(); err != nil {
		return CommandResult{}, err
	}
	return result, nil
}

func (runner *LocalCommandRunner) runProcess(
	ctx context.Context,
	request CommandRequest,
	relativeCwd, absoluteCwd string,
	environment []string,
	progress func(string),
) (CommandResult, error) {
	runCtx := ctx
	cancel := func() {}
	if request.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(request.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	if err := runCtx.Err(); err != nil {
		return runner.interruptedResult(request, relativeCwd, err), nil
	}
	name, args := shellCommand(runner.shell, runner.executable, request.Command)
	command := exec.Command(name, args...)
	command.Dir = absoluteCwd
	command.Env = environment
	configureProcessTree(command)
	if err := runner.workspace.verifyRootIdentity(); err != nil {
		return CommandResult{}, err
	}
	var artifactWriter agent.ToolArtifactWriter
	if store := agent.ToolArtifactStoreFromContext(runCtx); store != nil {
		writer, beginErr := store.BeginToolArtifact(runCtx, agent.ToolArtifactRequest{
			ToolName: string(runner.shell), Purpose: agent.ToolArtifactPurposeCompleteToolOutput,
			MIMEType: "text/plain; charset=utf-8", Extension: "log",
			Description: "Complete merged stdout/stderr from one foreground command",
		})
		if beginErr != nil {
			// The store error may include a host path or credential. The command has
			// not started yet, so return a stable retryable classification only.
			return CommandResult{}, fmt.Errorf("begin command output artifact: %s", agent.ToolArtifactFailureBegin)
		}
		artifactWriter = writer
	}
	artifactFinalized := false
	defer func() {
		if artifactWriter != nil && !artifactFinalized {
			_ = artifactWriter.Abort()
		}
	}()

	reader, ptyHandle, wait, err := startCommand(command, request.PTY)
	if err != nil {
		return CommandResult{}, err
	}
	defer reader.Close()
	if ptyHandle != nil {
		defer ptyHandle.Close()
	}

	waitResult := make(chan error, 1)
	go func() {
		var waitErr error
		defer func() {
			if recovered := recover(); recovered != nil {
				waitErr = fmt.Errorf("wait for command panic: %v\n%s", recovered, debug.Stack())
			}
			cleanupProcessTree(command.Process)
			if ptyHandle != nil {
				finishPTYAfterWait(ptyHandle)
			}
			waitResult <- waitErr
		}()
		waitErr = wait()
	}()
	outputSettled := make(chan struct{})
	go func() {
		defer func() {
			_ = recover()
		}()
		select {
		case <-runCtx.Done():
			terminateProcessTree(command.Process)
			_ = reader.Close()
		case <-outputSettled:
		}
	}()

	decoder := utf8OutputDecoder{}
	capture := boundedOutput{limit: runner.workspace.Limits().MaxResultBytes}
	var outputBytes int64
	var artifactWriteErr error
	consumeOutput := func(delta string) {
		if delta == "" {
			return
		}
		outputBytes += int64(len(delta))
		if progress != nil {
			progress(delta)
		}
		capture.Write(delta)
		if artifactWriter != nil && artifactWriteErr == nil {
			_, artifactWriteErr = io.WriteString(artifactWriter, delta)
		}
	}
	buffer := make([]byte, 32*1024)
	var readErr error
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			consumeOutput(decoder.Feed(buffer[:count], false))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				readErr = err
			}
			break
		}
	}
	if tail := decoder.Feed(nil, true); tail != "" {
		consumeOutput(tail)
	}
	close(outputSettled)
	waitErr := <-waitResult
	var artifact *agent.ToolArtifactRef
	artifactError := ""
	if artifactWriter != nil {
		if artifactWriteErr != nil {
			artifactError = agent.ToolArtifactFailureWrite
			_ = artifactWriter.Abort()
		} else {
			published, commitErr := artifactWriter.Commit()
			if commitErr != nil {
				artifactError = agent.ToolArtifactFailureCommit
				_ = artifactWriter.Abort()
			} else {
				artifact = &published
			}
		}
		artifactFinalized = true
	}
	if readErr != nil && runCtx.Err() == nil && !request.PTY {
		return CommandResult{}, fmt.Errorf("read command output: %w", readErr)
	}
	if waitErr != nil {
		var exitError *exec.ExitError
		if runCtx.Err() == nil && !errors.As(waitErr, &exitError) {
			return CommandResult{}, fmt.Errorf("wait for command: %w", waitErr)
		}
	}

	result := CommandResult{
		Status: ProcessStatusSuccess, Shell: runner.shell, Engine: runner.engine,
		Version: runner.version, ExitCode: 0, Output: capture.String(),
		OutputBytes: outputBytes, OutputTruncated: capture.truncated,
		Artifact: artifact, ArtifactError: artifactError, Cwd: relativeCwd,
		PTY: request.PTY, TimeoutSeconds: request.TimeoutSeconds,
	}
	if runErr := runCtx.Err(); runErr != nil {
		interrupted := runner.interruptedResult(request, relativeCwd, runErr)
		interrupted.Output, interrupted.OutputTruncated = result.Output, result.OutputTruncated
		interrupted.OutputBytes, interrupted.Artifact, interrupted.ArtifactError = result.OutputBytes, result.Artifact, result.ArtifactError
		return interrupted, nil
	}
	if waitErr != nil {
		var exitError *exec.ExitError
		if errors.As(waitErr, &exitError) {
			result.Status = ProcessStatusFailed
			result.ExitCode = exitError.ExitCode()
		}
	}
	return result, nil
}

func (runner *LocalCommandRunner) interruptedResult(request CommandRequest, cwd string, err error) CommandResult {
	status := ProcessStatusCancelled
	if errors.Is(err, context.DeadlineExceeded) {
		status = ProcessStatusTimedOut
	}
	return CommandResult{
		Status: status, Shell: runner.shell, Engine: runner.engine, Version: runner.version,
		ExitCode: -1, Cwd: cwd, PTY: request.PTY, TimeoutSeconds: request.TimeoutSeconds,
	}
}

func startCommand(command *exec.Cmd, usePTY bool) (io.ReadCloser, xpty.Pty, func() error, error) {
	if usePTY {
		terminal, err := xpty.NewPty(defaultPTYWidth, defaultPTYHeight)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("allocate command PTY: %w", err)
		}
		if err := terminal.Start(command); err != nil {
			_ = terminal.Close()
			return nil, nil, nil, fmt.Errorf("start command in PTY: %w", err)
		}
		preparePTYAfterStart(terminal)
		return terminal, terminal, func() error { return xpty.WaitProcess(context.Background(), command) }, nil
	}
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create merged command output: %w", err)
	}
	command.Stdout = writePipe
	command.Stderr = writePipe
	if err := command.Start(); err != nil {
		_ = readPipe.Close()
		_ = writePipe.Close()
		return nil, nil, nil, fmt.Errorf("start command: %w", err)
	}
	_ = writePipe.Close()
	return readPipe, nil, command.Wait, nil
}

func (runner *LocalCommandRunner) resolveCommandDirectory(input string) (string, string, error) {
	if strings.TrimSpace(input) == "" {
		input = "."
	}
	relative, info, err := runner.workspace.stat(input, true)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("command cwd is not a directory: %s", relative)
	}
	absolute := filepath.Join(runner.workspace.Root(), filepath.FromSlash(relative))
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", "", fmt.Errorf("canonicalize command cwd: %w", err)
	}
	confirmed, err := filepath.Rel(runner.workspace.Root(), canonical)
	if err != nil || confirmed == ".." || strings.HasPrefix(confirmed, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("command cwd is outside the active workspace: %s", input)
	}
	return filepath.ToSlash(confirmed), canonical, nil
}

func commandEnvironment(base []string, extra map[string]string, pty bool, cwd string) ([]string, error) {
	if len(extra) > maxCommandEnvironmentEntries {
		return nil, fmt.Errorf("command env may contain at most %d entries", maxCommandEnvironmentEntries)
	}
	keys := make([]string, 0, len(extra))
	total := 0
	for key, value := range extra {
		if !portableEnvironmentName.MatchString(key) {
			return nil, fmt.Errorf("invalid command env name %q", key)
		}
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("command env %q contains a NUL byte", key)
		}
		total += len(key) + len(value) + 1
		if total > maxCommandEnvironmentBytes {
			return nil, fmt.Errorf("command env exceeds %d bytes", maxCommandEnvironmentBytes)
		}
		keys = append(keys, key)
	}
	environment := environmentMap(base)
	if len(environment) == 0 {
		environment = environmentMap(os.Environ())
	}
	if strings.TrimSpace(cwd) != "" {
		environment["PWD"] = cwd
		delete(environment, "OLDPWD")
	}
	for _, key := range keys {
		environment[key] = extra[key]
	}
	if pty && environment["TERM"] == "" {
		environment["TERM"] = "xterm-256color"
	}
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+environment[name])
	}
	return result, nil
}

func normalizeBaseEnvironment(source []string) []string {
	if len(source) == 0 {
		return nil
	}
	environment := environmentMap(source)
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+environment[name])
	}
	return result
}

func environmentMap(source []string) map[string]string {
	result := make(map[string]string, len(source))
	for _, entry := range source {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" || strings.IndexByte(name, 0) >= 0 || strings.IndexByte(value, 0) >= 0 {
			continue
		}
		result[name] = value
	}
	return result
}

func resolveShellExecutable(shell ShellKind, configured string, lookPath func(string) (string, error)) (string, string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		executable, err := validateExecutable(configured)
		if err != nil {
			return "", "", err
		}
		if !compatibleShellExecutable(shell, executable) {
			return "", "", fmt.Errorf("configured executable %q is not compatible with %s", executable, shell)
		}
		return executable, shellEngine(shell, executable), nil
	}
	if lookPath == nil {
		return "", "", fmt.Errorf("resolve %s executable: PATH lookup is unavailable", shell)
	}
	candidates := []string{"bash"}
	if shell == ShellPwsh {
		candidates = []string{"pwsh", "pwsh.exe", "powershell.exe"}
	} else if shell != ShellBash {
		return "", "", fmt.Errorf("unsupported shell %q", shell)
	}
	for _, candidate := range candidates {
		resolved, err := lookPath(candidate)
		if err != nil || strings.TrimSpace(resolved) == "" {
			continue
		}
		executable, err := validateExecutable(resolved)
		if err != nil {
			continue
		}
		return executable, shellEngine(shell, executable), nil
	}
	return "", "", fmt.Errorf("%s executable was not found", shell)
}

func compatibleShellExecutable(shell ShellKind, executable string) bool {
	base := strings.ToLower(filepath.Base(executable))
	switch shell {
	case ShellBash:
		return base == "bash" || base == "bash.exe"
	case ShellPwsh:
		return base == "pwsh" || base == "pwsh.exe" || base == "powershell" || base == "powershell.exe"
	default:
		return false
	}
}

func detectShellVersion(shell ShellKind, executable string) string {
	var command *exec.Cmd
	if shell == ShellPwsh {
		command = exec.Command(executable, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "$PSVersionTable.PSVersion.ToString()")
	} else {
		command = exec.Command(executable, "--version")
	}
	output, err := command.Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return boundedString(line, 256)
}

type boundedOutput struct {
	content   strings.Builder
	limit     int
	truncated bool
}

func (output *boundedOutput) Write(value string) {
	if value == "" || output.limit <= 0 {
		return
	}
	remaining := output.limit - output.content.Len()
	if remaining <= 0 {
		output.truncated = true
		return
	}
	if len(value) > remaining {
		value = truncateUTF8(value, remaining)
		output.truncated = true
	}
	output.content.WriteString(value)
}

func (output *boundedOutput) String() string { return output.content.String() }

type utf8OutputDecoder struct{ pending []byte }

func (decoder *utf8OutputDecoder) Feed(chunk []byte, final bool) string {
	data := make([]byte, 0, len(decoder.pending)+len(chunk))
	data = append(data, decoder.pending...)
	data = append(data, chunk...)
	decoder.pending = decoder.pending[:0]
	var result strings.Builder
	for len(data) > 0 {
		if !utf8.FullRune(data) && !final {
			decoder.pending = append(decoder.pending, data...)
			break
		}
		runeValue, size := utf8.DecodeRune(data)
		if runeValue == utf8.RuneError && size == 1 {
			result.WriteRune(utf8.RuneError)
			data = data[1:]
			continue
		}
		result.Write(data[:size])
		data = data[size:]
	}
	return result.String()
}
