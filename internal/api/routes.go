package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	hertzapp "github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/handlers"
	"denova/internal/webfs"
)

// registerRoutes 注册 HTTP API 和静态文件路由。
func (s *Server) registerRoutes(h *hertzserver.Hertz) {
	apiHandlers := handlers.New(s.app)
	api := h.Group("/api")
	{
		api.POST("/autosave-conflicts", apiHandlers.HandleAutosaveConflictCreate)
		projects := api.Group("/projects/:project_id", apiHandlers.ProjectScopeMiddleware)
		projects.POST("/files/resolve", apiHandlers.HandleProjectFileTreeResolve)
		projects.GET("/files/file", apiHandlers.HandleProjectFileRead)
		projects.GET("/files/asset", apiHandlers.HandleProjectFileAsset)
		projects.PUT("/files/file", apiHandlers.HandleProjectFileSave)
		projects.POST("/files/operations", apiHandlers.HandleProjectFileOperations)
		projects.GET("/book", apiHandlers.HandleProjectBookSnapshot)
		projects.GET("/book/tree", apiHandlers.HandleProjectBookTree)
		projects.GET("/book/summary", apiHandlers.HandleProjectBookSummary)
		projects.PATCH("/book/chapter-status", apiHandlers.HandleProjectBookChapterStatus)
		projects.POST("/book/import-character-card", apiHandlers.HandleProjectCharacterCardImport)
		projects.GET("/book/lore/items", apiHandlers.HandleProjectLoreItems)
		projects.POST("/book/lore/items", apiHandlers.HandleProjectLoreItemCreate)
		projects.PUT("/book/lore/items/:id", apiHandlers.HandleProjectLoreItemUpdate)
		projects.DELETE("/book/lore/items/:id", apiHandlers.HandleProjectLoreItemDelete)
		projects.POST("/book/lore/classification/preview", apiHandlers.HandleLoreClassificationPreview)
		projects.POST("/book/lore/classification/apply", apiHandlers.HandleLoreClassificationApply)
		projects.POST("/book/lore/items/:id/image/generate", apiHandlers.HandleLoreItemImageGenerate)
		projects.DELETE("/book/lore/items/:id/image", apiHandlers.HandleLoreItemImageDelete)
		projects.POST("/book/lore/images/generate/stream", apiHandlers.HandleLoreImagesGenerateStream)
		projects.POST("/book/lore/images/generate/abort", apiHandlers.HandleLoreImagesGenerateAbort)
		projects.GET("/book/document-review", apiHandlers.HandleProjectDocumentReview)
		projects.POST("/book/document-comments", apiHandlers.HandleProjectDocumentCommentCreate)
		projects.PATCH("/book/document-comments/:id", apiHandlers.HandleProjectDocumentCommentUpdate)
		projects.DELETE("/book/document-comments/:id", apiHandlers.HandleProjectDocumentCommentDelete)
		projects.GET("/changes/groups", apiHandlers.HandleWorkspaceChangeGroups)
		projects.GET("/changes/groups/:id", apiHandlers.HandleWorkspaceChangeGroup)
		projects.GET("/changes/review-threads/:id", apiHandlers.HandleWorkspaceChangeReviewThread)
		projects.POST("/changes/groups/:id/review", apiHandlers.HandleWorkspaceChangeReview)
		projects.POST("/changes/groups/:id/undo", apiHandlers.HandleWorkspaceChangeUndo)
		projects.POST("/changes/groups/:id/redo", apiHandlers.HandleWorkspaceChangeRedo)
		projects.POST("/changes/comments", apiHandlers.HandleWorkspaceChangeCommentCreate)
		projects.PATCH("/changes/comments/:id", apiHandlers.HandleWorkspaceChangeCommentUpdate)
		projects.DELETE("/changes/comments/:id", apiHandlers.HandleWorkspaceChangeCommentDelete)
		projects.GET("/events", apiHandlers.HandleProjectFileEvents)
		projects.GET("/skills", apiHandlers.HandleSkills)
		projects.GET("/skills/document", apiHandlers.HandleSkillDocument)
		projects.GET("/skills/file", apiHandlers.HandleSkillFileDocument)
		projects.POST("/skills", apiHandlers.HandleSkillCreate)
		projects.PUT("/skills/document", apiHandlers.HandleSkillSave)
		projects.PUT("/skills/file", apiHandlers.HandleSkillFileSave)
		projects.DELETE("/skills/document", apiHandlers.HandleSkillDelete)
		projects.POST("/skills/install/zip/preview", apiHandlers.HandleSkillInstallZipPreview)
		projects.POST("/skills/install/zip", apiHandlers.HandleSkillInstallZip)
		projects.POST("/skills/install/remote/preview", apiHandlers.HandleSkillInstallRemotePreview)
		projects.POST("/skills/install/remote", apiHandlers.HandleSkillInstallRemote)
		projects.POST("/skills/install/github/preview", apiHandlers.HandleSkillInstallGitHubPreview)
		projects.POST("/skills/install/github", apiHandlers.HandleSkillInstallGitHub)
		projects.GET("/settings", apiHandlers.HandleSettingsGet)
		projects.PATCH("/settings", apiHandlers.HandleSettingsPatch)
		projects.POST("/config-manager/stream", apiHandlers.HandleConfigManagerStream)
		projects.GET("/config-manager/stream", apiHandlers.HandleConfigManagerTaskStream)
		projects.GET("/config-manager/active", apiHandlers.HandleConfigManagerActive)
		projects.POST("/config-manager/recovery", apiHandlers.HandleConfigManagerRecovery)
		projects.GET("/config-manager/messages", apiHandlers.HandleConfigManagerMessages)
		projects.POST("/config-manager/clear", apiHandlers.HandleConfigManagerClear)
		projects.POST("/config-manager/asks/:ask_id/answer", apiHandlers.HandleConfigManagerAskAnswer)
		projects.POST("/config-manager/asks/:ask_id/cancel", apiHandlers.HandleConfigManagerAskCancel)
		projects.GET("/agent-runs", apiHandlers.HandleAgentRunTraces)
		projects.GET("/agent-runs/:id/export", apiHandlers.HandleAgentRunTraceExport)
		projects.GET("/agent-runs/:id", apiHandlers.HandleAgentRunTrace)
		projects.GET("/workspace/search", apiHandlers.HandleWorkspaceSearch)
		projects.POST("/workspace/replace", apiHandlers.HandleWorkspaceReplace)
		projects.GET("/versions/status", apiHandlers.HandleVersionStatus)
		projects.GET("/versions", apiHandlers.HandleVersionHistory)
		projects.POST("/versions", apiHandlers.HandleVersionCreate)
		projects.GET("/versions/:id/diff", apiHandlers.HandleVersionDiff)
		projects.POST("/versions/:id/restore-plan", apiHandlers.HandleVersionRestorePlan)
		projects.POST("/versions/:id/restore", apiHandlers.HandleVersionRestore)
		projects.POST("/agent-chat/sessions", apiHandlers.HandleAgentChatSessionCreate)
		projects.POST("/agent-chat/sessions/rename", apiHandlers.HandleAgentChatSessionRename)
		projects.POST("/agent-chat/sessions/delete", apiHandlers.HandleAgentChatSessionDelete)
		projects.POST("/agent-chat/chat", apiHandlers.HandleAgentChat)
		projects.GET("/agent-chat/chat/stream", apiHandlers.HandleAgentChatStream)
		projects.GET("/agent-chat/chat/active", apiHandlers.HandleAgentChatActive)
		projects.POST("/agent-chat/chat/commands", apiHandlers.HandleAgentChatCommand)
		projects.POST("/agent-chat/chat/recovery", apiHandlers.HandleAgentChatRecovery)
		projects.POST("/agent-chat/chat/context-analysis", apiHandlers.HandleAgentChatContextAnalysis)
		projects.GET("/agent-chat/session/messages", apiHandlers.HandleAgentChatMessages)
		projects.POST("/agent-chat/session/asks/:ask_id/answer", apiHandlers.HandleAgentChatAskAnswer)
		projects.POST("/agent-chat/session/asks/:ask_id/cancel", apiHandlers.HandleAgentChatAskCancel)
		projects.POST("/agent-chat/command", apiHandlers.HandleAgentChatSlashCommand)
		projects.POST("/terminal/sessions", apiHandlers.HandleTerminalSessionCreate)
		projects.GET("/conversation-config", apiHandlers.HandleConversationConfigGet)
		projects.PATCH("/conversation-config", apiHandlers.HandleConversationConfigPatch)
		api.POST("/imports/character-card/preview", apiHandlers.HandleCharacterCardPreview)
		api.POST("/books/import-character-card", apiHandlers.HandleNewBookCharacterCardImport)
		api.POST("/workspace/switch", apiHandlers.HandleWorkspaceSwitch)
		api.GET("/workspace/current", apiHandlers.HandleWorkspaceCurrent)
		api.GET("/books", apiHandlers.HandleBooks)
		api.POST("/books/create", apiHandlers.HandleCreateBook)
		api.GET("/books/cover", apiHandlers.HandleBookCover)
		api.POST("/books/cover/generate", apiHandlers.HandleBookCoverGenerate)
		api.POST("/books/cover/upload", apiHandlers.HandleBookCoverUpload)
		api.GET("/books/export", apiHandlers.HandleBookExport)
		api.POST("/books/import-novel/preview", apiHandlers.HandlePreviewNovelImport)
		api.POST("/books/import-novel/preview/stream", apiHandlers.HandlePreviewNovelImportStream)
		api.POST("/books/import-novel", apiHandlers.HandleNovelImport)
		api.POST("/books/remove", apiHandlers.HandleBookRemove)
		api.POST("/books/reorder", apiHandlers.HandleBookReorder)
		api.POST("/books/sort-mode", apiHandlers.HandleBookSortMode)
		api.GET("/books/info", apiHandlers.HandleBookInfo)
		api.PUT("/books/info", apiHandlers.HandleUpdateBookInfo)
		api.GET("/interactive/stories", apiHandlers.HandleInteractiveStories)
		api.POST("/interactive/stories", apiHandlers.HandleInteractiveStoryCreate)
		api.PATCH("/interactive/stories/:id", apiHandlers.HandleInteractiveStoryUpdate)
		api.POST("/interactive/stories/:id/select", apiHandlers.HandleInteractiveStorySelect)
		api.DELETE("/interactive/stories/:id", apiHandlers.HandleInteractiveStoryDelete)
		api.GET("/interactive/stories/:id/snapshot", apiHandlers.HandleInteractiveSnapshot)
		api.GET("/interactive/stories/:id/history", apiHandlers.HandleInteractiveHistory)
		api.POST("/interactive/stories/:id/rules/resolutions/:resolution_id/reroll", apiHandlers.HandleInteractiveRuleResolutionReroll)
		api.GET("/interactive/stories/:id/director", apiHandlers.HandleInteractiveDirector)
		api.GET("/interactive/stories/:id/director/status", apiHandlers.HandleInteractiveDirectorStatus)
		api.PATCH("/interactive/stories/:id/director", apiHandlers.HandleInteractiveDirectorUpdate)
		api.POST("/interactive/stories/:id/director/rebuild", apiHandlers.HandleInteractiveDirectorRebuild)
		api.POST("/interactive/stories/:id/director/run", apiHandlers.HandleInteractiveDirectorRun)
		api.POST("/interactive/stories/:id/director/context-analysis", apiHandlers.HandleInteractiveDirectorContextAnalysis)
		api.GET("/interactive/stories/:id/branches", apiHandlers.HandleInteractiveBranches)
		api.POST("/interactive/stories/:id/branches", apiHandlers.HandleInteractiveBranchCreate)
		api.DELETE("/interactive/stories/:id/branches/:branch", apiHandlers.HandleInteractiveBranchDelete)
		api.POST("/interactive/stories/:id/switch-branch", apiHandlers.HandleInteractiveBranchSwitch)
		api.POST("/interactive/stories/:id/switch-turn-version", apiHandlers.HandleInteractiveTurnVersionSwitch)
		api.PATCH("/interactive/stories/:id/turns/:turn_id/narrative", apiHandlers.HandleInteractiveTurnNarrativeUpdate)
		api.POST("/interactive/stories/:id/images/generate", apiHandlers.HandleInteractiveImageGenerate)
		api.POST("/interactive/stories/:id/context-compaction", apiHandlers.HandleInteractiveContextCompaction)
		api.DELETE("/interactive/stories/:id/context-compaction/active", apiHandlers.HandleInteractiveContextCompactionRemove)
		api.GET("/interactive/tellers", apiHandlers.HandleInteractiveTellers)
		api.POST("/interactive/tellers", apiHandlers.HandleInteractiveTellerCreate)
		api.GET("/interactive/tellers/:id", apiHandlers.HandleInteractiveTeller)
		api.PATCH("/interactive/tellers/:id", apiHandlers.HandleInteractiveTellerUpdate)
		api.DELETE("/interactive/tellers/:id", apiHandlers.HandleInteractiveTellerDelete)
		api.GET("/styles", apiHandlers.HandleStyleReferences)
		api.POST("/styles", apiHandlers.HandleStyleReferenceSave)
		api.GET("/styles/file", apiHandlers.HandleStyleReferenceFile)
		api.PUT("/styles/file", apiHandlers.HandleStyleReferenceFileUpdate)
		api.DELETE("/styles", apiHandlers.HandleStyleReferenceDelete)
		api.POST("/interactive/actor-traits/roll", apiHandlers.HandleInteractiveActorTraitRoll)
		api.POST("/interactive/chat", apiHandlers.HandleInteractiveChat)
		api.POST("/interactive/chat/commands", apiHandlers.HandleInteractiveChatCommand)
		api.POST("/interactive/chat/recovery", apiHandlers.HandleInteractiveChatRecovery)
		api.POST("/interactive/chat/context-analysis", apiHandlers.HandleInteractiveChatContextAnalysis)
		api.GET("/interactive/chat/stream", apiHandlers.HandleInteractiveChatStream)
		api.GET("/interactive/chat/active", apiHandlers.HandleInteractiveChatActive)
		api.POST("/chat", apiHandlers.HandleChat)
		api.POST("/chat/commands", apiHandlers.HandleChatCommand)
		api.POST("/chat/recovery", apiHandlers.HandleChatRecovery)
		api.POST("/chat/context-analysis", apiHandlers.HandleChatContextAnalysis)
		api.POST("/chat/context-compaction", apiHandlers.HandleChatContextCompaction)
		api.DELETE("/chat/context-compaction/active", apiHandlers.HandleChatContextCompactionRemove)
		api.GET("/chat/stream", apiHandlers.HandleChatStream)
		api.GET("/chat/active", apiHandlers.HandleChatActive)
		api.POST("/images/generate", apiHandlers.HandleImageGenerate)
		api.GET("/story-directors", apiHandlers.HandleStoryDirectors)
		api.POST("/story-directors", apiHandlers.HandleStoryDirectorCreate)
		api.GET("/story-directors/:id", apiHandlers.HandleStoryDirector)
		api.PATCH("/story-directors/:id", apiHandlers.HandleStoryDirectorUpdate)
		api.DELETE("/story-directors/:id", apiHandlers.HandleStoryDirectorDelete)
		api.GET("/event-packages", apiHandlers.HandleEventPackages)
		api.POST("/event-packages", apiHandlers.HandleEventPackageCreate)
		api.GET("/event-packages/:id", apiHandlers.HandleEventPackage)
		api.PATCH("/event-packages/:id", apiHandlers.HandleEventPackageUpdate)
		api.DELETE("/event-packages/:id", apiHandlers.HandleEventPackageDelete)
		api.GET("/rule-systems", apiHandlers.HandleRuleSystems)
		api.POST("/rule-systems", apiHandlers.HandleRuleSystemCreate)
		api.GET("/rule-systems/:id", apiHandlers.HandleRuleSystem)
		api.PATCH("/rule-systems/:id", apiHandlers.HandleRuleSystemUpdate)
		api.DELETE("/rule-systems/:id", apiHandlers.HandleRuleSystemDelete)
		api.GET("/actor-states", apiHandlers.HandleActorStates)
		api.POST("/actor-states", apiHandlers.HandleActorStateCreate)
		api.GET("/actor-states/:id", apiHandlers.HandleActorState)
		api.PATCH("/actor-states/:id", apiHandlers.HandleActorStateUpdate)
		api.DELETE("/actor-states/:id", apiHandlers.HandleActorStateDelete)
		api.GET("/image-presets", apiHandlers.HandleImagePresets)
		api.POST("/image-presets", apiHandlers.HandleImagePresetCreate)
		api.GET("/image-presets/:id", apiHandlers.HandleImagePreset)
		api.PATCH("/image-presets/:id", apiHandlers.HandleImagePresetUpdate)
		api.DELETE("/image-presets/:id", apiHandlers.HandleImagePresetDelete)
		api.GET("/activity/summary", apiHandlers.HandleActivitySummary)
		api.GET("/messages", apiHandlers.HandleMessages)
		api.POST("/messages/read-all", apiHandlers.HandleMessagesReadAll)
		api.POST("/messages/:id/read", apiHandlers.HandleMessageRead)
		api.GET("/agents/:agent/session/messages", apiHandlers.HandleAgentSessionMessages)
		api.POST("/agents/:agent/session/clear", apiHandlers.HandleAgentSessionClear)
		api.GET("/skills", apiHandlers.HandleSkills)
		api.GET("/skills/document", apiHandlers.HandleSkillDocument)
		api.GET("/skills/file", apiHandlers.HandleSkillFileDocument)
		api.POST("/skills", apiHandlers.HandleSkillCreate)
		api.PUT("/skills/document", apiHandlers.HandleSkillSave)
		api.PUT("/skills/file", apiHandlers.HandleSkillFileSave)
		api.DELETE("/skills/document", apiHandlers.HandleSkillDelete)
		api.POST("/skills/install/zip/preview", apiHandlers.HandleSkillInstallZipPreview)
		api.POST("/skills/install/zip", apiHandlers.HandleSkillInstallZip)
		api.POST("/skills/install/remote/preview", apiHandlers.HandleSkillInstallRemotePreview)
		api.POST("/skills/install/remote", apiHandlers.HandleSkillInstallRemote)
		api.POST("/skills/install/github/preview", apiHandlers.HandleSkillInstallGitHubPreview)
		api.POST("/skills/install/github", apiHandlers.HandleSkillInstallGitHub)
		api.GET("/automations", apiHandlers.HandleAutomations)
		api.GET("/automations/templates", apiHandlers.HandleAutomationTemplates)
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
		api.POST("/automations/:id/run/stream", apiHandlers.HandleAutomationRunStream)
		api.POST("/command", apiHandlers.HandleCommand)
		api.GET("/session/messages", apiHandlers.HandleSessionMessages)
		api.POST("/session/asks/:ask_id/answer", apiHandlers.HandleSessionAskAnswer)
		api.POST("/session/asks/:ask_id/cancel", apiHandlers.HandleSessionAskCancel)
		api.GET("/sessions", apiHandlers.HandleSessions)
		api.POST("/sessions", apiHandlers.HandleSessionCreate)
		api.POST("/sessions/switch", apiHandlers.HandleSessionSwitch)
		api.POST("/sessions/rename", apiHandlers.HandleSessionRename)
		api.POST("/sessions/delete", apiHandlers.HandleSessionDelete)
		api.GET("/settings", apiHandlers.HandleSettingsGet)
		api.PATCH("/settings", apiHandlers.HandleSettingsPatch)
		api.DELETE("/settings/agent-approval-rules/:id", apiHandlers.HandleAgentApprovalRuleDelete)
		api.GET("/models/catalog", apiHandlers.HandleModelCatalog)
		api.POST("/models/discover", apiHandlers.HandleModelList)
		api.POST("/models/ping", apiHandlers.HandleModelPing)
		api.GET("/conversation-config", apiHandlers.HandleConversationConfigGet)
		api.PATCH("/conversation-config", apiHandlers.HandleConversationConfigPatch)
		api.GET("/update/check", apiHandlers.HandleUpdateCheck)
		api.POST("/update/install", apiHandlers.HandleUpdateInstall)
		api.POST("/update/install/stream", apiHandlers.HandleUpdateInstallStream)
		api.POST("/update/apply", apiHandlers.HandleUpdateApply)
		api.POST("/host/dialogs/directory", localHostEffectMiddleware, apiHandlers.HandleDirectoryPicker)
		api.GET("/agent-chat/projects", apiHandlers.HandleAgentChatProjects)
		api.POST("/agent-chat/projects", apiHandlers.HandleAgentChatProjectCreate)
		api.POST("/agent-chat/projects/reorder", apiHandlers.HandleAgentChatProjectReorder)
		api.PATCH("/agent-chat/projects/:id", apiHandlers.HandleAgentChatProjectUpdate)
		api.DELETE("/agent-chat/projects/:id", apiHandlers.HandleAgentChatProjectArchive)
		api.GET("/agent-chat/history", apiHandlers.HandleAgentChatHistory)
		api.GET("/terminal/sessions", apiHandlers.HandleTerminalSessions)
		api.DELETE("/terminal/sessions/:id", apiHandlers.HandleTerminalSessionDelete)
		api.GET("/terminal/sessions/:id/attach", apiHandlers.HandleTerminalAttach)
		api.GET("/status", apiHandlers.HandleStatus)
	}

	if webRoot := resolveWebRoot(); webRoot != "" {
		slog.InfoContext(context.Background(), fmt.Sprintf("[startup] Web 静态资源目录: %s", webRoot))
		staticFS := &hertzapp.FS{Root: webRoot, IndexNames: []string{"index.html"}}
		if spaFallback := spaFallbackHandler(webRoot); spaFallback != nil {
			staticFS.PathNotFound = spaFallback
		}
		h.StaticFS("/", staticFS)
	} else {
		slog.InfoContext(context.Background(), "[startup] 未找到 Web 静态资源目录，仅注册 API 路由")
	}
}

// spaFallbackHandler serves index.html for unknown GET/HEAD paths so that
// client-side deep links and full-page reloads resolve to the SPA shell
// instead of Hertz's default "Cannot open requested path" 404. This matters
// most on phones, where refresh and "add to home screen" deep links land on
// arbitrary in-app paths. Real API requests are matched under the /api group
// before the static catch-all, so they are unaffected; a genuinely missing
// static asset simply gets the shell, matching standard SPA behaviour.
//
// Returns nil (keeping the default 404) if index.html cannot be read, which
// should not happen given resolveWebRoot already verified its presence.
func spaFallbackHandler(webRoot string) hertzapp.HandlerFunc {
	indexPath := filepath.Join(webRoot, "index.html")
	indexHTML, err := os.ReadFile(indexPath)
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[startup] 读取 index.html 失败，禁用 SPA 回退: %v", err))
		return nil
	}
	return func(ctx context.Context, c *hertzapp.RequestContext) {
		method := string(c.Request.Method())
		if method != "GET" && method != "HEAD" {
			c.SetStatusCode(consts.StatusNotFound)
			return
		}
		c.SetContentType("text/html; charset=utf-8")
		c.SetStatusCode(consts.StatusOK)
		c.SetBodyString(string(indexHTML))
	}
}

func resolveWebRoot() string {
	candidates := []string{}
	if v := os.Getenv("DENOVA_WEB_DIR"); v != "" {
		candidates = append(candidates, v)
	} else if v := os.Getenv("NOVA_WEB_DIR"); v != "" {
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
	// Last resort: assets embedded into the binary (build tag "embedweb").
	// Lets a bare nova binary serve the frontend with no web/ directory on
	// disk — useful for go install / single-binary distribution. Extracts to
	// a temp dir the file-based static handler can serve from.
	if webfs.HasEmbedded() {
		root, err := webfs.ExtractEmbedded()
		if err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[startup] 解压内嵌前端资源失败，仅注册 API 路由: %v", err))
			return ""
		}
		slog.InfoContext(context.Background(), fmt.Sprintf("[startup] 未找到磁盘 Web 目录，使用内嵌前端资源: %s", root))
		return root
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
