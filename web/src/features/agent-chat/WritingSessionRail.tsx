import { useTranslation } from 'react-i18next'
import { formatDateTime } from '@/i18n'
import { cn } from '@/lib/utils'
import type { SessionSummary } from '@/lib/api'
import { formatCompactSessionTime, useSessionTimeNow } from '@/components/Chat/session-time'

interface WritingSessionRailProps {
  sessions: SessionSummary[]
  activeSessionId: string
  onSwitch: (sessionId: string) => void | Promise<void>
}

/** Persistent, readable navigation for conversations in the current Book. */
export function WritingSessionRail({
  sessions,
  activeSessionId,
  onSwitch,
}: WritingSessionRailProps) {
  const { t } = useTranslation()
  const now = useSessionTimeNow()

  return (
    <nav
      aria-label={t('chat.sessionRail.label')}
      className="flex h-full w-44 max-w-full shrink-0 flex-col overflow-hidden border-l border-[var(--nova-border)] bg-[var(--nova-surface-2)]"
    >
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-2">
        <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text-muted)]">
          {t('chat.view.sessions')}
        </span>
      </div>

      <div className="min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto">
        <div className="flex w-full min-w-0 flex-col gap-0.5 p-1.5">
          {sessions.map((session) => {
            const title = session.title || t('chat.untitledSession')
            const active = session.id === activeSessionId
            const timestamp = session.updated_at || session.created_at
            return (
              <button
                key={session.id}
                type="button"
                onClick={() => void onSwitch(session.id)}
                aria-current={active ? 'page' : undefined}
                aria-label={t('chat.sessionRail.switchWithStatus', {
                  title,
                  status: t(session.running ? 'chat.sessionRail.running' : 'chat.sessionRail.idle'),
                })}
                data-running={session.running ? 'true' : 'false'}
                title={title}
                className={cn(
                  'grid h-8 w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 overflow-hidden rounded-[var(--nova-radius)] px-2 text-left text-xs outline-none transition-colors focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]',
                  active
                    ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
                    : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
                )}
              >
                <span className="min-w-0 truncate">{title}</span>
                <time
                  dateTime={timestamp}
                  title={formatDateTime(timestamp)}
                  className="text-[10px] tabular-nums text-[var(--nova-text-faint)]"
                >
                  {formatCompactSessionTime(timestamp, now)}
                </time>
              </button>
            )
          })}
        </div>
      </div>
    </nav>
  )
}
