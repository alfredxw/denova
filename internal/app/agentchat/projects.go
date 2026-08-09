package agentchat

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/session"
	projectdomain "denova/internal/project"
)

// ProjectSessionsLimit bounds metadata work when the complete user library is
// shown. A running conversation is always retained outside the recent window.
const ProjectSessionsLimit = 50

// AddProject registers a directory without changing the foreground Writing
// workspace. Existing project identity and custom naming are preserved.
func (service *Service) AddProject(path string) (projectdomain.Record, error) {
	if service == nil || service.registry == nil {
		return projectdomain.Record{}, fmt.Errorf("project registry is unavailable")
	}
	kind, err := projectdomain.DetectType(path)
	if err != nil {
		return projectdomain.Record{}, err
	}
	if existing, found, findErr := service.registry.FindByPath(path, true); findErr != nil {
		return projectdomain.Record{}, findErr
	} else if found {
		kind = existing.Type
	}
	record, err := service.registry.Add(path, kind, "")
	if err != nil {
		return projectdomain.Record{}, err
	}
	if _, err := service.registry.EnsureState(record); err != nil {
		return projectdomain.Record{}, err
	}
	return record, nil
}

func (service *Service) RenameProject(id, name string) (projectdomain.Record, error) {
	if service == nil || service.registry == nil {
		return projectdomain.Record{}, fmt.Errorf("project registry is unavailable")
	}
	return service.registry.Rename(id, name)
}

func (service *Service) ReorderProjects(ids []string) error {
	if service == nil || service.registry == nil {
		return fmt.Errorf("project registry is unavailable")
	}
	return service.registry.Reorder(ids)
}

func (service *Service) Projects() []Project {
	if service == nil || service.registry == nil {
		return nil
	}
	startedAt := time.Now()
	currentWorkspace := ""
	if service.host != nil {
		currentWorkspace = service.host.CurrentWorkspace()
	}
	records, err := service.registry.List(false)
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[app/agentchat] list projects failed err=%v", err))
		return []Project{}
	}
	runningBindings := service.runningBindingKeys()
	projects := make([]Project, 0, len(records))
	for _, record := range records {
		project := Project{
			ID: record.ID, Type: record.Type, Path: record.WorkspacePath,
			Name: record.Name, Status: record.Status,
			Current: record.Type == projectdomain.TypeBook && canonicalWorkspaceKey(record.WorkspacePath) == canonicalWorkspaceKey(currentWorkspace),
		}
		layout, layoutErr := service.registry.Layout(record)
		if layoutErr != nil {
			project.Error = layoutErr.Error()
			projects = append(projects, project)
			continue
		}
		metas, readErr := service.readProjectSessions(record.ID, layout.SessionsDir())
		if readErr != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[app/agentchat] read project sessions failed project_id=%s err=%v", record.ID, readErr))
			project.Error = readErr.Error()
			projects = append(projects, project)
			continue
		}
		project.Total = len(metas)
		metas = visibleProjectSessions(metas, record.ID, runningBindings)
		project.Sessions = make([]Session, 0, len(metas))
		for _, meta := range metas {
			project.Sessions = append(project.Sessions, sessionFromMeta(meta, record.ID, runningBindings))
		}
		projects = append(projects, project)
	}
	totalSessions := 0
	for _, project := range projects {
		totalSessions += project.Total
	}
	slog.DebugContext(context.Background(), fmt.Sprintf(
		"[app/agentchat] listed project session metadata projects=%d sessions=%d duration=%s",
		len(projects), totalSessions, time.Since(startedAt),
	))
	return projects
}

func visibleProjectSessions(metas []session.SessionMeta, projectID string, runningBindings map[string]struct{}) []session.SessionMeta {
	if len(metas) <= ProjectSessionsLimit {
		return metas
	}
	visible := append([]session.SessionMeta(nil), metas[:ProjectSessionsLimit]...)
	for _, meta := range metas[ProjectSessionsLimit:] {
		if sessionRunning(projectID, meta.ID, runningBindings) {
			visible = append(visible, meta)
		}
	}
	return visible
}

func sessionRunning(projectID, sessionID string, runningBindings map[string]struct{}) bool {
	_, running := runningBindings[bindingKey(Binding{ProjectID: projectID, SessionID: sessionID})]
	return running
}

func sessionFromMeta(meta session.SessionMeta, projectID string, runningBindings map[string]struct{}) Session {
	return Session{
		ID: meta.ID, Title: meta.Title, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt,
		MessageCount: meta.MessageCount, Running: sessionRunning(projectID, meta.ID, runningBindings),
	}
}

func (service *Service) History(query HistoryQuery) HistoryPage {
	page := HistoryPage{Items: []HistoryItem{}, Offset: max(query.Offset, 0)}
	if service == nil || service.registry == nil || query.Limit <= 0 {
		return page
	}
	startedAt := time.Now()
	normalizedQuery := strings.ToLower(strings.TrimSpace(query.Search))
	projectID := strings.TrimSpace(query.ProjectID)
	runningBindings := service.runningBindingKeys()
	items := make([]HistoryItem, 0)
	records, _ := service.registry.List(false)
	for _, record := range records {
		if projectID != "" && record.ID != projectID {
			continue
		}
		layout, layoutErr := service.registry.Layout(record)
		if layoutErr != nil {
			continue
		}
		metas, err := service.readProjectSessions(record.ID, layout.SessionsDir())
		if err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[app/agentchat] read project history failed project_id=%s err=%v", record.ID, err))
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
			items = append(items, HistoryItem{
				ProjectID: record.ID, ProjectName: record.Name,
				Session: sessionFromMeta(meta, record.ID, runningBindings),
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
	slog.DebugContext(context.Background(), fmt.Sprintf(
		"[app/agentchat] searched conversation history query_length=%d total=%d offset=%d returned=%d duration=%s",
		len([]rune(normalizedQuery)), page.Total, page.Offset, len(page.Items), time.Since(startedAt),
	))
	return page
}

func (service *Service) readProjectSessions(projectID, sessionsDir string) ([]session.SessionMeta, error) {
	store, err := service.projectSessionStore(projectID, sessionsDir, false)
	if err != nil {
		return nil, err
	}
	metas, err := store.List("")
	if err != nil {
		return nil, err
	}
	visible := make([]session.SessionMeta, 0, len(metas))
	for _, meta := range metas {
		if !agentconversation.IsReservedSessionID(meta.ID) {
			visible = append(visible, meta)
		}
	}
	sort.SliceStable(visible, func(i, j int) bool { return visible[i].UpdatedAt.After(visible[j].UpdatedAt) })
	return visible, nil
}

func (service *Service) CreateSession(projectID, title string) (Session, error) {
	binding, err := service.ResolveBinding(Binding{ProjectID: projectID, SessionID: "draft"})
	if err != nil {
		return Session{}, err
	}
	project, err := service.projectRuntime(context.Background(), binding.ProjectID)
	if err != nil {
		return Session{}, err
	}
	runtimeCfg, err := refreshRuntimeConfig(project)
	if err != nil {
		return Session{}, err
	}
	seed, err := agentconversation.RecentSessionSeed(project.store, &runtimeCfg, binding.agentKind, "")
	if err != nil {
		return Session{}, err
	}
	sess, err := project.store.CreateWithRuntimeConfig(title, seed)
	if err != nil {
		return Session{}, err
	}
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[app/agentchat] created project session project_id=%s workspace=%q session_id=%s",
		binding.ProjectID, binding.Workspace, sess.ID,
	))
	return Session{
		ID: sess.ID, Title: sess.Title(), CreatedAt: sess.CreatedAt, UpdatedAt: sess.UpdatedAt,
		MessageCount: sess.MessageCount(),
	}, nil
}

func (service *Service) RenameSession(projectID, sessionID, title string) error {
	binding, err := service.ResolveBinding(Binding{ProjectID: projectID, SessionID: sessionID})
	if err != nil {
		return err
	}
	project, err := service.projectRuntime(context.Background(), binding.ProjectID)
	if err != nil {
		return err
	}
	return project.store.Rename(binding.SessionID, title)
}

// DeleteSession refuses to remove a running conversation and never re-points
// the foreground Writing Session.
func (service *Service) DeleteSession(projectID, sessionID string) error {
	binding, err := service.ResolveBinding(Binding{ProjectID: projectID, SessionID: sessionID})
	if err != nil {
		return err
	}
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
	service.starts.ReleaseScope(binding.ProjectID, binding.SessionID)
	return nil
}
