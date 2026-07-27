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
		project.Total = len(metas)
		if len(metas) > AgentChatProjectSessionsLimit {
			metas = metas[:AgentChatProjectSessionsLimit]
		}
		runningWorkspace := record.Path
		if len(runningBindings) > 0 {
			if canonical, canonicalErr := canonicalWorkspacePath(record.Path); canonicalErr == nil {
				runningWorkspace = canonical
			}
		}
		project.Sessions = make([]AgentChatSession, 0, len(metas))
		for _, meta := range metas {
			_, running := runningBindings[agentChatBindingKey(AgentChatBinding{
				Workspace: runningWorkspace, SessionID: meta.ID,
			})]
			project.Sessions = append(project.Sessions, AgentChatSession{
				ID: meta.ID, Title: meta.Title, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt,
				MessageCount: meta.MessageCount, Running: running,
			})
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
