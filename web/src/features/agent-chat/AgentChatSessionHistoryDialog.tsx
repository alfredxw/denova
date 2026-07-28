import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Clock3, Edit3, LoaderCircle, MessageSquareText, MoreHorizontal, Search, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { formatDateTime } from '@/i18n'
import { getAgentChatHistory, type AgentChatHistoryItem } from './api'

const HISTORY_PAGE_SIZE = 80
const HISTORY_SEARCH_DELAY_MS = 180
const HISTORY_SEARCH_MAX_LENGTH = 200

interface AgentChatSessionHistoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onOpenSession: (item: AgentChatHistoryItem) => void
  onRenameSession: (item: AgentChatHistoryItem, title: string) => void | Promise<void>
  onDeleteSession: (item: AgentChatHistoryItem) => void | Promise<void>
}

/** Bounded, searchable durable history kept separate from the live activity navigator. */
export function AgentChatSessionHistoryDialog({
  open,
  onOpenChange,
  onOpenSession,
  onRenameSession,
  onDeleteSession,
}: AgentChatSessionHistoryDialogProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [items, setItems] = useState<AgentChatHistoryItem[]>([])
  const [total, setTotal] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const [reloadVersion, setReloadVersion] = useState(0)
  const [editing, setEditing] = useState<AgentChatHistoryItem | null>(null)
  const [draftTitle, setDraftTitle] = useState('')
  const [pendingDelete, setPendingDelete] = useState<AgentChatHistoryItem | null>(null)
  const requestSequenceRef = useRef(0)
  const editingRef = useRef<AgentChatHistoryItem | null>(null)
  const renameSubmittingRef = useRef(false)

  useEffect(() => {
    if (!open) return
    const timer = window.setTimeout(() => setDebouncedQuery(query), HISTORY_SEARCH_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [open, query])

  useEffect(() => {
    if (open) return
    editingRef.current = null
    renameSubmittingRef.current = false
    setEditing(null)
    setDraftTitle('')
    setPendingDelete(null)
  }, [open])

  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    const sequence = ++requestSequenceRef.current
    setLoading(true)
    setError('')
    void getAgentChatHistory({ query: debouncedQuery, limit: HISTORY_PAGE_SIZE, signal: controller.signal }).then((page) => {
      if (requestSequenceRef.current !== sequence) return
      setItems(page.items)
      setTotal(page.total)
      setHasMore(page.has_more)
    }).catch((loadError) => {
      if (controller.signal.aborted || requestSequenceRef.current !== sequence) return
      console.error('[features/agent-chat/AgentChatSessionHistoryDialog.tsx] loading history failed', {
        queryLength: debouncedQuery.length,
        error: loadError,
      })
      setItems([])
      setError(loadError instanceof Error ? loadError.message : String(loadError))
    }).finally(() => {
      if (requestSequenceRef.current === sequence) setLoading(false)
    })
    return () => controller.abort()
  }, [debouncedQuery, open, reloadVersion])

  const loadMore = async () => {
    if (loadingMore || !hasMore) return
    const sequence = ++requestSequenceRef.current
    setLoadingMore(true)
    setError('')
    try {
      const page = await getAgentChatHistory({ query: debouncedQuery, offset: items.length, limit: HISTORY_PAGE_SIZE })
      if (requestSequenceRef.current !== sequence) return
      setItems((current) => [...current, ...page.items])
      setTotal(page.total)
      setHasMore(page.has_more)
    } catch (loadError) {
      if (requestSequenceRef.current !== sequence) return
      console.error('[features/agent-chat/AgentChatSessionHistoryDialog.tsx] loading more history failed', {
        offset: items.length,
        error: loadError,
      })
      setError(loadError instanceof Error ? loadError.message : String(loadError))
    } finally {
      if (requestSequenceRef.current === sequence) setLoadingMore(false)
    }
  }

  const beginRename = (item: AgentChatHistoryItem) => {
    setError('')
    editingRef.current = item
    setEditing(item)
    setDraftTitle(item.session.title || '')
  }

  const cancelRename = () => {
    editingRef.current = null
    setEditing(null)
    setDraftTitle('')
  }

  const submitRename = async () => {
    const item = editingRef.current
    if (!item || renameSubmittingRef.current) return
    const title = draftTitle.trim()
    if (!title || title === item.session.title) {
      cancelRename()
      return
    }
    renameSubmittingRef.current = true
    setError('')
    try {
      await onRenameSession(item, title)
      cancelRename()
      setReloadVersion((version) => version + 1)
    } catch (renameError) {
      setError(renameError instanceof Error ? renameError.message : String(renameError))
    } finally {
      renameSubmittingRef.current = false
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="nova-panel flex h-[min(72dvh,620px)] w-[min(680px,calc(100vw-2rem))] max-w-[min(680px,calc(100vw-2rem))] flex-col gap-0 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
          <DialogHeader className="shrink-0 gap-1 border-b border-[var(--nova-border)] px-4 py-3 pr-12 text-left">
            <DialogTitle className="flex items-center gap-2 text-sm">
              <Clock3 className="size-4 text-[var(--nova-text-muted)]" />
              {t('agentChat.history.title')}
            </DialogTitle>
            <DialogDescription className="text-[11px] text-[var(--nova-text-faint)]">
              {t('agentChat.history.description')}
            </DialogDescription>
          </DialogHeader>

          <div className="shrink-0 border-b border-[var(--nova-border-soft)] p-2.5">
            <div className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
              <Input
                autoFocus
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                maxLength={HISTORY_SEARCH_MAX_LENGTH}
                placeholder={t('agentChat.history.searchPlaceholder')}
                aria-label={t('agentChat.history.searchLabel')}
                className="h-8 border-[var(--nova-border)] bg-[var(--nova-surface-2)] pl-8 text-xs"
              />
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto p-2">
            {error ? <InlineErrorNotice className="mb-2" title={t('agentChat.history.loadFailed')} message={error} /> : null}
            {loading ? (
              <div role="status" className="flex h-28 items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]">
                <LoaderCircle className="size-4 animate-spin" />
                {t('router.loading')}
              </div>
            ) : items.length === 0 ? (
              <div className="flex h-28 flex-col items-center justify-center gap-2 px-6 text-center text-xs text-[var(--nova-text-faint)]">
                <MessageSquareText className="size-5" />
                {t(query.trim() ? 'agentChat.history.noResults' : 'agentChat.history.empty')}
              </div>
            ) : (
              <div className="space-y-0.5">
                {items.map((item) => {
                  const key = historyItemKey(item)
                  return editing && historyItemKey(editing) === key ? (
                    <div key={key} className="flex items-center gap-2 rounded-[var(--nova-radius)] bg-[var(--nova-active)] px-2 py-1.5">
                      <MessageSquareText className="size-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                      <Input
                        autoFocus
                        value={draftTitle}
                        onChange={(event) => setDraftTitle(event.target.value)}
                        onBlur={() => { void submitRename() }}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') {
                            event.preventDefault()
                            void submitRename()
                          }
                          if (event.key === 'Escape') {
                            event.preventDefault()
                            cancelRename()
                          }
                        }}
                        aria-label={t('chat.sessionTitle')}
                        className="h-7 min-w-0 flex-1 text-xs"
                      />
                    </div>
                  ) : (
                    <HistoryRow
                      key={key}
                      item={item}
                      onOpen={() => {
                        onOpenSession(item)
                        onOpenChange(false)
                      }}
                      onRename={() => beginRename(item)}
                      onDelete={() => setPendingDelete(item)}
                    />
                  )
                })}
                {hasMore ? (
                  <div className="flex justify-center py-2">
                    <Button type="button" variant="ghost" size="xs" disabled={loadingMore} onClick={() => { void loadMore() }}>
                      {loadingMore ? <LoaderCircle className="animate-spin" /> : null}
                      {t('agentChat.history.loadMore')}
                    </Button>
                  </div>
                ) : null}
              </div>
            )}
          </div>

          <div className="flex h-8 shrink-0 items-center border-t border-[var(--nova-border-soft)] px-3 text-[10px] text-[var(--nova-text-faint)]">
            {t('agentChat.history.count', { shown: items.length, total })}
          </div>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(nextOpen) => { if (!nextOpen) setPendingDelete(null) }}
        title={t('agentChat.sidebar.deleteTitle')}
        description={t('agentChat.sidebar.deleteDescription', {
          title: pendingDelete?.session.title || t('chat.untitledSession'),
        })}
        tone="danger"
        confirmLabel={t('common.delete')}
        onConfirm={async () => {
          if (!pendingDelete) return
          await onDeleteSession(pendingDelete)
          setPendingDelete(null)
          setReloadVersion((version) => version + 1)
        }}
      />
    </>
  )
}

function HistoryRow({
  item,
  onOpen,
  onRename,
  onDelete,
}: {
  item: AgentChatHistoryItem
  onOpen: () => void
  onRename: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const title = item.session.title || t('chat.untitledSession')
  const projectName = item.project_name || item.workspace
  return (
    <div className="group flex min-w-0 items-center gap-0.5 rounded-[var(--nova-radius)] pr-0.5 hover:bg-[var(--nova-hover)]">
      <button
        type="button"
        onClick={onOpen}
        aria-label={t('agentChat.history.openSession', { title })}
        className="flex min-w-0 flex-1 items-center gap-2 rounded-[var(--nova-radius)] px-2 py-2 text-left outline-none focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)]"
      >
        <MessageSquareText className={`size-3.5 shrink-0 ${item.session.running ? 'text-[var(--nova-success)]' : 'text-[var(--nova-text-faint)]'}`} />
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate text-xs text-[var(--nova-text)]">{title}</span>
            {item.session.running ? (
              <span className="inline-flex shrink-0 items-center gap-1 text-[9px] text-[var(--nova-success)]">
                <span aria-hidden="true" className="size-1.5 animate-pulse rounded-full bg-current" />
                {t('agentChat.sidebar.status.running')}
              </span>
            ) : null}
          </span>
          <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[10px] text-[var(--nova-text-faint)]">
            <span className="max-w-[45%] truncate">{projectName}</span>
            <span aria-hidden="true">·</span>
            <span className="shrink-0">{formatDateTime(item.session.updated_at || item.session.created_at)}</span>
            <span aria-hidden="true">·</span>
            <span className="shrink-0">{t('agentChat.history.messageCount', { count: item.session.message_count })}</span>
          </span>
        </span>
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
          <DropdownMenuItem onSelect={onRename}>
            <Edit3 />
            {t('common.rename')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" disabled={item.session.running} onSelect={onDelete}>
            <Trash2 />
            {t('common.delete')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function historyItemKey(item: AgentChatHistoryItem): string {
  return `${item.workspace}\u0000${item.session.id}`
}
