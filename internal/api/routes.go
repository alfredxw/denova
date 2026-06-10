package api

import (
	"log"
	"os"
	"path/filepath"

	hertzapp "github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"

	"nova/internal/api/handlers"
)

// registerRoutes 注册 HTTP API 和静态文件路由。
func (s *Server) registerRoutes(h *hertzserver.Hertz) {
	apiHandlers := handlers.New(s.app)
	api := h.Group("/api")
	{
		api.GET("/workspace/tree", apiHandlers.HandleWorkspaceTree)
		api.GET("/workspace/summary", apiHandlers.HandleWorkspaceSummary)
		api.GET("/workspace/file", apiHandlers.HandleWorkspaceFile)
		api.GET("/workspace/search", apiHandlers.HandleWorkspaceSearch)
		api.POST("/workspace/file", apiHandlers.HandleWorkspaceFileWrite)
		api.POST("/workspace/create", apiHandlers.HandleWorkspaceCreate)
		api.POST("/workspace/delete", apiHandlers.HandleWorkspaceDelete)
		api.POST("/workspace/rename", apiHandlers.HandleWorkspaceRename)
		api.POST("/workspace/copy", apiHandlers.HandleWorkspaceCopy)
		api.POST("/workspace/move", apiHandlers.HandleWorkspaceMove)
		api.POST("/workspace/import-character-card/preview", apiHandlers.HandleWorkspacePreviewCharacterCard)
		api.POST("/workspace/import-character-card", apiHandlers.HandleWorkspaceImportCharacterCard)
		api.POST("/workspace/switch", apiHandlers.HandleWorkspaceSwitch)
		api.GET("/workspace/current", apiHandlers.HandleWorkspaceCurrent)
		api.GET("/books", apiHandlers.HandleBooks)
		api.POST("/books/create", apiHandlers.HandleCreateBook)
		api.POST("/books/import-novel/preview", apiHandlers.HandlePreviewNovelImport)
		api.POST("/books/import-novel/preview/stream", apiHandlers.HandlePreviewNovelImportStream)
		api.POST("/books/import-novel", apiHandlers.HandleNovelImport)
		api.POST("/books/remove", apiHandlers.HandleBookRemove)
		api.GET("/books/info", apiHandlers.HandleBookInfo)
		api.PUT("/books/info", apiHandlers.HandleUpdateBookInfo)
		api.GET("/lore/items", apiHandlers.HandleLoreItems)
		api.POST("/lore/items", apiHandlers.HandleLoreItemCreate)
		api.PATCH("/lore/items/:id", apiHandlers.HandleLoreItemUpdate)
		api.DELETE("/lore/items/:id", apiHandlers.HandleLoreItemDelete)
		api.POST("/lore/agent", apiHandlers.HandleLoreAgent)
		api.POST("/lore/agent/stream", apiHandlers.HandleLoreAgentStream)
		api.GET("/lore/agent/messages", apiHandlers.HandleLoreAgentMessages)
		api.POST("/lore/agent/clear", apiHandlers.HandleLoreAgentClear)
		api.GET("/lore/versions", apiHandlers.HandleLoreVersions)
		api.POST("/lore/versions", apiHandlers.HandleLoreVersionCreate)
		api.POST("/lore/versions/:id/restore", apiHandlers.HandleLoreVersionRestore)
		api.GET("/styles", apiHandlers.HandleStyles)
		api.GET("/interactive/stories", apiHandlers.HandleInteractiveStories)
		api.POST("/interactive/stories", apiHandlers.HandleInteractiveStoryCreate)
		api.PATCH("/interactive/stories/:id", apiHandlers.HandleInteractiveStoryUpdate)
		api.DELETE("/interactive/stories/:id", apiHandlers.HandleInteractiveStoryDelete)
		api.GET("/interactive/stories/:id/snapshot", apiHandlers.HandleInteractiveSnapshot)
		api.GET("/interactive/stories/:id/branches", apiHandlers.HandleInteractiveBranches)
		api.POST("/interactive/stories/:id/branches", apiHandlers.HandleInteractiveBranchCreate)
		api.DELETE("/interactive/stories/:id/branches/:branch", apiHandlers.HandleInteractiveBranchDelete)
		api.POST("/interactive/stories/:id/switch-branch", apiHandlers.HandleInteractiveBranchSwitch)
		api.POST("/interactive/stories/:id/switch-turn-version", apiHandlers.HandleInteractiveTurnVersionSwitch)
		api.POST("/interactive/stories/:id/hot-choices", apiHandlers.HandleInteractiveHotChoices)
		api.GET("/interactive/tellers", apiHandlers.HandleInteractiveTellers)
		api.POST("/interactive/tellers", apiHandlers.HandleInteractiveTellerCreate)
		api.POST("/interactive/tellers/agent/stream", apiHandlers.HandleInteractiveTellerAgentStream)
		api.GET("/interactive/tellers/agent/messages", apiHandlers.HandleInteractiveTellerAgentMessages)
		api.POST("/interactive/tellers/agent/clear", apiHandlers.HandleInteractiveTellerAgentClear)
		api.GET("/interactive/tellers/:id", apiHandlers.HandleInteractiveTeller)
		api.PATCH("/interactive/tellers/:id", apiHandlers.HandleInteractiveTellerUpdate)
		api.DELETE("/interactive/tellers/:id", apiHandlers.HandleInteractiveTellerDelete)
		api.POST("/interactive/chat", apiHandlers.HandleInteractiveChat)
		api.POST("/interactive/chat/abort", apiHandlers.HandleInteractiveChatAbort)
		api.POST("/chat", apiHandlers.HandleChat)
		api.GET("/chat/stream", apiHandlers.HandleChatStream)
		api.GET("/chat/active", apiHandlers.HandleChatActive)
		api.POST("/chat/abort", apiHandlers.HandleChatAbort)
		api.GET("/agents/:agent/session/messages", apiHandlers.HandleAgentSessionMessages)
		api.POST("/agents/:agent/session/clear", apiHandlers.HandleAgentSessionClear)
		api.GET("/versions/status", apiHandlers.HandleVersionStatus)
		api.GET("/versions", apiHandlers.HandleVersionHistory)
		api.POST("/versions", apiHandlers.HandleVersionCreate)
		api.GET("/versions/:id/diff", apiHandlers.HandleVersionDiff)
		api.POST("/versions/:id/restore", apiHandlers.HandleVersionRestore)
		api.POST("/command", apiHandlers.HandleCommand)
		api.GET("/session/messages", apiHandlers.HandleSessionMessages)
		api.GET("/sessions", apiHandlers.HandleSessions)
		api.POST("/sessions", apiHandlers.HandleSessionCreate)
		api.POST("/sessions/switch", apiHandlers.HandleSessionSwitch)
		api.POST("/sessions/rename", apiHandlers.HandleSessionRename)
		api.POST("/sessions/delete", apiHandlers.HandleSessionDelete)
		api.GET("/settings", apiHandlers.HandleSettingsGet)
		api.PUT("/settings/user", apiHandlers.HandleSettingsUserUpdate)
		api.PUT("/settings/workspace", apiHandlers.HandleSettingsWorkspaceUpdate)
		api.GET("/status", apiHandlers.HandleStatus)
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
