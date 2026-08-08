import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Group, Panel, Separator } from 'react-resizable-panels'
import { BookOpen, Bot, Clock3, Database, History, MessageSquareText, PanelLeft, PenLine, Search, Settings, SlidersHorizontal, Sparkles, Terminal } from 'lucide-react'
import { AnimatePresence, LayoutGroup, motion } from 'motion/react'
import { WorkspaceLayout } from '@/components/layout/workspace-layout'
import { WorkspaceMobileLayout, type MobileNavItem } from '@/components/layout/workspace-mobile-layout'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { novaEase, novaSpring } from '@/features/motion/motion-tokens'
import { MessageCenterButton } from '@/features/messages/MessageCenter'
import type { AutomationMessageNavigation } from '@/features/messages/types'
import { requestAutomationNavigation } from '@/features/automations/automation-navigation'
import { setActivityMessageUnreadCount, useActivitySummary } from '@/features/activity/use-activity-summary'
import { useIsMobile } from '@/hooks/useIsMobile'
import type { BookRecord, ChapterSummary, WorkspaceSummary } from '@/lib/api'
import { isSharedWorkspaceMode, useWorkspaceStore, type RightPanel, type WorkspaceMode } from '@/stores/workspace-store'
import type { InteractiveSubmode } from '@/features/interactive/types'
import { formatNumber } from './workbench-utils'
import { formatDateTime } from '@/i18n'
import { BookSwitcher } from './BookSwitcher'
import { WorkbenchNoticePill } from './WorkbenchNoticePill'
import type { WorkbenchNotice } from '@/features/notices/use-workbench-notice'

export type WorkbenchPresentedLayout = 'writing' | 'interactive' | 'full'

interface WorkbenchShellProps {
  mode: WorkspaceMode
  presentedLayout: WorkbenchPresentedLayout
  booksReturnMode: 'ide' | 'interactive'
  currentBookName: string
  workspace: string
  books: BookRecord[]
  appVersion: string
  summary: WorkspaceSummary | null
  currentChapter?: ChapterSummary
  editorLine?: number
  isStreaming: boolean
  projectVisible: boolean
  activityBarExpanded: boolean
  rightPanel: RightPanel
  settingsOpen: boolean
  interactiveSubmode: InteractiveSubmode
  sidebar: ReactNode
  main: ReactNode
  rightPanelContent: ReactNode
  rightPanelWide?: boolean
  centerFocus?: boolean
  notice?: WorkbenchNotice | null
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onSetInteractiveSubmode: (mode: InteractiveSubmode) => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleSettings: () => void
  onCloseSettings: () => void
  onQuickSwitchBook: (path: string) => Promise<boolean>
  onDismissNotice?: () => void
}

type ActivityItemId = 'writing' | 'story' | 'lore' | 'teller' | 'versions' | 'books' | 'agentchat' | 'skills' | 'agents' | 'automations'
type PrimaryNavigationId = ActivityItemId | 'settings'
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
const DEFAULT_IDE_ACTIVITY_ORDER: ActivityItemId[] = ['writing', 'agentchat', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']
const DEFAULT_INTERACTIVE_ACTIVITY_ORDER: ActivityItemId[] = ['story', 'agentchat', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']
// User-level width preference; it should survive reloads and development hot updates.
const ACTIVITY_BAR_WIDTH_STORAGE_KEY = 'nova.layout.activityBarWidth'
const ACTIVITY_BAR_COLLAPSED_WIDTH = 64
const ACTIVITY_BAR_MIN_WIDTH = 112
const ACTIVITY_BAR_LEGACY_DEFAULT_WIDTH = 152
const ACTIVITY_BAR_DEFAULT_WIDTH = 180
const ACTIVITY_BAR_MAX_WIDTH = 280
const ACTIVITY_BAR_WIDTH_KEYBOARD_STEP = 8
const PRIMARY_NAVIGATION_TRANSITION = { type: 'tween', duration: 0.12, ease: novaEase } as const

function NovaBrandIcon() {
  return (
    <img
      src="/favicon.svg"
      alt="Denova"
      className="h-6 w-6 shrink-0 rounded-[7px]"
      draggable={false}
    />
  )
}

export function WorkbenchShell({
  mode,
  presentedLayout,
  booksReturnMode,
  currentBookName,
  workspace,
  books,
  appVersion,
  summary,
  currentChapter,
  editorLine,
  isStreaming,
  projectVisible,
  activityBarExpanded,
  rightPanel,
  settingsOpen,
  interactiveSubmode,
  sidebar,
  main,
  rightPanelContent,
  rightPanelWide = false,
  centerFocus = false,
  notice,
  onSetMode,
  onToggleActivityBarExpanded,
  onSetInteractiveSubmode,
  onSetRightPanel,
  onToggleSettings,
  onCloseSettings,
  onQuickSwitchBook,
  onDismissNotice,
}: WorkbenchShellProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const setCommandOpen = useWorkspaceStore((state) => state.setCommandOpen)
  const [activityOrders, setActivityOrders] = useState<Record<ActivityOrderScope, ActivityItemId[]>>(readStoredActivityOrders)
  const [activityBarWidth, setActivityBarWidth] = useState(readStoredActivityBarWidth)
  const activitySummary = useActivitySummary().data
  const messageUnread = activitySummary?.message_unread_count ?? 0
  const automationInboxUnread = activitySummary?.automation_inbox_unread_count ?? 0
  const automationRunning = activitySummary?.automation_running_count ?? 0
  const [optimisticNavigationId, setOptimisticNavigationId] = useState<PrimaryNavigationId | null>(null)
  const navigationFrameRef = useRef<number | null>(null)
  const [mainContentHost] = useState(() => {
    const host = createStablePortalHost('h-full min-h-0 w-full min-w-0 overflow-hidden')
    if (host) host.dataset.novaWorkbenchMainHost = 'true'
    return host
  })
  const sensors = useSensors(
    // The complete button remains the drag target. A slightly larger threshold keeps a small
    // pointer wobble from stealing an ordinary menu click without introducing a separate handle.
    useSensor(PointerSensor, { activationConstraint: { distance: 10 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const selectPrimaryNavigation = useCallback((id: PrimaryNavigationId, action: () => void) => {
    setOptimisticNavigationId(id)
    if (navigationFrameRef.current !== null) window.cancelAnimationFrame(navigationFrameRef.current)

    // Let the optimistic selected state reach one paint before the workspace performs the
    // potentially expensive route update. Two frames guarantee a paint boundary in browsers.
    navigationFrameRef.current = window.requestAnimationFrame(() => {
      navigationFrameRef.current = window.requestAnimationFrame(() => {
        navigationFrameRef.current = null
        try {
          action()
        } finally {
          setOptimisticNavigationId(null)
        }
      })
    })
  }, [])

  useEffect(() => () => {
    if (navigationFrameRef.current !== null) window.cancelAnimationFrame(navigationFrameRef.current)
  }, [])

  useEffect(() => {
    cleanupLegacyActivityOrderStorage()
    setActivityOrders(readStoredActivityOrders())
  }, [])

  const loreVisible = rightPanel === 'lore'
  const tellerVisible = rightPanel === 'teller'
  const versionsVisible = rightPanel === 'versions'
  const sharedMenuActive = settingsOpen || versionsVisible || isSharedWorkspaceMode(mode)
  const ideModeActive = mode === 'ide' && !sharedMenuActive
  const interactiveModeActive = mode === 'interactive' && !sharedMenuActive
  const skillsActive = mode === 'skills' && !settingsOpen
  const agentsActive = mode === 'agents' && !settingsOpen
  const automationsActive = mode === 'automations' && !settingsOpen
  const agentChatActive = mode === 'agentchat' && !settingsOpen
  // Navigation state updates optimistically, while heavyweight route content is deferred.
  // Layout must follow the content that is actually painted; mixing it with the newer
  // navigation state briefly squeezes or stretches the outgoing page.
  const writingContentVisible = presentedLayout === 'writing'
  const interactiveContentVisible = presentedLayout === 'interactive'
  const modeLabel = settingsOpen ? t('workbench.mode.settings') : versionsVisible ? t('workbench.activity.versions') : mode === 'interactive' ? t('workbench.mode.interactive') : mode === 'books' ? t('workbench.mode.books') : mode === 'skills' ? t('workbench.mode.skills') : mode === 'agents' ? t('workbench.mode.agents') : mode === 'automations' ? t('workbench.mode.automations') : mode === 'agentchat' ? t('workbench.mode.agentchat') : t('workbench.mode.ide')
  const navigationMode = isSharedWorkspaceMode(mode) ? booksReturnMode : mode
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
    // A workspace panel can briefly retain its previous value while a shared route is being
    // presented. Only treat this as a toggle-off when that panel is actually the active IDE menu.
    const panelAlreadyActive = mode === 'ide' && !settingsOpen && rightPanel === panel
    closeSettingsIfOpen()
    onSetMode('ide')
    onSetRightPanel(panelAlreadyActive ? null : panel)
  }

  const openVersions = () => {
    closeSettingsIfOpen()
    if (isSharedWorkspaceMode(mode)) {
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

  const manageBooks = () => {
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

  const openAgentChat = () => {
    if (mode === 'agentchat' && !settingsOpen) {
      returnFromBooks()
      return
    }
    closeSettingsIfOpen()
    if (versionsVisible) onSetRightPanel(null)
    onSetMode('agentchat')
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

  const openAutomationNotification = (target: AutomationMessageNavigation) => {
    closeSettingsIfOpen()
    if (versionsVisible) onSetRightPanel(null)
    requestAutomationNavigation(target)
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
      label: t('workbench.activity.game'),
      onClick: () => openInteractiveSubmode('story'),
      active: interactiveModeActive && (interactiveSubmode === 'story' || interactiveSubmode === 'director' || interactiveSubmode === 'timeline'),
      icon: <MessageSquareText className="h-4 w-4" />,
    },
    {
      id: 'lore',
      label: t('workbench.activity.lore'),
      onClick: () => openInteractiveSubmode('lore'),
      active: interactiveModeActive && interactiveSubmode === 'lore',
      icon: <Database className="h-4 w-4" />,
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
      id: 'agentchat',
      label: t('workbench.activity.agentchat'),
      onClick: openAgentChat,
      active: agentChatActive,
      icon: <Terminal className="h-4 w-4" />,
    },
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
      icon: <ActivityIconBadge count={automationInboxUnread} running={automationRunning > 0}><Clock3 className="size-3" /></ActivityIconBadge>,
    },
  ]

  const activityItems = useMemo(
    () => sortActivityItems([
      ...(navigationMode === 'interactive' ? interactiveActivityItems : ideActivityItems),
      ...sharedActivityItems,
    ], activityOrder, defaultActivityOrderForScope(activityOrderScope)),
    [activityOrder, activityOrderScope, agentChatActive, agentsActive, automationInboxUnread, automationRunning, automationsActive, booksReturnMode, ideModeActive, interactiveModeActive, interactiveSubmode, loreVisible, mode, navigationMode, settingsOpen, skillsActive, t, tellerVisible, versionsVisible],
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

  const resizeActivityBar = (nextWidth: number) => {
    const clampedWidth = clampActivityBarWidth(nextWidth)
    setActivityBarWidth(clampedWidth)
    storeActivityBarWidth(clampedWidth)
  }

  const handleActivityBarResizePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!activityBarExpanded) return
    event.preventDefault()
    const startX = event.clientX
    const startWidth = activityBarWidth
    const handlePointerMove = (moveEvent: PointerEvent) => {
      resizeActivityBar(startWidth + moveEvent.clientX - startX)
    }
    const handlePointerUp = () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', handlePointerUp)
    }
    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', handlePointerUp, { once: true })
  }

  const handleActivityBarResizeKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (!activityBarExpanded) return
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      resizeActivityBar(activityBarWidth - ACTIVITY_BAR_WIDTH_KEYBOARD_STEP)
    } else if (event.key === 'ArrowRight') {
      event.preventDefault()
      resizeActivityBar(activityBarWidth + ACTIVITY_BAR_WIDTH_KEYBOARD_STEP)
    } else if (event.key === 'Home') {
      event.preventDefault()
      resizeActivityBar(ACTIVITY_BAR_MIN_WIDTH)
    } else if (event.key === 'End') {
      event.preventDefault()
      resizeActivityBar(ACTIVITY_BAR_MAX_WIDTH)
    }
  }

  const topBar = (
    <header className="nova-topbar grid h-10 shrink-0 grid-cols-[minmax(0,1fr)_auto] items-center border-b px-3 text-xs">
      <div className="flex min-w-0 items-center gap-2">
        <NovaBrandIcon />
        <LayoutGroup id="workbench-mode-switch">
        <div role="group" className="flex h-7 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('workbench.modeSwitch')}>
          <button
            type="button"
            aria-pressed={navigationMode === 'ide'}
            onClick={() => switchNavigationMode('ide')}
            data-onboarding-anchor="mode-ide"
            className={`relative overflow-hidden rounded-[6px] px-2.5 py-0.5 text-[11px] transition-colors ${navigationMode === 'ide' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            {navigationMode === 'ide' && <motion.span layoutId="workbench-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
            <span className="relative z-10">{t('workbench.mode.ideButton')}</span>
          </button>
          <button
            type="button"
            aria-pressed={navigationMode === 'interactive'}
            onClick={() => switchNavigationMode('interactive')}
            data-onboarding-anchor="mode-interactive"
            className={`relative overflow-hidden rounded-[6px] px-2.5 py-0.5 text-[11px] transition-colors ${navigationMode === 'interactive' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
          >
            {navigationMode === 'interactive' && <motion.span layoutId="workbench-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
            <span className="relative z-10">{t('workbench.mode.interactiveButton')}</span>
          </button>
        </div>
        </LayoutGroup>
        <BookSwitcher
          books={books}
          currentBookName={currentBookName}
          currentChapterCount={summary?.chapter_count}
          workspace={workspace}
          onSwitchBook={onQuickSwitchBook}
          onManageBooks={manageBooks}
        />
      </div>
      <div className="nova-ui-compact flex items-center justify-end gap-2 text-[var(--nova-text-faint)]">
        <MessageCenterButton className="!size-7 !min-w-7" unreadCount={messageUnread} onUnreadCountChange={setActivityMessageUnreadCount} onOpenAutomation={openAutomationNotification} />
        <span>{modeLabel}</span>
      </div>
    </header>
  )

  const activityBar = (
    <LayoutGroup id="workbench-activity-bar">
    <DndContext key={activityOrderScope} sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleActivityDragEnd}>
    <aside
      className={`nova-activity-bar relative flex shrink-0 flex-col gap-2 border-r p-3 transition-[width] duration-500 ease-[var(--nova-ease)] ${activityBarExpanded ? 'is-expanded items-stretch' : 'items-center'}`}
      style={{ width: activityBarExpanded ? activityBarWidth : ACTIVITY_BAR_COLLAPSED_WIDTH }}
    >
      <SortableContext key={activityOrderScope} items={activityItems.map((item) => toSortableActivityId(activityOrderScope, item.id))} strategy={verticalListSortingStrategy}>
        {activityItems.map((item) => (
          <SortableActivityButton
            key={toSortableActivityId(activityOrderScope, item.id)}
            id={toSortableActivityId(activityOrderScope, item.id)}
            activityId={item.id}
            dragDisabled={settingsOpen}
            expanded={activityBarExpanded}
            label={item.label}
            onClick={() => selectPrimaryNavigation(item.id, item.onClick)}
            active={optimisticNavigationId ? optimisticNavigationId === item.id : item.active}
            className="nova-icon-button mb-2"
          >
            {item.icon}
          </SortableActivityButton>
        ))}
      </SortableContext>
      <div className="mt-auto flex flex-col gap-2">
        {notice && (
          <WorkbenchNoticePill
            expanded={activityBarExpanded}
            notice={notice}
            onOpenSettings={onToggleSettings}
            onDismiss={onDismissNotice}
          />
        )}
        <ActivityButton
          expanded={activityBarExpanded}
          label={activityBarExpanded ? t('workbench.activity.toggleCollapse') : t('workbench.activity.toggleExpand')}
          onClick={onToggleActivityBarExpanded}
          className="nova-icon-button"
          showTooltip={false}
        >
          <PanelLeft className={`h-4 w-4 transition-transform ${activityBarExpanded ? '' : 'rotate-180'}`} />
        </ActivityButton>
        <ActivityButton
          expanded={activityBarExpanded}
          label={t('workbench.activity.settings')}
          onClick={() => selectPrimaryNavigation('settings', onToggleSettings)}
          active={optimisticNavigationId ? optimisticNavigationId === 'settings' : settingsOpen}
          data-onboarding-anchor="activity-settings"
          className="nova-icon-button"
        >
          <Settings className="h-4 w-4" />
        </ActivityButton>
      </div>
      {activityBarExpanded && (
        <div
          role="separator"
          tabIndex={0}
          aria-label={t('layout.resize.activityBar')}
          aria-orientation="vertical"
          aria-valuemin={ACTIVITY_BAR_MIN_WIDTH}
          aria-valuemax={ACTIVITY_BAR_MAX_WIDTH}
          aria-valuenow={Math.round(activityBarWidth)}
          className="nova-activity-bar-resize-handle"
          onPointerDown={handleActivityBarResizePointerDown}
          onKeyDown={handleActivityBarResizeKeyDown}
        />
      )}
    </aside>
    </DndContext>
    </LayoutGroup>
  )

  const statusBar = (
    <div className="nova-statusbar nova-topbar flex h-6 shrink-0 items-center border-t px-3">
      <span>Denova v{appVersion}</span>
      {writingContentVisible && summary && (
        <span className="ml-4">{t('workbench.status.summary', { title: summary.title || t('workbench.untitled'), chapters: formatNumber(summary.chapter_count), words: formatNumber(summary.total_words) })}</span>
      )}
      {writingContentVisible && currentChapter && (
        <span className="ml-4">{t('workbench.status.currentChapter', { title: currentChapter.display_title, words: formatNumber(currentChapter.words), status: currentChapter.status })}</span>
      )}
      {writingContentVisible && currentChapter && (
        <span className="ml-4">
          {t('editor.updatedAt', { time: currentChapter.updated_at ? formatDateTime(currentChapter.updated_at) : t('editor.unknownTime') })}
          {editorLine !== undefined && ` · ${t('editor.currentLine', { line: formatNumber(editorLine) })}`}
        </span>
      )}
      {isStreaming && <span className="ml-auto">{t('workbench.status.streaming')}</span>}
    </div>
  )

  // Keep business content on one React subtree while its DOM host moves between
  // the desktop resizable workspace and the mobile shell. This preserves local
  // editor state when the viewport crosses the mobile breakpoint.
  const mainContentPortal = mainContentHost ? createPortal(main, mainContentHost, 'workbench-main-content') : null
  const mainContentSlot = (
    <StablePortalSlot
      host={mainContentHost}
      fallback={main}
      wrapFallback={false}
      data-nova-workbench-main-slot="true"
      className="h-full min-h-0 w-full min-w-0 overflow-hidden"
    />
  )

  if (isMobile) {
    const compactMobileNavigation = interactiveContentVisible && interactiveSubmode === 'story'
    const mobileTopBar = (
      <header className="nova-mobile-topbar nova-topbar shrink-0 border-b border-[var(--nova-border)] py-2 pl-3 pr-3">
        <div className="flex min-w-0 items-center justify-between gap-2">
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <NovaBrandIcon />
            <BookSwitcher
              books={books}
              currentBookName={currentBookName}
              currentChapterCount={summary?.chapter_count}
              workspace={workspace}
              compact
              onSwitchBook={onQuickSwitchBook}
              onManageBooks={manageBooks}
            />
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <MessageCenterButton className="!size-8 !min-w-8" unreadCount={messageUnread} onUnreadCountChange={setActivityMessageUnreadCount} onOpenAutomation={openAutomationNotification} />
            <button
              type="button"
              onClick={() => setCommandOpen(true)}
              className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              aria-label={t('command.openButton')}
            >
              <Search className="h-4 w-4" />
            </button>
            <LayoutGroup id="workbench-mobile-mode-switch">
            <div role="group" className="flex h-8 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('workbench.modeSwitch')}>
              <button
                type="button"
                aria-pressed={navigationMode === 'ide'}
                onClick={() => switchNavigationMode('ide')}
                data-onboarding-anchor="mode-ide"
                className={`relative min-w-0 overflow-hidden rounded-[6px] px-2 py-1 text-[11px] transition-colors ${navigationMode === 'ide' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
              >
                {navigationMode === 'ide' && <motion.span layoutId="workbench-mobile-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
                <span className="relative z-10">{t('workbench.mode.ideButton')}</span>
              </button>
              <button
                type="button"
                aria-pressed={navigationMode === 'interactive'}
                onClick={() => switchNavigationMode('interactive')}
                data-onboarding-anchor="mode-interactive"
                className={`relative min-w-0 overflow-hidden rounded-[6px] px-2 py-1 text-[11px] transition-colors ${navigationMode === 'interactive' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
              >
                {navigationMode === 'interactive' && <motion.span layoutId="workbench-mobile-mode-active" className="absolute inset-0 rounded-[6px] bg-[var(--nova-active)]" transition={novaSpring} />}
                <span className="relative z-10">{t('workbench.mode.interactiveButton')}</span>
              </button>
            </div>
          </LayoutGroup>
          </div>
        </div>
        {notice && (
          <div className="mt-2 flex justify-end">
            <WorkbenchNoticePill
              expanded
              notice={notice}
              starSecondaryText="description"
              onOpenSettings={onToggleSettings}
              onDismiss={onDismissNotice}
            />
          </div>
        )}
      </header>
    )
    const mobileActivityItems: MobileNavItem[] = [
      ...(navigationMode === 'interactive' ? interactiveActivityItems : ideActivityItems),
      ...sharedActivityItems,
    ]
      .filter((item) => item.id !== 'writing')
      .map((item) => ({
        id: item.id,
        label: item.label,
        icon: item.icon,
        active: optimisticNavigationId ? optimisticNavigationId === item.id : item.active,
        onClick: () => selectPrimaryNavigation(item.id, item.onClick),
      }))
    const mobileProjectDrawer = sidebar ? {
      id: 'project' as const,
      title: t('workbench.mobile.project'),
      icon: <PanelLeft className="h-4 w-4" />,
      side: 'left' as const,
      content: sidebar,
    } : undefined
    // Direction B: editor + Agent in a vertical split (Agent docked at bottom,
    // always visible) instead of Agent hidden in a right drawer. Only when the
    // Agent panel is active and no full-workspace panel covers the screen.
    const mobileAgentDocked = writingContentVisible && Boolean(rightPanelContent)
    const mobileMain = (
      <div className="relative flex h-full min-h-0 flex-col">
        {mobileAgentDocked ? (
          <Group
            orientation="vertical"
            disableCursor
            resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
            className="flex min-h-0 flex-1 flex-col"
          >
            <Panel id="nova-mobile-editor" minSize="30%" className="min-h-0">
              {mainContentSlot}
            </Panel>
            <Separator aria-label={t('layout.resize.bottom')} className="nova-resize-handle h-2.5 shrink-0 cursor-row-resize border-y border-[var(--nova-border)] bg-[var(--nova-surface-2)] transition-colors" />
            <Panel id="nova-mobile-agent" defaultSize="38%" minSize="20%" className="min-h-0">
              {rightPanelContent}
            </Panel>
          </Group>
        ) : mainContentSlot}
        {/* Floating button to reopen the Agent dock when it's hidden */}
        {writingContentVisible && !mobileAgentDocked && (
          <button
            type="button"
            className="absolute bottom-3 right-3 z-30 flex h-12 w-12 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] shadow-lg hover:bg-[var(--nova-hover)]"
            onClick={() => onSetRightPanel('ai')}
            aria-label={t('chat.agent')}
          >
            <Bot className="h-5 w-5" />
          </button>
        )}
      </div>
    )

    return (
      <>
        <WorkspaceMobileLayout
          topBar={mobileTopBar}
          main={mobileMain}
          activityItems={mobileActivityItems}
          projectDrawer={mobileProjectDrawer}
          settingsItem={{
            id: 'settings',
            label: t('workbench.activity.settings'),
            icon: <Settings className="h-4 w-4" />,
            active: optimisticNavigationId ? optimisticNavigationId === 'settings' : settingsOpen,
            onClick: () => selectPrimaryNavigation('settings', onToggleSettings),
          }}
          closeLabel={t('common.close')}
          navigationLabel={t('workbench.mobile.navigation')}
          compactNavigation={compactMobileNavigation}
          compactNavigationLabel={t('workbench.mobile.navigationMenu')}
        />
        {mainContentPortal}
      </>
    )
  }

  return (
    <>
      <WorkspaceLayout
        topBar={topBar}
        activityBar={activityBar}
        sidebar={sidebar}
        sidebarVisible={writingContentVisible && projectVisible}
        main={mainContentSlot}
        rightPanel={rightPanelContent}
        rightPanelVisible={writingContentVisible && Boolean(rightPanelContent)}
        rightPanelWide={rightPanelWide && writingContentVisible && Boolean(rightPanelContent)}
        centerFocus={centerFocus && writingContentVisible}
        statusBar={statusBar}
      />
      {mainContentPortal}
    </>
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
        data-onboarding-anchor={`activity-${activityId}`}
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
  showTooltip,
  ...props
}: React.ComponentProps<'button'> & {
  expanded: boolean
  label: string
  children: ReactNode
  active?: boolean
  showTooltip?: boolean
}) {
  return (
    <TooltipIconButton
      label={label}
      showTooltip={showTooltip ?? !expanded}
      className={`${className || ''} relative overflow-hidden ${expanded ? 'w-full gap-3 px-3' : ''} ${active ? 'is-active' : ''}`}
      {...props}
      aria-current={active ? 'page' : undefined}
    >
      {active && <motion.span layoutId="workbench-activity-active" className="absolute inset-0 rounded-[var(--nova-radius)] bg-[var(--nova-active)]" transition={PRIMARY_NAVIGATION_TRANSITION} />}
      <span className="relative z-10 flex shrink-0 items-center justify-center">{children}</span>
      <AnimatePresence initial={false}>
        {expanded && (
          <motion.span
            key="label"
            initial={{ opacity: 0, x: -4 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: -4 }}
            transition={{ duration: 0.16 }}
            className="relative z-10 min-w-0 truncate text-left text-xs font-medium"
          >
            {label}
          </motion.span>
        )}
      </AnimatePresence>
    </TooltipIconButton>
  )
}

function ActivityIconBadge({ count, running, children }: { count: number; running?: boolean; children: ReactNode }) {
  return (
    <span className="relative inline-flex size-3 items-center justify-center">
      {children}
      {running && <span className="absolute -bottom-1 -left-1 h-2 w-2 rounded-full bg-[var(--nova-success)] ring-2 ring-[var(--nova-surface)]" />}
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
    return insertMissingActivityItems(stored, defaultOrder)
  } catch {
    return defaultOrder
  }
}

/** Preserve the user's relative order while placing newly introduced items by their defaults. */
function insertMissingActivityItems(stored: ActivityItemId[], defaultOrder: ActivityItemId[]) {
  const result = [...stored]
  for (const id of defaultOrder) {
    if (result.includes(id)) continue
    const defaultIndex = defaultOrder.indexOf(id)
    const precedingID = defaultOrder.slice(0, defaultIndex).reverse().find((candidate) => result.includes(candidate))
    if (precedingID) {
      result.splice(result.indexOf(precedingID) + 1, 0, id)
      continue
    }
    const followingID = defaultOrder.slice(defaultIndex + 1).find((candidate) => result.includes(candidate))
    if (followingID) result.splice(result.indexOf(followingID), 0, id)
    else result.push(id)
  }
  return result
}

function storeActivityOrder(scope: ActivityOrderScope, order: ActivityItemId[]) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(ACTIVITY_ORDER_STORAGE_KEYS[scope], JSON.stringify(order))
  cleanupLegacyActivityOrderStorage()
}

export function readStoredActivityBarWidth() {
  if (typeof window === 'undefined') return ACTIVITY_BAR_DEFAULT_WIDTH
  const raw = window.localStorage.getItem(ACTIVITY_BAR_WIDTH_STORAGE_KEY)
  if (raw === null) return ACTIVITY_BAR_DEFAULT_WIDTH
  const value = Number(raw)
  if (value === ACTIVITY_BAR_LEGACY_DEFAULT_WIDTH) return ACTIVITY_BAR_DEFAULT_WIDTH
  return Number.isFinite(value) ? clampActivityBarWidth(value) : ACTIVITY_BAR_DEFAULT_WIDTH
}

function storeActivityBarWidth(width: number) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(ACTIVITY_BAR_WIDTH_STORAGE_KEY, String(clampActivityBarWidth(width)))
}

function clampActivityBarWidth(width: number) {
  return Math.min(ACTIVITY_BAR_MAX_WIDTH, Math.max(ACTIVITY_BAR_MIN_WIDTH, Math.round(width)))
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
