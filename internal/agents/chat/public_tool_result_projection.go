package chat

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"

	agentrun "denova/internal/agents/run"
	producttools "denova/internal/agents/tools"
	workspacechange "denova/internal/workspace/change"
)

// projectPublicToolResult derives Denova-only display data from the bounded
// public ToolResult. The projection never feeds data back into model context;
// public Agent remains the sole owner of tool execution and durable receipts.
func projectPublicToolResult(
	options agentrun.Options,
	toolName, payload string,
	eventMeta agentEventMetadata,
	data map[string]any,
	emit func(agentrun.Event),
) []error {
	var warnings []error
	if itemIDs, deletedIDs := parseWriteLoreItemsToolResult(toolName, payload); len(itemIDs) > 0 || len(deletedIDs) > 0 {
		data["item_ids"] = itemIDs
		data["deleted_ids"] = deletedIDs
	}
	if illustration, err := producttools.ParseChapterIllustrationResult(toolName, payload); err != nil {
		warnings = append(warnings, err)
	} else if illustration != nil {
		data["illustration"] = illustration
		data["target"] = illustration.MetaPath
	} else if image, err := producttools.ParseInteractiveImageResult(toolName, payload); err != nil {
		warnings = append(warnings, err)
	} else if image != nil {
		data["interactive_image"] = image
		data["target"] = image.MetaPath
		// The generate_image tool has a general image descriptor, while this
		// particular result is interactive media. Preserve the call renderer and
		// refine only the result renderer so reconnect/replay never invents a
		// second call card.
		presentation := agent.UniformToolPresentation(agent.ToolPresentationImage)
		if projected := eventDataToolPresentation(data); projected != nil {
			presentation = *projected
		}
		presentation.Result = agent.ToolPresentationInteractiveMedia
		data["tool_presentation"] = presentation
	} else if target := producttools.ParseGeneratedImageTarget(toolName, payload); target != "" {
		data["target"] = target
	}
	if receipt, ok := workspacechange.ParseToolReceipt(toolName, payload); ok {
		data["workspace_change"] = receipt
		if emit != nil {
			emit(agentrun.Event{Type: "workspace_change", Data: eventMeta.appendTo(map[string]any{
				"id": receipt.ChangeSetID, "project_id": options.ProjectID, "workspace": receipt.Workspace,
				"change_group_id": receipt.ChangeGroupID, "review_thread_id": receipt.ReviewThreadID,
				"change_set_id": receipt.ChangeSetID, "path": receipt.Path,
				"affected_paths": []string{receipt.Path}, "base_revision": receipt.BaseRevision,
				"revision": receipt.Revision, "review_status": receipt.ReviewStatus,
				"apply_state": receipt.ApplyState, "workspace_change": receipt,
			})})
		}
	}
	return warnings
}

func isWorkspaceArtifactRead(toolName, target string) bool {
	if strings.TrimSpace(toolName) != "read" {
		return false
	}
	path := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(target), "\\", "/"))
	return strings.HasPrefix(path, ".denova/artifacts/") || strings.HasPrefix(path, ".nova/artifacts/") ||
		strings.Contains(path, "/.denova/artifacts/") || strings.Contains(path, "/.nova/artifacts/") ||
		strings.Contains(path, ".jsonl.artifacts/") || strings.Contains(path, "/artifacts/game/")
}
