package filewatch

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
)

const eventSource = "watcher"

// Service owns the active workspace watcher and fans ephemeral events out to
// any number of clients. Slow clients receive a resync marker instead of an
// incomplete event suffix.
type Service struct {
	mu          sync.Mutex
	workspace   string
	watcher     *workspaceWatcher
	generation  uint64
	subscribers map[chan Event]struct{}
	closed      bool
}

func NewService() *Service {
	return &Service{subscribers: make(map[chan Event]struct{})}
}

// SetWorkspace atomically replaces the active recursive watcher. An empty path
// disables watching while keeping existing subscribers connected.
func (s *Service) SetWorkspace(workspace string) error {
	if s == nil {
		return nil
	}
	canonical := ""
	var watchErr error
	if workspace != "" {
		canonical, watchErr = filepath.Abs(workspace)
		if watchErr == nil {
			canonical = filepath.Clean(canonical)
		}
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	if watchErr == nil && canonical == s.workspace && s.watcher != nil {
		s.broadcastLocked(Event{Workspace: canonical, Source: eventSource, Resync: true})
		s.mu.Unlock()
		return nil
	}
	s.generation++
	generation := s.generation
	previous := s.watcher
	s.workspace = canonical
	s.watcher = nil
	s.mu.Unlock()

	if previous != nil {
		if err := previous.Close(); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[filewatch] close previous workspace watcher failed workspace=%q err=%v", canonical, err))
		}
	}

	var next *workspaceWatcher
	if watchErr == nil && canonical != "" {
		next, watchErr = newWorkspaceWatcher(canonical)
	}

	s.mu.Lock()
	if s.closed || generation != s.generation {
		s.mu.Unlock()
		if next != nil {
			_ = next.Close()
		}
		return nil
	}
	s.watcher = next
	s.broadcastLocked(Event{Workspace: canonical, Source: eventSource, Resync: true})
	s.mu.Unlock()
	if next != nil {
		go s.forwardSafely(generation, canonical, next)
	}

	if watchErr != nil {
		return fmt.Errorf("watch workspace %q: %w", canonical, watchErr)
	}
	return nil
}

// Subscribe returns an event channel and an idempotent release function. The
// first event is always a resync marker, so reconnection never needs replay.
func (s *Service) Subscribe() (<-chan Event, func()) {
	channel := make(chan Event, 8)
	if s == nil {
		close(channel)
		return channel, func() {}
	}
	s.mu.Lock()
	if s.closed {
		close(channel)
		s.mu.Unlock()
		return channel, func() {}
	}
	s.subscribers[channel] = struct{}{}
	channel <- Event{Workspace: s.workspace, Source: eventSource, Resync: true}
	s.mu.Unlock()

	var once sync.Once
	return channel, func() {
		once.Do(func() {
			s.mu.Lock()
			if _, exists := s.subscribers[channel]; exists {
				delete(s.subscribers, channel)
				close(channel)
			}
			s.mu.Unlock()
		})
	}
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
	s.generation++
	watcher := s.watcher
	s.watcher = nil
	for subscriber := range s.subscribers {
		close(subscriber)
		delete(s.subscribers, subscriber)
	}
	s.mu.Unlock()
	if watcher != nil {
		if err := watcher.Close(); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[filewatch] close workspace watcher failed workspace=%q err=%v", s.workspace, err))
		}
	}
}

func (s *Service) forwardSafely(generation uint64, workspace string, watcher *workspaceWatcher) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[filewatch] event forwarding panic recovered workspace=%q err=%v", workspace, recovered))
			s.publish(generation, Event{Workspace: workspace, Source: eventSource, Resync: true})
		}
	}()
	for next := range watcher.Events() {
		paths := make([]string, 0, len(next.changes))
		for _, change := range next.changes {
			paths = append(paths, change.Path)
		}
		s.publish(generation, Event{
			Workspace: workspace,
			Source:    eventSource,
			Changes:   next.changes,
			Paths:     paths,
			Resync:    next.resync,
		})
	}
	s.publish(generation, Event{Workspace: workspace, Source: eventSource, Resync: true})
}

func (s *Service) publish(generation uint64, event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || generation != s.generation {
		return
	}
	s.broadcastLocked(event)
}

func (s *Service) broadcastLocked(event Event) {
	for subscriber := range s.subscribers {
		select {
		case subscriber <- event:
		default:
			// The stream is only an invalidation hint. Replacing a lagging
			// suffix with resync is both bounded and lossless at the semantic level.
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
			case subscriber <- Event{Workspace: event.Workspace, Source: eventSource, Resync: true}:
			default:
			}
		}
	}
}
