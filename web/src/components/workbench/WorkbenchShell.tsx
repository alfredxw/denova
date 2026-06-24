import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { BookOpen, Bot, Clock3, Database, History, MessageSquareText, NotebookText, PanelLeft, PenLine, Settings, SlidersHorizontal, Sparkles, X } from 'lucide-react'
import { AnimatePresence, LayoutGroup, motion } from 'motion/react'
import { WorkspaceLayout } from '@/components/layout/workspace-layout'
import { WorkspaceMobileLayout, type MobileNavItem } from '@/components/layout/workspace-mobile-layout'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { novaSpring } from '@/features/motion/motion-tokens'
import { useIsMobile } from '@/hooks/useIsMobile'
import { getAutomationInbox, type ChapterSummary, type WorkspaceSummary } from '@/lib/api'
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
  updateNotice?: { latestVersion: string } | null
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onSetInteractiveSubmode: (mode: InteractiveSubmode) => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleSettings: () => void
  onCloseSettings: () => void
  onDismissUpdateNotice?: () => void
}

type ActivityItemId = 'writing' | 'story' | 'timeline' | 'memory' | 'lore' | 'teller' | 'versions' | 'books' | 'skills' | 'agents' | 'automations'
type ActivityOrderScope = 'ide' | 'interactive'
type SortableActivityItemId = `${ActivityOrderScope}:${ActivityItemId}`

interface ActivityItem {
  id: ActivityItemId
  label: string
  onClick: () => void
  active: boolean
  icon: ReactNode
}

const LEGACY_ACTIVITY_ORDER_STORAGE_KEY = 'nova.activity.order.v1'
const LEGACY_SCOPED_ACTIVITY_ORDER_STORAGE_KEYS: Record<ActivityOrderScope, string> = {
  ide: 'nova.activity.order.ide.v1',
  interactive: 'nova.activity.order.interactive.v1',
}
const ACTIVITY_ORDER_STORAGE_KEYS: Record<ActivityOrderScope, string> = {
  ide: 'nova.activity.order.ide.v2',
  interactive: 'nova.activity.order.interactive.v2',
}
const DEFAULT_IDE_ACTIVITY_ORDER: ActivityItemId[] = ['writing', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']
const DEFAULT_INTERACTIVE_ACTIVITY_ORDER: ActivityItemId[] = ['story', 'timeline', 'memory', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']

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
  updateNotice,
  onSetMode,
  onToggleActivityBarExpanded,
  onSetInteractiveSubmode,
  onSetRightPanel,
  onToggleSettings,
  onCloseSettings,
  onDismissUpdateNotice,
}: WorkbenchShellProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const [activityOrders, setActivityOrders] = useState<Record<ActivityOrderScope, ActivityItemId[]>>(readStoredActivityOrders)
  const [automationInboxUnread, setAutomationInboxUnread] = useState(0)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  useEffect(() => {
    cleanupLegacyActivityOrderStorage()
    setActivityOrders(readStoredActivityOrders())
  }, [])

  useEffect(() => {
    let cancelled = false
    async function loadAutomationInboxCount() {
      try {
        const items = await getAutomationInbox()
        if (!cancelled) setAutomationInboxUnread(items.filter((item) => item.status === 'pending' && !item.read_at).length)
      } catch {
        if (!cancelled) setAutomationInboxUnread(0)
      }
    }
    void loadAutomationInboxCount()
    const timer = window.setInterval(loadAutomationInboxCount, 30000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [workspace])

  const loreVisible = rightPanel === 'lore'
  const tellerVisible = rightPanel === 'teller'
  const versionsVisible = rightPanel === 'versions'
  const sharedMenuActive = settingsOpen || versionsVisible || mode === 'books' || mode === 'skills' || mode === 'agents' || mode === 'automations'
  const ideModeActive = mode === 'ide' && !sharedMenuActive
  const interactiveModeActive = mode === 'interactive' && !sharedMenuActive
  const skillsActive = mode === 'skills' && !settingsOpen
  const agentsActive = mode === 'agents' && !settingsOpen
  const automationsActive = mode === 'automations' && !settingsOpen
  const fullWorkspacePanelVisible = settingsOpen || versionsVisible || mode === 'skills' || mode === 'agents' || mode === 'automations' || (mode === 'ide' && (loreVisible || tellerVisible))
  const modeLabel = settingsOpen ? t('workbench.mode.settings') : versionsVisible ? t('workbench.activity.versions') : mode === 'interactive' ? t('workbench.mode.interactive') : mode === 'books' ? t('workbench.mode.books') : mode === 'skills' ? t('workbench.mode.skills') : mode === 'agents' ? t('workbench.mode.agents') : mode === 'automations' ? t('workbench.mode.automations') : t('workbench.mode.ide')
  const navigationMode = mode === 'books' || mode === 'skills' || mode === 'agents' || mode === 'automations' ? booksReturnMode : mode
  const activityOrderScope: ActivityOrderScope = navigationMode === 'interactive' ? 'interactive' : 'ide'
  const activityOrder = activityOrders[activityOrderScope]

  const closeSettingsIfOpen = () => {
    if (settingsOpen) onCloseSettings()
  }

  const openWriting = () => {
    closeSettingsIfOpen()
    onSetMode('ide')
    if (loreVisible || tellerVisible || versionsVisible) onSetRightPanel(null)
  }

  const switchNavigationMode = (nextMode: 'ide' | 'interactive') => {
    closeSettingsIfOpen()
    if (versionsVisible) onSetRightPanel(null)
    onSetMode(nextMode)
  }

  const toggleIdePanel = (panel: NonNullable<RightPanel>) => {
    closeSettingsIfOpen()
    onSetMode('ide')
    onSetRightPanel(rightPanel === panel ? null : panel)
  }

  const openVersions = () => {
    closeSettingsIfOpen()
    if (mode === 'books' || mode === 'skills' || mode === 'agents' || mode === 'automations') {
      onSetMode(booksReturnMode)
    }
    onSetRightPanel(versionsVisible ? null : 'versions')
  }

  const openInteractiveSubmode = (nextMode: InteractiveSubmode) => {
    closeSettingsIfOpen()
    if (versionsVisible) onSetRightPanel(null)
    onSetMode('interactive')
    onSetInteractiveSubmode(nextMode)
  }

  const returnFromBooks = () => {
    if (booksReturnMode === 'interactive') {
      onSetMode('interactive')
      return
    }
    onSetMode('ide')
    if (loreVisible || tellerVisible || versionsVisible) onSetRightPanel(null)
  }

  const openBooks = () => {
    if (mode === 'books' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    if (versionsVisible) onSetRightPanel(null)
    onSetMode('books')
  }

  const openAgents = () => {
    if (mode === 'agents' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    if (versionsVisible) onSetRightPanel(null)
    onSetMode('agents')
  }

  const openSkills = () => {
    if (mode === 'skills' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    if (versionsVisible) onSetRightPanel(null)
    onSetMode('skills')
  }

  const openAutomations = () => {
    if (mode === 'automations' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    if (versionsVisible) onSetRightPanel(null)
    onSetMode('automations')
  }

  const ideActivityItems: ActivityItem[] = [
    {
      id: 'writing',
      label: t('workbench.activity.writing'),
      onClick: openWriting,
      active: ideModeActive && !loreVisible && !tellerVisible,
      icon: <PenLine className="h-4 w-4" />,
    },
    {
      id: 'lore',
      label: t('workbench.activity.lore'),
      onClick: () => toggleIdePanel('lore'),
      active: ideModeActive && loreVisible,
      icon: <Database className="h-4 w-4" />,
    },
    {
      id: 'teller',
      label: t('workbench.activity.teller'),
      onClick: () => toggleIdePanel('teller'),
      active: ideModeActive && tellerVisible,
      icon: <SlidersHorizontal className="h-4 w-4" />,
    },
  ]

  const interactiveActivityItems: ActivityItem[] = [
    {
      id: 'story',
      label: t('workbench.activity.story'),
      onClick: () => openInteractiveSubmode('story'),
      active: interactiveModeActive && interactiveSubmode === 'story',
      icon: <MessageSquareText className="h-4 w-4" />,
    },
    {
      id: 'timeline',
      label: t('workbench.activity.timeline'),
      onClick: () => openInteractiveSubmode('timeline'),
      active: interactiveModeActive && interactiveSubmode === 'timeline',
      icon: <History className="h-4 w-4" />,
    },
    {
      id: 'lore',
      label: t('workbench.activity.lore'),
      onClick: () => openInteractiveSubmode('lore'),
      active: interactiveModeActive && interactiveSubmode === 'lore',
      icon: <Database className="h-4 w-4" />,
    },
    {
      id: 'memory',
      label: t('workbench.activity.memory'),
      onClick: () => openInteractiveSubmode('memory'),
      active: interactiveModeActive && interactiveSubmode === 'memory',
      icon: <NotebookText className="h-4 w-4" />,
    },
    {
      id: 'teller',
      label: t('workbench.activity.teller'),
      onClick: () => openInteractiveSubmode('teller'),
      active: interactiveModeActive && interactiveSubmode === 'teller',
      icon: <SlidersHorizontal className="h-4 w-4" />,
    },
  ]

  const sharedActivityItems: ActivityItem[] = [
    {
      id: 'books',
      label: t('workbench.activity.books'),
      onClick: openBooks,
      active: mode === 'books' && !settingsOpen,
      icon: <BookOpen className="h-4 w-4" />,
    },
    {
      id: 'versions',
      label: t('workbench.activity.versions'),
      onClick: openVersions,
      active: versionsVisible && !settingsOpen,
      icon: <History className="h-4 w-4" />,
    },
    {
      id: 'skills',
      label: t('workbench.activity.skills'),
      onClick: openSkills,
      active: skillsActive,
      icon: <Sparkles className="h-4 w-4" />,
    },
    {
      id: 'agents',
      label: t('workbench.activity.agents'),
      onClick: openAgents,
      active: agentsActive,
      icon: <Bot className="h-4 w-4" />,
    },
    {
      id: 'automations',
      label: t('workbench.activity.automations'),
      onClick: openAutomations,
      active: automationsActive,
      icon: <ActivityIconBadge count={automationInboxUnread}><Clock3 className="size-3" /></ActivityIconBadge>,
    },
  ]

  const activityItems = useMemo(
    () => sortActivityItems([
      ...(navigationMode === 'interactive' ? interactiveActivityItems : ideActivityItems),
      ...sharedActivityItems,
    ], activityOrder, defaultActivityOrderForScope(activityOrderScope)),
    [activityOrder, activityOrderScope, agentsActive, automationInboxUnread, automationsActive, booksReturnMode, ideModeActive, interactiveModeActive, interactiveSubmode, loreVisible, mode, navigationMode, settingsOpen, skillsActive, tellerVisible, versionsVisible],
  )

  const handleActivityDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const activeId = parseSortableActivityId(active.id, activityOrderScope)
    const overId = parseSortableActivityId(over.id, activityOrderScope)
    if (!activeId || !overId) return
    const visibleIds = activityItems.map((item) => item.id)
    const oldIndex = visibleIds.indexOf(activeId)
    const newIndex = visibleIds.indexOf(overId)
    if (oldIndex === -1 || newIndex === -1) return

    const nextVisibleIds = arrayMove(visibleIds, oldIndex, newIndex)
    const nextOrder = mergeVisibleActivityOrder(nextVisibleIds, activityOrder, defaultActivityOrderForScope(activityOrderScope))
    setActivityOrders((current) => ({ ...current, [activityOrderScope]: nextOrder }))
    storeActivityOrder(activityOrderScope, nextOrder)
  }

  const topBar = (
    <header className="nova-topbar grid h-10 shrink-0 grid-cols-[auto_1fr_auto] items-center border-b px-3 text-xs">
      <div className="flex items-center gap-3">
        <div className="font-semibold text-[var(--nova-text)]">Nova</div>
        <LayoutGroup id="workbench-mode-switch">
        <div className="flex h-7 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('workbench.modeSwitch')}>
          <button
            type="button"
            onClick={() => switchNavigationMode('ide')}
            className={`relative overflow-hidden rounded-[6px] px-2.5 py-0.5 text-[11px] transition-colors ${navigationMode === 'ide' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            {navigationMode === 'ide' && <motion.span layoutId="workbench-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
            <span className="relative z-10">{t('workbench.mode.ideButton')}</span>
          </button>
          <button
            type="button"
            onClick={() => switchNavigationMode('interactive')}
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

  const activityBar = (
    <LayoutGroup id="workbench-activity-bar">
    <DndContext key={activityOrderScope} sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleActivityDragEnd}>
    <aside className={`nova-activity-bar flex shrink-0 flex-col gap-2 border-r p-3 transition-[width] duration-500 ease-[var(--nova-ease)] ${activityBarExpanded ? 'is-expanded w-48 items-stretch' : 'w-16 items-center'}`}>
      <SortableContext key={activityOrderScope} items={activityItems.map((item) => toSortableActivityId(activityOrderScope, item.id))} strategy={verticalListSortingStrategy}>
        {activityItems.map((item) => (
          <SortableActivityButton
            key={toSortableActivityId(activityOrderScope, item.id)}
            id={toSortableActivityId(activityOrderScope, item.id)}
            activityId={item.id}
            dragDisabled={settingsOpen}
            expanded={activityBarExpanded}
            label={item.label}
            onClick={item.onClick}
            active={item.active}
            className="nova-icon-button mb-2"
          >
            {item.icon}
          </SortableActivityButton>
        ))}
      </SortableContext>
      <div className="mt-auto flex flex-col gap-2">
        {updateNotice && (
          <UpdateNoticePill
            expanded={activityBarExpanded}
            latestVersion={updateNotice.latestVersion}
            onOpenSettings={onToggleSettings}
            onDismiss={onDismissUpdateNotice}
          />
        )}
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
    </DndContext>
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

  if (isMobile) {
    const compactMobileNavigation = mode === 'interactive' && interactiveSubmode === 'story' && !sharedMenuActive
    const mobileTopBar = (
      <header className="nova-mobile-topbar nova-topbar shrink-0 border-b border-[var(--nova-border)] py-2 pl-3 pr-3" title={workspace || currentBookName}>
        <div className="flex min-w-0 items-center justify-between gap-2">
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <div className="shrink-0 font-semibold text-[var(--nova-text)]">Nova</div>
            <div className="flex min-w-0 items-center gap-1.5 text-[11px] text-[var(--nova-text-faint)]">
              <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
              <span className="min-w-0 truncate font-medium text-[var(--nova-text-muted)]">{currentBookName}</span>
            </div>
          </div>
          <LayoutGroup id="workbench-mobile-mode-switch">
            <div className="flex h-8 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('workbench.modeSwitch')}>
              <button
                type="button"
                onClick={() => switchNavigationMode('ide')}
                className={`relative min-w-0 overflow-hidden rounded-[6px] px-2 py-1 text-[11px] transition-colors ${navigationMode === 'ide' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
              >
                {navigationMode === 'ide' && <motion.span layoutId="workbench-mobile-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
                <span className="relative z-10">{t('workbench.mode.ideButton')}</span>
              </button>
              <button
                type="button"
                onClick={() => switchNavigationMode('interactive')}
                className={`relative min-w-0 overflow-hidden rounded-[6px] px-2 py-1 text-[11px] transition-colors ${navigationMode === 'interactive' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
              >
                {navigationMode === 'interactive' && <motion.span layoutId="workbench-mobile-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
                <span className="relative z-10">{t('workbench.mode.interactiveButton')}</span>
              </button>
            </div>
          </LayoutGroup>
        </div>
        {updateNotice && (
          <div className="mt-2 flex justify-end">
            <UpdateNoticePill
              expanded
              latestVersion={updateNotice.latestVersion}
              onOpenSettings={onToggleSettings}
              onDismiss={onDismissUpdateNotice}
            />
          </div>
        )}
      </header>
    )
    const mobileActivityItems: MobileNavItem[] = activityItems.map((item) => ({
      id: item.id,
      label: item.label,
      icon: item.icon,
      active: item.active,
      onClick: item.onClick,
    }))
    const mobileProjectDrawer = mode === 'ide' && !fullWorkspacePanelVisible && sidebar ? {
      id: 'project' as const,
      title: t('workbench.mobile.project'),
      icon: <PanelLeft className="h-4 w-4" />,
      side: 'left' as const,
      content: sidebar,
    } : undefined
    const mobileAgentDrawer = mode === 'ide' && !fullWorkspacePanelVisible ? {
      id: 'agent' as const,
      title: t('workbench.mobile.agent'),
      icon: <Bot className="h-4 w-4" />,
      side: 'right' as const,
      content: rightPanelContent,
      onOpen: () => onSetRightPanel('ai'),
      onClose: () => {
        if (rightPanel === 'ai') onSetRightPanel(null)
      },
    } : undefined

    return (
      <WorkspaceMobileLayout
        topBar={mobileTopBar}
        main={main}
        activityItems={mobileActivityItems}
        projectDrawer={mobileProjectDrawer}
        agentDrawer={mobileAgentDrawer}
        settingsItem={{
          id: 'settings',
          label: t('workbench.activity.settings'),
          icon: <Settings className="h-4 w-4" />,
          active: settingsOpen,
          onClick: onToggleSettings,
        }}
        closeLabel={t('common.close')}
        navigationLabel={t('workbench.mobile.navigation')}
        compactNavigation={compactMobileNavigation}
        compactNavigationLabel={t('workbench.mobile.navigationMenu')}
      />
    )
  }

  return (
    <WorkspaceLayout
      topBar={topBar}
      activityBar={activityBar}
      sidebar={sidebar}
      sidebarVisible={mode === 'ide' && projectVisible && !fullWorkspacePanelVisible}
      main={main}
      rightPanel={rightPanelContent}
      rightPanelVisible={mode === 'ide' && !fullWorkspacePanelVisible && Boolean(rightPanelContent)}
      statusBar={statusBar}
    />
  )
}

function SortableActivityButton({
  id,
  activityId,
  dragDisabled,
  ...props
}: Omit<React.ComponentProps<'button'>, 'id'> & {
  id: SortableActivityItemId
  activityId: ActivityItemId
  dragDisabled?: boolean
  expanded: boolean
  label: string
  children: ReactNode
  active?: boolean
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id, disabled: dragDisabled })
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <div ref={setNodeRef} style={style} className={isDragging ? 'relative z-20 opacity-80' : undefined}>
      <ActivityButton
        data-activity-id={activityId}
        {...(dragDisabled ? {} : attributes)}
        {...(dragDisabled ? {} : listeners)}
        {...props}
        className={props.className}
      />
    </div>
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
      showTooltip={!expanded}
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

function UpdateNoticePill({
  expanded,
  latestVersion,
  onOpenSettings,
  onDismiss,
}: {
  expanded: boolean
  latestVersion: string
  onOpenSettings: () => void
  onDismiss?: () => void
}) {
  const { t } = useTranslation()
  return (
    <motion.div
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 4 }}
      transition={{ duration: 0.16 }}
      className={`relative z-20 flex items-center rounded-[var(--nova-radius)] border border-[var(--nova-accent)] bg-[var(--nova-surface)]/95 text-[11px] text-[var(--nova-text)] shadow-[var(--nova-shadow)] backdrop-blur ${expanded ? 'w-full' : 'w-44 -translate-x-1'}`}
    >
      <button
        type="button"
        className="min-w-0 flex-1 truncate px-2 py-1.5 text-left"
        title={t('workbench.updateNotice.available', { version: latestVersion })}
        onClick={onOpenSettings}
      >
        {t('workbench.updateNotice.available', { version: latestVersion })}
      </button>
      <button
        type="button"
        className="mr-1 flex h-5 w-5 shrink-0 items-center justify-center rounded-[6px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
        aria-label={t('workbench.updateNotice.dismiss')}
        title={t('workbench.updateNotice.dismiss')}
        onClick={onDismiss}
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </motion.div>
  )
}

function ActivityIconBadge({ count, children }: { count: number; children: ReactNode }) {
  return (
    <span className="relative inline-flex size-3 items-center justify-center">
      {children}
      {count > 0 && (
        <span className="absolute -right-1.5 -top-1.5 min-w-3 rounded-full bg-[var(--nova-danger-border)] px-0.5 text-center text-[8px] leading-3 text-white">
          {count > 9 ? '9+' : count}
        </span>
      )}
    </span>
  )
}

function sortActivityItems(items: ActivityItem[], order: ActivityItemId[], defaultOrder: ActivityItemId[]) {
  const orderIndex = new Map<ActivityItemId, number>()
  order.forEach((id, index) => orderIndex.set(id, index))
  const defaultIndex = new Map<ActivityItemId, number>()
  defaultOrder.forEach((id, index) => defaultIndex.set(id, index))
  return [...items].sort((a, b) => {
    const aIndex = orderIndex.get(a.id) ?? defaultOrder.length + (defaultIndex.get(a.id) ?? 0)
    const bIndex = orderIndex.get(b.id) ?? defaultOrder.length + (defaultIndex.get(b.id) ?? 0)
    return aIndex - bIndex
  })
}

function mergeVisibleActivityOrder(visibleIds: ActivityItemId[], currentOrder: ActivityItemId[], defaultOrder: ActivityItemId[]) {
  const visibleSet = new Set(visibleIds)
  const hiddenIds = currentOrder.filter((id) => !visibleSet.has(id))
  const knownIds = new Set([...visibleIds, ...hiddenIds])
  const missingIds = defaultOrder.filter((id) => !knownIds.has(id))
  return [...visibleIds, ...hiddenIds, ...missingIds]
}

function defaultActivityOrderForScope(scope: ActivityOrderScope) {
  return scope === 'interactive' ? DEFAULT_INTERACTIVE_ACTIVITY_ORDER : DEFAULT_IDE_ACTIVITY_ORDER
}

function readStoredActivityOrders(): Record<ActivityOrderScope, ActivityItemId[]> {
  return {
    ide: readStoredActivityOrder('ide'),
    interactive: readStoredActivityOrder('interactive'),
  }
}

function readStoredActivityOrder(scope: ActivityOrderScope): ActivityItemId[] {
  const defaultOrder = defaultActivityOrderForScope(scope)
  if (typeof window === 'undefined') return defaultOrder
  try {
    const raw = window.localStorage.getItem(ACTIVITY_ORDER_STORAGE_KEYS[scope])
    if (!raw) return defaultOrder
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return defaultOrder
    const validIds = new Set(defaultOrder)
    const stored = parsed.filter((id): id is ActivityItemId => validIds.has(id))
    const storedSet = new Set(stored)
    return [...stored, ...defaultOrder.filter((id) => !storedSet.has(id))]
  } catch {
    return defaultOrder
  }
}

function storeActivityOrder(scope: ActivityOrderScope, order: ActivityItemId[]) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(ACTIVITY_ORDER_STORAGE_KEYS[scope], JSON.stringify(order))
  cleanupLegacyActivityOrderStorage()
}

function cleanupLegacyActivityOrderStorage() {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem(LEGACY_ACTIVITY_ORDER_STORAGE_KEY)
  window.localStorage.removeItem(LEGACY_SCOPED_ACTIVITY_ORDER_STORAGE_KEYS.ide)
  window.localStorage.removeItem(LEGACY_SCOPED_ACTIVITY_ORDER_STORAGE_KEYS.interactive)
}

function toSortableActivityId(scope: ActivityOrderScope, id: ActivityItemId): SortableActivityItemId {
  return `${scope}:${id}`
}

function parseSortableActivityId(value: unknown, scope: ActivityOrderScope): ActivityItemId | null {
  if (typeof value !== 'string') return null
  const prefix = `${scope}:`
  if (!value.startsWith(prefix)) return null
  const id = value.slice(prefix.length)
  return isActivityItemId(id) ? id : null
}

function isActivityItemId(value: string): value is ActivityItemId {
  return DEFAULT_IDE_ACTIVITY_ORDER.includes(value as ActivityItemId) || DEFAULT_INTERACTIVE_ACTIVITY_ORDER.includes(value as ActivityItemId)
}
