import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { fetchSettings } from '@/features/settings/api'
import { applyFontSettings, fontSettingsFromEffective } from '@/features/settings/font-variables'
import { getLoreItems, importCharacterCard, previewCharacterCard, type CharacterCardPreview, type LoreItem, type WorkspaceSearchResult } from '@/lib/api'
import { CommandPalette } from '@/components/common/command-palette'
import { useWorkspace } from '@/hooks/useWorkspace'
import { useChat } from '@/hooks/useChat'
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
import {
  CharacterCardImportDialog,
  type CharacterCardTargetMode,
} from '@/components/workbench/CharacterCardImportDialog'
import { APP_VERSION } from '@/app-version'

const PROJECT_VISIBLE_KEY = 'nova.layout.projectVisible'
const ACTIVITY_BAR_EXPANDED_KEY = 'nova.layout.activityBarExpanded'
const INTERACTIVE_RIGHT_VISIBLE_KEY = 'nova.layout.interactiveRightVisible'
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
  const [activityBarExpanded, setActivityBarExpanded] = useState(() => readLayoutBoolean(ACTIVITY_BAR_EXPANDED_KEY, false))
  const [interactiveRightVisible, setInteractiveRightVisible] = useState(() => readLayoutBoolean(INTERACTIVE_RIGHT_VISIBLE_KEY, true))
  const [saveSignal, setSaveSignal] = useState(0)
  const [versionRefreshSignal, setVersionRefreshSignal] = useState(0)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [openTabs, setOpenTabs] = useState<Tab[]>([])
  const [activeTabKey, setActiveTabKey] = useState<string | null>(null)
  const [maxOpenTabs, setMaxOpenTabs] = useState<number>(MAX_OPEN_TABS_FALLBACK)
  const [editorAutoSaveEnabled, setEditorAutoSaveEnabled] = useState(AUTO_SAVE_ENABLED_FALLBACK)
  const [editorAutoSaveDelayMs, setEditorAutoSaveDelayMs] = useState(AUTO_SAVE_DELAY_FALLBACK_MS)
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
  const [characterCardPreviewing, setCharacterCardPreviewing] = useState(false)
  const [characterCardImporting, setCharacterCardImporting] = useState(false)
  const [characterCardError, setCharacterCardError] = useState('')
  const [loreItems, setLoreItems] = useState<LoreItem[]>([])
  const [booksReturnMode, setBooksReturnMode] = useState<BooksReturnMode>('ide')
  const booksReturnModeRef = useRef<BooksReturnMode>('ide')
  const writingRightPanelRef = useRef<WritingRightPanel>('ai')
  const characterCardInputRef = useRef<HTMLInputElement>(null)
  const chatWorkspaceRef = useRef('')
  const tabActivationsRef = useRef<Map<string, number>>(new Map())
  const tabActivationCounterRef = useRef(0)

  const rightPanel = useWorkspaceStore((state) => state.rightPanel)
  const commandOpen = useWorkspaceStore((state) => state.commandOpen)
  const mode = useWorkspaceStore((state) => state.mode)
  const setRightPanel = useWorkspaceStore((state) => state.setRightPanel)
  const setCommandOpen = useWorkspaceStore((state) => state.setCommandOpen)
  const setMode = useWorkspaceStore((state) => state.setMode)
  const setSelectedChapterId = useWorkspaceStore((state) => state.setSelectedChapterId)

  useEffect(() => {
    if (mode === 'books' || mode === 'skills' || mode === 'agents' || mode === 'automations') return
    const contentMode = mode === 'interactive' ? 'interactive' : 'ide'
    booksReturnModeRef.current = contentMode
    setBooksReturnMode(contentMode)
  }, [mode])

  const {
    tree, loading, selectedFile, fileContent, workspace, workspaceLoaded, summary, styles, books,
    selectFile, clearSelectedFile, saveCurrentFile, createItem, deleteItem, renameItem, copyItem, moveItem,
    refresh, refreshAfterAgentFileChange, refreshAll, refreshBooks, setWorkspace,
  } = useWorkspace()

  const notifyVersionChange = useCallback(() => {
    setVersionRefreshSignal(value => value + 1)
  }, [])

  const handleAgentFileChange = useCallback(async (path?: string) => {
    await refreshAfterAgentFileChange(path)
    notifyVersionChange()
  }, [notifyVersionChange, refreshAfterAgentFileChange])

  const {
    messages,
    sessions,
    activeSessionId,
    isStreaming,
    activityContent,
    references,
    styleReferences,
    textSelections,
    send,
    analyzeContext,
    stop,
    loadSessions,
    loadHistory,
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
    addStyleReference,
    removeStyleReference,
    addTextSelection,
    removeTextSelection,
  } = useChat({ onAgentFileChange: handleAgentFileChange })

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
          setNovaDir(data?.paths?.nova_dir || '')
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
    if (activeTabKey) touchTab(activeTabKey)
  }, [activeTabKey, touchTab])

  useEffect(() => {
    setOpenTabs((prev) => limitTabs(prev, activeTabKey))
  }, [maxOpenTabs, activeTabKey, limitTabs])

  useEffect(() => { window.localStorage.setItem(PROJECT_VISIBLE_KEY, String(projectVisible)) }, [projectVisible])
  useEffect(() => { window.localStorage.setItem(ACTIVITY_BAR_EXPANDED_KEY, String(activityBarExpanded)) }, [activityBarExpanded])
  useEffect(() => { window.localStorage.setItem(INTERACTIVE_RIGHT_VISIBLE_KEY, String(interactiveRightVisible)) }, [interactiveRightVisible])

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

  const handleSaveCurrentFile = useCallback(async (content: string) => {
    const saved = await saveCurrentFile(content)
    if (saved) notifyVersionChange()
    return saved
  }, [notifyVersionChange, saveCurrentFile])

  const handleCreateItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await createItem(path, type)
    notifyVersionChange()
  }, [createItem, notifyVersionChange])

  const handleDeleteItem = useCallback(async (path: string) => {
    await deleteItem(path)
    setOpenTabs((prev) => prev.filter((tab) => tab.path !== path && !tab.path.startsWith(`${path}/`)))
    notifyVersionChange()
  }, [deleteItem, notifyVersionChange])

  const handleRenameItem = useCallback(async (path: string, newName: string) => {
    await renameItem(path, newName)
    const parent = path.replace(/\/[^/]*$/, '')
    const newPath = parent ? `${parent}/${newName}` : newName
    setOpenTabs((prev) => dedupeTabs(prev.map((tab) => {
      if (tab.path === path) return { kind: 'file', path: newPath }
      if (tab.path.startsWith(`${path}/`)) return { kind: 'file', path: `${newPath}${tab.path.slice(path.length)}` }
      return tab
    })))
    notifyVersionChange()
  }, [notifyVersionChange, renameItem])

  const handleCopyItem = useCallback(async (from: string, to: string) => {
    await copyItem(from, to)
    notifyVersionChange()
  }, [copyItem, notifyVersionChange])

  const handleMoveItem = useCallback(async (from: string, to: string) => {
    await moveItem(from, to)
    setOpenTabs((prev) => dedupeTabs(prev.map((tab) => {
      if (tab.path === from) return { kind: 'file', path: to }
      if (tab.path.startsWith(`${from}/`)) return { kind: 'file', path: `${to}${tab.path.slice(from.length)}` }
      return tab
    })))
    notifyVersionChange()
  }, [moveItem, notifyVersionChange])

  const handleSelectFile = useCallback(async (path: string) => {
    setSelectedChapterId(path)
    const key = `file:${path}`
    setOpenTabs((prev) => {
      const next: Tab[] = prev.some((tab) => tabKey(tab) === key) ? prev : [...prev, { kind: 'file', path }]
      return limitTabs(next, key)
    })
    setActiveTabKey(key)
    await selectFile(path)
  }, [limitTabs, selectFile, setSelectedChapterId])

  const handleSelectSearchResult = useCallback(async (result: WorkspaceSearchResult, query: string) => {
    setSettingsOpen(false)
    setMode('ide')
    setProjectVisible(true)
    setSidebarView('search')
    await handleSelectFile(result.path)
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
  }, [characterCardBookTitle, characterCardFile, characterCardPreview?.user_placeholder_found, characterCardTargetMode, characterCardUserName, notifyVersionChange, refresh, refreshAll, resetCharacterCardImport, setMode, t, workspace])

  const handleActivateTab = useCallback((tab: Tab) => {
    const key = tabKey(tab)
    setActiveTabKey(key)
    if (selectedFile !== tab.path) void handleSelectFile(tab.path)
  }, [handleSelectFile, selectedFile])

  const handleCloseTab = useCallback((tab: Tab) => {
    const key = tabKey(tab)
    setOpenTabs((prev) => {
      const idx = prev.findIndex((item) => tabKey(item) === key)
      if (idx === -1) return prev
      const next = prev.filter((item) => tabKey(item) !== key)
      if (activeTabKey === key) {
        if (next.length === 0) {
          setActiveTabKey(null)
          clearSelectedFile()
        } else {
          const fallback = next[idx] ?? next[idx - 1] ?? next[0]
          handleActivateTab(fallback)
        }
      }
      return next
    })
  }, [activeTabKey, clearSelectedFile, handleActivateTab])

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

  const handleOpenGlobalSearch = useCallback(() => {
    setSettingsOpen(false)
    setMode('ide')
    setProjectVisible(true)
    setSidebarView('search')
  }, [setMode])

  useWorkspaceHotkeys({
    onSave: triggerSave,
    onOpenCommand: () => setCommandOpen(true),
    onOpenSearch: handleOpenGlobalSearch,
    onGenerate: continueWriting,
    onOpenDiff: handleOpenVersions,
    onEscape: () => {
      if (commandOpen) {
        setCommandOpen(false)
        return
      }
      if (rightPanel) handleSetRightPanel(null)
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
        tree={tree}
        loading={loading}
        selectedFile={selectedFile}
        fileContent={fileContent}
        styles={styles}
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
        styleReferences={styleReferences}
        textSelections={textSelections}
        onSetMode={handleSetMode}
        onToggleActivityBarExpanded={() => setActivityBarExpanded((value) => !value)}
        onToggleProjectVisible={() => setProjectVisible((value) => !value)}
        onSetRightPanel={handleSetRightPanel}
        onToggleSettings={() => setSettingsOpen((open) => !open)}
        onCloseSettings={() => setSettingsOpen(false)}
        onToggleInteractiveRightPanel={() => setInteractiveRightVisible((value) => !value)}
        onSwitchBook={handleWorkspaceSwitch}
        onBooksChange={refreshBooks}
        onOpenCharacterCardImport={handleOpenCharacterCardImportFromBooks}
        onSetSidebarView={setSidebarView}
        onSelectSearchResult={handleSelectSearchResult}
        onRefreshTree={refresh}
        onSelectFile={handleSelectFile}
        onReferenceFile={addReference}
        onCreateItem={handleCreateItem}
        onDeleteItem={handleDeleteItem}
        onRenameItem={handleRenameItem}
        onCopyItem={handleCopyItem}
        onMoveItem={handleMoveItem}
        onActivateTab={handleActivateTab}
        onCloseTab={handleCloseTab}
        onSaveCurrentFile={handleSaveCurrentFile}
        onQuoteSelection={addTextSelection}
        onCreateChatSession={createChatSession}
        onSwitchChatSession={switchChatSession}
        onRenameChatSession={renameChatSession}
        onDeleteChatSession={deleteChatSession}
        onSend={send}
        onAnalyzeContext={analyzeContext}
        onStop={stop}
        onReferenceRemove={removeReference}
        onLoreReferenceAdd={addLoreReference}
        onLoreReferenceRemove={removeLoreReference}
        onStyleReferenceAdd={addStyleReference}
        onStyleReferenceRemove={removeStyleReference}
        onTextSelectionRemove={removeTextSelection}
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
        onClosePanels={() => {
          handleSetRightPanel(null)
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
        previewing={characterCardPreviewing}
        importing={characterCardImporting}
        error={characterCardError}
        fileInputRef={characterCardInputRef}
        onOpenChange={handleCharacterCardDialogOpenChange}
        onFileSelected={handleCharacterCardSelected}
        onTargetModeChange={setCharacterCardTargetMode}
        onBookTitleChange={setCharacterCardBookTitle}
        onUserCharacterNameChange={setCharacterCardUserName}
        onImport={handleCharacterCardImport}
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
  return panel === 'ai' ? 'ai' : null
}

function normalizeAppTheme(theme?: string) {
  if (theme === 'light' || theme === 'dark' || theme === 'system') return theme
  return 'dark'
}

export default App
