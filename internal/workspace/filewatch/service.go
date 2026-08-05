package filewatch

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
)

const eventSource = "watcher"

// Service owns one on-demand recursive watcher per stable Project identity.
// Browser tabs share the watcher for a Project, while unrelated Projects can
// stream changes concurrently without following the foreground Book.
type Service struct {
	mu       sync.Mutex
	sequence uint64
	projects map[string]*projectWatch
	closed   bool
}

type projectWatch struct {
	projectID   string
	workspace   string
	generation  uint64
	watcher     *workspaceWatcher
	forwarding  bool
	subscribers map[chan Event]struct{}
}

func NewService() *Service {
	return &Service{projects: make(map[string]*projectWatch)}
}

// Subscribe starts a Project watcher on first use and returns an idempotent
// release function. Every subscriber receives an initial resync marker, so no
// durable replay protocol is needed.
func (s *Service) Subscribe(projectID, workspace string) (<-chan Event, func(), error) {
	channel := make(chan Event, 8)
	if s == nil {
		close(channel)
		return channel, func() {}, nil
	}
	projectID = strings.TrimSpace(projectID)
	workspace = strings.TrimSpace(workspace)
	if projectID == "" {
		close(channel)
		return channel, func() {}, fmt.Errorf("project ID is required")
	}
	canonical, err := filepath.Abs(workspace)
	if err != nil {
		close(channel)
		return channel, func() {}, err
	}
	canonical = filepath.Clean(canonical)

	var watcherToClose *workspaceWatcher
	s.mu.Lock()
	if s.closed {
		close(channel)
		s.mu.Unlock()
		return channel, func() {}, nil
	}
	entry := s.projects[projectID]
	if entry != nil && entry.workspace != canonical {
		// Relink normally closes the old entry explicitly. Replacing it here is
		// a final identity guard for callers that raced with catalog refresh.
		watcherToClose = entry.watcher
		for subscriber := range entry.subscribers {
			close(subscriber)
		}
		delete(s.projects, projectID)
		entry = nil
	}
	if entry == nil {
		s.sequence++
		entry = &projectWatch{
			projectID:   projectID,
			workspace:   canonical,
			generation:  s.sequence,
			subscribers: make(map[chan Event]struct{}),
		}
		s.projects[projectID] = entry
	}
	watchErr := error(nil)
	if entry.watcher == nil {
		entry.watcher, watchErr = newWorkspaceWatcher(canonical)
	}
	entry.subscribers[channel] = struct{}{}
	channel <- projectResyncEvent(entry)
	watcher := entry.watcher
	generation := entry.generation
	shouldForward := watcher != nil && !entry.forwarding
	if shouldForward {
		entry.forwarding = true
	}
	s.mu.Unlock()

	if watcherToClose != nil {
		if closeErr := watcherToClose.Close(); closeErr != nil {
			slog.ErrorContext(context.Background(), "[filewatch] close relinked Project watcher failed",
				"project_id", projectID, "workspace", canonical, "error", closeErr)
		}
	}
	if shouldForward {
		go s.forwardSafely(projectID, generation, canonical, watcher)
	}

	var once sync.Once
	release := func() {
		once.Do(func() { s.release(projectID, generation, channel) })
	}
	if watchErr != nil {
		return channel, release, fmt.Errorf("watch Project %s directory %q: %w", projectID, canonical, watchErr)
	}
	return channel, release, nil
}

// CloseProject terminates one Project generation. Relink/archive uses it after
// lifecycle admission is fenced; connected clients then reconnect by stable ID.
func (s *Service) CloseProject(projectID string) {
	if s == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	s.mu.Lock()
	entry := s.projects[projectID]
	delete(s.projects, projectID)
	if entry != nil {
		for subscriber := range entry.subscribers {
			close(subscriber)
			delete(entry.subscribers, subscriber)
		}
	}
	s.mu.Unlock()
	closeProjectWatcher(entry)
}

func (s *Service) release(projectID string, generation uint64, channel chan Event) {
	s.mu.Lock()
	entry := s.projects[projectID]
	if entry == nil || entry.generation != generation {
		s.mu.Unlock()
		return
	}
	if _, exists := entry.subscribers[channel]; exists {
		delete(entry.subscribers, channel)
		close(channel)
	}
	if len(entry.subscribers) > 0 {
		s.mu.Unlock()
		return
	}
	delete(s.projects, projectID)
	s.mu.Unlock()
	closeProjectWatcher(entry)
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	entries := make([]*projectWatch, 0, len(s.projects))
	for projectID, entry := range s.projects {
		for subscriber := range entry.subscribers {
			close(subscriber)
			delete(entry.subscribers, subscriber)
		}
		entries = append(entries, entry)
		delete(s.projects, projectID)
	}
	s.mu.Unlock()
	for _, entry := range entries {
		closeProjectWatcher(entry)
	}
}

func closeProjectWatcher(entry *projectWatch) {
	if entry == nil || entry.watcher == nil {
		return
	}
	if err := entry.watcher.Close(); err != nil {
		slog.ErrorContext(context.Background(), "[filewatch] close Project watcher failed",
			"project_id", entry.projectID, "workspace", entry.workspace, "error", err)
	}
}

func (s *Service) forwardSafely(projectID string, generation uint64, workspace string, watcher *workspaceWatcher) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(context.Background(), "[filewatch] Project event forwarding panic recovered",
				"project_id", projectID, "workspace", workspace, "error", recovered)
			s.publish(projectID, generation, Event{ProjectID: projectID, Workspace: workspace, Source: eventSource, Resync: true})
		}
	}()
	for next := range watcher.Events() {
		paths := make([]string, 0, len(next.changes))
		for _, change := range next.changes {
			paths = append(paths, change.Path)
		}
		s.publish(projectID, generation, Event{
			ProjectID: projectID,
			Workspace: workspace,
			Source:    eventSource,
			Changes:   next.changes,
			Paths:     paths,
			Resync:    next.resync,
		})
	}
	s.publish(projectID, generation, Event{ProjectID: projectID, Workspace: workspace, Source: eventSource, Resync: true})
}

func (s *Service) publish(projectID string, generation uint64, event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.projects[projectID]
	if s.closed || entry == nil || entry.generation != generation {
		return
	}
	broadcastProjectEvent(entry, event)
}

func broadcastProjectEvent(entry *projectWatch, event Event) {
	for subscriber := range entry.subscribers {
		select {
		case subscriber <- event:
		default:
			// Events are invalidation hints. Replace a lagging suffix with one
			// canonical resync instead of retaining an unbounded queue.
		drain:
			for {
				select {
				case <-subscriber:
					continue
				default:
					break drain
				}
			}
			select {
			case subscriber <- projectResyncEvent(entry):
			default:
			}
		}
	}
}

func projectResyncEvent(entry *projectWatch) Event {
	return Event{ProjectID: entry.projectID, Workspace: entry.workspace, Source: eventSource, Resync: true}
}
