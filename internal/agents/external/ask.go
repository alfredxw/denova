package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"denova/internal/agents/conversation"
	externaljournal "denova/internal/agents/external/journal"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

var (
	ErrAskNotFound = errors.New("external runtime question was not found")
	ErrAskConflict = errors.New("external runtime question has already been resolved differently")
)

// Interactions contains only wakeups. Pending questions and saved answers are
// derived exclusively from tool.started/tool.finished in the Product journal.
// A zero value is ready for use; restarting it does not lose any question.
type Interactions struct {
	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
}

// QuestionRequest reuses the public Ask validator without invoking the Native
// tool's Run-dependent waiting implementation. Free text and Other follow the
// existing Ask schema even if the model supplies extra repairable fields.
func QuestionRequest(executionID string, arguments json.RawMessage) (agent.InteractionRequest, error) {
	return externaljournal.QuestionRequest(executionID, arguments)
}

// Wait registers before reading the journal, so an answer racing registration
// cannot be lost. Cancelling this process-local waiter does not resolve the Ask;
// the operation's durable cancellation transaction owns that transition.
func (interactions *Interactions) Wait(ctx context.Context, projectID string, sess *session.Session, operationID, executionID string) (conversation.HostAskResolution, error) {
	if strings.TrimSpace(projectID) == "" || sess == nil {
		return conversation.HostAskResolution{}, errors.New("question requires a Project Session")
	}
	var incarnation string
	if err := sess.ReadExternal(ctx, func(state session.ExternalState) error { incarnation = state.Incarnation; return nil }); err != nil {
		return conversation.HostAskResolution{}, err
	}
	key := askWaitKey(projectID, incarnation, operationID, executionID)
	wake, release := interactions.subscribe(key)
	defer release()
	for {
		var result conversation.HostAskResolution
		resolved := false
		err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
			if state.Incarnation != incarnation {
				return ErrAskNotFound
			}
			operation := state.Projection.Operations[operationID]
			if operation == nil {
				return ErrAskNotFound
			}
			tool, exists := operation.Tools[executionID]
			if !exists || tool.Name != "ask" {
				return ErrAskNotFound
			}
			if tool.Finished == nil {
				return nil
			}
			value, err := readAskResult(state, *tool.Finished)
			result, resolved = value, err == nil
			return err
		})
		if err != nil {
			return conversation.HostAskResolution{}, err
		}
		if resolved {
			return result, nil
		}
		select {
		case <-ctx.Done():
			return conversation.HostAskResolution{}, ctx.Err()
		case <-wake:
			// The durable transition already committed before the wakeup. A
			// clear or source invalidation is detected by the subsequent read.
		}
	}
}

// Resolve routes by the original execution identity, not the currently selected
// engine. Same-answer retries return the saved result; conflicting answers fail.
func (interactions *Interactions) Resolve(ctx context.Context, projectID string, sess *session.Session, askID string, answers []conversation.HostAskAnswer, cancelReason *string) (conversation.HostAskResolution, error) {
	if projectID == "" || sess == nil {
		return conversation.HostAskResolution{}, ErrAskNotFound
	}
	executionID := strings.TrimPrefix(askID, "ask-")
	if executionID == askID || executionID == "" {
		return conversation.HostAskResolution{}, ErrAskNotFound
	}
	var revision uint64
	if err := sess.ReadExternal(ctx, func(state session.ExternalState) error {
		op, _, err := findQuestion(state, executionID)
		if err != nil {
			return err
		}
		revision = op.ConfigRevision
		return nil
	}); err != nil {
		return conversation.HostAskResolution{}, err
	}
	var result conversation.HostAskResolution
	var wakeKey string
	err := sess.UpdateExternal(ctx, revision, func(state session.ExternalState) (session.ExternalTransaction, error) {
		op, tool, err := findQuestion(state, executionID)
		if err != nil {
			return session.ExternalTransaction{}, err
		}
		source, err := state.Read(tool.Started)
		if err != nil {
			return session.ExternalTransaction{}, err
		}
		var start externaljournal.StartedTool
		if err := json.Unmarshal(source.Data, &start); err != nil {
			return session.ExternalTransaction{}, err
		}
		request, err := QuestionRequest(executionID, start.Arguments)
		if err != nil {
			return session.ExternalTransaction{}, err
		}
		candidate, err := resolveQuestion(ctx, request, answers, cancelReason)
		if err != nil {
			return session.ExternalTransaction{}, err
		}
		wakeKey = askWaitKey(projectID, state.Incarnation, op.ID, executionID)
		if tool.Finished != nil {
			saved, err := readAskResult(state, *tool.Finished)
			if err != nil {
				return session.ExternalTransaction{}, err
			}
			if !reflect.DeepEqual(saved, candidate) {
				return session.ExternalTransaction{}, ErrAskConflict
			}
			result = saved
			return session.ExternalTransaction{}, nil
		}
		if op.Status != externaljournal.Running && op.Status != externaljournal.Interrupted {
			return session.ExternalTransaction{}, ErrAskConflict
		}
		body, err := json.Marshal(candidate)
		if err != nil {
			return session.ExternalTransaction{}, err
		}
		if len(body) > 256<<10 {
			return session.ExternalTransaction{}, errors.New("question result exceeds 256 KiB")
		}
		record, err := externaljournal.NewRecord(externaljournal.ToolFinished, op.ID, revision, externaljournal.FinishedTool{ExecutionID: executionID, Success: true, Result: string(body)})
		if err != nil {
			return session.ExternalTransaction{}, err
		}
		result = candidate
		return session.ExternalTransaction{Records: []externaljournal.Record{record}}, nil
	})
	if err != nil {
		return conversation.HostAskResolution{}, err
	}
	if wakeKey != "" {
		interactions.notify(wakeKey)
	}
	return result, nil
}

func findQuestion(state session.ExternalState, executionID string) (*externaljournal.Operation, externaljournal.ToolState, error) {
	for _, operation := range state.Projection.Operations {
		if tool, exists := operation.Tools[executionID]; exists && tool.Name == "ask" {
			return operation, tool, nil
		}
	}
	return nil, externaljournal.ToolState{}, ErrAskNotFound
}

func readAskResult(state session.ExternalState, locator externaljournal.Locator) (conversation.HostAskResolution, error) {
	record, err := state.Read(locator)
	if err != nil {
		return conversation.HostAskResolution{}, err
	}
	var finished externaljournal.FinishedTool
	if err := json.Unmarshal(record.Data, &finished); err != nil {
		return conversation.HostAskResolution{}, err
	}
	var result conversation.HostAskResolution
	if err := json.Unmarshal([]byte(finished.Result), &result); err != nil {
		return conversation.HostAskResolution{}, err
	}
	if result.Schema != "ask.result.v1" || (result.Status != session.AskAnswered && result.Status != session.AskCancelled) {
		return conversation.HostAskResolution{}, errors.New("invalid saved question result")
	}
	return result, nil
}

func resolveQuestion(ctx context.Context, request agent.InteractionRequest, answers []conversation.HostAskAnswer, cancelReason *string) (conversation.HostAskResolution, error) {
	response := agent.InteractionResponse{Cancelled: cancelReason != nil, Answers: conversation.InteractionAnswers(answers)}
	resolution, err := agent.StandardInteraction().Resolve(ctx, request, response)
	if err != nil {
		return conversation.HostAskResolution{}, err
	}
	reason := ""
	if cancelReason != nil {
		reason = *cancelReason
	}
	return conversation.ProjectAskResolution(request, resolution, reason), nil
}

func askWaitKey(projectID, incarnation, operationID, executionID string) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s", projectID, incarnation, operationID, executionID)
}

func (interactions *Interactions) subscribe(key string) (<-chan struct{}, func()) {
	interactions.mu.Lock()
	defer interactions.mu.Unlock()
	if interactions.waiters == nil {
		interactions.waiters = map[string]map[chan struct{}]struct{}{}
	}
	if interactions.waiters[key] == nil {
		interactions.waiters[key] = map[chan struct{}]struct{}{}
	}
	wake := make(chan struct{})
	interactions.waiters[key][wake] = struct{}{}
	return wake, func() {
		interactions.mu.Lock()
		defer interactions.mu.Unlock()
		delete(interactions.waiters[key], wake)
		if len(interactions.waiters[key]) == 0 {
			delete(interactions.waiters, key)
		}
	}
}

func (interactions *Interactions) notify(key string) {
	interactions.mu.Lock()
	defer interactions.mu.Unlock()
	for wake := range interactions.waiters[key] {
		close(wake)
	}
	delete(interactions.waiters, key)
}
