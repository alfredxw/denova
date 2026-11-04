import { closeMobilePanes } from '@/components/layout/mobile-pane-events'
import { startTransition, useLayoutEffect, useRef, useState, type ComponentProps } from 'react'
import { DndContext, KeyboardSensor, MouseSensor, TouchSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useTranslation } from 'react-i18next'
import { ArrowDownUp, Check, Clock3, Loader2, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { LoadingState } from '@/components/common/LoadingState'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { AgentChatProject, AgentChatSession } from './api'
import { AgentChatSidebarProject, projectSortableID, type AgentChatSidebarProjectDragData } from './AgentChatSidebarProject'
import type { AgentChatSidebarActivity } from './sidebar-activity'
import { AGENT_CHAT_SIDEBAR_SORT_MODES, useAgentChatSidebarPreferences, type AgentChatSidebarSortMode } from './sidebar-preferences'

export interface AgentChatActivitySidebarProps {
  projects: AgentChatProject[]
  activitiesByProject: ReadonlyMap<string, readonly AgentChatSidebarActivity[]>
  loading: boolean
  error: string
  activeProjectId: string
  onSelectProject: (project: AgentChatProject) => void
  onOpenActivity: (project: AgentChatProject, activity: AgentChatSidebarActivity) => void
  onOpenSession: (project: AgentChatProject, session: AgentChatSession) => void
  onRenameSession: (project: AgentChatProject, session: AgentChatSession) => void
  onDeleteSession: (project: AgentChatProject, session: AgentChatSession) => void
  onCreateSession: (project: AgentChatProject, customAgentId?: string) => void
  onOpenHistory: (project?: AgentChatProject) => void
  onAddProject: () => void
  projectDirectoryBusy: boolean
  onRenameProject: (project: AgentChatProject) => void
  onRelinkProject: (project: AgentChatProject) => void
  onArchiveProject: (project: AgentChatProject) => void
}

/** Project navigation with live work, bounded recent conversations, and durable history access. */
export function AgentChatActivitySidebar({
  projects,
  activitiesByProject,
  loading,
  error,
  activeProjectId,
  onSelectProject,
  onOpenActivity,
  onOpenSession,
  onRenameSession,
  onDeleteSession,
  onCreateSession,
  onOpenHistory,
  onAddProject,
  projectDirectoryBusy,
  onRenameProject,
  onRelinkProject,
  onArchiveProject,
}: AgentChatActivitySidebarProps) {
  const { t } = useTranslation()
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
  const handleDragEnd = (event: DragEndEvent) => {
    const active = event.active.data.current as AgentChatSidebarProjectDragData | undefined
    const over = event.over?.data.current as AgentChatSidebarProjectDragData | undefined
    if (!active || !over || active.kind !== 'project' || over.kind !== 'project') return
    preferences.moveProject(active.projectID, over.projectID)
  }

  return (
    <div className="flex h-full min-h-0 flex-col border-r border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div data-agent-chat-sidebar-header className="flex h-9 shrink-0 items-center gap-1 pl-2.5 pr-1">
        <span className="min-w-0 flex-1 truncate text-[10px] font-medium uppercase tracking-wide text-[var(--nova-text-faint)]">
          {t('agentChat.sidebar.projects')}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="shrink-0"
          disabled={projectDirectoryBusy}
          onClick={onAddProject}
          aria-label={t('agentChat.project.add')}
        >
          {projectDirectoryBusy ? <Loader2 className="animate-spin" /> : <Plus />}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="shrink-0"
          onClick={() => onOpenHistory()}
          aria-label={t('agentChat.history.open')}
        >
          <Clock3 />
        </Button>
        <SidebarSortMenu sortMode={preferences.sortMode} onSortModeChange={preferences.setSortMode} />

      </div>

      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <div className="min-h-0 flex-1 overflow-y-auto p-1.5 pt-0">
          {error ? <InlineErrorNotice className="mb-2" message={error} title={t('agentChat.sidebar.loadFailed')} /> : null}
          {loading && projects.length === 0 ? (
            <LoadingState label={t('router.loading')} variant="panel" className="min-h-0 px-0 py-1" />
          ) : projects.length === 0 ? (
            <p className="px-2 py-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agentChat.sidebar.noProjects')}</p>
          ) : (
            <>
              <SortableContext items={preferences.orderedProjects.map((project) => projectSortableID(project.id))} strategy={verticalListSortingStrategy}>
                {preferences.orderedProjects.map((project) => (
                  <ExpandableProject
                    deferContent={projects.length > 30}
                    key={project.id}
                    project={project}
                    activeProjectId={activeProjectId}
                    manualSorting={preferences.sortMode === 'manual'}
                    pinned={preferences.isProjectPinned(project.id)}
                    activities={activitiesByProject.get(project.id) ?? []}
                    onToggle={() => {
                      if (project.id === activeProjectId) { onSelectProject(project); return }
                      // Paint the row expansion before mounting the destination workspace.
                      startTransition(() => {
                        preferences.recordProjectOpened(project.id)
                        onSelectProject(project)
                      })
                    }}
                    onCreateSession={(customAgentId) => {
                      preferences.recordProjectOpened(project.id)
                      onCreateSession(project, customAgentId)
                      closeMobilePanes()
                    }}
                    onTogglePinned={() => preferences.toggleProjectPinned(project.id)}
                    onRename={() => onRenameProject(project)}
                    onRelink={() => onRelinkProject(project)}
                    onArchive={() => onArchiveProject(project)}
                    onOpenHistory={() => onOpenHistory(project)}
                    onOpenSession={(session) => {
                      preferences.recordProjectOpened(project.id)
                      onOpenSession(project, session)
                      closeMobilePanes()
                    }}
                    onRenameSession={(session) => onRenameSession(project, session)}
                    onDeleteSession={(session) => onDeleteSession(project, session)}
                    onOpenActivity={(activity) => {
                      preferences.recordProjectOpened(project.id)
                      onOpenActivity(project, activity)
                      closeMobilePanes()
                    }}
                  />
                ))}
              </SortableContext>
            </>
          )}
        </div>
      </DndContext>
    </div>
  )
}

/** Keep expansion local so toggling one row does not render the entire Project tree. */
function ExpandableProject({ activeProjectId, onToggle, ...props }: Omit<ComponentProps<typeof AgentChatSidebarProject>, 'active' | 'expanded'> & {
  activeProjectId: string
}) {
  const active = props.project.id === activeProjectId
  const [expanded, setExpanded] = useState(active)
  const rowTogglePending = useRef(false)
  useLayoutEffect(() => {
    // External navigation reveals its destination; a row click keeps its explicit toggle.
    if (active && !rowTogglePending.current) setExpanded(true)
    rowTogglePending.current = false
  }, [active, activeProjectId])
  return <AgentChatSidebarProject {...props} active={active} expanded={expanded} onToggle={() => {
    rowTogglePending.current = !active
    setExpanded(current => !current)
    onToggle()
  }} />
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
