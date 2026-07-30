import { useEffect, useRef, useState } from 'react'
import { DndContext, KeyboardSensor, MouseSensor, TouchSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useTranslation } from 'react-i18next'
import { ArrowDownUp, Check, Clock3, Loader2, PanelLeft, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { AgentChatProject } from './api'
import { AgentChatSidebarProject, projectSortableID, type AgentChatSidebarProjectDragData } from './AgentChatSidebarProject'
import type { AgentChatSidebarActivity } from './sidebar-activity'
import { AGENT_CHAT_SIDEBAR_SORT_MODES, useAgentChatSidebarPreferences, type AgentChatSidebarSortMode } from './sidebar-preferences'

export interface AgentChatActivitySidebarProps {
  projects: AgentChatProject[]
  activitiesByProject: ReadonlyMap<string, readonly AgentChatSidebarActivity[]>
  loading: boolean
  error: string
  activeProjectId: string
  /** Rendered in the header when the tree can be collapsed to a rail. */
  onCollapse?: () => void
  onSelectProject: (project: AgentChatProject) => void
  onOpenActivity: (project: AgentChatProject, activity: AgentChatSidebarActivity) => void
  onCreateSession: (project: AgentChatProject) => void
  onOpenHistory: () => void
  onAddProject: () => void
  projectDirectoryBusy: boolean
  onRenameProject: (project: AgentChatProject) => void
  onRelinkProject: (project: AgentChatProject) => void
  onArchiveProject: (project: AgentChatProject) => void
}

/** Project tree whose children are live work entry points rather than conversation history. */
export function AgentChatActivitySidebar({
  projects,
  activitiesByProject,
  loading,
  error,
  activeProjectId,
  onCollapse,
  onSelectProject,
  onOpenActivity,
  onCreateSession,
  onOpenHistory,
  onAddProject,
  projectDirectoryBusy,
  onRenameProject,
  onRelinkProject,
  onArchiveProject,
}: AgentChatActivitySidebarProps) {
  const { t } = useTranslation()
  const [collapsedProjects, setCollapsedProjects] = useState<ReadonlySet<string>>(() => new Set())
  const preferences = useAgentChatSidebarPreferences(projects)
  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: { delay: 180, tolerance: 5 },
    }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )
  const activityCount = [...activitiesByProject.values()].reduce((total, activities) => total + activities.length, 0)

  const toggleProject = (project: AgentChatProject) => {
    const selecting = project.id !== activeProjectId
    preferences.recordProjectOpened(project.id)
    onSelectProject(project)
    setCollapsedProjects((current) => {
      const next = new Set(current)
      if (selecting) next.delete(project.id)
      else if (next.has(project.id)) next.delete(project.id)
      else next.add(project.id)
      return next
    })
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const active = event.active.data.current as AgentChatSidebarProjectDragData | undefined
    const over = event.over?.data.current as AgentChatSidebarProjectDragData | undefined
    if (!active || !over || active.kind !== 'project' || over.kind !== 'project') return
    preferences.moveProject(active.projectID, over.projectID)
  }

  return (
    <div className="flex h-full min-h-0 flex-col border-r border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="flex h-9 shrink-0 items-center gap-1 pl-2.5 pr-1">
        <span className="min-w-0 flex-1 truncate text-[10px] font-medium uppercase tracking-wide text-[var(--nova-text-faint)]">
          {t('agentChat.sidebar.projects')} · {t('agentChat.sidebar.activeWork')}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="shrink-0"
          disabled={projectDirectoryBusy}
          onClick={onAddProject}
          aria-label={t('agentChat.project.add')}
          title={t('agentChat.project.add')}
        >
          {projectDirectoryBusy ? <Loader2 className="animate-spin" /> : <Plus />}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="shrink-0"
          onClick={onOpenHistory}
          aria-label={t('agentChat.history.open')}
          title={t('agentChat.history.open')}
        >
          <Clock3 />
        </Button>
        <SidebarSortMenu sortMode={preferences.sortMode} onSortModeChange={preferences.setSortMode} />
        {onCollapse ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            className="shrink-0"
            onClick={onCollapse}
            aria-label={t('agentChat.sidebar.hide')}
            title={t('agentChat.sidebar.hide')}
          >
            <PanelLeft />
          </Button>
        ) : null}
      </div>

      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <div className="min-h-0 flex-1 overflow-y-auto p-1.5 pt-0">
          {error ? <InlineErrorNotice className="mb-2" message={error} title={t('agentChat.sidebar.loadFailed')} /> : null}
          {loading && projects.length === 0 ? (
            <p className="px-2 py-3 text-[11px] text-[var(--nova-text-faint)]">{t('router.loading')}</p>
          ) : projects.length === 0 ? (
            <p className="px-2 py-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agentChat.sidebar.noProjects')}</p>
          ) : (
            <>
              <SortableContext items={preferences.orderedProjects.map((project) => projectSortableID(project.id))} strategy={verticalListSortingStrategy}>
                {preferences.orderedProjects.map((project) => (
                  <AgentChatSidebarProject
                    key={project.id}
                    project={project}
                    active={project.id === activeProjectId}
                    expanded={!collapsedProjects.has(project.id)}
                    manualSorting={preferences.sortMode === 'manual'}
                    pinned={preferences.isProjectPinned(project.id)}
                    activities={activitiesByProject.get(project.id) ?? []}
                    onToggle={() => toggleProject(project)}
                    onCreateSession={() => {
                      preferences.recordProjectOpened(project.id)
                      onCreateSession(project)
                    }}
                    onTogglePinned={() => preferences.toggleProjectPinned(project.id)}
                    onRename={() => onRenameProject(project)}
                    onRelink={() => onRelinkProject(project)}
                    onArchive={() => onArchiveProject(project)}
                    onOpenActivity={(activity) => {
                      preferences.recordProjectOpened(project.id)
                      onOpenActivity(project, activity)
                    }}
                  />
                ))}
              </SortableContext>
              {activityCount === 0 ? (
                <p className="px-2 pb-2 pt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('agentChat.sidebar.noActiveWork')}</p>
              ) : null}
            </>
          )}
        </div>
      </DndContext>
    </div>
  )
}

function SidebarSortMenu({ sortMode, onSortModeChange }: { sortMode: AgentChatSidebarSortMode; onSortModeChange: (mode: AgentChatSidebarSortMode) => void }) {
  const { t } = useTranslation()
  const label = t(`agentChat.sidebar.sort.${sortMode}`)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="shrink-0"
          aria-label={t('agentChat.sidebar.sort.label', { mode: label })}
          title={t('agentChat.sidebar.sort.label', { mode: label })}
        >
          <ArrowDownUp />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-40">
        {AGENT_CHAT_SIDEBAR_SORT_MODES.map((mode) => (
          <DropdownMenuItem key={mode} onSelect={() => onSortModeChange(mode)} aria-current={mode === sortMode ? 'true' : undefined}>
            <Check className={mode === sortMode ? '' : 'opacity-0'} />
            {t(`agentChat.sidebar.sort.${mode}`)}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** Grace period before a peek closes, so crossing the rail's edge diagonally does not dismiss it. */
const PEEK_CLOSE_DELAY_MS = 160
/** Avoid mounting the full project tree when the pointer is only crossing toward a rail action. */
const PEEK_OPEN_DELAY_MS = 120

interface AgentChatSidebarRailProps extends Omit<AgentChatActivitySidebarProps, 'onCollapse'> {
  onExpand: () => void
  /** Starts a conversation in the current project without expanding the tree first. */
  onCreateDefaultSession: () => void
  createDisabled: boolean
}

/** Compact launcher plus a temporary full activity-tree peek. */
export function AgentChatSidebarRail({ onExpand, onCreateDefaultSession, createDisabled, ...tree }: AgentChatSidebarRailProps) {
  const { t } = useTranslation()
  const [peeking, setPeeking] = useState(false)
  const openTimerRef = useRef<number | null>(null)
  const closeTimerRef = useRef<number | null>(null)

  const cancelOpen = () => {
    if (openTimerRef.current === null) return
    window.clearTimeout(openTimerRef.current)
    openTimerRef.current = null
  }
  const cancelClose = () => {
    if (closeTimerRef.current === null) return
    window.clearTimeout(closeTimerRef.current)
    closeTimerRef.current = null
  }
  const schedulePeek = () => {
    cancelClose()
    if (peeking || openTimerRef.current !== null) return
    openTimerRef.current = window.setTimeout(() => {
      openTimerRef.current = null
      setPeeking(true)
    }, PEEK_OPEN_DELAY_MS)
  }
  const closePeek = () => {
    cancelOpen()
    cancelClose()
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null
      setPeeking(false)
    }, PEEK_CLOSE_DELAY_MS)
  }
  const expandSidebar = () => {
    cancelOpen()
    cancelClose()
    setPeeking(false)
    onExpand()
  }

  useEffect(() => () => {
    cancelOpen()
    cancelClose()
  }, [])

  return (
    <div
      className="relative z-40 flex h-full w-10 shrink-0 flex-col items-center gap-1 border-r border-[var(--nova-border)] bg-[var(--nova-surface)] py-1"
      onMouseEnter={schedulePeek}
      onMouseLeave={closePeek}
      onFocusCapture={schedulePeek}
      onBlurCapture={closePeek}
    >
      <Button type="button" variant="ghost" size="icon-xs" onClick={expandSidebar} aria-label={t('agentChat.sidebar.show')} title={t('agentChat.sidebar.show')}>
        <PanelLeft className="rotate-180" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        disabled={createDisabled}
        onClick={onCreateDefaultSession}
        aria-label={t('agentChat.sidebar.newChat')}
        title={t('agentChat.sidebar.newChat')}
      >
        <Plus />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        onClick={tree.onOpenHistory}
        aria-label={t('agentChat.history.open')}
        title={t('agentChat.history.open')}
      >
        <Clock3 />
      </Button>

      {peeking ? (
        <div
          className="absolute left-full top-0 h-full w-[clamp(200px,18vw,280px)] shadow-[8px_0_24px_-18px_rgba(0,0,0,0.8)]"
          onKeyDown={(event) => {
            if (event.key === 'Escape') setPeeking(false)
          }}
        >
          <AgentChatActivitySidebar
            {...tree}
            onOpenActivity={(project, activity) => {
              setPeeking(false)
              tree.onOpenActivity(project, activity)
            }}
            onOpenHistory={() => {
              setPeeking(false)
              tree.onOpenHistory()
            }}
          />
        </div>
      ) : null}
    </div>
  )
}
