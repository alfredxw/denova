import { BarChart3, Clock3, Hash } from 'lucide-react'
import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import type { ChatMessage } from '@/lib/api'
import { focusDialogContentOnOpen } from './dialog-focus'

interface TokenUsagePanelProps {
  messages: ChatMessage[]
}

type TokenUsageSummary = {
  count: number
  recent: ChatMessage[]
  promptTokens: number
  cachedPromptTokens: number
  uncachedPromptTokens: number
  completionTokens: number
  reasoningTokens: number
  totalTokens: number
  modelCalls: number
  cacheHitRate: number
}

type TokenUsageRow = {
  id: string
  runID: string
  agentKind: string
  requestCreatedAt?: string
  callCreatedAt?: string
  finishReason: string
  requestedTools: string[]
  afterTools: string[]
  callIndex: number
  promptTokens: number
  cachedPromptTokens: number
  uncachedPromptTokens: number
  cacheHitRate: number
  completionTokens: number
  reasoningTokens: number
  totalTokens: number
  modelCalls: number
  generatedBytes: number
}

type TokenUsageGroup = {
  id: string
  message: ChatMessage
  rows: TokenUsageRow[]
}

export function TokenUsagePanel({ messages }: TokenUsagePanelProps) {
  const { t } = useTranslation()
  const stats = useMemo(() => summarizeTokenUsage(messages), [messages])
  const hasUsage = stats.count > 0

  return (
    <div className="rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2.5 text-xs">
      <div className="flex items-center gap-2">
        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
          <BarChart3 className="h-3.5 w-3.5" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="font-medium text-[var(--nova-text)]">{t('chat.tokenUsage.title')}</div>
          <div className="truncate text-[11px] text-[var(--nova-text-faint)]">
            {hasUsage ? t('chat.tokenUsage.subtitle', { count: stats.count }) : t('chat.tokenUsage.empty')}
          </div>
        </div>
      </div>
      <div className="mt-2 grid grid-cols-2 gap-1.5 sm:grid-cols-4">
        <MetricCell label={t('chat.tokenUsage.cacheHit')} value={hasUsage ? formatPercent(stats.cacheHitRate) : '-'} />
        <MetricCell label={t('chat.tokenUsage.uncachedTokens')} value={hasUsage ? formatCompactNumber(stats.uncachedPromptTokens) : '-'} />
        <MetricCell label={t('chat.tokenUsage.totalTokens')} value={hasUsage ? formatCompactNumber(stats.totalTokens) : '-'} />
        <MetricCell label={t('chat.tokenUsage.modelCalls')} value={hasUsage ? formatCompactNumber(stats.modelCalls) : '-'} />
      </div>
      {hasUsage ? (
        <>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-[var(--nova-surface-2)]">
            <div className="h-full rounded-full bg-[var(--nova-accent-green)]" style={{ width: `${Math.round(stats.cacheHitRate * 100)}%` }} />
          </div>
          <div className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-[11px] text-[var(--nova-text-faint)]">
            <span>{t('chat.tokenUsage.promptTokens', { count: formatCompactNumber(stats.promptTokens) })}</span>
            <span>{t('chat.tokenUsage.cachedTokens', { count: formatCompactNumber(stats.cachedPromptTokens) })}</span>
            <span>{t('chat.tokenUsage.uncachedTokensWithCount', { count: formatCompactNumber(stats.uncachedPromptTokens) })}</span>
            <span>{t('chat.tokenUsage.outputTokens', { count: formatCompactNumber(stats.completionTokens) })}</span>
            <span>{t('chat.tokenUsage.reasoningTokens', { count: formatCompactNumber(stats.reasoningTokens) })}</span>
          </div>
          <div className="mt-2 space-y-1 border-t border-[var(--nova-border-soft)] pt-2">
            {stats.recent.map((message, index) => (
              <div key={message.run_id || message.id || index} className="flex items-center justify-between gap-2 text-[11px]">
                <span className="min-w-0 truncate text-[var(--nova-text-faint)]">
                  {t('chat.tokenUsage.recentRun', { count: index + 1 })}
                </span>
                <span className="shrink-0 font-mono text-[var(--nova-text-muted)]">
                  {formatPercent(numberOrZero(message.cache_hit_rate))} · {t('chat.tokenUsage.uncachedTokensWithCount', { count: formatCompactNumber(messageUncachedPromptTokens(message)) })} · {formatCompactNumber(numberOrZero(message.total_tokens))}
                </span>
              </div>
            ))}
          </div>
        </>
      ) : null}
    </div>
  )
}

export function TokenUsageDialog({ open, messages, onOpenChange }: {
  open: boolean
  messages: ChatMessage[]
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const usageMessages = useMemo(() => normalizeTokenUsageMessages(messages), [messages])
  const usageGroups = useMemo(() => buildTokenUsageGroups(usageMessages), [usageMessages])
  const usageRows = useMemo(() => usageGroups.flatMap((group) => group.rows), [usageGroups])
  const stats = useMemo(() => summarizeTokenUsage(usageMessages), [usageMessages])
  const hasUsage = usageRows.length > 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        tabIndex={-1}
        onOpenAutoFocus={focusDialogContentOnOpen}
        className="flex max-h-[88vh] max-w-[min(96vw,1400px)] flex-col gap-0 overflow-hidden border-[var(--nova-border)] bg-[var(--nova-bg)] p-0 text-[var(--nova-text)]"
      >
        <DialogHeader className="border-b border-[var(--nova-border)] px-4 py-3">
          <DialogTitle className="flex items-center gap-2 text-sm">
            <BarChart3 className="h-4 w-4 text-[var(--nova-text-muted)]" />
            {t('chat.tokenUsage.dialogTitle')}
          </DialogTitle>
          <DialogDescription className="text-xs text-[var(--nova-text-faint)]">
            {t('chat.tokenUsage.dialogDescription')}
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          {hasUsage ? (
            <div className="space-y-4">
              <TokenUsageSummaryGrid stats={stats} />
              <TokenUsageSourceNote />
              <div className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
                <div className="flex items-center justify-between gap-3 border-b border-[var(--nova-border)] px-3 py-2">
                  <h3 className="text-xs font-medium text-[var(--nova-text)]">{t('chat.tokenUsage.requestList')}</h3>
                  <span className="text-[11px] text-[var(--nova-text-faint)]">
                    {t('chat.tokenUsage.requestAndCallCount', { requests: usageMessages.length, calls: usageRows.length })}
                  </span>
                </div>
                <div className="space-y-3 p-2">
                  {usageGroups.map((group, groupIndex) => (
                    <TokenUsageRequestGroup
                      key={group.id}
                      group={group}
                      requestIndex={groupIndex + 1}
                    />
                  ))}
                </div>
              </div>
            </div>
          ) : (
            <div className="flex min-h-40 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-xs text-[var(--nova-text-faint)]">
              {t('chat.tokenUsage.emptyDetail')}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function TokenUsageSourceNote() {
  const { t } = useTranslation()
  return (
    <div className="grid gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 text-[11px] text-[var(--nova-text-muted)] md:grid-cols-3">
      <InfoItem
        icon={<BarChart3 className="h-3.5 w-3.5" />}
        title={t('chat.tokenUsage.source.title')}
        body={t('chat.tokenUsage.source.body')}
      />
      <InfoItem
        icon={<Hash className="h-3.5 w-3.5" />}
        title={t('chat.tokenUsage.calls.title')}
        body={t('chat.tokenUsage.calls.body')}
      />
      <InfoItem
        icon={<Clock3 className="h-3.5 w-3.5" />}
        title={t('chat.tokenUsage.timing.title')}
        body={t('chat.tokenUsage.timing.body')}
      />
    </div>
  )
}

function InfoItem({ icon, title, body }: { icon: ReactNode; title: string; body: string }) {
  return (
    <div className="flex min-w-0 gap-2">
      <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border-soft)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]">
        {icon}
      </span>
      <div className="min-w-0">
        <div className="font-medium text-[var(--nova-text)]">{title}</div>
        <div className="mt-0.5 leading-4 text-[var(--nova-text-faint)]">{body}</div>
      </div>
    </div>
  )
}

function TokenUsageSummaryGrid({ stats }: { stats: TokenUsageSummary }) {
  const { t } = useTranslation()
  const items = [
    { label: t('chat.tokenUsage.summary.requests'), value: formatNumber(stats.count) },
    { label: t('chat.tokenUsage.cacheHit'), value: formatPercent(stats.cacheHitRate) },
    { label: t('chat.tokenUsage.totalTokens'), value: formatNumber(stats.totalTokens) },
    { label: t('chat.tokenUsage.modelCalls'), value: formatNumber(stats.modelCalls) },
    { label: t('chat.tokenUsage.summary.prompt'), value: formatNumber(stats.promptTokens) },
    { label: t('chat.tokenUsage.summary.cached'), value: formatNumber(stats.cachedPromptTokens) },
    { label: t('chat.tokenUsage.summary.uncached'), value: formatNumber(stats.uncachedPromptTokens) },
    { label: t('chat.tokenUsage.summary.output'), value: formatNumber(stats.completionTokens) },
    { label: t('chat.tokenUsage.summary.reasoning'), value: formatNumber(stats.reasoningTokens) },
  ]
  return (
    <div className="grid gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[11px] sm:grid-cols-4">
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <div className="truncate text-[var(--nova-text-faint)]">{item.label}</div>
          <div className="mt-0.5 truncate font-mono font-medium text-[var(--nova-text)]">{item.value}</div>
        </div>
      ))}
    </div>
  )
}

function TokenUsageRequestGroup({ group, requestIndex }: { group: TokenUsageGroup; requestIndex: number }) {
  const { t } = useTranslation()
  const message = group.message
  const runID = message.run_id || message.id || ''
  return (
    <section className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border-soft)] bg-[var(--nova-surface)]">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-b border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px]">
        <div className="min-w-0">
          <div className="font-medium text-[var(--nova-text)]">
            {t('chat.tokenUsage.requestGroup', { count: requestIndex })}
          </div>
          <div className="mt-0.5 flex flex-wrap gap-x-3 gap-y-1 text-[var(--nova-text-faint)]">
            <span>{message.agent_kind || '-'}</span>
            <span>{formatTimestamp(message.created_at)}</span>
            <span>{t('chat.tokenUsage.columns.run')}: <span className="font-mono" title={runID}>{runID || '-'}</span></span>
          </div>
        </div>
        <div className="flex flex-wrap gap-x-3 gap-y-1 font-mono text-[var(--nova-text-muted)]">
          <span>{t('chat.tokenUsage.modelCalls')}: {formatNumber(numberOrZero(message.model_calls))}</span>
          <span>{t('chat.tokenUsage.totalTokens')}: {formatNumber(numberOrZero(message.total_tokens))}</span>
          <span>{t('chat.tokenUsage.uncachedTokens')}: {formatNumber(messageUncachedPromptTokens(message))}</span>
          <span>{t('chat.tokenUsage.cacheHit')}: {formatPercent(numberOrZero(message.cache_hit_rate))}</span>
        </div>
      </div>
      <div className="divide-y divide-[var(--nova-border-soft)]">
        {group.rows.map((row) => (
          <TokenUsageCallCard key={row.id} row={row} />
        ))}
      </div>
    </section>
  )
}

function TokenUsageCallCard({ row }: { row: TokenUsageRow }) {
  const { t } = useTranslation()
  return (
    <div className="grid gap-2 p-2.5 text-xs md:grid-cols-[92px_minmax(250px,1.15fr)_minmax(210px,1fr)_minmax(210px,1fr)_minmax(170px,0.8fr)_92px] md:items-center md:gap-3">
      <div className="min-w-0">
        <div className="font-medium text-[var(--nova-text)]">{t('chat.tokenUsage.callIndex', { count: row.callIndex })}</div>
        <div className="mt-0.5 text-[10px] text-[var(--nova-text-faint)]">{formatCallPurpose(row, t)}</div>
      </div>
      <TokenBreakdown row={row} />
      <div className="min-w-0">
        <div className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.columns.requestedTools')}</div>
        <ToolList tools={row.requestedTools} empty={t('chat.tokenUsage.tool.none')} />
      </div>
      <div className="min-w-0">
        <div className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.columns.afterTools')}</div>
        <ToolList tools={row.afterTools} empty="-" />
      </div>
      <div className="min-w-0">
        <div className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.columns.callTime')}</div>
        <div className="mt-0.5 font-mono text-[11px] text-[var(--nova-text)]">{formatTimestamp(row.callCreatedAt || row.requestCreatedAt)}</div>
      </div>
      <div className="min-w-0 md:text-right">
        <div className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.columns.cacheHit')}</div>
        <div className="mt-0.5 font-mono font-medium text-[var(--nova-text)]">{formatPercent(row.cacheHitRate)}</div>
      </div>
    </div>
  )
}

function ToolList({ tools, empty }: { tools: string[]; empty: string }) {
  if (tools.length === 0) {
    return <div className="mt-0.5 text-[11px] text-[var(--nova-text-faint)]">{empty}</div>
  }
  return (
    <div className="mt-1 flex flex-wrap gap-1">
      {tools.map((tool, index) => (
        <span key={`${tool}-${index}`} className="inline-flex min-w-0 max-w-full rounded-md border border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px]">
          <span className="min-w-0 truncate font-mono text-[var(--nova-text-muted)]">{tool}</span>
        </span>
      ))}
    </div>
  )
}

function TokenBreakdown({ row }: { row: TokenUsageRow }) {
  const { t } = useTranslation()
  return (
    <div className="min-w-0">
      <div className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.columns.callTokens')}</div>
        <div className="font-mono font-medium text-[var(--nova-text)]">{formatNumber(row.totalTokens)}</div>
      <div className="mt-1 flex flex-wrap gap-x-2 gap-y-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">
        <span>{t('chat.tokenUsage.columns.prompt')}: <span className="font-mono">{formatNumber(row.promptTokens)}</span></span>
        <span>{t('chat.tokenUsage.columns.cached')}: <span className="font-mono">{formatNumber(row.cachedPromptTokens)}</span></span>
        <span>{t('chat.tokenUsage.columns.uncached')}: <span className="font-mono">{formatNumber(row.uncachedPromptTokens)}</span></span>
        <span>{t('chat.tokenUsage.columns.completion')}: <span className="font-mono">{formatNumber(row.completionTokens)}</span></span>
        <span>{t('chat.tokenUsage.columns.reasoning')}: <span className="font-mono">{formatNumber(row.reasoningTokens)}</span></span>
      </div>
    </div>
  )
}

function summarizeTokenUsage(messages: ChatMessage[]): TokenUsageSummary {
  const usageMessages = normalizeTokenUsageMessages(messages)
  const summary = usageMessages.reduce<TokenUsageSummary>((acc, message) => {
    acc.count += 1
    acc.promptTokens += numberOrZero(message.prompt_tokens)
    acc.cachedPromptTokens += numberOrZero(message.cached_prompt_tokens)
    acc.uncachedPromptTokens += messageUncachedPromptTokens(message)
    acc.completionTokens += numberOrZero(message.completion_tokens)
    acc.reasoningTokens += numberOrZero(message.reasoning_tokens)
    acc.totalTokens += numberOrZero(message.total_tokens)
    acc.modelCalls += numberOrZero(message.model_calls)
    return acc
  }, {
    count: 0,
    recent: usageMessages.slice(-5).reverse(),
    promptTokens: 0,
    cachedPromptTokens: 0,
    uncachedPromptTokens: 0,
    completionTokens: 0,
    reasoningTokens: 0,
    totalTokens: 0,
    modelCalls: 0,
    cacheHitRate: 0,
  })
  summary.cacheHitRate = summary.promptTokens > 0 ? summary.cachedPromptTokens / summary.promptTokens : 0
  return summary
}

function normalizeTokenUsageMessages(messages: ChatMessage[]) {
  return messages
    .filter((message) => message.role === 'token_usage' && numberOrZero(message.model_calls) > 0)
    .slice()
    .sort((a, b) => timestampValue(a.created_at) - timestampValue(b.created_at))
}

function buildTokenUsageGroups(messages: ChatMessage[]): TokenUsageGroup[] {
  return messages.map((message) => {
    const runID = message.run_id || message.id || ''
    const agentKind = message.agent_kind || ''
    const calls = Array.isArray(message.usage_calls) ? message.usage_calls.filter((call) => (
      numberOrZero(call.prompt_tokens) > 0 || numberOrZero(call.completion_tokens) > 0 || numberOrZero(call.total_tokens) > 0
    )) : []
    if (calls.length === 0) {
      return {
        id: runID || message.id || message.created_at || 'usage',
        message,
        rows: [{
          id: `${runID || 'usage'}:summary`,
          runID,
          agentKind,
          requestCreatedAt: message.created_at,
          callCreatedAt: message.created_at,
          finishReason: '',
          requestedTools: [],
          afterTools: [],
          callIndex: 1,
          promptTokens: numberOrZero(message.prompt_tokens),
          cachedPromptTokens: numberOrZero(message.cached_prompt_tokens),
          uncachedPromptTokens: messageUncachedPromptTokens(message),
          cacheHitRate: numberOrZero(message.cache_hit_rate),
          completionTokens: numberOrZero(message.completion_tokens),
          reasoningTokens: numberOrZero(message.reasoning_tokens),
          totalTokens: numberOrZero(message.total_tokens),
          modelCalls: numberOrZero(message.model_calls),
          generatedBytes: numberOrZero(message.generated_bytes),
        }],
      }
    }
    const orderedCalls = calls.slice().sort((a, b) => {
      const indexDiff = numberOrZero(a.index) - numberOrZero(b.index)
      if (indexDiff !== 0) return indexDiff
      return timestampValue(a.created_at) - timestampValue(b.created_at)
    })
    return {
      id: runID || message.id || message.created_at || 'usage',
      message,
      rows: orderedCalls.map((call, index) => ({
        id: `${runID || 'usage'}:${call.index || index + 1}`,
        runID,
        agentKind,
        requestCreatedAt: message.created_at,
        callCreatedAt: call.created_at || message.created_at,
        finishReason: call.finish_reason || '',
        requestedTools: Array.isArray(call.requested_tools) ? call.requested_tools.filter(Boolean) : [],
        afterTools: Array.isArray(call.after_tools) ? call.after_tools.filter(Boolean) : [],
        callIndex: numberOrZero(call.index) || index + 1,
        promptTokens: numberOrZero(call.prompt_tokens),
        cachedPromptTokens: numberOrZero(call.cached_prompt_tokens),
        uncachedPromptTokens: callUncachedPromptTokens(call),
        cacheHitRate: numberOrZero(call.cache_hit_rate),
        completionTokens: numberOrZero(call.completion_tokens),
        reasoningTokens: numberOrZero(call.reasoning_tokens),
        totalTokens: numberOrZero(call.total_tokens),
        modelCalls: 1,
        generatedBytes: index === orderedCalls.length - 1 ? numberOrZero(message.generated_bytes) : 0,
      })),
    }
  })
}

function MetricCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] px-2 py-1.5">
      <div className="truncate text-[10px] text-[var(--nova-text-faint)]">{label}</div>
      <div className="truncate font-mono text-[12px] text-[var(--nova-text)]">{value}</div>
    </div>
  )
}

function numberOrZero(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

type PromptCacheUsageValue = {
  prompt_tokens?: number
  cached_prompt_tokens?: number
  uncached_prompt_tokens?: number
}

function messageUncachedPromptTokens(message: PromptCacheUsageValue) {
  return promptCacheMissTokens(message)
}

function callUncachedPromptTokens(call: PromptCacheUsageValue) {
  return promptCacheMissTokens(call)
}

function promptCacheMissTokens(value: PromptCacheUsageValue) {
  const explicit = numberOrZero(value.uncached_prompt_tokens)
  if (explicit > 0) return explicit
  const prompt = numberOrZero(value.prompt_tokens)
  const cached = numberOrZero(value.cached_prompt_tokens)
  if (prompt <= 0) return 0
  if (cached <= 0) return prompt
  return Math.max(0, prompt - cached)
}

function formatPercent(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0%'
  return `${Math.round(value * 1000) / 10}%`
}

function formatCompactNumber(value: number) {
  if (!Number.isFinite(value)) return '0'
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(value)
}

function formatNumber(value: number) {
  if (!Number.isFinite(value)) return '0'
  return new Intl.NumberFormat().format(value)
}

function formatCallPurpose(row: TokenUsageRow, t: ReturnType<typeof useTranslation>['t']) {
  if (row.requestedTools.length > 0) {
    return t('chat.tokenUsage.finishReason.toolCallsWithName', { tools: row.requestedTools.join(', ') })
  }
  if (row.afterTools.length > 0) {
    return t('chat.tokenUsage.finishReason.afterTool', { tools: row.afterTools.join(', ') })
  }
  switch (row.finishReason) {
    case 'tool_calls':
      return t('chat.tokenUsage.finishReason.toolCalls')
    case 'stop':
    case 'end_turn':
      return t('chat.tokenUsage.finishReason.stop')
    case 'length':
      return t('chat.tokenUsage.finishReason.length')
    case 'content_filter':
      return t('chat.tokenUsage.finishReason.contentFilter')
    default:
      return row.finishReason ? t('chat.tokenUsage.finishReason.raw', { reason: row.finishReason }) : t('chat.tokenUsage.finishReason.unknown')
  }
}

function timestampValue(value?: string) {
  if (!value) return 0
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : 0
}

function formatTimestamp(value?: string) {
  if (!value) return '-'
  const timestamp = timestampValue(value)
  if (!timestamp) return '-'
  const parts = new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).formatToParts(new Date(timestamp))
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((item) => item.type === type)?.value || ''
  return `${part('year')}-${part('month')}-${part('day')} ${part('hour')}:${part('minute')}:${part('second')}`
}
