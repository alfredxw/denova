import { useEffect, useMemo, useState } from 'react'
import { Check, CircleAlert, FileClock, ListChecks, MessagesSquare, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { formatDateTime } from '@/i18n'
import { getHarnessTrajectories, getHarnessTrajectory } from '@/lib/api'
import type { HarnessTrajectorySummary } from '@/lib/api'

type RangeValue = '1d' | '7d' | '30d' | 'all'

export function HarnessTrajectoryPanel({
  refreshToken = 0,
  onEvidenceChange,
}: {
  refreshToken?: number
  onEvidenceChange: (uris: string[], ready: boolean) => void
}) {
  const { t } = useTranslation()
  const [range, setRange] = useState<RangeValue>('1d')
  const [items, setItems] = useState<HarnessTrajectorySummary[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [activeURI, setActiveURI] = useState('')
  const [detail, setDetail] = useState('')
  const [issues, setIssues] = useState(0)
  const [loading, setLoading] = useState(true)
  const [catalogReady, setCatalogReady] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setCatalogReady(false)
    setError(null)
    onEvidenceChange([], false)
    void getHarnessTrajectories(sinceForRange(range))
      .then((result) => {
        if (cancelled) return
        setItems(result.items)
        setIssues(result.issues?.length ?? 0)
        const nextSelected = new Set(result.items.map((item) => item.uri))
        setSelected(nextSelected)
        setActiveURI((current) => result.items.some((item) => item.uri === current) ? current : result.items[0]?.uri ?? '')
        setCatalogReady(true)
      })
      .catch((cause) => {
        if (cancelled) return
        setItems([])
        setSelected(new Set())
        setActiveURI('')
        setDetail('')
        setIssues(0)
        setError(errorMessage(cause))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [onEvidenceChange, range, refreshToken])

  useEffect(() => {
    if (!catalogReady) return
    onEvidenceChange(items.filter((item) => selected.has(item.uri)).map((item) => item.uri), true)
  }, [catalogReady, items, onEvidenceChange, selected])

  useEffect(() => {
    let cancelled = false
    if (!activeURI) {
      setDetail('')
      return () => { cancelled = true }
    }
    setDetailLoading(true)
    setError(null)
    void getHarnessTrajectory(activeURI)
      .then((result) => {
        if (!cancelled) setDetail(result.content)
      })
      .catch((cause) => {
        if (!cancelled) setError(errorMessage(cause))
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false)
      })
    return () => { cancelled = true }
  }, [activeURI])

  const selectedCount = useMemo(() => items.reduce((count, item) => count + Number(selected.has(item.uri)), 0), [items, selected])
  const toggleEvidence = (uri: string) => {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(uri)) next.delete(uri)
      else next.add(uri)
      return next
    })
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-11 shrink-0 flex-wrap items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2 sm:px-4">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <ListChecks className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
          <span className="text-[11px] font-medium text-[var(--nova-text)]">{t('continualLearning.trajectories.analysisBasis')}</span>
          <Badge variant="outline" className="h-5 px-1.5 text-[9px]">
            {t('continualLearning.trajectories.selected', { selected: selectedCount, total: items.length })}
          </Badge>
        </div>
        <Select value={range} onValueChange={(value) => setRange(value as RangeValue)}>
          <SelectTrigger size="sm" className="w-32" aria-label={t('continualLearning.trajectories.range')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="end">
            <SelectItem value="1d">{t('continualLearning.trajectories.range1d')}</SelectItem>
            <SelectItem value="7d">{t('continualLearning.trajectories.range7d')}</SelectItem>
            <SelectItem value="30d">{t('continualLearning.trajectories.range30d')}</SelectItem>
            <SelectItem value="all">{t('continualLearning.trajectories.rangeAll')}</SelectItem>
          </SelectContent>
        </Select>
        <Button type="button" size="xs" variant="ghost" disabled={items.length === 0} onClick={() => setSelected(new Set(items.map((item) => item.uri)))}>
          {t('continualLearning.trajectories.selectAll')}
        </Button>
        <Button type="button" size="xs" variant="ghost" disabled={selectedCount === 0} onClick={() => setSelected(new Set())}>
          {t('continualLearning.trajectories.clear')}
        </Button>
      </div>
      {error && <div className="shrink-0 border-b border-[var(--nova-border)] bg-red-500/5 px-4 py-2 text-xs text-red-400">{error}</div>}
      {issues > 0 && (
        <div className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-amber-500/5 px-4 py-1.5 text-[10px] text-amber-500">
          <CircleAlert className="h-3.5 w-3.5" />
          {t('continualLearning.trajectories.partial', { count: issues })}
        </div>
      )}
      {loading ? (
        <div className="grid min-h-0 flex-1 place-items-center text-xs text-[var(--nova-text-faint)]">{t('common.loading')}</div>
      ) : items.length === 0 ? (
        <div className="grid min-h-0 flex-1 place-items-center px-6 text-center">
          <div>
            <Route className="mx-auto h-6 w-6 text-[var(--nova-text-faint)]" />
            <div className="mt-2 text-xs font-medium text-[var(--nova-text)]">{t('continualLearning.trajectories.empty')}</div>
            <p className="mt-1 max-w-sm text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('continualLearning.trajectories.emptyDescription')}</p>
          </div>
        </div>
      ) : (
        <div className="grid min-h-0 flex-1 md:grid-cols-[minmax(280px,40%)_minmax(0,1fr)]">
          <div className="min-h-0 overflow-y-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 md:border-r md:border-b-0">
            <div className="relative ml-2 border-l border-[var(--nova-border)] pl-3">
              {items.map((item) => {
                const checked = selected.has(item.uri)
                const active = activeURI === item.uri
                return (
                  <article key={item.uri} className="relative pb-2">
                    <span className={`absolute -left-[17px] top-3 h-2 w-2 rounded-full ring-2 ring-[var(--nova-surface-2)] ${item.kind === 'run' ? 'bg-amber-500' : 'bg-emerald-500'}`} />
                    <div className={`flex min-w-0 items-start gap-2 rounded-[var(--nova-radius)] border px-2 py-2 transition-colors ${active ? 'border-[var(--nova-border-strong)] bg-[var(--nova-surface)]' : 'border-transparent hover:border-[var(--nova-border)] hover:bg-[var(--nova-surface)]'}`}>
                      <button
                        type="button"
                        role="checkbox"
                        aria-checked={checked}
                        aria-label={t('continualLearning.trajectories.include', { name: trajectoryLabel(item) })}
                        onClick={() => toggleEvidence(item.uri)}
                        className={`mt-0.5 grid h-4 w-4 shrink-0 place-items-center rounded-[4px] border transition-colors ${checked ? 'border-[var(--nova-text)] bg-[var(--nova-text)] text-[var(--nova-bg)]' : 'border-[var(--nova-border-strong)] bg-transparent'}`}
                      >
                        {checked && <Check className="h-3 w-3" />}
                      </button>
                      <button type="button" className="min-w-0 flex-1 text-left" onClick={() => setActiveURI(item.uri)}>
                        <div className="flex min-w-0 items-center gap-1.5">
                          {item.kind === 'run' ? <FileClock className="h-3.5 w-3.5 shrink-0 text-amber-500" /> : <MessagesSquare className="h-3.5 w-3.5 shrink-0 text-emerald-500" />}
                          <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text)]">{trajectoryLabel(item)}</span>
                          <span className="shrink-0 text-[9px] uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">{item.kind}</span>
                        </div>
                        <div className="mt-1 flex min-w-0 items-center gap-2 text-[9px] text-[var(--nova-text-faint)]">
                          <span className="min-w-0 flex-1 truncate">{item.project_name}</span>
                          <span className="shrink-0">{formatDateTime(item.updated_at)}</span>
                        </div>
                        <div className="mt-1 truncate font-mono text-[9px] text-[var(--nova-text-muted)]">{trajectoryMeta(item, t)}</div>
                      </button>
                    </div>
                  </article>
                )
              })}
            </div>
          </div>
          <section className="flex min-h-0 min-w-0 flex-col" aria-label={t('continualLearning.trajectories.detail')}>
            <div className="flex h-9 shrink-0 items-center border-b border-[var(--nova-border)] px-3 font-mono text-[9px] text-[var(--nova-text-faint)]">
              <span className="truncate">{activeURI}</span>
            </div>
            <pre className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-[10px] leading-5 text-[var(--nova-text-muted)]">
              {detailLoading ? t('common.loading') : detail}
            </pre>
          </section>
        </div>
      )}
    </div>
  )
}

function sinceForRange(range: RangeValue) {
  if (range === 'all') return new Date(0).toISOString()
  const days = range === '1d' ? 1 : range === '7d' ? 7 : 30
  return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString()
}

function trajectoryLabel(item: HarnessTrajectorySummary) {
  return item.title?.trim() || item.agent_kind?.trim() || item.id
}

function trajectoryMeta(item: HarnessTrajectorySummary, t: (key: string, values?: Record<string, unknown>) => string) {
  if (item.kind === 'session') {
    return t('continualLearning.trajectories.sessionMeta', { count: item.message_count ?? 0 })
  }
  return t('continualLearning.trajectories.runMeta', {
    status: item.status || '—',
    events: item.event_count ?? 0,
    tools: item.tool_calls ?? 0,
    duration: formatDuration(item.duration_ms ?? 0),
  })
}

function formatDuration(milliseconds: number) {
  if (milliseconds <= 0) return '—'
  if (milliseconds < 1000) return `${milliseconds}ms`
  if (milliseconds < 60_000) return `${(milliseconds / 1000).toFixed(1)}s`
  return `${Math.round(milliseconds / 60_000)}m`
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'Unknown error')
}
