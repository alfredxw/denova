import { useMemo, useState } from 'react'
import { ChevronDown, MessageCircleMore } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { formatDateTime } from '@/i18n'
import { cn } from '@/lib/utils'
import type { SessionSummary } from '@/lib/api'
import { formatCompactSessionTime, useSessionTimeNow } from './session-time'

interface SessionHistoryPopoverProps {
  sessions: SessionSummary[]
  activeSessionId: string
  active?: boolean
  disabled?: boolean
  onSwitch: (id: string) => void | Promise<void>
  onManage: () => void
}

/** Search-first access to recent conversations without replacing the current chat. */
export function SessionHistoryPopover({
  sessions,
  activeSessionId,
  active = false,
  disabled = false,
  onSwitch,
  onManage,
}: SessionHistoryPopoverProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [switchingId, setSwitchingId] = useState('')
  const now = useSessionTimeNow(open)
  const orderedSessions = useMemo(
    () => [...sessions].sort((left, right) => sessionTimestamp(right) - sessionTimestamp(left)),
    [sessions],
  )

  const switchSession = async (sessionId: string) => {
    if (!sessionId || switchingId) return
    if (sessionId === activeSessionId) {
      setOpen(false)
      return
    }
    setOpen(false)
    setSwitchingId(sessionId)
    try {
      await onSwitch(sessionId)
    } catch (error) {
      console.error('[components/Chat/SessionHistoryPopover.tsx] failed to switch chat session', {
        sessionID: sessionId,
        error,
      })
    } finally {
      setSwitchingId('')
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={t('chat.sessionHistory.open')}
          className={cn(
            'flex h-6 items-center gap-1 rounded-[6px] px-2 text-[11px] outline-none transition-colors focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]',
            active || open
              ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
              : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]',
          )}
        >
          {t('chat.view.sessions')}
          <ChevronDown aria-hidden="true" className="size-3" />
        </button>
      </PopoverTrigger>

      <PopoverContent align="start" sideOffset={5} className="w-[min(22rem,calc(100vw-1rem))] gap-0 p-0 data-closed:hidden">
        <Command label={t('chat.searchSession')}>
          <CommandInput autoFocus placeholder={t('chat.searchSessionPlaceholder')} aria-label={t('chat.searchSession')} />
          <CommandList className="max-h-[min(24rem,50vh)]">
            <CommandEmpty>{t('chat.noMatchedSession')}</CommandEmpty>
            <CommandGroup heading={t('chat.sessionHistory.all')}>
              {orderedSessions.map((session) => {
                const title = session.title || t('chat.untitledSession')
                const activeSession = session.id === activeSessionId
                const timestamp = session.updated_at || session.created_at
                return (
                  <CommandItem
                    key={session.id}
                    value={session.id}
                    keywords={[title]}
                    disabled={disabled || Boolean(switchingId)}
                    data-checked={activeSession ? 'true' : undefined}
                    onSelect={() => void switchSession(session.id)}
                    aria-label={t('chat.sessionRail.switch', { title })}
                    className="h-9"
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        'size-1.5 shrink-0 rounded-full',
                        session.running ? 'bg-[var(--nova-success)]' : 'bg-transparent',
                      )}
                    />
                    <span className="min-w-0 flex-1 truncate">{title}</span>
                    <time
                      dateTime={timestamp}
                      title={formatDateTime(timestamp)}
                      className="shrink-0 text-xs tabular-nums text-muted-foreground"
                    >
                      {formatCompactSessionTime(timestamp, now)}
                    </time>
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </CommandList>
        </Command>
        <Separator />
        <div className="p-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="w-full justify-start"
            onClick={() => {
              setOpen(false)
              onManage()
            }}
          >
            <MessageCircleMore data-icon="inline-start" />
            {t('chat.sessionRail.manage')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function sessionTimestamp(session: SessionSummary): number {
  const timestamp = Date.parse(session.updated_at || session.created_at || '')
  return Number.isNaN(timestamp) ? 0 : timestamp
}
