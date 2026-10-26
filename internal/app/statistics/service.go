// Package statistics owns the creator statistics read model and observed saved
// manuscript changes. It never activates or switches the foreground Project.
package statistics

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"denova/internal/app/projectbook"
	"denova/internal/book"
	"denova/internal/project"
	stats "denova/internal/statistics"
	"denova/internal/workspace/filewatch"
)

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WritingProject struct {
	Project
	Words     int64 `json:"words"`
	Chapters  int   `json:"chapters"`
	Confirmed int   `json:"confirmed"`
}

type WritingDay struct {
	Date  string `json:"date"`
	Delta *int64 `json:"delta"`
}

type UsageDay struct {
	Date string `json:"date"`
	stats.Totals
}

type Model struct {
	Name    string `json:"name"`
	Runtime string `json:"runtime"`
	stats.Totals
}

type Report struct {
	Projects []Project  `json:"projects"`
	Game     GameReport `json:"game"`
	Writing  struct {
		Projects []WritingProject `json:"projects"`
		Days     []WritingDay     `json:"days"`
	} `json:"writing"`
	Usage struct {
		Totals stats.Totals `json:"totals"`
		Days   []UsageDay   `json:"days"`
		Models []Model      `json:"models"`
	} `json:"usage"`
	Issues int `json:"issues"`
}

// Service coalesces ephemeral invalidations into saved-state observations. The
// first observation in each process/resync is a baseline, never historical output.
type Service struct {
	registry      *project.Registry
	books         *projectbook.Service
	mu            sync.Mutex
	pending       map[string]bool
	closed        bool
	wake          chan struct{}
	stop          chan struct{}
	done          chan struct{}
	drainMu       sync.Mutex
	observationMu sync.Mutex
	baselines     map[string]int64
}

type observationMode uint8

const (
	snapshotOnly observationMode = iota
	resyncBaseline
	savedChange
)

func New(registry *project.Registry, books *projectbook.Service) *Service {
	s := &Service{registry: registry, books: books, pending: make(map[string]bool), wake: make(chan struct{}, 1),
		stop: make(chan struct{}), done: make(chan struct{}), baselines: make(map[string]int64)}
	go s.run()
	return s
}

func (s *Service) Queue(event filewatch.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.pending[event.ProjectID] = s.pending[event.ProjectID] || event.Resync
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.stop)
	}
	s.mu.Unlock()
	<-s.done
}

func (s *Service) run() {
	defer close(s.done)
	defer func() {
		if value := recover(); value != nil {
			slog.Error("Statistics observer panic recovered", "error", value)
		}
	}()
	for {
		select {
		case <-s.stop:
			s.drain()
			return
		case <-s.wake:
			s.drain()
		}
	}
}

func (s *Service) drain() {
	s.drainMu.Lock()
	defer s.drainMu.Unlock()
	s.mu.Lock()
	pending := s.pending
	s.pending = make(map[string]bool)
	s.mu.Unlock()
	for id, reset := range pending {
		record, layout, err := s.registry.Resolve(id, true)
		if err != nil || record.Type != project.TypeBook {
			continue
		}
		mode := savedChange
		if reset {
			mode = resyncBaseline
		}
		if _, err := s.observe(context.Background(), record, layout, mode); err != nil {
			slog.Warn("Observe manuscript statistics failed", "project_id", id, "error", err)
		}
	}
}

func (s *Service) observe(ctx context.Context, record project.Record, layout project.Layout, mode observationMode) (book.WorkspaceSummary, error) {
	s.observationMu.Lock()
	defer s.observationMu.Unlock()
	_, summary, err := s.books.Summary(ctx, record.ID)
	if err != nil || summary.Incomplete {
		delete(s.baselines, record.ID)
		if err == nil {
			err = errors.New("incomplete manuscript scan")
		}
		return summary, err
	}
	words := int64(summary.TotalWords)
	before, known := s.baselines[record.ID]
	// A dashboard read can race a pending watcher event. It may show current
	// stock, but must not consume the event's before-state and lose its delta.
	if known && mode == snapshotOnly {
		return summary, nil
	}
	if known && before == words {
		return summary, nil
	}
	observation := stats.Writing{At: time.Now().UTC(), Words: words}
	if known && mode == savedChange {
		delta := words - before
		observation.Delta = &delta
	}
	if err := stats.AppendWriting(layout.StoreRoot, observation); err != nil {
		return summary, err
	}
	s.baselines[record.ID] = words
	return summary, nil
}

// Query uses the viewer's timezone to bucket one bounded date range, including
// historical daylight-saving boundaries. Persisted observations stay in UTC.
// Historical gaps stay null. A selected Project never changes app navigation.
func (s *Service) Query(ctx context.Context, projectID string, days int, location *time.Location, now time.Time) (Report, error) {
	// Join already-observed invalidations, including the subscription baseline,
	// before serving a chart. This also preserves ordering with the worker.
	s.drain()
	local := now.In(location)
	until := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location)
	from := until.AddDate(0, 0, -days)
	report := Report{Projects: []Project{}}
	report.Game.Stories = []GameStory{}
	report.Game.Days = make([]GameDay, days)
	report.Writing.Projects = []WritingProject{}
	report.Writing.Days = make([]WritingDay, days)
	report.Usage.Days = make([]UsageDay, days)
	report.Usage.Models = []Model{}
	dayIndex := make(map[string]int, days)
	for i := range days {
		date := from.AddDate(0, 0, i).Format(time.DateOnly)
		dayIndex[date] = i
		report.Writing.Days[i].Date, report.Usage.Days[i].Date = date, date
		report.Game.Days[i].Date = date
	}
	records, err := s.registry.List(true)
	if err != nil {
		return report, err
	}
	models := make(map[string]*Model)
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		report.Projects = append(report.Projects, Project{ID: record.ID, Name: record.Name})
		if projectID != "" && projectID != record.ID {
			continue
		}
		layout, err := s.registry.Layout(record)
		if err != nil {
			report.Issues++
			continue
		}
		if record.Type == project.TypeBook && record.Status == project.StatusAvailable {
			summary, err := s.observe(ctx, record, layout, snapshotOnly)
			if err != nil {
				report.Issues++
			} else {
				item := WritingProject{Project: Project{ID: record.ID, Name: record.Name}, Words: int64(summary.TotalWords), Chapters: summary.ChapterCount}
				for _, chapter := range summary.Chapters {
					if chapter.Confirmed {
						item.Confirmed++
					}
				}
				report.Writing.Projects = append(report.Writing.Projects, item)
			}
		}
		if record.Status == project.StatusMissing {
			report.Issues++
		}
		if record.Status == project.StatusAvailable {
			report.readGame(ctx, record, layout, from, until, dayIndex)
		}
		if err := stats.ReadWriting(layout.StoreRoot, from, until, func(observation stats.Writing) {
			if observation.Delta == nil {
				return
			}
			index := dayIndex[observation.At.In(location).Format(time.DateOnly)]
			if report.Writing.Days[index].Delta == nil {
				report.Writing.Days[index].Delta = new(int64)
			}
			*report.Writing.Days[index].Delta += *observation.Delta
		}); err != nil {
			report.Issues++
			slog.Warn("Read manuscript statistics failed", "project_id", record.ID, "error", err)
		}
		if err := stats.ReadCalls(layout.StoreRoot, from, until, func(call stats.Call) {
			report.Usage.Totals.Add(call)
			index := dayIndex[call.At.In(location).Format(time.DateOnly)]
			report.Usage.Days[index].Add(call)
			key := call.Runtime + "\x00" + call.Model
			model := models[key]
			if model == nil {
				model = &Model{Name: call.Model, Runtime: call.Runtime}
				models[key] = model
			}
			model.Add(call)
		}); err != nil {
			report.Issues++
			slog.Warn("Read model statistics failed", "project_id", record.ID, "error", err)
		}
	}
	for _, model := range models {
		report.Usage.Models = append(report.Usage.Models, *model)
	}
	report.Game.sortStories()
	if err := ctx.Err(); err != nil {
		return report, err
	}
	sort.Slice(report.Usage.Models, func(i, j int) bool {
		left, right := report.Usage.Models[i], report.Usage.Models[j]
		if left.Tokens != right.Tokens {
			return left.Tokens > right.Tokens
		}
		if left.Runtime != right.Runtime {
			return left.Runtime < right.Runtime
		}
		return left.Name < right.Name
	})
	sort.Slice(report.Writing.Projects, func(i, j int) bool {
		left, right := report.Writing.Projects[i], report.Writing.Projects[j]
		if left.Words != right.Words {
			return left.Words > right.Words
		}
		return left.ID < right.ID
	})
	return report, nil
}
