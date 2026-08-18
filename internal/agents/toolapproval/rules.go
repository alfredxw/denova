package toolapproval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"denova/config"
)

// ArgumentsHash returns a stable digest for one complete tool input. JSON
// object key order and insignificant whitespace do not create distinct rules;
// malformed input still receives an exact byte-level digest and remains
// subject to the normal fail-closed policy.
func ArgumentsHash(arguments string) string {
	canonical := []byte(strings.TrimSpace(arguments))
	decoder := json.NewDecoder(bytes.NewBufferString(arguments))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err == nil {
		var trailing any
		if err := decoder.Decode(&trailing); err == io.EOF {
			if encoded, marshalErr := json.Marshal(value); marshalErr == nil {
				canonical = encoded
			}
		}
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

// NewWorkspaceRule creates the only rule shape the approval UI may persist.
// The deterministic ID makes retries idempotent across settings writes and
// session recovery.
func NewWorkspaceRule(
	projectID, workspace string,
	proposal RuleProposal,
	approvedArgsHash, approvedInput, approvedContext, sourceRuleID string,
	createdAt time.Time,
) (config.AgentApprovalRule, error) {
	projectID = strings.TrimSpace(projectID)
	workspace = canonicalWorkspace(workspace)
	toolName := strings.ToLower(strings.TrimSpace(proposal.ToolName))
	approvedArgsHash = strings.ToLower(strings.TrimSpace(approvedArgsHash))
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%d\x00%s",
		projectID, workspace, toolName, proposal.Matcher, proposal.MatcherVersion, proposal.MatchKey,
	)))
	rule := config.AgentApprovalRule{
		ID: "approval-" + hex.EncodeToString(digest[:16]), Scope: config.AgentApprovalRuleWorkspace,
		ProjectID: projectID, Workspace: workspace, ToolName: toolName,
		Matcher: proposal.Matcher, MatcherVersion: proposal.MatcherVersion, MatchKey: proposal.MatchKey,
		DisplayPattern: proposal.DisplayPattern, ApprovedArgsHash: approvedArgsHash,
		ApprovedInput: strings.TrimSpace(approvedInput), ApprovedContext: strings.TrimSpace(approvedContext),
		SourceRuleID: strings.TrimSpace(sourceRuleID), CreatedAt: createdAt.UTC(),
	}
	if err := config.ValidateAgentApprovalRules([]config.AgentApprovalRule{rule}); err != nil {
		return config.AgentApprovalRule{}, fmt.Errorf("build workspace approval rule: %w", err)
	}
	return rule, nil
}

func matchingWorkspaceRule(request Request, proposal RuleProposal) *config.AgentApprovalRule {
	if len(request.Rules) == 0 {
		return nil
	}
	toolName := strings.ToLower(strings.TrimSpace(proposal.ToolName))
	workspace := canonicalWorkspace(request.Workspace)
	projectID := strings.TrimSpace(request.ProjectID)
	for index := range request.Rules {
		rule := request.Rules[index]
		if rule.Scope != config.AgentApprovalRuleWorkspace || rule.ToolName != toolName ||
			rule.Matcher != proposal.Matcher || rule.MatcherVersion != proposal.MatcherVersion || rule.MatchKey != proposal.MatchKey {
			continue
		}
		if rule.ProjectID != "" && rule.ProjectID != projectID {
			continue
		}
		if ruleWorkspace := canonicalWorkspace(rule.Workspace); ruleWorkspace != "" && ruleWorkspace != workspace {
			continue
		}
		return &request.Rules[index]
	}
	return nil
}

func canonicalWorkspace(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return filepath.Clean(workspace)
	}
	return filepath.Clean(abs)
}
