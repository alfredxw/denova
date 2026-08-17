package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/session"
	appconversation "denova/internal/app/conversation"
	appsettings "denova/internal/app/settings"
	"denova/internal/book"
	"denova/internal/interactive"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

type ProjectType = projectdomain.Type
type ProjectLayout = projectdomain.Layout

const ProjectTypeBook = projectdomain.TypeBook

type runtimeState struct {
	projectID        string
	projectStateRoot string
	workspace        string
	bookState        *book.State
	bookService      *book.Service
	interactive      *interactive.Store
	sessionStore     *session.Store
	session          *session.Session
	versionService   *book.VersionService
}

// buildRuntimeExclusively initializes a runtime while holding the same
// per-workspace mutation boundary used by editors and agents. This matters for
// inactive automation targets too: selecting that workspace cannot rebuild
// session/story projections concurrently with a background write.
func buildRuntimeExclusively(ctx context.Context, cfg *config.Config, layout ProjectLayout) (*runtimeState, error) {
	workspace := layout.ContentRoot
	changes, err := workspacechange.ForWorkspaceAt(workspace, layout.StateRoot)
	if err != nil {
		return nil, err
	}
	var runtime *runtimeState
	err = changes.WithExclusiveWorkspace(ctx, func() error {
		var buildErr error
		runtime, buildErr = buildRuntime(ctx, cfg, layout)
		return buildErr
	})
	return runtime, err
}

func buildRuntime(ctx context.Context, cfg *config.Config, layout ProjectLayout) (*runtimeState, error) {
	workspace := layout.ContentRoot
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(absWorkspace)
	if err != nil {
		return nil, fmt.Errorf("resolve canonical workspace path: %w", err)
	}
	info, err := os.Stat(canonicalWorkspace)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("workspace directory does not exist: %s", canonicalWorkspace)
	}
	absWorkspace = filepath.Clean(canonicalWorkspace)

	state := book.NewState(absWorkspace)
	if err := state.InitWorkspace(); err != nil {
		return nil, fmt.Errorf("initialize workspace: %w", err)
	}
	store, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		return nil, fmt.Errorf("create session store: %w", err)
	}
	keepStore := false
	defer func() {
		if !keepStore {
			_ = store.Close()
		}
	}()
	runtimeCfg := *cfg
	runtimeCfg.Workspace = absWorkspace
	runtimeCfg.ProjectID = layout.ProjectID
	runtimeCfg.ProjectStateDir = layout.StateRoot
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(runtimeCfg.DataDir(), absWorkspace, layout.ConfigPath()); loadErr == nil {
		appsettings.ApplyLayered(&runtimeCfg, layered)
	} else {
		return nil, fmt.Errorf("load project settings: %w", loadErr)
	}
	sess, err := activeUserSessionOrCreate(store, &runtimeCfg)
	if err != nil {
		return nil, fmt.Errorf("create conversation session: %w", err)
	}
	interactiveStore := interactive.NewStoreWithNovaDir(absWorkspace, runtimeCfg.DataDir())
	interruptedDirectorRuns, directorRecoveryErr := interactiveStore.RecoverInterruptedDirectorRuns()
	if directorRecoveryErr != nil {
		// Recovery is branch-scoped and reports partial failures. A corrupt
		// optional projection must not make the user's whole project unavailable.
		slog.ErrorContext(ctx, fmt.Sprintf("[interactive-director] interrupted run recovery incomplete workspace=%s recovered=%d error=%v", absWorkspace, interruptedDirectorRuns, directorRecoveryErr))
	} else if interruptedDirectorRuns > 0 {
		slog.InfoContext(ctx, fmt.Sprintf("[interactive-director] recovered interrupted runs workspace=%s runs=%d", absWorkspace, interruptedDirectorRuns))
	}
	runtime := &runtimeState{
		projectID:        layout.ProjectID,
		projectStateRoot: layout.StateRoot,
		workspace:        absWorkspace,
		bookState:        state,
		bookService:      book.NewService(absWorkspace),
		interactive:      interactiveStore,
		sessionStore:     store,
		session:          sess,
		versionService:   book.NewVersionService(absWorkspace, layout.VersionRepositoryDir()),
	}
	keepStore = true
	return runtime, nil
}

func projectSessionConversation(runtime ideChatRuntime, request agentchat.ChatRequest) *agentconversation.SessionConversation {
	return appconversation.ProjectConversation(sharedConversationRuntime(runtime), request)
}

func (a *App) resolveProject(id string, requireAvailable bool) (projectdomain.Record, projectdomain.Layout, error) {
	if a == nil || a.projectRegistry == nil {
		return projectdomain.Record{}, projectdomain.Layout{}, fmt.Errorf("project registry is unavailable")
	}
	return a.projectRegistry.Resolve(id, requireAvailable)
}

func (a *App) resolveProjectByWorkspace(workspace string) (projectdomain.Record, projectdomain.Layout, error) {
	if a == nil || a.projectRegistry == nil {
		return projectdomain.Record{}, projectdomain.Layout{}, fmt.Errorf("project registry is unavailable")
	}
	return a.projectRegistry.ResolveByPath(workspace, true)
}

func (a *App) projectLayoutForWorkspace(workspace string) (projectdomain.Layout, error) {
	_, layout, err := a.resolveProjectByWorkspace(workspace)
	return layout, err
}
