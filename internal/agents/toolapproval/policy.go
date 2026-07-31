// Package toolapproval classifies product tool calls before any durable start
// or host side effect. It is deliberately pure: callers own persistence and UI.
package toolapproval

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

type Action string

const (
	ActionAllow  Action = "allow"
	ActionPrompt Action = "prompt"
	ActionDeny   Action = "deny"
)

type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

// Request contains only stable host facts. Mode and workspace must be
// snapshotted by the run owner; a tool call cannot select either value.
type Request struct {
	Mode       config.AgentApprovalMode
	Workspace  string
	ToolName   string
	Arguments  string
	Descriptor agent.ToolDescriptor
	GOOS       string
}

// Decision is suitable for durable audit records and user approval cards.
// RuleID is stable; Reason is bilingual presentation text.
type Decision struct {
	Action  Action `json:"action"`
	Risk    Risk   `json:"risk"`
	RuleID  string `json:"rule_id"`
	Reason  string `json:"reason"`
	Command string `json:"command,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
}

type commandArguments struct {
	Command string            `json:"command"`
	Cwd     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func Evaluate(request Request) Decision {
	request.Mode = config.NormalizeAgentApprovalMode(request.Mode)
	request.ToolName = strings.ToLower(strings.TrimSpace(request.ToolName))
	request.Workspace = strings.TrimSpace(request.Workspace)
	if request.GOOS == "" {
		request.GOOS = runtime.GOOS
	}

	if request.Descriptor.Source == agent.ToolSourceShell || request.ToolName == "bash" || request.ToolName == "pwsh" {
		return evaluateShell(request)
	}
	if request.ToolName == "browser" {
		return evaluateBrowser(request)
	}

	// Product-owned structured tools have bounded schemas and their existing
	// capability checks remain authoritative. Workspace/session/config changes
	// therefore do not inherit the ambiguity of an arbitrary shell program.
	switch request.Descriptor.MutationScope {
	case agent.ToolMutationNone, agent.ToolMutationWorkspace, agent.ToolMutationSession, agent.ToolMutationConfig:
		if isNetworkSource(request.Descriptor.Source) && request.Mode == config.AgentApprovalAsk {
			return prompt("network_access", RiskMedium,
				"该工具将访问网络；Ask 模式需要你的确认。 / This tool will access the network; Ask mode requires approval.")
		}
		return allow("structured_tool", RiskLow,
			"结构化工具调用符合已启用的能力范围。 / The structured tool call is within its enabled capability.")
	case agent.ToolMutationExternal:
		if request.Mode == config.AgentApprovalFullAccess {
			return allow("external_full_access", RiskHigh,
				"Full access 模式允许非 Shell 的外部副作用。 / Full access mode allows this non-shell external side effect.")
		}
		if request.Mode == config.AgentApprovalWrite && isNetworkSource(request.Descriptor.Source) {
			return allow("network_write_mode", RiskMedium,
				"Write 模式允许网络读取与下载。 / Write mode allows network reads and downloads.")
		}
		return prompt("external_mutation", RiskHigh,
			"该工具可能修改工作区之外的状态，需要你的确认。 / This tool may change state outside the workspace and requires approval.")
	default:
		return prompt("unknown_scope", RiskHigh,
			"工具没有可识别的副作用范围，需要你的确认。 / The tool has no recognized side-effect scope and requires approval.")
	}
}

func evaluateShell(request Request) Decision {
	var input commandArguments
	if err := json.Unmarshal([]byte(request.Arguments), &input); err != nil {
		return deny("invalid_shell_arguments", RiskHigh,
			"Shell 参数不是有效的结构化输入，已拒绝执行。 / Shell arguments are not valid structured input and were denied.")
	}
	input.Command = strings.TrimSpace(input.Command)
	input.Cwd = strings.TrimSpace(input.Cwd)
	if input.Command == "" {
		return deny("empty_shell_command", RiskHigh,
			"Shell 命令为空，已拒绝执行。 / The shell command is empty and was denied.")
	}
	if name := dangerousShellEnvironment(input.Env); name != "" {
		result := deny("critical_shell_environment_override", RiskCritical,
			fmt.Sprintf("禁止覆盖可能改变命令解析或注入代码的环境变量 %s。 / Overriding environment variable %s could change command resolution or inject code and is blocked.", name, name))
		result.Command, result.Cwd = input.Command, input.Cwd
		return result
	}
	if critical := matchCriticalCommand(input.Command, request.ToolName, request.GOOS); critical != nil {
		critical.Command = input.Command
		critical.Cwd = input.Cwd
		return *critical
	}
	if request.Mode == config.AgentApprovalFullAccess {
		result := allow("full_access_non_critical", RiskHigh,
			"Full access 模式允许此命令；它未命中极高危拦截规则。 / Full access mode allows this command because it did not match a critical block rule.")
		result.Command, result.Cwd = input.Command, input.Cwd
		return result
	}
	if len(input.Env) != 0 {
		result := prompt("shell_environment_override", RiskHigh,
			"命令将覆盖进程环境变量，需要你的确认。 / The command overrides process environment variables and requires approval.")
		result.Command, result.Cwd = input.Command, input.Cwd
		return result
	}

	workspace, err := filepath.Abs(request.Workspace)
	if err != nil || workspace == "" {
		result := prompt("workspace_unavailable", RiskHigh,
			"无法确认命令的工作区边界，需要你的确认。 / The workspace boundary cannot be verified, so approval is required.")
		result.Command, result.Cwd = input.Command, input.Cwd
		return result
	}
	classification := classifyBash(input.Command, workspace, input.Cwd, request.Mode)
	if request.ToolName == "pwsh" || request.GOOS == "windows" {
		classification = classifyPowerShell(input.Command, workspace, input.Cwd, request.Mode)
	}
	classification.Command, classification.Cwd = input.Command, input.Cwd
	return classification
}

func dangerousShellEnvironment(environment map[string]string) string {
	for name := range environment {
		normalized := strings.ToUpper(strings.TrimSpace(name))
		switch normalized {
		case "BASH_ENV", "ENV", "BASHOPTS", "SHELLOPTS", "CDPATH", "PATH",
			"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "DYLD_LIBRARY_PATH",
			"PROMPT_COMMAND", "PS4", "GIT_EXTERNAL_DIFF", "GIT_SSH", "GIT_SSH_COMMAND",
			"GIT_ASKPASS", "SSH_ASKPASS", "PAGER", "GIT_PAGER", "LESSOPEN", "PERL5OPT",
			"PYTHONPATH", "PYTHONSTARTUP", "RUBYOPT", "NODE_OPTIONS":
			return normalized
		}
		if strings.HasPrefix(normalized, "BASH_FUNC_") {
			return normalized
		}
	}
	return ""
}

func isNetworkSource(source agent.ToolSource) bool {
	return source == agent.ToolSourceWeb || source == agent.ToolSourceImage
}

func allow(rule string, risk Risk, reason string) Decision {
	return Decision{Action: ActionAllow, Risk: risk, RuleID: rule, Reason: reason}
}

func prompt(rule string, risk Risk, reason string) Decision {
	return Decision{Action: ActionPrompt, Risk: risk, RuleID: rule, Reason: reason}
}

func deny(rule string, risk Risk, reason string) Decision {
	return Decision{Action: ActionDeny, Risk: risk, RuleID: rule, Reason: reason}
}

func (decision Decision) Validate() error {
	switch decision.Action {
	case ActionAllow, ActionPrompt, ActionDeny:
	default:
		return fmt.Errorf("unknown approval action %q", decision.Action)
	}
	if strings.TrimSpace(decision.RuleID) == "" || strings.TrimSpace(decision.Reason) == "" {
		return fmt.Errorf("approval decision requires a rule id and reason")
	}
	return nil
}
