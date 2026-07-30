package app

import (
	"sync"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/book"
)

// ChatAppService 负责普通创作 Agent 任务与会话管理。
type ChatAppService struct {
	app       *App
	admission sync.RWMutex
	starts    writingStartRegistry

	// recoveryRefreshPending is process-local because a restarted process loads
	// a fresh canonical Session projection. Within one process it must outlive
	// the display Task that discovered the durable structural recovery commit.
	recoveryRefreshMu      sync.Mutex
	recoveryRefreshPending map[string]agents.RuntimeRecoveryAction
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
	chatService    *agents.ChatService
	workspace      string
	versionService *book.VersionService
	cfg            config.Config
	ideTeller      agents.IDEStoryTeller
}
