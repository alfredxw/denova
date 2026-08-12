package documentreview

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	workspacelayout "denova/internal/workspace"
)

const (
	maxCommentBodyBytes = 64 * 1024
)

var reviewServices = struct {
	sync.Mutex
	items map[string]*Service
}{items: map[string]*Service{}}

// eventLog abstracts the durable ledger backing a Service. Depending on the
// interface instead of the concrete *eventStore keeps the replay/apply logic
// testable with in-memory fakes and isolates storage concerns behind one seam.
type eventLog interface {
	append(event ledgerEvent) error
	readAll() ([]ledgerEvent, error)
	close()
}

// Service owns one Project's durable author-created text-resource comments.
// Resource stores retain mutation ownership; this service records review
// metadata only and never rewrites manuscripts or lore entries.
type Service struct {
	workspace string
	stateRoot string
	store     eventLog

	mu       sync.RWMutex
	threads  map[string]*Thread
	comments map[string]*Comment
	// threadComments indexes non-deleted comment IDs by thread ID so
	// currentThreadLocked can iterate only a thread's own comments instead of
	// scanning every comment in the workspace (O(threads × comments)).
	threadComments map[string]map[string]struct{}
	order          []string
}

func ForWorkspace(workspace string) (*Service, error) {
	canonical, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	stateRoot, err := normalizeStateRoot(workspacelayout.Dir(canonical))
	if err != nil {
		return nil, err
	}
	return forWorkspaceAt(canonical, stateRoot)
}

// ForWorkspaceAt returns the process-wide review service identified by the
// stable Project state root. The content path is presentation context and may
// be rebound after a Project directory is relinked.
func ForWorkspaceAt(workspace, stateRoot string) (*Service, error) {
	canonical, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	canonicalStateRoot, err := normalizeStateRoot(stateRoot)
	if err != nil {
		return nil, err
	}
	return forWorkspaceAt(canonical, canonicalStateRoot)
}

func forWorkspaceAt(workspace, stateRoot string) (*Service, error) {
	reviewServices.Lock()
	defer reviewServices.Unlock()
	if existing := reviewServices.items[stateRoot]; existing != nil {
		existing.bindWorkspace(workspace)
		return existing, nil
	}
	service, err := newRuntime(workspace, stateRoot)
	if err != nil {
		return nil, err
	}
	reviewServices.items[stateRoot] = service
	return service, nil
}

// NewService creates an isolated service for tests.
func NewService(workspace string) (*Service, error) {
	canonical, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	stateRoot, err := normalizeStateRoot(workspacelayout.Dir(canonical))
	if err != nil {
		return nil, err
	}
	return newRuntime(canonical, stateRoot)
}

func newRuntime(workspace, stateRoot string) (*Service, error) {
	store, err := newEventStore(stateRoot)
	if err != nil {
		return nil, err
	}
	service := &Service{
		workspace:      workspace,
		stateRoot:      stateRoot,
		store:          store,
		threads:        map[string]*Thread{},
		comments:       map[string]*Comment{},
		threadComments: map[string]map[string]struct{}{},
	}
	events, err := store.readAll()
	if err != nil {
		store.close()
		return nil, err
	}
	for _, event := range events {
		if err := service.applyEvent(event); err != nil {
			store.close()
			return nil, err
		}
	}
	return service, nil
}

func normalizeWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", newError(ErrorCodeConflict, "workspace path is empty", nil)
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", newError(ErrorCodeConflict, "workspace path is not a directory", map[string]any{"workspace": abs})
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}

func normalizeStateRoot(stateRoot string) (string, error) {
	stateRoot = strings.TrimSpace(stateRoot)
	if stateRoot == "" {
		return "", newError(ErrorCodeConflict, "project state root is empty", nil)
	}
	abs, err := filepath.Abs(stateRoot)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", newError(ErrorCodeConflict, "project state root is not a directory", map[string]any{"state_root": abs})
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}

func (s *Service) Workspace() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workspace
}

// StateRoot returns the stable Project-state boundary holding the ledger.
func (s *Service) StateRoot() string {
	if s == nil {
		return ""
	}
	return s.stateRoot
}

func (s *Service) bindWorkspace(workspace string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspace = workspace
}

func (s *Service) CurrentThread(ctx context.Context) (Thread, error) {
	if s == nil {
		return Thread{}, newError(ErrorCodeConflict, "text review service is nil", nil)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return Thread{}, err
	}
	return s.currentThreadLocked(), nil
}

func (s *Service) AddComment(ctx context.Context, req AddCommentRequest, snapshot Snapshot) (Thread, Comment, error) {
	if s == nil {
		return Thread{}, Comment{}, newError(ErrorCodeConflict, "text review service is nil", nil)
	}
	target, err := NormalizeTarget(req.Target)
	if err != nil {
		return Thread{}, Comment{}, err
	}
	body, err := validateBody(req.Body)
	if err != nil {
		return Thread{}, Comment{}, err
	}
	req.Anchor = normalizeAnchor(req.Anchor)
	if err := ValidateAnchor(snapshot, req.Anchor); err != nil {
		return Thread{}, Comment{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Thread{}, Comment{}, err
	}
	now := time.Now().UTC()
	thread := s.currentThreadLocked()
	if thread.ID == "" {
		thread = Thread{ID: newID("review-thread"), CreatedAt: now, UpdatedAt: now, Comments: []Comment{}}
		if err := s.appendAndApply(ledgerEvent{Type: eventThreadCreated, CreatedAt: now, Thread: cloneThreadPtr(thread)}); err != nil {
			return Thread{}, Comment{}, err
		}
	}
	comment := Comment{
		ID:        newID("document-comment"),
		ThreadID:  thread.ID,
		Target:    target,
		Body:      body,
		Anchor:    req.Anchor,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentsUpserted, CreatedAt: now, Comments: []Comment{comment}}); err != nil {
		return Thread{}, Comment{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[document-review] comment created workspace=%q target_kind=%q target_id=%q target_field=%q thread_id=%s comment_id=%s", s.workspace, comment.Target.Kind, comment.Target.ID, comment.Target.Field, comment.ThreadID, comment.ID))
	return s.currentThreadLocked(), comment, nil
}

func (s *Service) UpdateComment(ctx context.Context, req UpdateCommentRequest) (Thread, Comment, error) {
	body, err := validateBody(req.Body)
	if err != nil {
		return Thread{}, Comment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Thread{}, Comment{}, err
	}
	current := s.comments[strings.TrimSpace(req.ID)]
	if current == nil || current.Deleted {
		return Thread{}, Comment{}, newError(ErrorCodeNotFound, "document comment not found", map[string]any{"comment_id": req.ID})
	}
	next := *current
	next.Body = body
	next.UpdatedAt = time.Now().UTC()
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentsUpserted, CreatedAt: next.UpdatedAt, Comments: []Comment{next}}); err != nil {
		return Thread{}, Comment{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[document-review] comment updated workspace=%q target_kind=%q target_id=%q thread_id=%s comment_id=%s", s.workspace, next.Target.Kind, next.Target.ID, next.ThreadID, next.ID))
	return s.currentThreadLocked(), next, nil
}

func (s *Service) DeleteComment(ctx context.Context, req DeleteCommentRequest) (Thread, Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Thread{}, Comment{}, err
	}
	current := s.comments[strings.TrimSpace(req.ID)]
	if current == nil || current.Deleted {
		return Thread{}, Comment{}, newError(ErrorCodeNotFound, "document comment not found", map[string]any{"comment_id": req.ID})
	}
	next := *current
	next.Deleted = true
	next.UpdatedAt = time.Now().UTC()
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentsUpserted, CreatedAt: next.UpdatedAt, Comments: []Comment{next}}); err != nil {
		return Thread{}, Comment{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[document-review] comment deleted workspace=%q target_kind=%q target_id=%q thread_id=%s comment_id=%s", s.workspace, next.Target.Kind, next.Target.ID, next.ThreadID, next.ID))
	return s.currentThreadLocked(), next, nil
}

// GetReviewComments resolves one exact pending feedback batch in caller order.
func (s *Service) GetReviewComments(ctx context.Context, threadID string, commentIDs []string) ([]Comment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return s.reviewCommentsLocked(threadID, commentIDs)
}

// ConsumeReviewComments marks the batch deleted after its user message has
// crossed the durable conversation boundary.
func (s *Service) ConsumeReviewComments(ctx context.Context, threadID string, commentIDs []string) ([]Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	comments, err := s.reviewCommentsLocked(threadID, commentIDs)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	consumed := make([]Comment, 0, len(comments))
	for _, comment := range comments {
		comment.Deleted = true
		comment.UpdatedAt = now
		consumed = append(consumed, comment)
	}
	if len(consumed) == 0 {
		return nil, nil
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentsUpserted, CreatedAt: now, Comments: consumed}); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[document-review] feedback consumed workspace=%q thread_id=%s comment_count=%d", s.workspace, threadID, len(consumed)))
	return append([]Comment{}, consumed...), nil
}

// ConsumeReviewCommentsForAgentInput persists an exact canonical-input
// receipt on every consumed comment. Exact retries are no-ops; unrelated
// deletions cannot satisfy the Agent outbox.
func (s *Service) ConsumeReviewCommentsForAgentInput(ctx context.Context, threadID string, commentIDs []string, effectID string) ([]Comment, error) {
	effectID = strings.TrimSpace(effectID)
	if effectID == "" || len(effectID) > 128 {
		return nil, newError(ErrorCodeInvalid, "Agent input effect id is invalid", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	pending, err := s.reviewCommentsForAgentInputLocked(threadID, commentIDs, effectID)
	if err != nil || len(pending) == 0 {
		return nil, err
	}
	now := time.Now().UTC()
	for index := range pending {
		pending[index].Deleted = true
		pending[index].AgentInputEffectID = effectID
		pending[index].UpdatedAt = now
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentsUpserted, CreatedAt: now, Comments: pending}); err != nil {
		return nil, err
	}
	return append([]Comment(nil), pending...), nil
}

func (s *Service) ValidateReviewCommentsForAgentInput(ctx context.Context, threadID string, commentIDs []string, effectID string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return err
	}
	_, err := s.reviewCommentsForAgentInputLocked(threadID, commentIDs, strings.TrimSpace(effectID))
	return err
}

// ReviewCommentsConsumed is the read-only receipt query for an Agent input
// obligation. It never mutates or resurrects comment state.
func (s *Service) ReviewCommentsConsumed(ctx context.Context, threadID string, commentIDs []string, effectID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return false, err
	}
	threadID = strings.TrimSpace(threadID)
	effectID = strings.TrimSpace(effectID)
	if effectID == "" {
		return false, newError(ErrorCodeInvalid, "Agent input effect id is required", nil)
	}
	if s.threads[threadID] == nil {
		return false, newError(ErrorCodeNotFound, "document review thread not found", map[string]any{"review_thread_id": threadID})
	}
	for _, requestedID := range commentIDs {
		comment := s.comments[strings.TrimSpace(requestedID)]
		if comment == nil || comment.ThreadID != threadID {
			return false, newError(ErrorCodeConflict, "document review comment is unavailable", map[string]any{"review_thread_id": threadID, "comment_id": requestedID})
		}
		if !comment.Deleted || comment.AgentInputEffectID != effectID {
			return false, nil
		}
	}
	return true, nil
}

func (s *Service) reviewCommentsForAgentInputLocked(threadID string, commentIDs []string, effectID string) ([]Comment, error) {
	threadID = strings.TrimSpace(threadID)
	if effectID == "" {
		return nil, newError(ErrorCodeInvalid, "Agent input effect id is required", nil)
	}
	if s.threads[threadID] == nil {
		return nil, newError(ErrorCodeNotFound, "document review thread not found", map[string]any{"review_thread_id": threadID})
	}
	seen := make(map[string]bool, len(commentIDs))
	pending := make([]Comment, 0, len(commentIDs))
	for _, requestedID := range commentIDs {
		id := strings.TrimSpace(requestedID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		comment := s.comments[id]
		if comment == nil || comment.ThreadID != threadID {
			return nil, newError(ErrorCodeConflict, "document review comment is unavailable", map[string]any{"review_thread_id": threadID, "comment_id": id})
		}
		if comment.Deleted {
			if comment.AgentInputEffectID != effectID {
				return nil, newError(ErrorCodeConflict, "document review comment was consumed by another action", map[string]any{"comment_id": id})
			}
			continue
		}
		pending = append(pending, *comment)
	}
	return pending, nil
}

// RestoreConsumedReviewComments compensates a failed cross-ledger feedback
// batch. It only restores the exact comment versions returned by consumption.
func (s *Service) RestoreConsumedReviewComments(ctx context.Context, threadID string, consumed []Comment) ([]Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	threadID = strings.TrimSpace(threadID)
	if s.threads[threadID] == nil {
		return nil, newError(ErrorCodeNotFound, "document review thread not found", map[string]any{"review_thread_id": threadID})
	}
	now := time.Now().UTC()
	restored := make([]Comment, 0, len(consumed))
	seen := make(map[string]bool, len(consumed))
	for _, consumedComment := range consumed {
		id := strings.TrimSpace(consumedComment.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		current := s.comments[id]
		if current == nil || current.ThreadID != threadID {
			return nil, newError(ErrorCodeConflict, "consumed document review comment changed threads", map[string]any{
				"review_thread_id": threadID, "comment_id": id,
			})
		}
		if !current.Deleted {
			continue
		}
		if !current.UpdatedAt.Equal(consumedComment.UpdatedAt) {
			return nil, newError(ErrorCodeConflict, "consumed document review comment changed after consumption", map[string]any{"comment_id": id})
		}
		next := *current
		next.Deleted = false
		next.AgentInputEffectID = ""
		next.UpdatedAt = now
		restored = append(restored, next)
	}
	if len(restored) == 0 {
		return nil, nil
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentsUpserted, CreatedAt: now, Comments: restored}); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[document-review] feedback consumption restored workspace=%q thread_id=%s comment_count=%d", s.workspace, threadID, len(restored)))
	return append([]Comment{}, restored...), nil
}

func (s *Service) reviewCommentsLocked(threadID string, commentIDs []string) ([]Comment, error) {
	threadID = strings.TrimSpace(threadID)
	if s.threads[threadID] == nil {
		return nil, newError(ErrorCodeNotFound, "document review thread not found", map[string]any{"review_thread_id": threadID})
	}
	seen := make(map[string]bool, len(commentIDs))
	result := make([]Comment, 0, len(commentIDs))
	for _, id := range commentIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		comment := s.comments[id]
		if comment == nil || comment.Deleted || comment.ThreadID != threadID {
			return nil, newError(ErrorCodeConflict, "document review comment is unavailable", map[string]any{
				"review_thread_id": threadID, "comment_id": id,
			})
		}
		result = append(result, *comment)
	}
	if len(result) == 0 {
		return nil, newError(ErrorCodeConflict, "document review feedback is empty", nil)
	}
	return result, nil
}

func (s *Service) appendAndApply(event ledgerEvent) error {
	if err := s.store.append(event); err != nil {
		return err
	}
	return s.applyEvent(event)
}

func (s *Service) applyEvent(event ledgerEvent) error {
	switch event.Type {
	case eventThreadCreated:
		return s.applyThreadCreated(event)
	case eventCommentsUpserted:
		return s.applyCommentsUpserted(event)
	default:
		return errors.New("unknown document review ledger event type: " + event.Type)
	}
}

// applyThreadCreated seeds a new thread into the read projection. It is also
// reached while replaying legacy comments_upserted events that smuggled the
// thread in via the optional Thread field, so older ledgers remain replayable.
func (s *Service) applyThreadCreated(event ledgerEvent) error {
	if event.Thread == nil {
		return newError(ErrorCodeConflict, "document review thread_created event has no thread", nil)
	}
	thread := cloneThread(*event.Thread)
	if s.threads[thread.ID] == nil {
		s.order = append(s.order, thread.ID)
	}
	s.threads[thread.ID] = &thread
	if s.threadComments[thread.ID] == nil {
		s.threadComments[thread.ID] = map[string]struct{}{}
	}
	return nil
}

// applyCommentsUpserted projects comment upserts, deletes, and restores into
// the read projection and keeps the threadComments index consistent.
func (s *Service) applyCommentsUpserted(event ledgerEvent) error {
	// Legacy ledgers embedded the thread in the comments_upserted event. Seed it
	// first so comments can reference it; new ledgers emit a separate thread_created.
	if event.Thread != nil {
		if err := s.applyThreadCreated(ledgerEvent{Type: eventThreadCreated, CreatedAt: event.CreatedAt, Thread: event.Thread}); err != nil {
			return err
		}
	}
	if len(event.Comments) == 0 {
		return newError(ErrorCodeConflict, "document review ledger event has no comments", nil)
	}
	for _, input := range event.Comments {
		comment := input
		thread := s.threads[comment.ThreadID]
		if thread == nil {
			return newError(ErrorCodeConflict, "document review comment references a missing thread", map[string]any{"thread_id": comment.ThreadID})
		}
		s.comments[comment.ID] = &comment
		if comment.UpdatedAt.After(thread.UpdatedAt) {
			thread.UpdatedAt = comment.UpdatedAt
		}
		if s.threadComments[comment.ThreadID] == nil {
			s.threadComments[comment.ThreadID] = map[string]struct{}{}
		}
		if comment.Deleted {
			delete(s.threadComments[comment.ThreadID], comment.ID)
		} else {
			s.threadComments[comment.ThreadID][comment.ID] = struct{}{}
		}
	}
	return nil
}

func (s *Service) currentThreadLocked() Thread {
	for index := len(s.order) - 1; index >= 0; index-- {
		threadID := s.order[index]
		thread := s.threads[threadID]
		if thread == nil {
			continue
		}
		commentIDs := s.threadComments[threadID]
		if len(commentIDs) == 0 {
			continue
		}
		comments := make([]Comment, 0, len(commentIDs))
		for id := range commentIDs {
			if comment := s.comments[id]; comment != nil && !comment.Deleted {
				comments = append(comments, *comment)
			}
		}
		if len(comments) == 0 {
			continue
		}
		sort.SliceStable(comments, func(left, right int) bool {
			if comments[left].CreatedAt.Equal(comments[right].CreatedAt) {
				return comments[left].ID < comments[right].ID
			}
			return comments[left].CreatedAt.Before(comments[right].CreatedAt)
		})
		result := cloneThread(*thread)
		result.Comments = comments
		return result
	}
	return Thread{Comments: []Comment{}}
}

func cloneThread(input Thread) Thread {
	input.Comments = append([]Comment{}, input.Comments...)
	return input
}

func cloneThreadPtr(input Thread) *Thread {
	cloned := cloneThread(input)
	return &cloned
}

func validateBody(value string) (string, error) {
	body := strings.TrimSpace(value)
	if body == "" {
		return "", newError(ErrorCodeInvalid, "document comment body is empty", nil)
	}
	if len(body) > maxCommentBodyBytes {
		return "", newError(ErrorCodeInvalid, "document comment body is too large", map[string]any{"max_bytes": maxCommentBodyBytes})
	}
	return body, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func newID(prefix string) string {
	var random [12]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "-" + hex.EncodeToString(random[:])
}
