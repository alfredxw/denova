import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { ChartEmpty, ChartPanel, MetricStrip } from './StatisticsCharts'
import { StatisticsTable } from './StatisticsTable'
import type { StatisticsReport, UsageTotals } from './api'

type Metric = 'tokens' | 'requests' | 'latency'

function reportedTokens(item: UsageTotals): number | null {
  return item.requests + item.external_turns > item.unknown_usage ? item.tokens : null
}

export function ModelStatistics({ data }: { data: StatisticsReport['usage'] }) {
  const { t, i18n } = useTranslation()
  const [metric, setMetric] = useState<Metric>('tokens')
  const number = (value: number) => new Intl.NumberFormat(i18n.language).format(value)
  const compact = (value: number) => new Intl.NumberFormat(i18n.language, { notation: 'compact', maximumFractionDigits: 1 }).format(value)
  const percent = (value: number) => new Intl.NumberFormat(i18n.language, { style: 'percent', minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(value)
  const seconds = (ms: number | null) => ms === null ? '—' : t('statistics.seconds', { value: new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(ms / 1000) })
  const mean = (item: UsageTotals) => item.requests ? item.duration_ms / item.requests : null
  const firstChunk = (item: UsageTotals) => item.ttft_requests ? item.ttft_ms / item.ttft_requests : null
  const total = data.totals
  const calls = total.requests + total.external_turns
  const tokenTotal = reportedTokens(total)
  const metricValue = (item: UsageTotals): number | null => {
    switch (metric) {
      case 'tokens': return reportedTokens(item)
      case 'requests': return item.requests
      case 'latency': return mean(item)
    }
  }
  const days = data.days.map(item => ({ ...item, value: metricValue(item) }))
  const labels = { tokens: t('statistics.tokens'), requests: t('statistics.requests'), latency: t('statistics.latency') }
  const cacheWrite = total.cache_write ?? 0
  const tokenParts = [
    { name: t('statistics.uncached'), value: Math.max(0, total.prompt - total.cached - cacheWrite), color: 'var(--foreground)' },
    { name: t('statistics.cacheWrite'), value: cacheWrite, color: 'var(--nova-warning)' },
    { name: t('statistics.cached'), value: total.cached, color: 'var(--nova-success)' },
    { name: t('statistics.output'), value: total.output, color: 'var(--chart-2)' },
    { name: t('statistics.otherTokens'), value: Math.max(0, total.tokens - total.prompt - total.output), color: 'var(--chart-3)' },
  ].filter(item => item.value > 0)
  const partsTotal = tokenParts.reduce((sum, part) => sum + part.value, 0)

  return <div className="flex flex-col gap-4">
    <MetricStrip items={[
      { label: labels.tokens, value: tokenTotal === null ? '—' : compact(tokenTotal), detail: calls > 0 ? t('statistics.usageSamples', { reported: number(calls - total.unknown_usage), total: number(calls) }) : undefined },
      { label: labels.requests, value: number(total.requests), detail: total.external_turns ? t('statistics.externalTurns', { count: total.external_turns }) : undefined },
      { label: t('statistics.unsuccessful'), value: total.requests ? percent(total.failed_requests / total.requests) : '—', detail: total.requests ? `${number(total.failed_requests)} / ${number(total.requests)}` : undefined },
      { label: t('statistics.cacheHit'), value: total.prompt ? percent(total.cached / total.prompt) : '—' },
      { label: t('statistics.firstChunk'), value: seconds(firstChunk(total)), detail: total.ttft_requests ? t('statistics.samples', { count: total.ttft_requests }) : undefined },
      { label: t('statistics.latencyP95'), value: seconds(total.latency_p95_ms), detail: total.requests ? t('statistics.samples', { count: total.requests }) : undefined },
    ]} />
    <div className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
      <ChartPanel title={t('statistics.usageTrend')} action={
        <ToggleGroup type="single" size="sm" variant="outline" spacing={0} value={metric} onValueChange={value => { if (value) setMetric(value as Metric) }} aria-label={t('statistics.metric')}>
          {(['tokens', 'requests', 'latency'] as const).map(value => <ToggleGroupItem key={value} value={value}>{labels[value]}</ToggleGroupItem>)}
        </ToggleGroup>
      }>
        {(metric === 'tokens' ? tokenTotal !== null : total.requests > 0) ? <ChartContainer config={{ value: { label: labels[metric], color: 'var(--foreground)' } }} className="h-48 w-full aspect-auto">
          <BarChart accessibilityLayer data={days} margin={{ top: 8, right: 8, left: 0 }}>
            <CartesianGrid vertical={false} />
            <XAxis dataKey="date" tickLine={false} axisLine={false} minTickGap={28} tickFormatter={value => value.slice(5).replace('-', '/')} />
            <YAxis tickLine={false} axisLine={false} width={48} allowDecimals={metric !== 'requests'} tickFormatter={value => metric === 'latency' ? seconds(Number(value)) : compact(value)} />
            <ChartTooltip content={<ChartTooltipContent formatter={value => <span className="font-mono tabular-nums">{metric === 'latency' ? seconds(Number(value)) : number(Number(value))}</span>} />} />
            <Bar isAnimationActive={false} dataKey="value" fill="var(--color-value)" radius={3} maxBarSize={28} />
          </BarChart>
        </ChartContainer> : <ChartEmpty label={t(calls ? metric === 'tokens' ? 'statistics.noReportedTokens' : 'statistics.noNativeCalls' : 'statistics.noCalls')} />}
      </ChartPanel>
      <ChartPanel title={t('statistics.tokenDistribution')}>
        {tokenParts.length ? <div className="flex flex-col gap-4 py-2">
          <div className="flex h-3 overflow-hidden rounded-sm" aria-hidden="true">{tokenParts.map(part => <div key={part.name} style={{ width: `${100 * part.value / partsTotal}%`, backgroundColor: part.color }} />)}</div>
          <dl className="flex flex-col gap-3">{tokenParts.map(part => <div key={part.name} className="flex items-center justify-between gap-3 text-xs">
            <dt className="flex items-center gap-2"><span className="size-2 shrink-0 rounded-xs" style={{ backgroundColor: part.color }} />{part.name}</dt>
            <dd className="text-right tabular-nums"><span title={number(part.value)}>{compact(part.value)}</span><span className="text-muted-foreground"> · {percent(part.value / partsTotal)}</span></dd>
          </div>)}</dl>
        </div> : <ChartEmpty label={t(tokenTotal === null ? 'statistics.noReportedTokens' : 'statistics.zeroTokens')} />}
      </ChartPanel>
    </div>
    <ChartPanel title={t('statistics.configuredModels')}>
      {data.models.length ? <StatisticsTable label={t('statistics.configuredModels')} rows={data.models} rowKey={item => `${item.runtime}:${item.name}`} defaultSort="tokens" columns={[
        { id: 'name', label: t('statistics.model'), render: item => <><div>{item.name || t('statistics.unknownModel')}</div><div className="text-xs text-muted-foreground">{t(`statistics.runtime.${item.runtime}`)}</div></> },
        { id: 'tokens', label: labels.tokens, sortValue: reportedTokens, render: item => <>
          <div title={reportedTokens(item) === null ? undefined : number(item.tokens)}>{reportedTokens(item) === null ? '—' : compact(item.tokens)}</div>
          {reportedTokens(item) !== null && <div className="text-xs text-muted-foreground">{t('statistics.inputOutput', { input: compact(item.prompt), output: compact(item.output) })}</div>}
          {item.unknown_usage > 0 && <div className="text-xs text-muted-foreground">{t('statistics.unknownUsage', { count: item.unknown_usage })}</div>}
        </> },
        { id: 'requests', label: labels.requests, sortValue: item => item.requests || null, render: item => item.requests ? number(item.requests) : '—' },
        ...(total.external_turns ? [{ id: 'turns', label: t('statistics.externalTurnsLabel'), sortValue: (item: UsageTotals) => item.external_turns || null, render: (item: UsageTotals) => item.external_turns ? number(item.external_turns) : '—' }] : []),
        { id: 'cache', label: t('statistics.cacheHit'), sortValue: item => item.prompt ? item.cached / item.prompt : null, render: item => item.prompt ? percent(item.cached / item.prompt) : '—' },
        { id: 'first', label: t('statistics.firstChunk'), sortValue: firstChunk, render: item => <><div>{seconds(firstChunk(item))}</div>{item.ttft_requests > 0 && <div className="text-xs text-muted-foreground">{t('statistics.samples', { count: item.ttft_requests })}</div>}</> },
        { id: 'latency', label: t('statistics.latencyPercentiles'), sortValue: item => item.latency_p95_ms, render: item => <><div>{seconds(item.latency_p50_ms)} / {seconds(item.latency_p95_ms)}</div>{item.requests > 0 && <div className="text-xs text-muted-foreground">{t('statistics.meanLatency', { value: seconds(mean(item)) })}</div>}</> },
        { id: 'unsuccessful', label: t('statistics.unsuccessful'), sortValue: item => item.requests ? item.failed_requests / item.requests : null, render: item => item.requests ? <><div>{number(item.failed_requests)} / {number(item.requests)}</div><div className="text-xs text-muted-foreground">{percent(item.failed_requests / item.requests)}</div></> : '—' },
      ]} /> : <ChartEmpty label={t('statistics.noCalls')} />}
    </ChartPanel>
    <div className="flex flex-col gap-1 text-xs text-muted-foreground">
      <p>{total.external_turns ? t('statistics.externalCoverage', { count: total.external_turns }) : t('statistics.newRecords')}</p>
      <p>{t('statistics.performanceCoverage')}</p>
    </div>
  </div>
}
