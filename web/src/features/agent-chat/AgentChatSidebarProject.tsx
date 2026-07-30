import type { CSSProperties } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useTranslation } from 'react-i18next'
import { CircleAlert, Bot, ChevronRight, Folder, FolderOpen, MessageSquareText, MoreHorizontal, Pin, PinOff, Plus, TerminalSquare } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { AgentChatProject } from './api'
import { summarizeSidebarActivities, type AgentChatActivityStatus, type AgentChatSidebarActivity } from './sidebar-activity'

export interface AgentChatSidebarProjectDragData {
  kind: 'project'
  projectID: string
}

export function projectSortableID(projectID: string) {
  return `agent-chat-project:${projectID}`
}

interface AgentChatSidebarProjectProps {
  project: AgentChatProject
  active: boolean
  expanded: boolean
  manualSorting: boolean
  pinned: boolean
  activities: readonly AgentChatSidebarActivity[]
  onToggle: () => void
  onCreateSession: () => void
  onTogglePinned: () => void
  onRename: () => void
  onRelink: () => void
  onArchive: () => void
  onOpenActivity: (activity: AgentChatSidebarActivity) => void
}

/** One sortable project with a read-only projection of its active conversations and terminals. */
export function AgentChatSidebarProject({
  project,
  active,
  expanded,
  manualSorting,
  pinned,
  activities,
  onToggle,
  onCreateSession,
  onTogglePinned,
  onRename,
  onRelink,
  onArchive,
  onOpenActivity,
}: AgentChatSidebarProjectProps) {
  const { t } = useTranslation()
  const name = project.name || project.path
  const summary = summarizeSidebarActivities(activities)
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id: projectSortableID(project.id),
    data: {
      kind: 'project',
      projectID: project.id,
    } satisfies AgentChatSidebarProjectDragData,
    disabled: !manualSorting,
  })
  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <div ref={setNodeRef} style={style} className={`mb-1 ${isDragging ? 'relative z-20 opacity-80' : ''}`}>
      <div className="group relative flex items-center gap-0.5 rounded-[var(--nova-radius)] pr-0.5 transition-colors hover:bg-[var(--nova-hover)]">
        {active && !expanded ? (
          <span
            data-slot="agent-chat-project-active-indicator"
            aria-hidden="true"
            className="absolute bottom-2 left-0 top-2 w-px rounded-full bg-[var(--nova-text-faint)]"
          />
        ) : null}
        <button
          ref={setActivatorNodeRef}
          type="button"
          {...(manualSorting ? attributes : {})}
          {...(manualSorting ? listeners : {})}
          onClick={onToggle}
          aria-expanded={expanded}
          title={manualSorting ? `${project.path} · ${t('agentChat.sidebar.longPressToReorder')}` : project.path}
          className={`flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] px-1 py-1.5 text-left outline-none focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)] ${manualSorting ? 'cursor-default' : ''}`}
        >
          <ChevronRight className={`size-3 shrink-0 text-[var(--nova-text-faint)] transition-transform ${expanded ? 'rotate-90' : ''}`} />
          {project.type === 'general' ? (
            <Bot className="size-3.5 shrink-0 text-[var(--nova-text-muted)]" />
          ) : expanded ? (
            <FolderOpen className="size-3.5 shrink-0 text-[var(--nova-text-muted)]" />
          ) : (
            <Folder className="size-3.5 shrink-0 text-[var(--nova-text-muted)]" />
          )}
          <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text)]">{name}</span>
          {project.status === 'missing' ? (
            <CircleAlert className="size-3 shrink-0 text-[var(--nova-warning)]" aria-label={t('agentChat.project.missing')} />
          ) : null}
          <ProjectActivitySummary expanded={expanded} summary={summary} />
          {pinned ? <Pin className="size-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" /> : null}
        </button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="shrink-0 opacity-60 transition-opacity hover:opacity-100 focus-visible:opacity-100"
          disabled={project.status !== 'available'}
          onClick={onCreateSession}
          aria-label={t('agentChat.sidebar.newChatIn', { name })}
          title={t('agentChat.sidebar.newChat')}
        >
          <Plus />
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className="shrink-0 opacity-0 transition-opacity group-hover:opacity-100 data-[state=open]:opacity-100 focus-visible:opacity-100"
              aria-label={t('agentChat.sidebar.projectActions', { name })}
            >
              <MoreHorizontal />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-36">
            <DropdownMenuItem onSelect={onRename}>{t('agentChat.project.rename')}</DropdownMenuItem>
            <DropdownMenuItem onSelect={onRelink}>{t('agentChat.project.relink')}</DropdownMenuItem>
            <DropdownMenuItem onSelect={onTogglePinned}>
              {pinned ? <PinOff /> : <Pin />}
              {t(pinned ? 'agentChat.sidebar.unpinProject' : 'agentChat.sidebar.pinProject')}
            </DropdownMenuItem>
            <DropdownMenuItem className="text-[var(--nova-danger)]" onSelect={onArchive}>
              {t('agentChat.project.archive')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {expanded && (project.error || activities.length > 0) ? (
        <div className="ml-3 mt-1 border-l border-[var(--nova-border-soft)] pl-1.5">
          {project.error ? (
            <p className="px-2 py-2 text-[11px] text-[var(--nova-danger)]">{project.error}</p>
          ) : (
            activities.map((activity) => <ActivityRow key={activity.id} activity={activity} onOpen={() => onOpenActivity(activity)} />)
          )}
        </div>
      ) : null}
    </div>
  )
}

function ProjectActivitySummary({ expanded, summary }: { expanded: boolean; summary: ReturnType<typeof summarizeSidebarActivities> }) {
  const { t } = useTranslation()
  if (summary.total === 0) return null
  return (
    <span className="flex shrink-0 items-center gap-1 text-[9px] tabular-nums text-[var(--nova-text-faint)]">
      {summary.running > 0 ? (
        <span
          className="inline-flex items-center gap-0.5 text-[var(--nova-success)]"
          aria-label={t('agentChat.sidebar.summary.running', {
            count: summary.running,
          })}
        >
          <span aria-hidden="true" className="size-1.5 animate-pulse rounded-full bg-current" />
          {summary.running}
        </span>
      ) : null}
      {summary.attention > 0 ? (
        <span
          className="inline-flex items-center gap-0.5 text-[var(--nova-danger)]"
          aria-label={t('agentChat.sidebar.summary.attention', {
            count: summary.attention,
          })}
        >
          <CircleAlert aria-hidden="true" className="size-2.5" />
          {summary.attention}
        </span>
      ) : null}
      {!expanded && summary.running === 0 && summary.attention === 0 ? summary.total : null}
    </span>
  )
}

function ActivityRow({ activity, onOpen }: { activity: AgentChatSidebarActivity; onOpen: () => void }) {
  const { t } = useTranslation()
  const Icon = activity.kind === 'agent' ? MessageSquareText : TerminalSquare
  const label = t(`agentChat.sidebar.status.${activity.status}`)
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-current={activity.focused ? 'page' : undefined}
      title={`${activity.title} · ${label}`}
      className={`group/activity relative flex w-full min-w-0 items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1.5 text-left outline-none transition-colors focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)] ${
        activity.focused
          ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
          : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
      }`}
    >
      <span
        aria-hidden="true"
        className={`absolute bottom-1.5 left-0 top-1.5 w-0.5 rounded-full ${
          activity.focused ? 'bg-[var(--nova-text)]' : activity.paneVisible ? 'bg-[var(--nova-text-faint)]' : 'bg-transparent'
        }`}
      />
      <Icon className="size-3.5 shrink-0 text-[var(--nova-text-faint)] group-hover/activity:text-[var(--nova-text-muted)]" />
      <span className="min-w-0 flex-1 truncate text-xs">{activity.title}</span>
      <ActivityStatus status={activity.status} label={label} />
    </button>
  )
}

function ActivityStatus({ status, label }: { status: AgentChatActivityStatus; label: string }) {
  const tone = activityStatusTone(status)
  return (
    <span className={`inline-flex shrink-0 items-center gap-1 text-[9px] ${tone}`}>
      <span aria-hidden="true" className={`size-1.5 rounded-full bg-current ${status === 'running' || status === 'connecting' ? 'animate-pulse' : ''}`} />
      <span>{label}</span>
    </span>
  )
}

function activityStatusTone(status: AgentChatActivityStatus): string {
  switch (status) {
    case 'running':
    case 'ready':
      return 'text-[var(--nova-success)]'
    case 'connecting':
      return 'text-[var(--nova-warning)]'
    case 'error':
      return 'text-[var(--nova-danger)]'
    case 'idle':
    case 'exited':
      return 'text-[var(--nova-text-faint)]'
  }
}
