package agent

import (
	"strings"

	"denova/internal/session"
)

func userMessageReferencesForRequest(req ChatRequest) []session.UserMessageReference {
	result := make([]session.UserMessageReference, 0, len(req.References)+len(req.LoreReferences)+len(req.StyleScenes)+len(req.Selections)+len(req.ResolvedReviewFeedback.Comments))
	for _, path := range req.References {
		if label := strings.TrimSpace(path); label != "" {
			result = append(result, session.UserMessageReference{Kind: "file", Label: label})
		}
	}
	for _, id := range req.LoreReferences {
		if label := strings.TrimSpace(id); label != "" {
			result = append(result, session.UserMessageReference{Kind: "lore", ID: label, Label: label})
		}
	}
	for _, scene := range req.StyleScenes {
		if label := strings.TrimSpace(scene); label != "" {
			result = append(result, session.UserMessageReference{Kind: "style", Label: label})
		}
	}
	for _, selection := range req.Selections {
		label := strings.TrimSpace(selection.FileName)
		if label == "" {
			label = "selection"
		}
		result = append(result, session.UserMessageReference{
			Kind:      "selection",
			Label:     label,
			Detail:    selection.Content,
			StartLine: selection.StartLine,
			EndLine:   selection.EndLine,
		})
	}
	for _, comment := range req.ResolvedReviewFeedback.Comments {
		label := strings.TrimSpace(comment.Path)
		if label == "" {
			label = strings.TrimSpace(comment.ID)
		}
		if label == "" {
			continue
		}
		result = append(result, session.UserMessageReference{
			Kind:   "review_comment",
			ID:     strings.TrimSpace(comment.ID),
			Label:  label,
			Detail: comment.Body,
		})
	}
	return result
}
