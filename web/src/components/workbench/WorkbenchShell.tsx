import type { ReactNode } from 'react'
import { BookMarked, BookOpen, Database, GitBranch, MessageSquareText, PanelLeft, PanelRight, PenLine, Settings, SlidersHorizontal } from 'lucide-react'
import { WorkspaceLayout } from '@/components/layout/workspace-layout'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import type { ChapterSummary, WorkspaceSummary } from '@/lib/api'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import type { InteractiveSubmode } from '@/features/interactive/types'
import { formatNumber } from './workbench-utils'

interface WorkbenchShellProps {
  mode: WorkspaceMode
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
  interactiveRightPanelVisible: boolean
  sidebar: ReactNode
  main: ReactNode
  rightPanelContent: ReactNode
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onSetInteractiveSubmode: (mode: InteractiveSubmode) => void
  onToggleInteractiveRightPanel: () => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleSettings: () => void
}

export function WorkbenchShell({
  mode,
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
  interactiveRightPanelVisible,
  sidebar,
  main,
  rightPanelContent,
  onSetMode,
  onToggleActivityBarExpanded,
  onSetInteractiveSubmode,
  onToggleInteractiveRightPanel,
  onSetRightPanel,
  onToggleSettings,
}: WorkbenchShellProps) {
  const loreVisible = rightPanel === 'lore'
  const creatorVisible = rightPanel === 'creator'
  const tellerVisible = rightPanel === 'teller'
  const versionsVisible = rightPanel === 'versions'
  const fullWorkspacePanelVisible = mode === 'ide' && (loreVisible || creatorVisible || tellerVisible)
  const modeLabel = mode === 'interactive' ? '互动工作台' : mode === 'books' ? '书籍管理' : '小说 IDE'

  const topBar = (
    <header className="nova-topbar grid h-10 shrink-0 grid-cols-[auto_1fr_auto] items-center border-b px-3 text-xs">
      <div className="flex items-center gap-3">
        <div className="font-semibold text-[var(--nova-text)]">Nova</div>
      </div>
      <div className="mx-auto flex min-w-0 max-w-[520px] items-center justify-center gap-1.5" title={workspace || currentBookName}>
        <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
        <span className="truncate font-medium text-[var(--nova-text)]">{currentBookName}</span>
      </div>
      <div className="flex items-center justify-end gap-2 text-[11px] text-[var(--nova-text-faint)]">
        <span>{modeLabel}</span>
      </div>
    </header>
  )

  const ideActivityButtons = (
    <>
      <ActivityButton
        expanded={activityBarExpanded}
        label="资料库"
        onClick={() => onSetRightPanel(loreVisible ? null : 'lore')}
        className={`nova-icon-button mb-2 ${loreVisible ? 'is-active' : ''}`}
      >
        <Database className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="创作者"
        onClick={() => onSetRightPanel(creatorVisible ? null : 'creator')}
        className={`nova-icon-button mb-2 ${creatorVisible ? 'is-active' : ''}`}
      >
        <BookMarked className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="讲述者"
        onClick={() => onSetRightPanel(tellerVisible ? null : 'teller')}
        className={`nova-icon-button mb-2 ${tellerVisible ? 'is-active' : ''}`}
      >
        <SlidersHorizontal className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="版本管理"
        onClick={() => onSetRightPanel(versionsVisible ? null : 'versions')}
        className={`nova-icon-button mb-2 ${versionsVisible ? 'is-active' : ''}`}
      >
        <GitBranch className="h-4 w-4" />
      </ActivityButton>
    </>
  )

  const interactiveActivityButtons = (
    <>
      <ActivityButton
        expanded={activityBarExpanded}
        label="剧情"
        onClick={() => onSetInteractiveSubmode('story')}
        className={`nova-icon-button mb-2 ${interactiveSubmode === 'story' ? 'is-active' : ''}`}
      >
        <MessageSquareText className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="剧情路线图"
        onClick={() => onSetInteractiveSubmode('timeline')}
        className={`nova-icon-button mb-2 ${interactiveSubmode === 'timeline' ? 'is-active' : ''}`}
      >
        <GitBranch className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="资料库"
        onClick={() => onSetInteractiveSubmode('lore')}
        className={`nova-icon-button mb-2 ${interactiveSubmode === 'lore' ? 'is-active' : ''}`}
      >
        <Database className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="创作者"
        onClick={() => onSetInteractiveSubmode('creator')}
        className={`nova-icon-button mb-2 ${interactiveSubmode === 'creator' ? 'is-active' : ''}`}
      >
        <BookMarked className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="讲述者"
        onClick={() => onSetInteractiveSubmode('teller')}
        className={`nova-icon-button mb-2 ${interactiveSubmode === 'teller' ? 'is-active' : ''}`}
      >
        <SlidersHorizontal className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={interactiveRightPanelVisible ? '隐藏场景记忆' : '显示场景记忆'}
        onClick={onToggleInteractiveRightPanel}
        className={`nova-icon-button mb-2 ${interactiveRightPanelVisible ? 'is-active' : ''}`}
      >
        <PanelRight className="h-4 w-4" />
      </ActivityButton>
    </>
  )

  const activityBar = (
    <aside className={`nova-activity-bar flex shrink-0 flex-col gap-2 border-r p-3 transition-[width] duration-500 ease-[var(--nova-ease)] ${activityBarExpanded ? 'is-expanded w-48 items-stretch' : 'w-16 items-center'}`}>
      <ActivityButton
        expanded={activityBarExpanded}
        label="写作"
        onClick={() => onSetMode('ide')}
        className={`nova-icon-button ${mode === 'ide' ? 'is-active' : ''}`}
      >
        <PenLine className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="互动"
        onClick={() => onSetMode('interactive')}
        className={`nova-icon-button ${mode === 'interactive' ? 'is-active' : ''}`}
      >
        <MessageSquareText className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label="书籍管理"
        onClick={() => onSetMode('books')}
        className={`nova-icon-button ${mode === 'books' ? 'is-active' : ''}`}
      >
        <BookOpen className="h-4 w-4" />
      </ActivityButton>
      {mode === 'ide' ? ideActivityButtons : null}
      {mode === 'interactive' ? interactiveActivityButtons : null}
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
    <div className="nova-topbar flex h-6 shrink-0 items-center border-t px-3 text-[11px]">
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
