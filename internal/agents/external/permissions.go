package external

import (
	"fmt"
	"log/slog"

	"denova/config"
	"denova/internal/agents/toolapproval"
	agent "github.com/alfredxw/denova/agent"
)

// hostPermissionError enforces the accepted engine selection where tools
// actually execute. Engine process sandboxes do not contain host callbacks.
// These modes have no interactive escalation: denied calls return a tool error.
func (operation *Operation) hostPermissionError(call ToolCall, tool preparedTool) string {
	selection := operation.request.Input.Selection
	if selection.Kind != config.RuntimeCodex || selection.Codex == nil {
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
	attachments := make([]string, 0, len(operation.request.Input.Attachments))
	for _, attachment := range operation.request.Input.Attachments {
		path := attachment.RuntimePath
		if path == "" {
			path = attachment.Path
		}
		attachments = append(attachments, path)
	}
	decision := toolapproval.Evaluate(toolapproval.Request{
		Mode: mode, ProjectID: operation.request.ProjectID, Workspace: operation.request.ToolPolicy.Workspace,
		ToolName: call.Name, Arguments: string(call.Arguments), Descriptor: tool.definition.Descriptor, AttachmentPaths: attachments,
	})
	denied := decision.Action != toolapproval.ActionAllow
	if sandbox == config.CodexReadOnly {
		// Shell descriptors cover both reads and writes; the existing classifier
		// must additionally establish that the exact command is low-risk read-only.
		if tool.definition.Descriptor.Source == agent.ToolSourceShell {
			denied = denied || decision.Risk != toolapproval.RiskLow
		} else {
			denied = denied || tool.definition.Descriptor.MutationScope != agent.ToolMutationNone
		}
	}
	if !denied {
		return ""
	}
	slog.Info("External host tool blocked by execution permissions", "operation_id", operation.id, "tool", call.Name, "sandbox", sandbox, "rule", decision.RuleID)
	return fmt.Sprintf("Tool %s is blocked by the selected %s execution permissions (rule: %s). The user can change permissions in the conversation before retrying.", call.Name, sandbox, decision.RuleID)
}
