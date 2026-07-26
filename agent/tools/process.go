package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// ShellKind is the command language presented to the model.
type ShellKind string

const (
	ShellBash ShellKind = "bash"
	ShellPwsh ShellKind = "pwsh"
)

// ProcessStatus distinguishes completed command failure from tool protocol or
// process-start errors. Non-zero exit, cancellation, and timeout are expected
// structured outcomes rather than opaque tool errors.
type ProcessStatus string

const (
	ProcessStatusSuccess   ProcessStatus = "success"
	ProcessStatusFailed    ProcessStatus = "failed"
	ProcessStatusCancelled ProcessStatus = "cancelled"
	ProcessStatusTimedOut  ProcessStatus = "timed_out"
)

// CommandRequest is the provider-neutral request accepted by a Process
// Adapter. TimeoutSeconds zero means no command deadline.
type CommandRequest struct {
	Command        string
	Cwd            string
	Env            map[string]string
	PTY            bool
	TimeoutSeconds int
}

// CommandResult is the final structured outcome of one foreground process.
type CommandResult struct {
	Status          ProcessStatus
	Shell           ShellKind
	Engine          string
	Version         string
	ExitCode        int
	Output          string
	OutputTruncated bool
	Cwd             string
	PTY             bool
	TimeoutSeconds  int
}

// CommandRunner executes one foreground command and reports its merged
// stdout/stderr stream in arrival order. The bounded final result must retain
// the same output needed by the next model step.
type CommandRunner interface {
	Run(context.Context, CommandRequest, func(string)) (CommandResult, error)
}

// CommandRunGuard coordinates a process with other workspace mutation paths.
// Denova injects workspacechange.WithExclusiveWorkspace here.
type CommandRunGuard func(context.Context, func() error) error

// CommandRunnerOptions configure the reusable local Process implementation.
type CommandRunnerOptions struct {
	Workspace  *LocalWorkspace
	Shell      ShellKind
	Executable string
	Guard      CommandRunGuard
}

type commandInput struct {
	Command        string            `json:"command" jsonschema:"maxLength=1048576" jsonschema_description:"Foreground command to execute using this tool's exact shell language, up to the command safety limit."`
	Cwd            string            `json:"cwd,omitempty" jsonschema:"maxLength=4096" jsonschema_description:"Workspace-relative working directory; defaults to the workspace root."`
	Env            map[string]string `json:"env,omitempty" jsonschema_description:"Additional non-secret environment variables. Keys must be portable environment names."`
	PTY            bool              `json:"pty,omitempty" jsonschema_description:"Allocate a pseudo-terminal for commands that require terminal behavior."`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty" jsonschema:"minimum=0" jsonschema_description:"Command deadline in seconds; zero or omitted means unlimited."`
}

// Bash defines the Unix Bash tool. Callers register it only on platforms where
// Bash is the intended command language.
func Bash(runner CommandRunner, options ...DefinitionOption) (agent.ToolDefinition, error) {
	return commandTool("bash", ShellBash, runner, options...)
}

// Pwsh defines the Windows PowerShell tool. The result reports whether the
// actual engine is modern pwsh or the controlled Windows PowerShell fallback.
func Pwsh(runner CommandRunner, options ...DefinitionOption) (agent.ToolDefinition, error) {
	return commandTool("pwsh", ShellPwsh, runner, options...)
}

func commandTool(name string, shell ShellKind, runner CommandRunner, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if runner == nil {
		return agent.ToolDefinition{}, fmt.Errorf("%s CommandRunner is nil", name)
	}
	description := "Execute one foreground " + string(shell) + " command from a workspace directory. stdout and stderr are streamed in arrival order. Optional env, timeout, and PTY are explicit; background jobs are unsupported.\n\n" +
		"在工作区目录中以前台方式执行一条 " + string(shell) + " 命令，stdout 与 stderr 按到达顺序流式展示。可显式设置 env、timeout 和 PTY；不支持后台任务。"
	descriptor := shellDescriptor(options...)
	tool, err := agent.InferTool(name, description, func(ctx context.Context, input commandInput) (agent.ToolResult, error) {
		if strings.TrimSpace(input.Command) == "" {
			return agent.ToolResult{}, errors.New("command is required")
		}
		if len(input.Command) > maxCommandBytes {
			return agent.ToolResult{}, fmt.Errorf("command exceeds the %d-byte limit", maxCommandBytes)
		}
		result, err := runner.Run(ctx, CommandRequest{
			Command: input.Command, Cwd: input.Cwd, Env: input.Env,
			PTY: input.PTY, TimeoutSeconds: input.TimeoutSeconds,
		}, func(delta string) {
			agent.EmitToolProgress(ctx, delta)
		})
		if err != nil {
			return agent.ToolResult{}, err
		}
		if result.Shell == "" {
			result.Shell = shell
		} else if result.Shell != shell {
			return agent.ToolResult{}, fmt.Errorf("%s runner reported incompatible shell %q", name, result.Shell)
		}
		return commandToolResult(result, descriptor.MaxResultBytes)
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: descriptor}, err
}

type processRecovery struct {
	Retryable  bool   `json:"retryable"`
	Suggestion string `json:"suggestion,omitempty"`
}

type processEnvelope struct {
	Schema          string          `json:"schema"`
	Status          ProcessStatus   `json:"status"`
	Shell           ShellKind       `json:"shell"`
	Engine          string          `json:"engine"`
	Version         string          `json:"version,omitempty"`
	ExitCode        int             `json:"exit_code"`
	Cwd             string          `json:"cwd"`
	PTY             bool            `json:"pty"`
	TimeoutSeconds  int             `json:"timeout_seconds"`
	OutputTruncated bool            `json:"output_truncated"`
	Recovery        processRecovery `json:"recovery"`
}

func commandToolResult(result CommandResult, maxResultBytes int) (agent.ToolResult, error) {
	if result.Status == "" {
		if result.ExitCode == 0 {
			result.Status = ProcessStatusSuccess
		} else {
			result.Status = ProcessStatusFailed
		}
	}
	recovery := processRecovery{}
	switch result.Status {
	case ProcessStatusSuccess:
	case ProcessStatusFailed:
		recovery = processRecovery{Retryable: true, Suggestion: "Inspect the merged output and correct the command. / 检查合并输出并修正命令。"}
	case ProcessStatusTimedOut:
		recovery = processRecovery{Retryable: true, Suggestion: "Narrow the command or explicitly increase timeout_seconds. / 缩小命令范围或显式增加 timeout_seconds。"}
	case ProcessStatusCancelled:
		recovery = processRecovery{Retryable: false}
	default:
		return agent.ToolResult{}, fmt.Errorf("process returned unsupported status %q", result.Status)
	}
	metadata := processEnvelope{
		Schema: "process.result.v1", Status: result.Status, Shell: result.Shell,
		Engine: result.Engine, Version: result.Version, ExitCode: result.ExitCode,
		Cwd: result.Cwd, PTY: result.PTY, TimeoutSeconds: result.TimeoutSeconds,
		OutputTruncated: result.OutputTruncated, Recovery: recovery,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("serialize process result: %w", err)
	}
	if len(encoded) > maxResultBytes {
		return agent.ToolResult{}, fmt.Errorf("process result metadata exceeds the %d-byte result limit", maxResultBytes)
	}
	output := result.Output
	if output == "" && result.Status == ProcessStatusSuccess {
		output = "[Command executed successfully with no output]"
	}
	available := maxResultBytes - len(encoded)
	if output != "" && available > 0 {
		available-- // newline separating the mandatory envelope from output
	}
	projected, truncated := truncateUTF8WithMarker(output, "\n"+processTruncatedMarker, max(0, available))
	if truncated && !metadata.OutputTruncated {
		metadata.OutputTruncated = true
		encoded, err = json.Marshal(metadata)
		if err != nil {
			return agent.ToolResult{}, fmt.Errorf("serialize truncated process result: %w", err)
		}
		if len(encoded) > maxResultBytes {
			return agent.ToolResult{}, fmt.Errorf("process result metadata exceeds the %d-byte result limit", maxResultBytes)
		}
		available = maxResultBytes - len(encoded)
		if output != "" && available > 0 {
			available--
		}
		projected, _ = truncateUTF8WithMarker(output, "\n"+processTruncatedMarker, max(0, available))
	}
	content := string(encoded)
	if projected != "" {
		content += "\n" + projected
	}
	return agent.ToolResult{
		ModelContent: content, DisplayContent: content, Details: encoded,
		Status: agent.ToolResultSuccess,
	}, nil
}

func shellEngine(shell ShellKind, executable string) string {
	base := strings.ToLower(filepath.Base(executable))
	if shell == ShellPwsh && (base == "powershell" || base == "powershell.exe") {
		return "windows_powershell"
	}
	return string(shell)
}

func shellCommand(shell ShellKind, executable, command string) (string, []string) {
	switch shell {
	case ShellPwsh:
		args := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}
		if shellEngine(shell, executable) == "windows_powershell" {
			args = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command}
		}
		return executable, args
	default:
		return executable, []string{"-c", command}
	}
}
