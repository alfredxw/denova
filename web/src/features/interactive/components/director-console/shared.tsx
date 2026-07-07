import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { safeJSONString } from './utils'

export function InfoLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
      <span className="truncate text-[var(--nova-text-faint)]" title={label}>{label}</span>
      <span className="min-w-0 break-words text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{value}</span>
    </div>
  )
}
export function SyncBadge({ status, error, loading }: { status?: string; error?: string; loading?: boolean }) {
  const { t } = useTranslation()
  if (loading || status === 'pending') {
    return (
      <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]">
        <Loader2 className="h-3 w-3 animate-spin" />
        {t('memoryPanel.syncing')}
      </span>
    )
  }
  if (status === 'failed') {
    return <span className="inline-flex max-w-[120px] shrink-0 truncate rounded-full border border-[var(--nova-danger)] bg-[var(--nova-danger-bg)] px-2 py-0.5 text-[11px] text-[var(--nova-danger)]" title={error}>{t('memoryPanel.failed')}</span>
  }
  return <span className="inline-flex shrink-0 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]">{t('memoryPanel.ready')}</span>
}

export function MemoryChip({ children }: { children: string }) {
  return <span className="max-w-full truncate rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]">{children}</span>
}

export function StateValue({ value }: { value: unknown }) {
  if (value === null || value === undefined || value === '') return <span className="text-xs text-[var(--nova-text-faint)]">-</span>
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return <p className="whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{String(value)}</p>
  }
  return <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-[var(--nova-radius)] bg-[var(--nova-surface-2)] p-2 text-[11px] leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{safeJSONString(value)}</pre>
}
