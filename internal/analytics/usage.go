package analytics

import (
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

// observeLocked preserves the last release's identity and first-active date.
// PostHog's first received event must not turn existing instances into new users.
func (s *Service) observeLocked(now time.Time) bool {
	if s.state.PostHogSeen != "" {
		return true
	}
	next := s.state
	next.PostHogSeen = now.Format("2006-01-02")
	next.CohortOrigin, next.FirstVersion = "new_instance", s.version
	if next.FirstActive != "" {
		next.CohortOrigin, next.FirstVersion = "migrated", "unknown"
	}
	// The previous collector may already have counted today's activity.
	next.Day, next.Features = "", nil
	if !s.persistLocked(next) {
		return false
	}
	s.enqueueLocked(s.newEvent("app_first_seen", eventData{}, now))
	return true
}

// Opened counts the first authenticated UI opening per host process. Keeping
// a server running or executing automation does not imply that somebody opened it.
func (s *Service) Opened() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() || s.opened {
		return
	}
	now := time.Now().UTC()
	if !s.observeLocked(now) {
		return
	}
	s.opened = true
	s.enqueueLocked(s.newEvent("app_opened", eventData{}, now))
}

// Activity records a focused 30-second reading milestone. It measures qualified
// observations, not clicks or time spent. Feature/day reach is reported separately.
func (s *Service) Activity(feature Feature) {
	if s == nil || !feature.Valid() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() {
		return
	}
	s.trackUseLocked(feature, eventData{Operation: "read", Trigger: User}, time.Now().UTC())
}

// ContentEdited is called only after changed manuscript content was saved.
// Imports, initialization and saves without changes must not call this method.
func (s *Service) ContentEdited() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() {
		return
	}
	now := time.Now().UTC()
	if s.trackUseLocked(Writing, eventData{Operation: "edit", Trigger: User}, now) {
		s.valueLocked(Writing, now)
	}
}

func (s *Service) trackUseLocked(feature Feature, data eventData, now time.Time) bool {
	if !s.observeLocked(now) {
		return false
	}
	next := s.state
	day := now.Format("2006-01-02")
	first := next.FirstActive == ""
	if first {
		next.FirstActive = day
	}
	newDay := next.Day != day
	if newDay {
		next.Day, next.Features = day, nil
	}
	newFeature := !slices.Contains(next.Features, feature)
	if newFeature {
		next.Features = append(slices.Clone(next.Features), feature)
	}
	if (first || newDay || newFeature) && !s.persistLocked(next) {
		return false
	}
	if first {
		s.enqueueLocked(s.newEvent("app_first_active", eventData{}, now))
	}
	if newDay {
		s.enqueueLocked(s.newEvent("app_active_day", eventData{}, now))
	}
	if newFeature {
		s.enqueueLocked(s.newEvent("feature_active", eventData{Feature: feature}, now))
	}
	data.Feature = feature
	s.enqueueLocked(s.newEvent("feature_used", data, now))
	return true
}

// valueLocked counts successful user work separately from reading, admission,
// and automation. Writing execution success is a technical value proxy, not a
// judgement of manuscript quality or confirmation that a user kept every edit.
func (s *Service) valueLocked(feature Feature, now time.Time) {
	next := s.state
	day := now.Format("2006-01-02")
	if next.CoreDay != day {
		next.CoreDay, next.CoreFeatures = day, nil
	}
	first := !slices.Contains(next.ActivatedFeatures, feature)
	newDay := !slices.Contains(next.CoreFeatures, feature)
	if !first && !newDay {
		return
	}
	if first {
		next.ActivatedFeatures = append(slices.Clone(next.ActivatedFeatures), feature)
	}
	if newDay {
		next.CoreFeatures = append(slices.Clone(next.CoreFeatures), feature)
	}
	if !s.persistLocked(next) {
		return
	}
	if first {
		s.enqueueLocked(s.newEvent("app_activated", eventData{Feature: feature}, now))
	}
	if newDay {
		s.enqueueLocked(s.newEvent("core_active_day", eventData{Feature: feature}, now))
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
	r := Run{service: s, generation: s.generation, feature: feature, engine: engine, trigger: trigger, started: now, id: uuid.NewString(), once: &sync.Once{}}
	data := eventData{Feature: feature, Operation: "run", Runtime: engine, Trigger: trigger, RunID: r.id}
	if trigger == User {
		if !s.trackUseLocked(feature, data, now) {
			return Run{}
		}
	} else {
		s.enqueueLocked(s.newEvent("feature_used", data, now))
	}
	return r
}

func (s *Service) recordRun(r Run, result Result, code ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() || r.generation != s.generation {
		return
	}
	now := time.Now().UTC()
	duration := now.Sub(r.started).Milliseconds()
	data := eventData{Feature: r.feature, Operation: "run", Runtime: r.engine, Trigger: r.trigger, Outcome: result, DurationMS: &duration, RunID: r.id}
	name := "feature_ended"
	switch result {
	case Success:
		name = "feature_completed"
	case Failure:
		name = "feature_failed"
		if code == "" {
			code = ErrorUnknown
		}
		data.ErrorCode = code
	case Cancelled, Suspended, Preempted:
	}
	s.enqueueLocked(s.newEvent(name, data, now))
	if result == Success && r.trigger == User {
		s.valueLocked(r.feature, now)
	}
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
	if !s.trackUseLocked(feature, eventData{Operation: string(action), Trigger: User}, now) {
		return
	}
	s.enqueueLocked(s.newEvent("action_completed", eventData{Feature: feature, Action: action}, now))
}

// SettingChanged accepts only harmless product preferences after a successful
// change. API endpoints, keys, model names, prompts and paths are not settings
// analytics values. Opting out is never reported after collection is stopped.
func (s *Service) SettingChanged(name, value string) {
	valid := false
	switch name {
	case "theme":
		valid = value == "dark" || value == "light" || value == "system"
	case "language":
		valid = value == "zh-CN" || value == "en-US" || value == "auto"
	}
	if s == nil || !valid {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() {
		return
	}
	s.enqueueLocked(s.newEvent("settings_changed", eventData{SettingName: name, SettingValue: value}, time.Now().UTC()))
}
