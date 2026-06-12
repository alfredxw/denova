import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { BookMarked, BookOpen, Bot, Clock3, Database, History, MessageSquareText, PanelLeft, PenLine, Settings, SlidersHorizontal, Sparkles } from 'lucide-react'
import { AnimatePresence, LayoutGroup, motion } from 'motion/react'
import { WorkspaceLayout } from '@/components/layout/workspace-layout'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { novaSpring } from '@/features/motion/motion-tokens'
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
  const { t } = useTranslation()
  const loreVisible = rightPanel === 'lore'
  const creatorVisible = rightPanel === 'creator'
  const tellerVisible = rightPanel === 'teller'
  const versionsVisible = rightPanel === 'versions'
  const sharedMenuActive = settingsOpen || mode === 'books' || mode === 'skills' || mode === 'agents' || mode === 'automations'
  const ideModeActive = mode === 'ide' && !sharedMenuActive
  const interactiveModeActive = mode === 'interactive' && !sharedMenuActive
  const skillsActive = mode === 'skills' && !settingsOpen
  const agentsActive = mode === 'agents' && !settingsOpen
  const automationsActive = mode === 'automations' && !settingsOpen
  const fullWorkspacePanelVisible = settingsOpen || mode === 'skills' || mode === 'agents' || mode === 'automations' || (mode === 'ide' && (loreVisible || creatorVisible || tellerVisible || versionsVisible))
  const modeLabel = settingsOpen ? t('workbench.mode.settings') : mode === 'interactive' ? t('workbench.mode.interactive') : mode === 'books' ? t('workbench.mode.books') : mode === 'skills' ? t('workbench.mode.skills') : mode === 'agents' ? t('workbench.mode.agents') : mode === 'automations' ? t('workbench.mode.automations') : t('workbench.mode.ide')
  const navigationMode = mode === 'books' || mode === 'skills' || mode === 'agents' || mode === 'automations' ? booksReturnMode : mode

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

  const openSkills = () => {
    if (mode === 'skills' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    onSetMode('skills')
  }

  const openAutomations = () => {
    if (mode === 'automations' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    onSetMode('automations')
  }

  const topBar = (
    <header className="nova-topbar grid h-10 shrink-0 grid-cols-[auto_1fr_auto] items-center border-b px-3 text-xs">
      <div className="flex items-center gap-3">
        <div className="font-semibold text-[var(--nova-text)]">Nova</div>
        <LayoutGroup id="workbench-mode-switch">
        <div className="flex h-7 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('workbench.modeSwitch')}>
          <button
            type="button"
            onClick={() => onSetMode('ide')}
            className={`relative overflow-hidden rounded-[6px] px-2.5 py-0.5 text-[11px] transition-colors ${navigationMode === 'ide' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            {navigationMode === 'ide' && <motion.span layoutId="workbench-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
            <span className="relative z-10">{t('workbench.mode.ideButton')}</span>
          </button>
          <button
            type="button"
            onClick={() => onSetMode('interactive')}
            className={`relative overflow-hidden rounded-[6px] px-2.5 py-0.5 text-[11px] transition-colors ${navigationMode === 'interactive' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            {navigationMode === 'interactive' && <motion.span layoutId="workbench-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
            <span className="relative z-10">{t('workbench.mode.interactiveButton')}</span>
          </button>
        </div>
        </LayoutGroup>
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
        label={t('workbench.activity.lore')}
        onClick={() => toggleIdePanel('lore')}
        active={ideModeActive && loreVisible}
        className="nova-icon-button mb-2"
      >
        <Database className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.creator')}
        onClick={() => toggleIdePanel('creator')}
        active={ideModeActive && creatorVisible}
        className="nova-icon-button mb-2"
      >
        <BookMarked className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.teller')}
        onClick={() => toggleIdePanel('teller')}
        active={ideModeActive && tellerVisible}
        className="nova-icon-button mb-2"
      >
        <SlidersHorizontal className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.versions')}
        onClick={() => toggleIdePanel('versions')}
        active={ideModeActive && versionsVisible}
        className="nova-icon-button mb-2"
      >
        <History className="h-4 w-4" />
      </ActivityButton>
    </>
  )

  const interactiveActivityButtons = (
    <>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.story')}
        onClick={() => openInteractiveSubmode('story')}
        active={interactiveModeActive && interactiveSubmode === 'story'}
        className="nova-icon-button mb-2"
      >
        <MessageSquareText className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.timeline')}
        onClick={() => openInteractiveSubmode('timeline')}
        active={interactiveModeActive && interactiveSubmode === 'timeline'}
        className="nova-icon-button mb-2"
      >
        <History className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.lore')}
        onClick={() => openInteractiveSubmode('lore')}
        active={interactiveModeActive && interactiveSubmode === 'lore'}
        className="nova-icon-button mb-2"
      >
        <Database className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.creator')}
        onClick={() => openInteractiveSubmode('creator')}
        active={interactiveModeActive && interactiveSubmode === 'creator'}
        className="nova-icon-button mb-2"
      >
        <BookMarked className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.teller')}
        onClick={() => openInteractiveSubmode('teller')}
        active={interactiveModeActive && interactiveSubmode === 'teller'}
        className="nova-icon-button mb-2"
      >
        <SlidersHorizontal className="h-4 w-4" />
      </ActivityButton>
    </>
  )

  const activityBar = (
    <LayoutGroup id="workbench-activity-bar">
    <aside className={`nova-activity-bar flex shrink-0 flex-col gap-2 border-r p-3 transition-[width] duration-500 ease-[var(--nova-ease)] ${activityBarExpanded ? 'is-expanded w-48 items-stretch' : 'w-16 items-center'}`}>
      {navigationMode === 'interactive' ? interactiveActivityButtons : (
        <>
          <ActivityButton
            expanded={activityBarExpanded}
            label={t('workbench.activity.writing')}
            onClick={openWriting}
            active={ideModeActive && !loreVisible && !creatorVisible && !tellerVisible && !versionsVisible}
            className="nova-icon-button mb-2"
          >
            <PenLine className="h-4 w-4" />
          </ActivityButton>
          {ideActivityButtons}
        </>
      )}
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.books')}
        onClick={openBooks}
        active={mode === 'books' && !settingsOpen}
        className="nova-icon-button mb-2"
      >
        <BookOpen className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.skills')}
        onClick={openSkills}
        active={skillsActive}
        className="nova-icon-button mb-2"
      >
        <Sparkles className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.agents')}
        onClick={openAgents}
        active={agentsActive}
        className="nova-icon-button mb-2"
      >
        <Bot className="h-4 w-4" />
      </ActivityButton>
      <ActivityButton
        expanded={activityBarExpanded}
        label={t('workbench.activity.automations')}
        onClick={openAutomations}
        active={automationsActive}
        className="nova-icon-button mb-2"
      >
        <Clock3 className="h-4 w-4" />
      </ActivityButton>
      <div className="mt-auto flex flex-col gap-2">
        <ActivityButton
          expanded={activityBarExpanded}
          label={activityBarExpanded ? t('workbench.activity.toggleCollapse') : t('workbench.activity.toggleExpand')}
          onClick={onToggleActivityBarExpanded}
          className="nova-icon-button"
        >
          <PanelLeft className={`h-4 w-4 transition-transform ${activityBarExpanded ? '' : 'rotate-180'}`} />
        </ActivityButton>
        <ActivityButton
          expanded={activityBarExpanded}
          label={t('workbench.activity.settings')}
          onClick={onToggleSettings}
          active={settingsOpen}
          className="nova-icon-button"
        >
          <Settings className="h-4 w-4" />
        </ActivityButton>
      </div>
    </aside>
    </LayoutGroup>
  )

  const statusBar = (
    <div className="nova-statusbar nova-topbar flex h-6 shrink-0 items-center border-t px-3">
      <span>Nova v{appVersion}</span>
      {mode === 'ide' && summary && (
        <span className="ml-4">{t('workbench.status.summary', { title: summary.title || t('workbench.untitled'), chapters: formatNumber(summary.chapter_count), words: formatNumber(summary.total_words) })}</span>
      )}
      {mode === 'ide' && currentChapter && (
        <span className="ml-4">{t('workbench.status.currentChapter', { title: currentChapter.display_title, words: formatNumber(currentChapter.words), status: currentChapter.status })}</span>
      )}
      <span className="ml-auto">{isStreaming ? t('workbench.status.streaming') : t('workbench.status.idle')} · DeepSeek</span>
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
  active = false,
  ...props
}: React.ComponentProps<'button'> & {
  expanded: boolean
  label: string
  children: ReactNode
  active?: boolean
}) {
  return (
    <TooltipIconButton
      label={label}
      className={`${className || ''} relative overflow-hidden ${expanded ? 'gap-3 px-3' : ''} ${active ? 'is-active' : ''}`}
      {...props}
    >
      {active && <motion.span layoutId="workbench-activity-active" className="absolute inset-0 rounded-[var(--nova-radius)] bg-[var(--nova-active)]" transition={novaSpring} />}
      <span className="relative z-10 flex shrink-0 items-center justify-center">{children}</span>
      <AnimatePresence initial={false}>
        {expanded && (
          <motion.span
            key="label"
            initial={{ opacity: 0, x: -4 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: -4 }}
            transition={{ duration: 0.16 }}
            className="relative z-10 min-w-0 truncate text-xs font-medium"
          >
            {label}
          </motion.span>
        )}
      </AnimatePresence>
    </TooltipIconButton>
  )
}
