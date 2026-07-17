package documentreview

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxCommentBodyBytes = 64 * 1024
	maxPathBytes        = 16 * 1024
)

var workspaceServices = struct {
	sync.Mutex
	items map[string]*Service
}{items: map[string]*Service{}}

// Service owns one workspace's durable author-created document comments.
// Content mutations remain owned by workspacechange; this service records only
// review metadata and never rewrites manuscript files.
type Service struct {
	workspace string
	store     *eventStore

	mu       sync.RWMutex
	threads  map[string]*Thread
	comments map[string]*Comment
	order    []string
}

func ForWorkspace(workspace string) (*Service, error) {
	canonical, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	workspaceServices.Lock()
	defer workspaceServices.Unlock()
	if existing := workspaceServices.items[canonical]; existing != nil {
		return existing, nil
	}
	service, err := newService(canonical)
	if err != nil {
		return nil, err
	}
	workspaceServices.items[canonical] = service
	return service, nil
}

// NewService creates an isolated service for tests.
func NewService(workspace string) (*Service, error) {
	canonical, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	return newService(canonical)
}

func newService(workspace string) (*Service, error) {
	store, err := newEventStore(workspace)
	if err != nil {
		return nil, err
	}
	service := &Service{
		workspace: workspace,
		store:     store,
		threads:   map[string]*Thread{},
		comments:  map[string]*Comment{},
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

func (s *Service) Workspace() string {
	if s == nil {
		return ""
	}
	return s.workspace
}

func (s *Service) CurrentThread(ctx context.Context) (Thread, error) {
	if s == nil {
		return Thread{}, newError(ErrorCodeConflict, "document review service is nil", nil)
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
		return Thread{}, Comment{}, newError(ErrorCodeConflict, "document review service is nil", nil)
	}
	path := strings.TrimSpace(req.Path)
	body, err := validateBody(req.Body)
	if err != nil {
		return Thread{}, Comment{}, err
	}
	if path == "" || len(path) > maxPathBytes {
		return Thread{}, Comment{}, newError(ErrorCodeInvalid, "document comment path is invalid", nil)
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
	var createdThread *Thread
	if thread.ID == "" {
		thread = Thread{ID: newID("review-thread"), CreatedAt: now, UpdatedAt: now, Comments: []Comment{}}
		createdThread = &thread
	}
	comment := Comment{
		ID:        newID("document-comment"),
		ThreadID:  thread.ID,
		Path:      filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))),
		Body:      body,
		Anchor:    req.Anchor,
		CreatedAt: now,
		UpdatedAt: now,
	}
	event := ledgerEvent{Type: eventCommentsUpserted, CreatedAt: now, Thread: createdThread, Comments: []Comment{comment}}
	if err := s.appendAndApply(event); err != nil {
		return Thread{}, Comment{}, err
	}
	log.Printf("[document-review] comment created workspace=%q path=%q thread_id=%s comment_id=%s", s.workspace, comment.Path, comment.ThreadID, comment.ID)
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
	log.Printf("[document-review] comment updated workspace=%q path=%q thread_id=%s comment_id=%s", s.workspace, next.Path, next.ThreadID, next.ID)
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
	log.Printf("[document-review] comment deleted workspace=%q path=%q thread_id=%s comment_id=%s", s.workspace, next.Path, next.ThreadID, next.ID)
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
	log.Printf("[document-review] feedback consumed workspace=%q thread_id=%s comment_count=%d", s.workspace, threadID, len(consumed))
	return append([]Comment{}, consumed...), nil
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
	if event.Type != eventCommentsUpserted {
		return errors.New("unknown document review ledger event type: " + event.Type)
	}
	if event.Thread != nil {
		thread := cloneThread(*event.Thread)
		if s.threads[thread.ID] == nil {
			s.order = append(s.order, thread.ID)
		}
		s.threads[thread.ID] = &thread
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
	}
	return nil
}

func (s *Service) currentThreadLocked() Thread {
	for index := len(s.order) - 1; index >= 0; index-- {
		thread := s.threads[s.order[index]]
		if thread == nil {
			continue
		}
		comments := make([]Comment, 0)
		for _, comment := range s.comments {
			if comment.ThreadID == thread.ID && !comment.Deleted {
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
