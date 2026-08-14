import { useTranslation } from 'react-i18next'
import type { TrajectorySpan } from './trajectory-analysis'

export function TrajectoryDefinitionList({ items }: { items: Array<readonly [string, string]> }) {
  return (
    <dl className="divide-y divide-[var(--nova-border-soft)] overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
      {items.map(([label, value]) => (
        <div key={label} className="grid grid-cols-[minmax(100px,38%)_minmax(0,1fr)] gap-3 px-3 py-2">
          <dt className="text-[10px] text-[var(--nova-text-faint)]">{label}</dt>
          <dd className="break-words text-right font-mono text-[10px] text-[var(--nova-text)]">{value || '—'}</dd>
        </div>
      ))}
    </dl>
  )
}

export function TrajectoryJSONBlock({ value, className = '' }: { value: unknown; className?: string }) {
  return <pre className={`overflow-auto whitespace-pre-wrap break-words rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 font-mono text-[10px] leading-5 text-[var(--nova-text-muted)] ${className}`}>{JSON.stringify(value, null, 2)}</pre>
}

export function EmptyInspectorTab() {
  const { t } = useTranslation()
  return <div className="py-8 text-center text-[11px] text-[var(--nova-text-faint)]">{t('trajectory.inspector.notAvailable')}</div>
}

export function formatExactTrajectoryTime(timestamp: number) {
  if (!timestamp) return '—'
  const date = new Date(timestamp)
  const milliseconds = String(date.getMilliseconds()).padStart(3, '0')
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString([], { hour12: false })}.${milliseconds}`
}

export function formatTrajectoryNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}

export function trajectoryThroughput(span: TrajectorySpan) {
  if (span.generationMs === null || span.generationMs <= 0 || span.outputTokens <= 0) return '—'
  return `${(span.outputTokens / (span.generationMs / 1_000)).toFixed(1)} tok/s`
}
