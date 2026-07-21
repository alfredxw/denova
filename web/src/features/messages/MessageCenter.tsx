import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowUpRight, Bell, CheckCheck, Loader2, Star } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { formatDateTime } from '@/i18n'
import { DENOVA_GITHUB_URL } from '@/lib/product-links'
import { getMessages, markAllMessagesRead, markMessageRead } from './api'
import type { AutomationMessageNavigation, ProductMessage } from './types'

const MESSAGE_CENTER_REFRESH_INTERVAL_MS = 30000

type MessageFilter = 'all' | 'action' | 'automation' | 'product'

export function MessageCenterButton({ className = '', onOpenAutomation }: { className?: string; onOpenAutomation?: (target: AutomationMessageNavigation) => void }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [items, setItems] = useState<ProductMessage[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const [activeId, setActiveId] = useState<string | null>(null)
  const [filter, setFilter] = useState<MessageFilter>('all')
  const [loading, setLoading] = useState(false)
  const [markingAllRead, setMarkingAllRead] = useState(false)
  const [error, setError] = useState('')
  const pendingReadRef = useRef<Set<string>>(new Set())

  const activeItem = useMemo(() => items.find((item) => item.id === activeId) || null, [activeId, items])
  const visibleItems = useMemo(() => items.filter((item) => messageMatchesFilter(item, filter)), [filter, items])

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const result = await getMessages()
      const nextItems = result.items || []
      setItems(nextItems)
      setUnreadCount(result.unread_count ?? countUnread(nextItems))
      setActiveId((current) => current && nextItems.some((item) => item.id === current) ? current : null)
    } catch (e) {
      setError(e instanceof Error ? e.message : t('messages.loadFailed'))
      setItems([])
      setUnreadCount(0)
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    let cancelled = false
    let running = false
    let timer: number | null = null
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
        void run()
      }, MESSAGE_CENTER_REFRESH_INTERVAL_MS)
    }
    const run = async () => {
      if (cancelled || running || document.visibilityState !== 'visible') return
      running = true
      try {
        await load()
      } finally {
        running = false
        scheduleNext()
      }
    }
    const handleVisibilityChange = () => {
      clearTimer()
      if (document.visibilityState === 'visible') void run()
    }
    void run()
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => {
      cancelled = true
      clearTimer()
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [load])

  const selectMessage = useCallback((id: string) => {
    setActiveId(id)
  }, [])

  const markRead = useCallback(async (id: string) => {
    if (pendingReadRef.current.has(id)) return
    pendingReadRef.current.add(id)
    const optimisticReadAt = new Date().toISOString()
    const wasUnread = items.some((item) => item.id === id && !item.read_at)
    setItems((current) => current.map((item) => item.id === id && !item.read_at ? { ...item, read_at: optimisticReadAt } : item))
    if (wasUnread) setUnreadCount((current) => Math.max(0, current - 1))
    try {
      const updated = await markMessageRead(id)
      setItems((current) => current.map((item) => item.id === id ? { ...item, ...updated } : item))
    } catch (e) {
      console.warn('[messages] 标记消息已读失败', e)
      setError(e instanceof Error ? e.message : t('messages.readFailed'))
      void load()
    } finally {
      pendingReadRef.current.delete(id)
    }
  }, [items, load, t])

  const markAllRead = useCallback(async () => {
    if (unreadCount <= 0 || markingAllRead) return
    setMarkingAllRead(true)
    setError('')
    const optimisticReadAt = new Date().toISOString()
    setItems((current) => current.map((item) => item.read_at ? item : { ...item, read_at: optimisticReadAt }))
    setUnreadCount(0)
    try {
      const result = await markAllMessagesRead()
      const nextItems = result.items || []
      setItems(nextItems)
      setUnreadCount(result.unread_count ?? countUnread(nextItems))
      setActiveId((current) => current && nextItems.some((item) => item.id === current) ? current : null)
    } catch (e) {
      console.warn('[messages] 标记全部消息已读失败', e)
      setError(e instanceof Error ? e.message : t('messages.readFailed'))
      void load()
    } finally {
      setMarkingAllRead(false)
    }
  }, [load, markingAllRead, t, unreadCount])

  useEffect(() => {
    if (!open || visibleItems.length === 0) return
    if (activeId && visibleItems.some((item) => item.id === activeId)) return
    const firstUnread = visibleItems.find((item) => !item.read_at)
    setActiveId((firstUnread || visibleItems[0]).id)
  }, [activeId, open, visibleItems])

  useEffect(() => {
    if (!open || !activeItem || activeItem.read_at) return
    void markRead(activeItem.id)
  }, [activeItem, markRead, open])

  return (
    <>
      <button
        type="button"
        className={`nova-icon-button relative flex items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${className}`}
        aria-label={t('messages.open')}
        title={t('messages.open')}
        onClick={() => setOpen(true)}
      >
        <Bell className="h-4 w-4" />
        {unreadCount > 0 && (
          <span className="absolute -right-1 -top-1 min-w-3.5 rounded-full bg-[var(--nova-danger-border)] px-0.5 text-center text-[8px] font-medium leading-3.5 text-white">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent
          side="right"
          style={{ width: 'min(920px, calc(100vw - 1rem))', maxWidth: 'none' }}
          className="gap-0 border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
        >
          <SheetHeader className="shrink-0 gap-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-4 py-3">
            <div className="flex items-start justify-between gap-3 pr-9">
              <div className="min-w-0">
                <SheetTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('messages.title')}</SheetTitle>
                <SheetDescription className="mt-1 text-xs text-[var(--nova-text-faint)]">
                  {t('messages.description')}
                </SheetDescription>
              </div>
              <button
                type="button"
                className="nova-ui-compact inline-flex shrink-0 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 text-xs text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-50"
                aria-label={t('messages.markAllRead')}
                title={t('messages.markAllRead')}
                disabled={unreadCount <= 0 || markingAllRead}
                onClick={markAllRead}
              >
                {markingAllRead ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckCheck className="h-3.5 w-3.5" />}
                <span>{t('messages.markAllRead')}</span>
              </button>
            </div>
          </SheetHeader>
          <div className="flex min-h-0 flex-1 flex-col md:flex-row">
            <div className="max-h-56 shrink-0 overflow-y-auto border-b border-[var(--nova-border)] md:max-h-none md:w-72 md:border-b-0 md:border-r">
              <div className="sticky top-0 z-10 flex gap-1 overflow-x-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
                {(['all', 'action', 'automation', 'product'] as const).map((itemFilter) => (
                  <button
                    key={itemFilter}
                    type="button"
                    aria-pressed={filter === itemFilter}
                    className={`shrink-0 rounded-[var(--nova-radius)] px-2 py-1 text-[10px] ${filter === itemFilter ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
                    onClick={() => setFilter(itemFilter)}
                  >
                    {t(`messages.filter.${itemFilter}`)}
                  </button>
                ))}
              </div>
              {loading && items.length === 0 ? (
                <div className="flex h-32 items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  {t('messages.loading')}
                </div>
              ) : visibleItems.length === 0 ? (
                <div className="flex h-32 items-center justify-center px-4 text-center text-xs text-[var(--nova-text-faint)]">
                  {error || t('messages.empty')}
                </div>
              ) : (
                <div className="p-2">
                  {visibleItems.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      className={`mb-1 flex w-full items-start gap-2 rounded-[var(--nova-radius)] px-2 py-2 text-left text-xs transition-colors hover:bg-[var(--nova-hover)] ${activeId === item.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`}
                      onClick={() => selectMessage(item.id)}
                    >
                      <span className={`mt-1 h-1.5 w-1.5 shrink-0 rounded-full ${item.read_at ? 'bg-transparent' : 'bg-[var(--nova-danger-border)]'}`} />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-medium text-[var(--nova-text)]">{messageTitle(item, t)}</span>
                        <span className="mt-1 line-clamp-2 block leading-4 text-[var(--nova-text-faint)]">{item.summary || t('messages.noSummary')}</span>
                        <span className="mt-1 block truncate text-[10px] text-[var(--nova-text-faint)]">{messageMeta(item, t)}</span>
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
              {activeItem ? (
                <article className="chat-agent-message min-w-0 text-[var(--nova-text-muted)]">
                  <div className="mb-4 border-b border-[var(--nova-border)] pb-3">
                    <h2 className="m-0 text-base font-semibold text-[var(--nova-text)]">{messageTitle(activeItem, t)}</h2>
                    <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{messageMeta(activeItem, t)}</div>
                  </div>
                  {activeItem.type === 'changelog' && <DonationPrompt />}
                  {activeItem.type === 'changelog' && <GitHubStarPrompt />}
                  {onOpenAutomation && activeItem.task_id && (
                    <button
                      type="button"
                      className="mb-4 inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 py-1.5 text-xs font-medium text-[var(--nova-text)] hover:bg-[var(--nova-hover)]"
                      onClick={() => {
                        onOpenAutomation({
                          taskId: activeItem.task_id || '',
                          runId: activeItem.run_id,
                          inboxId: activeItem.inbox_id,
                          workspace: activeItem.workspace,
                        })
                        setOpen(false)
                      }}
                    >
                      <ArrowUpRight className="h-3.5 w-3.5" />
                      {t(activeItem.action_required ? 'messages.openAutomationAction' : 'messages.openAutomation')}
                    </button>
                  )}
                  <MarkdownRenderer content={activeItem.body} />
                </article>
              ) : (
                <div className="flex h-full min-h-48 items-center justify-center text-xs text-[var(--nova-text-faint)]">
                  {error || t('messages.selectEmpty')}
                </div>
              )}
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </>
  )
}

function DonationPrompt() {
  const { t } = useTranslation()
  return (
    <section
      className="mb-4 flex flex-col gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--nova-surface-2)_88%,transparent)] p-3 text-xs leading-5 text-[var(--nova-text-muted)] shadow-sm backdrop-blur sm:flex-row sm:items-center sm:justify-between"
      aria-label={t('messages.donation.title')}
    >
      <div className="min-w-0">
        <div className="text-sm font-medium text-[var(--nova-text)]">{t('messages.donation.title')}</div>
        <p className="m-0 mt-1">{t('messages.donation.description')}</p>
      </div>
      <img
        src="/donate.png"
        alt={t('messages.donation.alt')}
        loading="lazy"
        className="h-auto max-h-24 w-auto max-w-[120px] shrink-0 self-center rounded-md border border-[var(--nova-border-soft)] bg-white p-1 sm:max-h-32"
      />
    </section>
  )
}

function GitHubStarPrompt() {
  const { t } = useTranslation()
  return (
    <section
      className="mb-4 flex flex-col gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--nova-surface-2)_88%,transparent)] p-3 text-xs leading-5 text-[var(--nova-text-muted)] shadow-sm backdrop-blur sm:flex-row sm:items-center sm:justify-between"
      aria-label={t('messages.github.title')}
    >
      <div className="min-w-0">
        <div className="text-sm font-medium text-[var(--nova-text)]">{t('messages.github.title')}</div>
        <p className="m-0 mt-1">{t('messages.github.description')}</p>
      </div>
      <a
        href={DENOVA_GITHUB_URL}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex shrink-0 items-center gap-1.5 self-start rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-1.5 text-xs font-medium text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] sm:self-center"
      >
        <Star className="h-3.5 w-3.5" />
        {t('messages.github.star')}
      </a>
    </section>
  )
}

function countUnread(items: ProductMessage[]) {
  return items.filter((item) => !item.read_at).length
}

function messageTitle(item: ProductMessage, t: (key: string, options?: Record<string, string>) => string) {
  if (item.type === 'changelog') {
    const label = item.title.toLowerCase() === 'unreleased' ? t('messages.unreleased') : item.title
    return t('messages.changelogTitle', { version: label })
  }
  return item.title
}

function messageMeta(item: ProductMessage, t: (key: string, options?: Record<string, string>) => string) {
  const parts = [messageTypeLabel(item, t)]
  const date = formatMessagePublishedAt(item.published_at)
  if (date) parts.push(date)
  return parts.join(' · ')
}

function messageTypeLabel(item: ProductMessage, t: (key: string, options?: Record<string, string>) => string) {
  if (item.type === 'changelog') return t('messages.type.changelog')
  if (item.type === 'automation_action') return t('messages.type.automationAction')
  if (item.type === 'automation') return t('messages.type.automation')
  return item.type
}

function messageMatchesFilter(item: ProductMessage, filter: MessageFilter) {
  if (filter === 'all') return true
  if (filter === 'action') return Boolean(item.action_required) || item.type === 'automation_action'
  if (filter === 'automation') return item.type === 'automation' || item.type === 'automation_action'
  return item.type !== 'automation' && item.type !== 'automation_action'
}

function formatMessagePublishedAt(value: string | undefined) {
  if (!value) return ''
  return formatDateTime(value)
}
