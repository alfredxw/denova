import { BookMarked, BookOpen, CheckCircle2, ChevronDown, ChevronRight, Circle, Database, FileText, Loader2, PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen, SlidersHorizontal, Sparkles } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { KeyboardEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { FileTree } from '@/components/Sidebar/FileTree'
import { SearchPanel } from '@/components/Sidebar/SearchPanel'
import { AgentPanel } from '@/components/Chat/AgentPanel'
import { FilePreview } from '@/components/workbench/FilePreview'
import { MarkdownEditor } from '@/components/Editor/MarkdownEditor'
import { VersionPanel } from '@/components/Versions/VersionPanel'
import { HomeView } from '@/components/Home/HomeView'
import { InteractiveLayout } from '@/features/interactive/components/InteractiveLayout'
import { SettingPanel } from '@/features/interactive/components/SettingPanel'
import { getImagePresets, getInteractiveTellers } from '@/features/interactive/api'
import { useInteractiveStore } from '@/features/interactive/stores/interactive-store'
import { AgentsView } from '@/features/agents/AgentsView'
import { AutomationsView } from '@/features/automations/AutomationsView'
import { SkillsView } from '@/features/skills/SkillsView'
import { SettingsView } from '@/features/settings/SettingsView'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import type { FileNode } from '@/hooks/useWorkspace'
import type { BookRecord, ChapterIllustration, ChapterSummary, ChatMessage, ContextAnalysis, DocumentPreview, LoreItem, SessionSummary, TextSelection, WorkspaceSearchResult, WorkspaceSummary } from '@/lib/api'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import { workspaceFileKind } from '@/lib/workspace-file-kind'
import type { Tab } from './TabController'
import { TabController, tabKey } from './TabController'
import { WorkbenchShell } from './WorkbenchShell'
import { flattenFileTree, formatNumber } from './workbench-utils'

const WRITING_AGENT_INIT_EVENT = 'nova:writing-agent-init'
type MainRouteId = 'settings' | 'skills' | 'agents' | 'automations' | 'books' | 'interactive' | 'versions' | 'ide-lore' | 'ide-teller' | 'ide-writing'
type PlanningDocumentIcon = 'ideas' | 'outline' | 'plan' | 'creator' | 'progress' | 'characterState'

interface PlanningDocumentItem {
  document: DocumentPreview
  icon: PlanningDocumentIcon
}

interface PlanningShortcutItem extends PlanningDocumentItem {
  label: string
}

interface ModeRouterProps {
  mode: WorkspaceMode
  booksReturnMode: 'ide' | 'interactive'
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
  openTabs: Tab[]
  activeTabKey: string | null
  sidebarView: 'outline' | 'files' | 'search'
  editorSearchIntent: { path: string; query: string; line: number; nonce: number } | null
  saveSignal: number
  editorAutoSaveEnabled: boolean
  editorAutoSaveDelayMs: number
  versionRefreshSignal: number
  messages: ChatMessage[]
  sessions: SessionSummary[]
  activeSessionId: string
  activityContent: string
  references: string[]
  loreReferences: string[]
  loreItems: LoreItem[]
  styleScenes: string[]
  textSelections: TextSelection[]
  updateNotice?: { latestVersion: string } | null
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
  onSetSidebarView: (view: 'outline' | 'files' | 'search') => void
  onSelectSearchResult: (result: WorkspaceSearchResult, query: string) => void | Promise<void>
  onSelectFile: (path: string) => void | Promise<void>
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
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
  onSend: (message: string, options?: { writingSkill?: string; ideContext?: { currentFile?: string; openFiles?: string[] }; imagePresetId?: string }) => void
  onAnalyzeContext: (message: string, options?: { writingSkill?: string; ideContext?: { currentFile?: string; openFiles?: string[] }; imagePresetId?: string }) => Promise<ContextAnalysis>
  onStop: () => void
  onReferenceRemove: (path: string) => void
  onLoreReferenceAdd: (id: string) => void
  onLoreReferenceRemove: (id: string) => void
  onStyleSceneAdd: (scene: string) => void
  onStyleSceneRemove: (scene: string) => void
  onTextSelectionRemove: (index: number) => void
  onDismissUpdateNotice?: () => void
}

export function ModeRouter(props: ModeRouterProps) {
  const { t, i18n } = useTranslation()
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
    openTabs,
    activeTabKey,
    sidebarView,
    editorSearchIntent,
    saveSignal,
    editorAutoSaveEnabled,
    editorAutoSaveDelayMs,
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
    updateNotice,
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
    onSelectSearchResult,
    onSelectFile,
    onSetChapterConfirmed,
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
    onAnalyzeContext,
    onStop,
    onReferenceRemove,
    onLoreReferenceAdd,
    onLoreReferenceRemove,
    onStyleSceneAdd,
    onStyleSceneRemove,
    onTextSelectionRemove,
    onDismissUpdateNotice,
  } = props

  const activeTab = openTabs.find((tab) => tabKey(tab) === activeTabKey) ?? null
  const activeFileKind = selectedFile ? workspaceFileKind(selectedFile) : null
  const ideContext = useMemo(() => ({
    currentFile: selectedFile || undefined,
    openFiles: openTabs.map((tab) => tab.path),
  }), [openTabs, selectedFile])
  const versionsVisible = rightPanel === 'versions'
  const agentsVisible = mode === 'agents'
  const automationsVisible = mode === 'automations'
  const skillsVisible = mode === 'skills'
  const ideWorkspacePanel = mode === 'ide' && (rightPanel === 'lore' || rightPanel === 'teller') ? rightPanel : null
  const interactiveSubmode = useInteractiveStore((state) => state.submode)
  const setInteractiveSubmode = useInteractiveStore((state) => state.setSubmode)
  const [tellers, setTellers] = useState<Teller[]>([])
  const [imagePresets, setImagePresets] = useState<ImagePreset[]>([])
  const [agentSubAgentDetailsOpen, setAgentSubAgentDetailsOpen] = useState(false)
  const [illustrationInsertSignal, setIllustrationInsertSignal] = useState<{ illustration: ChapterIllustration; nonce: number } | null>(null)

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
  const showSidebarLoading = loading && tree.length === 0 && !summary

  const requestLoreInit = () => {
    onSetMode('interactive')
    setInteractiveSubmode('lore')
  }
  const requestWritingInit = () => {
    onSetMode('ide')
    onSetRightPanel('ai')
    window.setTimeout(() => {
      window.dispatchEvent(new CustomEvent(WRITING_AGENT_INIT_EVENT, {
        detail: { prompt: t('writingAgent.initPrompt') },
      }))
    }, 0)
  }
  const requestSkillsAgent = (prompt: string) => {
    onSetMode('ide')
    onSetRightPanel('ai')
    window.setTimeout(() => {
      window.dispatchEvent(new CustomEvent(WRITING_AGENT_INIT_EVENT, {
        detail: { prompt },
      }))
    }, 0)
  }
  const requestChapterIllustration = (chapterPath: string) => {
    const target = currentChapter?.path || chapterPath || selectedFile || ''
    if (!target) return
    onSetMode('ide')
    onSetRightPanel('ai')
    window.setTimeout(() => {
      window.dispatchEvent(new CustomEvent(WRITING_AGENT_INIT_EVENT, {
        detail: {
          autoSend: true,
          prompt: [
            '/<chapter-illustration>',
            '',
            `目标章节 / Target chapter: ${target}`,
            '',
            '请基于这个章节生成一张非剧透插画。只生成图像和 meta.json，不要自动插入正文；生成后等待我手动点击“插入正文”。',
          ].join('\n'),
        },
      }))
    }, 0)
  }
  const insertIllustrationIntoEditor = (illustration: ChapterIllustration) => {
    const apply = () => {
      setIllustrationInsertSignal((current) => ({ illustration, nonce: (current?.nonce || 0) + 1 }))
    }
    if (illustration.chapter_path && selectedFile !== illustration.chapter_path) {
      void Promise.resolve(onSelectFile(illustration.chapter_path)).finally(() => window.setTimeout(apply, 0))
      return
    }
    apply()
  }
  const aiVisible = rightPanel === 'ai'
  const closeBooks = () => {
    if (booksReturnMode === 'interactive') {
      onSetMode('interactive')
      return
    }
    onSetMode('ide')
    if (rightPanel === 'lore' || rightPanel === 'teller' || rightPanel === 'versions') onSetRightPanel(null)
  }
  const visibleMainRoute: MainRouteId = settingsOpen
    ? 'settings'
    : skillsVisible
      ? 'skills'
      : agentsVisible
        ? 'agents'
        : automationsVisible
          ? 'automations'
          : mode === 'books'
            ? 'books'
            : versionsVisible
              ? 'versions'
              : mode === 'interactive'
                ? 'interactive'
                : ideWorkspacePanel
                  ? `ide-${ideWorkspacePanel}`
                  : 'ide-writing'
  const [mountedRoutes, setMountedRoutes] = useState<ReadonlySet<MainRouteId>>(() => new Set(['ide-writing', visibleMainRoute]))

  useEffect(() => {
    setMountedRoutes((current) => {
      if (current.has(visibleMainRoute)) return current
      const next = new Set(current)
      next.add(visibleMainRoute)
      return next
    })
  }, [visibleMainRoute])

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
      <div className="flex-1 overflow-y-auto p-2 text-xs">
        {showSidebarLoading ? (
          <div className="py-4 text-center text-[var(--nova-text-muted)]">{t('router.loading')}</div>
        ) : sidebarView === 'outline' ? (
          <ChapterOutline
            chapters={summary?.chapters || []}
            ideas={summary?.ideas}
            outline={summary?.outline}
            chapterPlans={summary?.chapter_plans || []}
            selectedFile={selectedFile}
            onSelectFile={onSelectFile}
            onSetChapterConfirmed={onSetChapterConfirmed}
          />
        ) : sidebarView === 'search' ? (
          <SearchPanel
            workspace={workspace}
            onSelectResult={onSelectSearchResult}
          />
        ) : tree.length === 0 ? (
          <div className="py-4 text-center text-[var(--nova-text-muted)]">{t('router.noFiles')}</div>
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
    <main className="relative h-full min-w-0 overflow-hidden bg-[var(--nova-bg)]">
      <MainRouteLayer visible={visibleMainRoute === 'ide-writing'}>
        <TabController
          tabs={openTabs}
          activeTabKey={activeTabKey}
          summary={summary}
          actions={(
            <IdeWritingInfoActions
              projectVisible={projectVisible}
              aiVisible={aiVisible}
              onToggleProjectVisible={onToggleProjectVisible}
              onToggleAgent={() => onSetRightPanel(aiVisible ? null : 'ai')}
            />
          )}
          onActivateTab={onActivateTab}
          onCloseTab={onCloseTab}
        />
        <div className="flex min-h-0 flex-1 flex-col">
          {activeTab ? (
            activeFileKind === 'image' || activeFileKind === 'json' || activeFileKind === 'jsonl' ? (
              <FilePreview path={selectedFile || activeTab.path} content={fileContent} />
            ) : (
              <MarkdownEditor
                fileName={selectedFile}
                content={fileContent}
                onSave={onSaveCurrentFile}
                onQuoteSelection={onQuoteSelection}
                saveSignal={saveSignal}
                autoSaveEnabled={editorAutoSaveEnabled}
                autoSaveDelayMs={editorAutoSaveDelayMs}
                chapterSummary={currentChapter}
                searchIntent={editorSearchIntent?.path === selectedFile ? editorSearchIntent : null}
                onGenerateIllustration={requestChapterIllustration}
                generateIllustrationDisabled={isStreaming || !currentChapter}
                illustrationInsertSignal={illustrationInsertSignal}
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
      </MainRouteLayer>

      {mountedRoutes.has('interactive') && (
        <MainRouteLayer visible={visibleMainRoute === 'interactive'}>
          <InteractiveLayout
            workspace={workspace}
            imagePresets={imagePresets}
            onImagePresetsChange={setImagePresets}
            loreEmpty={loreEmpty}
            onRequestLoreInit={requestLoreInit}
            rightPanelVisible={interactiveRightVisible}
            onToggleRightPanel={onToggleInteractiveRightPanel}
          />
        </MainRouteLayer>
      )}

      {mountedRoutes.has('versions') && (
        <MainRouteLayer visible={visibleMainRoute === 'versions'}>
          <VersionPanel
            workspace={workspace}
            refreshSignal={versionRefreshSignal}
            visible={versionsVisible}
            onClose={() => onSetRightPanel(null)}
          />
        </MainRouteLayer>
      )}
      {mountedRoutes.has('ide-lore') && (
        <MainRouteLayer visible={visibleMainRoute === 'ide-lore'}>
          <IdeWorkspacePanel
            title={t('workbench.activity.lore')}
            icon={<Database className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />}
            onClose={() => onSetRightPanel(null)}
          >
            <SettingPanel mode="lore" workspace={workspace} />
          </IdeWorkspacePanel>
        </MainRouteLayer>
      )}
      {mountedRoutes.has('ide-teller') && (
        <MainRouteLayer visible={visibleMainRoute === 'ide-teller'}>
          <IdeWorkspacePanel
            title={t('workbench.activity.teller')}
            icon={<SlidersHorizontal className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />}
            onClose={() => onSetRightPanel(null)}
          >
            <SettingPanel mode="teller" workspace={workspace} tellers={tellers} imagePresets={imagePresets} onTellersChange={setTellers} onImagePresetsChange={setImagePresets} />
          </IdeWorkspacePanel>
        </MainRouteLayer>
      )}

      {mountedRoutes.has('books') && (
        <MainRouteLayer visible={visibleMainRoute === 'books'}>
          <HomeView
            workspace={workspace}
            novaDir={novaDir}
            books={books}
            onSwitch={onSwitchBook}
            onBooksChange={onBooksChange}
            onOpenCharacterCardImport={onOpenCharacterCardImport}
            onClose={closeBooks}
          />
        </MainRouteLayer>
      )}
      {mountedRoutes.has('skills') && (
        <MainRouteLayer visible={visibleMainRoute === 'skills'}>
          <SkillsView workspace={workspace} onClose={() => onSetMode(booksReturnMode)} onRequestAgent={requestSkillsAgent} />
        </MainRouteLayer>
      )}
      {mountedRoutes.has('agents') && (
        <MainRouteLayer visible={visibleMainRoute === 'agents'}>
          <AgentsView onClose={() => onSetMode(booksReturnMode)} />
        </MainRouteLayer>
      )}
      {mountedRoutes.has('automations') && (
        <MainRouteLayer visible={visibleMainRoute === 'automations'}>
          <AutomationsView workspace={workspace} onClose={() => onSetMode(booksReturnMode)} />
        </MainRouteLayer>
      )}
      {mountedRoutes.has('settings') && (
        <MainRouteLayer visible={visibleMainRoute === 'settings'}>
          <SettingsView onClose={onCloseSettings} />
        </MainRouteLayer>
      )}
    </main>
  )

  const rightPanelContent = rightPanel === 'ai' ? (
    <AgentPanel
      workspace={workspace}
      currentChapter={currentChapter}
      selectedFile={selectedFile}
      tellers={tellers}
      imagePresets={imagePresets}
      messages={messages}
      sessions={sessions}
      activeSessionId={activeSessionId}
      isStreaming={isStreaming}
      activityContent={activityContent}
      references={references}
      loreReferences={loreReferences}
      loreReferenceLabels={loreReferenceLabels}
      loreSuggestions={loreSuggestions}
      styleScenes={styleScenes}
      textSelections={textSelections}
      fileSuggestions={flattenFileTree(tree)}
      onCreateSession={onCreateChatSession}
      onSwitchSession={onSwitchChatSession}
      onRenameSession={onRenameChatSession}
      onDeleteSession={onDeleteChatSession}
      onSend={onSend}
      onAnalyzeContext={onAnalyzeContext}
      ideContext={ideContext}
      onStop={onStop}
      onReferenceRemove={onReferenceRemove}
      onLoreReferenceAdd={onLoreReferenceAdd}
      onLoreReferenceRemove={onLoreReferenceRemove}
      onStyleSceneAdd={onStyleSceneAdd}
      onStyleSceneRemove={onStyleSceneRemove}
      onTextSelectionRemove={onTextSelectionRemove}
      onInsertIllustration={insertIllustrationIntoEditor}
      onClose={() => onSetRightPanel(null)}
      onSubAgentDetailsChange={setAgentSubAgentDetailsOpen}
    />
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
      rightPanelWide={agentSubAgentDetailsOpen}
      settingsOpen={settingsOpen}
      interactiveSubmode={interactiveSubmode}
      sidebar={sidebar}
      main={main}
      rightPanelContent={rightPanelContent}
      updateNotice={updateNotice}
      onSetMode={onSetMode}
      onToggleActivityBarExpanded={onToggleActivityBarExpanded}
      onSetInteractiveSubmode={setInteractiveSubmode}
      onSetRightPanel={onSetRightPanel}
      onToggleSettings={onToggleSettings}
      onCloseSettings={onCloseSettings}
      onDismissUpdateNotice={onDismissUpdateNotice}
    />
  )
}

function MainRouteLayer({ visible, children }: { visible: boolean; children: ReactNode }) {
  return (
    <section hidden={!visible} aria-hidden={!visible} className="absolute inset-0 flex min-h-0 flex-col">
      {children}
    </section>
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
  const { t } = useTranslation()
  return (
    <section className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="nova-topbar flex h-10 shrink-0 items-center justify-between border-b border-[var(--nova-border)] px-3">
        <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
          {icon}
          {title}
        </div>
        <button type="button" onClick={onClose} className="nova-nav-item rounded px-1 text-xs" aria-label={`${t('common.close')} ${title}`}>×</button>
      </div>
      <div className="min-h-0 flex-1">
        {children}
      </div>
    </section>
  )
}

function ChapterOutline({
  chapters,
  ideas,
  outline,
  chapterPlans,
  selectedFile,
  onSelectFile,
  onSetChapterConfirmed,
}: {
  chapters: ChapterSummary[]
  ideas?: DocumentPreview
  outline?: DocumentPreview
  chapterPlans: DocumentPreview[]
  selectedFile: string | null
  onSelectFile: (path: string) => void | Promise<void>
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [collapsedVolumes, setCollapsedVolumes] = useState<Set<string>>(() => new Set())
  const [bookSettingsExpanded, setBookSettingsExpanded] = useState(false)
  const [chapterPlanHistoryExpanded, setChapterPlanHistoryExpanded] = useState(false)
  const volumes = useMemo(() => groupChaptersByVolume(chapters, t), [chapters, t])
  const settingShortcuts = useMemo<PlanningShortcutItem[]>(() => [
    { document: planningDocument(outline, 'setting/outline.md', t('planning.outline')), icon: 'outline', label: t('planning.outlineTab') },
    { document: planningDocument(undefined, 'CREATOR.md', t('planning.creatorRules')), icon: 'creator', label: t('planning.creatorRulesTab') },
    { document: planningDocument(undefined, 'setting/progress.md', t('planning.writingProgress')), icon: 'progress', label: t('planning.writingProgressTab') },
  ], [outline, t])
  const bookSettings = useMemo<PlanningDocumentItem[]>(() => [
    { document: planningDocument(ideas, 'ideas.md', t('planning.ideas')), icon: 'ideas' },
    { document: planningDocument(undefined, 'setting/character-states.md', t('planning.characterStates')), icon: 'characterState' },
  ], [ideas, t])
  const hasPlanning = settingShortcuts.length > 0 || bookSettings.length > 0 || chapterPlans.length > 0
  const latestChapterPlan = chapterPlans[chapterPlans.length - 1]
  const historicalChapterPlans = useMemo(() => chapterPlans.slice(0, -1), [chapterPlans])
  useEffect(() => {
    if (selectedFile && historicalChapterPlans.some((plan) => plan.path === selectedFile)) {
      setChapterPlanHistoryExpanded(true)
    }
  }, [historicalChapterPlans, selectedFile])

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
        {t('planning.noChapters')}
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <section className="space-y-1.5">
        <div className="flex items-center justify-between gap-2 px-1">
          <span className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('planning.bookSettings')}</span>
          <button
            type="button"
            className="nova-nav-item flex min-w-0 items-center gap-1 rounded-[var(--nova-radius)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]"
            onClick={() => setBookSettingsExpanded((expanded) => !expanded)}
          >
            {bookSettingsExpanded ? (
              <ChevronDown className="h-3 w-3 shrink-0" />
            ) : (
              <ChevronRight className="h-3 w-3 shrink-0" />
            )}
            <span className="truncate">{t('planning.bookSettingCount', { count: bookSettings.length })}</span>
          </button>
        </div>
        <div className="flex items-center gap-1">
          {settingShortcuts.map((item) => {
            const selected = selectedFile === item.document.path
            return (
              <button
                key={item.document.path}
                type="button"
                className={`nova-nav-item min-w-0 flex-1 px-1.5 py-1 text-[11px] font-medium ${
                  selected ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'
                }`}
                title={item.document.title}
                onClick={() => onSelectFile(item.document.path)}
              >
                <span className="block truncate">{item.label}</span>
              </button>
            )
          })}
        </div>
        {bookSettingsExpanded && (
          <div className="space-y-0.5 pl-1">
            {bookSettings.map((item) => (
              <PlanningListItem
                key={item.document.path}
                document={item.document}
                icon={item.icon}
                selected={selectedFile === item.document.path}
                onSelectFile={onSelectFile}
                compact
              />
            ))}
          </div>
        )}
      </section>

      <section className="space-y-1.5">
        <div className="flex items-center justify-between px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">
          <span>{t('planning.chapterPlans')}</span>
          {chapterPlans.length > 0 && <span>{t('planning.chapterPlanCount', { count: chapterPlans.length })}</span>}
        </div>
        {chapterPlans.length > 0 ? (
          <div className="space-y-1">
            {latestChapterPlan && (
              <PlanningListItem document={latestChapterPlan} icon="plan" selected={selectedFile === latestChapterPlan.path} onSelectFile={onSelectFile} />
            )}
            {historicalChapterPlans.length > 0 && (
              <div className="space-y-1">
                <button
                  type="button"
                  className="nova-nav-item flex w-full items-center gap-2 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-[11px] text-[var(--nova-text-muted)]"
                  onClick={() => setChapterPlanHistoryExpanded((expanded) => !expanded)}
                >
                  {chapterPlanHistoryExpanded ? (
                    <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  ) : (
                    <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  )}
                  <span className="min-w-0 flex-1 truncate">{t('planning.chapterPlanHistory')}</span>
                  <span className="shrink-0 text-[var(--nova-text-faint)]">{t('planning.chapterPlanCount', { count: historicalChapterPlans.length })}</span>
                </button>
                {chapterPlanHistoryExpanded && (
                  <div className="space-y-1 pl-4">
                    {historicalChapterPlans.map((plan) => (
                      <PlanningListItem key={plan.path} document={plan} icon="plan" selected={selectedFile === plan.path} onSelectFile={onSelectFile} />
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        ) : (
          <PlanningEmptyState text={t('planning.chapterPlansEmpty')} />
        )}
      </section>

      <section className="space-y-1.5">
        <div className="px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('planning.volumeChapters')}</div>
        {volumes.length === 0 ? (
          <PlanningEmptyState text={t('planning.noChapters')} />
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
                    <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{t('common.chapters', { count: volume.chapters.length })}</span>
                  </button>
                  {expanded && (
                    <div className="space-y-1 pl-4">
                      {volume.chapters.map((chapter) => (
                        <ChapterOutlineItem
                          key={chapter.path}
                          chapter={chapter}
                          active={selectedFile === chapter.path}
                          onSelectFile={onSelectFile}
                          onSetChapterConfirmed={onSetChapterConfirmed}
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
  compact = false,
}: {
  document: DocumentPreview
  icon: PlanningDocumentIcon
  selected: boolean
  onSelectFile: (path: string) => void | Promise<void>
  compact?: boolean
}) {
  const Icon = planningIcon(icon)
  return (
    <button
      type="button"
      className={`nova-nav-item w-full border text-left ${compact ? 'px-2 py-1' : 'px-3 py-2'} ${
        selected
          ? 'is-active border-[var(--nova-border)]'
          : 'border-transparent bg-[var(--nova-surface)]'
      }`}
      onClick={() => onSelectFile(document.path)}
    >
      <div className={`flex min-w-0 items-center ${compact ? 'gap-1.5' : 'gap-2'}`}>
        <Icon className={`${compact ? 'h-3 w-3' : 'h-3.5 w-3.5'} shrink-0 ${selected ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
        <span className={`min-w-0 flex-1 truncate font-medium ${compact ? 'text-[11px]' : 'text-xs'}`}>{document.title}</span>
      </div>
    </button>
  )
}

function planningDocument(source: DocumentPreview | undefined, path: string, title: string): DocumentPreview {
  return {
    path: source?.path ?? path,
    title,
    excerpt: source?.excerpt ?? '',
    words: source?.words ?? 0,
    updated_at: source?.updated_at ?? '',
  }
}

function planningIcon(icon: PlanningDocumentIcon) {
  switch (icon) {
    case 'outline':
      return BookMarked
    case 'creator':
      return SlidersHorizontal
    case 'progress':
      return CheckCircle2
    case 'ideas':
    case 'plan':
    case 'characterState':
      return FileText
  }
}

function PlanningEmptyState({ text }: { text: string }) {
  return (
    <div className="rounded border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-[11px] text-[var(--nova-text-faint)]">
      {text}
    </div>
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

function ChapterOutlineItem({
  chapter,
  active,
  onSelectFile,
  onSetChapterConfirmed,
}: {
  chapter: ChapterSummary
  active: boolean
  onSelectFile: (path: string) => void | Promise<void>
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const handleSelect = () => {
    void onSelectFile(chapter.path)
  }
  const handleSelectKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    handleSelect()
  }
  const handleToggleConfirmed = async () => {
    if (saving || chapter.words === 0) return
    setSaving(true)
    try {
      await onSetChapterConfirmed(chapter.path, !chapter.confirmed)
    } catch (error) {
      console.error('更新章节确认状态失败', error)
    } finally {
      setSaving(false)
    }
  }
  const ConfirmIcon = saving ? Loader2 : chapter.confirmed ? CheckCircle2 : Circle
  const toggleTitle = saving ? t('common.loading') : chapter.confirmed ? t('planning.markDraft') : t('planning.confirmChapter')
  return (
    <div
      className={`nova-nav-item w-full border px-3 py-2 text-left ${
        active
          ? 'is-active border-[var(--nova-border)]'
          : 'border-transparent bg-[var(--nova-surface)]'
      }`}
      role="button"
      tabIndex={0}
      onClick={handleSelect}
      onKeyDown={handleSelectKeyDown}
    >
      <div className="flex w-full min-w-0 items-center gap-2 text-left">
        <BookOpen className={`h-3.5 w-3.5 shrink-0 ${active ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
        <span className="min-w-0 flex-1 truncate text-xs font-medium">{chapter.display_title}</span>
      </div>
      <div className="mt-1 flex items-center justify-between text-[11px] text-[var(--nova-text-faint)]">
        <span>{t('common.words', { count: formatNumber(chapter.words) })}</span>
        <div className="flex items-center gap-1.5">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[var(--nova-text-muted)]">{chapter.status}</span>
          <button
            type="button"
            className={`inline-flex h-5 w-5 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-faint)] hover:bg-[var(--nova-surface-2)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-40 ${saving ? 'opacity-70' : ''}`}
            disabled={chapter.words === 0}
            title={toggleTitle}
            aria-label={toggleTitle}
            aria-busy={saving}
            aria-disabled={saving || chapter.words === 0}
            onClick={(event) => {
              event.stopPropagation()
              void handleToggleConfirmed()
            }}
          >
            <ConfirmIcon className={`h-3.5 w-3.5 ${saving ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>
    </div>
  )
}

function groupChaptersByVolume(chapters: ChapterSummary[], t: (key: string) => string) {
  const map = new Map<string, { key: string; label: string; chapters: ChapterSummary[] }>()
  for (const chapter of chapters) {
    const key = chapter.volume_path || chapter.volume || 'chapters'
    const label = chapter.volume || t('planning.unvolumed')
    const existing = map.get(key)
    if (existing) {
      existing.chapters.push(chapter)
    } else {
      map.set(key, { key, label, chapters: [chapter] })
    }
  }
  return Array.from(map.values())
}

function loreTypeLabel(type: LoreItem['type'], t: (key: string) => string) {
  const key = `lore.type.${type}`
  const label = t(key)
  return label === key ? t('lore.type.default') : label
}

function loreImportanceLabel(importance: LoreItem['importance'], t: (key: string) => string) {
  const key = `lore.importance.${importance}`
  const label = t(key)
  return label === key ? t('lore.importance.default') : label
}

function loreLoadModeLabel(loadMode: LoreItem['load_mode'], t: (key: string) => string) {
  const key = `lore.loadMode.${loadMode}`
  const label = t(key)
  return label === key ? t('lore.loadMode.default') : label
}
