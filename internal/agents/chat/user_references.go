package chat

import (
	"strings"

	agentcontext "denova/internal/agents/context"
)

func userMessageReferencesForRequest(req ChatRequest) []agentcontext.UserReference {
	result := make([]agentcontext.UserReference, 0, len(req.References)+len(req.LoreReferences)+len(req.StyleScenes)+len(req.Selections)+req.ResolvedReviewFeedback.CommentCount())
	for _, path := range req.References {
		if label := strings.TrimSpace(path); label != "" {
			result = append(result, agentcontext.UserReference{Kind: "file", Label: label})
		}
	}
	for _, id := range req.LoreReferences {
		if label := strings.TrimSpace(id); label != "" {
			result = append(result, agentcontext.UserReference{Kind: "lore", ID: label, Label: label})
		}
	}
	for _, scene := range req.StyleScenes {
		if label := strings.TrimSpace(scene); label != "" {
			result = append(result, agentcontext.UserReference{Kind: "style", Label: label})
		}
	}
	for _, selection := range req.Selections {
		label := strings.TrimSpace(selection.FileName)
		if label == "" {
			label = "selection"
		}
		result = append(result, agentcontext.UserReference{
			Kind:      "selection",
			Label:     label,
			Detail:    selection.Content,
			StartLine: selection.StartLine,
			EndLine:   selection.EndLine,
		})
	}
	for _, feedback := range req.ResolvedReviewFeedback {
		for _, comment := range feedback.Comments {
			label := strings.TrimSpace(comment.Path)
			if label == "" {
				label = strings.TrimSpace(comment.ID)
			}
			if label == "" {
				continue
			}
			result = append(result, agentcontext.UserReference{
				Kind:   "review_comment",
				ID:     strings.TrimSpace(comment.ID),
				Label:  label,
				Detail: comment.Body,
			})
		}
	}
	return result
}

// UserMessageReferencesForRequest returns the Session metadata source shared
// by normal model-input commit and provider-free accepted-input materialization.
// Hosts call it only after resolving canonical review IDs.
func UserMessageReferencesForRequest(req ChatRequest) []agentcontext.UserReference {
	return userMessageReferencesForRequest(req)
}
