package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agentsession "github.com/alfredxw/denova/agent/session"
)

const maxTaskCompletionIDBytes = 1024

// TaskCompletion is one child-task result waiting for delivery to the parent
// model loop. The child Session terminal record remains the authoritative
// result; this value is only an in-process notification and bounded projection.
type TaskCompletion struct {
	ID      string
	Message *Message
}

// TaskCompletionWatch is a level-triggered snapshot plus an activity edge.
// Callers must inspect PendingIDs before waiting on Activity so a completion
// arriving immediately before subscription cannot be missed.
type TaskCompletionWatch struct {
	PendingIDs []string
	Activity   <-chan struct{}
}

type taskCompletionMailbox struct {
	pending   map[string]TaskCompletion
	order     []string
	delivered map[string]struct{}
	activity  chan struct{}
}

func newTaskCompletionMailbox() taskCompletionMailbox {
	return taskCompletionMailbox{
		pending:   make(map[string]TaskCompletion),
		delivered: make(map[string]struct{}),
		activity:  make(chan struct{}),
	}
}

// EnqueueTaskCompletion queues a completion without starting or steering a
// parent Run. It returns false when the same completion is already pending or
// durably delivered.
func (session *Session) EnqueueTaskCompletion(ctx context.Context, completion TaskCompletion) (bool, error) {
	if session == nil {
		return false, ErrSessionClosed
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return false, err
		}
	}
	completion.ID = strings.TrimSpace(completion.ID)
	if err := validateTaskCompletion(completion); err != nil {
		return false, err
	}
	completion.Message = completion.Message.Clone()

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return false, ErrSessionClosed
	}
	if _, ok := session.taskCompletions.delivered[completion.ID]; ok {
		return false, nil
	}
	if _, ok := session.taskCompletions.pending[completion.ID]; ok {
		return false, nil
	}
	session.taskCompletions.pending[completion.ID] = completion
	session.taskCompletions.order = append(session.taskCompletions.order, completion.ID)
	close(session.taskCompletions.activity)
	session.taskCompletions.activity = make(chan struct{})
	return true, nil
}

// WatchTaskCompletions atomically checks the requested IDs and subscribes to
// future mailbox activity. Activity is intentionally broader than the filter;
// callers re-check PendingIDs after every wakeup.
func (session *Session) WatchTaskCompletions(ctx context.Context, ids []string) (TaskCompletionWatch, error) {
	if session == nil {
		return TaskCompletionWatch{}, ErrSessionClosed
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return TaskCompletionWatch{}, err
		}
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	if session.closed {
		return TaskCompletionWatch{}, ErrSessionClosed
	}
	pending := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := session.taskCompletions.pending[id]; ok {
			pending = append(pending, id)
		}
	}
	return TaskCompletionWatch{PendingIDs: pending, Activity: session.taskCompletions.activity}, nil
}

func validateTaskCompletion(completion TaskCompletion) error {
	if completion.ID == "" {
		return errors.New("task completion requires an ID")
	}
	if len(completion.ID) > maxTaskCompletionIDBytes {
		return fmt.Errorf("task completion ID exceeds %d bytes", maxTaskCompletionIDBytes)
	}
	message := completion.Message
	if message == nil || message.Role != User || message.TaskCompletion == nil {
		return errors.New("task completion requires a typed user message")
	}
	if strings.TrimSpace(message.TaskCompletion.CompletionID) != completion.ID {
		return errors.New("task completion message ID does not match its envelope")
	}
	if strings.TrimSpace(message.TaskCompletion.Author) == "" || strings.TrimSpace(message.TaskCompletion.Recipient) == "" {
		return errors.New("task completion message requires author and recipient")
	}
	if len(message.ToolCalls) != 0 || message.ToolCallID != "" || message.ToolResult != nil {
		return errors.New("task completion cannot carry tool protocol fields")
	}
	return nil
}

func (session *Session) pendingTaskCompletions() []TaskCompletion {
	if session == nil {
		return nil
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	if session.closed || len(session.taskCompletions.pending) == 0 {
		return nil
	}
	result := make([]TaskCompletion, 0, len(session.taskCompletions.pending))
	for _, id := range session.taskCompletions.order {
		completion, ok := session.taskCompletions.pending[id]
		if !ok {
			continue
		}
		completion.Message = completion.Message.Clone()
		result = append(result, completion)
	}
	return result
}

func (session *Session) commitTaskCompletionCheckpoint(
	ctx context.Context,
	state json.RawMessage,
	ids []string,
) error {
	if len(state) == 0 || len(ids) == 0 {
		return errors.New("task completion checkpoint requires state and delivery IDs")
	}
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > maxTaskCompletionIDBytes {
			return errors.New("task completion checkpoint contains an invalid delivery ID")
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return errors.New("task completion checkpoint contains no delivery IDs")
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	for _, id := range unique {
		if _, pending := session.taskCompletions.pending[id]; !pending {
			return fmt.Errorf("task completion %q is no longer pending", id)
		}
	}
	if session.canonicalMessages {
		previous := session.engineState
		session.engineState = append(json.RawMessage(nil), state...)
		if err := session.persistTranscriptLocked(ctx); err != nil {
			session.engineState = previous
			return err
		}
	} else {
		transcript, err := json.Marshal(persistedSessionTranscript{
			EngineState: append(json.RawMessage(nil), state...),
		})
		if err != nil {
			return fmt.Errorf("encode Agent Session transcript: %w", err)
		}
		delivery, err := json.Marshal(persistedTaskCompletionDelivery{IDs: append([]string(nil), unique...)})
		if err != nil {
			return fmt.Errorf("encode Agent task completion delivery: %w", err)
		}
		if err := session.appendRecordsLocked(ctx,
			agentsession.Record{Kind: sessionTranscriptRecord, Version: sessionRecordVersion, Data: transcript},
			agentsession.Record{Kind: sessionTaskCompletionDeliveryRecord, Version: sessionRecordVersion, Data: delivery},
		); err != nil {
			return err
		}
	}

	session.engineState = append(json.RawMessage(nil), state...)
	for _, id := range unique {
		session.taskCompletions.delivered[id] = struct{}{}
		delete(session.taskCompletions.pending, id)
	}
	retained := session.taskCompletions.order[:0]
	for _, id := range session.taskCompletions.order {
		if _, delivered := seen[id]; !delivered {
			retained = append(retained, id)
		}
	}
	session.taskCompletions.order = retained
	return nil
}

type taskCompletionSessionContextKey struct{}

func contextWithTaskCompletionSession(ctx context.Context, session *Session) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if session == nil {
		return ctx
	}
	return context.WithValue(ctx, taskCompletionSessionContextKey{}, session)
}

func pendingTaskCompletionsFromContext(ctx context.Context) []TaskCompletion {
	if ctx == nil {
		return nil
	}
	session, _ := ctx.Value(taskCompletionSessionContextKey{}).(*Session)
	return session.pendingTaskCompletions()
}
