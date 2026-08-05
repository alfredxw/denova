import { PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen, Sparkles } from 'lucide-react'
import { lazy, memo, Suspense, useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { SearchPanel } from '@/components/Sidebar/SearchPanel'
import { AgentPanel, WRITING_COMPOSER_SETTING_DEFAULTS } from '@/components/Chat/AgentPanel'
import { FilePreview } from '@/components/workbench/FilePreview'
import { MarkdownEditor, type EditorFlushHandler } from '@/components/Editor/MarkdownEditor'
import { ChapterOutline, type OutlineRevealRequest } from '@/components/workbench/outline/ChapterOutline'
import { getImagePresets, getInteractiveTellers } from '@/features/interactive/api'
import { WritingProjectExplorer } from '@/features/project-explorer/WritingProjectExplorer'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import type { FileNode } from '@/hooks/useWorkspace'
import type { ActiveChatTask, AgentRuntimeQueuedCommand, BookRecord, BookSortMode, ChapterIllustration, ChapterSummary, ContextAnalysis, DocumentPreview, LoreItem, SessionSummary, TextSelection, WorkspaceSearchResult, WorkspaceSummary } from '@/lib/api'
import { GLOBAL_RESOURCE_TARGET, projectResourceTarget } from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import type { ChatSendOptions } from '@/hooks/useAgentChat'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { useLayeredSettingsDraft } from '@/features/settings/use-layered-settings-draft'
import { GLOBAL_SETTINGS_TARGET } from '@/features/settings/api'
import { applyReadingTypographySettings } from '@/features/settings/font-variables'
import type { AgentPartRef } from '@/lib/agent-message-view'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import { workspaceFileKind } from '@/lib/workspace-file-kind'
import { isLoreItemsPath } from '@/lib/workspace-path'
import { useWritingChangeReview } from '@/features/changes/use-writing-change-review'
import type { ReviewFeedbackBatch, ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import { useDocumentReview } from '@/features/document-review/use-document-review'
import { loreImportanceLabel, loreLoadModeLabel, loreTypeLabel } from '@/features/lore/options'
import { ChangeReviewWorkspace } from '@/features/changes/review/ChangeReviewWorkspace'
import type { WorkbenchNotice } from '@/features/notices/use-workbench-notice'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'
import type { Tab } from './TabController'
import { TabController, tabKey } from './TabController'
import { WorkbenchShell, type WorkbenchPresentedLayout } from './WorkbenchShell'
import { flattenFileTree } from './workbench-utils'
import { useReviewFeedbackNavigation } from './use-review-feedback-navigation'

const WRITING_AGENT_INIT_EVENT = 'nova:writing-agent-init'
const InteractiveLayout = memo(lazy(() => import('@/features/interactive/components/InteractiveLayout').then((module) => ({ default: module.InteractiveLayout }))))
const SettingPanel = memo(lazy(() => import('@/features/interactive/components/SettingPanel').then((module) => ({ default: module.SettingPanel }))))
const VersionPanel = memo(lazy(() => import('@/components/Versions/VersionPanel').then((module) => ({ default: module.VersionPanel }))))
const HomeView = memo(lazy(() => import('@/components/Home/HomeView').then((module) => ({ default: module.HomeView }))))
const AgentsView = memo(lazy(() => import('@/features/agents/AgentsView').then((module) => ({ default: module.AgentsView }))))
const AutomationsView = memo(lazy(() => import('@/features/automations/AutomationsView').then((module) => ({ default: module.AutomationsView }))))
const SkillsView = memo(lazy(() => import('@/features/skills/SkillsView').then((module) => ({ default: module.SkillsView }))))
const SettingsView = memo(lazy(() => import('@/features/settings/SettingsView').then((module) => ({ default: module.SettingsView }))))
const AgentChatRoute = memo(lazy(() => import('@/features/agent-chat/AgentChatRoute').then((module) => ({ default: module.AgentChatRoute }))))
const LoreWorkspaceTab = memo(lazy(() => import('@/features/lore/LoreWorkspaceTab').then((module) => ({ default: module.LoreWorkspaceTab }))))
const StableWritingProjectExplorer = memo(WritingProjectExplorer)
const StableSearchPanel = memo(SearchPanel)
const StableChapterOutline = memo(ChapterOutline)
const StableFilePreview = memo(FilePreview)
const StableMarkdownEditor = memo(MarkdownEditor)
const StableChangeReviewWorkspace = memo(ChangeReviewWorkspace)
const StableTabController = memo(TabController)
const EMPTY_CHAPTERS: ChapterSummary[] = []
const EMPTY_CHAPTER_PLANS: DocumentPreview[] = []
const EXTERNAL_CONTENT_CHANGE = { impact: 'content', origin: 'external' } satisfies WorkspaceChangeMetadata
const EXTERNAL_STRUCTURE_CHANGE = { impact: 'structure', origin: 'external' } satisfies WorkspaceChangeMetadata
type MainRouteId = 'settings' | 'skills' | 'agents' | 'automations' | 'agentchat' | 'books' | 'interactive' | 'versions' | 'ide-lore' | 'ide-teller' | 'ide-writing'
const PRESENTED_LAYOUT_BY_ROUTE = {
  settings: 'full',
  skills: 'full',
  agents: 'full',
  automations: 'full',
  agentchat: 'full',
  books: 'full',
  interactive: 'interactive',
  versions: 'full',
  'ide-lore': 'full',
  'ide-teller': 'full',
  'ide-writing': 'writing',
} satisfies Record<MainRouteId, WorkbenchPresentedLayout>

function normalizeReadingFontSize(value: unknown) {
  const parsed = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(parsed)) return 18
  return Math.min(28, Math.max(14, Math.round(parsed)))
}

interface ModeRouterProps {
  mode: WorkspaceMode
  booksReturnMode: 'ide' | 'interactive'
  currentBookName: string
  workspace: string
  projectId: string
  appVersion: string
  summary: WorkspaceSummary | null
  currentChapter?: ChapterSummary
  chapterStats: Record<string, ChapterSummary>
  isStreaming: boolean
  isExecutionActive: boolean
  runtimeProjection?: ActiveChatTask | null
  abortPending?: boolean
  commandSubmitting?: boolean
  queueActionPendingCommandID?: string
  projectVisible: boolean
  activityBarExpanded: boolean
  rightPanel: RightPanel
  settingsOpen: boolean
  interactiveRightVisible: boolean
  novaDir: string
  books: BookRecord[]
  bookSortMode: BookSortMode
  tree: FileNode[]
  loading: boolean
  selectedFile: string | null
  fileContent: string
  fileRevision: string
  openTabs: Tab[]
  activeTabKey: string | null
  sidebarView: 'outline' | 'files' | 'search'
  editorSearchIntent: { path: string; query: string; line: number; nonce: number } | null
  saveSignal: number
  editorAutoSaveEnabled: boolean
  editorAutoSaveDelayMs: number
  projectExplorerRefreshSignal: number
  versionRefreshSignal: number
  messages: AgentUIMessage[]
  sessions: SessionSummary[]
  activeSessionId: string
  activityContent: string
  references: string[]
  loreReferences: string[]
  loreItems: LoreItem[]
  styleScenes: string[]
  textSelections: TextSelection[]
  chatPlanMode: boolean
  hasEarlierMessages: boolean
  isLoadingEarlierHistory: boolean
  notice?: WorkbenchNotice | null
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onToggleProjectVisible: () => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleSettings: () => void
  onCloseSettings: () => void
  onToggleInteractiveRightPanel: () => void
  onSwitchBook: (path: string) => void
  onQuickSwitchBook: (path: string) => Promise<boolean>
  onBeforeWorkspaceSwitch: EditorFlushHandler
  onBooksChange: () => void | Promise<void>
  onOpenCharacterCardImport: () => void
  onSetSidebarView: (view: 'outline' | 'files' | 'search') => void
  onSelectSearchResult: (result: WorkspaceSearchResult, query: string) => void | Promise<void>
  onSelectFile: (path: string) => boolean | void | Promise<boolean | void>
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
  onReferenceFile: (path: string) => void
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  onRefreshWorkspace: () => void | Promise<void>
  onActivateTab: (tab: Tab) => void
  onCloseTab: (tab: Tab) => void
  onToggleTabPin: (tab: Tab) => void
  onMoveTab: (sourceKey: string, targetKey: string) => void
  onOpenLoreTab: () => Promise<boolean>
  onSaveCurrentFile: (path: string, content: string, baseRevision: string) => Promise<{ revision?: string }>
  onEditorFlushHandlerChange: (handler: EditorFlushHandler | null) => void
  onWorkspaceChanged: (paths: string[], metadata: WorkspaceChangeMetadata) => void | Promise<void>
  onQuoteSelection: (selection: TextSelection) => void
  onCreateChatSession: (title?: string) => void | Promise<void>
  onSwitchChatSession: (id: string) => void | Promise<void>
  onRenameChatSession: (id: string, title: string) => void | Promise<void>
  onDeleteChatSession: (id: string) => void | Promise<void>
  onLoadEarlierHistory: () => void | Promise<void>
  onRefreshChatHistory: (sessionId?: string) => void | Promise<void>
  onSend: (message: string, options?: ChatSendOptions) => boolean | Promise<boolean>
  onAnalyzeContext: (message: string, options?: { writingSkill?: string; ideContext?: { currentFile?: string; openFiles?: string[] }; imagePresetId?: string; tellerId?: string }) => Promise<ContextAnalysis>
  onStop: () => void
  onSteerQueuedCommand?: (item: AgentRuntimeQueuedCommand) => boolean | Promise<boolean>
  onDeleteQueuedCommand?: (item: AgentRuntimeQueuedCommand) => boolean | Promise<boolean>
  onEditQueuedCommand?: (item: AgentRuntimeQueuedCommand) => string | null | Promise<string | null>
  onReferenceRemove: (path: string) => void
  onLoreReferenceAdd: (id: string) => void
  onLoreReferenceRemove: (id: string) => void
  onStyleSceneAdd: (scene: string) => void
  onStyleSceneRemove: (scene: string) => void
  onTextSelectionRemove: (index: number) => void
  onChatPlanModeChange: (value: boolean) => void
  onChatPlanModeToggle: () => void
  onApproveProposedPlan: (ref: AgentPartRef) => void
  onExitChatPlanMode: () => void
  onDismissNotice?: () => void
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
    isExecutionActive,
    runtimeProjection,
    abortPending,
    commandSubmitting,
    queueActionPendingCommandID,
    projectVisible,
    activityBarExpanded,
    rightPanel,
    settingsOpen,
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
    || 'source-han-serif'
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
  const agentsVisible = mode === 'agents'
  const automationsVisible = mode === 'automations'
  const skillsVisible = mode === 'skills'
  const agentChatVisible = mode === 'agentchat'
  const ideWorkspacePanel = mode === 'ide' && (rightPanel === 'lore' || rightPanel === 'teller') ? rightPanel : null
  const interactiveSubmode = useInteractiveStore((state) => state.submode)
  const setInteractiveSubmode = useInteractiveStore((state) => state.setSubmode)
  const [tellers, setTellers] = useState<Teller[]>([])
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>([])
  const [agentSubAgentDetailsOpen, setAgentSubAgentDetailsOpen] = useState(false)
  const [agentPanelHost] = useState(() => createStablePortalHost('h-full min-h-0 w-full min-w-0 overflow-hidden'))
  const [illustrationInsertSignal, setIllustrationInsertSignal] = useState<{ illustration: ChapterIllustration; nonce: number } | null>(null)
  const [outlineRevealRequest, setOutlineRevealRequest] = useState<OutlineRevealRequest | null>(null)
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
  const showSidebarLoading = Boolean(workspace) && (
    sidebarView === 'files'
      ? !projectId
      : sidebarView === 'outline' && loading && tree.length === 0 && !summary
  )

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
  const visibleMainRoute: MainRouteId = settingsOpen
    ? 'settings'
    : skillsVisible
      ? 'skills'
      : agentsVisible
        ? 'agents'
        : automationsVisible
          ? 'automations'
          : agentChatVisible
            ? 'agentchat'
            : mode === 'books'
              ? 'books'
              : versionsVisible
                ? 'versions'
                : mode === 'interactive'
                  ? 'interactive'
                  : ideWorkspacePanel
                    ? `ide-${ideWorkspacePanel}`
                    : 'ide-writing'
  // Navigation chrome updates immediately; route work follows at transition priority so a
  // complex editor or Markdown page cannot hold the selected-menu feedback hostage.
  const presentedMainRoute = useDeferredValue(visibleMainRoute)
  const presentedRightPanel = useDeferredValue(rightPanel)
  const presentedLayout = PRESENTED_LAYOUT_BY_ROUTE[presentedMainRoute]
  const [mountedRoutes, setMountedRoutes] = useState<ReadonlySet<MainRouteId>>(() => new Set(['ide-writing', visibleMainRoute]))
  const renderedRoutes = useMemo(() => {
    if (mountedRoutes.has(presentedMainRoute)) return mountedRoutes
    return new Set([...mountedRoutes, presentedMainRoute])
  }, [mountedRoutes, presentedMainRoute])

  useEffect(() => {
    setMountedRoutes((current) => {
      if (current.has(visibleMainRoute)) return current
      const next = new Set(current)
      next.add(visibleMainRoute)
      return next
    })
  }, [visibleMainRoute])

  // Foreground Writing keeps one stable AgentPanel instance. AgentChat deliberately owns
  // separate project/session-bound panels, so its tabs can run concurrently without moving
  // or re-pointing this foreground instance.
  const agentPanel = (
    <AgentPanel
      projectId={projectId}
      workspace={workspace}
      chrome="panel"
      composerSettings={composerSettings}
      currentChapter={currentChapter}
      selectedFile={selectedFile}
      tellers={tellers}
      imagePresets={imagePresets}
      messages={messages}
      sessions={sessions}
      activeSessionId={activeSessionId}
      isStreaming={isStreaming}
      isExecutionActive={isExecutionActive}
      runtimeProjection={runtimeProjection}
      abortPending={abortPending}
      commandSubmitting={commandSubmitting}
      queueActionPendingCommandID={queueActionPendingCommandID}
      activityContent={activityContent}
      references={references}
      loreReferences={loreReferences}
      loreReferenceLabels={loreReferenceLabels}
      loreSuggestions={loreSuggestions}
      styleScenes={styleScenes}
      textSelections={textSelections}
      planMode={chatPlanMode}
      hasEarlierMessages={hasEarlierMessages}
      isLoadingEarlierHistory={isLoadingEarlierHistory}
      fileSuggestions={fileSuggestions}
      onCreateSession={onCreateChatSession}
      onSwitchSession={onSwitchChatSession}
      onRenameSession={onRenameChatSession}
      onDeleteSession={onDeleteChatSession}
      onLoadEarlierHistory={onLoadEarlierHistory}
      onRefreshHistory={onRefreshChatHistory}
      onSend={onSend}
      onAnalyzeContext={onAnalyzeContext}
      ideContext={ideContext}
      onStop={onStop}
      onSteerQueuedCommand={onSteerQueuedCommand}
      onDeleteQueuedCommand={onDeleteQueuedCommand}
      onEditQueuedCommand={onEditQueuedCommand}
      onReferenceRemove={onReferenceRemove}
      onLoreReferenceAdd={onLoreReferenceAdd}
      onLoreReferenceRemove={onLoreReferenceRemove}
      onStyleSceneAdd={onStyleSceneAdd}
      onStyleSceneRemove={onStyleSceneRemove}
      onTextSelectionRemove={onTextSelectionRemove}
      onInsertIllustration={insertIllustrationIntoEditor}
      onPlanModeChange={onChatPlanModeChange}
      onPlanModeToggle={onChatPlanModeToggle}
      onApproveProposedPlan={onApproveProposedPlan}
      onExitPlanMode={onExitChatPlanMode}
      reviewFeedback={reviewFeedback}
      onReviewFeedbackOpen={openActiveReviewFeedback}
      onReviewFeedbackRemove={removeActiveReviewFeedback}
      onReviewFeedbackSubmitted={submitActiveReviewFeedback}
      onReviewFeedbackSubmissionFailed={restoreActiveReviewFeedback}
      onOpenChangeReview={openAgentChangeReview}
      onWorkspaceChanged={notifyExternalStructureChange}
      onClose={closeIdeWorkspacePanel}
      onSubAgentDetailsChange={setAgentSubAgentDetailsOpen}
    />
  )
  const agentPanelSlot = useMemo(() => (
    <StablePortalSlot
      host={agentPanelHost}
      fallback={null}
      className="h-full min-h-0 w-full min-w-0 overflow-hidden"
    />
  ), [agentPanelHost])
  const hostedAgentPanel = agentPanelHost ? agentPanelSlot : agentPanel
  const agentPanelPortal = agentPanelHost ? createPortal(agentPanel, agentPanelHost, 'workbench-agent-panel') : null
  const rightPanelContent = presentedRightPanel === 'ai' && presentedMainRoute !== 'agentchat' ? hostedAgentPanel : null

  const sidebar = (
    <section className="nova-sidebar flex h-full flex-col border-r">
      <div className="border-b border-[var(--nova-border)] px-3 py-2">
        <div className="grid grid-cols-3 gap-1">
          <button
            type="button"
            onClick={() => onSetSidebarView('outline')}
            className={`nova-nav-item h-7 min-w-0 truncate whitespace-nowrap px-1 text-[11px] ${sidebarView === 'outline' ? 'is-active' : 'bg-[var(--nova-surface-2)]'}`}
          >
            {t('router.outline')}
          </button>
          <button
            type="button"
            onClick={() => onSetSidebarView('files')}
            className={`nova-nav-item h-7 min-w-0 truncate whitespace-nowrap px-1 text-[11px] ${sidebarView === 'files' ? 'is-active' : 'bg-[var(--nova-surface-2)]'}`}
          >
            {t('router.files')}
          </button>
          <button
            type="button"
            onClick={() => onSetSidebarView('search')}
            className={`nova-nav-item h-7 min-w-0 truncate whitespace-nowrap px-1 text-[11px] ${sidebarView === 'search' ? 'is-active' : 'bg-[var(--nova-surface-2)]'}`}
          >
            {t('router.search')}
          </button>
        </div>
      </div>
      <div className="min-h-0 flex-1 text-xs">
        {showSidebarLoading ? (
          <div className="h-full overflow-y-auto px-2 py-4 text-center text-[var(--nova-text-muted)]">{t('router.loading')}</div>
        ) : sidebarView === 'outline' ? (
          <StableChapterOutline
            projectId={projectId}
            tree={tree}
            chapters={chapters}
            ideas={summary?.ideas}
            outline={summary?.outline}
            chapterPlans={summary?.chapter_plans ?? EMPTY_CHAPTER_PLANS}
            selectedFile={selectedFile}
            loreTabActive={activeTab?.kind === 'lore'}
            revealRequest={outlineRevealRequest}
            onSelectFile={selectOutlineFile}
            onOpenLoreTab={onOpenLoreTab}
            onReferenceFile={onReferenceFile}
            onRevealFile={revealFileInProject}
            onRenameItem={onRenameItem}
            onDeleteItem={onDeleteItem}
            onRequestBookSettingCreate={requestBookSettingCreate}
            onSetChapterConfirmed={onSetChapterConfirmed}
          />
        ) : (
          <div className="h-full min-h-0">
            {sidebarView === 'search' ? (
              <div className="h-full overflow-y-auto p-2">
                <StableSearchPanel
                  projectId={projectId}
                  onSelectResult={selectWorkspaceSearchResult}
                  onBeforeReplace={flushBeforeWorkspaceSwitch}
                  onWorkspaceChanged={notifyExternalContentChange}
                />
              </div>
            ) : !projectId ? (
              <div className="px-2 py-4 text-center text-[var(--nova-text-muted)]">{t('router.loading')}</div>
            ) : (
              <StableWritingProjectExplorer
                key={projectId}
                projectId={projectId}
                workspace={workspace}
                selectedPath={selectedFile}
                chapterStats={chapterStats}
                structureRefreshSignal={projectExplorerRefreshSignal}
                onSelectFile={selectWorkspacePath}
                onReferenceFile={onReferenceFile}
                onCreateItem={onCreateItem}
                onDeleteItem={onDeleteItem}
                onRenameItem={onRenameItem}
                onCopyItem={onCopyItem}
                onMoveItem={onMoveItem}
                onRefreshWorkspace={onRefreshWorkspace}
              />
            )}
          </div>
        )}
      </div>
    </section>
  )

  const main = (
    <main className="relative h-full min-w-0 overflow-hidden bg-[var(--nova-bg)]">
      <MainRouteLayer visible={presentedMainRoute === 'ide-writing'} loadingLabel={t('router.loading')}>
        {activeReviewThreadID ? (
          <StableChangeReviewWorkspace
            projectId={projectId}
            threadID={activeReviewThreadID}
            scopeRequest={activeReviewRequest}
            disabled={isStreaming}
            selectedPath={selectedFile}
            agentVisible={aiVisible}
            onToggleAgent={toggleAgent}
            onClose={closeChangeReview}
            onOpenFile={openReviewFile}
            onWorkspaceChanged={notifyExternalStructureChange}
            onFeedbackCommentsChange={selectReviewFeedback}
            hiddenCommentIDs={submittedReviewCommentIDs}
          />
        ) : (
          <>
            <StableTabController
              tabs={openTabs}
              activeTabKey={activeTabKey}
              summary={summary}
              actions={writingInfoActions}
              onActivateTab={onActivateTab}
              onCloseTab={onCloseTab}
              onTogglePin={onToggleTabPin}
              onMoveTab={onMoveTab}
            />
            <div className="flex min-h-0 flex-1 flex-col">
              {activeTab ? (
                activeTab.kind === 'lore' ? (
                  <LoreWorkspaceTab
                    projectId={projectId}
                    documentReview={documentReviewController}
                    navigationIntent={documentReviewNavigationTarget?.target.kind === 'lore_item' ? documentReviewNavigationTarget : null}
                    onEditorFlushHandlerChange={onEditorFlushHandlerChange}
                    onOpenLibrary={openLoreLibrary}
                    onReferenceItem={referenceLoreFromWorkspace}
                  />
                ) : activeFileKind === 'image' || activeFileKind === 'json' || activeFileKind === 'jsonl' ? (
                  <StableFilePreview projectId={projectId} path={selectedFile || activeTab.path} content={fileContent} revision={fileRevision} />
                ) : (
                  <StableMarkdownEditor
                    projectId={projectId}
                    fileName={selectedFile}
                    content={fileContent}
                    revision={fileRevision}
                    onSave={onSaveCurrentFile}
                    onQuoteSelection={onQuoteSelection}
                    saveSignal={saveSignal}
                    autoSaveEnabled={editorAutoSaveEnabled}
                    autoSaveDelayMs={editorAutoSaveDelayMs}
                    chapterSummary={currentChapter}
                    onRevealChapter={revealCurrentChapterInOutline}
                    searchIntent={editorSearchIntent?.path === selectedFile ? editorSearchIntent : null}
                    onGenerateIllustration={requestChapterIllustration}
                    generateIllustrationDisabled={isStreaming || !currentChapter}
                    illustrationInsertSignal={illustrationInsertSignal}
                    onLineChange={setEditorLine}
                    onFlushHandlerChange={onEditorFlushHandlerChange}
                    documentReview={documentReviewController}
                    documentReviewNavigationIntent={documentReviewNavigationTarget?.target.kind === 'workspace_file' && documentReviewNavigationTarget.target.id === selectedFile ? documentReviewNavigationTarget : null}
                    readingTypography={readingTypography}
                  />
                )
              ) : (
                loreEmpty ? (
                  <EmptyLoreGuide
                    emptyText={t('router.chooseFile')}
                    title={t('loreInit.ideTitle')}
                    description={t('loreInit.ideDescription')}
                    action={t('loreInit.ideAction')}
                    onClick={requestWritingInit}
                  />
                ) : (
                  <div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">
                    {t('router.chooseFile')}
                  </div>
                )
              )}
            </div>
          </>
        )}
      </MainRouteLayer>

      {renderedRoutes.has('interactive') && (
        <MainRouteLayer visible={presentedMainRoute === 'interactive'} loadingLabel={t('router.loading')}>
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
          />
        </MainRouteLayer>
      )}

      {renderedRoutes.has('versions') && (
        <MainRouteLayer visible={presentedMainRoute === 'versions'} loadingLabel={t('router.loading')}>
          <VersionPanel
            projectId={projectId}
            workspace={workspace}
            refreshSignal={versionRefreshSignal}
            visible={versionsVisible}
            onClose={closeIdeWorkspacePanel}
          />
        </MainRouteLayer>
      )}
      {renderedRoutes.has('ide-lore') && (
        <MainRouteLayer visible={presentedMainRoute === 'ide-lore'} loadingLabel={t('router.loading')}>
          <SettingPanel
            mode="lore"
            projectId={projectId}
            documentReview={documentReviewController}
            documentReviewNavigationIntent={documentReviewNavigationTarget?.target.kind === 'lore_item' ? documentReviewNavigationTarget : null}
            onClose={closeIdeWorkspacePanel}
            onFlushHandlerChange={handleLoreLibraryFlushHandlerChange}
          />
        </MainRouteLayer>
      )}
      {renderedRoutes.has('ide-teller') && (
        <MainRouteLayer visible={presentedMainRoute === 'ide-teller'} loadingLabel={t('router.loading')}>
          <SettingPanel projectId={projectId} mode="teller" presetUsageMode="writing" tellers={tellers} imagePresets={imagePresets} onTellersChange={setTellers} onImagePresetsChange={setImagePresets} onClose={closeIdeWorkspacePanel} />
        </MainRouteLayer>
      )}

      {renderedRoutes.has('books') && (
        <MainRouteLayer visible={presentedMainRoute === 'books'} loadingLabel={t('router.loading')}>
          <HomeView
            workspace={workspace}
            novaDir={novaDir}
            books={books}
            bookSortMode={bookSortMode}
            onSwitch={onSwitchBook}
            onBeforeSwitch={flushBeforeWorkspaceSwitch}
            onBooksChange={onBooksChange}
            onOpenCharacterCardImport={onOpenCharacterCardImport}
            onClose={closeBooks}
          />
        </MainRouteLayer>
      )}
      {renderedRoutes.has('skills') && (
        <MainRouteLayer visible={presentedMainRoute === 'skills'} loadingLabel={t('router.loading')}>
          <SkillsView target={resourceTarget} onClose={returnToContentMode} />
        </MainRouteLayer>
      )}
      {renderedRoutes.has('agents') && (
        <MainRouteLayer visible={presentedMainRoute === 'agents'} loadingLabel={t('router.loading')}>
          <AgentsView target={resourceTarget} onClose={returnToContentMode} />
        </MainRouteLayer>
      )}
      {renderedRoutes.has('automations') && (
        <MainRouteLayer visible={presentedMainRoute === 'automations'} loadingLabel={t('router.loading')}>
          <AutomationsView projectId={projectId} workspace={workspace} onClose={returnToContentMode} />
        </MainRouteLayer>
      )}
      {renderedRoutes.has('agentchat') && (
        <RetainedMainRouteLayer
          visible={presentedMainRoute === 'agentchat'}
          loadingLabel={t('router.loading')}
          retentionKey={workspace}
        >
          <AgentChatRoute
            projectId={projectId}
            composerSettings={composerSettings}
            tellers={tellers}
            imagePresets={imagePresets}
            autoSaveEnabled={editorAutoSaveEnabled}
            autoSaveDelayMs={editorAutoSaveDelayMs}
            onTellersChange={setTellers}
            onImagePresetsChange={setImagePresets}
            onFlushHandlerChange={handleAgentChatFlushHandlerChange}
            onWorkspaceChanged={onWorkspaceChanged}
          />
        </RetainedMainRouteLayer>
      )}
      {renderedRoutes.has('settings') && (
        <MainRouteLayer visible={presentedMainRoute === 'settings'} loadingLabel={t('router.loading')}>
          <SettingsView onClose={onCloseSettings} />
        </MainRouteLayer>
      )}
    </main>
  )


  return (
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
      interactiveSubmode={interactiveSubmode}
      sidebar={sidebar}
      main={main}
      rightPanelContent={rightPanelContent}
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
    {agentPanelPortal}
    </>
  )
}

interface MainRouteLayerProps {
  visible: boolean
  loadingLabel: string
  children: ReactNode
  /** Hidden retained routes must refresh immediately when their resource owner changes. */
  retentionKey?: string
}

function MainRouteLayer({ visible, loadingLabel, children }: MainRouteLayerProps) {
  return (
    <section hidden={!visible} aria-hidden={!visible} className="absolute inset-0 flex min-h-0 flex-col">
      <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">{loadingLabel}</div>}>
        {children}
      </Suspense>
    </section>
  )
}

/**
 * Keeps route state and effects alive while preventing unrelated foreground renders from
 * reconciling a large hidden subtree. The latest children are applied as soon as it is shown.
 */
const RetainedMainRouteLayer = memo(MainRouteLayer, (previous, next) => (
  !previous.visible
  && !next.visible
  && previous.retentionKey === next.retentionKey
  && previous.loadingLabel === next.loadingLabel
))

function IdeWritingInfoActions({
  projectVisible,
  aiVisible,
  onToggleProjectVisible,
  onToggleAgent,
}: {
  projectVisible: boolean
  aiVisible: boolean
  onToggleProjectVisible: () => void
  onToggleAgent: () => void
}) {
  const { t } = useTranslation()
  const ProjectIcon = projectVisible ? PanelLeftClose : PanelLeftOpen
  const AgentIcon = aiVisible ? PanelRightClose : PanelRightOpen
  const projectLabel = projectVisible ? t('router.hideOutline') : t('router.showOutline')
  const agentLabel = aiVisible ? t('router.hideAgent') : t('router.showAgent')

  return (
    <>
      <button
        type="button"
        onClick={onToggleProjectVisible}
        aria-label={projectLabel}
        aria-pressed={projectVisible}
        className={`nova-nav-item flex h-7 w-7 items-center justify-center ${projectVisible ? 'is-active' : ''}`}
        title={projectLabel}
      >
        <ProjectIcon className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        onClick={onToggleAgent}
        aria-label={agentLabel}
        aria-pressed={aiVisible}
        className={`nova-nav-item flex h-7 w-7 items-center justify-center ${aiVisible ? 'is-active' : ''}`}
        title={agentLabel}
      >
        <AgentIcon className="h-3.5 w-3.5" />
      </button>
    </>
  )
}

function EmptyLoreGuide({
  emptyText,
  title,
  description,
  action,
  onClick,
}: {
  emptyText: string
  title: string
  description: string
  action: string
  onClick: () => void
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-md flex-col items-center gap-3 rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
        <Sparkles className="h-4 w-4 text-[var(--nova-text-muted)]" />
        <div className="space-y-1">
          <div className="text-xs text-[var(--nova-text-faint)]">{emptyText}</div>
          <div className="text-sm font-medium text-[var(--nova-text)]">{title}</div>
          <div className="text-xs leading-5 text-[var(--nova-text-faint)]">{description}</div>
        </div>
        <button
          type="button"
          className="nova-nav-item rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1.5 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
          onClick={onClick}
        >
          {action}
        </button>
      </div>
    </div>
  )
}
