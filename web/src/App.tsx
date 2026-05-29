import { useCallback, useEffect, useRef, useState } from 'react'
import { SettingsView } from '@/features/settings/SettingsView'
import { fetchSettings } from '@/features/settings/api'
import { fontStackFor } from '@/features/settings/font-options'
import { getLoreItems, importCharacterCard, previewCharacterCard, type CharacterCardPreview, type LoreItem } from '@/lib/api'
import { CommandPalette } from '@/components/common/command-palette'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { useWorkspace } from '@/hooks/useWorkspace'
import { useChat } from '@/hooks/useChat'
import { useWorkspaceHotkeys } from '@/hooks/use-workspace-hotkeys'
import { useWorkspaceStore, type RightPanel, type WorkspaceMode } from '@/stores/workspace-store'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import type { ChapterSummary } from '@/lib/api'
import { toast } from 'sonner'
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

const PROJECT_VISIBLE_KEY = 'nova.layout.projectVisible'
const ACTIVITY_BAR_EXPANDED_KEY = 'nova.layout.activityBarExpanded'
const INTERACTIVE_RIGHT_VISIBLE_KEY = 'nova.layout.interactiveRightVisible'
const APP_VERSION = __APP_VERSION__
const MAX_OPEN_TABS_FALLBACK = 5

function App() {
  const [projectVisible, setProjectVisible] = useState(() => readLayoutBoolean(PROJECT_VISIBLE_KEY, true))
  const [activityBarExpanded, setActivityBarExpanded] = useState(() => readLayoutBoolean(ACTIVITY_BAR_EXPANDED_KEY, false))
  const [interactiveRightVisible, setInteractiveRightVisible] = useState(() => readLayoutBoolean(INTERACTIVE_RIGHT_VISIBLE_KEY, true))
  const [saveSignal, setSaveSignal] = useState(0)
  const [gitRefreshSignal, setGitRefreshSignal] = useState(0)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [openTabs, setOpenTabs] = useState<Tab[]>([])
  const [activeTabKey, setActiveTabKey] = useState<string | null>(null)
  const [maxOpenTabs, setMaxOpenTabs] = useState<number>(MAX_OPEN_TABS_FALLBACK)
  const [novaDir, setNovaDir] = useState('')
  const [sidebarView, setSidebarView] = useState<'outline' | 'files'>('outline')
  const [characterCardDialogOpen, setCharacterCardDialogOpen] = useState(false)
  const [characterCardFile, setCharacterCardFile] = useState<File | null>(null)
  const [characterCardPreview, setCharacterCardPreview] = useState<CharacterCardPreview | null>(null)
  const [characterCardTargetMode, setCharacterCardTargetMode] = useState<CharacterCardTargetMode>('current')
  const [characterCardBookTitle, setCharacterCardBookTitle] = useState('')
  const [characterCardPreviewing, setCharacterCardPreviewing] = useState(false)
  const [characterCardImporting, setCharacterCardImporting] = useState(false)
  const [characterCardError, setCharacterCardError] = useState('')
  const [loreItems, setLoreItems] = useState<LoreItem[]>([])
  const characterCardInputRef = useRef<HTMLInputElement>(null)
  const chatBootstrappedRef = useRef(false)
  const tabActivationsRef = useRef<Map<string, number>>(new Map())
  const tabActivationCounterRef = useRef(0)

  const rightPanel = useWorkspaceStore((state) => state.rightPanel)
  const commandOpen = useWorkspaceStore((state) => state.commandOpen)
  const mode = useWorkspaceStore((state) => state.mode)
  const setRightPanel = useWorkspaceStore((state) => state.setRightPanel)
  const setCommandOpen = useWorkspaceStore((state) => state.setCommandOpen)
  const setMode = useWorkspaceStore((state) => state.setMode)
  const setSelectedChapterId = useWorkspaceStore((state) => state.setSelectedChapterId)

  const {
    tree, loading, selectedFile, fileContent, workspace, workspaceLoaded, summary, styles, books,
    selectFile, clearSelectedFile, saveCurrentFile, createItem, deleteItem, renameItem, copyItem, moveItem,
    refresh, refreshAfterAgentFileChange, refreshAll, refreshBooks, setWorkspace,
  } = useWorkspace()

  const notifyGitChange = useCallback(() => {
    setGitRefreshSignal(value => value + 1)
  }, [])

  const handleAgentFileChange = useCallback(async (path?: string) => {
    await refreshAfterAgentFileChange(path)
    notifyGitChange()
  }, [notifyGitChange, refreshAfterAgentFileChange])

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
    '未选择书籍'

  const touchTab = useCallback((key: string) => {
    tabActivationCounterRef.current += 1
    tabActivationsRef.current.set(key, tabActivationCounterRef.current)
  }, [])

  const limitTabs = useCallback((tabs: Tab[], protectedKey: string | null): Tab[] => {
    return enforceTabLimit(tabs, protectedKey, maxOpenTabs, tabActivationsRef.current)
  }, [maxOpenTabs])

  useEffect(() => {
    if (chatBootstrappedRef.current) return
    chatBootstrappedRef.current = true
    void Promise.all([loadSessions(), loadHistory()]).then(() => resumeActiveChat())
  }, [loadHistory, loadSessions, resumeActiveChat])

  useEffect(() => {
    let cancelled = false
    const reload = () => {
      fetchSettings()
        .then((data) => {
          if (cancelled) return
          const v = data?.effective?.max_open_tabs
          if (typeof v === 'number' && v >= 1) setMaxOpenTabs(Math.floor(v))
          setNovaDir(data?.paths?.nova_dir || '')
          applyFontSettings(data?.effective?.ui_font_family, data?.effective?.reading_font_family)
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
  }, [workspace])

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
    if (!workspace) {
      if (!workspaceLoaded) return
      setOpenTabs([])
      setActiveTabKey(null)
      clearSelectedFile()
      setMode('books')
      return
    }
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
  }, [workspace, workspaceLoaded])

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
    setMode('ide')
    refreshAll()
    notifyGitChange()
    void Promise.all([loadSessions(), loadHistory()]).then(() => resumeActiveChat())
  }

  const handleSaveCurrentFile = useCallback(async (content: string) => {
    const saved = await saveCurrentFile(content)
    if (saved) notifyGitChange()
    return saved
  }, [notifyGitChange, saveCurrentFile])

  const handleCreateItem = useCallback(async (path: string, type: 'file' | 'dir') => {
    await createItem(path, type)
    notifyGitChange()
  }, [createItem, notifyGitChange])

  const handleDeleteItem = useCallback(async (path: string) => {
    await deleteItem(path)
    setOpenTabs((prev) => prev.filter((tab) => tab.path !== path && !tab.path.startsWith(`${path}/`)))
    notifyGitChange()
  }, [deleteItem, notifyGitChange])

  const handleRenameItem = useCallback(async (path: string, newName: string) => {
    await renameItem(path, newName)
    const parent = path.replace(/\/[^/]*$/, '')
    const newPath = parent ? `${parent}/${newName}` : newName
    setOpenTabs((prev) => dedupeTabs(prev.map((tab) => {
      if (tab.path === path) return { kind: 'file', path: newPath }
      if (tab.path.startsWith(`${path}/`)) return { kind: 'file', path: `${newPath}${tab.path.slice(path.length)}` }
      return tab
    })))
    notifyGitChange()
  }, [notifyGitChange, renameItem])

  const handleCopyItem = useCallback(async (from: string, to: string) => {
    await copyItem(from, to)
    notifyGitChange()
  }, [copyItem, notifyGitChange])

  const handleMoveItem = useCallback(async (from: string, to: string) => {
    await moveItem(from, to)
    setOpenTabs((prev) => dedupeTabs(prev.map((tab) => {
      if (tab.path === from) return { kind: 'file', path: to }
      if (tab.path.startsWith(`${from}/`)) return { kind: 'file', path: `${to}${tab.path.slice(from.length)}` }
      return tab
    })))
    notifyGitChange()
  }, [moveItem, notifyGitChange])

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

  const resetCharacterCardImport = useCallback(() => {
    setCharacterCardFile(null)
    setCharacterCardPreview(null)
    setCharacterCardTargetMode(workspace ? 'current' : 'new_book')
    setCharacterCardBookTitle('')
    setCharacterCardPreviewing(false)
    setCharacterCardImporting(false)
    setCharacterCardError('')
    if (characterCardInputRef.current) {
      characterCardInputRef.current.value = ''
    }
  }, [workspace])

  const handleCharacterCardDialogOpenChange = useCallback((open: boolean) => {
    setCharacterCardDialogOpen(open)
    if (!open) resetCharacterCardImport()
    if (open && !workspace) setCharacterCardTargetMode('new_book')
  }, [resetCharacterCardImport, workspace])

  const handleOpenCharacterCardImportFromBooks = useCallback(() => {
    handleCharacterCardDialogOpenChange(true)
  }, [handleCharacterCardDialogOpenChange])

  const handleCharacterCardSelected = useCallback(async (file: File | undefined) => {
    if (!file) return
    setCharacterCardFile(file)
    setCharacterCardPreview(null)
    setCharacterCardTargetMode(workspace ? 'current' : 'new_book')
    setCharacterCardError('')
    setCharacterCardPreviewing(true)
    try {
      const preview = await previewCharacterCard(file)
      setCharacterCardPreview(preview)
      setCharacterCardBookTitle(preview.name)
    } catch (e) {
      setCharacterCardError(e instanceof Error ? e.message : '解析酒馆角色卡失败')
    } finally {
      setCharacterCardPreviewing(false)
      if (characterCardInputRef.current) {
        characterCardInputRef.current.value = ''
      }
    }
  }, [workspace])

  const handleCharacterCardImport = useCallback(async () => {
    if (!characterCardFile) {
      setCharacterCardError('请先选择酒馆角色卡文件')
      return
    }
    if (characterCardTargetMode === 'current' && !workspace) {
      setCharacterCardError('当前没有打开的书籍，请选择“导入成新书”')
      return
    }
    setCharacterCardImporting(true)
    setCharacterCardError('')
    try {
      const result = await importCharacterCard(characterCardFile, {
        targetMode: characterCardTargetMode,
        bookTitle: characterCardTargetMode === 'new_book' ? characterCardBookTitle.trim() : undefined,
      })
      toast.success(result.message || `已导入酒馆角色卡「${result.name}」`)
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
      notifyGitChange()
      setCharacterCardDialogOpen(false)
      resetCharacterCardImport()
    } catch (e) {
      const message = e instanceof Error ? e.message : '导入酒馆角色卡失败'
      setCharacterCardError(message)
      toast.error(message)
    } finally {
      setCharacterCardImporting(false)
    }
  }, [characterCardBookTitle, characterCardFile, characterCardTargetMode, notifyGitChange, refresh, refreshAll, resetCharacterCardImport, setMode, workspace])

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

  const handleSetMode = useCallback((nextMode: WorkspaceMode) => setMode(nextMode), [setMode])
  const handleSetRightPanel = useCallback((panel: RightPanel) => setRightPanel(panel), [setRightPanel])

  useWorkspaceHotkeys({
    onSave: triggerSave,
    onOpenCommand: () => setCommandOpen(true),
    onGenerate: continueWriting,
    onOpenDiff: () => setRightPanel('versions'),
    onEscape: () => {
      if (commandOpen) {
        setCommandOpen(false)
        return
      }
      if (rightPanel) setRightPanel(null)
    },
  })

  return (
    <>
      <ModeRouter
        mode={mode}
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
        saveSignal={saveSignal}
        gitRefreshSignal={gitRefreshSignal}
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
        onToggleInteractiveRightPanel={() => setInteractiveRightVisible((value) => !value)}
        onSwitchBook={handleWorkspaceSwitch}
        onBooksChange={refreshBooks}
        onOpenCharacterCardImport={handleOpenCharacterCardImportFromBooks}
        onSetSidebarView={setSidebarView}
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
          setRightPanel('ai')
        }}
        onOpenVersions={() => setRightPanel('versions')}
        onContinueWriting={continueWriting}
        onClosePanels={() => {
          setRightPanel(null)
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
        previewing={characterCardPreviewing}
        importing={characterCardImporting}
        error={characterCardError}
        fileInputRef={characterCardInputRef}
        onOpenChange={handleCharacterCardDialogOpenChange}
        onFileSelected={handleCharacterCardSelected}
        onTargetModeChange={setCharacterCardTargetMode}
        onBookTitleChange={setCharacterCardBookTitle}
        onImport={handleCharacterCardImport}
      />
      <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
        <DialogContent
          className="nova-panel left-[2vw] top-[4vh] flex h-[92dvh] max-h-[calc(100dvh-2rem)] min-h-0 w-[96vw] max-w-[calc(100vw-2rem)] min-w-0 translate-x-0 translate-y-0 resize overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)] sm:max-w-[calc(100vw-2rem)]"
          showCloseButton={false}
          aria-describedby={undefined}
        >
          <DialogTitle className="sr-only">设置</DialogTitle>
          <SettingsView onClose={() => setSettingsOpen(false)} />
        </DialogContent>
      </Dialog>
    </>
  )
}

function readLayoutBoolean(key: string, fallback: boolean) {
  if (typeof window === 'undefined') return fallback
  const value = window.localStorage.getItem(key)
  if (value === null) return fallback
  return value === 'true'
}

function applyFontSettings(uiFont?: string, readingFont?: string) {
  if (typeof document === 'undefined') return
  document.documentElement.style.setProperty('--nova-ui-font-family', fontStackFor(uiFont, 'system-sans'))
  document.documentElement.style.setProperty('--nova-reading-font-family', fontStackFor(readingFont, 'source-han-serif'))
}

export default App
