package api

import (
	"log"
	"os"
	"path/filepath"

	hertzapp "github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
)

// registerRoutes 注册 HTTP API 和静态文件路由。
func (s *Server) registerRoutes(h *hertzserver.Hertz) {
	api := h.Group("/api")
	{
		api.GET("/workspace/tree", s.handleWorkspaceTree)
		api.GET("/workspace/summary", s.handleWorkspaceSummary)
		api.GET("/workspace/file", s.handleWorkspaceFile)
		api.GET("/workspace/search", s.handleWorkspaceSearch)
		api.POST("/workspace/file", s.handleWorkspaceFileWrite)
		api.POST("/workspace/create", s.handleWorkspaceCreate)
		api.POST("/workspace/delete", s.handleWorkspaceDelete)
		api.POST("/workspace/rename", s.handleWorkspaceRename)
		api.POST("/workspace/copy", s.handleWorkspaceCopy)
		api.POST("/workspace/move", s.handleWorkspaceMove)
		api.POST("/workspace/import-character-card/preview", s.handleWorkspacePreviewCharacterCard)
		api.POST("/workspace/import-character-card", s.handleWorkspaceImportCharacterCard)
		api.POST("/workspace/switch", s.handleWorkspaceSwitch)
		api.GET("/workspace/current", s.handleWorkspaceCurrent)
		api.GET("/books", s.handleBooks)
		api.POST("/books/create", s.handleCreateBook)
		api.POST("/books/remove", s.handleBookRemove)
		api.GET("/books/info", s.handleBookInfo)
		api.PUT("/books/info", s.handleUpdateBookInfo)
		api.GET("/lore/items", s.handleLoreItems)
		api.POST("/lore/items", s.handleLoreItemCreate)
		api.PATCH("/lore/items/:id", s.handleLoreItemUpdate)
		api.DELETE("/lore/items/:id", s.handleLoreItemDelete)
		api.POST("/lore/agent", s.handleLoreAgent)
		api.POST("/lore/agent/stream", s.handleLoreAgentStream)
		api.GET("/lore/agent/messages", s.handleLoreAgentMessages)
		api.POST("/lore/agent/clear", s.handleLoreAgentClear)
		api.GET("/lore/versions", s.handleLoreVersions)
		api.POST("/lore/versions", s.handleLoreVersionCreate)
		api.POST("/lore/versions/:id/restore", s.handleLoreVersionRestore)
		api.GET("/styles", s.handleStyles)
		api.GET("/interactive/stories", s.handleInteractiveStories)
		api.POST("/interactive/stories", s.handleInteractiveStoryCreate)
		api.PATCH("/interactive/stories/:id", s.handleInteractiveStoryUpdate)
		api.DELETE("/interactive/stories/:id", s.handleInteractiveStoryDelete)
		api.GET("/interactive/stories/:id/snapshot", s.handleInteractiveSnapshot)
		api.GET("/interactive/stories/:id/branches", s.handleInteractiveBranches)
		api.POST("/interactive/stories/:id/branches", s.handleInteractiveBranchCreate)
		api.DELETE("/interactive/stories/:id/branches/:branch", s.handleInteractiveBranchDelete)
		api.POST("/interactive/stories/:id/switch-branch", s.handleInteractiveBranchSwitch)
		api.POST("/interactive/stories/:id/switch-turn-version", s.handleInteractiveTurnVersionSwitch)
		api.POST("/interactive/stories/:id/hot-choices", s.handleInteractiveHotChoices)
		api.GET("/interactive/tellers", s.handleInteractiveTellers)
		api.POST("/interactive/tellers", s.handleInteractiveTellerCreate)
		api.POST("/interactive/tellers/agent/stream", s.handleInteractiveTellerAgentStream)
		api.GET("/interactive/tellers/agent/messages", s.handleInteractiveTellerAgentMessages)
		api.POST("/interactive/tellers/agent/clear", s.handleInteractiveTellerAgentClear)
		api.GET("/interactive/tellers/:id", s.handleInteractiveTeller)
		api.PATCH("/interactive/tellers/:id", s.handleInteractiveTellerUpdate)
		api.DELETE("/interactive/tellers/:id", s.handleInteractiveTellerDelete)
		api.POST("/interactive/chat", s.handleInteractiveChat)
		api.POST("/interactive/chat/abort", s.handleInteractiveChatAbort)
		api.POST("/chat", s.handleChat)
		api.GET("/chat/stream", s.handleChatStream)
		api.GET("/chat/active", s.handleChatActive)
		api.POST("/chat/abort", s.handleChatAbort)
		api.GET("/git/status", s.handleGitStatus)
		api.GET("/git/history", s.handleGitHistory)
		api.GET("/git/diff", s.handleGitDiff)
		api.POST("/git/init", s.handleGitInit)
		api.POST("/git/commit", s.handleGitCommit)
		api.POST("/git/rollback", s.handleGitRollback)
		api.POST("/git/stash", s.handleGitStash)
		api.POST("/git/stash/pop", s.handleGitStashPop)
		api.POST("/git/command", s.handleGitCommand)
		api.POST("/command", s.handleCommand)
		api.GET("/session/messages", s.handleSessionMessages)
		api.GET("/sessions", s.handleSessions)
		api.POST("/sessions", s.handleSessionCreate)
		api.POST("/sessions/switch", s.handleSessionSwitch)
		api.POST("/sessions/rename", s.handleSessionRename)
		api.POST("/sessions/delete", s.handleSessionDelete)
		api.GET("/settings", s.handleSettingsGet)
		api.PUT("/settings/user", s.handleSettingsUserUpdate)
		api.PUT("/settings/workspace", s.handleSettingsWorkspaceUpdate)
		api.GET("/status", s.handleStatus)
	}

	if webRoot := resolveWebRoot(); webRoot != "" {
		log.Printf("[startup] Web 静态资源目录: %s", webRoot)
		h.StaticFS("/", &hertzapp.FS{Root: webRoot, IndexNames: []string{"index.html"}})
	} else {
		log.Printf("[startup] 未找到 Web 静态资源目录，仅注册 API 路由")
	}
}

func resolveWebRoot() string {
	candidates := []string{}
	if v := os.Getenv("NOVA_WEB_DIR"); v != "" {
		candidates = append(candidates, v)
	}
	candidates = append(candidates, "web")
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "web"),
			filepath.Join(exeDir, "..", "web"),
			filepath.Join(exeDir, "..", "..", "web"),
		)
	}
	for _, candidate := range candidates {
		root := normalizeStaticRoot(candidate)
		if root == "" {
			continue
		}
		if fi, err := os.Stat(root); err == nil && fi.IsDir() {
			if _, err := os.Stat(filepath.Join(root, "index.html")); err == nil {
				return root
			}
		}
	}
	return ""
}

func normalizeStaticRoot(root string) string {
	if root == "" {
		return ""
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return filepath.Clean(root)
}
