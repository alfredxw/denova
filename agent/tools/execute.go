package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// CommandRunner is the execution port used by execute. Implementations own
// command admission, workspace coordination, cancellation, and process policy.
type CommandRunner interface {
	Run(context.Context, string) (*agent.StreamReader[string], error)
}

type executeInput struct {
	Command string `json:"command" jsonschema_description:"Foreground command to execute from the active workspace."`
}

type executeTool struct{ runner CommandRunner }

// Execute defines the standard foreground command tool. Progress is emitted
// through the Agent-owned progress collector rather than a second tool API.
func Execute(runner CommandRunner, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if _, err := agent.GoStruct2ToolInfo[executeInput]("execute", executeDescription); err != nil {
		return agent.ToolDefinition{}, fmt.Errorf("build execute schema: %w", err)
	}
	descriptor := applyDefinitionOptions(agent.ToolDescriptor{
		Source:            agent.ToolSourceShell,
		Execution:         agent.ToolExecutionWorkspaceExclusive,
		Recovery:          agent.ToolRecoveryNonIdempotent,
		ResultProjection:  agent.ToolResultBoundedModelContext,
		Steering:          agent.SteeringFinishCurrent,
		MutatesWorkspace:  true,
		MaxResultBytes:    defaultResultBytes,
		RequiresPostCheck: true,
	}, options)
	return agent.ToolDefinition{Tool: &executeTool{runner: runner}, Descriptor: descriptor}, nil
}

func (*executeTool) Info(context.Context) (*agent.ToolInfo, error) {
	return agent.GoStruct2ToolInfo[executeInput]("execute", executeDescription)
}

func (tool *executeTool) Run(ctx context.Context, arguments string, _ ...agent.ToolOption) (agent.ToolResult, error) {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	var input executeInput
	if err := decoder.Decode(&input); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode execute arguments: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return agent.ToolResult{}, errors.New("decode execute arguments: multiple JSON values are not allowed")
		}
		return agent.ToolResult{}, fmt.Errorf("decode execute arguments: invalid trailing JSON: %w", err)
	}
	if tool == nil || tool.runner == nil {
		return agent.ToolResult{}, errors.New("shell_execute capability is disabled")
	}
	if strings.TrimSpace(input.Command) == "" {
		return agent.ToolResult{}, errors.New("command is required")
	}
	stream, err := tool.runner.Run(ctx, input.Command)
	if err != nil {
		return agent.ToolResult{}, err
	}
	defer stream.Close()
	for {
		delta, recvErr := stream.Recv()
		if delta != "" {
			agent.EmitToolProgress(ctx, delta)
		}
		if errors.Is(recvErr, io.EOF) {
			return agent.ToolResult{Status: agent.ToolResultSuccess}, nil
		}
		if recvErr != nil {
			return agent.ToolResult{}, recvErr
		}
	}
}

// LocalCommandRunner runs one foreground shell command in a fixed canonical
// workspace. Its stdout and bounded stderr are returned through one stream.
type LocalCommandRunner struct {
	workspace string
}

// NewLocalCommandRunner binds command execution to a validated workspace.
func NewLocalCommandRunner(workspace *LocalWorkspace) (*LocalCommandRunner, error) {
	if workspace == nil || workspace.Root() == "" {
		return nil, errors.New("command workspace is required")
	}
	return &LocalCommandRunner{workspace: workspace.Root()}, nil
}

// Run executes a foreground command and terminates it when ctx is cancelled.
func (runner *LocalCommandRunner) Run(ctx context.Context, command string) (*agent.StreamReader[string], error) {
	if runner == nil || runner.workspace == "" {
		return nil, errors.New("command runner is not configured")
	}
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("command is required")
	}
	reader, writer := agent.Pipe[string](32)
	go runLocalCommand(ctx, runner.workspace, command, writer)
	return reader, nil
}

func runLocalCommand(ctx context.Context, workspace, command string, writer *agent.StreamWriter[string]) {
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = writer.Send("", fmt.Errorf("execute panic: %v\n%s", recovered, debug.Stack()))
		}
		writer.Close()
	}()
	name, args := "/bin/sh", []string{"-c", command}
	if runtime.GOOS == "windows" {
		name, args = "powershell.exe", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}
	}
	process := exec.CommandContext(ctx, name, args...)
	process.Dir = workspace
	stdout, err := process.StdoutPipe()
	if err != nil {
		_ = writer.Send("", fmt.Errorf("create command stdout: %w", err))
		return
	}
	stderr, err := process.StderrPipe()
	if err != nil {
		_ = writer.Send("", fmt.Errorf("create command stderr: %w", err))
		return
	}
	if err := process.Start(); err != nil {
		_ = writer.Send("", fmt.Errorf("start command: %w", err))
		return
	}
	diagnostic := make(chan []byte, 1)
	go func() {
		defer func() {
			if recover() != nil {
				diagnostic <- nil
			}
		}()
		data, _ := readBoundedAndDrain(stderr, defaultResultBytes)
		diagnostic <- data
	}()
	buffer := make([]byte, 32*1024)
	hasOutput := false
	for {
		count, readErr := stdout.Read(buffer)
		if count > 0 {
			hasOutput = true
			if writer.Send(string(buffer[:count]), nil) {
				if process.Process != nil {
					_ = process.Process.Kill()
				}
				return
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				_ = writer.Send("", fmt.Errorf("read command stdout: %w", readErr))
			}
			break
		}
	}
	stderrOutput := <-diagnostic
	waitErr := process.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		_ = writer.Send("", ctxErr)
		return
	}
	if waitErr != nil {
		message := fmt.Sprintf("[Command failed: %v]", waitErr)
		if len(stderrOutput) > 0 {
			message += "\n[stderr]:\n" + boundedString(string(stderrOutput), defaultResultBytes)
		}
		_ = writer.Send("\n"+message, nil)
		return
	}
	if !hasOutput {
		_ = writer.Send("[Command executed successfully with no output]", nil)
	}
}

func readBoundedAndDrain(reader io.Reader, limit int64) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: limit + 1}
	data, readErr := io.ReadAll(limited)
	_, drainErr := io.Copy(io.Discard, reader)
	if readErr != nil {
		return data, readErr
	}
	if drainErr != nil {
		return data, drainErr
	}
	if int64(len(data)) > limit {
		data = data[:limit]
	}
	return data, nil
}

const executeDescription = `Execute one foreground command from the active workspace and stream stdout while it runs. Background execution is unsupported; cancellation terminates the process.

在当前工作区以前台方式执行一条命令并流式返回 stdout。不支持后台执行；取消调用会终止进程。`
