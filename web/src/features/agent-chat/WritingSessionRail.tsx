import { MessageSquareText, MessagesSquare, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { formatDateTime } from '@/i18n'
import { cn } from '@/lib/utils'
import type { SessionSummary } from '@/lib/api'

interface WritingSessionRailProps {
  sessions: SessionSummary[]
  activeSessionId: string
  pending?: boolean
  onCreate: () => void | Promise<void>
  onSwitch: (sessionId: string) => void | Promise<void>
  onManage: () => void
}

/** Compact, status-aware navigation for conversations in the current Book. */
export function WritingSessionRail({
  sessions,
  activeSessionId,
  pending = false,
  onCreate,
  onSwitch,
  onManage,
}: WritingSessionRailProps) {
  const { t } = useTranslation()

  return (
    <nav
      aria-label={t('chat.sessionRail.label')}
      className="hidden h-full w-11 shrink-0 flex-col border-l border-[var(--nova-border)] bg-[var(--nova-surface-2)] md:flex"
    >
      <div className="flex h-9 shrink-0 items-center justify-center border-b border-[var(--nova-border)]">
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          disabled={pending}
          onClick={() => void onCreate()}
          aria-label={t('chat.newSession')}
        >
          <Plus />
        </Button>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col items-center gap-1 px-1.5 py-2">
          {sessions.map((session) => {
            const title = session.title || t('chat.untitledSession')
            const active = session.id === activeSessionId
            return (
              <HoverCard key={session.id} openDelay={260} closeDelay={100}>
                <HoverCardTrigger asChild>
                  <button
                    type="button"
                    onClick={() => void onSwitch(session.id)}
                    aria-current={active ? 'page' : undefined}
                    aria-label={t('chat.sessionRail.switch', { title })}
                    data-running={session.running ? 'true' : 'false'}
                    className={cn(
                      'relative flex size-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border text-[10px] font-semibold tracking-[-0.03em] outline-none transition-colors focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]',
                      active
                        ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] shadow-sm'
                        : 'border-transparent text-[var(--nova-text-muted)] hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
                    )}
                  >
                    <span aria-hidden="true" className="whitespace-nowrap">{sessionMonogram(title)}</span>
                    {active ? <span aria-hidden="true" className="absolute inset-y-1 left-0 w-0.5 rounded-r bg-[var(--nova-text)]" /> : null}
                    {session.running ? (
                      <span aria-hidden="true" className="absolute right-0.5 top-0.5 size-1.5 rounded-full bg-[var(--nova-success)] ring-1 ring-[var(--nova-surface-2)]" />
                    ) : null}
                  </button>
                </HoverCardTrigger>
                <HoverCardContent
                  side="left"
                  align="start"
                  sideOffset={8}
                  className="w-60 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
                >
                  <div className="flex items-start gap-2.5">
                    <MessageSquareText className="mt-0.5 size-4 shrink-0 text-[var(--nova-text-muted)]" />
                    <div className="min-w-0 flex-1">
                      <div className="line-clamp-2 text-xs font-medium leading-4">{title}</div>
                      <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[10px] text-[var(--nova-text-faint)]">
                        <span className={session.running ? 'text-[var(--nova-success)]' : undefined}>
                          {t(session.running ? 'chat.sessionRail.running' : 'chat.sessionRail.idle')}
                        </span>
                        <span>{t('common.messages', { count: session.message_count })}</span>
                        <span>{formatDateTime(session.updated_at || session.created_at) || t('chat.unknownTime')}</span>
                      </div>
                    </div>
                  </div>
                </HoverCardContent>
              </HoverCard>
            )
          })}
        </div>
      </ScrollArea>

      <div className="flex h-10 shrink-0 items-center justify-center border-t border-[var(--nova-border)]">
        <Button type="button" variant="ghost" size="icon-xs" onClick={onManage} aria-label={t('chat.sessionRail.manage')}>
          <MessagesSquare />
        </Button>
      </div>
    </nav>
  )
}

function sessionMonogram(title: string) {
  const compact = Array.from(title.trim().replace(/\s+/g, ''))
  return compact.slice(0, 2).join('').toLocaleUpperCase() || '·'
}
