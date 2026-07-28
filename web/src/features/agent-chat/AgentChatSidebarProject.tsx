import type { CSSProperties } from 'react'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useTranslation } from 'react-i18next'
import {
  ChevronRight,
  Edit3,
  Folder,
  FolderOpen,
  MessageSquareText,
  MoreHorizontal,
  Pin,
  PinOff,
  Plus,
  Trash2,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { formatDateTime } from '@/i18n'
import type { AgentChatProject, AgentChatSession } from './api'

export type AgentChatSidebarDragData =
  | { kind: 'project'; projectPath: string }
  | { kind: 'session'; projectPath: string; sessionID: string }

export function projectSortableID(path: string) {
  return `agent-chat-project:${path}`
}

export function sessionSortableID(path: string, sessionID: string) {
  return `agent-chat-session:${path}:${sessionID}`
}

interface AgentChatSidebarProjectProps {
  project: AgentChatProject
  active: boolean
  expanded: boolean
  manualSorting: boolean
  pinned: boolean
  visibleSessions: AgentChatSession[]
  hiddenCount: number
  showMoreCount: number
  canShowLess: boolean
  isSessionRunning: (session: AgentChatSession) => boolean
  isSessionPinned: (sessionID: string) => boolean
  editingSessionID: string | null
  draftTitle: string
  onDraftTitleChange: (value: string) => void
  onToggle: () => void
  onCreateSession: () => void
  onTogglePinned: () => void
  onOpenSession: (session: AgentChatSession) => void
  onToggleSessionPinned: (sessionID: string) => void
  onBeginRename: (session: AgentChatSession) => void
  onSubmitRename: (session: AgentChatSession) => void
  onCancelRename: () => void
  onRequestDelete: (session: AgentChatSession) => void
  onShowMore: () => void
  onShowLess: () => void
}

/** One sortable project group; row actions are isolated from the full-width expand target. */
export function AgentChatSidebarProject({
  project,
  active,
  expanded,
  manualSorting,
  pinned,
  visibleSessions,
  hiddenCount,
  showMoreCount,
  canShowLess,
  isSessionRunning,
  isSessionPinned,
  editingSessionID,
  draftTitle,
  onDraftTitleChange,
  onToggle,
  onCreateSession,
  onTogglePinned,
  onOpenSession,
  onToggleSessionPinned,
  onBeginRename,
  onSubmitRename,
  onCancelRename,
  onRequestDelete,
  onShowMore,
  onShowLess,
}: AgentChatSidebarProjectProps) {
  const { t } = useTranslation()
  const name = project.name || project.path
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id: projectSortableID(project.path),
    data: { kind: 'project', projectPath: project.path } satisfies AgentChatSidebarDragData,
    disabled: !manualSorting,
  })
  const style: CSSProperties = { transform: CSS.Transform.toString(transform), transition }

  return (
    <div ref={setNodeRef} style={style} className={`mb-1 ${isDragging ? 'relative z-20 opacity-80' : ''}`}>
      <div className={`group flex items-center gap-0.5 rounded-[var(--nova-radius)] pr-0.5 ${
        active ? 'bg-[var(--nova-active)]' : 'hover:bg-[var(--nova-hover)]'
      }`}>
        <button
          ref={setActivatorNodeRef}
          type="button"
          {...(manualSorting ? attributes : {})}
          {...(manualSorting ? listeners : {})}
          onClick={onToggle}
          aria-expanded={expanded}
          aria-current={active ? 'true' : undefined}
          title={manualSorting ? `${project.path} · ${t('agentChat.sidebar.longPressToReorder')}` : project.path}
          className={`flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] px-1 py-1.5 text-left outline-none focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)] ${manualSorting ? 'cursor-default' : ''}`}
        >
          <ChevronRight className={`size-3 shrink-0 text-[var(--nova-text-faint)] transition-transform ${expanded ? 'rotate-90' : ''}`} />
          {expanded
            ? <FolderOpen className="size-3.5 shrink-0 text-[var(--nova-text-muted)]" />
            : <Folder className="size-3.5 shrink-0 text-[var(--nova-text-muted)]" />}
          <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text)]">{name}</span>
          {pinned ? <Pin className="size-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" /> : null}
        </button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="shrink-0 opacity-60 transition-opacity hover:opacity-100 focus-visible:opacity-100"
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
            <DropdownMenuItem onSelect={onTogglePinned}>
              {pinned ? <PinOff /> : <Pin />}
              {t(pinned ? 'agentChat.sidebar.unpinProject' : 'agentChat.sidebar.pinProject')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {expanded ? (
        <div className="ml-3 border-l border-[var(--nova-border-soft)] pl-1.5">
          {project.error ? (
            <p className="px-2 py-2 text-[11px] text-[var(--nova-danger)]">{project.error}</p>
          ) : project.sessions.length === 0 ? (
            <p className="px-2 py-2 text-[11px] text-[var(--nova-text-faint)]">{t('agentChat.sidebar.noSessions')}</p>
          ) : (
            <>
              <SortableContext
                items={visibleSessions.map((session) => sessionSortableID(project.path, session.id))}
                strategy={verticalListSortingStrategy}
              >
                {visibleSessions.map((session) => (
                  <SessionRow
                    key={session.id}
                    projectPath={project.path}
                    session={session}
                    running={isSessionRunning(session)}
                    pinned={isSessionPinned(session.id)}
                    manualSorting={manualSorting}
                    editing={editingSessionID === session.id}
                    draftTitle={draftTitle}
                    onDraftTitleChange={onDraftTitleChange}
                    onOpen={() => onOpenSession(session)}
                    onTogglePinned={() => onToggleSessionPinned(session.id)}
                    onBeginRename={() => onBeginRename(session)}
                    onSubmitRename={() => onSubmitRename(session)}
                    onCancelRename={onCancelRename}
                    onRequestDelete={() => onRequestDelete(session)}
                  />
                ))}
              </SortableContext>
              {(hiddenCount > 0 || canShowLess) ? (
                <div className="flex items-center gap-2 px-1.5 py-1 text-[11px]">
                  {hiddenCount > 0 ? (
                    <button
                      type="button"
                      onClick={onShowMore}
                      className="rounded-[var(--nova-radius)] px-1 py-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
                    >
                      {t('agentChat.sidebar.showMore', { count: showMoreCount })}
                    </button>
                  ) : null}
                  {canShowLess ? (
                    <button
                      type="button"
                      onClick={onShowLess}
                      className="rounded-[var(--nova-radius)] px-1 py-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
                    >
                      {t('agentChat.sidebar.showLess')}
                    </button>
                  ) : null}
                </div>
              ) : null}
            </>
          )}
        </div>
      ) : null}
    </div>
  )
}

function SessionRow({
  projectPath,
  session,
  running,
  pinned,
  manualSorting,
  editing,
  draftTitle,
  onDraftTitleChange,
  onOpen,
  onTogglePinned,
  onBeginRename,
  onSubmitRename,
  onCancelRename,
  onRequestDelete,
}: {
  projectPath: string
  session: AgentChatSession
  running: boolean
  pinned: boolean
  manualSorting: boolean
  editing: boolean
  draftTitle: string
  onDraftTitleChange: (value: string) => void
  onOpen: () => void
  onTogglePinned: () => void
  onBeginRename: () => void
  onSubmitRename: () => void
  onCancelRename: () => void
  onRequestDelete: () => void
}) {
  const { t } = useTranslation()
  const title = session.title || t('chat.untitledSession')
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id: sessionSortableID(projectPath, session.id),
    data: { kind: 'session', projectPath, sessionID: session.id } satisfies AgentChatSidebarDragData,
    disabled: !manualSorting || editing,
  })
  const style: CSSProperties = { transform: CSS.Transform.toString(transform), transition }

  if (editing) {
    return (
      <div ref={setNodeRef} style={style} className="px-1 py-1">
        <input
          autoFocus
          value={draftTitle}
          onChange={(event) => onDraftTitleChange(event.target.value)}
          onBlur={onSubmitRename}
          onKeyDown={(event) => {
            if (event.key === 'Enter') onSubmitRename()
            if (event.key === 'Escape') onCancelRename()
          }}
          className="nova-field h-6 w-full rounded border px-1.5 text-xs outline-none"
          aria-label={t('chat.sessionTitle')}
        />
      </div>
    )
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`group flex items-center gap-0.5 rounded-[var(--nova-radius)] pr-0.5 hover:bg-[var(--nova-hover)] ${isDragging ? 'relative z-20 opacity-80' : ''}`}
    >
      <button
        ref={setActivatorNodeRef}
        type="button"
        {...(manualSorting ? attributes : {})}
        {...(manualSorting ? listeners : {})}
        onClick={onOpen}
        title={`${title} · ${formatDateTime(session.updated_at || session.created_at) || ''}${manualSorting ? ` · ${t('agentChat.sidebar.longPressToReorder')}` : ''}`}
        className={`flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1.5 text-left outline-none focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)] ${manualSorting ? 'cursor-default' : ''}`}
      >
        <MessageSquareText className={`size-3.5 shrink-0 ${running ? 'animate-pulse text-[var(--nova-accent)]' : 'text-[var(--nova-text-faint)]'}`} />
        <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text)]">{title}</span>
        {pinned ? <Pin className="size-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" /> : null}
      </button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            className="shrink-0 opacity-0 transition-opacity group-hover:opacity-100 data-[state=open]:opacity-100 focus-visible:opacity-100"
            aria-label={t('agentChat.sidebar.sessionActions', { title })}
          >
            <MoreHorizontal />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-36">
          <DropdownMenuItem onSelect={onTogglePinned}>
            {pinned ? <PinOff /> : <Pin />}
            {t(pinned ? 'agentChat.sidebar.unpinSession' : 'agentChat.sidebar.pinSession')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={onBeginRename}>
            <Edit3 />
            {t('common.rename')}
          </DropdownMenuItem>
          <DropdownMenuItem variant="destructive" disabled={running} onSelect={onRequestDelete}>
            <Trash2 />
            {t('common.delete')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
