import type { CSSProperties } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useTranslation } from 'react-i18next'
import { CircleAlert, Bot, ChevronRight, Folder, FolderOpen, MoreHorizontal, Pencil, Pin, PinOff, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger } from '@/components/ui/context-menu'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import type { AgentChatProject, AgentChatSession } from './api'
import { AgentChatProjectDetailsCard } from './AgentChatProjectDetailsCard'
import { summarizeSidebarActivities, type AgentChatActivityStatus, type AgentChatSidebarActivity } from './sidebar-activity'

const RECENT_CONVERSATION_LIMIT = 3

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
  onOpenHistory: () => void
  onOpenSession: (session: AgentChatSession) => void
  onRenameSession: (session: AgentChatSession) => void
  onOpenActivity: (activity: AgentChatSidebarActivity) => void
}

/** One sortable Project with stable recent conversations, live status, and terminal entries. */
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
  onOpenHistory,
  onOpenSession,
  onRenameSession,
  onOpenActivity,
}: AgentChatSidebarProjectProps) {
  const { t } = useTranslation()
  const name = project.name || project.path
  const summary = summarizeSidebarActivities(activities)
  const projectSessionIDs = new Set(project.sessions.map((session) => session.id))
  const activityBySessionID = new Map<string, AgentChatSidebarActivity>()
  const supplementalActivities: AgentChatSidebarActivity[] = []
  for (const activity of activities) {
    if (activity.kind === 'agent' && activity.sessionId && projectSessionIDs.has(activity.sessionId)) {
      activityBySessionID.set(activity.sessionId, activity)
    } else {
      // Terminals and not-yet-persisted conversation drafts have no durable recent-session row.
      supplementalActivities.push(activity)
    }
  }
  const recentSessions = project.sessions.slice(0, RECENT_CONVERSATION_LIMIT)
  const recentSessionIDs = new Set(recentSessions.map((session) => session.id))
  const additionalActiveSessions = project.sessions.filter(
    (session) => !recentSessionIDs.has(session.id) && activityBySessionID.has(session.id),
  )
  const visibleSessions = [...recentSessions, ...additionalActiveSessions]
  const hasExpandableContent = Boolean(
    project.error || visibleSessions.length > 0 || supplementalActivities.length > 0 || project.total > 0,
  )
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
      <ContextMenu>
        <AgentChatProjectDetailsCard project={project} active={active} manualSorting={manualSorting}>
          <ContextMenuTrigger asChild>
            <div
              data-current-project={active ? 'true' : undefined}
              className={cn(
                'group relative flex items-center gap-0.5 rounded-[var(--nova-radius)] px-0.5 transition-colors',
                active ? 'bg-[var(--nova-surface-2)]' : 'hover:bg-[var(--nova-hover)]',
              )}
            >
              {active ? (
                <span
                  data-slot="agent-chat-project-active-indicator"
                  aria-hidden="true"
                  className="absolute bottom-1.5 left-0 top-1.5 w-0.5 rounded-full bg-[var(--nova-accent)]"
                />
              ) : null}
              <button
                ref={setActivatorNodeRef}
                data-slot="agent-chat-project-toggle"
                data-project-id={project.id}
                type="button"
                {...(manualSorting ? attributes : {})}
                {...(manualSorting ? listeners : {})}
                onClick={onToggle}
                aria-expanded={hasExpandableContent ? expanded : undefined}
                aria-label={hasExpandableContent
                  ? t(expanded ? 'agentChat.sidebar.collapseProject' : 'agentChat.sidebar.expandProject', { name })
                  : name}
                aria-current={active ? 'location' : undefined}
                aria-description={manualSorting ? t('agentChat.sidebar.longPressToReorder') : undefined}
                className={`flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] py-1.5 pl-1 text-left outline-none focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)] ${manualSorting ? 'cursor-default' : ''}`}
              >
                <ChevronRight
                  data-slot="agent-chat-project-chevron"
                  aria-hidden="true"
                  className={cn(
                    'size-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform duration-[var(--nova-motion-fast)] ease-[var(--nova-panel-motion-ease)]',
                    expanded && 'rotate-90',
                    !hasExpandableContent && 'invisible',
                  )}
                />
                {project.type === 'general' ? (
                  <Bot className={cn('size-3.5 shrink-0', active ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-muted)]')} />
                ) : expanded ? (
                  <FolderOpen className={cn('size-3.5 shrink-0', active ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-muted)]')} />
                ) : (
                  <Folder className={cn('size-3.5 shrink-0', active ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-muted)]')} />
                )}
                <span className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--nova-text)]">{name}</span>
                {project.status === 'missing' ? (
                  <CircleAlert className="size-3 shrink-0 text-[var(--nova-warning)]" aria-label={t('agentChat.project.missing')} />
                ) : null}
                <ProjectActivitySummary expanded={expanded} summary={summary} />
                {pinned ? <Pin className="size-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" /> : null}
              </button>
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
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                className="shrink-0 opacity-60 transition-opacity hover:opacity-100 focus-visible:opacity-100"
                disabled={project.status !== 'available'}
                onClick={onCreateSession}
                aria-label={t('agentChat.sidebar.newChatIn', { name })}
              >
                <Plus />
              </Button>
            </div>
          </ContextMenuTrigger>
        </AgentChatProjectDetailsCard>
        <ContextMenuContent className="min-w-40">
          <ContextMenuItem onSelect={onRename}>
            <Pencil />
            {t('agentChat.project.rename')}
          </ContextMenuItem>
          <ContextMenuItem onSelect={onRelink}>{t('agentChat.project.relink')}</ContextMenuItem>
          <ContextMenuItem onSelect={onTogglePinned}>
            {pinned ? <PinOff /> : <Pin />}
            {t(pinned ? 'agentChat.sidebar.unpinProject' : 'agentChat.sidebar.pinProject')}
          </ContextMenuItem>
          <ContextMenuItem variant="destructive" onSelect={onArchive}>
            {t('agentChat.project.archive')}
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>

      {hasExpandableContent ? (
        <div
          data-slot="agent-chat-project-content"
          data-state={expanded ? 'open' : 'closed'}
          aria-hidden={!expanded}
          inert={!expanded}
          className={cn(
            'grid transition-[grid-template-rows,opacity] duration-[var(--nova-motion-fast)] ease-[var(--nova-panel-motion-ease)]',
            expanded ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
          )}
        >
          <div className="min-h-0 overflow-hidden">
            <div className="ml-3 mt-1 border-l border-[var(--nova-border-soft)] pl-1.5">
              {project.error ? (
                <p className="px-2 py-2 text-[11px] text-[var(--nova-danger)]">{project.error}</p>
              ) : null}
              {visibleSessions.length > 0 || supplementalActivities.length > 0 ? (
                <div className="pb-1 pt-1">
                  {visibleSessions.map((session) => (
                    <ConversationRow
                      key={session.id}
                      session={session}
                      activity={activityBySessionID.get(session.id)}
                      onOpen={() => onOpenSession(session)}
                      onRename={() => onRenameSession(session)}
                    />
                  ))}
                  {supplementalActivities.map((activity) => (
                    <LiveActivityRow key={activity.id} activity={activity} onOpen={() => onOpenActivity(activity)} />
                  ))}
                </div>
              ) : null}
              {project.total > 0 ? (
                <div className="mt-1 border-t border-[var(--nova-border-soft)] pt-1">
                  <button
                    type="button"
                    onClick={onOpenHistory}
                    aria-label={t('agentChat.sidebar.allConversationsIn', { name, count: project.total })}
                    className="flex w-full min-w-0 items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1.5 text-left text-[10px] font-medium text-[var(--nova-text-faint)] outline-none transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)]"
                  >
                    <span className="min-w-0 flex-1 truncate">{t('agentChat.sidebar.allConversations')}</span>
                    <span className="shrink-0 tabular-nums">{project.total}</span>
                  </button>
                </div>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function ConversationRow({
  session,
  activity,
  onOpen,
  onRename,
}: {
  session: AgentChatSession
  activity?: AgentChatSidebarActivity
  onOpen: () => void
  onRename: () => void
}) {
  const { t } = useTranslation()
  const title = session.title || t('chat.untitledSession')
  const focused = activity?.focused ?? false
  const statusLabel = activity ? t(`agentChat.sidebar.status.${activity.status}`) : ''
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <button
          data-slot="agent-chat-conversation-row"
          type="button"
          onClick={onOpen}
          aria-label={activity
            ? `${title} · ${statusLabel}`
            : t('agentChat.history.openSession', { title })}
          aria-current={focused ? 'page' : undefined}
          className={cn(
            'group/conversation relative flex w-full min-w-0 items-center gap-1.5 rounded-[var(--nova-radius)] py-1.5 pl-1.5 pr-1.5 text-left outline-none transition-colors focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)]',
            focused
              ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
              : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
          )}
        >
          <span
            aria-hidden="true"
            className={cn(
              'absolute bottom-1.5 left-0 top-1.5 w-0.5 rounded-full',
              focused ? 'bg-[var(--nova-text)]' : activity?.paneVisible ? 'bg-[var(--nova-text-faint)]' : 'bg-transparent',
            )}
          />
          <span className="min-w-0 flex-1 truncate text-[11px]">{title}</span>
          {activity ? <ActivityStatus status={activity.status} label={statusLabel} /> : null}
        </button>
      </ContextMenuTrigger>
      <ContextMenuContent className="min-w-40">
        <ContextMenuItem onSelect={onRename}>
          <Pencil />
          {t('chat.renameSession')}
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
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

function LiveActivityRow({ activity, onOpen }: { activity: AgentChatSidebarActivity; onOpen: () => void }) {
  const { t } = useTranslation()
  const isConversation = activity.kind === 'agent'
  const label = t(`agentChat.sidebar.status.${activity.status}`)
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-current={activity.focused ? 'page' : undefined}
      className={cn(
        'group/activity relative flex w-full min-w-0 items-center gap-1.5 rounded-[var(--nova-radius)] py-1.5 pr-1.5 text-left outline-none transition-colors focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)]',
        'pl-1.5',
        activity.focused
          ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
          : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
      )}
    >
      <span
        aria-hidden="true"
        className={`absolute bottom-1.5 left-0 top-1.5 w-0.5 rounded-full ${
          activity.focused ? 'bg-[var(--nova-text)]' : activity.paneVisible ? 'bg-[var(--nova-text-faint)]' : 'bg-transparent'
        }`}
      />
      <span className={cn('min-w-0 flex-1 truncate text-[11px]', !isConversation && 'font-mono text-[10px]')}>{activity.title}</span>
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
