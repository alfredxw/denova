package config

import (
	"strings"
	"testing"
	"time"
)

func TestAgentApprovalModeUsesWriteAsProductDefault(t *testing.T) {
	if mode := DefaultSettings().AgentApprovalMode; mode != AgentApprovalWrite {
		t.Fatalf("default Agent approval mode = %q, want %q", mode, AgentApprovalWrite)
	}
}

func TestAgentApprovalModeRejectsUnknownValues(t *testing.T) {
	if mode := NormalizeAgentApprovalMode("unknown"); mode != AgentApprovalAsk {
		t.Fatalf("normalized unknown mode = %q, want fail-closed %q", mode, AgentApprovalAsk)
	}
	if _, err := ParseAgentApprovalMode("unknown"); err == nil {
		t.Fatal("unknown mode was accepted")
	}
}

func TestAgentApprovalRuleValidationAndWorkspaceBoundary(t *testing.T) {
	t.Parallel()
	rule := AgentApprovalRule{
		ID: "approval-test", Scope: AgentApprovalRuleWorkspace,
		ProjectID: "project-test", Workspace: "/workspace", ToolName: "bash",
		Matcher:        AgentApprovalMatcherShell,
		MatcherVersion: AgentApprovalRuleMatcherVersion,
		MatchKey:       `["go","test"]`, DisplayPattern: "go test ...",
		ApprovedArgsHash: strings.Repeat("a", 64), ApprovedInput: "go test ./...",
		CreatedAt: time.Now(),
	}
	if err := ValidateAgentApprovalRules([]AgentApprovalRule{rule}); err != nil {
		t.Fatal(err)
	}
	invalid := rule
	invalid.MatcherVersion++
	if err := ValidateAgentApprovalRules([]AgentApprovalRule{invalid}); err == nil {
		t.Fatal("unknown matcher version was accepted")
	}
	projectOnly := rule
	projectOnly.Workspace = ""
	if err := ValidateAgentApprovalRules([]AgentApprovalRule{projectOnly}); err == nil {
		t.Fatal("project-only approval rule was accepted without a workspace boundary")
	}
	workspace := PrepareWorkspaceAgentSettingsForWrite(Settings{}, Settings{AgentApprovalRules: []AgentApprovalRule{rule}})
	if workspace.AgentApprovalRules != nil {
		t.Fatalf("workspace settings retained user-owned approval rules: %#v", workspace.AgentApprovalRules)
	}
}
