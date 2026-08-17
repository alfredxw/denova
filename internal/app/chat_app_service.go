package app

import (
	agentexecution "denova/internal/agents/execution"
	"strings"
	"sync"

	"denova/config"
	"denova/internal/agents/prompts"
	"denova/internal/agents/session"
	conversationapp "denova/internal/app/conversation"
	apptask "denova/internal/app/task"
	"denova/internal/book"
)

// confirmSelectedSessionID rejects requests whose explicit browser binding no
// longer matches the foreground Writing session. Callers that must linearize
// against session switches also hold the Chat admission lock.
func (s *ChatAppService) confirmSelectedSessionID(expectedSessionID string) error {
	if s == nil || s.app == nil {
		return ErrNoWorkspace
	}
	expectedSessionID = strings.TrimSpace(expectedSessionID)
	if expectedSessionID == "" {
		return ErrInvalidAgentBinding
	}
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.workspaceTransition {
		return ErrWorkspaceTransition
	}
	if a.session == nil {
		return ErrNoWorkspace
	}
	if strings.TrimSpace(a.session.ID) != expectedSessionID {
		return ErrAgentContextChanged
	}
	return nil
}

// ChatAppService 负责普通创作 Agent 任务与会话管理。
type ChatAppService struct {
	app       *App
	admission sync.RWMutex
	starts    apptask.StartRegistry
}

type ideChatRuntime struct {
	app              *App
	projectID        string
	projectType      ProjectType
	projectState     string
	agentKind        string
	sess             *session.Session
	state            *book.State
	bookService      *book.Service
	executionRuntime *agentexecution.Runtime
	workspace        string
	versionService   *book.VersionService
	cfg              config.Config
	ideTeller        prompts.IDEStoryTeller
}

func sharedConversationRuntime(runtime ideChatRuntime) conversationapp.Runtime {
	return conversationapp.Runtime{
		ProjectID: runtime.projectID, ProjectType: runtime.projectType, ProjectState: runtime.projectState,
		AgentKind: runtime.agentKind, Session: runtime.sess, State: runtime.state,
		BookService: runtime.bookService, ExecutionRuntime: runtime.executionRuntime, Workspace: runtime.workspace,
		VersionService: runtime.versionService, Config: runtime.cfg, IDETeller: runtime.ideTeller,
	}
}

func applySharedConversationRuntime(runtime ideChatRuntime, shared conversationapp.Runtime) ideChatRuntime {
	runtime.projectID = shared.ProjectID
	runtime.projectType = shared.ProjectType
	runtime.projectState = shared.ProjectState
	runtime.agentKind = shared.AgentKind
	runtime.sess = shared.Session
	runtime.state = shared.State
	runtime.bookService = shared.BookService
	runtime.executionRuntime = shared.ExecutionRuntime
	runtime.workspace = shared.Workspace
	runtime.versionService = shared.VersionService
	runtime.cfg = shared.Config
	runtime.ideTeller = shared.IDETeller
	return runtime
}
