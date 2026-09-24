package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"denova/config"
	"denova/internal/agents/toolapproval"
	agent "github.com/alfredxw/denova/agent"
)

// PermissionPolicy applies engine permissions at the common tool boundary.
// Saved native approval rules stay dormant while an external runtime is active.
type PermissionPolicy struct {
	Selection config.RuntimeSelection
	ProjectID string
	Workspace string
}

func (policy PermissionPolicy) Identity() agent.CapabilityIdentity {
	data, _ := json.Marshal(struct {
		Selection config.RuntimeSelection
		Project   string
	}{policy.Selection, policy.ProjectID})
	digest := sha256.Sum256(data)
	return agent.CapabilityIdentity{Kind: "denova.external_permissions", Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

func (policy PermissionPolicy) Evaluate(_ context.Context, request agent.PermissionRequest) (agent.PermissionDecision, error) {
	reason := HostPermissionError(policy.Selection, policy.ProjectID, policy.Workspace, ToolCall{ID: request.CallID, Name: request.Tool, Arguments: request.Arguments}, request.Descriptor, request.Attachments)
	if reason == "" {
		return agent.PermissionDecision{Kind: agent.PermissionAllow}, nil
	}
	return agent.PermissionDecision{Kind: agent.PermissionBlock, Reason: agent.LocalizedText{English: reason, Chinese: "当前运行时权限不允许执行此工具。请调整会话权限后重试。"}}, nil
}

func (PermissionPolicy) Resolve(context.Context, agent.PermissionResolveRequest) (agent.PermissionResolvedDecision, error) {
	return agent.PermissionResolvedDecision{}, fmt.Errorf("external execution permissions do not request interactive approval")
}

// hostPermissionError enforces the accepted engine selection where tools
// actually execute. Engine process sandboxes do not contain host callbacks.
// These modes have no interactive escalation: denied calls return a tool error.
func (operation *Operation) hostPermissionError(call ToolCall, tool preparedTool) string {
	operation.mu.Lock()
	files := append([]agent.Attachment(nil), operation.request.Input.Attachments...)
	operation.mu.Unlock()
	return HostPermissionError(operation.request.Input.Selection, operation.request.ProjectID, operation.request.ToolPolicy.Workspace, call, tool.definition.Descriptor, files)
}

// HostPermissionError enforces the selected engine policy where a product
// executes callbacks. It never reads Native approval defaults or stored rules.
func HostPermissionError(selection config.RuntimeSelection, projectID, workspace string, call ToolCall, descriptor agent.ToolDescriptor, files []agent.Attachment) string {
	if selection.Kind != config.RuntimeCodex || selection.Codex == nil {
		return ""
	}
	// Planning state belongs to the conversation, not the workspace sandbox.
	if descriptor.Capability == "todo" && descriptor.MutationScope == agent.ToolMutationSession {
		return ""
	}
	sandbox := selection.Codex.EffectiveSandbox()
	mode := config.AgentApprovalWrite
	if sandbox == config.CodexFullAccess {
		mode = config.AgentApprovalFullAccess
	}
	if sandbox == config.CodexReadOnly {
		mode = config.AgentApprovalAsk
	}
	attachments := make([]string, 0, len(files))
	for _, attachment := range files {
		path := attachment.RuntimePath
		if path == "" {
			path = attachment.Path
		}
		attachments = append(attachments, path)
	}
	decision := toolapproval.Evaluate(toolapproval.Request{
		Mode: mode, ProjectID: projectID, Workspace: workspace,
		ToolName: call.Name, Arguments: string(call.Arguments), Descriptor: descriptor, AttachmentPaths: attachments,
	})
	denied := decision.Action != toolapproval.ActionAllow
	if sandbox == config.CodexReadOnly {
		// Shell descriptors cover both reads and writes; the existing classifier
		// must additionally establish that the exact command is low-risk read-only.
		if descriptor.Source == agent.ToolSourceShell {
			denied = denied || decision.Risk != toolapproval.RiskLow
		} else {
			denied = denied || descriptor.MutationScope != agent.ToolMutationNone
		}
	}
	if !denied {
		return ""
	}
	slog.Info("External host tool blocked by execution permissions", "project_id", projectID, "tool", call.Name, "sandbox", sandbox, "rule", decision.RuleID)
	return fmt.Sprintf("Tool %s is blocked by the selected %s execution permissions (rule: %s). The user can change permissions in the conversation before retrying.", call.Name, sandbox, decision.RuleID)
}
