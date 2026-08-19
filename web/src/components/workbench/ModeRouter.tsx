import { lazy, memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { WRITING_COMPOSER_SETTING_DEFAULTS } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/MarkdownEditor'
import type { OutlineRevealRequest } from '@/components/workbench/outline/ChapterOutline'
import { getImagePresets, getInteractiveTellers } from '@/features/interactive/api'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import type { ChapterIllustration, ChapterSummary, DocumentPreview, WorkspaceSearchResult } from '@/lib/api'
import { GLOBAL_RESOURCE_TARGET, projectResourceTarget } from '@/lib/api'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { useLayeredSettingsDraft } from '@/features/settings/use-layered-settings-draft'
import { GLOBAL_SETTINGS_TARGET } from '@/features/settings/api'
import { applyReadingTypographySettings } from '@/features/settings/font-variables'
import { workspaceFileKind } from '@/lib/workspace-file-kind'
import { isLoreItemsPath } from '@/lib/workspace-path'
import { useWritingChangeReview } from '@/features/changes/use-writing-change-review'
import type { ReviewFeedbackBatch, ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import { useDocumentReview } from '@/features/document-review/use-document-review'
import { loreImportanceLabel, loreLoadModeLabel, loreTypeLabel } from '@/features/lore/options'
import type { AgentChatProjectNavigationState } from '@/features/agent-chat/AgentChatProjectSwitcher'
import { tabKey } from './TabController'
import { WorkbenchShell } from './WorkbenchShell'
import { useWorkbenchRouteHost, WorkbenchRouteLayer } from './WorkbenchRouteHost'
import { flattenFileTree } from './workbench-utils'
import { useReviewFeedbackNavigation } from './use-review-feedback-navigation'
import { WritingSidebar } from './WritingSidebar'
import { IdeWritingInfoActions, WritingMainRoute } from './WritingMainRoute'
import { useWritingAgentPanel } from './use-writing-agent-panel'
import type { ModeRouterProps } from './ModeRouter.types'
import { SharedWorkbenchRoutes } from './SharedWorkbenchRoutes'
import { AgentChatWorkbenchRoute } from './AgentChatWorkbenchRoute'
import { ToolNavigationProvider, type ToolNavigationIntent, type ToolNavigationTarget } from '@/components/Chat/tool-navigation'
import { requestAutomationNavigation } from '@/features/automations/automation-navigation'

const WRITING_AGENT_INIT_EVENT = 'nova:writing-agent-init'
const InteractiveLayout = memo(lazy(() => import('@/features/interactive/components/InteractiveLayout').then((module) => ({ default: module.InteractiveLayout }))))
const SettingPanel = memo(lazy(() => import('@/features/interactive/components/SettingPanel').then((module) => ({ default: module.SettingPanel }))))
const VersionPanel = memo(lazy(() => import('@/components/Versions/VersionPanel').then((module) => ({ default: module.VersionPanel }))))
const EMPTY_CHAPTERS: ChapterSummary[] = []
const EMPTY_CHAPTER_PLANS: DocumentPreview[] = []
const EXTERNAL_CONTENT_CHANGE = { impact: 'content', origin: 'external' } satisfies WorkspaceChangeMetadata
const EXTERNAL_STRUCTURE_CHANGE = { impact: 'structure', origin: 'external' } satisfies WorkspaceChangeMetadata
function normalizeReadingFontSize(value: unknown) {
  const parsed = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(parsed)) return 18
  return Math.min(28, Math.max(14, Math.round(parsed)))
}

export function ModeRouter(props: ModeRouterProps) {
  const { t, i18n } = useTranslation()
  const {
    mode,
    booksReturnMode,
    currentBookName,
    workspace,
    projectId,
    appVersion,
    summary,
    currentChapter,
    chapterStats,
    isStreaming,
    sessionTransitionPending = false,
    isExecutionActive,
    runtimeProjection,
    abortPending,
    commandSubmitting,
    queueActionPendingCommandID,
    projectVisible,
    activityBarExpanded,
    rightPanel,
    settingsOpen,
    developerMode = false,
    interactiveRightVisible,
    novaDir,
    books,
    bookSortMode,
    tree,
    loading,
    selectedFile,
    fileContent,
    fileRevision,
    openTabs,
    activeTabKey,
    sidebarView,
    editorSearchIntent,
    saveSignal,
    editorAutoSaveEnabled,
    editorAutoSaveDelayMs,
    projectExplorerRefreshSignal,
    versionRefreshSignal,
    messages,
    sessions,
    activeSessionId,
    activityContent,
    references,
    loreReferences,
    loreItems,
    styleScenes,
    textSelections,
    chatPlanMode,
    hasEarlierMessages,
    isLoadingEarlierHistory,
    notice,
    onSetMode,
    onToggleActivityBarExpanded,
    onToggleProjectVisible,
    onSetRightPanel,
    onToggleSettings,
    onCloseSettings,
    onToggleInteractiveRightPanel,
    onSwitchBook,
    onQuickSwitchBook,
    onBeforeWorkspaceSwitch,
    onBooksChange,
    onAgentChatBookCreated,
    onOpenCharacterCardImport,
    onSetSidebarView,
    onSelectSearchResult,
    onSelectFile,
    onSetChapterConfirmed,
    onReferenceFile,
    onCreateItem,
    onDeleteItem,
    onRenameItem,
    onCopyItem,
    onMoveItem,
    onRefreshWorkspace,
    onActivateTab,
    onCloseTab,
    onToggleTabPin,
    onMoveTab,
    onOpenLoreTab,
    onSaveCurrentFile,
    onEditorFlushHandlerChange,
    onWorkspaceChanged,
    onQuoteSelection,
    onCreateChatSession,
    onSwitchChatSession,
    onRenameChatSession,
    onDeleteChatSession,
    onLoadEarlierHistory,
    onRefreshChatHistory,
    onSend,
    onAnalyzeContext,
    onStop,
    onSteerQueuedCommand,
    onDeleteQueuedCommand,
    onEditQueuedCommand,
    onReferenceRemove,
    onLoreReferenceAdd,
    onLoreReferenceRemove,
    onStyleSceneAdd,
    onStyleSceneRemove,
    onTextSelectionRemove,
    onChatPlanModeChange,
    onChatPlanModeToggle,
    onApproveProposedPlan,
    onExitChatPlanMode,
    onDismissNotice,
  } = props
  const resourceTarget = useMemo(
    () => projectId.trim() ? projectResourceTarget(projectId) : GLOBAL_RESOURCE_TARGET,
    [projectId],
  )

  const notifyExternalContentChange = useCallback(
    (paths: string[]) => onWorkspaceChanged(paths, EXTERNAL_CONTENT_CHANGE),
    [onWorkspaceChanged],
  )
  const notifyExternalStructureChange = useCallback(
    (paths: string[]) => onWorkspaceChanged(paths, EXTERNAL_STRUCTURE_CHANGE),
    [onWorkspaceChanged],
  )

  const readingTypographyDraft = useLayeredSettingsDraft({
    target: GLOBAL_SETTINGS_TARGET,
    layer: 'user',
    sourcePrefix: 'editor-reading-typography',
  })
  const readingFontFamily = readingTypographyDraft.draft.reading_font_family?.trim()
    || readingTypographyDraft.layered?.effective.reading_font_family?.trim()
    || 'apple-system'
  const readingFontSize = normalizeReadingFontSize(
    readingTypographyDraft.draft.reading_font_size
      ?? readingTypographyDraft.layered?.effective.reading_font_size,
  )
  const updateReadingFontFamily = useCallback((fontFamily: string) => {
    applyReadingTypographySettings({ readingFont: fontFamily, readingFontSize })
    readingTypographyDraft.setDraft((current) => ({ ...current, reading_font_family: fontFamily }))
  }, [readingFontSize, readingTypographyDraft.setDraft])
  const updateReadingFontSize = useCallback((fontSize: number) => {
    const normalized = normalizeReadingFontSize(fontSize)
    applyReadingTypographySettings({ readingFont: readingFontFamily, readingFontSize: normalized })
    readingTypographyDraft.setDraft((current) => ({ ...current, reading_font_size: normalized }))
  }, [readingFontFamily, readingTypographyDraft.setDraft])
  const activeTab = openTabs.find((tab) => tabKey(tab) === activeTabKey) ?? null
  const activeFileKind = selectedFile ? workspaceFileKind(selectedFile) : null
  const ideContext = useMemo(() => ({
    currentFile: selectedFile || undefined,
    openFiles: openTabs.flatMap((tab) => tab.kind === 'file' ? [tab.path] : []),
  }), [openTabs, selectedFile])
  const versionsVisible = rightPanel === 'versions'
  const ideWorkspacePanel = mode === 'ide' && (rightPanel === 'lore' || rightPanel === 'teller') ? rightPanel : null
  const interactiveSubmode = useInteractiveStore((state) => state.submode)
  const setInteractiveSubmode = useInteractiveStore((state) => state.setSubmode)
  const [tellers, setTellers] = useState<Teller[]>([])
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>([])
  const [agentSubAgentDetailsOpen, setAgentSubAgentDetailsOpen] = useState(false)
  const [agentChatProjectNavigation, setAgentChatProjectNavigation] = useState<AgentChatProjectNavigationState | null>(null)
  const [illustrationInsertSignal, setIllustrationInsertSignal] = useState<{ illustration: ChapterIllustration; nonce: number } | null>(null)
  const [outlineRevealRequest, setOutlineRevealRequest] = useState<OutlineRevealRequest | null>(null)
  const [toolNavigationIntent, setToolNavigationIntent] = useState<ToolNavigationIntent | null>(null)
  const toolNavigationNonceRef = useRef(0)
  const loreLibraryFlushHandlerRef = useRef<EditorFlushHandler | null>(null)
  const agentChatFlushHandlerRef = useRef<EditorFlushHandler | null>(null)
  const [editorLine, setEditorLine] = useState(1)
  // The router is the lifecycle owner: the settings lane survives AgentPanel close/unmount.
  const composerSettings = usePersistedUserSettings({ workspace, defaults: WRITING_COMPOSER_SETTING_DEFAULTS })
  const flushComposerSettings = composerSettings.flushPending

  const flushComposerSettingsBestEffort = useCallback(() => {
    void flushComposerSettings().then((saved) => {
      if (saved) return
      toast.warning(t('common.autosave.preferencesPending'), {
        description: t('common.autosave.preferencesPendingDetail'),
      })
    }).catch((error) => {
      console.warn('[ModeRouter.tsx] preference autosave flush failed during navigation; pending edits remain owned', { error })
      toast.warning(t('common.autosave.preferencesPending'), {
        description: t('common.autosave.preferencesPendingDetail'),
      })
    })
  }, [flushComposerSettings, t])

  const flushLoreLibraryDraft = useCallback(async (): Promise<boolean> => {
    const handler = loreLibraryFlushHandlerRef.current
    return handler ? handler() : true
  }, [])
  const handleLoreLibraryFlushHandlerChange = useCallback((handler: EditorFlushHandler | null) => {
    loreLibraryFlushHandlerRef.current = handler
  }, [])
  const flushAgentChatDrafts = useCallback(async (): Promise<boolean> => {
    const handler = agentChatFlushHandlerRef.current
    return handler ? handler() : true
  }, [])
  const handleAgentChatFlushHandlerChange = useCallback((handler: EditorFlushHandler | null) => {
    agentChatFlushHandlerRef.current = handler
  }, [])

  const flushBeforeWorkspaceSwitch = useCallback(async (): Promise<boolean> => {
    flushComposerSettingsBestEffort()
    if (!(await flushLoreLibraryDraft())) return false
    if (!(await flushAgentChatDrafts())) return false
    return onBeforeWorkspaceSwitch()
  }, [flushAgentChatDrafts, flushComposerSettingsBestEffort, flushLoreLibraryDraft, onBeforeWorkspaceSwitch])

  const quickSwitchBook = useCallback(async (path: string): Promise<boolean> => {
    flushComposerSettingsBestEffort()
    if (!(await flushLoreLibraryDraft())) return false
    if (!(await flushAgentChatDrafts())) return false
    return onQuickSwitchBook(path)
  }, [flushAgentChatDrafts, flushComposerSettingsBestEffort, flushLoreLibraryDraft, onQuickSwitchBook])

  useEffect(() => {
    setEditorLine(1)
  }, [selectedFile])

  useEffect(() => {
    setOutlineRevealRequest(null)
  }, [workspace])

  useEffect(() => {
    let cancelled = false
    if (!workspace) {
      setTellers([])
      setImagePresets([])
      return () => { cancelled = true }
    }
    Promise.all([getInteractiveTellers(), getImagePresets()])
      .then(([nextTellers, nextImagePresets]) => {
        if (!cancelled) {
          setTellers(nextTellers)
          setImagePresets(nextImagePresets)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setTellers([])
          setImagePresets([])
        }
      })
    return () => { cancelled = true }
  }, [workspace])

  useEffect(() => {
    if (mode !== 'ide' || rightPanel !== 'ai') setAgentSubAgentDetailsOpen(false)
  }, [mode, rightPanel])

  const loreReferenceLabels = useMemo(() => Object.fromEntries(loreItems.map((item) => [item.id, item.name])), [loreItems])
  const loreSuggestions = useMemo(() => loreItems.map((item) => ({
    value: item.id,
    label: item.name,
    description: t('planning.loreDescription', {
      type: loreTypeLabel(item.type, t),
      importance: loreImportanceLabel(item.importance, t),
      loadMode: loreLoadModeLabel(item.load_mode, t),
      tags: item.tags?.length ? ` · ${item.tags.join(i18n.language.startsWith('zh') ? '、' : ', ')}` : '',
      brief: item.brief_description ? t('planning.loreBrief', { brief: item.brief_description }) : '',
    }),
  })), [i18n.language, loreItems, t])
  const loreEmpty = Boolean(workspace) && loreItems.length === 0
  const selectWorkspacePath = useCallback((path: string) => {
    if (isLoreItemsPath(path)) return onOpenLoreTab()
    return onSelectFile(path)
  }, [onOpenLoreTab, onSelectFile])
  const selectWorkspaceSearchResult = useCallback((result: WorkspaceSearchResult, query: string) => {
    if (isLoreItemsPath(result.path)) {
      void onOpenLoreTab()
      return
    }
    return onSelectSearchResult(result, query)
  }, [onOpenLoreTab, onSelectSearchResult])

  const openToolNavigationTarget = useCallback((target: ToolNavigationTarget) => {
    if (target.kind === 'workspace_file') {
      onSetMode('ide')
      if (rightPanel === 'lore' || rightPanel === 'teller' || rightPanel === 'versions') onSetRightPanel(null)
      void selectWorkspacePath(target.path)
      return
    }

    toolNavigationNonceRef.current += 1
    setToolNavigationIntent({ target, nonce: toolNavigationNonceRef.current })
    if (target.kind === 'lore_item') {
      if (mode === 'interactive') {
        setInteractiveSubmode('lore')
      } else {
        onSetMode('ide')
        void onOpenLoreTab()
      }
      return
    }

    switch (target.resource) {
      case 'skill':
        onSetMode('skills')
        return
      case 'agent_profile':
        onSetMode('agents')
        return
      case 'automation':
        if (target.id) requestAutomationNavigation({ taskId: target.id, projectId, workspace })
        onSetMode('automations')
        return
      case 'style_reference':
      case 'narrative_style':
      case 'story_director':
      case 'event_package':
      case 'rule_system':
      case 'state_system':
      case 'image_preset':
        if (mode === 'interactive') {
          setInteractiveSubmode('teller')
        } else {
          onSetMode('ide')
          onSetRightPanel('teller')
        }
        return
    }
  }, [mode, onOpenLoreTab, onSetMode, onSetRightPanel, projectId, rightPanel, selectWorkspacePath, setInteractiveSubmode, workspace])
  const toolNavigation = useMemo(() => ({ workspace, open: openToolNavigationTarget }), [openToolNavigationTarget, workspace])

  const requestLoreInit = useCallback(() => {
    onSetMode('interactive')
    setInteractiveSubmode('lore')
  }, [onSetMode, setInteractiveSubmode])
  const requestWritingInit = useCallback(() => {
    onSetMode('ide')
    onSetRightPanel('ai')
    window.setTimeout(() => {
      window.dispatchEvent(new CustomEvent(WRITING_AGENT_INIT_EVENT, {
        detail: { prompt: t('writingAgent.initPrompt') },
      }))
    }, 0)
  }, [onSetMode, onSetRightPanel, t])
  const requestSkillsAgent = useCallback((prompt: string) => {
    onSetMode('ide')
    onSetRightPanel('ai')
    window.setTimeout(() => {
      window.dispatchEvent(new CustomEvent(WRITING_AGENT_INIT_EVENT, {
        detail: { prompt },
      }))
    }, 0)
  }, [onSetMode, onSetRightPanel])
  const revealCurrentChapterInOutline = useCallback((path: string) => {
    if (!projectVisible) onToggleProjectVisible()
    onSetSidebarView('outline')
    setOutlineRevealRequest((current) => ({ path, nonce: (current?.nonce || 0) + 1 }))
  }, [onSetSidebarView, onToggleProjectVisible, projectVisible])
  const revealFileInProject = useCallback(async (path: string) => {
    if (!projectVisible) onToggleProjectVisible()
    onSetSidebarView('files')
    await Promise.resolve(selectWorkspacePath(path))
  }, [onSetSidebarView, onToggleProjectVisible, projectVisible, selectWorkspacePath])
  const requestChapterIllustration = useCallback((chapterPath: string) => {
    const target = currentChapter?.path || chapterPath || selectedFile || ''
    if (!target) return
    onSetMode('ide')
    onSetRightPanel('ai')
    window.setTimeout(() => {
      window.dispatchEvent(new CustomEvent(WRITING_AGENT_INIT_EVENT, {
        detail: {
          autoSend: true,
          prompt: [
            '/chapter-illustration',
            '',
            `目标章节 / Target chapter: ${target}`,
            '',
            '请基于这个章节生成一张非剧透插画。只生成图像和 meta.json，不要自动插入正文；生成后等待我手动点击“插入正文”。',
          ].join('\n'),
        },
      }))
    }, 0)
  }, [currentChapter?.path, onSetMode, onSetRightPanel, selectedFile])
  const insertIllustrationIntoEditor = useCallback((illustration: ChapterIllustration) => {
    const apply = () => {
      setIllustrationInsertSignal((current) => ({ illustration, nonce: (current?.nonce || 0) + 1 }))
    }
    if (illustration.chapter_path && selectedFile !== illustration.chapter_path) {
      void Promise.resolve(selectWorkspacePath(illustration.chapter_path)).then((navigated) => {
        if (navigated !== false) window.setTimeout(apply, 0)
      })
      return
    }
    apply()
  }, [selectWorkspacePath, selectedFile])
  const aiVisible = rightPanel === 'ai'
  const showAgent = useCallback(() => onSetRightPanel('ai'), [onSetRightPanel])
  const {
    activeReviewThreadID,
    activeReviewRequest,
    reviewFeedback: changeReviewFeedback,
    submittedReviewCommentIDs,
    openChangeReview,
    closeChangeReview,
    selectReviewFeedback,
    removeReviewFeedback,
    submitReviewFeedback,
    restoreReviewFeedback,
  } = useWritingChangeReview({
    workspace,
    contextKey: activeSessionId,
    hostActive: mode === 'ide' && !settingsOpen && !versionsVisible && !ideWorkspacePanel,
    selectedFile,
    agentVisible: aiVisible,
    onBeforeOpen: flushBeforeWorkspaceSwitch,
    onShowAgent: showAgent,
  })
  const documentReview = useDocumentReview({
    projectId,
    // AgentChat owns its own conversation surface. Creating a comment there must not reveal
    // the hidden foreground Writing Agent panel.
    agentVisible: aiVisible || mode === 'agentchat',
    onShowAgent: showAgent,
  })
  const reviewFeedback = useMemo<ReviewFeedbackBatch>(() => (
    [changeReviewFeedback, documentReview.feedback].filter((feedback): feedback is ReviewFeedbackSelection => Boolean(feedback))
  ), [changeReviewFeedback, documentReview.feedback])
  const documentReviewController = useMemo(() => ({
    comments: documentReview.visibleComments,
    onCreate: documentReview.addComment,
    onUpdate: documentReview.editComment,
    onDelete: documentReview.removeComment,
  }), [documentReview.addComment, documentReview.editComment, documentReview.removeComment, documentReview.visibleComments])
  const removeActiveReviewFeedback = useCallback((selection: ReviewFeedbackSelection, commentID: string) => {
    if (selection.source === 'document') documentReview.removeFeedback(commentID)
    else removeReviewFeedback(commentID)
  }, [documentReview.removeFeedback, removeReviewFeedback])
  const submitActiveReviewFeedback = useCallback((feedback: ReviewFeedbackBatch) => {
    for (const selection of feedback) {
      if (selection.source === 'document') documentReview.submitFeedback(selection)
      else submitReviewFeedback(selection)
    }
  }, [documentReview.submitFeedback, submitReviewFeedback])
  const restoreActiveReviewFeedback = useCallback((feedback: ReviewFeedbackBatch) => {
    for (const selection of feedback) {
      if (selection.source === 'document') documentReview.restoreFeedback(selection)
      else restoreReviewFeedback(selection)
    }
  }, [documentReview.restoreFeedback, restoreReviewFeedback])
  const {
    target: documentReviewNavigationTarget,
    open: openActiveReviewFeedback,
  } = useReviewFeedbackNavigation({
    workspace,
    selectedFile,
    onSelectFile: selectWorkspacePath,
    onOpenLoreTab,
    onOpenChangeReview: openChangeReview,
  })
  const reviewVisible = Boolean(activeReviewThreadID)
  const closeBooks = useCallback(() => {
    if (booksReturnMode === 'interactive') {
      onSetMode('interactive')
      return
    }
    onSetMode('ide')
    if (rightPanel === 'lore' || rightPanel === 'teller' || rightPanel === 'versions') onSetRightPanel(null)
  }, [booksReturnMode, onSetMode, onSetRightPanel, rightPanel])
  const chapters = summary?.chapters ?? EMPTY_CHAPTERS
  const fileSuggestions = useMemo(() => flattenFileTree(tree), [tree])
  const closeIdeWorkspacePanel = useCallback(() => onSetRightPanel(null), [onSetRightPanel])
  const returnToContentMode = useCallback(() => onSetMode(booksReturnMode), [booksReturnMode, onSetMode])
  const openAgentChatRoute = useCallback(() => onSetMode('agentchat'), [onSetMode])
  const selectOutlineFile = useCallback((path: string) => { void selectWorkspacePath(path) }, [selectWorkspacePath])
  const openLoreLibrary = useCallback(() => {
    void flushBeforeWorkspaceSwitch().then((saved) => {
      if (saved) onSetRightPanel('lore')
    })
  }, [flushBeforeWorkspaceSwitch, onSetRightPanel])
  const referenceLoreFromWorkspace = useCallback((id: string) => {
    onLoreReferenceAdd(id)
    onSetRightPanel('ai')
  }, [onLoreReferenceAdd, onSetRightPanel])
  const requestBookSettingCreate = useCallback((item: { path: string; title: string }) => {
    requestSkillsAgent(t('planning.bookSettingCreatePrompt', item))
  }, [requestSkillsAgent, t])
  const toggleAgent = useCallback(() => onSetRightPanel(aiVisible ? null : 'ai'), [aiVisible, onSetRightPanel])
  const writingInfoActions = useMemo(() => (
    <IdeWritingInfoActions
      projectVisible={projectVisible}
      aiVisible={aiVisible}
      onToggleProjectVisible={onToggleProjectVisible}
      onToggleAgent={toggleAgent}
    />
  ), [aiVisible, onToggleProjectVisible, projectVisible, toggleAgent])
  const openReviewFile = useCallback(async (path: string) => {
    const navigated = await selectWorkspacePath(path)
    if (navigated !== false) closeChangeReview()
  }, [closeChangeReview, selectWorkspacePath])
  const openAgentChangeReview = useCallback((reviewThreadID: string, groupID: string) => {
    void openChangeReview(reviewThreadID, groupID)
  }, [openChangeReview])
  const persistNarrativeStyle = useCallback((id: string) => (
    composerSettings.persist('interactive_story_teller_id', id)
  ), [composerSettings.persist])
  const readingTypography = useMemo(() => ({
    fontFamily: readingFontFamily,
    fontSize: readingFontSize,
    loading: readingTypographyDraft.layered === null,
    status: readingTypographyDraft.autosaveStatus,
    error: readingTypographyDraft.autosaveError || readingTypographyDraft.error,
    onFontFamilyChange: updateReadingFontFamily,
    onFontSizeChange: updateReadingFontSize,
    onRetry: readingTypographyDraft.saveNow,
  }), [
    readingFontFamily,
    readingFontSize,
    readingTypographyDraft.autosaveError,
    readingTypographyDraft.autosaveStatus,
    readingTypographyDraft.error,
    readingTypographyDraft.layered,
    readingTypographyDraft.saveNow,
    updateReadingFontFamily,
    updateReadingFontSize,
  ])
  const routeHost = useWorkbenchRouteHost({ mode, rightPanel, settingsOpen })
  const presentedMainRoute = routeHost.route
  const presentedRightPanel = routeHost.rightPanel
  const presentedLayout = routeHost.layout

  const writingAgent = useWritingAgentPanel({
    projectId,
    workspace,
    active: presentedRightPanel === 'ai' && presentedMainRoute !== 'agentchat',
    chrome: 'panel',
    composerSettings,
    currentChapter,
    selectedFile,
    tellers,
    imagePresets,
    messages,
    sessions,
    activeSessionId,
    isStreaming,
    sessionTransitionPending,
    isExecutionActive,
    runtimeProjection,
    abortPending,
    commandSubmitting,
    queueActionPendingCommandID,
    activityContent,
    references,
    loreReferences,
    loreReferenceLabels,
    loreSuggestions,
    styleScenes,
    textSelections,
    planMode: chatPlanMode,
    hasEarlierMessages,
    isLoadingEarlierHistory,
    fileSuggestions,
    onCreateSession: onCreateChatSession,
    onSwitchSession: onSwitchChatSession,
    onRenameSession: onRenameChatSession,
    onDeleteSession: onDeleteChatSession,
    onLoadEarlierHistory,
    onRefreshHistory: onRefreshChatHistory,
    onSend,
    onAnalyzeContext,
    ideContext,
    onStop,
    onSteerQueuedCommand,
    onDeleteQueuedCommand,
    onEditQueuedCommand,
    onReferenceRemove,
    onLoreReferenceAdd,
    onLoreReferenceRemove,
    onStyleSceneAdd,
    onStyleSceneRemove,
    onTextSelectionRemove,
    onInsertIllustration: insertIllustrationIntoEditor,
    onPlanModeChange: onChatPlanModeChange,
    onPlanModeToggle: onChatPlanModeToggle,
    onApproveProposedPlan,
    onExitPlanMode: onExitChatPlanMode,
    reviewFeedback,
    onReviewFeedbackOpen: openActiveReviewFeedback,
    onReviewFeedbackRemove: removeActiveReviewFeedback,
    onReviewFeedbackSubmitted: submitActiveReviewFeedback,
    onReviewFeedbackSubmissionFailed: restoreActiveReviewFeedback,
    onOpenChangeReview: openAgentChangeReview,
    onWorkspaceChanged: notifyExternalStructureChange,
    onClose: closeIdeWorkspacePanel,
    onSubAgentDetailsChange: setAgentSubAgentDetailsOpen,
  })

  const sidebar = (
    <WritingSidebar
      sidebarView={sidebarView}
      loading={loading}
      projectId={projectId}
      workspace={workspace}
      tree={tree}
      chapters={chapters}
      summaryAvailable={Boolean(summary)}
      ideas={summary?.ideas}
      outline={summary?.outline}
      chapterPlans={summary?.chapter_plans ?? EMPTY_CHAPTER_PLANS}
      selectedFile={selectedFile}
      loreTabActive={activeTab?.kind === 'lore'}
      revealRequest={outlineRevealRequest}
      chapterStats={chapterStats}
      projectExplorerRefreshSignal={projectExplorerRefreshSignal}
      onSetSidebarView={onSetSidebarView}
      onSelectOutlineFile={selectOutlineFile}
      onOpenLoreTab={onOpenLoreTab}
      onReferenceFile={onReferenceFile}
      onRevealFile={revealFileInProject}
      onRenameItem={onRenameItem}
      onDeleteItem={onDeleteItem}
      onRequestBookSettingCreate={requestBookSettingCreate}
      onSetChapterConfirmed={onSetChapterConfirmed}
      onSelectSearchResult={selectWorkspaceSearchResult}
      onBeforeReplace={flushBeforeWorkspaceSwitch}
      onExternalContentChange={notifyExternalContentChange}
      onSelectFile={selectWorkspacePath}
      onCreateItem={onCreateItem}
      onCopyItem={onCopyItem}
      onMoveItem={onMoveItem}
      onRefreshWorkspace={onRefreshWorkspace}
    />
  )

  const main = (
    <main className="relative h-full min-w-0 overflow-hidden bg-[var(--nova-bg)]">
      <WritingMainRoute
        visible={presentedMainRoute === 'ide-writing'}
        loadingLabel={t('router.loading')}
        projectId={projectId}
        activeReviewThreadID={activeReviewThreadID}
        activeReviewRequest={activeReviewRequest}
        submittedReviewCommentIDs={submittedReviewCommentIDs}
        isStreaming={isStreaming}
        selectedFile={selectedFile}
        agentVisible={aiVisible}
        tabs={openTabs}
        activeTabKey={activeTabKey}
        activeTab={activeTab}
        summary={summary}
        tabActions={writingInfoActions}
        activeFileKind={activeFileKind}
        fileContent={fileContent}
        fileRevision={fileRevision}
        saveSignal={saveSignal}
        editorAutoSaveEnabled={editorAutoSaveEnabled}
        editorAutoSaveDelayMs={editorAutoSaveDelayMs}
        currentChapter={currentChapter}
        editorSearchIntent={editorSearchIntent?.path === selectedFile ? editorSearchIntent : null}
        illustrationInsertSignal={illustrationInsertSignal}
        documentReview={documentReviewController}
        documentReviewNavigationTarget={documentReviewNavigationTarget}
        toolNavigationIntent={toolNavigationIntent}
        readingTypography={readingTypography}
        loreEmpty={loreEmpty}
        onToggleAgent={toggleAgent}
        onCloseReview={closeChangeReview}
        onOpenReviewFile={openReviewFile}
        onWorkspaceChanged={notifyExternalStructureChange}
        onFeedbackCommentsChange={selectReviewFeedback}
        onActivateTab={onActivateTab}
        onCloseTab={onCloseTab}
        onToggleTabPin={onToggleTabPin}
        onMoveTab={onMoveTab}
        onEditorFlushHandlerChange={onEditorFlushHandlerChange}
        onOpenLoreLibrary={openLoreLibrary}
        onReferenceLoreItem={referenceLoreFromWorkspace}
        onSaveCurrentFile={onSaveCurrentFile}
        onQuoteSelection={onQuoteSelection}
        onRevealChapter={revealCurrentChapterInOutline}
        onGenerateIllustration={requestChapterIllustration}
        onEditorLineChange={setEditorLine}
        emptyText={t('router.chooseFile')}
        emptyLoreTitle={t('loreInit.ideTitle')}
        emptyLoreDescription={t('loreInit.ideDescription')}
        emptyLoreAction={t('loreInit.ideAction')}
        onRequestWritingInit={requestWritingInit}
      />

      {routeHost.isMounted('interactive') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'interactive'} loadingLabel={t('router.loading')}>
          <InteractiveLayout
            projectId={projectId}
            workspace={workspace}
            active={presentedMainRoute === 'interactive'}
            recentNarrativeStyleID={composerSettings.values.interactive_story_teller_id}
            narrativeStyleLoading={composerSettings.loading}
            onNarrativeStyleChange={persistNarrativeStyle}
            imagePresets={imagePresets}
            onImagePresetsChange={setImagePresets}
            loreEmpty={loreEmpty}
            onRequestLoreInit={requestLoreInit}
            rightPanelVisible={interactiveRightVisible}
            onToggleRightPanel={onToggleInteractiveRightPanel}
            toolNavigationIntent={toolNavigationIntent}
          />
        </WorkbenchRouteLayer>
      )}

      {routeHost.isMounted('versions') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'versions'} loadingLabel={t('router.loading')}>
          <VersionPanel
            projectId={projectId}
            workspace={workspace}
            refreshSignal={versionRefreshSignal}
            visible={versionsVisible}
            onClose={closeIdeWorkspacePanel}
          />
        </WorkbenchRouteLayer>
      )}
      {routeHost.isMounted('ide-lore') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'ide-lore'} loadingLabel={t('router.loading')}>
          <SettingPanel
            mode="lore"
            projectId={projectId}
            documentReview={documentReviewController}
            documentReviewNavigationIntent={documentReviewNavigationTarget?.target.kind === 'lore_item' ? documentReviewNavigationTarget : null}
            onClose={closeIdeWorkspacePanel}
            onFlushHandlerChange={handleLoreLibraryFlushHandlerChange}
            toolNavigationIntent={toolNavigationIntent}
          />
        </WorkbenchRouteLayer>
      )}
      {routeHost.isMounted('ide-teller') && (
        <WorkbenchRouteLayer visible={presentedMainRoute === 'ide-teller'} loadingLabel={t('router.loading')}>
          <SettingPanel projectId={projectId} mode="teller" presetUsageMode="writing" tellers={tellers} imagePresets={imagePresets} onTellersChange={setTellers} onImagePresetsChange={setImagePresets} onClose={closeIdeWorkspacePanel} toolNavigationIntent={toolNavigationIntent} />
        </WorkbenchRouteLayer>
      )}

      <SharedWorkbenchRoutes
        route={presentedMainRoute}
        isMounted={routeHost.isMounted}
        loadingLabel={t('router.loading')}
        home={{ workspace, novaDir, books, bookSortMode, onSwitch: onSwitchBook,
          onBeforeSwitch: flushBeforeWorkspaceSwitch, onBooksChange, onOpenCharacterCardImport, onClose: closeBooks }}
        automations={{ projectId, workspace, onOpenAgentChat: openAgentChatRoute, onClose: returnToContentMode }}
        resourceTarget={resourceTarget}
        onReturnToContentMode={returnToContentMode}
        onCloseSettings={onCloseSettings}
        toolNavigationIntent={toolNavigationIntent}
      />
      <AgentChatWorkbenchRoute
        mounted={routeHost.isMounted('agentchat')}
        visible={presentedMainRoute === 'agentchat'}
        loadingLabel={t('router.loading')}
        retentionKey={workspace}
        projectId={projectId}
        novaDir={novaDir}
        composerSettings={composerSettings}
        tellers={tellers}
        imagePresets={imagePresets}
        autoSaveEnabled={editorAutoSaveEnabled}
        autoSaveDelayMs={editorAutoSaveDelayMs}
        onBeforeCreateBook={flushBeforeWorkspaceSwitch}
        onBookCreated={onAgentChatBookCreated}
        onBooksChange={onBooksChange}
        onFlushHandlerChange={handleAgentChatFlushHandlerChange}
        onProjectNavigationChange={setAgentChatProjectNavigation}
        onWorkspaceChanged={onWorkspaceChanged}
      />
    </main>
  )


  return (
    <ToolNavigationProvider value={toolNavigation}>
    <>
    <WorkbenchShell
      mode={mode}
      presentedLayout={presentedLayout}
      booksReturnMode={booksReturnMode}
      currentBookName={currentBookName}
      workspace={workspace}
      books={books}
      appVersion={appVersion}
      summary={summary}
      currentChapter={currentChapter}
      editorLine={editorLine}
      isStreaming={isStreaming}
      projectVisible={projectVisible && !reviewVisible}
      activityBarExpanded={activityBarExpanded}
      rightPanel={rightPanel}
      rightPanelWide={agentSubAgentDetailsOpen && !reviewVisible}
      centerFocus={reviewVisible}
      settingsOpen={settingsOpen}
      developerMode={developerMode}
      interactiveSubmode={interactiveSubmode}
      sidebar={sidebar}
      main={main}
      rightPanelContent={writingAgent.content}
      agentChatProjectNavigation={agentChatProjectNavigation}
      notice={notice}
      onSetMode={onSetMode}
      onToggleActivityBarExpanded={onToggleActivityBarExpanded}
      onSetInteractiveSubmode={setInteractiveSubmode}
      onSetRightPanel={onSetRightPanel}
      onToggleSettings={onToggleSettings}
      onCloseSettings={onCloseSettings}
      onQuickSwitchBook={quickSwitchBook}
      onDismissNotice={onDismissNotice}
    />
    {writingAgent.portal}
    </>
    </ToolNavigationProvider>
  )
}
