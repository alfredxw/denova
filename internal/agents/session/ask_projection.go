package session

import (
	"encoding/json"
	"fmt"

	"denova/internal/agents/sessionjournal"

	agent "github.com/alfredxw/denova/agent"
)

// Display delivery can stop before a paused interaction is answered. Read the
// canonical Agent facts when projecting that page instead of requiring a live
// display task or writing a second authoritative answer.
func applyJournalAskAnswers(entries []HistoryEntry, projection *sessionjournal.Projection) error {
	pending := make(map[string][]int)
	for index, entry := range entries {
		if entry.Ask != nil && entry.Ask.Status == AskPending {
			pending[entry.Ask.ID] = append(pending[entry.Ask.ID], index)
		}
	}
	if len(pending) == 0 || projection == nil {
		return nil
	}
	resolved := make(map[string]agent.InteractionResolution)
	finished := make(map[string]bool)
	for _, stream := range projection.Streams {
		for _, record := range stream.Facts {
			if record.Kind != "turn.interaction_response" {
				continue
			}
			var response struct {
				ID         string                      `json:"interaction_id"`
				State      string                      `json:"state"`
				Resolution agent.InteractionResolution `json:"resolution"`
			}
			if err := json.Unmarshal(record.Data, &response); err != nil {
				return fmt.Errorf("decode Agent interaction answer: %w", err)
			}
			if response.State == "answered" && len(pending[response.ID]) > 0 {
				resolved[response.ID] = response.Resolution
			}
		}
		for _, record := range stream.Turns {
			if record.Kind != "turn.finished" && record.Kind != "turn.interrupted" {
				continue
			}
			var turn struct {
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal(record.Data, &turn); err != nil {
				return fmt.Errorf("decode Agent settlement: %w", err)
			}
			finished[turn.RunID] = true
		}
	}
	for id, indexes := range pending {
		for _, index := range indexes {
			entry := &entries[index]
			ask := entry.Ask
			resolution, answered := resolved[id]
			if !answered {
				if finished[ask.AgentOperationID] || finished[entry.RunID] {
					ask.Status, entry.Status = AskCancelled, AskCancelled
				}
				continue
			}
			ask.Status = AskAnswered
			if resolution.Cancelled {
				ask.Status = AskCancelled
			}
			entry.Status = ask.Status
			for _, answer := range resolution.Answers {
				value := AskAnswerResult{QuestionID: answer.QuestionID, CustomInput: answer.Text}
				for _, question := range ask.Questions {
					if question.ID != answer.QuestionID {
						continue
					}
					value.Question = question.Question
					for _, selected := range answer.Values {
						for _, option := range question.Options {
							if option.ID == selected {
								value.SelectedOptions = append(value.SelectedOptions, AskSelectedOption{ID: selected, Label: option.Label})
							}
						}
					}
				}
				ask.Answers = append(ask.Answers, value)
			}
		}
	}
	return nil
}
