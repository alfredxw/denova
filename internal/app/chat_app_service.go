package app

import (
	"sync"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/session"
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
	recoveryRefreshPending map[string]agent.RuntimeRecoveryAction
}

type ideChatRuntime struct {
	app            *App
	sess           *session.Session
	state          *book.State
	bookService    *book.Service
	chatService    *agent.ChatService
	workspace      string
	versionService *book.VersionService
	cfg            config.Config
	ideTeller      agent.IDEStoryTeller
}
