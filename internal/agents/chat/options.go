package chat

import (
	"strings"

	"denova/internal/agents/run"
)

const runTraceMetadataValueMaxBytes = 256

func truncateUTF8StringBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && (value[maxBytes]&0xC0) == 0x80 {
		maxBytes--
	}
	return value[:maxBytes]
}

func runTraceMetadataForConversation(options agentrun.Options, conversation Conversation) agentrun.TraceMetadata {
	metadata := agentrun.TraceMetadata{
		StoryID:         options.StoryID,
		BranchID:        options.BranchID,
		TurnID:          options.TurnID,
		MaintenanceTask: options.MaintenanceTask,
	}
	if reporter, ok := conversation.(agentrun.TraceMetadataReporter); ok {
		reported := reporter.RunTraceMetadata()
		if strings.TrimSpace(reported.StoryID) != "" {
			metadata.StoryID = reported.StoryID
		}
		if strings.TrimSpace(reported.BranchID) != "" {
			metadata.BranchID = reported.BranchID
		}
		if strings.TrimSpace(reported.TurnID) != "" {
			metadata.TurnID = reported.TurnID
		}
		if strings.TrimSpace(reported.MaintenanceTask) != "" {
			metadata.MaintenanceTask = reported.MaintenanceTask
		}
	}
	metadata.StoryID = boundedRunTraceMetadataValue(metadata.StoryID)
	metadata.BranchID = boundedRunTraceMetadataValue(metadata.BranchID)
	metadata.TurnID = boundedRunTraceMetadataValue(metadata.TurnID)
	metadata.MaintenanceTask = boundedRunTraceMetadataValue(metadata.MaintenanceTask)
	return metadata
}

func boundedRunTraceMetadataValue(value string) string {
	return truncateUTF8StringBytes(strings.TrimSpace(value), runTraceMetadataValueMaxBytes)
}

func runTraceMetadataEmpty(metadata agentrun.TraceMetadata) bool {
	return metadata.StoryID == "" && metadata.BranchID == "" && metadata.TurnID == "" && metadata.MaintenanceTask == ""
}

func runTraceMetadataRecord(metadata agentrun.TraceMetadata) map[string]any {
	return map[string]any{
		"story_id":         metadata.StoryID,
		"branch_id":        metadata.BranchID,
		"turn_id":          metadata.TurnID,
		"maintenance_task": metadata.MaintenanceTask,
	}
}
