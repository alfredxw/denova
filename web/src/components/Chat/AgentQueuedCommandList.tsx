import { CornerDownRight, ListEnd, Loader2, MoreHorizontal, PenLine, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AgentRuntimeQueuedCommand } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

interface AgentQueuedCommandListProps {
  items: AgentRuntimeQueuedCommand[]
  pendingCommandID?: string
  disabled?: boolean
  onSteer?: (item: AgentRuntimeQueuedCommand) => boolean | void | Promise<boolean | void>
  onDelete?: (item: AgentRuntimeQueuedCommand) => boolean | void | Promise<boolean | void>
  onEdit?: (item: AgentRuntimeQueuedCommand) => boolean | void | Promise<boolean | void>
}

/** Compact controls for accepted input waiting directly above the composer. */
export function AgentQueuedCommandList({
  items,
  pendingCommandID = '',
  disabled = false,
  onSteer,
  onDelete,
  onEdit,
}: AgentQueuedCommandListProps) {
  const { t } = useTranslation()
  if (items.length === 0) return null

  return (
    <section className="mb-2 space-y-1.5" aria-label={t('chat.queue.label')}>
      {items.map((item) => {
        const actionPending = pendingCommandID === item.command_id
        const steering = item.delivery === 'steer' || item.steer_requested === true
        const controlsDisabled = disabled || actionPending || steering
        const canSteer = item.delivery === 'follow_up' && !steering && Boolean(onSteer)
        const canManage = item.delivery !== 'steer'
        const editDisabled = controlsDisabled || item.message_truncated === true
        return (
          <div
            key={item.command_id}
            className="flex min-w-0 items-center gap-2 rounded-[14px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 shadow-[0_8px_24px_-22px_rgba(0,0,0,0.72)]"
            aria-busy={actionPending || steering || undefined}
          >
            <div className="flex min-w-0 flex-1 items-center gap-2 px-1">
              <ListEnd className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
              <span className="line-clamp-2 min-w-0 text-xs leading-4 text-[var(--nova-text-muted)]" title={item.message}>
                {item.message}
              </span>
            </div>
            <div className="flex shrink-0 items-center gap-0.5">
              {steering ? (
                <span className="inline-flex h-8 items-center gap-1.5 px-1.5 text-[11px] text-[var(--nova-text-faint)]" aria-live="polite">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
                  {t('chat.queue.steering')}
                </span>
              ) : canSteer ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={controlsDisabled}
                  onClick={() => void onSteer?.(item)}
                  className="h-8 rounded-[9px] px-2 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                  aria-label={t('chat.queue.steer')}
                  title={t('chat.queue.steerDescription')}
                >
                  <CornerDownRight data-icon="inline-start" />
                  {t('chat.queue.steer')}
                </Button>
              ) : null}
              {canManage && onDelete ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={controlsDisabled}
                  onClick={() => void onDelete(item)}
                  className="h-8 w-8 rounded-[9px] text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
                  aria-label={t('chat.queue.delete')}
                  title={t('chat.queue.delete')}
                >
                  {actionPending ? <Loader2 className="animate-spin" /> : <Trash2 />}
                </Button>
              ) : null}
              {canManage && onEdit ? (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      disabled={controlsDisabled}
                      className="h-8 w-8 rounded-[9px] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                      aria-label={t('chat.queue.more')}
                      title={t('chat.queue.more')}
                    >
                      <MoreHorizontal />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" side="top" className="w-48 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
                    <DropdownMenuItem
                      disabled={editDisabled}
                      onSelect={() => void onEdit(item)}
                      className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                      title={item.message_truncated ? t('chat.queue.editTruncated') : undefined}
                    >
                      <PenLine />
                      {t('chat.queue.edit')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              ) : null}
            </div>
          </div>
        )
      })}
    </section>
  )
}
