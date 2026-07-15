import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

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
  if (loading || status === 'pending' || status === 'running') {
    return (
      <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]">
        <Loader2 className="h-3 w-3 animate-spin" />
        {t('directorPanel.syncing')}
      </span>
    )
  }
  if (status === 'failed') {
    return <span className="inline-flex max-w-[120px] shrink-0 truncate rounded-full border border-[var(--nova-danger)] bg-[var(--nova-danger-bg)] px-2 py-0.5 text-[11px] text-[var(--nova-danger)]" title={error}>{t('directorPanel.failed')}</span>
  }
  return <span className="inline-flex shrink-0 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]">{t('directorPanel.ready')}</span>
}

export function AuditChip({ children }: { children: string }) {
  return <span className="max-w-full truncate rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]">{children}</span>
}

export function StateValue({ value }: { value: unknown }) {
  const { t } = useTranslation()
  if (value === null || value === undefined || value === '') return <span className="text-xs text-[var(--nova-text-faint)]">—</span>
  if (typeof value === 'boolean') {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-[var(--nova-text-muted)]">
        <span className={`h-1.5 w-1.5 rounded-full ${value ? 'bg-[var(--nova-success)]' : 'bg-[var(--nova-text-faint)]'}`} />
        {value ? t('directorPanel.stateValue.yes') : t('directorPanel.stateValue.no')}
      </span>
    )
  }
  if (typeof value === 'number') return <span className="font-mono text-xs tabular-nums text-[var(--nova-text)]">{value}</span>
  if (typeof value === 'string') {
    return <p className="whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{value}</p>
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return <span className="text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateValue.empty')}</span>
    const simple = value.every((item) => item === null || ['string', 'number', 'boolean'].includes(typeof item))
    if (simple) {
      return (
        <div className="flex min-w-0 flex-wrap gap-1.5">
          {value.map((item, index) => (
            <span key={`${String(item)}-${index}`} className="max-w-full min-w-0 whitespace-normal break-words rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 text-[11px] leading-4 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">
              {item === null ? '—' : String(item)}
            </span>
          ))}
        </div>
      )
    }
    return (
      <ol className="space-y-1.5">
        {value.slice(0, 16).map((item, index) => (
          <li key={index} className="grid grid-cols-[18px_minmax(0,1fr)] gap-1.5 rounded-[8px] bg-[var(--nova-surface)] px-2 py-1.5">
            <span className="pt-0.5 font-mono text-[9px] text-[var(--nova-text-faint)]">{String(index + 1).padStart(2, '0')}</span>
            <StateValue value={item} />
          </li>
        ))}
      </ol>
    )
  }
  if (isRecord(value)) {
    const entries = Object.entries(value).filter(([, item]) => item !== undefined)
    if (entries.length === 0) return <span className="text-xs text-[var(--nova-text-faint)]">{t('directorPanel.stateValue.empty')}</span>
    return (
      <dl className="divide-y divide-[var(--nova-border-soft)] overflow-hidden rounded-[8px] border border-[var(--nova-border-soft)] bg-[var(--nova-surface)]">
        {entries.slice(0, 20).map(([key, item]) => (
          <div key={key} className="grid grid-cols-[minmax(72px,0.8fr)_minmax(0,1.2fr)] gap-2 px-2 py-1.5">
            <dt className="break-words text-[10px] leading-4 text-[var(--nova-text-faint)] [overflow-wrap:anywhere]">{humanizeStateKey(key)}</dt>
            <dd className="min-w-0"><StateValue value={item} /></dd>
          </div>
        ))}
      </dl>
    )
  }
  return <span className="break-words text-xs text-[var(--nova-text-muted)]">{String(value)}</span>
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function humanizeStateKey(value: string) {
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/^\w/, (letter) => letter.toUpperCase())
}
