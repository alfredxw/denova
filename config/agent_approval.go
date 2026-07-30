package config

import (
	"fmt"
	"strings"
)

// AgentApprovalMode is the user-owned safety posture snapshotted when an Agent
// is built. Workspace configuration and SubAgents cannot raise this ceiling.
type AgentApprovalMode string

const (
	AgentApprovalAsk   AgentApprovalMode = "ask"
	AgentApprovalWrite AgentApprovalMode = "write"
	AgentApprovalYolo  AgentApprovalMode = "yolo"
)

// NormalizeAgentApprovalMode fails closed to Ask for empty or invalid values.
// The empty value remains meaningful in the user settings layer: it tells the
// frontend that the mandatory first-run choice has not been persisted yet.
func NormalizeAgentApprovalMode(value AgentApprovalMode) AgentApprovalMode {
	switch AgentApprovalMode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AgentApprovalAsk:
		return AgentApprovalAsk
	case AgentApprovalWrite:
		return AgentApprovalWrite
	case AgentApprovalYolo:
		return AgentApprovalYolo
	default:
		return AgentApprovalAsk
	}
}

func ParseAgentApprovalMode(value string) (AgentApprovalMode, error) {
	mode := AgentApprovalMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case AgentApprovalAsk, AgentApprovalWrite, AgentApprovalYolo:
		return mode, nil
	default:
		return "", fmt.Errorf("agent approval mode must be %q, %q, or %q", AgentApprovalAsk, AgentApprovalWrite, AgentApprovalYolo)
	}
}

// ShellEnvironmentMode controls how Agent Bash inherits a desktop user's
// exported environment. It never changes manual terminal sessions.
type ShellEnvironmentMode string

const (
	ShellEnvironmentAuto    ShellEnvironmentMode = "auto"
	ShellEnvironmentProcess ShellEnvironmentMode = "process"
)

func normalizeShellEnvironmentMode(value ShellEnvironmentMode) ShellEnvironmentMode {
	switch ShellEnvironmentMode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case ShellEnvironmentAuto:
		return ShellEnvironmentAuto
	case ShellEnvironmentProcess:
		return ShellEnvironmentProcess
	default:
		return ShellEnvironmentAuto
	}
}
