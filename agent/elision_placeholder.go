package agent

import (
	"encoding/json"
	"strings"
)

// A placeholder keeps the exact recovery reference and outcome receipt. If
// either cannot fit, the body stays intact; recovery instructions are never cut.
func elisionPlaceholder(message *Message) string {
	if message == nil || message.Role != ToolRole || message.ToolResult == nil ||
		len(message.Attachments) != 0 || len(message.MultiContent) != 0 || len(message.AssistantGenMultiContent) != 0 {
		return ""
	}
	result := message.ToolResult
	if result.Status != ToolResultSuccess || result.SyntheticReason != "" || result.ResultRetention == ToolResultProtected ||
		result.ContextHints == nil {
		return ""
	}
	if persistence := result.ArtifactPersistence; persistence != nil && persistence.Attempted && !persistence.Complete {
		return ""
	}
	recovery := result.ContextHints.Recovery
	var instruction string
	switch recovery.Kind {
	case ToolResultRecoveryRead, ToolResultRecoveryRefetch, ToolResultRecoveryRerun:
		if len(recovery.Reference) == 0 || !completeElisionReference(recovery.Reference) {
			return ""
		}
		encoded, err := json.Marshal(recovery.Reference)
		if err != nil {
			return ""
		}
		action := "Read current content again with the original tool"
		switch recovery.Kind {
		case ToolResultRecoveryRefetch:
			action = "Fetch the source again with the original tool"
		case ToolResultRecoveryRerun:
			action = "Rerun the retained read-only tool invocation"
		}
		instruction = action + " and these arguments: " + string(encoded) + ". The source may have changed since this observation."
	case ToolResultRecoveryArtifact:
		for _, artifact := range result.Artifacts {
			if artifact.ReadablePath != "" && artifact.ReadablePath == recovery.ArtifactPath && artifact.Complete && artifact.ContentType != "" &&
				(artifact.Purpose == ToolArtifactPurposeCompleteModelOutput || artifact.Purpose == ToolArtifactPurposeCompleteToolOutput) {
				instruction = "Read the complete saved output at " + artifact.ReadablePath + ". Do not repeat the original operation to recover its output."
				break
			}
		}
		if instruction == "" {
			return ""
		}
	default:
		return ""
	}
	stub := "[Earlier tool output elided; status=success.] " + instruction
	if receipt := result.ProtectedReceipt; receipt != nil {
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return ""
		}
		stub += "\nOperation receipt: " + string(encoded)
	}
	if len(stub) > 12*1024 {
		return ""
	}
	return stub
}

func completeElisionReference(value any) bool {
	switch typed := value.(type) {
	case string:
		return !strings.Contains(typed, toolResultHintRedactedValue) && !strings.Contains(typed, toolResultHintTruncatedValue) &&
			!strings.HasSuffix(strings.ToLower(strings.TrimSpace(typed)), "...[truncated]")
	case map[string]any:
		for key, child := range typed {
			if !completeElisionReference(key) || !completeElisionReference(child) {
				return false
			}
		}
	case []any:
		for _, child := range typed {
			if !completeElisionReference(child) {
				return false
			}
		}
	case nil, bool, json.Number, float64, int:
	default:
		return false
	}
	return true
}
