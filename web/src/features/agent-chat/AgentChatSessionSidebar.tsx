import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight, Edit3, FolderOpen, MessageSquareText, MoreHorizontal, PanelLeft, Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { formatDateTime } from '@/i18n'
import type { AgentChatProject, AgentChatSession } from './api'

/**
 * Conversations shown per project, and how many each "show more" adds. Revealing the list a
 * page at a time keeps a project with hundreds of conversations from burying its neighbours.
 */
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

/**
 * Project-grouped conversation tree.
 *
 * Every book is listed regardless of which workspace the backend has open, because AgentChat
 * manages the whole library. Each project row can start its own conversation.
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
  /** How many conversations each project currently shows; absent means the first page. */
  const [visibleCounts, setVisibleCounts] = useState<ReadonlyMap<string, number>>(() => new Map())
  const [editing, setEditing] = useState<{ path: string; id: string } | null>(null)
  const [draftTitle, setDraftTitle] = useState('')
  const [pendingDelete, setPendingDelete] = useState<{ project: AgentChatProject; session: AgentChatSession } | null>(null)

  const toggleProject = (path: string) => {
    setCollapsedProjects((current) => {
      const next = new Set(current)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }

  /** Grow or shrink one project's list a page at a time, never below the first page. */
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

  return (
    <div className="flex h-full min-h-0 flex-col border-r border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="flex h-9 shrink-0 items-center gap-1 pl-2.5 pr-1">
        <span className="min-w-0 flex-1 truncate text-[10px] font-medium uppercase tracking-wide text-[var(--nova-text-faint)]">
          {t('agentChat.sidebar.projects')}
        </span>
        {onCollapse && (
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
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-1.5 pt-0">
        {error ? <InlineErrorNotice className="mb-2" message={error} title={t('agentChat.sidebar.loadFailed')} /> : null}
        {loading && projects.length === 0 ? (
          <p className="px-2 py-3 text-[11px] text-[var(--nova-text-faint)]">{t('router.loading')}</p>
        ) : projects.length === 0 ? (
          <p className="px-2 py-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agentChat.sidebar.noProjects')}</p>
        ) : (
          projects.map((project) => {
            const expanded = !collapsedProjects.has(project.path)
            const shown = visibleCounts.get(project.path) ?? SESSION_PAGE_SIZE
            const visibleSessions = project.sessions.slice(0, shown)
            const hiddenCount = project.total - visibleSessions.length
            return (
              <div key={project.path} className="mb-1">
                <div className={`group flex items-center gap-0.5 rounded-[var(--nova-radius)] pr-0.5 ${
                  project.path === activeProjectPath ? 'bg-[var(--nova-active)]' : 'hover:bg-[var(--nova-hover)]'
                }`}>
                  <button
                    type="button"
                    onClick={() => toggleProject(project.path)}
                    aria-expanded={expanded}
                    aria-label={expanded ? t('common.collapse') : t('common.expand')}
                    className="flex size-6 shrink-0 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-faint)] outline-none hover:text-[var(--nova-text)] focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)]"
                  >
                    <ChevronRight className={`size-3 shrink-0 text-[var(--nova-text-faint)] transition-transform ${expanded ? 'rotate-90' : ''}`} />
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      onSelectProject(project)
                      if (!expanded) toggleProject(project.path)
                    }}
                    aria-current={project.path === activeProjectPath ? 'true' : undefined}
                    title={project.path}
                    className="flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] py-1.5 pr-1 text-left outline-none focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)]"
                  >
                    <FolderOpen className="size-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                    <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text)]">{project.name || project.path}</span>
                  </button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    className="shrink-0 opacity-60 transition-opacity hover:opacity-100 focus-visible:opacity-100"
                    onClick={() => onCreateSession(project)}
                    aria-label={t('agentChat.sidebar.newChatIn', { name: project.name || project.path })}
                    title={t('agentChat.sidebar.newChat')}
                  >
                    <Plus />
                  </Button>
                </div>

                {expanded && (
                  <div className="ml-3 border-l border-[var(--nova-border-soft)] pl-1.5">
                    {project.error ? (
                      <p className="px-2 py-2 text-[11px] text-[var(--nova-danger)]">{project.error}</p>
                    ) : project.sessions.length === 0 ? (
                      <p className="px-2 py-2 text-[11px] text-[var(--nova-text-faint)]">{t('agentChat.sidebar.noSessions')}</p>
                    ) : (
                      <>
                        {visibleSessions.map((session) => (
                          <SessionRow
                            key={session.id}
                            session={session}
                            running={isSessionRunning(project, session)}
                            editing={editing?.path === project.path && editing.id === session.id}
                            draftTitle={draftTitle}
                            onDraftTitleChange={setDraftTitle}
                            onOpen={() => onOpenSession(project, session)}
                            onBeginRename={() => {
                              setEditing({ path: project.path, id: session.id })
                              setDraftTitle(session.title || '')
                            }}
                            onSubmitRename={() => void submitRename(project, session)}
                            onCancelRename={() => setEditing(null)}
                            onRequestDelete={() => setPendingDelete({ project, session })}
                          />
                        ))}
                        {(hiddenCount > 0 || visibleSessions.length > SESSION_PAGE_SIZE) && (
                          <div className="flex items-center gap-2 px-1.5 py-1 text-[11px]">
                            {hiddenCount > 0 && (
                              <button
                                type="button"
                                onClick={() => stepVisibleCount(project.path, 1)}
                                className="rounded-[var(--nova-radius)] px-1 py-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
                              >
                                {t('agentChat.sidebar.showMore', { count: Math.min(hiddenCount, SESSION_PAGE_SIZE) })}
                              </button>
                            )}
                            {visibleSessions.length > SESSION_PAGE_SIZE && (
                              <button
                                type="button"
                                onClick={() => stepVisibleCount(project.path, -1)}
                                className="rounded-[var(--nova-radius)] px-1 py-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)]"
                              >
                                {t('agentChat.sidebar.showLess')}
                              </button>
                            )}
                          </div>
                        )}
                      </>
                    )}
                  </div>
                )}
              </div>
            )
          })
        )}
      </div>

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

/** Grace period before a peek closes, so crossing the rail's edge diagonally does not dismiss it. */
const PEEK_CLOSE_DELAY_MS = 160

interface AgentChatSidebarRailProps extends Omit<AgentChatSessionSidebarProps, 'onCollapse'> {
  onExpand: () => void
  /** Starts a conversation in the current project without expanding the tree first. */
  onCreateDefaultSession: () => void
  createDisabled: boolean
}

/**
 * The tree collapsed to a rail.
 *
 * The rail keeps the two things worth reaching without the tree — reopening it and starting a
 * conversation — and pointing at it peeks the full tree over the workbench, so a collapsed
 * sidebar still answers "what else is open?" without giving the space back permanently.
 */
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

      {peeking && (
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
      )}
    </div>
  )
}

function SessionRow({
  session,
  running,
  editing,
  draftTitle,
  onDraftTitleChange,
  onOpen,
  onBeginRename,
  onSubmitRename,
  onCancelRename,
  onRequestDelete,
}: {
  session: AgentChatSession
  running: boolean
  editing: boolean
  draftTitle: string
  onDraftTitleChange: (value: string) => void
  onOpen: () => void
  onBeginRename: () => void
  onSubmitRename: () => void
  onCancelRename: () => void
  onRequestDelete: () => void
}) {
  const { t } = useTranslation()
  const title = session.title || t('chat.untitledSession')

  if (editing) {
    return (
      <div className="px-1 py-1">
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
    <div className="group flex items-center gap-1 rounded-[var(--nova-radius)] pr-0.5 hover:bg-[var(--nova-hover)]">
      <button
        type="button"
        onClick={onOpen}
        title={`${title} · ${formatDateTime(session.updated_at || session.created_at) || ''}`}
        className="flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1.5 text-left outline-none focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)]"
      >
        <MessageSquareText className={`size-3.5 shrink-0 ${running ? 'animate-pulse text-[var(--nova-accent)]' : 'text-[var(--nova-text-faint)]'}`} />
        <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text)]">{title}</span>
      </button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            className="shrink-0 opacity-0 transition-opacity group-hover:opacity-100 data-[state=open]:opacity-100"
            aria-label={t('agentChat.sidebar.sessionActions', { title })}
          >
            <MoreHorizontal />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-36">
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
