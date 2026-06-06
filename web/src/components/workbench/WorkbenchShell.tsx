import type { ReactNode } from 'react'
import { BookMarked, BookOpen, Bot, Database, History, MessageSquareText, PanelLeft, PenLine, Settings, SlidersHorizontal } from 'lucide-react'
import { WorkspaceLayout } from '@/components/layout/workspace-layout'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import type { ChapterSummary, WorkspaceSummary } from '@/lib/api'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import type { InteractiveSubmode } from '@/features/interactive/types'
import { formatNumber } from './workbench-utils'

interface WorkbenchShellProps {
  mode: WorkspaceMode
  booksReturnMode: 'ide' | 'interactive'
  currentBookName: string
  workspace: string
  appVersion: string
  summary: WorkspaceSummary | null
  currentChapter?: ChapterSummary
  isStreaming: boolean
  projectVisible: boolean
  activityBarExpanded: boolean
  rightPanel: RightPanel
  settingsOpen: boolean
  interactiveSubmode: InteractiveSubmode
  sidebar: ReactNode
  main: ReactNode
  rightPanelContent: ReactNode
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onSetInteractiveSubmode: (mode: InteractiveSubmode) => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleSettings: () => void
  onCloseSettings: () => void
}

export function WorkbenchShell({
  mode,
  booksReturnMode,
  currentBookName,
  workspace,
  appVersion,
  summary,
  currentChapter,
  isStreaming,
  projectVisible,
  activityBarExpanded,
  rightPanel,
  settingsOpen,
  interactiveSubmode,
  sidebar,
  main,
  rightPanelContent,
  onSetMode,
  onToggleActivityBarExpanded,
  onSetInteractiveSubmode,
  onSetRightPanel,
  onToggleSettings,
  onCloseSettings,
}: WorkbenchShellProps) {
  const loreVisible = rightPanel === 'lore'
  const creatorVisible = rightPanel === 'creator'
  const tellerVisible = rightPanel === 'teller'
  const versionsVisible = rightPanel === 'versions'
  const ideModeActive = mode === 'ide' && !settingsOpen
  const agentsActive = mode === 'agents' && !settingsOpen
  const fullWorkspacePanelVisible = settingsOpen || mode === 'agents' || (mode === 'ide' && (loreVisible || creatorVisible || tellerVisible || versionsVisible))
  const modeLabel = settingsOpen ? '设置' : mode === 'interactive' ? '互动工作台' : mode === 'books' ? '书籍管理' : mode === 'agents' ? 'Agents' : '小说 IDE'
  const navigationMode = mode === 'books' || mode === 'agents' ? booksReturnMode : mode

  const closeSettingsIfOpen = () => {
    if (settingsOpen) onCloseSettings()
  }

  const openWriting = () => {
    closeSettingsIfOpen()
    onSetMode('ide')
    if (loreVisible || creatorVisible || tellerVisible || versionsVisible) onSetRightPanel(null)
  }

  const toggleIdePanel = (panel: NonNullable<RightPanel>) => {
    closeSettingsIfOpen()
    onSetMode('ide')
    onSetRightPanel(rightPanel === panel ? null : panel)
  }

  const openInteractiveSubmode = (nextMode: InteractiveSubmode) => {
    closeSettingsIfOpen()
    onSetMode('interactive')
    onSetInteractiveSubmode(nextMode)
  }

  const returnFromBooks = () => {
    if (booksReturnMode === 'interactive') {
      onSetMode('interactive')
      onSetInteractiveSubmode('story')
      return
    }
    onSetMode('ide')
    if (loreVisible || creatorVisible || tellerVisible || versionsVisible) onSetRightPanel(null)
  }

  const openBooks = () => {
    if (mode === 'books' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    onSetMode('books')
  }

  const openAgents = () => {
    if (mode === 'agents' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    onSetMode('agents')
  }

  const topBar = (
    <header className="nova-topbar grid h-10 shrink-0 grid-cols-[auto_1fr_auto] items-center border-b px-3 text-xs">
      <div className="flex items-center gap-3">
        <div className="font-semibold text-[var(--nova-text)]">Nova</div>
        <div className="flex h-7 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label="模式切换">
          <button
            type="button"
            onClick={() => onSetMode('ide')}
            className={`rounded-[6px] px-2.5 py-0.5 text-[11px] transition-colors ${navigationMode === 'ide' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            IDE 模式
          </button>
          <button
            type="button"
            onClick={() => onSetMode('interactive')}
            className={`rounded-[6px] px-2.5 py-0.5 text-[11px] transition-colors ${navigationMode === 'interactive' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            互动模式
          </button>
        </div>
      </div>
      <div className="mx-auto flex min-w-0 max-w-[520px] items-center justify-center gap-1.5" title={workspace || currentBookName}>
        <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
        <span className="truncate font-medium text-[var(--nova-text)]">{currentBookName}</span>
      </div>
      <div className="nova-ui-compact flex items-center justify-end gap-2 text-[var(--nova-text-faint)]">
        <span>{modeLabel}</span>
      </div>
    </header>
  )

  const ideActivityButtons = (
    <>
      <ActivityButton
        expanded={activityBarExpanded}
        label="资料库"
        onClick={() => toggleIdePanel('lore')}
        className={`nova-icon-button mb-2 ${ideModeActive && loreVisible ? 'is-active' : ''}`}
      >
        <Database className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="创作者"
        onClick={() => toggleIdePanel('creator')}
        className={`nova-icon-button mb-2 ${ideModeActive && creatorVisible ? 'is-active' : ''}`}
      >
        <BookMarked className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="讲述者"
        onClick={() => toggleIdePanel('teller')}
        className={`nova-icon-button mb-2 ${ideModeActive && tellerVisible ? 'is-active' : ''}`}
      >
        <SlidersHorizontal className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="版本管理"
        onClick={() => toggleIdePanel('versions')}
        className={`nova-icon-button mb-2 ${ideModeActive && versionsVisible ? 'is-active' : ''}`}
      >
        <History className="h-4 w-4" />
      </ActivityButton>
    </>
  )

  const interactiveActivityButtons = (
    <>
      <ActivityButton
        expanded={activityBarExpanded}
        label="剧情"
        onClick={() => openInteractiveSubmode('story')}
        className={`nova-icon-button mb-2 ${mode === 'interactive' && interactiveSubmode === 'story' ? 'is-active' : ''}`}
      >
        <MessageSquareText className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="剧情路线图"
        onClick={() => openInteractiveSubmode('timeline')}
        className={`nova-icon-button mb-2 ${mode === 'interactive' && interactiveSubmode === 'timeline' ? 'is-active' : ''}`}
      >
        <History className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="资料库"
        onClick={() => openInteractiveSubmode('lore')}
        className={`nova-icon-button mb-2 ${mode === 'interactive' && interactiveSubmode === 'lore' ? 'is-active' : ''}`}
      >
        <Database className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="创作者"
        onClick={() => openInteractiveSubmode('creator')}
        className={`nova-icon-button mb-2 ${mode === 'interactive' && interactiveSubmode === 'creator' ? 'is-active' : ''}`}
      >
        <BookMarked className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="讲述者"
        onClick={() => openInteractiveSubmode('teller')}
        className={`nova-icon-button mb-2 ${mode === 'interactive' && interactiveSubmode === 'teller' ? 'is-active' : ''}`}
      >
        <SlidersHorizontal className="h-4 w-4" />
      </ActivityButton>
    </>
  )

  const activityBar = (
    <aside className={`nova-activity-bar flex shrink-0 flex-col gap-2 border-r p-3 transition-[width] duration-500 ease-[var(--nova-ease)] ${activityBarExpanded ? 'is-expanded w-48 items-stretch' : 'w-16 items-center'}`}>
      {navigationMode === 'interactive' ? interactiveActivityButtons : (
        <>
          <ActivityButton
            expanded={activityBarExpanded}
            label="写作"
            onClick={openWriting}
            className={`nova-icon-button mb-2 ${mode === 'ide' && !settingsOpen && !loreVisible && !creatorVisible && !tellerVisible && !versionsVisible ? 'is-active' : ''}`}
          >
            <PenLine className="h-4 w-4" />
          </ActivityButton>
          {ideActivityButtons}
        </>
      )}
      <ActivityButton
        expanded={activityBarExpanded}
        label="书籍管理"
        onClick={openBooks}
        className={`nova-icon-button mb-2 ${mode === 'books' && !settingsOpen ? 'is-active' : ''}`}
      >
        <BookOpen className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="Agents"
        onClick={openAgents}
        className={`nova-icon-button mb-2 ${agentsActive ? 'is-active' : ''}`}
      >
        <Bot className="h-4 w-4" />
      </ActivityButton>
      <div className="mt-auto flex flex-col gap-2">
        <ActivityButton
          expanded={activityBarExpanded}
          label={activityBarExpanded ? '收起一级菜单' : '展开一级菜单'}
          onClick={onToggleActivityBarExpanded}
          className="nova-icon-button"
        >
          <PanelLeft className={`h-4 w-4 transition-transform ${activityBarExpanded ? '' : 'rotate-180'}`} />
        </ActivityButton>
        <ActivityButton
          expanded={activityBarExpanded}
          label="设置"
          onClick={onToggleSettings}
          className={`nova-icon-button ${settingsOpen ? 'is-active' : ''}`}
        >
          <Settings className="h-4 w-4" />
        </ActivityButton>
      </div>
    </aside>
  )

  const statusBar = (
    <div className="nova-statusbar nova-topbar flex h-6 shrink-0 items-center border-t px-3">
      <span>Nova v{appVersion}</span>
      {mode === 'ide' && summary && (
        <span className="ml-4">《{summary.title || '未命名'}》 · {summary.chapter_count} 章 · {formatNumber(summary.total_words)} 字</span>
      )}
      {mode === 'ide' && currentChapter && (
        <span className="ml-4">当前：{currentChapter.display_title} · {formatNumber(currentChapter.words)} 字 · {currentChapter.status}</span>
      )}
      <span className="ml-auto">{isStreaming ? '生成中' : '空闲'} · DeepSeek</span>
    </div>
  )

  return (
    <WorkspaceLayout
      topBar={topBar}
      activityBar={activityBar}
      sidebar={sidebar}
      sidebarVisible={mode === 'ide' && projectVisible && !fullWorkspacePanelVisible}
      main={main}
      rightPanel={mode === 'ide' && !fullWorkspacePanelVisible ? rightPanelContent : null}
      rightPanelVisible={mode === 'ide' && !fullWorkspacePanelVisible && Boolean(rightPanelContent)}
      statusBar={statusBar}
    />
  )
}

function ActivityButton({
  expanded,
  label,
  children,
  className,
  ...props
}: React.ComponentProps<'button'> & {
  expanded: boolean
  label: string
  children: ReactNode
}) {
  return (
    <TooltipIconButton
      label={label}
      className={`${className || ''} ${expanded ? 'gap-3 px-3' : ''}`}
      {...props}
    >
      {children}
      {expanded && <span className="min-w-0 truncate text-xs font-medium">{label}</span>}
    </TooltipIconButton>
  )
}
