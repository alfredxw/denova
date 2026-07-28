package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"denova/internal/agents/session"
	"denova/internal/workspacepath"
)

// AgentChatProjectSessionsLimit bounds how many conversations are read per
// project when the whole user library is shown at once.
const AgentChatProjectSessionsLimit = 50

// AgentChatSession is one conversation shown in the AgentChat project tree.
type AgentChatSession struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
	// Active is retained for the session DTO contract, but AgentChat has no
	// singleton active session. Running is the scoped execution state.
	Active  bool `json:"active"`
	Running bool `json:"running"`
}

// AgentChatProject is one registered book with its conversations.
type AgentChatProject struct {
	Path string `json:"path"`
	Name string `json:"name"`
	// Current is informational only: AgentChat never changes this foreground
	// Writing selection when a project or conversation is opened.
	Current  bool               `json:"current"`
	Total    int                `json:"total"`
	Sessions []AgentChatSession `json:"sessions"`
	Error    string             `json:"error,omitempty"`
}

// AgentChatHistoryItem keeps project ownership explicit when conversations from the whole
// library are searched together.
type AgentChatHistoryItem struct {
	Workspace   string           `json:"workspace"`
	ProjectName string           `json:"project_name"`
	Session     AgentChatSession `json:"session"`
}

// AgentChatHistoryPage is bounded independently from the lightweight project activity snapshot.
type AgentChatHistoryPage struct {
	Items   []AgentChatHistoryItem `json:"items"`
	Total   int                    `json:"total"`
	Offset  int                    `json:"offset"`
	HasMore bool                   `json:"has_more"`
}

// AgentChatHistoryQuery keeps project scope, search, and pagination explicit at the app boundary.
type AgentChatHistoryQuery struct {
	Workspace string
	Search    string
	Offset    int
	Limit     int
}

// AgentChatProjects lists every book without changing App.workspace or the
// foreground Writing session.
func (a *App) AgentChatProjects() []AgentChatProject {
	if a == nil {
		return nil
	}
	startedAt := time.Now()
	a.mu.RLock()
	currentWorkspace := a.workspace
	a.mu.RUnlock()

	books := a.Books()
	runningBindings := a.agentChat().runningBindingKeys()
	projects := make([]AgentChatProject, 0, len(books))
	for _, record := range books {
		project := AgentChatProject{
			Path: record.Path, Name: record.Name,
			Current: lifecycleWorkspaceKey(record.Path) == lifecycleWorkspaceKey(currentWorkspace),
		}
		metas, err := readWorkspaceSessions(record.Path, "")
		if err != nil {
			log.Printf("[app/agent_chat_projects.go] reading sessions failed workspace=%q err=%v", record.Path, err)
			project.Error = err.Error()
			projects = append(projects, project)
			continue
		}
		runningWorkspace := record.Path
		if len(runningBindings) > 0 {
			if canonical, canonicalErr := canonicalWorkspacePath(record.Path); canonicalErr == nil {
				runningWorkspace = canonical
			}
		}
		project.Total = len(metas)
		metas = visibleAgentChatProjectSessions(metas, runningWorkspace, runningBindings)
		project.Sessions = make([]AgentChatSession, 0, len(metas))
		for _, meta := range metas {
			project.Sessions = append(project.Sessions, agentChatSessionFromMeta(meta, runningWorkspace, runningBindings))
		}
		projects = append(projects, project)
	}
	totalSessions := 0
	for _, project := range projects {
		totalSessions += project.Total
	}
	log.Printf(
		"[app/agent_chat_projects.go] listed project session metadata projects=%d sessions=%d duration=%s",
		len(projects), totalSessions, time.Since(startedAt),
	)
	return projects
}

// Keep the project snapshot cheap without ever hiding a running conversation. Detached tasks
// must remain reachable from the activity sidebar even when their older metadata falls outside
// the normal recent-session window.
func visibleAgentChatProjectSessions(
	metas []session.SessionMeta,
	workspace string,
	runningBindings map[string]struct{},
) []session.SessionMeta {
	if len(metas) <= AgentChatProjectSessionsLimit {
		return metas
	}
	visible := append([]session.SessionMeta(nil), metas[:AgentChatProjectSessionsLimit]...)
	for _, meta := range metas[AgentChatProjectSessionsLimit:] {
		if agentChatSessionRunning(workspace, meta.ID, runningBindings) {
			visible = append(visible, meta)
		}
	}
	return visible
}

func agentChatSessionRunning(workspace, sessionID string, runningBindings map[string]struct{}) bool {
	_, running := runningBindings[agentChatBindingKey(AgentChatBinding{Workspace: workspace, SessionID: sessionID})]
	return running
}

func agentChatSessionFromMeta(
	meta session.SessionMeta,
	workspace string,
	runningBindings map[string]struct{},
) AgentChatSession {
	return AgentChatSession{
		ID: meta.ID, Title: meta.Title, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt,
		MessageCount: meta.MessageCount, Running: agentChatSessionRunning(workspace, meta.ID, runningBindings),
	}
}

// AgentChatHistory searches durable conversation metadata without switching the foreground
// workspace. An optional workspace scope lets the history browser paginate one project at a time.
func (a *App) AgentChatHistory(query AgentChatHistoryQuery) AgentChatHistoryPage {
	page := AgentChatHistoryPage{Items: []AgentChatHistoryItem{}, Offset: max(query.Offset, 0)}
	if a == nil || query.Limit <= 0 {
		return page
	}
	startedAt := time.Now()
	normalizedQuery := strings.ToLower(strings.TrimSpace(query.Search))
	workspaceKey := ""
	if workspace := strings.TrimSpace(query.Workspace); workspace != "" {
		workspaceKey = lifecycleWorkspaceKey(workspace)
	}
	runningBindings := a.agentChat().runningBindingKeys()
	items := make([]AgentChatHistoryItem, 0)
	for _, record := range a.Books() {
		if workspaceKey != "" && lifecycleWorkspaceKey(record.Path) != workspaceKey {
			continue
		}
		metas, err := readWorkspaceSessions(record.Path, "")
		if err != nil {
			log.Printf("[app/agent_chat_projects.go] reading history failed workspace=%q err=%v", record.Path, err)
			continue
		}
		runningWorkspace := record.Path
		if len(runningBindings) > 0 {
			if canonical, canonicalErr := canonicalWorkspacePath(record.Path); canonicalErr == nil {
				runningWorkspace = canonical
			}
		}
		projectSearchText := strings.ToLower(record.Name + " " + record.Path)
		for _, meta := range metas {
			if normalizedQuery != "" {
				sessionSearchText := strings.ToLower(meta.Title + " " + meta.ID)
				if !strings.Contains(sessionSearchText, normalizedQuery) && !strings.Contains(projectSearchText, normalizedQuery) {
					continue
				}
			}
			items = append(items, AgentChatHistoryItem{
				Workspace: record.Path, ProjectName: record.Name,
				Session: agentChatSessionFromMeta(meta, runningWorkspace, runningBindings),
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].Session.UpdatedAt.Equal(items[j].Session.UpdatedAt) {
			return items[i].Session.UpdatedAt.After(items[j].Session.UpdatedAt)
		}
		if items[i].Workspace != items[j].Workspace {
			return items[i].Workspace < items[j].Workspace
		}
		return items[i].Session.ID < items[j].Session.ID
	})
	page.Total = len(items)
	if page.Offset >= page.Total {
		return page
	}
	end := min(page.Offset+query.Limit, page.Total)
	page.Items = append(page.Items, items[page.Offset:end]...)
	page.HasMore = end < page.Total
	log.Printf(
		"[app/agent_chat_projects.go] searched conversation history query_length=%d total=%d offset=%d returned=%d duration=%s",
		len([]rune(normalizedQuery)), page.Total, page.Offset, len(page.Items), time.Since(startedAt),
	)
	return page
}

func readWorkspaceSessions(workspace, activeID string) ([]session.SessionMeta, error) {
	store, err := session.NewStore(workspacepath.Path(workspace, "sessions"))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Printf("[app/agent_chat_projects.go] closing session store failed workspace=%q err=%v", workspace, closeErr)
		}
	}()
	metas, err := listUserSessions(store, activeID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(metas, func(i, j int) bool { return metas[i].UpdatedAt.After(metas[j].UpdatedAt) })
	return metas, nil
}

// CreateProjectSession creates a conversation without selecting it in the
// foreground Writing workspace.
func (a *App) CreateProjectSession(workspace, title string) (AgentChatSession, error) {
	canonical, err := a.canonicalAgentChatWorkspace(workspace)
	if err != nil {
		return AgentChatSession{}, err
	}
	project, err := a.agentChat().projectRuntime(context.Background(), canonical)
	if err != nil {
		return AgentChatSession{}, err
	}
	sess, err := project.store.Create(title)
	if err != nil {
		return AgentChatSession{}, err
	}
	log.Printf("[app/agent_chat_projects.go] created project session workspace=%q session=%s", canonical, sess.ID)
	return AgentChatSession{
		ID: sess.ID, Title: sess.Title(), CreatedAt: sess.CreatedAt, UpdatedAt: sess.UpdatedAt,
		MessageCount: sess.MessageCount(),
	}, nil
}

func (a *App) RenameProjectSession(workspace, sessionID, title string) error {
	canonical, err := a.canonicalAgentChatWorkspace(workspace)
	if err != nil {
		return err
	}
	project, err := a.agentChat().projectRuntime(context.Background(), canonical)
	if err != nil {
		return err
	}
	return project.store.Rename(strings.TrimSpace(sessionID), title)
}

// DeleteProjectSession refuses to remove a running conversation and never
// re-points the foreground Writing session.
func (a *App) DeleteProjectSession(workspace, sessionID string) error {
	canonical, err := a.canonicalAgentChatWorkspace(workspace)
	if err != nil {
		return err
	}
	binding := AgentChatBinding{Workspace: canonical, SessionID: strings.TrimSpace(sessionID)}
	service := a.agentChat()
	if err := service.requireIdle(binding); err != nil {
		return err
	}
	project, err := service.projectRuntime(context.Background(), canonical)
	if err != nil {
		return err
	}
	if err := project.chatService.CloseAgentChatSessionBindings(context.Background(), canonical, binding.SessionID); err != nil {
		return err
	}
	if err := project.store.Delete(binding.SessionID); err != nil {
		return err
	}
	service.starts.releaseConfigManagerScope(canonical, binding.SessionID)
	return nil
}

// canonicalAgentChatWorkspace accepts only projects registered in the user's
// library. Explicit project binding therefore cannot become arbitrary
// filesystem access.
func (a *App) canonicalAgentChatWorkspace(workspace string) (string, error) {
	if a == nil {
		return "", ErrNoWorkspace
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", fmt.Errorf("AgentChat project workspace is required")
	}
	requested, err := canonicalWorkspacePath(workspace)
	if err != nil {
		return "", err
	}
	for _, record := range a.Books() {
		candidate, candidateErr := canonicalWorkspacePath(record.Path)
		if candidateErr == nil && candidate == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("AgentChat project is not registered: %s", workspace)
}

func canonicalWorkspacePath(path string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}
