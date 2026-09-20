package speech

import (
	"context"
	"strings"
	"sync"
	"time"

	"denova/config"
)

type request struct {
	cancel  context.CancelFunc
	expires time.Time
}

// Requests lets browsers cancel synthesis explicitly on transports that do not
// propagate disconnects into handler contexts (including native Windows).
// IDs are random per request, never persisted, and cancellation tombstones only
// bridge the race where DELETE arrives just before the corresponding POST.
type Requests struct {
	mu      sync.Mutex
	entries map[string]request
}

func validRequestID(id string) bool {
	return len(id) >= 16 && len(id) <= 80 && strings.IndexFunc(id, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) == -1
}

func (requests *Requests) prune() {
	if requests.entries == nil {
		requests.entries = make(map[string]request)
	}
	for id, entry := range requests.entries {
		if entry.cancel == nil && time.Now().After(entry.expires) {
			delete(requests.entries, id)
		}
	}
}

func (requests *Requests) Synthesize(ctx context.Context, id string, settings config.SpeechSettings, input string) ([]byte, error) {
	if !validRequestID(id) {
		return nil, InvalidInput
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	requests.mu.Lock()
	requests.prune()
	if _, exists := requests.entries[id]; exists {
		requests.mu.Unlock()
		return nil, context.Canceled
	}
	requests.entries[id] = request{cancel: cancel}
	requests.mu.Unlock()
	defer func() {
		requests.mu.Lock()
		if requests.entries[id].cancel != nil {
			delete(requests.entries, id)
		}
		requests.mu.Unlock()
	}()
	return Synthesize(ctx, settings, input)
}

func (requests *Requests) Cancel(id string) {
	if !validRequestID(id) {
		return
	}
	requests.mu.Lock()
	defer requests.mu.Unlock()
	requests.prune()
	if entry := requests.entries[id]; entry.cancel != nil {
		entry.cancel()
	}
	requests.entries[id] = request{expires: time.Now().Add(2 * time.Minute)}
}
