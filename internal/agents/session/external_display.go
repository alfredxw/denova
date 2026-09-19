package session

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	externaljournal "denova/internal/agents/external/journal"
)

// External tool cards are derived from execution facts. They are never written
// as a second display journal and answering requires no active display worker.
func appendExternalRecordLine(sess *Session, line []byte, lineNumber int) error {
	var record externaljournal.Record
	if err := json.Unmarshal(line, &record); err != nil {
		return err
	}
	switch record.Kind {
	case externaljournal.ToolStarted:
		event, err := externalStartedDisplay(record)
		if err != nil {
			return err
		}
		body, err := json.Marshal(displayRecord{Type: historyTypeDisplay, RecordID: "external-tool:" + event.ID, DisplayEvent: event})
		if err != nil {
			return err
		}
		return appendDisplayRecordLine(sess, body, lineNumber)
	case externaljournal.ToolFinished:
		var finished externaljournal.FinishedTool
		if err := json.Unmarshal(record.Data, &finished); err != nil {
			return err
		}
		for index := range sess.records {
			display := sess.records[index].display
			if display == nil || display.RunID != record.OperationID {
				continue
			}
			if display.ID != finished.ExecutionID && (display.Ask == nil || display.Ask.ToolCallID != finished.ExecutionID) {
				continue
			}
			status, err := applyExternalFinished(display.Ask, finished, record)
			if err != nil {
				return err
			}
			display.Status, display.Result = status, finished.Result
		}
	case externaljournal.OperationClosed:
		event, err := ExternalUsageDisplay(record)
		if err != nil || event == nil {
			return err
		}
		body, err := json.Marshal(displayRecord{Type: historyTypeDisplay, RecordID: event.ID, DisplayEvent: *event})
		if err != nil {
			return err
		}
		return appendDisplayRecordLine(sess, body, lineNumber)
	case externaljournal.OperationAccepted, externaljournal.ContextCheckpoint:
		// Product messages and execution metadata have their own projections.
	}
	return nil
}

// ExternalUsageDisplay derives live and restored usage from the same terminal
// fact. Provider totals do not imply a known number of model requests.
func ExternalUsageDisplay(record externaljournal.Record) (*DisplayEvent, error) {
	var closed externaljournal.Closed
	if err := json.Unmarshal(record.Data, &closed); err != nil {
		return nil, err
	}
	if closed.Usage == nil {
		return nil, nil
	}
	u := closed.Usage
	event := &DisplayEvent{ID: "external-usage:" + record.OperationID, Role: "token_usage", RunID: record.OperationID, AgentKind: closed.AgentKind, CreatedAt: record.CreatedAt,
		PromptTokens: u.PromptTokens, CachedPromptTokens: u.PromptTokenDetails.CachedTokens,
		UncachedPromptTokens: max(0, u.PromptTokens-u.PromptTokenDetails.CachedTokens), CompletionTokens: u.CompletionTokens,
		ReasoningTokens: u.CompletionTokensDetails.ReasoningTokens, TotalTokens: u.TotalTokens}
	if u.PromptTokens > 0 {
		event.CacheHitRate = float64(event.CachedPromptTokens) / float64(u.PromptTokens)
	}
	return event, nil
}

func externalStartedDisplay(record externaljournal.Record) (DisplayEvent, error) {
	var started externaljournal.StartedTool
	if err := json.Unmarshal(record.Data, &started); err != nil {
		return DisplayEvent{}, err
	}
	event := DisplayEvent{ID: started.ExecutionID, Role: "tool_call", Content: started.Tool, Name: started.Tool, Args: string(started.Arguments), Status: "running", RunID: record.OperationID, AgentKind: started.AgentKind, CreatedAt: record.CreatedAt}
	if started.Tool == "ask" {
		request, err := externaljournal.QuestionRequest(started.ExecutionID, started.Arguments)
		if err != nil {
			return DisplayEvent{}, err
		}
		ask := ProjectInteractionRequest(request, record.CreatedAt)
		ask.ToolCallID, ask.AgentOperationID, ask.AgentKind = started.ExecutionID, record.OperationID, started.AgentKind
		event.ID, event.Role, event.Status, event.Ask = ask.ID, "ask", AskPending, ask
		event.Content, event.Args = "", ""
	}
	return event, nil
}

func applyExternalFinished(ask *AskInteraction, finished externaljournal.FinishedTool, record externaljournal.Record) (string, error) {
	status := "success"
	if !finished.Success {
		status = "error"
	}
	if ask == nil {
		return status, nil
	}
	var result struct {
		Schema       string            `json:"schema"`
		ID           string            `json:"id"`
		Status       string            `json:"status"`
		Answers      []AskAnswerResult `json:"answers,omitempty"`
		CancelReason string            `json:"cancel_reason,omitempty"`
	}
	if err := json.Unmarshal([]byte(finished.Result), &result); err != nil {
		return "", err
	}
	if result.Schema != "ask.result.v1" || result.ID != ask.ID || (result.Status != AskAnswered && result.Status != AskCancelled) {
		return "", errors.New("invalid canonical external question result")
	}
	ask.Status, ask.Answers, ask.CancelReason = result.Status, result.Answers, result.CancelReason
	ask.ResolvedAt = &record.CreatedAt
	return result.Status, nil
}

func (s *Session) applyExternalHistoryOutcomesLocked(ctx context.Context, entries []HistoryEntry) error {
	if s.projection == nil || len(s.projection.External.Operations) == 0 {
		return nil
	}
	var state *ExternalState
	for index := range entries {
		entry := &entries[index]
		operation := s.projection.External.Operations[entry.RunID]
		if operation == nil {
			continue
		}
		executionID := entry.ID
		if entry.Ask != nil {
			executionID = entry.Ask.ToolCallID
			entry.Ask.AgentCommandID = operation.CommandID
		}
		tool, ok := operation.Tools[executionID]
		if !ok || tool.Finished == nil {
			continue
		}
		if state == nil {
			value, err := s.externalStateLocked(ctx)
			if err != nil {
				return err
			}
			state = &value
		}
		record, err := state.Read(*tool.Finished)
		if err != nil {
			return err
		}
		var finished externaljournal.FinishedTool
		if err := json.Unmarshal(record.Data, &finished); err != nil {
			return err
		}
		status, err := applyExternalFinished(entry.Ask, finished, record)
		if err != nil {
			return err
		}
		entry.Status, entry.Result = status, finished.Result
	}
	return nil
}

// PendingExternalAsks reads original owners even after an engine disconnect or
// process restart. Sorting by journal location preserves model question order.
func (s *Session) PendingExternalAsks(ctx context.Context) ([]*AskInteraction, error) {
	var pending []*AskInteraction
	err := s.ReadExternal(ctx, func(state ExternalState) error {
		type item struct {
			cursor  externaljournal.Locator
			command string
		}
		var items []item
		for _, operation := range state.Projection.Operations {
			for _, tool := range operation.Tools {
				if tool.Name == "ask" && tool.Finished == nil {
					items = append(items, item{tool.Started, operation.CommandID})
				}
			}
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].cursor.Cursor == items[j].cursor.Cursor {
				return items[i].cursor.Index < items[j].cursor.Index
			}
			return items[i].cursor.Cursor < items[j].cursor.Cursor
		})
		for _, item := range items {
			record, err := state.Read(item.cursor)
			if err != nil {
				return err
			}
			event, err := externalStartedDisplay(record)
			if err != nil {
				return err
			}
			event.Ask.AgentCommandID = item.command
			pending = append(pending, event.Ask)
		}
		return nil
	})
	return pending, err
}
