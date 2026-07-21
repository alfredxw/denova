import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { checkForUpdate, fetchSettings } from '@/features/settings/api'
import { applyFontSettings, fontSettingsFromEffective } from '@/features/settings/font-variables'
import { markAutoUpdateChecked, shouldRunAutoUpdateCheck, UPDATE_CHECK_RESULT_EVENT } from '@/features/settings/update-check-cache'
import type { UpdateCheckResult } from '@/features/settings/types'
import { getLoreItems, importCharacterCard, previewCharacterCard, setChapterConfirmed, switchWorkspace, type CharacterCardPreview, type LoreItem, type WorkspaceSearchResult } from '@/lib/api'
import { CommandPalette } from '@/components/common/command-palette'
import { useWorkspace } from '@/hooks/useWorkspace'
import { useAgentChat } from '@/hooks/useAgentChat'
import { useWorkspaceHotkeys } from '@/hooks/use-workspace-hotkeys'
import { useWorkspaceStore, type RightPanel, type WorkspaceMode } from '@/stores/workspace-store'
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
  tabKey,
  type Tab,
} from '@/components/workbench/TabController'
import { ModeRouter } from '@/components/workbench/ModeRouter'
import type { EditorFlushHandler } from '@/components/Editor/MarkdownEditor'
import {
  CharacterCardImportDialog,
  type CharacterCardTargetMode,
} from '@/components/workbench/CharacterCardImportDialog'
import { APP_VERSION } from '@/app-version'
import { RemoteAccessLogin } from '@/components/RemoteAccessLogin'
import { OnboardingGuide, type OnboardingNavigationTarget } from '@/features/onboarding/OnboardingGuide'
import { SETTINGS_SECTION_EVENT, WRITING_AGENT_INIT_EVENT } from '@/features/onboarding/events'
import { isWorkspaceChangeForWorkspace, type WorkspaceChangeEvent } from '@/features/changes/types'
import {
  AUTOSAVE_CONFLICT_PRESERVED_EVENT,
  type AutosaveConflictPreservedDetail,
} from '@/lib/autosave/rebase-with-recovery'
import { useWorkbenchNotice } from '@/features/notices/use-workbench-notice'

const PROJECT_VISIBLE_KEY = 'nova.layout.projectVisible'
const ACTIVITY_BAR_EXPANDED_KEY = 'nova.layout.activityBarExpanded'
const INTERACTIVE_RIGHT_VISIBLE_KEY = 'nova.layout.interactiveRightVisible'
const SETTINGS_OPEN_KEY = 'nova.layout.settingsOpen'
const CONTENT_MODE_STORAGE_KEY = 'nova:content-mode'
const MAX_OPEN_TABS_FALLBACK = 5
const AUTO_SAVE_ENABLED_FALLBACK = true
const AUTO_SAVE_DELAY_FALLBACK_MS = 1500
type SidebarView = 'outline' | 'files' | 'search'
type WritingRightPanel = Extract<RightPanel, 'ai'> | null
type BooksReturnMode = 'ide' | 'interactive'

function App() {
  const { t } = useTranslation()
  const { setTheme } = useTheme()
  const [projectVisible, setProjectVisible] = useState(() => readLayoutBoolean(PROJECT_VISIBLE_KEY, true))
  const [activityBarExpanded, setActivityBarExpanded] = useState(() => readLayoutBoolean(ACTIVITY_BAR_EXPANDED_KEY, true))
  const [interactiveRightVisible, setInteractiveRightVisible] = useState(() => readLayoutBoolean(INTERACTIVE_RIGHT_VISIBLE_KEY, true))
  const [saveSignal, setSaveSignal] = useState(0)
  const [versionRefreshSignal, setVersionRefreshSignal] = useState(0)
  const [settingsOpen, setSettingsOpen] = useState(() => readLayoutBoolean(SETTINGS_OPEN_KEY, false))
  const [openTabs, setOpenTabs] = useState<Tab[]>([])
  const [activeTabKey, setActiveTabKey] = useState<string | null>(null)
  const [maxOpenTabs, setMaxOpenTabs] = useState<number>(MAX_OPEN_TABS_FALLBACK)
  const [editorAutoSaveEnabled, setEditorAutoSaveEnabled] = useState(AUTO_SAVE_ENABLED_FALLBACK)
  const [editorAutoSaveDelayMs, setEditorAutoSaveDelayMs] = useState(AUTO_SAVE_DELAY_FALLBACK_MS)
  const [updateCheckEnabled, setUpdateCheckEnabled] = useState<boolean | null>(null)
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
  const [booksReturnMode, setBooksReturnMode] = useState<BooksReturnMode>(() => readContentMode())
  const booksReturnModeRef = useRef<BooksReturnMode>(readContentMode())
  const writingRightPanelRef = useRef<WritingRightPanel>('ai')
  const characterCardInputRef = useRef<HTMLInputElement>(null)
  const chatWorkspaceRef = useRef('')
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
  const workspaceAutoRefreshEnabled = mode === 'ide' && !settingsOpen && (rightPanel === 'ai' || rightPanel === null)

  useEffect(() => {
    if (mode === 'books' || mode === 'skills' || mode === 'agents' || mode === 'automations') return
    const contentMode = mode === 'interactive' ? 'interactive' : 'ide'
    booksReturnModeRef.current = contentMode
    setBooksReturnMode(contentMode)
  }, [mode])

  const {
    tree, loading, selectedFile, fileContent, fileRevision, workspace, workspaceLoaded, summary, books, bookSortMode,
    selectFile, clearSelectedFile, saveFileDraft, createItem, deleteItem, renameItem, copyItem, moveItem,
    refresh, refreshSummary, refreshAfterAgentFileChange, refreshAll, refreshBooks, setWorkspace,
  } = useWorkspace({ autoRefreshEnabled: workspaceAutoRefreshEnabled })

  const notifyVersionChange = useCallback(() => {
    setVersionRefreshSignal(value => value + 1)
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
      console.error('导航前保存编辑器草稿失败', error)
      toast.error(t('editor.saveFailed'))
      return false
    }
  }, [t])

  const handleAgentFileChange = useCallback(async (path?: string) => {
    await refreshAfterAgentFileChange(path)
    notifyVersionChange()
  }, [notifyVersionChange, refreshAfterAgentFileChange])

  const handleReviewedWorkspaceChange = useCallback(async (paths: string[]) => {
    const currentPath = selectedFile && paths.includes(selectedFile) ? selectedFile : undefined
    await handleAgentFileChange(currentPath)
  }, [handleAgentFileChange, selectedFile])

  const handleWorkspaceChangeEvent = useCallback(async (event: WorkspaceChangeEvent) => {
    if (!isWorkspaceChangeForWorkspace(event, workspace)) return
    const paths = Array.from(new Set([...(event.affected_paths ?? []), ...(event.paths ?? []), ...(event.path ? [event.path] : [])]))
    const path = selectedFile && paths.includes(selectedFile) ? selectedFile : paths[0]
    await handleAgentFileChange(path)
  }, [handleAgentFileChange, selectedFile, workspace])

  const {
    messages,
    sessions,
    activeSessionId,
    isStreaming,
    activityContent,
    references,
    styleScenes,
    textSelections,
    planMode,
    setPlanMode,
    togglePlanMode,
    send,
    analyzeContext,
    submitPlanQuestion,
    approveProposedPlan,
    exitPlanMode,
    stop,
    loadSessions,
    loadHistory,
    loadEarlierHistory,
    hasEarlierMessages,
    isLoadingEarlierHistory,
    resumeActiveChat,
    createChatSession,
    switchChatSession,
    renameChatSession,
    deleteChatSession,
    addReference,
    removeReference,
    loreReferences,
    addLoreReference,
    removeLoreReference,
    addStyleScene,
    removeStyleScene,
    addTextSelection,
    removeTextSelection,
  } = useAgentChat({ workspace, onAgentFileChange: handleAgentFileChange, onWorkspaceChange: handleWorkspaceChangeEvent })

  const { notice, applyUpdateCheckResult, dismissNotice } = useWorkbenchNotice({ messages, isStreaming })

  const handleChatPlanModeChange = useCallback((value: boolean) => {
    setPlanMode(value)
  }, [setPlanMode])

  const handleChatPlanModeToggle = useCallback(() => {
    togglePlanMode()
  }, [togglePlanMode])

  const refreshLoreItems = useCallback(async () => {
    if (!workspace) {
      setLoreItems([])
      return
    }
    try {
      setLoreItems(await getLoreItems())
    } catch (e) {
      console.warn('加载资料库条目失败', e)
      setLoreItems([])
    }
  }, [workspace])

  useEffect(() => {
    void refreshLoreItems()
    const onLoreUpdated = () => void refreshLoreItems()
    window.addEventListener('nova:lore-updated', onLoreUpdated)
    return () => window.removeEventListener('nova:lore-updated', onLoreUpdated)
  }, [refreshLoreItems])

  const chapterStats: Record<string, ChapterSummary> = Object.fromEntries((summary?.chapters || []).map((chapter) => [chapter.path, chapter]))
  const currentChapter = selectedFile ? chapterStats[selectedFile] : undefined
  const currentBookName = summary?.title?.trim() ||
    books.find((book) => book.path === workspace)?.name?.trim() ||
    workspace.replace(/\/+$/, '').split('/').pop() ||
    t('workbench.noBook')

  const touchTab = useCallback((key: string) => {
    tabActivationCounterRef.current += 1
    tabActivationsRef.current.set(key, tabActivationCounterRef.current)
  }, [])

  const limitTabs = useCallback((tabs: Tab[], protectedKey: string | null): Tab[] => {
    return enforceTabLimit(tabs, protectedKey, maxOpenTabs, tabActivationsRef.current)
  }, [maxOpenTabs])

  useEffect(() => {
    if (!workspaceLoaded || !workspace || chatWorkspaceRef.current === workspace) return
    chatWorkspaceRef.current = workspace
    void Promise.all([loadSessions(), loadHistory()]).then(() => resumeActiveChat())
  }, [loadHistory, loadSessions, resumeActiveChat, workspace, workspaceLoaded])

  useEffect(() => {
    let cancelled = false
    const reload = () => {
      fetchSettings()
        .then((data) => {
          if (cancelled) return
          const effective = data?.effective
          const v = effective?.max_open_tabs
          if (typeof v === 'number' && v >= 1) setMaxOpenTabs(Math.floor(v))
          setEditorAutoSaveEnabled(effective?.auto_save_enabled ?? AUTO_SAVE_ENABLED_FALLBACK)
          setEditorAutoSaveDelayMs(normalizeAutoSaveDelayMs(effective?.auto_save_interval_ms))
          setUpdateCheckEnabled(effective?.update_check_enabled !== false)
          setNovaDir(data?.paths?.denova_dir || data?.paths?.nova_dir || '')
          setConfiguredLocale(effective?.language)
          setTheme(normalizeAppTheme(effective?.theme))
          setMotionIntensity(normalizeMotionIntensity(effective?.motion_intensity))
          applyFontSettings(fontSettingsFromEffective(effective))
        })
        .catch((e) => console.warn('加载界面配置失败', e))
    }
    reload()
    const onUpdated = () => reload()
    window.addEventListener('nova:settings-updated', onUpdated)
    return () => {
      cancelled = true
      window.removeEventListener('nova:settings-updated', onUpdated)
    }
  }, [setTheme, workspace])

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
      .catch((e) => console.warn('[updates] 自动检查更新失败', e))
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
  useEffect(() => { writeContentMode(booksReturnMode) }, [booksReturnMode])

  useEffect(() => {
    if (workspace || !workspaceLoaded) return
    setOpenTabs([])
    setActiveTabKey(null)
    clearSelectedFile()
    if (mode !== 'books') setMode('books')
  }, [clearSelectedFile, mode, setMode, workspace, workspaceLoaded])

  useEffect(() => {
    if (!workspace) return
    const tabs = readTabsFor(workspace)
    const storedKey = readActiveTabKeyFor(workspace)
    const activeKey = storedKey && tabs.some((tab) => tabKey(tab) === storedKey) ? storedKey : (tabs.length > 0 ? tabKey(tabs[0]) : null)
    tabActivationsRef.current = new Map()
    tabActivationCounterRef.current = 0
    for (const tab of tabs) touchTab(tabKey(tab))
    if (activeKey) touchTab(activeKey)
    const limited = limitTabs(tabs, activeKey)
    setOpenTabs(limited)
    setActiveTabKey(activeKey)
    if (activeKey) {
      const target = tabs.find((tab) => tabKey(tab) === activeKey)
      if (target) {
        void selectFile(target.path)
      } else {
        clearSelectedFile()
      }
    } else {
      clearSelectedFile()
    }
  // 仅在 workspace 变更时触发；selectFile/clearSelectedFile 引用稳定
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspace])

  useEffect(() => {
    try {
      persistTabsFor(workspace, openTabs)
    } catch (e) {
      console.warn('保存 tab 列表失败', e)
    }
  }, [openTabs, workspace])

  useEffect(() => {
    persistActiveTabKeyFor(workspace, activeTabKey)
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

  const handleWorkspaceSwitch = (newPath: string) => {
    setWorkspace(newPath)
    setMode(booksReturnModeRef.current)
    refreshAll()
    notifyVersionChange()
  }

  const handleQuickWorkspaceSwitch = useCallback(async (newPath: string): Promise<boolean> => {
    if (!newPath || newPath === workspace) return true
    if (!(await flushEditorDraft())) return false
    try {
      const result = await switchWorkspace(newPath)
      const nextWorkspace = result.workspace || newPath
      console.info('[App.tsx] 标题栏切换书籍完成', { from: workspace, to: nextWorkspace })
      setWorkspace(nextWorkspace)
      await refreshAll()
      notifyVersionChange()
      return true
    } catch (error) {
      console.error('[App.tsx] 标题栏切换书籍失败', { from: workspace, to: newPath, error })
      toast.error(t('workbench.bookSwitcher.switchError'), {
        description: error instanceof Error ? error.message : String(error),
      })
      return false
    }
  }, [flushEditorDraft, notifyVersionChange, refreshAll, setWorkspace, t, workspace])

  const handleSaveCurrentFile = useCallback(async (path: string, content: string, baseRevision: string) => {
    const saved = await saveFileDraft(path, content, baseRevision)
    notifyVersionChange()
    return saved
  }, [notifyVersionChange, saveFileDraft])

  const handleCreateItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await createItem(path, type)
    notifyVersionChange()
  }, [createItem, notifyVersionChange])

  const handleDeleteItem = useCallback(async (path: string) => {
    if ((selectedFile === path || selectedFile?.startsWith(`${path}/`)) && !(await flushEditorDraft())) return
    await deleteItem(path)
    setOpenTabs((prev) => prev.filter((tab) => tab.path !== path && !tab.path.startsWith(`${path}/`)))
    notifyVersionChange()
  }, [deleteItem, flushEditorDraft, notifyVersionChange, selectedFile])

  const handleRenameItem = useCallback(async (path: string, newName: string) => {
    if ((selectedFile === path || selectedFile?.startsWith(`${path}/`)) && !(await flushEditorDraft())) return
    await renameItem(path, newName)
    const parent = path.replace(/\/[^/]*$/, '')
    const newPath = parent ? `${parent}/${newName}` : newName
    setOpenTabs((prev) => dedupeTabs(prev.map((tab) => {
      if (tab.path === path) return { kind: 'file', path: newPath }
      if (tab.path.startsWith(`${path}/`)) return { kind: 'file', path: `${newPath}${tab.path.slice(path.length)}` }
      return tab
    })))
    notifyVersionChange()
  }, [flushEditorDraft, notifyVersionChange, renameItem, selectedFile])

  const handleCopyItem = useCallback(async (from: string, to: string) => {
    await copyItem(from, to)
    notifyVersionChange()
  }, [copyItem, notifyVersionChange])

  const handleMoveItem = useCallback(async (from: string, to: string) => {
    if ((selectedFile === from || selectedFile?.startsWith(`${from}/`)) && !(await flushEditorDraft())) return
    await moveItem(from, to)
    setOpenTabs((prev) => dedupeTabs(prev.map((tab) => {
      if (tab.path === from) return { kind: 'file', path: to }
      if (tab.path.startsWith(`${from}/`)) return { kind: 'file', path: `${to}${tab.path.slice(from.length)}` }
      return tab
    })))
    notifyVersionChange()
  }, [flushEditorDraft, moveItem, notifyVersionChange, selectedFile])

  const handleSelectFile = useCallback(async (path: string) => {
    if (selectedFile !== path && !(await flushEditorDraft())) return false
    setSelectedChapterId(path)
    const key = `file:${path}`
    setOpenTabs((prev) => {
      const next: Tab[] = prev.some((tab) => tabKey(tab) === key) ? prev : [...prev, { kind: 'file', path }]
      return limitTabs(next, key)
    })
    setActiveTabKey(key)
    await selectFile(path)
    return true
  }, [flushEditorDraft, limitTabs, selectFile, selectedFile, setSelectedChapterId])

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
    if (characterCardTargetMode === 'current' && !workspace) {
      setCharacterCardError(t('importCard.noCurrentBookImportNew'))
      return
    }
    setCharacterCardImporting(true)
    setCharacterCardError('')
    try {
      const result = await importCharacterCard(characterCardFile, {
        targetMode: characterCardTargetMode,
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
      setMode('interactive')
      useInteractiveStore.getState().setSubmode('lore')
      window.setTimeout(() => {
        window.dispatchEvent(new CustomEvent('nova:lore-updated', { detail: result }))
      }, 0)
      notifyVersionChange()
      setCharacterCardDialogOpen(false)
      resetCharacterCardImport()
    } catch (e) {
      const message = e instanceof Error ? e.message : t('importCard.importFailed')
      setCharacterCardError(message)
      toast.error(message)
    } finally {
      setCharacterCardImporting(false)
    }
  }, [characterCardBookTitle, characterCardFile, characterCardPreview, characterCardSemanticClassification, characterCardTargetMode, characterCardUserName, notifyVersionChange, refresh, refreshAll, resetCharacterCardImport, setMode, t, workspace])

  const handleActivateTab = useCallback(async (tab: Tab) => {
    const key = tabKey(tab)
    if (selectedFile === tab.path) {
      setActiveTabKey(key)
      return
    }
    await handleSelectFile(tab.path)
  }, [handleSelectFile, selectedFile])

  const handleCloseTab = useCallback(async (tab: Tab) => {
    const key = tabKey(tab)
    const idx = openTabs.findIndex((item) => tabKey(item) === key)
    if (idx === -1) return
    if (activeTabKey === key && !(await flushEditorDraft())) return
    const next = openTabs.filter((item) => tabKey(item) !== key)
    setOpenTabs(next)
    if (activeTabKey !== key) return
    if (next.length === 0) {
      setActiveTabKey(null)
      clearSelectedFile()
      return
    }
    const fallback = next[idx] ?? next[idx - 1] ?? next[0]
    await handleActivateTab(fallback)
  }, [activeTabKey, clearSelectedFile, flushEditorDraft, handleActivateTab, openTabs])

  const triggerSave = useCallback(() => setSaveSignal((value) => value + 1), [])
  const continueWriting = useCallback(() => {
    if (!isStreaming) send('/continue')
  }, [isStreaming, send])

  const handleSetMode = useCallback((nextMode: WorkspaceMode) => {
    if (nextMode === 'books' || nextMode === 'skills' || nextMode === 'agents' || nextMode === 'automations') {
      const returnMode = mode === 'ide' || mode === 'interactive' ? mode : booksReturnModeRef.current
      booksReturnModeRef.current = returnMode
      setBooksReturnMode(returnMode)
    } else if (nextMode === 'ide' || nextMode === 'interactive') {
      booksReturnModeRef.current = nextMode
      setBooksReturnMode(nextMode)
    }
    setSettingsOpen(false)
    setMode(nextMode)
  }, [mode, setMode])
  const handleSetRightPanel = useCallback((panel: RightPanel) => {
    setSettingsOpen(false)
    if (isIdeWorkspacePanel(panel)) {
      if (!isIdeWorkspacePanel(rightPanel)) writingRightPanelRef.current = toWritingRightPanel(rightPanel)
      setRightPanel(panel)
      return
    }
    if (panel === null && isIdeWorkspacePanel(rightPanel)) {
      setRightPanel(writingRightPanelRef.current)
      return
    }
    if (panel === 'ai' || panel === null) writingRightPanelRef.current = panel
    setRightPanel(panel)
  }, [rightPanel, setRightPanel])
  const handleOpenVersions = useCallback(() => {
    setSettingsOpen(false)
    if (mode !== 'ide' && mode !== 'interactive') {
      setMode(booksReturnModeRef.current)
    }
    handleSetRightPanel('versions')
  }, [handleSetRightPanel, mode, setMode])

  const handleSetChapterConfirmed = useCallback(async (path: string, confirmed: boolean) => {
    await setChapterConfirmed(path, confirmed)
    await refreshSummary({ showLoading: false, clearOnError: false })
  }, [refreshSummary])

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
      if (rightPanel === 'lore' || rightPanel === 'teller' || rightPanel === 'versions') handleSetRightPanel(null)
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
      handleSetMode('interactive')
      if (rightPanel === 'versions') handleSetRightPanel(null)
      return
    }
    if (target === 'lore') {
      setSettingsOpen(false)
      if (mode === 'interactive') {
        useInteractiveStore.getState().setSubmode('lore')
      } else {
        handleSetMode('ide')
        handleSetRightPanel('lore')
      }
      return
    }
    if (target === 'teller') {
      setSettingsOpen(false)
      if (mode === 'interactive') {
        useInteractiveStore.getState().setSubmode('teller')
      } else {
        handleSetMode('ide')
        handleSetRightPanel('teller')
      }
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
  }, [handleOpenVersions, handleSetMode, handleSetRightPanel, mode, rightPanel])

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
        booksReturnMode={booksReturnMode}
        currentBookName={currentBookName}
        workspace={workspace}
        appVersion={APP_VERSION}
        summary={summary}
        currentChapter={currentChapter}
        chapterStats={chapterStats}
        isStreaming={isStreaming}
        projectVisible={projectVisible}
        activityBarExpanded={activityBarExpanded}
        rightPanel={rightPanel}
        settingsOpen={settingsOpen}
        interactiveRightVisible={interactiveRightVisible}
        novaDir={novaDir}
        books={books}
        bookSortMode={bookSortMode}
        tree={tree}
        loading={loading}
        selectedFile={selectedFile}
        fileContent={fileContent}
        fileRevision={fileRevision}
        openTabs={openTabs}
        activeTabKey={activeTabKey}
        sidebarView={sidebarView}
        editorSearchIntent={editorSearchIntent}
        saveSignal={saveSignal}
        editorAutoSaveEnabled={editorAutoSaveEnabled}
        editorAutoSaveDelayMs={editorAutoSaveDelayMs}
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
        chatPlanMode={planMode}
        hasEarlierMessages={hasEarlierMessages}
        isLoadingEarlierHistory={isLoadingEarlierHistory}
        onSetMode={handleSetMode}
        onToggleActivityBarExpanded={() => setActivityBarExpanded((value) => !value)}
        onToggleProjectVisible={() => setProjectVisible((value) => !value)}
        onSetRightPanel={handleSetRightPanel}
        onToggleSettings={() => setSettingsOpen((open) => !open)}
        onCloseSettings={() => setSettingsOpen(false)}
        notice={notice}
        onDismissNotice={dismissNotice}
        onToggleInteractiveRightPanel={() => setInteractiveRightVisible((value) => !value)}
        onSwitchBook={handleWorkspaceSwitch}
        onQuickSwitchBook={handleQuickWorkspaceSwitch}
        onBeforeWorkspaceSwitch={flushEditorDraft}
        onBooksChange={refreshBooks}
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
        onActivateTab={handleActivateTab}
        onCloseTab={handleCloseTab}
        onSaveCurrentFile={handleSaveCurrentFile}
        onEditorFlushHandlerChange={handleEditorFlushHandlerChange}
        onWorkspaceChanged={handleReviewedWorkspaceChange}
        onQuoteSelection={addTextSelection}
        onCreateChatSession={createChatSession}
        onSwitchChatSession={switchChatSession}
        onRenameChatSession={renameChatSession}
        onDeleteChatSession={deleteChatSession}
        onLoadEarlierHistory={loadEarlierHistory}
        onSend={send}
        onAnalyzeContext={analyzeContext}
        onStop={stop}
        onReferenceRemove={removeReference}
        onLoreReferenceAdd={addLoreReference}
        onLoreReferenceRemove={removeLoreReference}
        onStyleSceneAdd={addStyleScene}
        onStyleSceneRemove={removeStyleScene}
        onTextSelectionRemove={removeTextSelection}
        onChatPlanModeChange={handleChatPlanModeChange}
        onChatPlanModeToggle={handleChatPlanModeToggle}
        onSubmitPlanQuestion={submitPlanQuestion}
        onApproveProposedPlan={approveProposedPlan}
        onExitChatPlanMode={exitPlanMode}
      />
      <CommandPalette
        open={commandOpen}
        isStreaming={isStreaming}
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
      <RemoteAccessLogin />
      <OnboardingGuide
        mode={mode}
        rightPanel={rightPanel}
        settingsOpen={settingsOpen}
        workspace={workspace}
        booksCount={books.length}
        currentBookName={currentBookName}
        messages={messages}
        isStreaming={isStreaming}
        onNavigate={handleOnboardingNavigate}
      />
    </NovaMotionProvider>
  )
}

function readLayoutBoolean(key: string, fallback: boolean) {
  if (typeof window === 'undefined') return fallback
  const value = window.localStorage.getItem(key)
  if (value === null) return fallback
  return value === 'true'
}

function readContentMode(): BooksReturnMode {
  if (typeof window === 'undefined') return 'ide'
  const value = window.localStorage.getItem(CONTENT_MODE_STORAGE_KEY)
  return value === 'interactive' ? 'interactive' : 'ide'
}

function writeContentMode(mode: BooksReturnMode) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(CONTENT_MODE_STORAGE_KEY, mode)
}

function normalizeAutoSaveDelayMs(value: number | null | undefined) {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return AUTO_SAVE_DELAY_FALLBACK_MS
  }
  return Math.floor(value)
}

function isIdeWorkspacePanel(panel: RightPanel): panel is 'lore' | 'creator' | 'teller' | 'versions' {
  return panel === 'lore' || panel === 'creator' || panel === 'teller' || panel === 'versions'
}

function toWritingRightPanel(panel: RightPanel): WritingRightPanel {
  return panel === 'ai' ? panel : null
}

function normalizeAppTheme(theme?: string) {
  if (theme === 'light' || theme === 'dark' || theme === 'system') return theme
  return 'dark'
}

export default App
