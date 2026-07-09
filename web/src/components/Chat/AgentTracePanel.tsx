import { useEffect, useMemo, useState } from 'react'
import { Activity, Bot, Braces, CheckCircle2, Database, Hammer, RefreshCw, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getAgentRunTrace, getAgentRunTraces } from '@/lib/api'
import type { AgentRunTrace, AgentRunTraceRecord, AgentRunTraceSummary } from '@/lib/api'

type TraceFilter = 'all' | 'llm' | 'tools' | 'context' | 'errors'
type TraceCategory = 'run' | 'llm' | 'tools' | 'context' | 'verification' | 'errors' | 'event'

interface AgentTracePanelProps {
  disabled?: boolean
  selectedRunId?: string
}

const SPAN_RECORD_TYPES = new Set([
  'agent_run',
  'llm_call',
  'tool_call',
  'context_build',
  'context_compaction',
  'post_run_verification',
])

export function AgentTracePanel({ disabled, selectedRunId }: AgentTracePanelProps) {
  const { t } = useTranslation()
  const [runs, setRuns] = useState<AgentRunTraceSummary[]>([])
  const [selectedID, setSelectedID] = useState(selectedRunId || '')
  const [trace, setTrace] = useState<AgentRunTrace | null>(null)
  const [filter, setFilter] = useState<TraceFilter>('all')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const loadRuns = async (preferredID?: string) => {
    setLoading(true)
    setError('')
    try {
      const list = await getAgentRunTraces(30)
      setRuns(list)
      const nextID = preferredID || selectedRunId || selectedID || list[0]?.id || ''
      setSelectedID(nextID)
      setTrace(nextID ? await getAgentRunTrace(nextID) : null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadRuns(selectedRunId)
  }, [])

  useEffect(() => {
    if (selectedRunId && selectedRunId !== selectedID) {
      setSelectedID(selectedRunId)
    }
  }, [selectedRunId, selectedID])

  useEffect(() => {
    if (!selectedID) {
      setTrace(null)
      return
    }
    let cancelled = false
    setLoading(true)
    setError('')
    getAgentRunTrace(selectedID)
      .then((next) => {
        if (!cancelled) setTrace(next)
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [selectedID])

  const stats = useMemo(() => trace ? summarizeTrace(trace.records) : emptyTraceStats(), [trace])
  const timelineRecords = useMemo(() => {
    const records = trace?.records.filter((record) => record.type !== 'run_created') || []
    return records.filter((record) => matchesFilter(record, filter))
  }, [filter, trace])

  const filterItems: Array<{ id: TraceFilter; label: string; count: number }> = [
    { id: 'all', label: t('chat.tracePanel.filterAll'), count: stats.total },
    { id: 'llm', label: t('chat.tracePanel.filterLLM'), count: stats.llm },
    { id: 'tools', label: t('chat.tracePanel.filterTools'), count: stats.tools },
    { id: 'context', label: t('chat.tracePanel.filterContext'), count: stats.context },
    { id: 'errors', label: t('chat.tracePanel.filterErrors'), count: stats.errors },
  ]

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface)]">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
        <div className="flex min-w-0 flex-1 items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
          <Activity className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          <span className="truncate">{t('chat.tracePanel.title')}</span>
        </div>
        <button
          type="button"
          disabled={disabled || loading}
          onClick={() => void loadRuns(selectedID)}
          className="nova-nav-item rounded p-1 disabled:cursor-not-allowed disabled:opacity-45"
          aria-label={t('chat.tracePanel.refresh')}
          title={t('chat.tracePanel.refresh')}
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>
      <div className="grid min-h-0 flex-1 grid-rows-[178px_1fr]">
        <div className="min-h-0 overflow-y-auto border-b border-[var(--nova-border)] p-2">
          {runs.length === 0 && !loading ? (
            <div className="px-2 py-4 text-xs text-[var(--nova-text-faint)]">{t('chat.tracePanel.empty')}</div>
          ) : (
            <div className="space-y-1">
              {runs.map((run) => (
                <button
                  key={run.id}
                  type="button"
                  onClick={() => setSelectedID(run.id)}
                  className={`w-full rounded-[6px] border px-2 py-1.5 text-left text-[11px] transition-colors ${selectedID === run.id ? 'border-[var(--nova-accent)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'}`}
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="min-w-0 flex-1 truncate font-mono">{run.id}</span>
                    <StatusPill status={run.status} />
                  </div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-[var(--nova-text-faint)]">
                    <span>{formatTraceTime(run.created_at)}</span>
                    <span>{t('chat.tracePanel.events', { count: run.events })}</span>
                    <span>{t('chat.tracePanel.llmCallsCount', { count: numberOrZero(run.llm_calls) })}</span>
                    <span>{formatDuration(numberOrZero(run.duration_ms))}</span>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>
        <div className="min-h-0 overflow-y-auto p-3">
          {error && <div className="mb-2 rounded border border-[var(--nova-danger)] px-2 py-1.5 text-xs text-[var(--nova-danger)]">{error}</div>}
          {trace ? (
            <div className="space-y-3">
              <TraceSummaryGrid trace={trace} stats={stats} />
              <div className="flex flex-wrap gap-1">
                {filterItems.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => setFilter(item.id)}
                    className={`inline-flex h-7 items-center gap-1 rounded-[6px] border px-2 text-[11px] transition-colors ${filter === item.id ? 'border-[var(--nova-accent)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'}`}
                  >
                    <span>{item.label}</span>
                    <span className="font-mono text-[10px] text-[var(--nova-text-faint)]">{item.count}</span>
                  </button>
                ))}
              </div>
              {timelineRecords.length === 0 ? (
                <div className="rounded-[6px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-6 text-center text-xs text-[var(--nova-text-faint)]">
                  {t('chat.tracePanel.noRecords')}
                </div>
              ) : (
                <div className="space-y-2">
                  {timelineRecords.map((record, index) => (
                    <TraceRecordCard key={`${record.type}-${record.created_at}-${index}`} record={record} />
                  ))}
                </div>
              )}
              {trace.truncated && <div className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.tracePanel.truncated')}</div>}
            </div>
          ) : (
            <div className="px-2 py-4 text-xs text-[var(--nova-text-faint)]">{loading ? t('common.loading') : t('chat.tracePanel.selectRun')}</div>
          )}
        </div>
      </div>
    </div>
  )
}

function TraceSummaryGrid({ trace, stats }: { trace: AgentRunTrace; stats: TraceStats }) {
  const { t } = useTranslation()
  const summary = trace.summary
  const items = [
    { label: t('chat.tracePanel.summaryStatus'), value: summary.status || '-' },
    { label: t('chat.tracePanel.summaryDuration'), value: formatDuration(numberOrZero(summary.duration_ms)) },
    { label: t('chat.tracePanel.summaryLLM'), value: formatNumber(numberOrZero(summary.llm_calls) || stats.llm) },
    { label: t('chat.tracePanel.summaryTools'), value: formatToolSummary(summary, stats, t) },
    { label: t('chat.tracePanel.summaryContext'), value: formatNumber(numberOrZero(summary.context_parts) || stats.context) },
    { label: t('chat.tracePanel.summaryErrors'), value: formatNumber(stats.errors) },
  ]
  return (
    <div className="grid gap-2 rounded-[6px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[11px] sm:grid-cols-3">
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <div className="truncate text-[var(--nova-text-faint)]">{item.label}</div>
          <div className="mt-0.5 truncate font-mono text-[var(--nova-text)]">{item.value}</div>
        </div>
      ))}
    </div>
  )
}

function TraceRecordCard({ record }: { record: AgentRunTraceRecord }) {
  const { t } = useTranslation()
  const category = traceCategory(record)
  const data = record.data || {}
  const attrs = readObject(data.attrs)
  const status = recordStatus(record)
  const duration = numberOrZero(data.duration_ms)
  const chips = recordChips(record, t)
  const Icon = categoryIcon(category, status)
  const title = recordTitle(record, t)
  const isSpan = SPAN_RECORD_TYPES.has(record.type) || Boolean(readString(data.span_id))

  return (
    <article className="rounded-[6px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
      <div className="flex min-w-0 items-start gap-2">
        <span className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-md border ${status === 'error' ? 'border-[var(--nova-danger)] text-[var(--nova-danger)]' : 'border-[var(--nova-border-soft)] text-[var(--nova-text-faint)]'}`}>
          <Icon className="h-3.5 w-3.5" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <span className="min-w-0 truncate text-xs font-medium text-[var(--nova-text)]">{title}</span>
            <StatusPill status={status} />
            {duration > 0 ? <span className="rounded border border-[var(--nova-border-soft)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--nova-text-faint)]">{formatDuration(duration)}</span> : null}
          </div>
          <div className="mt-1 flex flex-wrap gap-1">
            <span className="rounded border border-[var(--nova-border-soft)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--nova-text-faint)]">{record.type}</span>
            {isSpan ? <TraceChip label={t('chat.tracePanel.span')} value={readString(data.span_id)} /> : null}
            {readString(data.parent_span_id) ? <TraceChip label={t('chat.tracePanel.parentSpan')} value={readString(data.parent_span_id)} /> : null}
            {chips.map((chip) => <TraceChip key={`${chip.label}:${chip.value}`} label={chip.label} value={chip.value} />)}
          </div>
          <div className="mt-1 text-[10px] text-[var(--nova-text-faint)]">
            {formatTraceTime(readString(data.started_at) || record.created_at)}
            {readString(data.ended_at) ? ` - ${formatTraceTime(readString(data.ended_at))}` : ''}
          </div>
          {recordError(record) ? (
            <div className="mt-2 rounded border border-[var(--nova-danger)] bg-[var(--nova-danger-bg)] px-2 py-1.5 text-[11px] text-[var(--nova-danger)]">
              {recordError(record)}
            </div>
          ) : null}
          {Object.keys(attrs).length > 0 || !isSpan ? (
            <details className="mt-2">
              <summary className="cursor-pointer select-none text-[10px] text-[var(--nova-text-faint)]">{t('chat.tracePanel.rawData')}</summary>
              <pre className="mt-1 max-h-44 overflow-auto whitespace-pre-wrap break-words rounded border border-[var(--nova-border-soft)] bg-[var(--nova-surface)] p-2 font-mono text-[10px] leading-relaxed text-[var(--nova-text-muted)]">{formatTraceData(isSpan ? attrs : data)}</pre>
            </details>
          ) : null}
        </div>
      </div>
    </article>
  )
}

function TraceChip({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <span className="inline-flex max-w-full items-center gap-1 rounded border border-[var(--nova-border-soft)] px-1.5 py-0.5 text-[10px]">
      <span className="shrink-0 text-[var(--nova-text-faint)]">{label}</span>
      <span className="min-w-0 truncate font-mono text-[var(--nova-text-muted)]">{value}</span>
    </span>
  )
}

function StatusPill({ status }: { status?: string }) {
  if (!status) return null
  const normalized = status.toLowerCase()
  const tone = normalized === 'error' || normalized === 'failed'
    ? 'border-[var(--nova-danger)] text-[var(--nova-danger)]'
    : normalized === 'success' || normalized === 'ok' || normalized === 'completed'
      ? 'border-[var(--nova-success)] text-[var(--nova-success)]'
      : 'border-[var(--nova-border-soft)] text-[var(--nova-text-faint)]'
  return <span className={`rounded border px-1.5 py-0.5 text-[10px] ${tone}`}>{status}</span>
}

type TraceStats = {
  total: number
  llm: number
  tools: number
  context: number
  errors: number
}

function emptyTraceStats(): TraceStats {
  return { total: 0, llm: 0, tools: 0, context: 0, errors: 0 }
}

function summarizeTrace(records: AgentRunTraceRecord[]): TraceStats {
  return records.reduce<TraceStats>((stats, record) => {
    if (record.type === 'run_created') return stats
    stats.total += 1
    const category = traceCategory(record)
    if (category === 'llm') stats.llm += 1
    if (category === 'tools') stats.tools += 1
    if (category === 'context') stats.context += 1
    if (recordIsError(record)) stats.errors += 1
    return stats
  }, emptyTraceStats())
}

function matchesFilter(record: AgentRunTraceRecord, filter: TraceFilter) {
  if (filter === 'all') return true
  if (filter === 'errors') return recordIsError(record)
  const category = traceCategory(record)
  if (filter === 'llm') return category === 'llm'
  if (filter === 'tools') return category === 'tools'
  return category === 'context'
}

function traceCategory(record: AgentRunTraceRecord): TraceCategory {
  if (recordIsError(record)) return 'errors'
  switch (record.type) {
    case 'agent_run':
    case 'run_finished':
      return 'run'
    case 'llm_call':
    case 'token_usage':
      return 'llm'
    case 'tool_call':
    case 'tool_decision':
    case 'tool_execution':
      return 'tools'
    case 'context_build':
    case 'context_compaction':
    case 'context_ledger':
      return 'context'
    case 'post_run_verification':
      return 'verification'
    default:
      return 'event'
  }
}

function categoryIcon(category: TraceCategory, status?: string) {
  if (status === 'error' || category === 'errors') return XCircle
  switch (category) {
    case 'run':
      return Bot
    case 'llm':
      return Activity
    case 'tools':
      return Hammer
    case 'context':
      return Database
    case 'verification':
      return CheckCircle2
    default:
      return Braces
  }
}

function recordTitle(record: AgentRunTraceRecord, t: ReturnType<typeof useTranslation>['t']) {
  const data = record.data || {}
  const attrs = readObject(data.attrs)
  const model = readString(attrs.model) || readString(data.model)
  const tool = readString(attrs.tool_name) || readString(attrs.name) || readString(data.tool_name) || readString(data.name)
  switch (traceCategory(record)) {
    case 'run':
      return t('chat.tracePanel.recordRun')
    case 'llm':
      return model ? t('chat.tracePanel.recordLLMWithModel', { model }) : t('chat.tracePanel.recordLLM')
    case 'tools':
      return tool ? t('chat.tracePanel.recordToolWithName', { tool }) : t('chat.tracePanel.recordTool')
    case 'context':
      return record.type === 'context_compaction' ? t('chat.tracePanel.recordCompaction') : t('chat.tracePanel.recordContext')
    case 'verification':
      return t('chat.tracePanel.recordVerification')
    case 'errors':
      return t('chat.tracePanel.recordError')
    default:
      return record.type
  }
}

function recordChips(record: AgentRunTraceRecord, t: ReturnType<typeof useTranslation>['t']) {
  const data = record.data || {}
  const attrs = readObject(data.attrs)
  const chips: Array<{ label: string; value?: string }> = []
  const model = readString(attrs.model) || readString(data.model)
  const providerRequestID = readString(attrs.provider_request_id) || readString(data.provider_request_id)
  const callID = readString(attrs.call_id) || readString(data.call_id)
  const finishReason = readString(attrs.finish_reason) || readString(data.finish_reason)
  const toolCallID = readString(attrs.tool_call_id) || readString(data.tool_call_id)
  const target = readString(attrs.mutation_target) || readString(attrs.target) || readString(data.target)
  const totalTokens = numberOrZero(attrs.total_tokens || data.total_tokens)
  const promptTokens = numberOrZero(attrs.prompt_tokens || data.prompt_tokens)
  const cachedTokens = numberOrZero(attrs.cached_prompt_tokens || data.cached_prompt_tokens)
  const reasoningTokens = numberOrZero(attrs.reasoning_tokens || data.reasoning_tokens)
  const truncated = boolString(attrs.truncated ?? data.truncated)
  if (model) chips.push({ label: t('chat.tracePanel.model'), value: model })
  if (totalTokens > 0) chips.push({ label: t('chat.tracePanel.tokens'), value: formatNumber(totalTokens) })
  if (promptTokens > 0 || cachedTokens > 0) chips.push({ label: t('chat.tracePanel.cache'), value: `${formatNumber(cachedTokens)} / ${formatNumber(promptTokens)}` })
  if (reasoningTokens > 0) chips.push({ label: t('chat.tracePanel.reasoning'), value: formatNumber(reasoningTokens) })
  if (providerRequestID) chips.push({ label: t('chat.tracePanel.providerRequest'), value: providerRequestID })
  if (callID) chips.push({ label: t('chat.tracePanel.callID'), value: callID })
  if (finishReason) chips.push({ label: t('chat.tracePanel.finishReason'), value: finishReason })
  if (toolCallID) chips.push({ label: t('chat.tracePanel.toolCallID'), value: toolCallID })
  if (target) chips.push({ label: t('chat.tracePanel.target'), value: target })
  if (truncated) chips.push({ label: t('chat.tracePanel.truncation'), value: truncated })
  return chips
}

function recordStatus(record: AgentRunTraceRecord) {
  const data = record.data || {}
  const attrs = readObject(data.attrs)
  return readString(data.status) || readString(attrs.status)
}

function recordIsError(record: AgentRunTraceRecord) {
  const status = recordStatus(record).toLowerCase()
  return status === 'error' || status === 'failed' || Boolean(recordError(record)) || record.type.includes('error')
}

function recordError(record: AgentRunTraceRecord) {
  const data = record.data || {}
  const attrs = readObject(data.attrs)
  const direct = readString(data.error) || readString(attrs.error) || readString(data.message)
  if (direct) return direct
  const structured = readObject(data.error)
  return readString(structured.message) || readString(structured.preview)
}

function readObject(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function readString(value: unknown) {
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return ''
}

function boolString(value: unknown) {
  if (typeof value === 'boolean') return value ? 'true' : ''
  if (typeof value === 'string' && value) return value
  return ''
}

function numberOrZero(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function formatTraceData(data?: Record<string, unknown>) {
  if (!data) return '{}'
  const text = JSON.stringify(data, null, 2)
  return text.length > 4000 ? `${text.slice(0, 4000)}...` : text
}

function formatTraceTime(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatDuration(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '-'
  if (value < 1000) return `${Math.round(value)}ms`
  return `${(value / 1000).toFixed(value < 10000 ? 1 : 0)}s`
}

function formatNumber(value: number) {
  if (!Number.isFinite(value)) return '0'
  return new Intl.NumberFormat().format(value)
}

function formatToolSummary(summary: AgentRunTraceSummary, stats: TraceStats, t: ReturnType<typeof useTranslation>['t']) {
  const calls = numberOrZero(summary.tool_calls)
  if (calls <= 0) return formatNumber(stats.tools)
  const errors = numberOrZero(summary.tool_errors)
  const blocked = numberOrZero(summary.tool_blocked)
  const truncated = numberOrZero(summary.tool_truncated)
  const details = [
    errors > 0 ? t('chat.tracePanel.toolErrorsShort', { count: formatNumber(errors) }) : '',
    blocked > 0 ? t('chat.tracePanel.toolBlockedShort', { count: formatNumber(blocked) }) : '',
    truncated > 0 ? t('chat.tracePanel.toolTruncatedShort', { count: formatNumber(truncated) }) : '',
  ].filter(Boolean)
  return details.length > 0 ? `${formatNumber(calls)} (${details.join(', ')})` : formatNumber(calls)
}
