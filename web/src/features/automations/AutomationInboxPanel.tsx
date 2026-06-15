import { useMemo } from 'react'
import { Check, Inbox, MessageSquareText, Play, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AutomationInboxItem, AutomationTask } from '@/lib/api'

export function InboxPanel({
  items,
  tasks,
  onRead,
  onConfirm,
  onDismiss,
  onOpenRun,
}: {
  items: AutomationInboxItem[]
  tasks: AutomationTask[]
  onRead: (item: AutomationInboxItem) => void
  onConfirm: (item: AutomationInboxItem) => void
  onDismiss: (item: AutomationInboxItem) => void
  onOpenRun: (runId: string) => void
}) {
  const { t } = useTranslation()
  const taskNames = useMemo(() => new Map(tasks.map((task) => [task.id || '', task.name])), [tasks])
  if (items.length === 0) {
    return <div className="flex min-h-0 flex-1 items-center justify-center text-xs text-[var(--nova-text-faint)]">{t('automations.inbox.empty')}</div>
  }
  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
      <div className="mx-auto max-w-5xl space-y-3">
        {items.map((item) => (
          <div key={item.id} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 text-xs">
            <div className="flex items-start gap-3">
              <Inbox className={`mt-0.5 h-4 w-4 shrink-0 ${item.read_at ? 'text-[var(--nova-text-faint)]' : 'text-[var(--nova-text)]'}`} />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-[var(--nova-text)]">{item.title}</span>
                  <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{inboxPurposeLabel(item.purpose || 'trigger', t)}</span>
                  <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{inboxStatusLabel(item.status, t)}</span>
                  <span className="text-[11px] text-[var(--nova-text-faint)]">{taskNames.get(item.task_id) || item.task_id}</span>
                  <span className="ml-auto text-[11px] text-[var(--nova-text-faint)]">{new Date(item.created_at).toLocaleString()}</span>
                </div>
                <div className="mt-1 whitespace-pre-wrap leading-5 text-[var(--nova-text-muted)]">{item.summary}</div>
                {item.evidence.length > 0 && (
                  <div className="mt-2 space-y-1">
                    {item.evidence.slice(0, 3).map((evidence, index) => (
                      <div key={`${item.id}-${index}`} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
                        <span className="font-medium">{evidence.source}</span>
                        <span className="mx-1 text-[var(--nova-text-faint)]">·</span>
                        <span>{evidence.title}</span>
                        {evidence.ref && <span className="ml-1 text-[var(--nova-text-faint)]">{evidence.ref}</span>}
                        {evidence.snippet && <div className="mt-0.5 line-clamp-2 text-[var(--nova-text-faint)]">{evidence.snippet}</div>}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
            <div className="mt-3 flex flex-wrap justify-end gap-2">
              {!item.read_at && (
                <button type="button" onClick={() => onRead(item)} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-[var(--nova-text-muted)]">
                  <Check className="h-3.5 w-3.5" />
                  {t('automations.inbox.markRead')}
                </button>
              )}
              {item.run_id && (
                <button type="button" onClick={() => onOpenRun(item.run_id || '')} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-[var(--nova-text-muted)]">
                  <MessageSquareText className="h-3.5 w-3.5" />
                  {t('automations.runs.viewTimeline')}
                </button>
              )}
              {!item.run_id && item.source_run_id && (
                <button type="button" onClick={() => onOpenRun(item.source_run_id || '')} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-[var(--nova-text-muted)]">
                  <MessageSquareText className="h-3.5 w-3.5" />
                  {t('automations.inbox.viewSourceRun')}
                </button>
              )}
              {item.status === 'pending' && item.action_policy === 'confirm' && (
                <button type="button" onClick={() => onConfirm(item)} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-2 py-1 text-[var(--nova-text)]">
                  <Play className="h-3.5 w-3.5" />
                  {item.purpose === 'write_confirmation' ? t('automations.inbox.confirmWrite') : t('automations.inbox.confirmRun')}
                </button>
              )}
              {item.status === 'pending' && (
                <button type="button" onClick={() => onDismiss(item)} className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-[var(--nova-text-muted)]">
                  <X className="h-3.5 w-3.5" />
                  {t('automations.inbox.dismiss')}
                </button>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function inboxStatusLabel(status: AutomationInboxItem['status'], t: (key: string) => string) {
  return t(`automations.inbox.status.${status}`)
}

function inboxPurposeLabel(purpose: NonNullable<AutomationInboxItem['purpose']>, t: (key: string) => string) {
  return t(`automations.inbox.purpose.${purpose}`)
}
