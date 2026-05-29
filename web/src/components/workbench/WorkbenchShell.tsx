import type { ReactNode } from 'react'
import { BookOpen, Bot, Database, FolderTree, GitBranch, MessageSquareText, PenLine, Settings, SlidersHorizontal } from 'lucide-react'
import { WorkspaceLayout } from '@/components/layout/workspace-layout'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import type { ChapterSummary, WorkspaceSummary } from '@/lib/api'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
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
  rightPanel: RightPanel
  bookManagerOpen: boolean
  settingsOpen: boolean
  sidebar: ReactNode
  main: ReactNode
  rightPanelContent: ReactNode
  onSetMode: (mode: WorkspaceMode) => void
  onToggleProjectVisible: () => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleBookManager: () => void
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
  rightPanel,
  bookManagerOpen,
  settingsOpen,
  sidebar,
  main,
  rightPanelContent,
  onSetMode,
  onToggleProjectVisible,
  onSetRightPanel,
  onToggleBookManager,
  onToggleSettings,
}: WorkbenchShellProps) {
  const aiVisible = rightPanel === 'ai'
  const loreVisible = rightPanel === 'lore'
  const tellerVisible = rightPanel === 'teller'
  const versionsVisible = rightPanel === 'versions'
  const fullWorkspacePanelVisible = mode === 'ide' && (loreVisible || tellerVisible)

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
        <span>{mode === 'interactive' ? '互动工作台' : '小说 IDE'}</span>
      </div>
    </header>
  )

  const ideActivityButtons = (
    <>
      <TooltipIconButton
        label="显示/隐藏项目结构"
        onClick={onToggleProjectVisible}
        className={`nova-icon-button mb-2 ${projectVisible ? 'is-active' : ''}`}
      >
        <FolderTree className="h-4 w-4" />
      </TooltipIconButton>
      <TooltipIconButton
        label="显示/隐藏 创作Agent"
        onClick={() => onSetRightPanel(aiVisible ? null : 'ai')}
        className={`nova-icon-button mb-2 ${aiVisible ? 'is-active' : ''}`}
      >
        <Bot className="h-4 w-4" />
      </TooltipIconButton>
      <TooltipIconButton
        label="资料库"
        onClick={() => onSetRightPanel(loreVisible ? null : 'lore')}
        className={`nova-icon-button mb-2 ${loreVisible ? 'is-active' : ''}`}
      >
        <Database className="h-4 w-4" />
      </TooltipIconButton>
      <TooltipIconButton
        label="讲述者"
        onClick={() => onSetRightPanel(tellerVisible ? null : 'teller')}
        className={`nova-icon-button mb-2 ${tellerVisible ? 'is-active' : ''}`}
      >
        <SlidersHorizontal className="h-4 w-4" />
      </TooltipIconButton>
      <TooltipIconButton
        label="版本管理"
        onClick={() => onSetRightPanel(versionsVisible ? null : 'versions')}
        className={`nova-icon-button mb-2 ${versionsVisible ? 'is-active' : ''}`}
      >
        <GitBranch className="h-4 w-4" />
      </TooltipIconButton>
    </>
  )

  const activityBar = (
    <aside className="nova-activity-bar flex w-16 shrink-0 flex-col items-center gap-2 border-r p-3">
      <TooltipIconButton
        label="写作"
        onClick={() => onSetMode('ide')}
        className={`nova-icon-button ${mode === 'ide' ? 'is-active' : ''}`}
      >
        <PenLine className="h-4 w-4" />
      </TooltipIconButton>
      <TooltipIconButton
        label="互动"
        onClick={() => onSetMode('interactive')}
        className={`nova-icon-button ${mode === 'interactive' ? 'is-active' : ''}`}
      >
        <MessageSquareText className="h-4 w-4" />
      </TooltipIconButton>
      <TooltipIconButton
        label="书籍管理"
        onClick={onToggleBookManager}
        className={`nova-icon-button ${bookManagerOpen ? 'is-active' : ''}`}
      >
        <BookOpen className="h-4 w-4" />
      </TooltipIconButton>
      {mode === 'ide' ? ideActivityButtons : null}
      <TooltipIconButton
        label="设置"
        onClick={onToggleSettings}
        className={`nova-icon-button mt-auto ${settingsOpen ? 'is-active' : ''}`}
      >
        <Settings className="h-4 w-4" />
      </TooltipIconButton>
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
