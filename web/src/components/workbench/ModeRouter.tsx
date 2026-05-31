import { BookMarked, BookOpen, Bot, ChevronDown, ChevronRight, Database, FileText, PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen, RefreshCw, SearchCheck, SlidersHorizontal, Sparkles, WandSparkles, PenLine } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { FileTree } from '@/components/Sidebar/FileTree'
import { MessageList } from '@/components/Chat/MessageList'
import { InputArea } from '@/components/Chat/InputArea'
import { SessionManager } from '@/components/Chat/SessionManager'
import { MarkdownEditor } from '@/components/Editor/MarkdownEditor'
import { GitPanel } from '@/components/Git/GitPanel'
import { HomeView } from '@/components/Home/HomeView'
import { InteractiveLayout } from '@/features/interactive/components/InteractiveLayout'
import { SettingPanel } from '@/features/interactive/components/SettingPanel'
import { getInteractiveTellers } from '@/features/interactive/api'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import { fetchSettings, updateWorkspaceSettings } from '@/features/settings/api'
import { SettingsView } from '@/features/settings/SettingsView'
import type { Teller } from '@/features/interactive/types'
import type { FileNode } from '@/hooks/useWorkspace'
import type { BookRecord, ChapterSummary, ChatMessage, DocumentPreview, LoreItem, SessionSummary, TextSelection, WorkspaceSummary } from '@/lib/api'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import type { Tab } from './TabController'
import { TabController, tabKey } from './TabController'
import { WorkbenchShell } from './WorkbenchShell'
import { flattenFileTree, formatNumber } from './workbench-utils'

interface ModeRouterProps {
  mode: WorkspaceMode
  booksReturnMode: Exclude<WorkspaceMode, 'books'>
  currentBookName: string
  workspace: string
  appVersion: string
  summary: WorkspaceSummary | null
  currentChapter?: ChapterSummary
  chapterStats: Record<string, ChapterSummary>
  isStreaming: boolean
  projectVisible: boolean
  activityBarExpanded: boolean
  rightPanel: RightPanel
  settingsOpen: boolean
  interactiveRightVisible: boolean
  novaDir: string
  books: BookRecord[]
  tree: FileNode[]
  loading: boolean
  selectedFile: string | null
  fileContent: string
  styles: string[]
  openTabs: Tab[]
  activeTabKey: string | null
  sidebarView: 'outline' | 'files'
  saveSignal: number
  gitRefreshSignal: number
  messages: ChatMessage[]
  sessions: SessionSummary[]
  activeSessionId: string
  activityContent: string
  references: string[]
  loreReferences: string[]
  loreItems: LoreItem[]
  styleReferences: string[]
  textSelections: TextSelection[]
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onToggleProjectVisible: () => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleSettings: () => void
  onCloseSettings: () => void
  onToggleInteractiveRightPanel: () => void
  onSwitchBook: (path: string) => void
  onBooksChange: () => void | Promise<void>
  onOpenCharacterCardImport: () => void
  onSetSidebarView: (view: 'outline' | 'files') => void
  onRefreshTree: () => void
  onSelectFile: (path: string) => void | Promise<void>
  onReferenceFile: (path: string) => void
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  onActivateTab: (tab: Tab) => void
  onCloseTab: (tab: Tab) => void
  onSaveCurrentFile: (content: string) => Promise<boolean>
  onQuoteSelection: (selection: TextSelection) => void
  onCreateChatSession: (title?: string) => void | Promise<void>
  onSwitchChatSession: (id: string) => void | Promise<void>
  onRenameChatSession: (id: string, title: string) => void | Promise<void>
  onDeleteChatSession: (id: string) => void | Promise<void>
  onSend: (message: string) => void
  onStop: () => void
  onReferenceRemove: (path: string) => void
  onLoreReferenceAdd: (id: string) => void
  onLoreReferenceRemove: (id: string) => void
  onStyleReferenceAdd: (path: string) => void
  onStyleReferenceRemove: (path: string) => void
  onTextSelectionRemove: (index: number) => void
}

export function ModeRouter(props: ModeRouterProps) {
  const {
    mode,
    booksReturnMode,
    currentBookName,
    workspace,
    appVersion,
    summary,
    currentChapter,
    chapterStats,
    isStreaming,
    projectVisible,
    activityBarExpanded,
    rightPanel,
    settingsOpen,
    interactiveRightVisible,
    novaDir,
    books,
    tree,
    loading,
    selectedFile,
    fileContent,
    styles,
    openTabs,
    activeTabKey,
    sidebarView,
    saveSignal,
    gitRefreshSignal,
    messages,
    sessions,
    activeSessionId,
    activityContent,
    references,
    loreReferences,
    loreItems,
    styleReferences,
    textSelections,
    onSetMode,
    onToggleActivityBarExpanded,
    onToggleProjectVisible,
    onSetRightPanel,
    onToggleSettings,
    onCloseSettings,
    onToggleInteractiveRightPanel,
    onSwitchBook,
    onBooksChange,
    onOpenCharacterCardImport,
    onSetSidebarView,
    onRefreshTree,
    onSelectFile,
    onReferenceFile,
    onCreateItem,
    onDeleteItem,
    onRenameItem,
    onCopyItem,
    onMoveItem,
    onActivateTab,
    onCloseTab,
    onSaveCurrentFile,
    onQuoteSelection,
    onCreateChatSession,
    onSwitchChatSession,
    onRenameChatSession,
    onDeleteChatSession,
    onSend,
    onStop,
    onReferenceRemove,
    onLoreReferenceAdd,
    onLoreReferenceRemove,
    onStyleReferenceAdd,
    onStyleReferenceRemove,
    onTextSelectionRemove,
  } = props

  const activeTab = openTabs.find((tab) => tabKey(tab) === activeTabKey) ?? null
  const versionsVisible = rightPanel === 'versions'
  const ideWorkspacePanel = mode === 'ide' && (rightPanel === 'lore' || rightPanel === 'creator' || rightPanel === 'teller' || rightPanel === 'versions') ? rightPanel : null
  const interactiveSubmode = useInteractiveStore((state) => state.submode)
  const setInteractiveSubmode = useInteractiveStore((state) => state.setSubmode)
  const [tellers, setTellers] = useState<Teller[]>([])

  useEffect(() => {
    let cancelled = false
    if (!workspace) {
      setTellers([])
      return () => { cancelled = true }
    }
    getInteractiveTellers()
      .then((data) => {
        if (!cancelled) setTellers(data)
      })
      .catch(() => {
        if (!cancelled) setTellers([])
      })
    return () => { cancelled = true }
  }, [workspace])

  const loreReferenceLabels = useMemo(() => Object.fromEntries(loreItems.map((item) => [item.id, item.name])), [loreItems])
  const loreSuggestions = useMemo(() => loreItems.map((item) => ({
    value: item.id,
    label: item.name,
    description: `${loreTypeLabel(item.type)} · ${loreImportanceLabel(item.importance)}${item.tags?.length ? ` · ${item.tags.join('、')}` : ''}`,
  })), [loreItems])
  const aiVisible = rightPanel === 'ai'

  const sidebar = (
    <section className="nova-sidebar flex h-full flex-col border-r">
      <div className="flex min-h-[92px] flex-col gap-2 border-b border-[var(--nova-border)] px-3 py-3">
        <div className="flex items-center justify-between">
          <div>
            <div className="text-xs font-medium text-[var(--nova-text)]">{summary?.title || '作品'}</div>
            <div className="mt-0.5 text-[11px] text-[var(--nova-text-faint)]">
              {summary ? `${summary.chapter_count} 章 · ${formatNumber(summary.total_words)} 字` : '正在加载作品进度'}
            </div>
          </div>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={onRefreshTree}
              className="nova-nav-item rounded p-1"
              title="刷新目录"
            >
              <RefreshCw className="h-3.5 w-3.5" />
            </button>
            <button
              type="button"
              onClick={onToggleProjectVisible}
              className="nova-nav-item rounded px-1"
            >
              ×
            </button>
          </div>
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => onSetSidebarView('outline')}
            className={`nova-nav-item flex-1 px-2 py-1 text-xs ${sidebarView === 'outline' ? 'is-active' : 'bg-[var(--nova-surface-2)]'}`}
          >
            作品目录
          </button>
          <button
            type="button"
            onClick={() => onSetSidebarView('files')}
            className={`nova-nav-item flex-1 px-2 py-1 text-xs ${sidebarView === 'files' ? 'is-active' : 'bg-[var(--nova-surface-2)]'}`}
          >
            项目文件
          </button>
        </div>
      </div>
      <div className="flex-1 overflow-y-auto p-2 text-xs">
        {loading ? (
          <div className="py-4 text-center text-[#858b96]">加载中…</div>
        ) : sidebarView === 'outline' ? (
          <ChapterOutline
            chapters={summary?.chapters || []}
            outline={summary?.outline}
            chapterPlans={summary?.chapter_plans || []}
            selectedFile={selectedFile}
            onSelectFile={onSelectFile}
          />
        ) : tree.length === 0 ? (
          <div className="py-4 text-center text-[#858b96]">暂无文件</div>
        ) : (
          <FileTree
            nodes={tree}
            selectedFile={selectedFile}
            onSelectFile={onSelectFile}
            onReferenceFile={onReferenceFile}
            chapterStats={chapterStats}
            onCreateItem={onCreateItem}
            onDeleteItem={onDeleteItem}
            onRenameItem={onRenameItem}
            onCopyItem={onCopyItem}
            onMoveItem={onMoveItem}
          />
        )}
      </div>
    </section>
  )

  const main = (
    <main className={`flex h-full min-w-0 flex-col bg-[var(--nova-bg)] ${mode === 'ide' && !settingsOpen && !ideWorkspacePanel ? 'border-r border-[var(--nova-border)]' : ''}`}>
      {settingsOpen ? (
        <SettingsView onClose={onCloseSettings} />
      ) : mode === 'books' ? (
        <HomeView
          workspace={workspace}
          novaDir={novaDir}
          books={books}
          onSwitch={onSwitchBook}
          onBooksChange={onBooksChange}
          onOpenCharacterCardImport={onOpenCharacterCardImport}
          onClose={() => onSetMode(booksReturnMode)}
        />
      ) : mode === 'interactive' ? (
        <InteractiveLayout
          workspace={workspace}
          rightPanelVisible={interactiveRightVisible}
          onToggleRightPanel={onToggleInteractiveRightPanel}
        />
      ) : ideWorkspacePanel === 'versions' ? (
        <GitPanel
          workspace={workspace}
          refreshSignal={gitRefreshSignal}
          visible={versionsVisible}
          onClose={() => onSetRightPanel(null)}
        />
      ) : ideWorkspacePanel === 'lore' ? (
        <IdeWorkspacePanel
          title="资料库"
          icon={<Database className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />}
          onClose={() => onSetRightPanel(null)}
        >
          <SettingPanel mode="lore" workspace={workspace} />
        </IdeWorkspacePanel>
      ) : ideWorkspacePanel === 'creator' ? (
        <IdeWorkspacePanel
          title="创作者"
          icon={<BookMarked className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />}
          onClose={() => onSetRightPanel(null)}
        >
          <SettingPanel mode="creator" workspace={workspace} />
        </IdeWorkspacePanel>
      ) : ideWorkspacePanel === 'teller' ? (
        <IdeWorkspacePanel
          title="讲述者"
          icon={<SlidersHorizontal className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />}
          onClose={() => onSetRightPanel(null)}
        >
          <SettingPanel mode="teller" workspace={workspace} tellers={tellers} onTellersChange={setTellers} />
        </IdeWorkspacePanel>
      ) : (
        <>
          <TabController
            tabs={openTabs}
            activeTabKey={activeTabKey}
            summary={summary}
            onActivateTab={onActivateTab}
            onCloseTab={onCloseTab}
          />
          <div className="flex min-h-0 flex-1 flex-col">
            {activeTab ? (
              <MarkdownEditor
                fileName={selectedFile}
                content={fileContent}
                onSave={onSaveCurrentFile}
                onQuoteSelection={onQuoteSelection}
                saveSignal={saveSignal}
                chapterSummary={currentChapter}
                workspaceSummary={summary}
                toolbarActions={(
                  <IdeWritingInfoActions
                    projectVisible={projectVisible}
                    aiVisible={aiVisible}
                    onToggleProjectVisible={onToggleProjectVisible}
                    onToggleAgent={() => onSetRightPanel(aiVisible ? null : 'ai')}
                  />
                )}
              />
            ) : (
              <div className="flex h-full items-center justify-center text-xs text-[#7f8590]">
                请从左侧目录树选择文件，或打开「书籍管理」选择书籍
              </div>
            )}
          </div>
        </>
      )}
    </main>
  )

  const rightPanelContent = rightPanel === 'ai' ? (
    <aside className="nova-sidebar flex h-full flex-col">
      <div className="flex h-10 items-center gap-3 border-b border-[var(--nova-border)] px-3">
        <div className="flex shrink-0 items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
          <Bot className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          创作Agent
        </div>
        <IdeTellerSelector workspace={workspace} tellers={tellers} />
        <SessionManager
          sessions={sessions}
          activeSessionId={activeSessionId}
          disabled={isStreaming}
          onCreate={onCreateChatSession}
          onSwitch={onSwitchChatSession}
          onRename={onRenameChatSession}
          onDelete={onDeleteChatSession}
        />
        <div className="flex shrink-0 items-center gap-2">
          <span className="text-[11px] text-[var(--nova-text-faint)]">{isStreaming ? '创作中…' : '等待'}</span>
          <button
            type="button"
            onClick={() => onSetRightPanel(null)}
            className="nova-nav-item rounded px-1 text-xs"
          >
            ×
          </button>
        </div>
      </div>
      {messages.length === 0 && !isStreaming && (
        <AgentQuickActions
          chapter={currentChapter}
          selectedFile={selectedFile}
          onSend={onSend}
        />
      )}
      <MessageList
        messages={messages}
        isStreaming={isStreaming}
        activityContent={activityContent}
        scrollResetKey={`${workspace || 'none'}:${activeSessionId || 'current'}`}
      />
      <InputArea
        onSend={onSend}
        onStop={onStop}
        disabled={isStreaming}
        referencedFiles={references}
        onReferenceRemove={onReferenceRemove}
        fileSuggestions={flattenFileTree(tree)}
        loreReferences={loreReferences}
        loreReferenceLabels={loreReferenceLabels}
        onLoreReferenceAdd={onLoreReferenceAdd}
        onLoreReferenceRemove={onLoreReferenceRemove}
        loreSuggestions={loreSuggestions}
        styleReferences={styleReferences}
        onStyleReferenceAdd={onStyleReferenceAdd}
        onStyleReferenceRemove={onStyleReferenceRemove}
        styleSuggestions={styles}
        textSelections={textSelections}
        onTextSelectionRemove={onTextSelectionRemove}
      />
    </aside>
  ) : null

  return (
    <WorkbenchShell
      mode={mode}
      booksReturnMode={booksReturnMode}
      currentBookName={currentBookName}
      workspace={workspace}
      appVersion={appVersion}
      summary={summary}
      currentChapter={currentChapter}
      isStreaming={isStreaming}
      projectVisible={projectVisible}
      activityBarExpanded={activityBarExpanded}
      rightPanel={rightPanel}
      settingsOpen={settingsOpen}
      interactiveSubmode={interactiveSubmode}
      sidebar={sidebar}
      main={main}
      rightPanelContent={rightPanelContent}
      onSetMode={onSetMode}
      onToggleActivityBarExpanded={onToggleActivityBarExpanded}
      onSetInteractiveSubmode={setInteractiveSubmode}
      onSetRightPanel={onSetRightPanel}
      onToggleSettings={onToggleSettings}
      onCloseSettings={onCloseSettings}
    />
  )
}

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
  const ProjectIcon = projectVisible ? PanelLeftClose : PanelLeftOpen
  const AgentIcon = aiVisible ? PanelRightClose : PanelRightOpen

  return (
    <>
      <button
        type="button"
        onClick={onToggleProjectVisible}
        aria-label={projectVisible ? '隐藏目录' : '显示目录'}
        aria-pressed={projectVisible}
        className={`nova-nav-item flex h-7 w-7 items-center justify-center ${projectVisible ? 'is-active' : ''}`}
        title={projectVisible ? '隐藏目录' : '显示目录'}
      >
        <ProjectIcon className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        onClick={onToggleAgent}
        aria-label={aiVisible ? '隐藏创作 Agent' : '显示创作 Agent'}
        aria-pressed={aiVisible}
        className={`nova-nav-item flex h-7 w-7 items-center justify-center ${aiVisible ? 'is-active' : ''}`}
        title={aiVisible ? '隐藏创作 Agent' : '显示创作 Agent'}
      >
        <AgentIcon className="h-3.5 w-3.5" />
      </button>
    </>
  )
}

function IdeWorkspacePanel({
  title,
  icon,
  children,
  onClose,
}: {
  title: string
  icon: ReactNode
  children: ReactNode
  onClose: () => void
}) {
  return (
    <section className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="nova-topbar flex h-10 shrink-0 items-center justify-between border-b border-[var(--nova-border)] px-3">
        <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
          {icon}
          {title}
        </div>
        <button type="button" onClick={onClose} className="nova-nav-item rounded px-1 text-xs" aria-label={`关闭${title}`}>×</button>
      </div>
      <div className="min-h-0 flex-1">
        {children}
      </div>
    </section>
  )
}

function IdeTellerSelector({ workspace, tellers }: { workspace: string; tellers: Teller[] }) {
  const [value, setValue] = useState('classic')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let cancelled = false
    if (!workspace) {
      setValue('classic')
      return () => { cancelled = true }
    }
    fetchSettings()
      .then((settings) => {
        if (!cancelled) setValue(settings.effective.ide_story_teller_id || 'classic')
      })
      .catch(() => {
        if (!cancelled) setValue('classic')
      })
    return () => { cancelled = true }
  }, [workspace])

  const handleChange = async (next: string) => {
    if (!workspace || next === value) return
    const previous = value
    setValue(next)
    setSaving(true)
    try {
      const settings = await fetchSettings()
      await updateWorkspaceSettings({ ...settings.workspace, ide_story_teller_id: next })
      window.dispatchEvent(new CustomEvent('nova:settings-updated'))
    } catch (e) {
      console.warn('保存 IDE 默认讲述者失败', e)
      setValue(previous)
    } finally {
      setSaving(false)
    }
  }

  if (tellers.length === 0) return null

  return (
    <label className="flex min-w-[150px] shrink-0 items-center gap-1.5 text-[11px] text-[var(--nova-text-faint)]" title="IDE 创作 Agent 下一轮使用的默认讲述者">
      <span className="shrink-0">讲述者</span>
      <select
        value={tellers.some((teller) => teller.id === value) ? value : 'classic'}
        disabled={saving}
        onChange={(event) => void handleChange(event.target.value)}
        className="nova-field h-7 min-w-0 flex-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] text-[var(--nova-text-muted)] outline-none"
      >
        {tellers.map((teller) => (
          <option key={teller.id} value={teller.id}>{teller.name}</option>
        ))}
      </select>
    </label>
  )
}

function ChapterOutline({
  chapters,
  outline,
  chapterPlans,
  selectedFile,
  onSelectFile,
}: {
  chapters: ChapterSummary[]
  outline?: DocumentPreview
  chapterPlans: DocumentPreview[]
  selectedFile: string | null
  onSelectFile: (path: string) => void | Promise<void>
}) {
  const [collapsedVolumes, setCollapsedVolumes] = useState<Set<string>>(() => new Set())
  const volumes = useMemo(() => groupChaptersByVolume(chapters), [chapters])
  const hasPlanning = outline || chapterPlans.length > 0

  const toggleVolume = (key: string) => {
    setCollapsedVolumes(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  if (!hasPlanning && chapters.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-4 text-center text-xs text-[var(--nova-text-faint)]">
        chapters/ 下还没有章节
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <section className="space-y-1.5">
        <div className="px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">大纲</div>
        {outline ? (
          <PlanningListItem document={outline} icon="outline" selected={selectedFile === outline.path} onSelectFile={onSelectFile} />
        ) : (
          <PlanningEmptyState text="setting/outline.md 尚未生成" />
        )}
      </section>

      <section className="space-y-1.5">
        <div className="flex items-center justify-between px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">
          <span>章节组细纲</span>
          {chapterPlans.length > 0 && <span>{chapterPlans.length} 组</span>}
        </div>
        {chapterPlans.length > 0 ? (
          <div className="space-y-1">
            {chapterPlans.map((plan) => (
              <PlanningListItem key={plan.path} document={plan} icon="plan" selected={selectedFile === plan.path} onSelectFile={onSelectFile} />
            ))}
          </div>
        ) : (
          <PlanningEmptyState text="setting/chapter-groups/ 下还没有细纲" />
        )}
      </section>

      <section className="space-y-1.5">
        <div className="px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">分卷章节</div>
        {volumes.length === 0 ? (
          <PlanningEmptyState text="chapters/ 下还没有章节" />
        ) : (
          <div className="space-y-1.5">
            {volumes.map((volume) => {
              const expanded = !collapsedVolumes.has(volume.key)
              return (
                <div key={volume.key} className="space-y-1">
                  <button
                    type="button"
                    className="nova-nav-item flex w-full items-center gap-2 border border-transparent bg-[var(--nova-surface)] px-2 py-1.5 text-left"
                    onClick={() => toggleVolume(volume.key)}
                  >
                    {expanded ? (
                      <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                    ) : (
                      <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                    )}
                    <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                    <span className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--nova-text)]">{volume.label}</span>
                    <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{volume.chapters.length} 章</span>
                  </button>
                  {expanded && (
                    <div className="space-y-1 pl-4">
                      {volume.chapters.map((chapter) => (
                        <ChapterOutlineItem
                          key={chapter.path}
                          chapter={chapter}
                          active={selectedFile === chapter.path}
                          onSelectFile={onSelectFile}
                        />
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </section>
    </div>
  )
}

function PlanningListItem({
  document,
  icon,
  selected,
  onSelectFile,
}: {
  document: DocumentPreview
  icon: 'outline' | 'plan'
  selected: boolean
  onSelectFile: (path: string) => void | Promise<void>
}) {
  const Icon = icon === 'outline' ? BookMarked : FileText
  return (
    <button
      type="button"
      className={`nova-nav-item w-full border px-3 py-2 text-left ${
        selected
          ? 'is-active border-[var(--nova-border)]'
          : 'border-transparent bg-[var(--nova-surface)]'
      }`}
      onClick={() => onSelectFile(document.path)}
    >
      <div className="flex min-w-0 items-center gap-2">
        <Icon className={`h-3.5 w-3.5 shrink-0 ${selected ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
        <span className="min-w-0 flex-1 truncate text-xs font-medium">{document.title}</span>
      </div>
    </button>
  )
}

function PlanningEmptyState({ text }: { text: string }) {
  return (
    <div className="rounded border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-[11px] text-[var(--nova-text-faint)]">
      {text}
    </div>
  )
}

function ChapterOutlineItem({
  chapter,
  active,
  onSelectFile,
}: {
  chapter: ChapterSummary
  active: boolean
  onSelectFile: (path: string) => void | Promise<void>
}) {
  return (
    <button
      type="button"
      className={`nova-nav-item w-full border px-3 py-2 text-left ${
        active
          ? 'is-active border-[var(--nova-border)]'
          : 'border-transparent bg-[var(--nova-surface)]'
      }`}
      onClick={() => onSelectFile(chapter.path)}
    >
      <div className="flex min-w-0 items-center gap-2">
        <BookOpen className={`h-3.5 w-3.5 shrink-0 ${active ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
        <span className="truncate text-xs font-medium">{chapter.display_title}</span>
      </div>
      <div className="mt-1 flex items-center justify-between text-[11px] text-[var(--nova-text-faint)]">
        <span>{formatNumber(chapter.words)} 字</span>
        <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[var(--nova-text-muted)]">{chapter.status}</span>
      </div>
    </button>
  )
}

function groupChaptersByVolume(chapters: ChapterSummary[]) {
  const map = new Map<string, { key: string; label: string; chapters: ChapterSummary[] }>()
  for (const chapter of chapters) {
    const key = chapter.volume_path || chapter.volume || 'chapters'
    const label = chapter.volume || '未分卷'
    const existing = map.get(key)
    if (existing) {
      existing.chapters.push(chapter)
    } else {
      map.set(key, { key, label, chapters: [chapter] })
    }
  }
  return Array.from(map.values())
}

function loreTypeLabel(type: LoreItem['type']) {
  const labels: Record<LoreItem['type'], string> = {
    character: '角色',
    world: '世界观',
    location: '地点',
    faction: '势力',
    rule: '规则',
    item: '物品',
    other: '其他',
  }
  return labels[type] || '资料'
}

function loreImportanceLabel(importance: LoreItem['importance']) {
  const labels: Record<LoreItem['importance'], string> = {
    major: '主要',
    important: '重要',
    minor: '次要',
  }
  return labels[importance] || '资料'
}

function AgentQuickActions({
  chapter,
  selectedFile,
  onSend,
}: {
  chapter?: ChapterSummary
  selectedFile: string | null
  onSend: (message: string) => void
}) {
  const target = chapter ? `当前章节《${chapter.display_title}》` : (selectedFile ? `当前文件 ${selectedFile}` : '当前作品')
  const actions = [
    { label: '下一组细纲', icon: FileText, prompt: '请基于当前大纲、已定稿章节、progress.md 和角色状态，生成接下来一个短期情节单元的章节组细纲。只规划下一组，不要批量生成很多组；如实际定稿已经偏离大纲，请先指出偏差并让我确认是调整大纲还是拉回主线。' },
    { label: '按细纲写下一章', icon: PenLine, prompt: '请读取当前章节组细纲、长期大纲、progress.md、角色状态和前面至少两章定稿正文，按细纲安排创作下一章。若草稿流程未启用且我没有明确要求草稿，请直接写入 chapters/ 作为定稿候选。' },
    { label: '续写下一段', icon: PenLine, prompt: `请基于${target}的上下文，续写下一段正文，保持原有叙事节奏和人物状态。` },
    { label: '润色当前章', icon: WandSparkles, prompt: `请检查并润色${target}，重点优化语句节奏、动作描写和情绪推进，不改变核心剧情。` },
    { label: '定稿并同步状态', icon: FileText, prompt: `请将${target}视为章节定稿，检查其与前后文和当前章节组细纲的连续性，然后同步更新 progress.md 和 characters.md；除非我明确要求，不要修改长期大纲。` },
    { label: '一致性检查', icon: SearchCheck, prompt: `请对${target}做一致性检查，重点关注人物动机、时间线、道具、地点和前后文冲突。` },
  ]

  return (
    <div className="border-b border-[var(--nova-border)] bg-[var(--nova-bg)] p-3">
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-[var(--nova-text-muted)]">
        <Sparkles className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        快捷创作
      </div>
      <div className="grid grid-cols-2 gap-2">
        {actions.map((action) => {
          const Icon = action.icon
          return (
            <button
              key={action.label}
              type="button"
              className="nova-nav-item flex items-center gap-2 border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-left text-xs"
              onClick={() => onSend(action.prompt)}
            >
              <Icon className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
              <span className="truncate">{action.label}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
