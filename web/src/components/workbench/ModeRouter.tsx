import { BookMarked, BookOpen, Bot, Database, FileText, FolderTree, RefreshCw, SearchCheck, SlidersHorizontal, Sparkles, WandSparkles, PenLine } from 'lucide-react'
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
import type { Teller } from '@/features/interactive/types'
import type { FileNode } from '@/hooks/useWorkspace'
import type { BookRecord, ChapterSummary, ChatMessage, LoreItem, SessionSummary, TextSelection, WorkspaceSummary } from '@/lib/api'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import type { Tab } from './TabController'
import { TabController, tabKey } from './TabController'
import { WorkbenchShell } from './WorkbenchShell'
import { flattenFileTree, formatNumber } from './workbench-utils'

interface ModeRouterProps {
  mode: WorkspaceMode
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
  const ideWorkspacePanel = mode === 'ide' && (rightPanel === 'lore' || rightPanel === 'creator' || rightPanel === 'teller') ? rightPanel : null
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
    <main className={`flex h-full min-w-0 flex-col bg-[var(--nova-bg)] ${mode === 'ide' && !ideWorkspacePanel ? 'border-r border-[var(--nova-border)]' : ''}`}>
      {mode === 'books' ? (
        <HomeView
          workspace={workspace}
          novaDir={novaDir}
          books={books}
          onSwitch={onSwitchBook}
          onBooksChange={onBooksChange}
          onOpenCharacterCardImport={onOpenCharacterCardImport}
        />
      ) : mode === 'interactive' ? (
        <InteractiveLayout
          workspace={workspace}
          rightPanelVisible={interactiveRightVisible}
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
          <IdeWritingToolbar
            projectVisible={projectVisible}
            aiVisible={aiVisible}
            onToggleProjectVisible={onToggleProjectVisible}
            onToggleAgent={() => onSetRightPanel(aiVisible ? null : 'ai')}
          />
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
  ) : rightPanel === 'versions' ? (
    <GitPanel
      workspace={workspace}
      refreshSignal={gitRefreshSignal}
      visible={versionsVisible}
      onClose={() => onSetRightPanel(null)}
    />
  ) : null

  return (
    <WorkbenchShell
      mode={mode}
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
      interactiveRightPanelVisible={interactiveRightVisible}
      sidebar={sidebar}
      main={main}
      rightPanelContent={rightPanelContent}
      onSetMode={onSetMode}
      onToggleActivityBarExpanded={onToggleActivityBarExpanded}
      onSetInteractiveSubmode={setInteractiveSubmode}
      onToggleInteractiveRightPanel={onToggleInteractiveRightPanel}
      onSetRightPanel={onSetRightPanel}
      onToggleSettings={onToggleSettings}
    />
  )
}

function IdeWritingToolbar({
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
  return (
    <div className="nova-topbar flex h-9 shrink-0 items-center gap-1 border-b border-[var(--nova-border)] px-3 text-xs">
      <button
        type="button"
        onClick={onToggleProjectVisible}
        className={`nova-nav-item flex items-center gap-1.5 px-2 py-1 ${projectVisible ? 'is-active' : ''}`}
        title={projectVisible ? '隐藏目录' : '显示目录'}
      >
        <FolderTree className="h-3.5 w-3.5" />
        目录
      </button>
      <button
        type="button"
        onClick={onToggleAgent}
        className={`nova-nav-item flex items-center gap-1.5 px-2 py-1 ${aiVisible ? 'is-active' : ''}`}
        title={aiVisible ? '隐藏创作 Agent' : '显示创作 Agent'}
      >
        <Bot className="h-3.5 w-3.5" />
        Agent
      </button>
    </div>
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
  selectedFile,
  onSelectFile,
}: {
  chapters: ChapterSummary[]
  selectedFile: string | null
  onSelectFile: (path: string) => void | Promise<void>
}) {
  if (chapters.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-4 text-center text-xs text-[var(--nova-text-faint)]">
        chapters/ 下还没有章节
      </div>
    )
  }

  return (
    <div className="space-y-1.5">
      {chapters.map((chapter) => {
        const active = selectedFile === chapter.path
        return (
          <button
            key={chapter.path}
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
      })}
    </div>
  )
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
    { label: '续写下一段', icon: PenLine, prompt: `请基于${target}的上下文，续写下一段正文，保持原有叙事节奏和人物状态。` },
    { label: '润色当前章', icon: WandSparkles, prompt: `请检查并润色${target}，重点优化语句节奏、动作描写和情绪推进，不改变核心剧情。` },
    { label: '提取本章摘要', icon: FileText, prompt: `请为${target}提取章节摘要，包含关键事件、角色状态变化、伏笔和下一章衔接点。` },
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
