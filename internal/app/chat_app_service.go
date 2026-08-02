package app

import (
	agentharness "denova/internal/agents/harness"
	"sync"

	"denova/config"
	"denova/internal/agents/prompts"
	"denova/internal/agents/session"
	conversationapp "denova/internal/app/conversation"
	apptask "denova/internal/app/task"
	"denova/internal/book"
)

// ChatAppService 负责普通创作 Agent 任务与会话管理。
type ChatAppService struct {
	app       *App
	admission sync.RWMutex
	starts    apptask.StartRegistry

	// recoveryRefreshPending is process-local because a restarted process loads
	// a fresh canonical Session projection. Within one process it must outlive
	// the display Task that discovered the durable structural recovery commit.
	recoveryRefreshMu      sync.Mutex
	recoveryRefreshPending map[string]agentharness.RuntimeRecoveryAction
}

type ideChatRuntime struct {
	app            *App
	projectID      string
	projectType    ProjectType
	projectState   string
	agentKind      string
	sess           *session.Session
	state          *book.State
	bookService    *book.Service
	chatService    *agentharness.Service
	workspace      string
	versionService *book.VersionService
	cfg            config.Config
	ideTeller      prompts.IDEStoryTeller
}

func sharedConversationRuntime(runtime ideChatRuntime) conversationapp.Runtime {
	return conversationapp.Runtime{
		ProjectID: runtime.projectID, ProjectType: runtime.projectType, ProjectState: runtime.projectState,
		AgentKind: runtime.agentKind, Session: runtime.sess, State: runtime.state,
		BookService: runtime.bookService, ChatService: runtime.chatService, Workspace: runtime.workspace,
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
	runtime.chatService = shared.ChatService
	runtime.workspace = shared.Workspace
	runtime.versionService = shared.VersionService
	runtime.cfg = shared.Config
	runtime.ideTeller = shared.IDETeller
	return runtime
}
