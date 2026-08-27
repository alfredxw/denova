package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/agents/trajectory"
)

type AgentAskAnswer = agentconversation.HostAskAnswer
type AgentAskSelectedOption = agentconversation.HostAskSelectedOption
type AgentAskAnswerResult = agentconversation.HostAskAnswerResult
type AgentAskResolution = agentconversation.HostAskResolution

var ErrAgentAskNotFound = agent.ErrInteractionStale

const defaultGlobalAgentRunTraceLimit = 100

var ErrDeveloperModeDisabled = errors.New("Developer Mode is disabled")

// GlobalAgentRunTraceSummary identifies a Run without relying on the foreground
// Project. TrajectoryURI is generated at the application boundary so clients do
// not duplicate the model-visible resource addressing contract.
type GlobalAgentRunTraceSummary struct {
	agentrun.RunTraceSummary
	ProjectID     string `json:"project_id"`
	ProjectName   string `json:"project_name"`
	SessionTitle  string `json:"session_title,omitempty"`
	TrajectoryURI string `json:"trajectory_uri"`
}

type GlobalAgentRunTraceIssue struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Message     string `json:"message"`
}

type GlobalAgentRunTraceCatalog struct {
	Runs   []GlobalAgentRunTraceSummary `json:"runs"`
	Issues []GlobalAgentRunTraceIssue   `json:"issues"`
}

// GlobalAgentRunTraceTarget keeps an explicitly opened Run in the bounded
// global catalog even when newer Runs would otherwise push it past the limit.
type GlobalAgentRunTraceTarget struct {
	ProjectID string
	RunID     string
}

func (a *App) persistAgentCall(agentKind, instruction, response string) {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	persistAgentCallWithStore(store, agentKind, instruction, response)
}

func persistAgentCallWithStore(store *session.Store, agentKind, instruction, response string) {
	if store == nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[agent-session] skip persist agent=%s reason=no_session_store", agentKind))
		return
	}
	if err := session.PersistAgentCall(store, agentKind, instruction, response); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-session] persist failed agent=%s err=%v", agentKind, err))
	}
}

func (a *App) AgentSessionMessages(agentKind string) ([]session.HistoryEntry, error) {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	sess, err := agentSessionFromStore(store, agentKind)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (a *App) AgentSessionMessagesPage(ctx context.Context, agentKind string, before, limit int) (session.HistoryPage, error) {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	sess, err := agentSessionFromStore(store, agentKind)
	if err != nil {
		return session.HistoryPage{}, err
	}
	return sess.ReadHistoryPage(ctx, before, limit)
}

func (a *App) ClearAgentSession(agentKind string) error {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	if store == nil {
		return ErrNoWorkspace
	}
	return session.ClearAgentSession(store, agentKind)
}

func persistAgentCallInStore(store *session.Store, agentKind, instruction, response string) error {
	return session.PersistAgentCall(store, agentKind, instruction, response)
}

func clearAgentSessionInStore(store *session.Store, agentKind string) error {
	return session.ClearAgentSession(store, agentKind)
}

func agentSessionFromStore(store *session.Store, agentKind string) (*session.Session, error) {
	if store == nil {
		return nil, ErrNoWorkspace
	}
	return session.AgentSession(store, agentKind)
}

func agentSessionID(agentKind string) (string, bool) {
	return session.AgentSessionID(agentKind)
}

// AnswerSessionAsk answers the exact pending ask in a user IDE session. The
// blocked tool call remains inside the same durable Agent task.
func (a *App) AnswerSessionAsk(ctx context.Context, sessionID, askID string, answers []AgentAskAnswer) (AgentAskResolution, error) {
	return a.resolveSessionAsk(ctx, sessionID, askID, session.AskAnswered, answers, "")
}

func (a *App) CancelSessionAsk(ctx context.Context, sessionID, askID, reason string) (AgentAskResolution, error) {
	return a.resolveSessionAsk(ctx, sessionID, askID, session.AskCancelled, nil, reason)
}

func (a *App) resolveSessionAsk(ctx context.Context, sessionID, askID, status string, answers []AgentAskAnswer, cancelReason string) (AgentAskResolution, error) {
	if a == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.sessionStore
	selected := a.session
	workspace := a.workspace
	executionRuntime := a.executionRuntime
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
	}
	a.mu.RUnlock()
	if store == nil || selected == nil || executionRuntime == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = selected.ID
	}
	if isAgentSessionID(sessionID) {
		return AgentAskResolution{}, fmt.Errorf("cannot resolve a fixed Agent ask through the IDE session endpoint: %s", sessionID)
	}
	if _, err := store.Get(sessionID); err != nil {
		return AgentAskResolution{}, err
	}
	return executionRuntime.ResolveAsk(ctx, agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, StateRoot: stateRoot,
		Workspace: workspace, SessionID: sessionID, Mode: "ide",
	}, askID, status, answers, cancelReason)
}

func (a *App) AgentRunTraces(limit int) ([]agentrun.RunTraceSummary, error) {
	location, ok := a.agentRunTraceLocation()
	if !ok {
		return []agentrun.RunTraceSummary{}, nil
	}
	return agentrun.ListRunTraces(location, limit)
}

// GlobalAgentRunTraces merges recent Runs from every registered Project. A
// broken Project is reported independently and never hides healthy Runs.
func (a *App) GlobalAgentRunTraces(ctx context.Context, limit int, target GlobalAgentRunTraceTarget) (GlobalAgentRunTraceCatalog, error) {
	if !a.DeveloperModeEnabled() {
		return GlobalAgentRunTraceCatalog{}, ErrDeveloperModeDisabled
	}
	if limit <= 0 || limit > defaultGlobalAgentRunTraceLimit {
		limit = defaultGlobalAgentRunTraceLimit
	}
	target.ProjectID = strings.TrimSpace(target.ProjectID)
	target.RunID = strings.TrimSpace(target.RunID)
	sources, issues, err := a.globalTrajectorySources(ctx)
	if err != nil {
		return GlobalAgentRunTraceCatalog{}, err
	}
	result := GlobalAgentRunTraceCatalog{
		Runs:   make([]GlobalAgentRunTraceSummary, 0),
		Issues: append([]GlobalAgentRunTraceIssue(nil), issues...),
	}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return GlobalAgentRunTraceCatalog{}, err
		}
		runs, listErr := agentrun.ListRunTraces(agentrun.TraceLocation{Workspace: source.Workspace, StateRoot: source.StateRoot}, limit)
		if listErr != nil {
			slog.WarnContext(ctx, "[trajectory] Project Run catalog read failed", "project_id", source.ProjectID, "error", listErr)
			result.Issues = append(result.Issues, GlobalAgentRunTraceIssue{
				ProjectID: source.ProjectID, ProjectName: source.Name, Message: "Run trajectories are unavailable for this Project",
			})
			continue
		}
		if source.ProjectID == target.ProjectID && target.RunID != "" && !slices.ContainsFunc(runs, func(run agentrun.RunTraceSummary) bool {
			return run.ID == target.RunID
		}) {
			trace, targetErr := agentrun.ReadRunTrace(agentrun.TraceLocation{Workspace: source.Workspace, StateRoot: source.StateRoot}, target.RunID)
			if targetErr == nil {
				runs = append(runs, trace.Summary)
			} else if !os.IsNotExist(targetErr) {
				slog.WarnContext(ctx, "[trajectory] targeted Run read failed", "project_id", source.ProjectID, "run_id", target.RunID, "error", targetErr)
			}
		}
		sessionTitles, titleErr := projectSessionTitles(source.StateRoot, runs)
		if titleErr != nil {
			slog.WarnContext(ctx, "[trajectory] Project Session titles are unavailable", "project_id", source.ProjectID, "error", titleErr)
		}
		for _, run := range runs {
			run.Path = ""
			result.Runs = append(result.Runs, GlobalAgentRunTraceSummary{
				RunTraceSummary: run,
				ProjectID:       source.ProjectID,
				ProjectName:     source.Name,
				SessionTitle:    sessionTitles[run.SessionID],
				TrajectoryURI:   trajectory.RunURI(source.ProjectID, run.ID),
			})
		}
	}
	sort.SliceStable(result.Runs, func(i, j int) bool {
		left, right := result.Runs[i], result.Runs[j]
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}
		if left.ProjectID != right.ProjectID {
			return left.ProjectID < right.ProjectID
		}
		return left.ID < right.ID
	})
	result.Runs = boundedGlobalRunCatalog(result.Runs, limit, target)
	return result, nil
}

func boundedGlobalRunCatalog(runs []GlobalAgentRunTraceSummary, limit int, target GlobalAgentRunTraceTarget) []GlobalAgentRunTraceSummary {
	if len(runs) <= limit {
		return runs
	}
	targetIndex := -1
	for index, run := range runs {
		if run.ProjectID == target.ProjectID && run.ID == target.RunID {
			targetIndex = index
			break
		}
	}
	if targetIndex < limit || targetIndex < 0 {
		return runs[:limit]
	}
	bounded := append([]GlobalAgentRunTraceSummary(nil), runs[:limit]...)
	bounded[limit-1] = runs[targetIndex]
	return bounded
}

// projectSessionTitles reads only existing Session projections. Missing or
// damaged metadata must never hide Runs; callers can fall back to Session IDs.
func projectSessionTitles(stateRoot string, runs []agentrun.RunTraceSummary) (map[string]string, error) {
	wanted := make(map[string]struct{})
	for _, run := range runs {
		if sessionID := strings.TrimSpace(run.SessionID); sessionID != "" {
			wanted[sessionID] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, nil
	}
	stateRoot = strings.TrimSpace(stateRoot)
	if stateRoot == "" {
		return nil, nil
	}
	directory := filepath.Join(stateRoot, "sessions")
	info, err := os.Stat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Session state path is not a directory")
	}
	store, err := session.NewStore(directory)
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()
	metas, err := store.List("")
	if err != nil {
		return nil, err
	}
	titles := make(map[string]string, len(wanted))
	for _, meta := range metas {
		if _, ok := wanted[meta.ID]; !ok {
			continue
		}
		if title := strings.TrimSpace(meta.Title); title != "" {
			titles[meta.ID] = title
		}
	}
	return titles, nil
}

func (a *App) DeveloperModeEnabled() bool {
	if a == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg != nil && a.cfg.Labs.DeveloperMode
}

func (a *App) globalTrajectorySources(ctx context.Context) ([]trajectory.Source, []GlobalAgentRunTraceIssue, error) {
	if a == nil || a.projectRegistry == nil {
		return nil, nil, fmt.Errorf("Project registry is unavailable")
	}
	records, err := a.projectRegistry.List(true)
	if err != nil {
		return nil, nil, err
	}
	sources := make([]trajectory.Source, 0, len(records))
	issues := make([]GlobalAgentRunTraceIssue, 0)
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		name := strings.TrimSpace(record.Name)
		if name == "" {
			name = record.ID
		}
		// Trajectory discovery must not open, migrate, or switch a dormant
		// Project. Its durable StateRoot is sufficient even when content is
		// archived or temporarily unavailable.
		layout, layoutErr := a.projectRegistry.Layout(record)
		if layoutErr != nil {
			issues = append(issues, GlobalAgentRunTraceIssue{
				ProjectID: record.ID, ProjectName: name, Message: "Project trajectory state is unavailable",
			})
			continue
		}
		sources = append(sources, trajectory.Source{
			ProjectID: record.ID, Name: name, Workspace: layout.ContentRoot, StateRoot: layout.StateRoot,
		})
	}
	return sources, issues, nil
}

func (a *App) AgentRunTrace(id string) (agentrun.RunTrace, error) {
	location, ok := a.agentRunTraceLocation()
	if !ok {
		return agentrun.RunTrace{}, ErrNoWorkspace
	}
	return agentrun.ReadRunTrace(location, id)
}

func (a *App) ExportAgentRunTrace(id string) (agentrun.RunTraceExport, error) {
	location, ok := a.agentRunTraceLocation()
	if !ok {
		return agentrun.RunTraceExport{}, ErrNoWorkspace
	}
	return agentrun.ExportRunTrace(location, id)
}

// ProjectAgentRunTraces reads trace summaries from one stable Project without
// consulting or switching the foreground Book.
func (a *App) ProjectAgentRunTraces(projectID string, limit int) ([]agentrun.RunTraceSummary, error) {
	location, err := a.projectAgentRunTraceLocation(projectID)
	if err != nil {
		return nil, err
	}
	return agentrun.ListRunTraces(location, limit)
}

func (a *App) ProjectAgentRunTrace(projectID, id string) (agentrun.RunTrace, error) {
	location, err := a.projectAgentRunTraceLocation(projectID)
	if err != nil {
		return agentrun.RunTrace{}, err
	}
	return agentrun.ReadRunTrace(location, id)
}

func (a *App) ExportProjectAgentRunTrace(projectID, id string) (agentrun.RunTraceExport, error) {
	location, err := a.projectAgentRunTraceLocation(projectID)
	if err != nil {
		return agentrun.RunTraceExport{}, err
	}
	return agentrun.ExportRunTrace(location, id)
}

func (a *App) projectAgentRunTraceLocation(projectID string) (agentrun.TraceLocation, error) {
	if a == nil || a.projectRegistry == nil {
		return agentrun.TraceLocation{}, fmt.Errorf("Project registry is unavailable")
	}
	record, err := a.projectRegistry.Get(strings.TrimSpace(projectID))
	if err != nil {
		return agentrun.TraceLocation{}, err
	}
	layout, err := a.projectRegistry.Layout(record)
	if err != nil {
		return agentrun.TraceLocation{}, err
	}
	return agentrun.TraceLocation{Workspace: layout.ContentRoot, StateRoot: layout.StateRoot}, nil
}

func (a *App) agentRunTraceLocation() (agentrun.TraceLocation, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	workspace := strings.TrimSpace(a.workspace)
	if workspace == "" {
		return agentrun.TraceLocation{}, false
	}
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = strings.TrimSpace(a.cfg.ProjectStateDir)
	}
	return agentrun.TraceLocation{Workspace: workspace, StateRoot: stateRoot}, true
}
