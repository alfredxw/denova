package agent

import "strings"

// ModelFinishReasonClass is the provider-neutral meaning of a raw model finish
// reason. Unknown reasons remain complete so new provider success states do not
// accidentally turn into failures.
type ModelFinishReasonClass string

const (
	ModelFinishReasonOther         ModelFinishReasonClass = "other"
	ModelFinishReasonOutputLimit   ModelFinishReasonClass = "output_limit"
	ModelFinishReasonContextLimit  ModelFinishReasonClass = "context_limit"
	ModelFinishReasonContentFilter ModelFinishReasonClass = "content_filter"
	ModelFinishReasonIncomplete    ModelFinishReasonClass = "incomplete"
)

// ClassifyModelFinishReason normalizes common provider spellings without
// discarding the raw reason retained in ResponseMeta.
func ClassifyModelFinishReason(reason string) ModelFinishReasonClass {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.Join(strings.Fields(normalized), "_")
	switch normalized {
	case "length", "max_tokens", "max_output_tokens", "token_limit":
		return ModelFinishReasonOutputLimit
	case "model_context_window_exceeded", "context_window_exceeded", "context_length_exceeded":
		return ModelFinishReasonContextLimit
	case "content_filter":
		return ModelFinishReasonContentFilter
	case "incomplete":
		return ModelFinishReasonIncomplete
	default:
		return ModelFinishReasonOther
	}
}

// Incomplete reports whether the response may be only a prefix of the model's
// intended output. Tool execution must be blocked for every such class.
func (class ModelFinishReasonClass) Incomplete() bool {
	switch class {
	case ModelFinishReasonOutputLimit, ModelFinishReasonContextLimit,
		ModelFinishReasonContentFilter, ModelFinishReasonIncomplete:
		return true
	default:
		return false
	}
}

// TerminalReason returns the stable application reason for this incomplete
// class. Complete and unknown classes have no incomplete terminal reason.
func (class ModelFinishReasonClass) TerminalReason() string {
	switch class {
	case ModelFinishReasonOutputLimit:
		return ModelOutputTruncatedReason
	case ModelFinishReasonContextLimit:
		return ModelContextWindowExceededReason
	case ModelFinishReasonContentFilter:
		return ModelOutputFilteredReason
	case ModelFinishReasonIncomplete:
		return ModelOutputIncompleteReason
	default:
		return ""
	}
}

func classifyResponseFinishReason(meta *ResponseMeta) (string, ModelFinishReasonClass) {
	if meta == nil {
		return "", ModelFinishReasonOther
	}
	reason := strings.TrimSpace(meta.FinishReason)
	return reason, ClassifyModelFinishReason(reason)
}

// IsModelIncompleteTerminalReason reports whether a settled Run reason is one
// of the stable model-incomplete codes safe to expose to localized clients.
func IsModelIncompleteTerminalReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case ModelOutputTruncatedReason, ModelContextWindowExceededReason,
		ModelOutputFilteredReason, ModelOutputIncompleteReason:
		return true
	default:
		return false
	}
}
