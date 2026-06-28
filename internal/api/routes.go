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
		api.PATCH("/workspace/chapter-status", apiHandlers.HandleWorkspaceChapterStatus)
		api.GET("/workspace/file", apiHandlers.HandleWorkspaceFile)
		api.GET("/workspace/asset", apiHandlers.HandleWorkspaceAsset)
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
		api.GET("/books/cover", apiHandlers.HandleBookCover)
		api.POST("/books/cover/generate", apiHandlers.HandleBookCoverGenerate)
		api.POST("/books/import-novel/preview", apiHandlers.HandlePreviewNovelImport)
		api.POST("/books/import-novel/preview/stream", apiHandlers.HandlePreviewNovelImportStream)
		api.POST("/books/import-novel", apiHandlers.HandleNovelImport)
		api.POST("/books/remove", apiHandlers.HandleBookRemove)
		api.POST("/books/reorder", apiHandlers.HandleBookReorder)
		api.GET("/books/info", apiHandlers.HandleBookInfo)
		api.PUT("/books/info", apiHandlers.HandleUpdateBookInfo)
		api.GET("/lore/items", apiHandlers.HandleLoreItems)
		api.POST("/lore/items", apiHandlers.HandleLoreItemCreate)
		api.PATCH("/lore/items/:id", apiHandlers.HandleLoreItemUpdate)
		api.DELETE("/lore/items/:id", apiHandlers.HandleLoreItemDelete)
		api.POST("/config-manager/stream", apiHandlers.HandleConfigManagerStream)
		api.GET("/config-manager/messages", apiHandlers.HandleConfigManagerMessages)
		api.POST("/config-manager/clear", apiHandlers.HandleConfigManagerClear)
		api.GET("/interactive/stories", apiHandlers.HandleInteractiveStories)
		api.POST("/interactive/stories", apiHandlers.HandleInteractiveStoryCreate)
		api.PATCH("/interactive/stories/:id", apiHandlers.HandleInteractiveStoryUpdate)
		api.DELETE("/interactive/stories/:id", apiHandlers.HandleInteractiveStoryDelete)
		api.GET("/interactive/stories/:id/snapshot", apiHandlers.HandleInteractiveSnapshot)
		api.GET("/interactive/stories/:id/memory", apiHandlers.HandleInteractiveMemory)
		api.POST("/interactive/stories/:id/memory", apiHandlers.HandleInteractiveMemoryCreate)
		api.PATCH("/interactive/stories/:id/memory/:memory_id", apiHandlers.HandleInteractiveMemoryUpdate)
		api.POST("/interactive/stories/:id/memory/:memory_id/archive", apiHandlers.HandleInteractiveMemoryArchive)
		api.GET("/interactive/stories/:id/story-memory", apiHandlers.HandleStoryMemory)
		api.PATCH("/interactive/stories/:id/story-memory/settings", apiHandlers.HandleStoryMemorySettingsUpdate)
		api.POST("/interactive/stories/:id/story-memory/structures", apiHandlers.HandleStoryMemoryStructureSave)
		api.PATCH("/interactive/stories/:id/story-memory/structures/:structure_id", apiHandlers.HandleStoryMemoryStructureSave)
		api.DELETE("/interactive/stories/:id/story-memory/structures/:structure_id", apiHandlers.HandleStoryMemoryStructureDelete)
		api.POST("/interactive/stories/:id/story-memory/records", apiHandlers.HandleStoryMemoryRecordSave)
		api.PATCH("/interactive/stories/:id/story-memory/records/:record_id", apiHandlers.HandleStoryMemoryRecordSave)
		api.POST("/interactive/stories/:id/story-memory/records/:record_id/archive", apiHandlers.HandleStoryMemoryRecordArchive)
		api.POST("/interactive/stories/:id/story-memory/generate", apiHandlers.HandleStoryMemoryGenerate)
		api.POST("/interactive/stories/:id/story-memory/generate/stream", apiHandlers.HandleStoryMemoryGenerateStream)
		api.GET("/interactive/stories/:id/branches", apiHandlers.HandleInteractiveBranches)
		api.POST("/interactive/stories/:id/branches", apiHandlers.HandleInteractiveBranchCreate)
		api.DELETE("/interactive/stories/:id/branches/:branch", apiHandlers.HandleInteractiveBranchDelete)
		api.POST("/interactive/stories/:id/switch-branch", apiHandlers.HandleInteractiveBranchSwitch)
		api.POST("/interactive/stories/:id/switch-turn-version", apiHandlers.HandleInteractiveTurnVersionSwitch)
		api.POST("/interactive/stories/:id/hot-choices", apiHandlers.HandleInteractiveHotChoices)
		api.POST("/interactive/stories/:id/images/generate", apiHandlers.HandleInteractiveImageGenerate)
		api.POST("/interactive/stories/:id/context-compaction", apiHandlers.HandleInteractiveContextCompaction)
		api.DELETE("/interactive/stories/:id/context-compaction/active", apiHandlers.HandleInteractiveContextCompactionRemove)
		api.GET("/interactive/tellers", apiHandlers.HandleInteractiveTellers)
		api.POST("/interactive/tellers", apiHandlers.HandleInteractiveTellerCreate)
		api.GET("/interactive/tellers/:id", apiHandlers.HandleInteractiveTeller)
		api.PATCH("/interactive/tellers/:id", apiHandlers.HandleInteractiveTellerUpdate)
		api.DELETE("/interactive/tellers/:id", apiHandlers.HandleInteractiveTellerDelete)
		api.POST("/interactive/chat", apiHandlers.HandleInteractiveChat)
		api.POST("/interactive/chat/context-analysis", apiHandlers.HandleInteractiveChatContextAnalysis)
		api.POST("/interactive/chat/abort", apiHandlers.HandleInteractiveChatAbort)
		api.POST("/chat", apiHandlers.HandleChat)
		api.POST("/chat/context-analysis", apiHandlers.HandleChatContextAnalysis)
		api.POST("/chat/context-compaction", apiHandlers.HandleChatContextCompaction)
		api.DELETE("/chat/context-compaction/active", apiHandlers.HandleChatContextCompactionRemove)
		api.GET("/chat/stream", apiHandlers.HandleChatStream)
		api.GET("/chat/active", apiHandlers.HandleChatActive)
		api.POST("/chat/abort", apiHandlers.HandleChatAbort)
		api.POST("/images/generate", apiHandlers.HandleImageGenerate)
		api.GET("/image-presets", apiHandlers.HandleImagePresets)
		api.POST("/image-presets", apiHandlers.HandleImagePresetCreate)
		api.GET("/image-presets/:id", apiHandlers.HandleImagePreset)
		api.PATCH("/image-presets/:id", apiHandlers.HandleImagePresetUpdate)
		api.DELETE("/image-presets/:id", apiHandlers.HandleImagePresetDelete)
		api.GET("/agent-runs", apiHandlers.HandleAgentRunTraces)
		api.GET("/agent-runs/:id", apiHandlers.HandleAgentRunTrace)
		api.GET("/agents/:agent/session/messages", apiHandlers.HandleAgentSessionMessages)
		api.POST("/agents/:agent/session/clear", apiHandlers.HandleAgentSessionClear)
		api.GET("/skills", apiHandlers.HandleSkills)
		api.GET("/skills/document", apiHandlers.HandleSkillDocument)
		api.POST("/skills", apiHandlers.HandleSkillCreate)
		api.PUT("/skills/document", apiHandlers.HandleSkillSave)
		api.DELETE("/skills/document", apiHandlers.HandleSkillDelete)
		api.GET("/automations", apiHandlers.HandleAutomations)
		api.POST("/automations", apiHandlers.HandleAutomationCreate)
		api.GET("/automations/inbox", apiHandlers.HandleAutomationInbox)
		api.POST("/automations/inbox/:item_id/confirm", apiHandlers.HandleAutomationInboxConfirm)
		api.POST("/automations/inbox/:item_id/dismiss", apiHandlers.HandleAutomationInboxDismiss)
		api.POST("/automations/inbox/:item_id/read", apiHandlers.HandleAutomationInboxRead)
		api.GET("/automations/runs/active", apiHandlers.HandleAutomationActiveRuns)
		api.GET("/automations/runs/:run_id/stream", apiHandlers.HandleAutomationRunStreamByID)
		api.POST("/automations/runs/:run_id/chat/stream", apiHandlers.HandleAutomationRunChatStream)
		api.POST("/automations/runs/:run_id/abort", apiHandlers.HandleAutomationRunAbort)
		api.GET("/automations/runs/:run_id/messages", apiHandlers.HandleAutomationRunMessages)
		api.PATCH("/automations/:id", apiHandlers.HandleAutomationUpdate)
		api.DELETE("/automations/:id", apiHandlers.HandleAutomationDelete)
		api.POST("/automations/:id/check", apiHandlers.HandleAutomationCheck)
		api.POST("/automations/:id/run", apiHandlers.HandleAutomationRun)
		api.POST("/automations/:id/run/stream", apiHandlers.HandleAutomationRunStream)
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
		api.GET("/update/check", apiHandlers.HandleUpdateCheck)
		api.POST("/update/install", apiHandlers.HandleUpdateInstall)
		api.POST("/update/install/stream", apiHandlers.HandleUpdateInstallStream)
		api.POST("/update/apply", apiHandlers.HandleUpdateApply)
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
