import { useEffect, useRef, useState } from 'react'
import {
  DndContext,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  closestCenter,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
} from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useTranslation } from 'react-i18next'
import { ArrowDownUp, Check, PanelLeft, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { AgentChatProject, AgentChatSession } from './api'
import {
  AgentChatSidebarProject,
  projectSortableID,
  type AgentChatSidebarDragData,
} from './AgentChatSidebarProject'
import {
  AGENT_CHAT_SIDEBAR_SORT_MODES,
  useAgentChatSidebarPreferences,
  type AgentChatSidebarSortMode,
} from './sidebar-preferences'

/** Conversations shown initially per project, and how many each reveal action adds. */
const SESSION_PAGE_SIZE = 5

interface AgentChatSessionSidebarProps {
  projects: AgentChatProject[]
  loading: boolean
  error: string
  activeProjectPath: string
  isSessionRunning: (project: AgentChatProject, session: AgentChatSession) => boolean
  /** Rendered in the header when the tree can be collapsed to a rail. */
  onCollapse?: () => void
  onSelectProject: (project: AgentChatProject) => void
  onOpenSession: (project: AgentChatProject, session: AgentChatSession) => void
  onCreateSession: (project: AgentChatProject) => void
  onRenameSession: (project: AgentChatProject, session: AgentChatSession, title: string) => void | Promise<void>
  onDeleteSession: (project: AgentChatProject, session: AgentChatSession) => void | Promise<void>
}

/** Restricts nested project/session sorting to the active hierarchy lane. */
const sidebarCollisionDetection: CollisionDetection = (args) => {
  const active = args.active.data.current as AgentChatSidebarDragData | undefined
  if (!active) return []
  const droppableContainers = args.droppableContainers.filter((container) => {
    const candidate = container.data.current as AgentChatSidebarDragData | undefined
    if (!candidate || candidate.kind !== active.kind) return false
    return active.kind === 'project'
      || (candidate.kind === 'session' && candidate.projectPath === active.projectPath)
  })
  return closestCenter({ ...args, droppableContainers })
}

/**
 * Project-grouped conversation tree.
 *
 * Pin, recent-open and manual order are user-level UI preferences. They never mutate a book or
 * enter Agent context; server timestamps remain authoritative for the Updated sort.
 */
export function AgentChatSessionSidebar({
  projects,
  loading,
  error,
  activeProjectPath,
  isSessionRunning,
  onCollapse,
  onSelectProject,
  onOpenSession,
  onCreateSession,
  onRenameSession,
  onDeleteSession,
}: AgentChatSessionSidebarProps) {
  const { t } = useTranslation()
  const [collapsedProjects, setCollapsedProjects] = useState<ReadonlySet<string>>(() => new Set())
  const [visibleCounts, setVisibleCounts] = useState<ReadonlyMap<string, number>>(() => new Map())
  const [editing, setEditing] = useState<{ path: string; id: string } | null>(null)
  const [draftTitle, setDraftTitle] = useState('')
  const [pendingDelete, setPendingDelete] = useState<{ project: AgentChatProject; session: AgentChatSession } | null>(null)
  const preferences = useAgentChatSidebarPreferences(projects)
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { delay: 180, tolerance: 5 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const toggleProject = (project: AgentChatProject) => {
    preferences.recordProjectOpened(project.path)
    onSelectProject(project)
    setCollapsedProjects((current) => {
      const next = new Set(current)
      if (next.has(project.path)) next.delete(project.path)
      else next.add(project.path)
      return next
    })
  }

  const stepVisibleCount = (path: string, direction: 1 | -1) => {
    setVisibleCounts((current) => {
      const shown = current.get(path) ?? SESSION_PAGE_SIZE
      const next = new Map(current)
      next.set(path, Math.max(SESSION_PAGE_SIZE, shown + direction * SESSION_PAGE_SIZE))
      return next
    })
  }

  const submitRename = async (project: AgentChatProject, session: AgentChatSession) => {
    const title = draftTitle.trim()
    setEditing(null)
    setDraftTitle('')
    if (!title || title === session.title) return
    await onRenameSession(project, session, title)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const active = event.active.data.current as AgentChatSidebarDragData | undefined
    const over = event.over?.data.current as AgentChatSidebarDragData | undefined
    if (!active || !over || active.kind !== over.kind) return
    if (active.kind === 'project' && over.kind === 'project') {
      preferences.moveProject(active.projectPath, over.projectPath)
      return
    }
    if (active.kind !== 'session' || over.kind !== 'session' || active.projectPath !== over.projectPath) return
    const project = projects.find((candidate) => candidate.path === active.projectPath)
    if (project) preferences.moveSession(project, active.sessionID, over.sessionID)
  }

  return (
    <div className="flex h-full min-h-0 flex-col border-r border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="flex h-9 shrink-0 items-center gap-1 pl-2.5 pr-1">
        <span className="min-w-0 flex-1 truncate text-[10px] font-medium uppercase tracking-wide text-[var(--nova-text-faint)]">
          {t('agentChat.sidebar.projects')}
        </span>
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

      <DndContext
        sensors={sensors}
        collisionDetection={sidebarCollisionDetection}
        onDragEnd={handleDragEnd}
      >
        <div className="min-h-0 flex-1 overflow-y-auto p-1.5 pt-0">
          {error ? <InlineErrorNotice className="mb-2" message={error} title={t('agentChat.sidebar.loadFailed')} /> : null}
          {loading && projects.length === 0 ? (
            <p className="px-2 py-3 text-[11px] text-[var(--nova-text-faint)]">{t('router.loading')}</p>
          ) : projects.length === 0 ? (
            <p className="px-2 py-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agentChat.sidebar.noProjects')}</p>
          ) : (
            <SortableContext
              items={preferences.orderedProjects.map((project) => projectSortableID(project.path))}
              strategy={verticalListSortingStrategy}
            >
              {preferences.orderedProjects.map((project) => {
                const expanded = !collapsedProjects.has(project.path)
                const shown = visibleCounts.get(project.path) ?? SESSION_PAGE_SIZE
                const orderedSessions = preferences.sessionsForProject(project)
                const visibleSessions = orderedSessions.slice(0, shown)
                const hiddenCount = Math.max(0, orderedSessions.length - visibleSessions.length)
                return (
                  <AgentChatSidebarProject
                    key={project.path}
                    project={project}
                    active={project.path === activeProjectPath}
                    expanded={expanded}
                    manualSorting={preferences.sortMode === 'manual'}
                    pinned={preferences.isProjectPinned(project.path)}
                    visibleSessions={visibleSessions}
                    hiddenCount={hiddenCount}
                    showMoreCount={Math.min(hiddenCount, SESSION_PAGE_SIZE)}
                    canShowLess={visibleSessions.length > SESSION_PAGE_SIZE}
                    isSessionRunning={(session) => isSessionRunning(project, session)}
                    isSessionPinned={(sessionID) => preferences.isSessionPinned(project.path, sessionID)}
                    editingSessionID={editing?.path === project.path ? editing.id : null}
                    draftTitle={draftTitle}
                    onDraftTitleChange={setDraftTitle}
                    onToggle={() => toggleProject(project)}
                    onCreateSession={() => {
                      preferences.recordProjectOpened(project.path)
                      onCreateSession(project)
                    }}
                    onTogglePinned={() => preferences.toggleProjectPinned(project.path)}
                    onOpenSession={(session) => {
                      preferences.recordSessionOpened(project.path, session.id)
                      onOpenSession(project, session)
                    }}
                    onToggleSessionPinned={(sessionID) => preferences.toggleSessionPinned(project.path, sessionID)}
                    onBeginRename={(session) => {
                      setEditing({ path: project.path, id: session.id })
                      setDraftTitle(session.title || '')
                    }}
                    onSubmitRename={(session) => { void submitRename(project, session) }}
                    onCancelRename={() => setEditing(null)}
                    onRequestDelete={(session) => setPendingDelete({ project, session })}
                    onShowMore={() => stepVisibleCount(project.path, 1)}
                    onShowLess={() => stepVisibleCount(project.path, -1)}
                  />
                )
              })}
            </SortableContext>
          )}
        </div>
      </DndContext>

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(open) => { if (!open) setPendingDelete(null) }}
        title={t('agentChat.sidebar.deleteTitle')}
        description={t('agentChat.sidebar.deleteDescription', {
          title: pendingDelete?.session.title || t('chat.untitledSession'),
        })}
        tone="danger"
        confirmLabel={t('common.delete')}
        onConfirm={async () => {
          if (!pendingDelete) return
          await onDeleteSession(pendingDelete.project, pendingDelete.session)
          setPendingDelete(null)
        }}
      />
    </div>
  )
}

function SidebarSortMenu({
  sortMode,
  onSortModeChange,
}: {
  sortMode: AgentChatSidebarSortMode
  onSortModeChange: (mode: AgentChatSidebarSortMode) => void
}) {
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

interface AgentChatSidebarRailProps extends Omit<AgentChatSessionSidebarProps, 'onCollapse'> {
  onExpand: () => void
  /** Starts a conversation in the current project without expanding the tree first. */
  onCreateDefaultSession: () => void
  createDisabled: boolean
}

/** Compact launcher plus a temporary full conversation-tree peek. */
export function AgentChatSidebarRail({ onExpand, onCreateDefaultSession, createDisabled, ...tree }: AgentChatSidebarRailProps) {
  const { t } = useTranslation()
  const [peeking, setPeeking] = useState(false)
  const closeTimerRef = useRef<number | null>(null)

  const cancelClose = () => {
    if (closeTimerRef.current === null) return
    window.clearTimeout(closeTimerRef.current)
    closeTimerRef.current = null
  }
  const openPeek = () => {
    cancelClose()
    setPeeking(true)
  }
  const closePeek = () => {
    cancelClose()
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null
      setPeeking(false)
    }, PEEK_CLOSE_DELAY_MS)
  }

  useEffect(() => cancelClose, [])

  return (
    <div
      className="relative z-40 flex h-full w-10 shrink-0 flex-col items-center gap-1 border-r border-[var(--nova-border)] bg-[var(--nova-surface)] py-1"
      onMouseEnter={openPeek}
      onMouseLeave={closePeek}
      onFocusCapture={openPeek}
      onBlurCapture={closePeek}
    >
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        onClick={onExpand}
        aria-label={t('agentChat.sidebar.show')}
        title={t('agentChat.sidebar.show')}
      >
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

      {peeking ? (
        <div
          className="absolute left-full top-0 h-full w-[clamp(200px,18vw,280px)] shadow-[8px_0_24px_-18px_rgba(0,0,0,0.8)]"
          onKeyDown={(event) => { if (event.key === 'Escape') setPeeking(false) }}
        >
          <AgentChatSessionSidebar
            {...tree}
            onOpenSession={(project, session) => {
              setPeeking(false)
              tree.onOpenSession(project, session)
            }}
          />
        </div>
      ) : null}
    </div>
  )
}
