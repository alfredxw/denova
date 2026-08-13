import { useEffect, useState } from 'react'
import { Check, Clipboard, PanelRightClose } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { TrajectoryEventRecord, TrajectorySpan } from './trajectory-analysis'
import { formatTrajectoryDuration } from './trajectory-analysis'

interface TrajectoryInspectorProps {
  span: TrajectorySpan | null
  event: TrajectoryEventRecord | null
  onClose: () => void
}

/** Local inspector for timing, usage, bounded attributes, and the persisted raw record. */
export function TrajectoryInspector({ span, event, onClose }: TrajectoryInspectorProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const record = span?.record ?? event?.record ?? null
  useEffect(() => setCopied(false), [record])
  if (!record) {
    return (
      <aside className="hidden min-h-0 w-[min(34vw,440px)] shrink-0 place-items-center bg-[var(--nova-surface-2)] px-6 text-center text-[11px] text-[var(--nova-text-faint)] lg:grid">
        {t('trajectory.inspector.empty')}
      </aside>
    )
  }

  const copyRecord = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(record, null, 2))
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch (error) {
      console.warn('[TrajectoryInspector.tsx] failed to copy trace record', error)
    }
  }

  return (
    <aside className="absolute inset-0 z-30 flex min-h-0 w-full min-w-0 shrink-0 flex-col bg-[var(--nova-surface-2)] lg:static lg:w-[min(38vw,480px)] lg:min-w-[320px]" aria-label={t('trajectory.inspector.title')}>
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
        <div className="min-w-0 flex-1">
          <div className="truncate text-[11px] font-medium text-[var(--nova-text)]">{span?.label ?? event?.label}</div>
          <div className="truncate font-mono text-[9px] text-[var(--nova-text-faint)]">{record.type}</div>
        </div>
        <Button type="button" size="icon-xs" variant="ghost" onClick={() => void copyRecord()} aria-label={t('trajectory.inspector.copy')}>
          {copied ? <Check className="text-[var(--nova-success)]" /> : <Clipboard />}
        </Button>
        <Button type="button" size="icon-xs" variant="ghost" onClick={onClose} aria-label={t('trajectory.inspector.close')}>
          <PanelRightClose />
        </Button>
      </div>
      <Tabs defaultValue="summary" className="min-h-0 flex-1 gap-0">
        <TabsList variant="line" className="mx-3 h-9 shrink-0">
          <TabsTrigger value="summary" className="text-[10px]">{t('trajectory.inspector.summary')}</TabsTrigger>
          <TabsTrigger value="timing" className="text-[10px]">{t('trajectory.inspector.timing')}</TabsTrigger>
          <TabsTrigger value="usage" className="text-[10px]">{t('trajectory.inspector.usage')}</TabsTrigger>
          <TabsTrigger value="data" className="text-[10px]">{t('trajectory.inspector.data')}</TabsTrigger>
          <TabsTrigger value="raw" className="text-[10px]">{t('trajectory.inspector.raw')}</TabsTrigger>
        </TabsList>
        <TabsContent value="summary" className="min-h-0 overflow-auto border-t border-[var(--nova-border)] p-3">
          <DefinitionList items={span ? [
            [t('trajectory.field.kind'), span.category],
            [t('trajectory.field.status'), span.status],
            [t('trajectory.field.spanId'), span.id],
            [t('trajectory.field.parentSpanId'), span.parentId || '—'],
            [t('trajectory.field.children'), String(span.children.length)],
            [t('trajectory.field.recordIndex'), `#${span.recordIndex + 1}`],
          ] : [
            [t('trajectory.field.kind'), event?.category ?? 'event'],
            [t('trajectory.field.status'), event?.status ?? 'recorded'],
            [t('trajectory.field.recordIndex'), `#${(event?.recordIndex ?? 0) + 1}`],
          ]} />
        </TabsContent>
        <TabsContent value="timing" className="min-h-0 overflow-auto border-t border-[var(--nova-border)] p-3">
          <DefinitionList items={span ? [
            [t('trajectory.field.started'), formatExactTime(span.startedAt)],
            [t('trajectory.field.ended'), formatExactTime(span.endedAt)],
            [t('trajectory.field.duration'), formatTrajectoryDuration(span.durationMs)],
            [t('trajectory.field.waitBefore'), formatTrajectoryDuration(span.gapBeforeMs)],
            ['TTFT', formatTrajectoryDuration(span.ttftMs)],
            [t('trajectory.field.generation'), formatTrajectoryDuration(span.generationMs)],
            [t('trajectory.field.throughput'), throughput(span)],
          ] : [
            [t('trajectory.field.recorded'), formatExactTime(event?.timestamp ?? 0)],
          ]} />
          {span?.category === 'model' && span.ttftMs !== null && (
            <div className="mt-4 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
              <div className="flex h-2">
                <span className="bg-[var(--nova-warning)]" style={{ width: `${Math.min(100, span.ttftMs / Math.max(1, span.durationMs) * 100)}%` }} />
                <span className="flex-1 bg-[var(--nova-text-muted)]" />
              </div>
              <div className="flex justify-between px-2 py-1.5 text-[9px] text-[var(--nova-text-faint)]">
                <span>TTFT {formatTrajectoryDuration(span.ttftMs)}</span>
                <span>{t('trajectory.field.generation')} {formatTrajectoryDuration(span.generationMs)}</span>
              </div>
            </div>
          )}
        </TabsContent>
        <TabsContent value="usage" className="min-h-0 overflow-auto border-t border-[var(--nova-border)] p-3">
          {span ? (
            <DefinitionList items={[
              [t('trajectory.field.promptTokens'), formatNumber(span.inputTokens)],
              [t('trajectory.field.cachedTokens'), formatNumber(span.cachedTokens)],
              [t('trajectory.field.uncachedTokens'), formatNumber(Math.max(0, span.inputTokens - span.cachedTokens))],
              [t('trajectory.field.completionTokens'), formatNumber(span.outputTokens)],
              [t('trajectory.field.reasoningTokens'), formatNumber(span.reasoningTokens)],
              [t('trajectory.field.cacheHitRate'), span.inputTokens > 0 ? `${(span.cachedTokens / span.inputTokens * 100).toFixed(1)}%` : '—'],
            ]} />
          ) : <EmptyTab />}
        </TabsContent>
        <TabsContent value="data" className="min-h-0 overflow-auto border-t border-[var(--nova-border)] p-3">
          <JSONBlock value={span?.attrs ?? event?.data ?? {}} />
        </TabsContent>
        <TabsContent value="raw" className="min-h-0 overflow-auto border-t border-[var(--nova-border)] p-3">
          <JSONBlock value={record} />
        </TabsContent>
      </Tabs>
    </aside>
  )
}

function DefinitionList({ items }: { items: Array<readonly [string, string]> }) {
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

function JSONBlock({ value }: { value: unknown }) {
  return <pre className="overflow-auto whitespace-pre-wrap break-words rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 font-mono text-[10px] leading-5 text-[var(--nova-text-muted)]">{JSON.stringify(value, null, 2)}</pre>
}

function EmptyTab() {
  const { t } = useTranslation()
  return <div className="py-8 text-center text-[11px] text-[var(--nova-text-faint)]">{t('trajectory.inspector.notAvailable')}</div>
}

function formatExactTime(timestamp: number) {
  if (!timestamp) return '—'
  const date = new Date(timestamp)
  const milliseconds = String(date.getMilliseconds()).padStart(3, '0')
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString([], { hour12: false })}.${milliseconds}`
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}

function throughput(span: TrajectorySpan) {
  if (span.generationMs === null || span.generationMs <= 0 || span.outputTokens <= 0) return '—'
  return `${(span.outputTokens / (span.generationMs / 1_000)).toFixed(1)} tok/s`
}
