import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeft, ArrowUpRight, Bell, CheckCheck, CircleAlert, Clock3, Loader2, Sparkles, Star, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Sheet, SheetClose, SheetTrigger, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useIsMobile } from '@/hooks/useIsMobile'
import { formatDateTime } from '@/i18n'
import { DENOVA_GITHUB_URL } from '@/lib/product-links'
import { cn } from '@/lib/utils'
import { getMessages, markAllMessagesRead, markMessageRead } from './api'
import type { AutomationMessageNavigation, ProductMessage } from './types'

type MessageFilter = 'all' | 'action' | 'automation' | 'product'

interface MessageCenterButtonProps {
  className?: string
  showLabel?: boolean
  unreadCount?: number
  onUnreadCountChange?: (count: number) => void
  onOpenAutomation?: (target: AutomationMessageNavigation) => void
}

export function MessageCenterButton({ className = '', showLabel = false, unreadCount: reportedUnreadCount = 0, onUnreadCountChange, onOpenAutomation }: MessageCenterButtonProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const listRef = useRef<HTMLDivElement>(null)
  const detailRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const [items, setItems] = useState<ProductMessage[]>([])
  const [unreadCount, setUnreadCount] = useState(reportedUnreadCount)
  const [activeId, setActiveId] = useState<string | null>(null)
  const [filter, setFilter] = useState<MessageFilter>('all')
  const [loading, setLoading] = useState(false)
  const [markingAllRead, setMarkingAllRead] = useState(false)
  const [error, setError] = useState('')
  const pendingReadRef = useRef<Set<string>>(new Set())

  const activeItem = useMemo(() => items.find((item) => item.id === activeId) || null, [activeId, items])
  const visibleItems = useMemo(() => items.filter((item) => messageMatchesFilter(item, filter)), [filter, items])
  const updateUnreadCount = useCallback((count: number) => {
    const normalized = Math.max(0, count)
    setUnreadCount(normalized)
    onUnreadCountChange?.(normalized)
  }, [onUnreadCountChange])

  useEffect(() => {
    setUnreadCount(reportedUnreadCount)
  }, [reportedUnreadCount])

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const result = await getMessages()
      const nextItems = result.items || []
      setItems(nextItems)
      updateUnreadCount(result.unread_count ?? countUnread(nextItems))
      setActiveId((current) => current && nextItems.some((item) => item.id === current) ? current : null)
    } catch (e) {
      console.warn('[features/messages/MessageCenter.tsx] loading messages failed', { error: e })
      setError(t('messages.loadFailed'))
      setItems([])
    } finally {
      setLoading(false)
    }
  }, [t, updateUnreadCount])

  useEffect(() => {
    if (open) void load()
  }, [load, open])

  const selectMessage = useCallback((id: string) => {
    setActiveId(id)
  }, [])

  const markRead = useCallback(async (id: string) => {
    if (pendingReadRef.current.has(id)) return
    pendingReadRef.current.add(id)
    const optimisticReadAt = new Date().toISOString()
    const wasUnread = items.some((item) => item.id === id && !item.read_at)
    setItems((current) => current.map((item) => item.id === id && !item.read_at ? { ...item, read_at: optimisticReadAt } : item))
    if (wasUnread) updateUnreadCount(unreadCount - 1)
    try {
      const updated = await markMessageRead(id)
      setItems((current) => current.map((item) => item.id === id ? { ...item, ...updated } : item))
    } catch (e) {
      console.warn('[features/messages/MessageCenter.tsx] marking message as read failed', { id, error: e })
      setError(t('messages.readFailed'))
      void load()
    } finally {
      pendingReadRef.current.delete(id)
    }
  }, [items, load, t, unreadCount, updateUnreadCount])

  const markAllRead = useCallback(async () => {
    if (unreadCount <= 0 || markingAllRead) return
    setMarkingAllRead(true)
    setError('')
    const optimisticReadAt = new Date().toISOString()
    setItems((current) => current.map((item) => item.read_at ? item : { ...item, read_at: optimisticReadAt }))
    updateUnreadCount(0)
    try {
      const result = await markAllMessagesRead()
      const nextItems = result.items || []
      setItems(nextItems)
      updateUnreadCount(result.unread_count ?? countUnread(nextItems))
      setActiveId((current) => current && nextItems.some((item) => item.id === current) ? current : null)
    } catch (e) {
      console.warn('[features/messages/MessageCenter.tsx] marking all messages as read failed', { error: e })
      setError(t('messages.readFailed'))
      void load()
    } finally {
      setMarkingAllRead(false)
    }
  }, [load, markingAllRead, t, unreadCount, updateUnreadCount])

  useEffect(() => {
    if (isMobile || !open || visibleItems.length === 0) return
    if (activeId && visibleItems.some((item) => item.id === activeId)) return
    const firstUnread = visibleItems.find((item) => !item.read_at)
    setActiveId((firstUnread || visibleItems[0]).id)
  }, [activeId, isMobile, open, visibleItems])

  useEffect(() => {
    if (!open || !activeItem || activeItem.read_at) return
    void markRead(activeItem.id)
  }, [activeItem, markRead, open])

  useEffect(() => {
    detailRef.current?.scrollTo({ top: 0 })
    if (isMobile && activeId) detailRef.current?.focus()
  }, [activeId, isMobile])

  const showDetail = isMobile && !!activeItem

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild><button
        type="button"
        className={cn(
          'nova-icon-button relative flex items-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
          showLabel
            ? '!h-9 !w-full !min-w-0 justify-start gap-2 px-2 text-xs font-medium'
            : 'justify-center',
          className,
        )}
        aria-label={t('messages.open')}
        onClick={() => {
          // Opening the inbox must not read a message the phone has not displayed.
          if (isMobile) setActiveId(null)
        }}
      >
        <span className="relative flex size-4 shrink-0 items-center justify-center">
          <Bell className="size-4" />
          {unreadCount > 0 && (
            <span className="absolute -right-2 -top-2 min-w-3.5 rounded-full bg-[var(--nova-danger-border)] px-0.5 text-center text-[8px] font-medium leading-3.5 text-white">
              {unreadCount > 99 ? '99+' : unreadCount}
            </span>
          )}
        </span>
        {showLabel ? <span className="truncate">{t('messages.label')}</span> : null}
      </button></SheetTrigger>
        <SheetContent
          side="right"
          showCloseButton={false}
          style={{ width: isMobile ? '100%' : 'min(920px, calc(100vw - 1rem))', maxWidth: 'none' }}
          className="nova-message-center gap-0 bg-background p-0 text-foreground"
        >
          <SheetHeader className="shrink-0 gap-0 border-b px-4 py-3 max-lg:px-2 max-lg:py-0.5">
            <div className="flex min-h-11 items-center gap-2">
              {showDetail && <Button variant="ghost" size="icon" aria-label={t('messages.back')} onClick={() => {
                setActiveId(null)
                requestAnimationFrame(() => Array.from(listRef.current?.querySelectorAll<HTMLElement>('[data-message-id]') ?? []).find((node) => node.dataset.messageId === activeId)?.focus())
              }}><ArrowLeft /></Button>}
              <div className="min-w-0 flex-1">
                <SheetTitle className="text-base">{t('messages.title')}</SheetTitle>
                <SheetDescription className="mt-1 text-xs max-lg:sr-only">{t('messages.description')}</SheetDescription>
              </div>
              {!showDetail && <Button variant="ghost" size={isMobile ? 'icon' : 'sm'} aria-label={t('messages.markAllRead')} disabled={unreadCount <= 0 || markingAllRead} onClick={markAllRead}>
                {markingAllRead ? <Loader2 className="animate-spin" /> : <CheckCheck />}
                {!isMobile && t('messages.markAllRead')}
              </Button>}
              <SheetClose asChild><Button variant="ghost" size="icon" aria-label={t('common.close')}><X /></Button></SheetClose>
            </div>
          </SheetHeader>
          {error && <div role="alert" className="flex items-center justify-between gap-2 border-b px-4 py-2 text-sm text-destructive">
            <span>{error}</span><Button variant="ghost" size="sm" onClick={() => void load()}>{t('common.retry')}</Button>
          </div>}
          <div className="flex min-h-0 flex-1">
            <div ref={listRef} hidden={showDetail} className={cn('min-h-0 flex-1 lg:w-72 lg:flex-none lg:border-r', showDetail && 'hidden')}>
              <Tabs value={filter} onValueChange={(value) => setFilter(value as MessageFilter)} className="h-full min-h-0 gap-0">
                <TabsList variant="line" aria-label={t('messages.filters')} className="!h-12 w-full shrink-0 justify-start rounded-none border-b px-2 py-1">
                  {(['all', 'action', 'automation', 'product'] as const).map((itemFilter) => (
                    <TabsTrigger key={itemFilter} value={itemFilter} className="h-11 px-2 text-xs after:!bottom-0">
                      {t(`messages.filter.${itemFilter}`)}
                    </TabsTrigger>
                  ))}
                </TabsList>
                <TabsContent value={filter} className="min-h-0 overflow-y-auto overscroll-contain">
                  {loading && items.length === 0 ? (
                    <div role="status" className="flex h-40 items-center justify-center gap-2 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />{t('messages.loading')}</div>
                  ) : visibleItems.length === 0 ? (
                    <Empty className="h-full"><EmptyHeader><EmptyMedia variant="icon"><Bell /></EmptyMedia><EmptyTitle>{t('messages.empty')}</EmptyTitle></EmptyHeader></Empty>
                  ) : <div className="divide-y px-3">
                    {visibleItems.map((item) => {
                      const Icon = item.action_required ? CircleAlert : item.type === 'automation' ? Clock3 : Sparkles
                      return <button
                        key={item.id}
                        data-message-id={item.id}
                        type="button"
                        aria-current={activeId === item.id ? 'true' : undefined}
                        className={cn('flex w-full items-start gap-3 rounded-md px-2 py-4 text-left transition-colors hover:bg-accent focus-visible:outline-ring', activeId === item.id && 'bg-accent')}
                        onClick={() => selectMessage(item.id)}
                      >
                        <span className={cn('mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground', item.action_required && 'bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]')}><Icon className="size-4" /></span>
                        <span className="min-w-0 flex-1">
                          <span className="flex items-center gap-2"><span className="line-clamp-2 flex-1 text-sm font-semibold leading-5">{messageTitle(item, t)}</span>{!item.read_at && <span className="size-2 shrink-0 rounded-full bg-primary"><span className="sr-only">{t('messages.unread')}</span></span>}</span>
                          {item.summary && <span className="mt-1 line-clamp-2 text-[13px] leading-5 text-muted-foreground">{item.summary}</span>}
                          <span className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"><Badge variant="secondary" className="text-[11px]">{messageTypeLabel(item, t)}</Badge><span>{formatMessagePublishedAt(item.published_at)}</span></span>
                        </span>
                      </button>
                    })}
                  </div>}
                </TabsContent>
              </Tabs>
            </div>
            <div ref={detailRef} tabIndex={-1} hidden={isMobile && !showDetail} className={cn('min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-5 py-5 outline-none', isMobile && !showDetail && 'hidden')}>
              {activeItem ? (
                <article className="chat-agent-message min-w-0 text-muted-foreground">
                  <div className="mb-5 border-b pb-4">
                    <h2 className="m-0 break-words text-lg font-semibold leading-7 text-foreground">{messageTitle(activeItem, t)}</h2>
                    <div className="mt-2 text-xs text-muted-foreground">{messageMeta(activeItem, t)}</div>
                  </div>
                  {onOpenAutomation && activeItem.task_id && (
                    <Button variant="secondary" className="mb-4" onClick={() => {
                      onOpenAutomation({ taskId: activeItem.task_id || '', runId: activeItem.run_id, inboxId: activeItem.inbox_id, projectId: activeItem.project_id, workspace: activeItem.workspace })
                      setOpen(false)
                    }}><ArrowUpRight />{t(activeItem.action_required ? 'messages.openAutomationAction' : 'messages.openAutomation')}</Button>
                  )}
                  <MarkdownRenderer content={activeItem.body} />
                  {activeItem.type === 'changelog' && <div className="mt-8 border-t pt-5"><GitHubStarPrompt /><DonationPrompt /></div>}
                </article>
              ) : <Empty className="h-full"><EmptyHeader><EmptyMedia variant="icon"><Bell /></EmptyMedia><EmptyTitle>{t('messages.selectEmpty')}</EmptyTitle></EmptyHeader></Empty>}
            </div>
          </div>
        </SheetContent>
    </Sheet>
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
