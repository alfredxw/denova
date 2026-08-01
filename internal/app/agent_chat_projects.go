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
	ID     string        `json:"id"`
	Type   ProjectType   `json:"type"`
	Path   string        `json:"path"`
	Name   string        `json:"name"`
	Status ProjectStatus `json:"status"`
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
	ProjectID   string           `json:"project_id"`
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
	ProjectID string
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

	records, err := a.Projects(false)
	if err != nil {
		log.Printf("[app/agent_chat_projects.go] listing projects failed err=%v", err)
		return []AgentChatProject{}
	}
	runningBindings := a.agentChat().runningBindingKeys()
	projects := make([]AgentChatProject, 0, len(records))
	for _, record := range records {
		project := AgentChatProject{
			ID: record.ID, Type: record.Type, Path: record.WorkspacePath, Name: record.Name, Status: record.Status,
			Current: record.Type == ProjectTypeBook && lifecycleWorkspaceKey(record.WorkspacePath) == lifecycleWorkspaceKey(currentWorkspace),
		}
		layout, layoutErr := a.projectRegistry.Layout(record)
		if layoutErr != nil {
			project.Error = layoutErr.Error()
			projects = append(projects, project)
			continue
		}
		metas, err := readProjectSessions(layout.SessionsDir(), "")
		if err != nil {
			log.Printf("[app/agent_chat_projects.go] reading sessions failed project_id=%s err=%v", record.ID, err)
			project.Error = err.Error()
			projects = append(projects, project)
			continue
		}
		project.Total = len(metas)
		metas = visibleAgentChatProjectSessions(metas, record.ID, runningBindings)
		project.Sessions = make([]AgentChatSession, 0, len(metas))
		for _, meta := range metas {
			project.Sessions = append(project.Sessions, agentChatSessionFromMeta(meta, record.ID, runningBindings))
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
	projectID string,
	runningBindings map[string]struct{},
) []session.SessionMeta {
	if len(metas) <= AgentChatProjectSessionsLimit {
		return metas
	}
	visible := append([]session.SessionMeta(nil), metas[:AgentChatProjectSessionsLimit]...)
	for _, meta := range metas[AgentChatProjectSessionsLimit:] {
		if agentChatSessionRunning(projectID, meta.ID, runningBindings) {
			visible = append(visible, meta)
		}
	}
	return visible
}

func agentChatSessionRunning(projectID, sessionID string, runningBindings map[string]struct{}) bool {
	_, running := runningBindings[agentChatBindingKey(AgentChatBinding{ProjectID: projectID, SessionID: sessionID})]
	return running
}

func agentChatSessionFromMeta(
	meta session.SessionMeta,
	projectID string,
	runningBindings map[string]struct{},
) AgentChatSession {
	return AgentChatSession{
		ID: meta.ID, Title: meta.Title, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt,
		MessageCount: meta.MessageCount, Running: agentChatSessionRunning(projectID, meta.ID, runningBindings),
	}
}

// AgentChatHistory searches durable conversation metadata without switching the foreground
// workspace. An optional stable Project ID lets the history browser paginate one project at a time.
func (a *App) AgentChatHistory(query AgentChatHistoryQuery) AgentChatHistoryPage {
	page := AgentChatHistoryPage{Items: []AgentChatHistoryItem{}, Offset: max(query.Offset, 0)}
	if a == nil || query.Limit <= 0 {
		return page
	}
	startedAt := time.Now()
	normalizedQuery := strings.ToLower(strings.TrimSpace(query.Search))
	projectID := strings.TrimSpace(query.ProjectID)
	runningBindings := a.agentChat().runningBindingKeys()
	items := make([]AgentChatHistoryItem, 0)
	records, _ := a.Projects(false)
	for _, record := range records {
		if projectID != "" && record.ID != projectID {
			continue
		}
		layout, layoutErr := a.projectRegistry.Layout(record)
		if layoutErr != nil {
			continue
		}
		metas, err := readProjectSessions(layout.SessionsDir(), "")
		if err != nil {
			log.Printf("[app/agent_chat_projects.go] reading history failed project_id=%s err=%v", record.ID, err)
			continue
		}
		projectSearchText := strings.ToLower(record.Name + " " + record.WorkspacePath)
		for _, meta := range metas {
			if normalizedQuery != "" {
				sessionSearchText := strings.ToLower(meta.Title + " " + meta.ID)
				if !strings.Contains(sessionSearchText, normalizedQuery) && !strings.Contains(projectSearchText, normalizedQuery) {
					continue
				}
			}
			items = append(items, AgentChatHistoryItem{
				ProjectID: record.ID, ProjectName: record.Name,
				Session: agentChatSessionFromMeta(meta, record.ID, runningBindings),
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].Session.UpdatedAt.Equal(items[j].Session.UpdatedAt) {
			return items[i].Session.UpdatedAt.After(items[j].Session.UpdatedAt)
		}
		if items[i].ProjectID != items[j].ProjectID {
			return items[i].ProjectID < items[j].ProjectID
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

func readProjectSessions(sessionsDir, activeID string) ([]session.SessionMeta, error) {
	store, err := session.NewStore(sessionsDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Printf("[app/agent_chat_projects.go] closing session store failed sessions_dir=%q err=%v", sessionsDir, closeErr)
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
func (a *App) CreateProjectSession(projectID, title string) (AgentChatSession, error) {
	binding, err := a.agentChat().resolveBinding(AgentChatBinding{ProjectID: projectID, SessionID: "draft"})
	if err != nil {
		return AgentChatSession{}, err
	}
	project, err := a.agentChat().projectRuntime(context.Background(), binding.ProjectID)
	if err != nil {
		return AgentChatSession{}, err
	}
	runtimeCfg, err := refreshConversationRuntimeConfig(project.cfg, project.workspace, project.stateRoot)
	if err != nil {
		return AgentChatSession{}, err
	}
	seed, err := recentConversationSeed(project.store, &runtimeCfg, binding.agentKind, "")
	if err != nil {
		return AgentChatSession{}, err
	}
	sess, err := project.store.CreateWithRuntimeConfig(title, seed)
	if err != nil {
		return AgentChatSession{}, err
	}
	log.Printf("[app/agent_chat_projects.go] created project session project_id=%s workspace=%q session=%s", binding.ProjectID, binding.Workspace, sess.ID)
	return AgentChatSession{
		ID: sess.ID, Title: sess.Title(), CreatedAt: sess.CreatedAt, UpdatedAt: sess.UpdatedAt,
		MessageCount: sess.MessageCount(),
	}, nil
}

func (a *App) RenameProjectSession(projectID, sessionID, title string) error {
	binding, err := a.agentChat().resolveBinding(AgentChatBinding{ProjectID: projectID, SessionID: sessionID})
	if err != nil {
		return err
	}
	project, err := a.agentChat().projectRuntime(context.Background(), binding.ProjectID)
	if err != nil {
		return err
	}
	return project.store.Rename(strings.TrimSpace(sessionID), title)
}

// DeleteProjectSession refuses to remove a running conversation and never
// re-points the foreground Writing session.
func (a *App) DeleteProjectSession(projectID, sessionID string) error {
	binding, err := a.agentChat().resolveBinding(AgentChatBinding{ProjectID: projectID, SessionID: strings.TrimSpace(sessionID)})
	if err != nil {
		return err
	}
	service := a.agentChat()
	if err := service.requireIdle(binding); err != nil {
		return err
	}
	project, err := service.projectRuntime(context.Background(), binding.ProjectID)
	if err != nil {
		return err
	}
	if err := project.chatService.CloseProjectSessionBindings(context.Background(), binding.ProjectID, binding.SessionID); err != nil {
		return err
	}
	if err := project.store.Delete(binding.SessionID); err != nil {
		return err
	}
	service.starts.releaseConfigManagerScope(binding.ProjectID, binding.SessionID)
	return nil
}

// canonicalAgentChatWorkspace is retained for the temporary path-based client
// migration. New callers must bind by stable project ID.
func (a *App) canonicalAgentChatWorkspace(workspace string) (string, error) {
	if a == nil {
		return "", ErrNoWorkspace
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", fmt.Errorf("AgentChat project workspace is required")
	}
	_, layout, err := a.resolveProjectByWorkspace(workspace)
	if err != nil {
		return "", fmt.Errorf("AgentChat project is not registered: %s: %w", workspace, err)
	}
	return layout.ContentRoot, nil
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
