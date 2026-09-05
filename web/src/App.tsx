import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { checkForUpdate, fetchProjectSettings, fetchSettings, refreshProjectSettings, refreshSettings } from '@/features/settings/api'
import { subscribeSettingsTarget } from '@/features/settings/query'
import { applyFontSettings, fontSettingsFromEffective } from '@/features/settings/font-variables'
import { markAutoUpdateChecked, shouldRunAutoUpdateCheck, UPDATE_CHECK_RESULT_EVENT } from '@/features/settings/update-check-cache'
import type { UpdateCheckResult } from '@/features/settings/types'
import { getProjectLoreItems, importCharacterCard, previewCharacterCard, setProjectChapterConfirmed, switchWorkspace, type CharacterCardPreview, type LoreItem, type WorkspaceSearchResult } from '@/lib/api'
import { withErrorLogID } from '@/lib/api-client'
import { CommandPalette } from '@/components/common/command-palette'
import { useWorkspace } from '@/hooks/useWorkspace'
import { useAgentChat } from '@/hooks/useAgentChat'
import { useWorkspaceHotkeys } from '@/hooks/use-workspace-hotkeys'
import { isSharedWorkspaceMode, useWorkspaceStore, workspaceModeRequiresBook, type RightPanel, type WorkspaceMode } from '@/stores/workspace-store'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import type { ChapterSummary } from '@/lib/api'
import { toast } from 'sonner'
import { setConfiguredLocale } from '@/i18n'
import { NovaMotionProvider, normalizeMotionIntensity } from '@/features/motion/motion-preferences'
import {
  dedupeTabs,
  enforceTabLimit,
  persistActiveTabKeyFor,
  persistTabsFor,
  readActiveTabKeyFor,
  readTabsFor,
  reorderTabs,
  setTabPinned,
  tabKey,
  WRITING_SUBAGENT_TAB_KEY,
  type Tab,
} from '@/components/workbench/TabController'
import type { AgentSubAgentSessionTarget } from '@/components/Chat/AgentSubAgentSessionPanel'
import { ModeRouter } from '@/components/workbench/ModeRouter'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import {
  CharacterCardImportDialog,
  type CharacterCardTargetMode,
} from '@/components/workbench/CharacterCardImportDialog'
import { RemoteAccessLogin } from '@/components/RemoteAccessLogin'
import { OnboardingGuide, type OnboardingNavigationTarget } from '@/features/onboarding/OnboardingGuide'
import { SETTINGS_SECTION_EVENT, WRITING_AGENT_INIT_EVENT } from '@/features/onboarding/events'
import {
  isProjectChangeForProject,
  workspaceChangeImpact,
  workspaceChangePaths,
  type WorkspaceChangeEvent,
  type WorkspaceChangeImpact,
  type WorkspaceChangeMetadata,
} from '@/features/changes/types'
import {
  AUTOSAVE_CONFLICT_PRESERVED_EVENT,
  type AutosaveConflictPreservedDetail,
} from '@/lib/autosave/rebase-with-recovery'
import { useWorkbenchNotice } from '@/features/notices/use-workbench-notice'
import { LORE_UPDATED_EVENT, notifyLoreUpdated, type LoreUpdatedDetail } from '@/features/lore/events'
import type { AgentChatConversationState } from '@/features/agent-chat/AgentChatConversationTab'

const PROJECT_VISIBLE_KEY = 'nova.layout.projectVisible'
const ACTIVITY_BAR_EXPANDED_KEY = 'nova.layout.activityBarExpanded'
const INTERACTIVE_RIGHT_VISIBLE_KEY = 'nova.layout.interactiveRightVisible'
const SETTINGS_OPEN_KEY = 'nova.layout.settingsOpen'
const CONTENT_MODE_STORAGE_KEY = 'nova:content-mode'
const MAX_OPEN_TABS_FALLBACK = 5
const AUTO_SAVE_ENABLED_FALLBACK = true
const AUTO_SAVE_DELAY_FALLBACK_MS = 1500
type SidebarView = 'outline' | 'files' | 'search'
type CreationRoute = 'ide' | 'interactive'

function App() {
  const { t } = useTranslation()
  const { setTheme } = useTheme()
  const [projectVisible, setProjectVisible] = useState(() => readLayoutBoolean(PROJECT_VISIBLE_KEY, true))
  const [activityBarExpanded, setActivityBarExpanded] = useState(() => readLayoutBoolean(ACTIVITY_BAR_EXPANDED_KEY, true))
  const [interactiveRightVisible, setInteractiveRightVisible] = useState(() => readLayoutBoolean(INTERACTIVE_RIGHT_VISIBLE_KEY, true))
  const [saveSignal, setSaveSignal] = useState(0)
  const [projectExplorerRefreshSignal, setProjectExplorerRefreshSignal] = useState(0)
  const [versionRefreshSignal, setVersionRefreshSignal] = useState(0)
  const [settingsOpen, setSettingsOpen] = useState(() => readLayoutBoolean(SETTINGS_OPEN_KEY, false))
  const [openTabs, setOpenTabs] = useState<Tab[]>([])
  const [activeTabKey, setActiveTabKey] = useState<string | null>(null)
  const [maxOpenTabs, setMaxOpenTabs] = useState<number>(MAX_OPEN_TABS_FALLBACK)
  const [editorAutoSaveEnabled, setEditorAutoSaveEnabled] = useState(AUTO_SAVE_ENABLED_FALLBACK)
  const [editorAutoSaveDelayMs, setEditorAutoSaveDelayMs] = useState(AUTO_SAVE_DELAY_FALLBACK_MS)
  const [updateCheckEnabled, setUpdateCheckEnabled] = useState<boolean | null>(null)
  const [developerMode, setDeveloperMode] = useState<boolean | null>(null)
  const [motionIntensity, setMotionIntensity] = useState('system')
  const [novaDir, setNovaDir] = useState('')
  const [sidebarView, setSidebarView] = useState<SidebarView>('outline')
  const [editorSearchIntent, setEditorSearchIntent] = useState<{ path: string; query: string; line: number; nonce: number } | null>(null)
  const [characterCardDialogOpen, setCharacterCardDialogOpen] = useState(false)
  const [characterCardFile, setCharacterCardFile] = useState<File | null>(null)
  const [characterCardPreview, setCharacterCardPreview] = useState<CharacterCardPreview | null>(null)
  const [characterCardTargetMode, setCharacterCardTargetMode] = useState<CharacterCardTargetMode>('new_book')
  const [characterCardBookTitle, setCharacterCardBookTitle] = useState('')
  const [characterCardUserName, setCharacterCardUserName] = useState('')
  const [characterCardSemanticClassification, setCharacterCardSemanticClassification] = useState(true)
  const [characterCardPreviewing, setCharacterCardPreviewing] = useState(false)
  const [characterCardImporting, setCharacterCardImporting] = useState(false)
  const [characterCardError, setCharacterCardError] = useState('')
  const [loreItems, setLoreItems] = useState<LoreItem[]>([])
  const [writingAgentConversation, setWritingAgentConversation] = useState<AgentChatConversationState>({
    sessionId: '',
    messages: [],
    isStreaming: false,
  })
  const [lastCreationRoute, setLastCreationRoute] = useState<CreationRoute>(() => readContentMode())
  const lastCreationRouteRef = useRef<CreationRoute>(readContentMode())
  const characterCardInputRef = useRef<HTMLInputElement>(null)
  const updateCheckInFlightRef = useRef(false)
  const tabActivationsRef = useRef<Map<string, number>>(new Map())
  const tabActivationCounterRef = useRef(0)
  const editorFlushHandlerRef = useRef<EditorFlushHandler | null>(null)

  const rightPanel = useWorkspaceStore((state) => state.rightPanel)
  const commandOpen = useWorkspaceStore((state) => state.commandOpen)
  const mode = useWorkspaceStore((state) => state.mode)
  const setRightPanel = useWorkspaceStore((state) => state.setRightPanel)
  const setCommandOpen = useWorkspaceStore((state) => state.setCommandOpen)
  const setMode = useWorkspaceStore((state) => state.setMode)
  const setSelectedChapterId = useWorkspaceStore((state) => state.setSelectedChapterId)
  const toggleActivityBarExpanded = useCallback(() => setActivityBarExpanded((value) => !value), [])
  const toggleProjectVisible = useCallback(() => setProjectVisible((value) => !value), [])
  const toggleSettings = useCallback(() => setSettingsOpen((open) => !open), [])
  const closeSettings = useCallback(() => setSettingsOpen(false), [])
  const toggleInteractiveRightPanel = useCallback(() => setInteractiveRightVisible((value) => !value), [])
  useEffect(() => {
    if (isSharedWorkspaceMode(mode)) return
    const contentMode = mode === 'interactive' ? 'interactive' : 'ide'
    lastCreationRouteRef.current = contentMode
    setLastCreationRoute(contentMode)
  }, [mode])

  const {
    tree, loading, selectedFile, fileDocument, fileContent, fileRevision, workspace, projectId, workspaceLoaded, summary, books, bookSortMode,
    selectFile, clearSelectedFile, saveFileDraft, createItem, deleteItem, renameItem, copyItem, moveItem,
    refresh, refreshSummary, refreshAfterAgentFileChange, refreshAll, refreshBooks,
  } = useWorkspace()
  const settingsWorkspaceRef = useRef<string | null>(null)

  const notifyVersionChange = useCallback(() => {
    setVersionRefreshSignal(value => value + 1)
  }, [])

  const notifyProjectStructureChange = useCallback(() => {
    setProjectExplorerRefreshSignal(value => value + 1)
  }, [])

  const handleEditorFlushHandlerChange = useCallback((handler: EditorFlushHandler | null) => {
    editorFlushHandlerRef.current = handler
  }, [])

  const flushEditorDraft = useCallback(async () => {
    const handler = editorFlushHandlerRef.current
    if (!handler) return true
    try {
      return await handler()
    } catch (error) {
      console.error('[App.tsx] failed to flush the editor draft before navigation', error)
      toast.error(withErrorLogID(t('editor.saveFailed'), error))
      return false
    }
  }, [t])

  const handleAgentFileChange = useCallback(async (
    path?: string,
    impact: WorkspaceChangeImpact = 'structure',
  ) => {
    try {
      await refreshAfterAgentFileChange(path, impact)
    } finally {
      // Lore tools persist outside the workspace file tree, so Agent completion
      // invalidates Lore projections explicitly as well as file summaries.
      notifyLoreUpdated({ projectId, source: 'writing-agent' })
      notifyVersionChange()
      if (impact === 'structure') notifyProjectStructureChange()
    }
  }, [notifyProjectStructureChange, notifyVersionChange, projectId, refreshAfterAgentFileChange])

  const handleReviewedWorkspaceChange = useCallback(async (
    paths: string[],
    metadata: WorkspaceChangeMetadata,
  ) => {
    const currentPath = selectedFile && paths.includes(selectedFile) ? selectedFile : undefined
    await handleAgentFileChange(currentPath, metadata.impact)
  }, [handleAgentFileChange, selectedFile])

  const handleWorkspaceChangeEvent = useCallback(async (event: WorkspaceChangeEvent) => {
    if (!isProjectChangeForProject(event, projectId)) return
    const paths = workspaceChangePaths(event)
    const path = selectedFile && paths.includes(selectedFile) ? selectedFile : paths[0]
    await handleAgentFileChange(path, workspaceChangeImpact(event))
  }, [handleAgentFileChange, projectId, selectedFile])

  const {
    messages,
    sessions,
    activeSessionId,
    sessionTransitionPending,
    isExecutionActive,
    runtimeProjection,
    abortPending,
    commandSubmitting,
    queueActionPendingCommandID,
    activityContent,
    references,
    styleScenes,
    textSelections,
    planMode,
    setPlanMode,
    togglePlanMode,
    send,
    analyzeContext,
    approveProposedPlan,
    exitPlanMode,
    stop,
    loadHistory,
    loadEarlierHistory,
    hasEarlierMessages,
    isLoadingEarlierHistory,
    createChatSession,
    switchChatSession,
    renameChatSession,
    deleteChatSession,
    steerQueuedCommand,
    deleteQueuedCommand,
    editQueuedCommand,
    addReference,
    removeReference,
    loreReferences,
    addLoreReference,
    removeLoreReference,
    addStyleScene,
    removeStyleScene,
    addTextSelection,
    removeTextSelection,
  } = useAgentChat({ projectId, onAgentFileChange: handleAgentFileChange, onWorkspaceChange: handleWorkspaceChangeEvent })

  const { notice, applyUpdateCheckResult, dismissNotice } = useWorkbenchNotice(writingAgentConversation)

  const handleChatPlanModeChange = useCallback((value: boolean) => {
    setPlanMode(value)
  }, [setPlanMode])

  const handleChatPlanModeToggle = useCallback(() => {
    togglePlanMode()
  }, [togglePlanMode])

  const refreshLoreItems = useCallback(async () => {
    if (!projectId) {
      setLoreItems([])
      return
    }
    try {
      setLoreItems(await getProjectLoreItems(projectId))
    } catch (e) {
      console.warn('[App.tsx] failed to load lore items', e)
      setLoreItems([])
    }
  }, [projectId])

  useEffect(() => {
    void refreshLoreItems()
    const onLoreUpdated = (event: Event) => {
      const detail = (event as CustomEvent<LoreUpdatedDetail>).detail
      if (detail?.projectId === projectId) void refreshLoreItems()
    }
    window.addEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
    return () => window.removeEventListener(LORE_UPDATED_EVENT, onLoreUpdated)
  }, [refreshLoreItems])

  const chapterStats = useMemo<Record<string, ChapterSummary>>(
    () => Object.fromEntries((summary?.chapters || []).map((chapter) => [chapter.path, chapter])),
    [summary?.chapters],
  )
  const currentChapter = selectedFile ? chapterStats[selectedFile] : undefined
  const currentBookName = workspaceLoaded
    ? summary?.title?.trim() ||
      books.find((book) => book.path === workspace)?.name?.trim() ||
      workspace.replace(/\/+$/, '').split('/').pop() ||
      t('workbench.noBook')
    : t('common.loading')

  const touchTab = useCallback((key: string) => {
    tabActivationCounterRef.current += 1
    tabActivationsRef.current.set(key, tabActivationCounterRef.current)
  }, [])

  const limitTabs = useCallback((tabs: Tab[], protectedKey: string | null): Tab[] => {
    return enforceTabLimit(tabs, protectedKey, maxOpenTabs, tabActivationsRef.current)
  }, [maxOpenTabs])

  useEffect(() => {
    let cancelled = false
    const target = projectId ? { kind: 'project' as const, projectId } : { kind: 'global' as const }
    const applySettings = (data: Awaited<ReturnType<typeof fetchSettings>>) => {
      if (cancelled) return
      const effective = data.effective
      const v = effective?.max_open_tabs
      if (typeof v === 'number' && v >= 1) setMaxOpenTabs(Math.floor(v))
      setEditorAutoSaveEnabled(effective?.auto_save_enabled ?? AUTO_SAVE_ENABLED_FALLBACK)
      setEditorAutoSaveDelayMs(normalizeAutoSaveDelayMs(effective?.auto_save_interval_ms))
      setUpdateCheckEnabled(effective?.update_check_enabled !== false)
      setDeveloperMode(effective?.labs?.developer_mode === true)
      setNovaDir(data.paths?.denova_dir || data.paths?.nova_dir || '')
      setConfiguredLocale(effective?.language)
      setTheme(normalizeAppTheme(effective?.theme))
      setMotionIntensity(normalizeMotionIntensity(effective?.motion_intensity))
      applyFontSettings(fontSettingsFromEffective(effective))
    }
    const reload = (fresh = false) => {
      const request = projectId
        ? (fresh ? refreshProjectSettings(projectId) : fetchProjectSettings(projectId))
        : (fresh ? refreshSettings() : fetchSettings())
      request
        .then(applySettings)
        .catch((e) => console.warn('[App.tsx] failed to load interface settings', e))
    }
    const workspaceChanged = workspaceLoaded
      && settingsWorkspaceRef.current !== null
      && settingsWorkspaceRef.current !== projectId
    if (workspaceLoaded) settingsWorkspaceRef.current = projectId
    reload(workspaceChanged)
    const unsubscribe = subscribeSettingsTarget(target, applySettings)
    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [projectId, setTheme, workspaceLoaded])

  useEffect(() => {
    const onUpdateCheckResult = (event: Event) => {
      const result = (event as CustomEvent<UpdateCheckResult>).detail
      if (result) applyUpdateCheckResult(result)
    }
    window.addEventListener(UPDATE_CHECK_RESULT_EVENT, onUpdateCheckResult)
    return () => window.removeEventListener(UPDATE_CHECK_RESULT_EVENT, onUpdateCheckResult)
  }, [applyUpdateCheckResult])

  useEffect(() => {
    const onConflictPreserved = (event: Event) => {
      const detail = (event as CustomEvent<AutosaveConflictPreservedDetail>).detail
      if (!detail?.id) return
      toast.warning(t('common.autosave.conflictPreserved'), {
        description: t('common.autosave.conflictPreservedDetail', { id: detail.id }),
      })
    }
    window.addEventListener(AUTOSAVE_CONFLICT_PRESERVED_EVENT, onConflictPreserved)
    return () => window.removeEventListener(AUTOSAVE_CONFLICT_PRESERVED_EVENT, onConflictPreserved)
  }, [t])

  useEffect(() => {
    if (updateCheckEnabled !== true || updateCheckInFlightRef.current || !shouldRunAutoUpdateCheck()) return
    updateCheckInFlightRef.current = true
    checkForUpdate()
      .then((result) => {
        applyUpdateCheckResult(result)
      })
      .catch((e) => console.warn('[App.tsx] automatic update check failed', e))
      .finally(() => {
        markAutoUpdateChecked()
        updateCheckInFlightRef.current = false
      })
  }, [applyUpdateCheckResult, updateCheckEnabled])

  useEffect(() => {
    if (activeTabKey) touchTab(activeTabKey)
  }, [activeTabKey, touchTab])

  useEffect(() => {
    setOpenTabs((prev) => limitTabs(prev, activeTabKey))
  }, [maxOpenTabs, activeTabKey, limitTabs])

  useEffect(() => { window.localStorage.setItem(PROJECT_VISIBLE_KEY, String(projectVisible)) }, [projectVisible])
  useEffect(() => { window.localStorage.setItem(ACTIVITY_BAR_EXPANDED_KEY, String(activityBarExpanded)) }, [activityBarExpanded])
  useEffect(() => { window.localStorage.setItem(INTERACTIVE_RIGHT_VISIBLE_KEY, String(interactiveRightVisible)) }, [interactiveRightVisible])
  useEffect(() => { window.localStorage.setItem(SETTINGS_OPEN_KEY, String(settingsOpen)) }, [settingsOpen])
  useEffect(() => { writeContentMode(lastCreationRoute) }, [lastCreationRoute])

  useEffect(() => {
    if (workspace || !workspaceLoaded) return
    setOpenTabs([])
    setActiveTabKey(null)
    clearSelectedFile()
    // User-owned surfaces, especially General Project AgentChat, remain usable
    // without a foreground Book. Only content modes need the Book picker fallback.
    if (workspaceModeRequiresBook(mode)) setMode('books')
  }, [clearSelectedFile, mode, setMode, workspace, workspaceLoaded])

  useEffect(() => {
    if (!workspaceLoaded) return
    if (!workspace) return

    let cancelled = false
    const restoreWorkspaceView = async () => {
      const tabs = readTabsFor(workspace)
      const storedKey = readActiveTabKeyFor(workspace)
      let activeKey = storedKey && tabs.some((tab) => tabKey(tab) === storedKey) ? storedKey : (tabs.length > 0 ? tabKey(tabs[0]) : null)
      tabActivationsRef.current = new Map()
      tabActivationCounterRef.current = 0
      for (const tab of tabs) touchTab(tabKey(tab))
      if (activeKey) touchTab(activeKey)
      let restoredTabs = limitTabs(tabs, activeKey)

      // A file can disappear while the app is closed. Resolve the restored target
      // before publishing the tab state so a deleted file cannot become a ghost tab.
      while (activeKey) {
        const target = restoredTabs.find((tab) => tabKey(tab) === activeKey)
        if (!target || target.kind !== 'file') break
        const result = await selectFile(target.path)
        if (cancelled) return
        if (result !== 'missing') break
        restoredTabs = restoredTabs.filter((tab) => tabKey(tab) !== activeKey)
        activeKey = restoredTabs.length > 0 ? tabKey(restoredTabs[0]) : null
      }

      if (cancelled) return
      setOpenTabs(restoredTabs)
      setActiveTabKey(activeKey)
      const target = activeKey ? restoredTabs.find((tab) => tabKey(tab) === activeKey) : null
      if (!target || target.kind !== 'file') clearSelectedFile()
    }
    void restoreWorkspaceView()
    return () => { cancelled = true }
  // This boundary intentionally follows workspace identity; callbacks remain stable.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspace, workspaceLoaded])

  useEffect(() => {
    try {
      persistTabsFor(workspace, openTabs)
    } catch (e) {
      console.warn('[App.tsx] failed to persist the tab list', e)
    }
  }, [openTabs, workspace])

  useEffect(() => {
    if (activeTabKey !== WRITING_SUBAGENT_TAB_KEY) persistActiveTabKeyFor(workspace, activeTabKey)
  }, [activeTabKey, workspace])

  useEffect(() => {
    if (!selectedFile) return
    const key = `file:${selectedFile}`
    setOpenTabs((prev) => {
      const next: Tab[] = prev.some((tab) => tabKey(tab) === key) ? prev : [...prev, { kind: 'file', path: selectedFile }]
      return limitTabs(next, key)
    })
    setActiveTabKey(key)
  }, [selectedFile, limitTabs])

  const handleWorkspaceSwitch = useCallback(async (newPath: string) => {
    await refreshAll()
    console.info('[App.tsx] Book Management switch synchronized', { workspace: newPath })
    setMode(lastCreationRouteRef.current)
    notifyVersionChange()
    notifyProjectStructureChange()
  }, [notifyProjectStructureChange, notifyVersionChange, refreshAll, setMode])

  const handleAgentChatBookCreated = useCallback(async (newPath: string) => {
    await refreshAll()
    console.info('[App.tsx] synchronized the Book created from Agent Chat', { workspace: newPath })
    notifyVersionChange()
    notifyProjectStructureChange()
  }, [notifyProjectStructureChange, notifyVersionChange, refreshAll])

  const handleQuickWorkspaceSwitch = useCallback(async (newPath: string): Promise<boolean> => {
    if (!newPath || newPath === workspace) return true
    if (!(await flushEditorDraft())) return false
    try {
      const result = await switchWorkspace(newPath)
      const nextWorkspace = result.workspace || newPath
      console.info('[App.tsx] title-bar Book switch completed', { from: workspace, to: nextWorkspace })
      await refreshAll()
      notifyVersionChange()
      notifyProjectStructureChange()
      return true
    } catch (error) {
      console.error('[App.tsx] title-bar Book switch failed', { from: workspace, to: newPath, error })
      toast.error(t('workbench.bookSwitcher.switchError'), {
        description: error instanceof Error ? error.message : String(error),
      })
      return false
    }
  }, [flushEditorDraft, notifyProjectStructureChange, notifyVersionChange, refreshAll, t, workspace])

  const handleSaveCurrentFile = useCallback(async (path: string, content: string, baseRevision: string) => {
    const saved = await saveFileDraft(path, content, baseRevision)
    notifyVersionChange()
    return saved
  }, [notifyVersionChange, saveFileDraft])

  const handleCreateItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await createItem(path, type)
    notifyVersionChange()
    notifyProjectStructureChange()
  }, [createItem, notifyProjectStructureChange, notifyVersionChange])

  const handleDeleteItem = useCallback(async (path: string) => {
    if ((selectedFile === path || selectedFile?.startsWith(`${path}/`)) && !(await flushEditorDraft())) return
    await deleteItem(path)
    setOpenTabs((prev) => prev.filter((tab) => tab.kind !== 'file' || (tab.path !== path && !tab.path.startsWith(`${path}/`))))
    notifyVersionChange()
    notifyProjectStructureChange()
  }, [deleteItem, flushEditorDraft, notifyProjectStructureChange, notifyVersionChange, selectedFile])

  const handleRenameItem = useCallback(async (path: string, newName: string) => {
    if ((selectedFile === path || selectedFile?.startsWith(`${path}/`)) && !(await flushEditorDraft())) return
    await renameItem(path, newName)
    const parent = path.replace(/\/[^/]*$/, '')
    const newPath = parent ? `${parent}/${newName}` : newName
    setOpenTabs((prev) => dedupeTabs(prev.map((tab) => {
      if (tab.kind !== 'file') return tab
      if (tab.path === path) return { ...tab, path: newPath }
      if (tab.path.startsWith(`${path}/`)) return { ...tab, path: `${newPath}${tab.path.slice(path.length)}` }
      return tab
    })))
    notifyVersionChange()
    notifyProjectStructureChange()
  }, [flushEditorDraft, notifyProjectStructureChange, notifyVersionChange, renameItem, selectedFile])

  const handleCopyItem = useCallback(async (from: string, to: string) => {
    await copyItem(from, to)
    notifyVersionChange()
    notifyProjectStructureChange()
  }, [copyItem, notifyProjectStructureChange, notifyVersionChange])

  const handleMoveItem = useCallback(async (from: string, to: string) => {
    if ((selectedFile === from || selectedFile?.startsWith(`${from}/`)) && !(await flushEditorDraft())) return
    await moveItem(from, to)
    setOpenTabs((prev) => dedupeTabs(prev.map((tab) => {
      if (tab.kind !== 'file') return tab
      if (tab.path === from) return { ...tab, path: to }
      if (tab.path.startsWith(`${from}/`)) return { ...tab, path: `${to}${tab.path.slice(from.length)}` }
      return tab
    })))
    notifyVersionChange()
    notifyProjectStructureChange()
  }, [flushEditorDraft, moveItem, notifyProjectStructureChange, notifyVersionChange, selectedFile])

  const handleSelectFile = useCallback(async (path: string) => {
    const key = `file:${path}`
    if (selectedFile === path && activeTabKey === key) return true
    if (selectedFile !== path && !(await flushEditorDraft())) return false
    if (selectedFile !== path) {
      const result = await selectFile(path)
      if (result === 'missing') {
        setOpenTabs((prev) => prev.filter((tab) => tabKey(tab) !== key))
      }
      if (result !== 'selected') return false
    }
    setSelectedChapterId(path)
    setOpenTabs((prev) => {
      const next: Tab[] = prev.some((tab) => tabKey(tab) === key) ? prev : [...prev, { kind: 'file', path }]
      return limitTabs(next, key)
    })
    setActiveTabKey(key)
    return true
  }, [activeTabKey, flushEditorDraft, limitTabs, selectFile, selectedFile, setSelectedChapterId])

  const handleOpenLoreTab = useCallback(async () => {
    const key = tabKey({ kind: 'lore' })
    if (activeTabKey === key) return true
    if (!(await flushEditorDraft())) return false
    setOpenTabs((current) => {
      const next: Tab[] = current.some((tab) => tab.kind === 'lore')
        ? current
        : [...current, { kind: 'lore' }]
      return limitTabs(next, key)
    })
    clearSelectedFile()
    setActiveTabKey(key)
    return true
  }, [activeTabKey, clearSelectedFile, flushEditorDraft, limitTabs])

  const handleSelectSearchResult = useCallback(async (result: WorkspaceSearchResult, query: string) => {
    setSettingsOpen(false)
    setMode('ide')
    setProjectVisible(true)
    setSidebarView('search')
    if (!(await handleSelectFile(result.path))) return
    setEditorSearchIntent({
      path: result.path,
      query,
      line: result.line,
      nonce: Date.now(),
    })
  }, [handleSelectFile, setMode])

  const resetCharacterCardImport = useCallback(() => {
    setCharacterCardFile(null)
    setCharacterCardPreview(null)
    setCharacterCardTargetMode('new_book')
    setCharacterCardSemanticClassification(true)
    setCharacterCardBookTitle('')
    setCharacterCardUserName('')
    setCharacterCardPreviewing(false)
    setCharacterCardImporting(false)
    setCharacterCardError('')
    if (characterCardInputRef.current) {
      characterCardInputRef.current.value = ''
    }
  }, [])

  const handleCharacterCardDialogOpenChange = useCallback((open: boolean) => {
    setCharacterCardDialogOpen(open)
    if (!open) resetCharacterCardImport()
    if (open) setCharacterCardTargetMode('new_book')
  }, [resetCharacterCardImport])

  const handleOpenCharacterCardImportFromBooks = useCallback(() => {
    handleCharacterCardDialogOpenChange(true)
  }, [handleCharacterCardDialogOpenChange])

  const handleCharacterCardSelected = useCallback(async (file: File | undefined) => {
    if (!file) return
    setCharacterCardFile(file)
    setCharacterCardPreview(null)
    setCharacterCardTargetMode('new_book')
    setCharacterCardBookTitle('')
    setCharacterCardUserName('')
    setCharacterCardError('')
    setCharacterCardPreviewing(true)
    try {
      const preview = await previewCharacterCard(file)
      setCharacterCardPreview(preview)
      setCharacterCardBookTitle(preview.name)
      setCharacterCardUserName(preview.user_placeholder_found ? t('importCard.defaultUserCharacterName') : '')
    } catch (e) {
      setCharacterCardError(e instanceof Error ? e.message : t('importCard.previewFailed'))
    } finally {
      setCharacterCardPreviewing(false)
      if (characterCardInputRef.current) {
        characterCardInputRef.current.value = ''
      }
    }
  }, [t])

  const handleCharacterCardImport = useCallback(async () => {
    if (!characterCardFile) {
      setCharacterCardError(t('importCard.chooseFileFirst'))
      return
    }
    if (characterCardTargetMode === 'current' && !projectId) {
      setCharacterCardError(t('importCard.noCurrentBookImportNew'))
      return
    }
    setCharacterCardImporting(true)
    setCharacterCardError('')
    try {
      const result = await importCharacterCard(characterCardFile, {
        targetMode: characterCardTargetMode,
        projectId: characterCardTargetMode === 'current' ? projectId : undefined,
        bookTitle: characterCardTargetMode === 'new_book' ? characterCardBookTitle.trim() : undefined,
        userCharacterName: characterCardPreview?.user_placeholder_found ? characterCardUserName.trim() : undefined,
        loreClassification: characterCardSemanticClassification ? 'semantic' : 'heuristic',
      })
      toast.success(result.message || t('importCard.importSuccess', { name: result.name }))
      if (characterCardTargetMode === 'new_book') {
        await refreshAll()
      } else {
        await refresh()
      }
      setMode('lore')
      window.setTimeout(() => {
        notifyLoreUpdated({
          projectId: result.project_id || projectId,
          ids: result.item_ids,
          source: 'character-card-import',
        })
      }, 0)
      notifyVersionChange()
      notifyProjectStructureChange()
      setCharacterCardDialogOpen(false)
      resetCharacterCardImport()
    } catch (e) {
      const message = e instanceof Error ? e.message : t('importCard.importFailed')
      setCharacterCardError(message)
      toast.error(message)
    } finally {
      setCharacterCardImporting(false)
    }
  }, [characterCardBookTitle, characterCardFile, characterCardPreview, characterCardSemanticClassification, characterCardTargetMode, characterCardUserName, notifyProjectStructureChange, notifyVersionChange, projectId, refresh, refreshAll, resetCharacterCardImport, setMode, t])

  const handleActivateTab = useCallback(async (tab: Tab) => {
    const key = tabKey(tab)
    if (tab.kind === 'subagent') {
      setActiveTabKey(key)
      return
    }
    if (tab.kind === 'lore') {
      await handleOpenLoreTab()
      return
    }
    if (selectedFile === tab.path) {
      setActiveTabKey(key)
      return
    }
    await handleSelectFile(tab.path)
  }, [handleOpenLoreTab, handleSelectFile, selectedFile])

  const handleCloseTab = useCallback(async (tab: Tab) => {
    const key = tabKey(tab)
    const idx = openTabs.findIndex((item) => tabKey(item) === key)
    if (idx === -1) return
    if (activeTabKey === key && tab.kind !== 'subagent' && !(await flushEditorDraft())) return
    const next = openTabs.filter((item) => tabKey(item) !== key)
    setOpenTabs(next)
    if (activeTabKey !== key) return
    if (next.length === 0) {
      setActiveTabKey(null)
      clearSelectedFile()
      return
    }
    const preferredReturnKey = tab.kind === 'subagent' ? tab.returnTabKey : null
    const fallback = (preferredReturnKey ? next.find((item) => tabKey(item) === preferredReturnKey) : null)
      ?? next[idx]
      ?? next[idx - 1]
      ?? next[0]
    await handleActivateTab(fallback)
  }, [activeTabKey, clearSelectedFile, flushEditorDraft, handleActivateTab, openTabs])

  const handleOpenWritingSubAgentSession = useCallback(async (target: AgentSubAgentSessionTarget) => {
    if (target.parentSessionId !== writingAgentConversation.sessionId) return
    if (activeTabKey !== WRITING_SUBAGENT_TAB_KEY && !(await flushEditorDraft())) return
    setOpenTabs((current) => {
      const existing = current.find((tab) => tab.kind === 'subagent')
      const returnTabKey = existing?.kind === 'subagent' && activeTabKey === WRITING_SUBAGENT_TAB_KEY
        ? existing.returnTabKey
        : activeTabKey
      const nextTab: Tab = {
        kind: 'subagent',
        parentSessionId: target.parentSessionId,
        sessionKey: target.sessionKey,
        title: target.name,
        returnTabKey,
      }
      return existing
        ? current.map((tab) => tab.kind === 'subagent' ? nextTab : tab)
        : [...current, nextTab]
    })
    setActiveTabKey(WRITING_SUBAGENT_TAB_KEY)
  }, [activeTabKey, flushEditorDraft, writingAgentConversation.sessionId])

  useEffect(() => {
    if (!writingAgentConversation.sessionId) return
    const subAgentTab = openTabs.find((tab) => tab.kind === 'subagent')
    if (!subAgentTab || subAgentTab.parentSessionId === writingAgentConversation.sessionId) return
    void handleCloseTab(subAgentTab)
  }, [handleCloseTab, openTabs, writingAgentConversation.sessionId])

  const handleToggleTabPin = useCallback((tab: Tab) => {
    setOpenTabs((current) => setTabPinned(current, tabKey(tab), !tab.pinned))
  }, [])

  const handleMoveTab = useCallback((sourceKey: string, targetKey: string) => {
    setOpenTabs((current) => reorderTabs(current, sourceKey, targetKey))
  }, [])

  const triggerSave = useCallback(() => setSaveSignal((value) => value + 1), [])
  const continueWriting = useCallback(() => {
    setMode('ide')
    setRightPanel('ai')
    window.requestAnimationFrame(() => {
      window.dispatchEvent(new CustomEvent(WRITING_AGENT_INIT_EVENT, {
        detail: { prompt: t('command.continueWritingPrompt'), autoSend: true },
      }))
    })
  }, [setMode, setRightPanel, t])

  const handleSetMode = useCallback((nextMode: WorkspaceMode) => {
    if (nextMode === 'ide' || nextMode === 'interactive') {
      lastCreationRouteRef.current = nextMode
      setLastCreationRoute(nextMode)
    }
    setSettingsOpen(false)
    setMode(nextMode)
  }, [setMode])
  useEffect(() => {
    if (developerMode === false && mode === 'trajectory') handleSetMode(lastCreationRouteRef.current)
  }, [developerMode, handleSetMode, mode])
  const handleSetRightPanel = useCallback((panel: RightPanel) => {
    setSettingsOpen(false)
    setRightPanel(panel)
  }, [setRightPanel])
  const handleOpenVersions = useCallback(() => {
    handleSetMode('versions')
  }, [handleSetMode])

  const handleSetChapterConfirmed = useCallback(async (path: string, confirmed: boolean) => {
    if (!projectId) return
    await setProjectChapterConfirmed(projectId, path, confirmed)
    await refreshSummary({ showLoading: false, clearOnError: false })
  }, [projectId, refreshSummary])

  const handleOpenGlobalSearch = useCallback(() => {
    setSettingsOpen(false)
    setMode('ide')
    setProjectVisible(true)
    setSidebarView('search')
  }, [setMode])

  const handleOnboardingNavigate = useCallback((target: OnboardingNavigationTarget, prompt?: string) => {
    if (target === 'settings-model') {
      setSettingsOpen(true)
      window.setTimeout(() => {
        window.dispatchEvent(new CustomEvent(SETTINGS_SECTION_EVENT, {
          detail: { section: 'model' },
        }))
      }, 0)
      return
    }
    if (target === 'books') {
      handleSetMode('books')
      return
    }
    if (target === 'writing') {
      handleSetMode('ide')
      return
    }
    if (target === 'writing-agent') {
      handleSetMode('ide')
      handleSetRightPanel('ai')
      if (prompt) {
        window.setTimeout(() => {
          window.dispatchEvent(new CustomEvent(WRITING_AGENT_INIT_EVENT, { detail: { prompt } }))
        }, 0)
      }
      return
    }
    if (target === 'interactive') {
      useInteractiveStore.getState().setSubmode('story')
      handleSetMode('interactive')
      return
    }
    if (target === 'lore') {
      handleSetMode('lore')
      return
    }
    if (target === 'teller') {
      handleSetMode('presets')
      return
    }
    if (target === 'versions') {
      handleOpenVersions()
      return
    }
    if (target === 'skills') {
      handleSetMode('skills')
      return
    }
    if (target === 'agents') {
      handleSetMode('agents')
      return
    }
    if (target === 'automations') {
      handleSetMode('automations')
    }
  }, [handleOpenVersions, handleSetMode, handleSetRightPanel])

  useWorkspaceHotkeys({
    onSave: triggerSave,
    onOpenCommand: () => setCommandOpen(true),
    onOpenSearch: handleOpenGlobalSearch,
    onGenerate: continueWriting,
    onOpenDiff: handleOpenVersions,
    onToggleRightPanel: () => {
      if (mode === 'interactive') {
        setInteractiveRightVisible((value) => !value)
        return
      }
      if (mode === 'ide') handleSetRightPanel(rightPanel ? null : 'ai')
    },
  })

  return (
    <NovaMotionProvider intensity={motionIntensity}>
      <ModeRouter
        mode={mode}
        lastCreationRoute={lastCreationRoute}
        currentBookName={currentBookName}
        workspace={workspace}
        projectId={projectId}
        summary={summary}
        currentChapter={currentChapter}
        isStreaming={writingAgentConversation.isStreaming}
        sessionTransitionPending={sessionTransitionPending}
        isExecutionActive={isExecutionActive}
        runtimeProjection={runtimeProjection}
        abortPending={abortPending}
        commandSubmitting={commandSubmitting}
        queueActionPendingCommandID={queueActionPendingCommandID}
        projectVisible={projectVisible}
        activityBarExpanded={activityBarExpanded}
        rightPanel={rightPanel}
        settingsOpen={settingsOpen}
        developerMode={developerMode === true}
        interactiveRightVisible={interactiveRightVisible}
        novaDir={novaDir}
        books={books}
        bookSortMode={bookSortMode}
        tree={tree}
        loading={loading}
        selectedFile={selectedFile}
        fileDocument={fileDocument}
        fileContent={fileContent}
        fileRevision={fileRevision}
        openTabs={openTabs}
        activeTabKey={activeTabKey}
        sidebarView={sidebarView}
        editorSearchIntent={editorSearchIntent}
        saveSignal={saveSignal}
        editorAutoSaveEnabled={editorAutoSaveEnabled}
        editorAutoSaveDelayMs={editorAutoSaveDelayMs}
        projectExplorerRefreshSignal={projectExplorerRefreshSignal}
        versionRefreshSignal={versionRefreshSignal}
        messages={messages}
        sessions={sessions}
        activeSessionId={activeSessionId}
        activityContent={activityContent}
        references={references}
        loreReferences={loreReferences}
        loreItems={loreItems}
        styleScenes={styleScenes}
        textSelections={textSelections}
        writingAgentConversation={writingAgentConversation}
        onWritingAgentConversationStateChange={setWritingAgentConversation}
        onOpenWritingSubAgentSession={handleOpenWritingSubAgentSession}
        chatPlanMode={planMode}
        hasEarlierMessages={hasEarlierMessages}
        isLoadingEarlierHistory={isLoadingEarlierHistory}
        onSetMode={handleSetMode}
        onToggleActivityBarExpanded={toggleActivityBarExpanded}
        onToggleProjectVisible={toggleProjectVisible}
        onSetRightPanel={handleSetRightPanel}
        onToggleSettings={toggleSettings}
        onCloseSettings={closeSettings}
        notice={notice}
        onDismissNotice={dismissNotice}
        onToggleInteractiveRightPanel={toggleInteractiveRightPanel}
        onSwitchBook={handleWorkspaceSwitch}
        onQuickSwitchBook={handleQuickWorkspaceSwitch}
        onBeforeWorkspaceSwitch={flushEditorDraft}
        onBooksChange={refreshBooks}
        onAgentChatBookCreated={handleAgentChatBookCreated}
        onOpenCharacterCardImport={handleOpenCharacterCardImportFromBooks}
        onSetSidebarView={setSidebarView}
        onSelectSearchResult={handleSelectSearchResult}
        onSelectFile={handleSelectFile}
        onSetChapterConfirmed={handleSetChapterConfirmed}
        onReferenceFile={addReference}
        onCreateItem={handleCreateItem}
        onDeleteItem={handleDeleteItem}
        onRenameItem={handleRenameItem}
        onCopyItem={handleCopyItem}
        onMoveItem={handleMoveItem}
        onRefreshWorkspace={refresh}
        onActivateTab={handleActivateTab}
        onCloseTab={handleCloseTab}
        onToggleTabPin={handleToggleTabPin}
        onMoveTab={handleMoveTab}
        onOpenLoreTab={handleOpenLoreTab}
        onSaveCurrentFile={handleSaveCurrentFile}
        onEditorFlushHandlerChange={handleEditorFlushHandlerChange}
        onWorkspaceChanged={handleReviewedWorkspaceChange}
        onQuoteSelection={addTextSelection}
        onCreateChatSession={createChatSession}
        onSwitchChatSession={switchChatSession}
        onRenameChatSession={renameChatSession}
        onDeleteChatSession={deleteChatSession}
        onLoadEarlierHistory={loadEarlierHistory}
        onRefreshChatHistory={loadHistory}
        onSend={send}
        onAnalyzeContext={analyzeContext}
        onStop={stop}
        onSteerQueuedCommand={steerQueuedCommand}
        onDeleteQueuedCommand={deleteQueuedCommand}
        onEditQueuedCommand={editQueuedCommand}
        onReferenceRemove={removeReference}
        onLoreReferenceAdd={addLoreReference}
        onLoreReferenceRemove={removeLoreReference}
        onStyleSceneAdd={addStyleScene}
        onStyleSceneRemove={removeStyleScene}
        onTextSelectionRemove={removeTextSelection}
        onChatPlanModeChange={handleChatPlanModeChange}
        onChatPlanModeToggle={handleChatPlanModeToggle}
        onApproveProposedPlan={approveProposedPlan}
        onExitChatPlanMode={exitPlanMode}
      />
      <CommandPalette
        open={commandOpen}
        isStreaming={writingAgentConversation.isStreaming}
        onOpenChange={setCommandOpen}
        onSave={triggerSave}
        onOpenAgent={() => {
          setMode('ide')
          handleSetRightPanel('ai')
        }}
        onOpenVersions={handleOpenVersions}
        onOpenSearch={handleOpenGlobalSearch}
        onContinueWriting={continueWriting}
        onToggleRightPanel={() => {
          if (mode === 'interactive') {
            setInteractiveRightVisible((value) => !value)
            return
          }
          if (mode === 'ide') handleSetRightPanel(rightPanel ? null : 'ai')
        }}
      />
      <CharacterCardImportDialog
        open={characterCardDialogOpen}
        workspace={workspace}
        currentBookName={currentBookName}
        novaDir={novaDir}
        file={characterCardFile}
        preview={characterCardPreview}
        targetMode={characterCardTargetMode}
        bookTitle={characterCardBookTitle}
        userCharacterName={characterCardUserName}
        semanticClassification={characterCardSemanticClassification}
        previewing={characterCardPreviewing}
        importing={characterCardImporting}
        error={characterCardError}
        fileInputRef={characterCardInputRef}
        onOpenChange={handleCharacterCardDialogOpenChange}
        onFileSelected={handleCharacterCardSelected}
        onTargetModeChange={setCharacterCardTargetMode}
        onBookTitleChange={setCharacterCardBookTitle}
        onUserCharacterNameChange={setCharacterCardUserName}
        onSemanticClassificationChange={setCharacterCardSemanticClassification}
        onImport={handleCharacterCardImport}
      />
      <OnboardingGuide
        mode={mode}
        rightPanel={rightPanel}
        settingsOpen={settingsOpen}
        workspace={workspace}
        booksCount={books.length}
        currentBookName={currentBookName}
        messages={writingAgentConversation.messages}
        isStreaming={writingAgentConversation.isStreaming}
        onNavigate={handleOnboardingNavigate}
      />
      <RemoteAccessLogin />
    </NovaMotionProvider>
  )
}

function readLayoutBoolean(key: string, fallback: boolean) {
  if (typeof window === 'undefined') return fallback
  const value = window.localStorage.getItem(key)
  if (value === null) return fallback
  return value === 'true'
}

function readContentMode(): CreationRoute {
  if (typeof window === 'undefined') return 'ide'
  const value = window.localStorage.getItem(CONTENT_MODE_STORAGE_KEY)
  return value === 'interactive' ? 'interactive' : 'ide'
}

function writeContentMode(mode: CreationRoute) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(CONTENT_MODE_STORAGE_KEY, mode)
}

function normalizeAutoSaveDelayMs(value: number | null | undefined) {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return AUTO_SAVE_DELAY_FALLBACK_MS
  }
  return Math.floor(value)
}

function normalizeAppTheme(theme?: string) {
  if (theme === 'light' || theme === 'dark' || theme === 'system') return theme
  return 'dark'
}

export default App
