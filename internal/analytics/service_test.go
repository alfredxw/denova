package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alfredxw/denova/agent/model/providers"
	"github.com/google/uuid"
)

type collector struct {
	mu     sync.Mutex
	events []event
	bodies []string
}

func openCollector(t *testing.T, path string) (*Service, *collector) {
	t.Helper()
	c := &collector{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing JSON content type")
		}
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Error(err)
		}
		var e event
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Error(err)
		}
		c.mu.Lock()
		c.events = append(c.events, e)
		c.bodies = append(c.bodies, string(raw))
		c.mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"Ok"}`))
	}))
	t.Cleanup(server.Close)
	s, err := Open(Options{Version: "0.6.3", StatePath: path, Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, c
}
func (c *collector) snapshot() []event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.events)
}
func awaitEvents(t *testing.T, c *collector, count int) []event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if events := c.snapshot(); len(events) >= count {
			return events
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected %d events, received %d", count, len(c.snapshot()))
	return nil
}
func eventsNamed(events []event, name string) []event {
	var found []event
	for _, e := range events {
		if e.Event == name {
			found = append(found, e)
		}
	}
	return found
}

func TestPostHogConsentProtocolAndActivityCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.json")
	s, c := openCollector(t, path)
	s.Opened()
	s.Activity(Writing)
	if len(c.snapshot()) != 0 || len(s.queue) != 0 {
		t.Fatal("collected before acknowledgement")
	}
	if _, err := s.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	s.Opened()
	s.Opened()
	s.Activity(Writing)
	s.Activity(Writing)
	s.Activity(Game)
	events := awaitEvents(t, c, 9)
	for name, count := range map[string]int{"app_first_seen": 1, "app_opened": 1, "app_first_active": 1, "app_active_day": 1, "feature_active": 2, "feature_used": 3} {
		if got := len(eventsNamed(events, name)); got != count {
			t.Fatalf("%s count=%d; events=%+v", name, got, events)
		}
	}
	for _, e := range events {
		if e.APIKey != projectToken || !strings.HasPrefix(e.DistinctID, "app_") || e.Timestamp.IsZero() {
			t.Fatalf("invalid capture: %+v", e)
		}
		if e.Properties.AppVersion != "0.6.3" || e.Properties.OS != runtime.GOOS || e.Properties.Source != "app" || e.Properties.SchemaVersion != 2 {
			t.Fatalf("invalid properties: %+v", e.Properties)
		}
		var raw map[string]any
		body, _ := json.Marshal(e)
		_ = json.Unmarshal(body, &raw)
		if len(raw) != 5 {
			t.Fatalf("unexpected envelope: %s", body)
		}
	}
	id := s.state.ID
	s.Close()
	restored, c2 := openCollector(t, path)
	restored.Opened()
	restored.Activity(Writing)
	restarted := awaitEvents(t, c2, 2)
	if restored.state.ID != id || len(eventsNamed(restarted, "app_first_seen")) != 0 || len(eventsNamed(restarted, "feature_active")) != 0 || len(eventsNamed(restarted, "app_active_day")) != 0 {
		t.Fatalf("restart changed identity or duplicated daily counters: %+v", restarted)
	}
}

func TestMigrationPreservesIdentityConsentAndCohort(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "analytics.json")
			identity := "app_" + uuid.NewString()
			first := "2026-10-18"
			legacy := state{Format: 1, ID: identity, Enabled: enabled, NoticeSeen: true, FirstActive: first, Day: time.Now().UTC().Format("2006-01-02"), Features: []Feature{Writing}, Week: "2026-W43", Month: "2026-10", Retention: []int{1}}
			if err := saveState(path, legacy); err != nil {
				t.Fatal(err)
			}
			s, c := openCollector(t, path)
			s.Activity(Writing)
			if !enabled {
				if len(s.queue) != 0 || len(c.snapshot()) != 0 {
					t.Fatal("disabled migration collected")
				}
				return
			}
			if s.Preferences().NoticeSeen || len(s.queue) != 0 || len(c.snapshot()) != 0 {
				t.Fatal("migration sent before acknowledging the new collector")
			}
			if _, err := s.SetEnabled(true); err != nil {
				t.Fatal(err)
			}
			s.Activity(Writing)
			events := awaitEvents(t, c, 4)
			for _, e := range events {
				if e.DistinctID != identity || e.Properties.CohortDate != first || e.Properties.CohortOrigin != "migrated" || e.Properties.FirstAppVersion != "unknown" {
					t.Fatalf("migration changed cohort: %+v", e)
				}
			}
			if len(eventsNamed(events, "app_first_active")) != 0 {
				t.Fatal("existing instance counted as new")
			}
		})
	}
}

func TestUTCDailyActivityAndCoreValueHaveIndependentDefinitions(t *testing.T) {
	s, c := openCollector(t, filepath.Join(t.TempDir(), "analytics.json"))
	_, _ = s.SetEnabled(true)
	first := time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC)
	s.mu.Lock()
	for _, day := range []int{0, 0, 1, 7, 30} {
		s.trackUseLocked(Writing, eventData{Operation: "read", Trigger: User}, first.AddDate(0, 0, day))
	}
	s.mu.Unlock()
	events := awaitEvents(t, c, 15)
	if len(eventsNamed(events, "app_active_day")) != 4 || len(eventsNamed(events, "feature_used")) != 5 || len(eventsNamed(events, "app_activated")) != 0 {
		t.Fatalf("invalid activity counts: %+v", events)
	}
	s.ContentEdited()
	s.ContentEdited()
	events = awaitEvents(t, c, 21)
	if len(eventsNamed(events, "app_activated")) != 1 || len(eventsNamed(events, "core_active_day")) != 1 {
		t.Fatal("edits did not produce deduplicated value metrics")
	}
}

func TestRunStartAndTerminalEventsPairAndDoNotUploadErrorText(t *testing.T) {
	s, c := openCollector(t, filepath.Join(t.TempDir(), "analytics.json"))
	_, _ = s.SetEnabled(true)
	run := s.BeginRun(Writing, Native, User)
	run.Fail(fmt.Errorf("wrapped: %w", &providers.APIError{StatusCode: 429, Message: "private manuscript secret-api-key https://private.example"}))
	run.Finish(Success)
	events := awaitEvents(t, c, 6)
	starts := eventsNamed(events, "feature_used")
	ends := eventsNamed(events, "feature_failed")
	if len(starts) != 1 || len(ends) != 1 || starts[0].Properties.RunID == "" || starts[0].Properties.RunID != ends[0].Properties.RunID {
		t.Fatalf("unpaired execution: %+v", events)
	}
	if ends[0].Properties.ErrorCode != ErrorRateLimit || ends[0].Properties.DurationMS == nil {
		t.Fatal("missing failure classification/duration")
	}
	for _, e := range events {
		body, _ := json.Marshal(e)
		if strings.Contains(string(body), "private") || strings.Contains(string(body), "secret-api-key") {
			t.Fatal("raw error crossed exporter")
		}
	}
	if len(eventsNamed(events, "app_activated")) != 0 {
		t.Fatal("failed run counted as activation")
	}
}

func TestAutomationsAndInterruptedRunsDoNotInflateActiveOrSuccess(t *testing.T) {
	s, c := openCollector(t, filepath.Join(t.TempDir(), "analytics.json"))
	_, _ = s.SetEnabled(true)
	scheduled := s.BeginRun(Automation, Codex, Scheduled)
	scheduled.started = time.Now().Add(-25 * time.Hour)
	scheduled.Finish(Success)
	events := awaitEvents(t, c, 2)
	if events[1].Properties.DurationMS == nil || *events[1].Properties.DurationMS < 25*time.Hour.Milliseconds() || s.state.FirstActive != "" || s.state.PostHogSeen != "" {
		t.Fatalf("automation inflated activity: %+v", events)
	}
	for _, result := range []Result{Cancelled, Suspended, Preempted} {
		s.BeginRun(Game, Claude, User).Finish(result)
	}
	events = awaitEvents(t, c, 12)
	if len(eventsNamed(events, "feature_ended")) != 3 || len(eventsNamed(events, "feature_failed")) != 0 || len(eventsNamed(events, "app_activated")) != 0 {
		t.Fatalf("interruption counted as success/failure: %+v", events)
	}
}

func TestSuccessfulRunsActivateEachFeatureOnce(t *testing.T) {
	s, c := openCollector(t, filepath.Join(t.TempDir(), "analytics.json"))
	_, _ = s.SetEnabled(true)
	for _, feature := range []Feature{Writing, Writing, Game} {
		s.BeginRun(feature, Native, User).Finish(Success)
	}
	events := awaitEvents(t, c, 15)
	if len(eventsNamed(events, "feature_completed")) != 3 || len(eventsNamed(events, "app_activated")) != 2 || len(eventsNamed(events, "core_active_day")) != 2 {
		t.Fatalf("invalid successful work metrics: %+v", events)
	}
	var value eventData
	if err := json.Unmarshal([]byte(`{"duration_ms":0}`), &value); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(value)
	if !strings.Contains(string(raw), `"duration_ms":0`) {
		t.Fatal("zero duration omitted")
	}
}

func TestSettingsAndClientErrorsHaveClosedPayloads(t *testing.T) {
	s, c := openCollector(t, filepath.Join(t.TempDir(), "analytics.json"))
	_, _ = s.SetEnabled(true)
	s.SettingChanged("api_key", "private")
	s.SettingChanged("theme", "private")
	s.ClientError("private")
	s.APIError(Game, "private route", ErrorHTTP, 503)
	s.APIError(Game, "read", "private error", 503)
	if len(s.queue) != 0 || len(c.snapshot()) != 0 {
		t.Fatal("unapproved properties admitted")
	}
	s.SettingChanged("theme", "dark")
	s.ClientError(ErrorRender)
	s.APIError(Game, "read", ErrorHTTP, 503)
	s.RecoveredPanic()
	events := awaitEvents(t, c, 4)
	want := eventData{AppVersion: "0.6.3", OS: runtime.GOOS, Source: "app", SchemaVersion: 2, SettingName: "theme", SettingValue: "dark"}
	if !reflect.DeepEqual(events[0].Properties, want) {
		t.Fatalf("settings leaked data: %+v", events[0])
	}
	if events[1].Properties.ErrorSource != Frontend || events[2].Properties.StatusCode != 503 || events[2].Properties.Operation != "read" || events[3].Properties.ErrorCode != ErrorPanic {
		t.Fatalf("missing error context: %+v", events)
	}
	if s.state.FirstActive != "" {
		t.Fatal("diagnostics inflated activity")
	}
}

func TestErrorClassificationUsesStructuredErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want ErrorCode
	}{
		{context.DeadlineExceeded, ErrorTimeout}, {context.Canceled, ErrorCancelled}, {os.ErrPermission, ErrorPermission}, {os.ErrNotExist, ErrorNotFound},
		{&providers.APIError{StatusCode: 401}, ErrorAuthentication}, {&providers.APIError{StatusCode: 503}, ErrorProvider},
		{&providers.APIError{StatusCode: 400}, ErrorInvalidRequest}, {errors.New("401 private error"), ErrorUnknown},
	} {
		if got := classifyError(tc.err); got != tc.want {
			t.Fatalf("classification=%s, want %s", got, tc.want)
		}
	}
}

type cancelTransport struct {
	started   chan struct{}
	cancelled chan struct{}
	once      sync.Once
}

func (transport *cancelTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.once.Do(func() { close(transport.started) })
	<-request.Context().Done()
	close(transport.cancelled)
	return nil, request.Context().Err()
}

func TestDisableCancelsDeliveryAndInvalidatesOldRuns(t *testing.T) {
	transport := &cancelTransport{started: make(chan struct{}), cancelled: make(chan struct{})}
	s, err := Open(Options{Version: "0.6.0", StatePath: filepath.Join(t.TempDir(), "analytics.json"), Client: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	run := s.BeginRun(Writing, Native, User)
	select {
	case <-transport.started:
	case <-time.After(time.Second):
		t.Fatal("delivery did not start")
	}
	if _, err = s.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-transport.cancelled:
	case <-time.After(time.Second):
		t.Fatal("delivery was not cancelled")
	}
	s.Activity(Game)
	if _, err = s.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	run.Finish(Success)
	if len(s.queue) != 0 {
		t.Fatal("disabled generation was replayed")
	}
	if _, err = s.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	stored, err := loadState(s.path)
	if err != nil || stored.Enabled || !stored.NoticeSeen {
		t.Fatalf("disabled preference was not persisted: %+v %v", stored, err)
	}
}

func TestSourceBuildsAndUnreadableStateNeverCollect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.json")
	s, err := Open(Options{Version: "dev", StatePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _ = s.SetEnabled(true)
	s.Activity(Writing)
	if _, err = os.Stat(path); !os.IsNotExist(err) || s.Preferences().Available || len(s.queue) > 0 {
		t.Fatal("source build accessed host statistics")
	}
	if err = os.WriteFile(path, []byte("invalid preserved state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(Options{Version: "0.6.0", StatePath: path}); err == nil {
		t.Fatal("corrupt state was silently overwritten")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "invalid preserved state" {
		t.Fatal("corrupt state changed")
	}
}
