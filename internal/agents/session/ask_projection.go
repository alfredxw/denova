package session

import (
	"context"
	"encoding/json"
	"fmt"

	"denova/internal/agents/conversationjournal"
	"denova/internal/agents/sessionjournal"

	agent "github.com/alfredxw/denova/agent"
)

// Display delivery can stop before a paused interaction is answered. Read the
// canonical Agent facts when projecting that page instead of requiring a live
// display task or writing a second authoritative answer.
func applyJournalAskAnswers(entries []HistoryEntry, projection *sessionjournal.Projection, journal *conversationjournal.Journal) error {
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
		for id, interaction := range stream.Recovery.Interactions {
			if len(pending[id]) == 0 || interaction.Response == 0 {
				continue
			}
			record, err := projection.ReadRecord(context.Background(), journal, stream.Key, interaction.Response)
			if err != nil {
				return err
			}
			var response struct {
				State      string                      `json:"state"`
				Resolution agent.InteractionResolution `json:"resolution"`
			}
			if err := json.Unmarshal(record.Data, &response); err != nil {
				return fmt.Errorf("decode Agent interaction answer: %w", err)
			}
			if response.State == "answered" {
				resolved[id] = response.Resolution
			}
		}
		for id, run := range stream.Recovery.Runs {
			if run.Settlement != 0 {
				finished[id] = true
			}
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
