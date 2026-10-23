package analytics

import (
	"bytes"
	"context"
	"denova/internal/localfs"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

const WebsiteID = "23394fdd-4d21-4dd0-b833-16bfedff38d8"
const cloudEndpoint = "https://gateway.umami.is/api/send"

// Options belongs to the application composition root. Transport and state
// paths are dependency injection boundaries, never editable Project settings.
type Options struct {
	Version     string
	Development bool
	StatePath   string
	Endpoint    string
	Client      *http.Client
}

type queuedEvent struct {
	event      event
	generation uint64
}

type Service struct {
	mu              sync.Mutex
	state           state
	path            string
	version         string
	available       bool
	closed          bool
	generation      uint64
	sendContext     context.Context
	cancelSends     context.CancelFunc
	stop            context.Context
	cancelStop      context.CancelFunc
	queue           chan queuedEvent
	done            chan struct{}
	client          *http.Client
	endpoint        string
	releaseLease    func() error
	preferenceDirty bool
}

func Open(options Options) (*Service, error) {
	// Source builds and all isolated test servers stay off the network and do
	// not read or mutate the real user's host analytics preferences.
	available := !options.Development && regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.-]+)?$`).MatchString(options.Version)
	s := &Service{version: options.Version, available: available, path: options.StatePath,
		state: state{Format: 1, Enabled: true, NoticeSeen: !available},
		queue: make(chan queuedEvent, 128), done: make(chan struct{}),
		client: options.Client, endpoint: options.Endpoint}
	if available {
		if s.path == "" {
			var err error
			s.path, err = hostStatePath()
			if err != nil {
				return nil, err
			}
		}
		var err error
		// One host user can open several data roots. A short, process-lifetime
		// lease keeps their shared identity/counters from racing or duplicating.
		leaseCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		s.releaseLease, err = localfs.AcquireLease(leaseCtx, s.path+".lock")
		cancel()
		if err != nil {
			return nil, fmt.Errorf("acquire host analytics lease: %w", err)
		}
		s.state, err = loadState(s.path)
		if err != nil {
			_ = s.releaseLease()
			return nil, fmt.Errorf("load analytics preferences: %w", err)
		}
	}
	if s.client == nil {
		s.client = &http.Client{Timeout: 5 * time.Second}
	}
	if s.endpoint == "" {
		s.endpoint = cloudEndpoint
	}
	s.stop, s.cancelStop = context.WithCancel(context.Background())
	s.sendContext, s.cancelSends = context.WithCancel(s.stop)
	go s.work()
	return s, nil
}

func (s *Service) Preferences() Preferences {
	if s == nil {
		return Preferences{NoticeSeen: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return Preferences{Enabled: s.state.Enabled, NoticeSeen: s.state.NoticeSeen, Available: s.available && !s.closed}
}

// SetEnabled also acknowledges the first-run notice. The default is enabled,
// but no event is admitted until that notice has been presented and dismissed.
func (s *Service) SetEnabled(enabled bool) (Preferences, error) {
	if s == nil {
		return Preferences{NoticeSeen: true}, fmt.Errorf("analytics is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Preferences{}, fmt.Errorf("analytics is closed")
	}
	next := s.state
	if next.Enabled == enabled && next.NoticeSeen && !s.preferenceDirty {
		return Preferences{Enabled: enabled, NoticeSeen: true, Available: s.available}, nil
	}
	next.Enabled, next.NoticeSeen = enabled, true
	if enabled && next.ID == "" && s.available {
		next.ID = "app_" + uuid.NewString()
	}
	s.generation++
	s.cancelSends()
	s.sendContext, s.cancelSends = context.WithCancel(s.stop)
drain:
	for {
		select {
		case <-s.queue:
		default:
			break drain
		}
	}
	if s.available {
		if err := saveState(s.path, next); err != nil {
			// Fence locally even when the host preference cannot be persisted.
			// Never keep sending after a failed disable request or block creation
			// behind an optional notice. A subsequent preference save can retry.
			s.state.Enabled, s.state.NoticeSeen = false, true
			s.preferenceDirty = true
			return Preferences{NoticeSeen: true, Available: s.available}, err
		}
	}
	s.state = next
	s.preferenceDirty = false
	slog.Info("analytics_preferences_updated", "enabled", enabled, "available", s.available)
	return Preferences{Enabled: enabled, NoticeSeen: true, Available: s.available}, nil
}

func (s *Service) collectingLocked() bool {
	return s.available && !s.closed && s.state.Enabled && s.state.NoticeSeen && s.state.ID != ""
}

// Activity counts a user-initiated operation or a focused reading milestone.
// The caller supplies a category only; browser tabs share this host-wide lane.
func (s *Service) Activity(feature Feature) {
	if s == nil || !feature.Valid() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() {
		return
	}
	s.activityLocked(feature, time.Now().UTC())
}

func (s *Service) activityLocked(feature Feature, now time.Time) {
	next := s.state
	next.Features = slices.Clone(next.Features)
	next.Retention = slices.Clone(next.Retention)
	day := now.Format("2006-01-02")
	year, weekNumber := now.ISOWeek()
	week := fmt.Sprintf("%04d-W%02d", year, weekNumber)
	month := now.Format("2006-01")
	var events []event
	add := func(name string, data eventData) { events = append(events, s.newEvent(name, feature, data, now)) }
	if next.FirstActive == "" {
		next.FirstActive = day
		add("app.first_active", eventData{CohortDate: day})
	}
	if next.Day != day {
		next.Day, next.Features = day, nil
		add("app.active_day", eventData{})
	}
	if next.Week != week {
		next.Week = week
		add("app.active_week", eventData{})
	}
	if next.Month != month {
		next.Month = month
		add("app.active_month", eventData{})
	}
	if !slices.Contains(next.Features, feature) {
		next.Features = append(next.Features, feature)
		add("app.feature_active", eventData{Feature: feature})
	}
	first, err := time.Parse("2006-01-02", next.FirstActive)
	if err == nil {
		elapsedDays := int(now.Sub(first) / (24 * time.Hour))
		if slices.Contains([]int{1, 7, 30}, elapsedDays) && !slices.Contains(next.Retention, elapsedDays) {
			next.Retention = append(next.Retention, elapsedDays)
			add(fmt.Sprintf("app.retained_d%d", elapsedDays), eventData{CohortDate: next.FirstActive})
		}
	}
	if len(events) == 0 {
		return
	}
	// Persist only bounded counters before attempting delivery. We deliberately
	// prefer a missing sample over duplicate counts after an ambiguous response.
	if err := saveState(s.path, next); err != nil {
		slog.Warn("analytics_state_save_failed", "error", err)
		return
	}
	s.state = next
	for _, e := range events {
		s.enqueueLocked(e)
	}
}

func (s *Service) BeginRun(feature Feature, engine Engine, trigger Trigger) Run {
	if s == nil || !feature.Valid() {
		return Run{}
	}
	switch engine {
	case Native, Codex, Claude, Grok, Antigravity:
	default:
		return Run{}
	}
	switch trigger {
	case User, Scheduled:
	default:
		return Run{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() {
		return Run{}
	}
	now := time.Now().UTC()
	if trigger == User {
		s.activityLocked(feature, now)
	}
	return Run{service: s, generation: s.generation, feature: feature, engine: engine, trigger: trigger, started: now, once: &sync.Once{}}
}

func (s *Service) recordRun(r Run, result Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() || r.generation != s.generation {
		return
	}
	now := time.Now().UTC()
	durationMS := now.Sub(r.started).Milliseconds()
	s.enqueueLocked(s.newEvent("app.run_finished", r.feature, eventData{
		Feature: r.feature, Runtime: r.engine, Trigger: r.trigger, Outcome: result,
		DurationMS: &durationMS,
	}, now))
}

func (s *Service) ActionCompleted(action Action) {
	if s == nil {
		return
	}
	var feature Feature
	switch action {
	case BookCreated:
		feature = Projects
	case ContentExported:
		feature = Writing
	case VersionRestored:
		feature = Versions
	default:
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() {
		return
	}
	now := time.Now().UTC()
	s.activityLocked(feature, now)
	s.enqueueLocked(s.newEvent("app.action_completed", feature, eventData{Action: action}, now))
}

func (s *Service) newEvent(name string, feature Feature, data eventData, now time.Time) event {
	data.Version, data.Platform = s.version, runtime.GOOS
	return event{Type: "event", Payload: payload{Website: WebsiteID, Hostname: "app.denova-ai.com", Tag: "app",
		ID: s.state.ID, URL: "/app/" + string(feature), Name: name, Timestamp: now.Unix(), Data: data}}
}

func (s *Service) enqueueLocked(e event) {
	select {
	case s.queue <- queuedEvent{event: e, generation: s.generation}:
	default:
		slog.Warn("analytics_queue_full", "event", e.Payload.Name)
	}
}

func (s *Service) work() {
	defer close(s.done)
	defer func() {
		if recovered := recover(); recovered != nil {
			s.mu.Lock()
			s.available = false
			s.cancelSends()
			s.mu.Unlock()
			slog.Error("analytics_worker_panicked", "panic", recovered)
		}
	}()
	for {
		select {
		case <-s.stop.Done():
			return
		case item := <-s.queue:
			s.send(item)
		}
	}
}

func (s *Service) send(item queuedEvent) {
	s.mu.Lock()
	if !s.collectingLocked() || item.generation != s.generation {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(s.sendContext, 5*time.Second)
	s.mu.Unlock()
	defer cancel()
	data, err := json.Marshal(item.event)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// Fixed across releases: analytics session identity must not change with the
	// app version or inherit identifying browser/device environment strings.
	// Umami Cloud bot filtering requires a Mozilla-compatible user agent. This
	// fixed value was verified against the real gateway; it carries no OS data.
	req.Header.Set("User-Agent", "Mozilla/5.0 Denova/1.0")
	response, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("analytics_delivery_failed", "event", item.event.Payload.Name, "error", err)
		}
		return
	}
	defer response.Body.Close()
	var receipt struct {
		Cache     string `json:"cache"`
		SessionID string `json:"sessionId"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&receipt)
	if response.StatusCode < 200 || response.StatusCode >= 300 || decodeErr != nil || receipt.Cache == "" || receipt.SessionID == "" {
		slog.Warn("analytics_delivery_rejected", "event", item.event.Payload.Name, "status", response.StatusCode)
	}
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cancelSends()
		s.cancelStop()
	}
	s.mu.Unlock()
	select {
	case <-s.done:
	case <-time.After(time.Second):
		slog.Warn("analytics_close_timed_out")
	}
	if s.releaseLease != nil {
		if err := s.releaseLease(); err != nil {
			slog.Warn("analytics_lease_release_failed", "error", err)
		}
	}
}
