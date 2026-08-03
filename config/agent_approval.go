package config

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"denova/internal/hostruntime"
)

// AgentApprovalMode is the user-owned safety posture snapshotted when an Agent
// is built. Workspace configuration and SubAgents cannot raise this ceiling.
type AgentApprovalMode string

const (
	AgentApprovalAsk        AgentApprovalMode = "ask"
	AgentApprovalWrite      AgentApprovalMode = "write"
	AgentApprovalFullAccess AgentApprovalMode = "full_access"
)

const (
	// AgentApprovalRuleWorkspace is deliberately the only persisted scope. A
	// project may never grant permissions to another workspace or to the host
	// globally through a checked-in configuration file.
	AgentApprovalRuleWorkspace      = "workspace"
	AgentApprovalRuleMatcherVersion = 1
	MaxAgentApprovalCommandBytes    = 16 * 1024
	MaxAgentApprovalRuleKeyBytes    = 2 * 1024
)

// AgentApprovalRule is a user-owned, workspace-scoped shell authorization.
// CommandKey is emitted and revalidated by the versioned policy matcher; it is
// never a user-authored glob or prefix. ApprovedArgsHash preserves the exact
// approved request for audit without making volatile arguments the match key.
type AgentApprovalRule struct {
	ID               string    `toml:"id" json:"id"`
	Scope            string    `toml:"scope" json:"scope"`
	ProjectID        string    `toml:"project_id,omitempty" json:"project_id,omitempty"`
	Workspace        string    `toml:"workspace,omitempty" json:"workspace,omitempty"`
	ToolName         string    `toml:"tool_name" json:"tool_name"`
	MatcherVersion   int       `toml:"matcher_version" json:"matcher_version"`
	CommandKey       string    `toml:"command_key" json:"command_key"`
	CommandPattern   string    `toml:"command_pattern" json:"command_pattern"`
	ApprovedArgsHash string    `toml:"approved_args_hash" json:"approved_args_hash"`
	ApprovedCommand  string    `toml:"approved_command" json:"approved_command"`
	ApprovedCwd      string    `toml:"approved_cwd,omitempty" json:"approved_cwd,omitempty"`
	SourceRuleID     string    `toml:"source_rule_id,omitempty" json:"source_rule_id,omitempty"`
	CreatedAt        time.Time `toml:"created_at" json:"created_at"`
}

// NormalizeAgentApprovalRules returns a detached canonical projection suitable
// for both persistence and immutable run configuration snapshots.
func NormalizeAgentApprovalRules(rules []AgentApprovalRule) []AgentApprovalRule {
	if rules == nil {
		return nil
	}
	result := make([]AgentApprovalRule, len(rules))
	for index, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Scope = strings.ToLower(strings.TrimSpace(rule.Scope))
		rule.ProjectID = strings.TrimSpace(rule.ProjectID)
		rule.Workspace = strings.TrimSpace(rule.Workspace)
		rule.ToolName = strings.ToLower(strings.TrimSpace(rule.ToolName))
		rule.CommandKey = strings.TrimSpace(rule.CommandKey)
		rule.CommandPattern = strings.TrimSpace(rule.CommandPattern)
		rule.ApprovedArgsHash = strings.ToLower(strings.TrimSpace(rule.ApprovedArgsHash))
		rule.ApprovedCommand = strings.TrimSpace(rule.ApprovedCommand)
		rule.ApprovedCwd = strings.TrimSpace(rule.ApprovedCwd)
		rule.SourceRuleID = strings.TrimSpace(rule.SourceRuleID)
		result[index] = rule
	}
	return result
}

// ValidateAgentApprovalRules rejects ambiguous rules before they enter the
// user settings file. There is intentionally no wildcard or prefix syntax.
func ValidateAgentApprovalRules(rules []AgentApprovalRule) error {
	seen := make(map[string]struct{}, len(rules))
	for index, rule := range NormalizeAgentApprovalRules(rules) {
		path := fmt.Sprintf("agent_approval_rules[%d]", index)
		if rule.ID == "" || len(rule.ID) > 96 {
			return fmt.Errorf("%s.id must contain 1-96 characters", path)
		}
		if _, duplicate := seen[rule.ID]; duplicate {
			return fmt.Errorf("%s.id %q is duplicated", path, rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if rule.Scope != AgentApprovalRuleWorkspace {
			return fmt.Errorf("%s.scope must be %q", path, AgentApprovalRuleWorkspace)
		}
		if rule.Workspace == "" {
			return fmt.Errorf("%s.workspace is required", path)
		}
		if rule.ToolName != "bash" && rule.ToolName != "pwsh" {
			return fmt.Errorf("%s.tool_name must be bash or pwsh", path)
		}
		if rule.MatcherVersion != AgentApprovalRuleMatcherVersion {
			return fmt.Errorf("%s.matcher_version must be %d", path, AgentApprovalRuleMatcherVersion)
		}
		if rule.CommandKey == "" || len(rule.CommandKey) > MaxAgentApprovalRuleKeyBytes {
			return fmt.Errorf("%s.command_key must contain 1-%d bytes", path, MaxAgentApprovalRuleKeyBytes)
		}
		if rule.CommandPattern == "" || len(rule.CommandPattern) > MaxAgentApprovalCommandBytes {
			return fmt.Errorf("%s.command_pattern must contain 1-%d bytes", path, MaxAgentApprovalCommandBytes)
		}
		decoded, err := hex.DecodeString(rule.ApprovedArgsHash)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("%s.approved_args_hash must be a SHA-256 digest", path)
		}
		if rule.ApprovedCommand == "" || len(rule.ApprovedCommand) > MaxAgentApprovalCommandBytes {
			return fmt.Errorf("%s.approved_command must contain 1-%d bytes", path, MaxAgentApprovalCommandBytes)
		}
		if rule.CreatedAt.IsZero() {
			return fmt.Errorf("%s.created_at is required", path)
		}
	}
	return nil
}

// NormalizeAgentApprovalMode fails closed to Ask for empty or invalid values.
// The empty value remains meaningful in the user settings layer: it means the
// user follows the product default instead of persisting an explicit choice.
func NormalizeAgentApprovalMode(value AgentApprovalMode) AgentApprovalMode {
	switch AgentApprovalMode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case AgentApprovalAsk:
		return AgentApprovalAsk
	case AgentApprovalWrite:
		return AgentApprovalWrite
	case AgentApprovalFullAccess:
		return AgentApprovalFullAccess
	default:
		return AgentApprovalAsk
	}
}

func ParseAgentApprovalMode(value string) (AgentApprovalMode, error) {
	mode := AgentApprovalMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case AgentApprovalAsk, AgentApprovalWrite, AgentApprovalFullAccess:
		return mode, nil
	default:
		return "", fmt.Errorf("agent approval mode must be %q, %q, or %q", AgentApprovalAsk, AgentApprovalWrite, AgentApprovalFullAccess)
	}
}

// ShellEnvironmentMode controls how Agent Bash inherits a desktop user's
// exported environment. It never changes manual terminal sessions.
type ShellEnvironmentMode = hostruntime.EnvironmentMode

const (
	ShellEnvironmentAuto    = hostruntime.EnvironmentAuto
	ShellEnvironmentProcess = hostruntime.EnvironmentProcess
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
