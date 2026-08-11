import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { BookOpen, Bot, Clock3, Database, FileText, History, MessageSquareText, MoreHorizontal, PanelLeft, PenLine, Settings, SlidersHorizontal, Sparkles } from 'lucide-react'
import { AnimatePresence, LayoutGroup, motion } from 'motion/react'
import { WorkspaceLayout } from '@/components/layout/workspace-layout'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { novaSpring } from '@/features/motion/motion-tokens'
import { MessageCenterButton } from '@/features/messages/MessageCenter'
import type { AutomationMessageNavigation } from '@/features/messages/types'
import { requestAutomationNavigation } from '@/features/automations/automation-navigation'
import { useIsMobile } from '@/hooks/useIsMobile'
import { getTasks, type BookRecord, type ChapterSummary, type TaskCenterResult, type TaskCenterTask, type WorkspaceSummary } from '@/lib/api'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import type { InteractiveSubmode } from '@/features/interactive/types'
import { formatNumber } from './workbench-utils'
import { formatDateTime } from '@/i18n'
import { maybeNotifyActionableTask } from '@/lib/task-notifications'
import { BookSwitcher } from './BookSwitcher'
import { WorkbenchNoticePill } from './WorkbenchNoticePill'
import type { WorkbenchNotice } from '@/features/notices/use-workbench-notice'
import { MobileWorkbenchShell, type MobileWorkbenchDestination } from '@/features/mobile-workbench/MobileWorkbenchShell'
import { UnsavedConfigGuardDialog } from '@/features/config-guard/UnsavedConfigGuardDialog'
import { discardExecutableDraft, hasPendingExecutableDraft } from '@/features/config-guard/executable-draft-guard'
import { MobileMoreMenu, type MobileMoreMenuItem } from '@/features/mobile-workbench/MobileMoreMenu'
import { requestAgentSessionRecovery, requestInteractiveStoryRecovery } from '@/features/mobile-workbench/task-recovery-navigation'
import { requestMobileWorkbenchDestination } from '@/features/mobile-workbench/navigation'

interface WorkbenchShellProps {
  mode: WorkspaceMode
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
  systemNotificationsEnabled?: boolean
}

type ActivityItemId = 'writing' | 'story' | 'timeline' | 'lore' | 'teller' | 'versions' | 'books' | 'skills' | 'agents' | 'automations'
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
const DEFAULT_INTERACTIVE_ACTIVITY_ORDER: ActivityItemId[] = ['story', 'timeline', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']
const ACTIVITY_BAR_WIDTH_STORAGE_KEY = 'nova.layout.activityBarWidth'
const ACTIVITY_BAR_COLLAPSED_WIDTH = 64
const ACTIVITY_BAR_MIN_WIDTH = 112
const ACTIVITY_BAR_LEGACY_DEFAULT_WIDTH = 152
const ACTIVITY_BAR_DEFAULT_WIDTH = 180
const ACTIVITY_BAR_MAX_WIDTH = 280
const ACTIVITY_BAR_WIDTH_KEYBOARD_STEP = 8
const TASK_ACTIVITY_REFRESH_INTERVAL_MS = 30000
const EMPTY_TASK_CENTER: TaskCenterResult = { tasks: [], action_required_count: 0 }

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
  systemNotificationsEnabled = false,
}: WorkbenchShellProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const [activityOrders, setActivityOrders] = useState<Record<ActivityOrderScope, ActivityItemId[]>>(readStoredActivityOrders)
  const [activityBarWidth, setActivityBarWidth] = useState(readStoredActivityBarWidth)
  const [taskActivity, setTaskActivity] = useState<{ result: TaskCenterResult; loadState: 'loading' | 'ready' | 'error' }>({
    result: EMPTY_TASK_CENTER,
    loadState: 'loading',
  })
  const [pendingLeave, setPendingLeave] = useState<{ key: string; proceed: () => void } | null>(null)
  const notifiedTaskIDsRef = useRef(new Set<string>())
  const [mainContentHost] = useState(() => {
    const host = createStablePortalHost('h-full min-h-0 w-full min-w-0 overflow-hidden')
    if (host) host.dataset.novaWorkbenchMainHost = 'true'
    return host
  })
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  useEffect(() => {
    cleanupLegacyActivityOrderStorage()
    setActivityOrders(readStoredActivityOrders())
  }, [])

  useEffect(() => {
    storeActivityBarWidth(activityBarWidth)
  }, [activityBarWidth])

  useEffect(() => {
    let cancelled = false
    let timer: number | null = null
    let running = false
    const clearTimer = () => {
      if (timer === null) return
      window.clearTimeout(timer)
      timer = null
    }
    const scheduleNext = () => {
      clearTimer()
      if (cancelled || document.visibilityState !== 'visible') return
      timer = window.setTimeout(() => {
        timer = null
        void loadTaskActivity()
      }, TASK_ACTIVITY_REFRESH_INTERVAL_MS)
    }
    async function loadTaskActivity() {
      if (cancelled || running || document.visibilityState !== 'visible') return
      running = true
      try {
        const result = await getTasks()
        if (cancelled) return
        setTaskActivity({ result, loadState: 'ready' })
        for (const task of result.tasks) {
          if (task.status !== 'waiting_user' && task.status !== 'failed') continue
          if (!systemNotificationsEnabled) continue
          if (notifiedTaskIDsRef.current.has(task.id)) continue
          notifiedTaskIDsRef.current.add(task.id)
          void maybeNotifyActionableTask({
            enabled: systemNotificationsEnabled,
            typeLabel: t(`workbench.mobile.taskCenter.type.${task.type}`),
            projectName: task.project.name,
          })
        }
      } catch {
        if (!cancelled) {
          setTaskActivity((current) => ({ ...current, loadState: 'error' }))
        }
      } finally {
        running = false
        scheduleNext()
      }
    }
    const handleVisibilityChange = () => {
      clearTimer()
      if (document.visibilityState === 'visible') void loadTaskActivity()
    }
    void loadTaskActivity()
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => {
      cancelled = true
      clearTimer()
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [systemNotificationsEnabled, t])

  const automationInboxUnread = taskActivity.result.tasks.filter((task) => task.type === 'automation' && task.status === 'waiting_user').length
  const automationRunning = taskActivity.result.tasks.filter((task) => task.type === 'automation' && task.status === 'running').length

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

  const leaveConfigSurface = (action: () => void) => {
    const key = mode === 'skills'
      ? 'skills'
      : mode === 'agents'
        ? 'agents'
        : mode === 'automations'
          ? 'automations'
          : mode === 'interactive' && interactiveSubmode === 'teller'
            ? 'setting-panel'
            : mode === 'ide' && (loreVisible || tellerVisible)
              ? 'setting-panel'
              : null
    if (key && !settingsOpen && hasPendingExecutableDraft(key)) {
      setPendingLeave({ key, proceed: action })
      return
    }
    action()
  }

  const openWriting = () => {
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      onSetMode('ide')
      if (loreVisible || tellerVisible || versionsVisible) onSetRightPanel(null)
    })
  }

  const switchNavigationMode = (nextMode: 'ide' | 'interactive') => {
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      onSetMode(nextMode)
    })
  }

  const toggleIdePanel = (panel: NonNullable<RightPanel>) => {
    if (rightPanel === 'teller' && panel === 'teller') {
      leaveConfigSurface(() => {
        closeSettingsIfOpen()
        onSetMode('ide')
        onSetRightPanel(null)
      })
      return
    }
    closeSettingsIfOpen()
    onSetMode('ide')
    onSetRightPanel(rightPanel === panel ? null : panel)
  }

  const openVersions = () => {
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (mode === 'books' || mode === 'skills' || mode === 'agents' || mode === 'automations') {
        onSetMode(booksReturnMode)
      }
      onSetRightPanel(versionsVisible ? null : 'versions')
    })
  }

  const openInteractiveSubmode = (nextMode: InteractiveSubmode) => {
    if (mode === 'interactive' && interactiveSubmode === nextMode) {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      return
    }
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      onSetMode('interactive')
      onSetInteractiveSubmode(nextMode)
    })
  }

  const openMobileAgent = () => {
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      onSetMode('ide')
      onSetRightPanel('ai')
    })
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
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      onSetMode('books')
    })
  }

  const manageBooks = () => {
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      onSetMode('books')
    })
  }

  const openAgents = () => {
    if (mode === 'agents' && !settingsOpen) {
      leaveConfigSurface(returnFromBooks)
      return
    }
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      onSetMode('agents')
    })
  }

  const openSkills = () => {
    if (mode === 'skills' && !settingsOpen) {
      leaveConfigSurface(returnFromBooks)
      return
    }
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      onSetMode('skills')
    })
  }

  const openAutomations = () => {
    if (mode === 'automations' && !settingsOpen) {
      leaveConfigSurface(returnFromBooks)
      return
    }
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      onSetMode('automations')
    })
  }

  const openAutomationNotification = (target: AutomationMessageNavigation) => {
    leaveConfigSurface(() => {
      closeSettingsIfOpen()
      if (versionsVisible) onSetRightPanel(null)
      requestAutomationNavigation(target)
      onSetMode('automations')
    })
  }

  const openMobileTask = (task: TaskCenterTask) => {
    switch (task.recovery.kind) {
      case 'automation_run':
      case 'automation_inbox':
        if (!task.recovery.task_id) return
        openAutomationNotification({
          taskId: task.recovery.task_id,
          runId: task.recovery.run_id,
          inboxId: task.recovery.inbox_id,
          workspace: task.recovery.workspace,
        })
        return
      case 'agent_session':
        if (!task.recovery.session_id) return
        void onQuickSwitchBook(task.recovery.workspace).then((switched) => {
          if (!switched) return
          requestAgentSessionRecovery({ sessionId: task.recovery.session_id!, taskId: task.recovery.task_id })
          openMobileAgent()
          requestMobileWorkbenchDestination('agent', 'ide')
        })
        return
      case 'config_manager':
        if (!task.recovery.task_id) return
        void onQuickSwitchBook(task.recovery.workspace).then((switched) => {
          if (!switched) return
          if (task.recovery.origin === 'skills') onSetMode('skills')
          else if (task.recovery.origin === 'automation') onSetMode('automations')
          else onSetMode('agents')
        })
        return
      case 'interactive_story':
        if (!task.recovery.story_id || !task.recovery.branch_id) return
        void onQuickSwitchBook(task.recovery.workspace).then((switched) => {
          if (!switched) return
          requestInteractiveStoryRecovery({ storyId: task.recovery.story_id!, branchId: task.recovery.branch_id!, taskId: task.recovery.task_id })
          onSetMode('interactive')
          onSetInteractiveSubmode('story')
          requestMobileWorkbenchDestination('story', 'interactive')
        })
        return
      case 'image_generation':
        void onQuickSwitchBook(task.recovery.workspace)
        return
      case 'import_export':
        void onQuickSwitchBook(task.recovery.workspace)
    }
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
      active: interactiveModeActive && (interactiveSubmode === 'story' || interactiveSubmode === 'director'),
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
      icon: <ActivityIconBadge count={automationInboxUnread} running={automationRunning > 0}><Clock3 className="size-3" /></ActivityIconBadge>,
    },
  ]

  const activityItems = useMemo(
    () => sortActivityItems([
      ...(navigationMode === 'interactive' ? interactiveActivityItems : ideActivityItems),
      ...sharedActivityItems,
    ], activityOrder, defaultActivityOrderForScope(activityOrderScope)),
    [activityOrder, activityOrderScope, agentsActive, automationInboxUnread, automationRunning, automationsActive, booksReturnMode, ideModeActive, interactiveModeActive, interactiveSubmode, loreVisible, mode, navigationMode, settingsOpen, skillsActive, tellerVisible, versionsVisible],
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
    setActivityBarWidth(clampActivityBarWidth(nextWidth))
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
        <MessageCenterButton className="h-7 w-7" onOpenAutomation={openAutomationNotification} />
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
            onClick={item.onClick}
            active={item.active}
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
        >
          <PanelLeft className={`h-4 w-4 transition-transform ${activityBarExpanded ? '' : 'rotate-180'}`} />
        </ActivityButton>
        <ActivityButton
          expanded={activityBarExpanded}
          label={t('workbench.activity.settings')}
          onClick={onToggleSettings}
          active={settingsOpen}
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
      {mode === 'ide' && summary && (
        <span className="ml-4">{t('workbench.status.summary', { title: summary.title || t('workbench.untitled'), chapters: formatNumber(summary.chapter_count), words: formatNumber(summary.total_words) })}</span>
      )}
      {mode === 'ide' && currentChapter && (
        <span className="ml-4">{t('workbench.status.currentChapter', { title: currentChapter.display_title, words: formatNumber(currentChapter.words), status: currentChapter.status })}</span>
      )}
      {mode === 'ide' && currentChapter && (
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
    const writingDestinations: MobileWorkbenchDestination[] = [
      {
        id: 'manuscript',
        label: t('workbench.mobile.manuscript'),
        title: currentChapter?.display_title || t('workbench.mobile.manuscript'),
        icon: <FileText className="size-4" />,
        content: mainContentSlot,
        onSelect: openWriting,
      },
      {
        id: 'project',
        label: t('workbench.mobile.project'),
        title: t('workbench.mobile.project'),
        icon: <PanelLeft className="size-4" />,
        content: sidebar,
        onSelect: openWriting,
      },
      {
        id: 'agent',
        label: t('workbench.mobile.agent'),
        title: t('workbench.mobile.agent'),
        icon: <Bot className="size-4" />,
        content: rightPanelContent,
        onSelect: openMobileAgent,
      },
    ]
    const interactiveDestinations: MobileWorkbenchDestination[] = [
      {
        id: 'story',
        label: t('workbench.mobile.story'),
        title: t('workbench.mobile.story'),
        icon: <MessageSquareText className="size-4" />,
        content: mainContentSlot,
        onSelect: () => openInteractiveSubmode('story'),
      },
      {
        id: 'storylines',
        label: t('workbench.mobile.storylines'),
        title: t('workbench.mobile.storylines'),
        icon: <History className="size-4" />,
        content: mainContentSlot,
        onSelect: () => openInteractiveSubmode('timeline'),
      },
      {
        id: 'reference',
        label: t('workbench.mobile.reference'),
        title: t('workbench.mobile.reference'),
        icon: <Database className="size-4" />,
        content: mainContentSlot,
        onSelect: () => openInteractiveSubmode('lore'),
      },
    ]
    const mobileMode = navigationMode === 'interactive' ? 'interactive' : 'ide'
    const destinations = mobileMode === 'interactive' ? interactiveDestinations : writingDestinations
    const defaultDestinationId = mobileMode === 'interactive'
      ? interactiveSubmode === 'timeline' ? 'storylines' : interactiveSubmode === 'lore' ? 'reference' : 'story'
      : 'manuscript'
    const mobileMoreItems: MobileMoreMenuItem[] = [
      { id: 'versions', label: t('workbench.activity.versions'), icon: <History className="size-4" />, onSelect: openVersions },
      { id: 'skills', label: t('workbench.activity.skills'), icon: <Sparkles className="size-4" />, onSelect: openSkills },
      { id: 'agents', label: t('workbench.activity.agents'), icon: <Bot className="size-4" />, onSelect: openAgents },
      { id: 'automations', label: t('workbench.activity.automations'), icon: <Clock3 className="size-4" />, onSelect: openAutomations },
      { id: 'settings', label: t('workbench.activity.settings'), icon: <Settings className="size-4" />, onSelect: onToggleSettings },
    ]
    const mobileMoreMenu = (
      <MobileMoreMenu
        mode={mobileMode}
        modeSwitchLabel={t('workbench.modeSwitch')}
        writingModeLabel={t('workbench.mode.ideButton')}
        gameModeLabel={t('workbench.mode.interactiveButton')}
        items={mobileMoreItems}
        taskCenter={taskActivity.result}
        taskCenterLoadState={taskActivity.loadState}
        notice={notice ? <WorkbenchNoticePill expanded notice={notice} starSecondaryText="description" onOpenSettings={onToggleSettings} onDismiss={onDismissNotice} /> : undefined}
        onSelectMode={switchNavigationMode}
        onOpenTask={openMobileTask}
      />
    )

    return (
      <>
        <MobileWorkbenchShell
          persistenceKey={workspace}
          modeKey={mobileMode}
          defaultDestinationId={defaultDestinationId}
          destinations={destinations}
          moreLabel={t('workbench.mobile.more')}
          moreIcon={<MoreHorizontal className="size-4" />}
          moreMenu={mobileMoreMenu}
          moreBadgeCount={taskActivity.result.action_required_count}
          moreBadgeLabel={t('workbench.mobile.taskCenter.badge', { count: taskActivity.result.action_required_count })}
          sharedContent={mainContentSlot}
          sharedMenuActive={sharedMenuActive}
          navigationLabel={t('workbench.mobile.navigation')}
          projectSwitcher={(
            <BookSwitcher
              books={books}
              currentBookName={currentBookName}
              currentChapterCount={summary?.chapter_count}
              workspace={workspace}
              compact
              onSwitchBook={onQuickSwitchBook}
              onManageBooks={manageBooks}
            />
          )}
        />
        {mainContentPortal}
        <UnsavedConfigGuardDialog
          open={Boolean(pendingLeave)}
          onOpenChange={(open) => {
            if (!open) setPendingLeave(null)
          }}
          onDiscard={() => {
            const target = pendingLeave
            if (!target) return
            setPendingLeave(null)
            void discardExecutableDraft(target.key)
            target.proceed()
          }}
        />
      </>
    )
  }

  return (
    <>
      <WorkspaceLayout
        topBar={topBar}
        activityBar={activityBar}
        sidebar={sidebar}
        sidebarVisible={mode === 'ide' && projectVisible && !fullWorkspacePanelVisible}
        main={mainContentSlot}
        rightPanel={rightPanelContent}
        rightPanelVisible={mode === 'ide' && !fullWorkspacePanelVisible && Boolean(rightPanelContent)}
        rightPanelWide={rightPanelWide && mode === 'ide' && rightPanel === 'ai' && !fullWorkspacePanelVisible}
        centerFocus={centerFocus && mode === 'ide' && !fullWorkspacePanelVisible}
        statusBar={statusBar}
      />
      {mainContentPortal}
      <UnsavedConfigGuardDialog
        open={Boolean(pendingLeave)}
        onOpenChange={(open) => {
          if (!open) setPendingLeave(null)
        }}
        onDiscard={() => {
          const target = pendingLeave
          if (!target) return
          setPendingLeave(null)
          void discardExecutableDraft(target.key)
          target.proceed()
        }}
      />
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
      className={`${className || ''} relative overflow-hidden ${expanded ? 'w-full gap-3 px-3' : ''} ${active ? 'is-active' : ''}`}
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
