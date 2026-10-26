import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, Pie, PieChart, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { ChartEmpty, ChartPanel, MetricStrip, RankingChart } from './StatisticsCharts'
import type { StatisticsReport, UsageTotals } from './api'

type Metric = 'tokens' | 'requests' | 'latency'

// The Magpie-inspired hierarchy keeps totals unboxed, a dominant trend and two
// supporting distributions. Chart tokens are neutral; cache reads use green.
export function ModelStatistics({ data }: { data: StatisticsReport['usage'] }) {
  const { t, i18n } = useTranslation()
  const [metric, setMetric] = useState<Metric>('tokens')
  const number = (value: number) => new Intl.NumberFormat(i18n.language).format(value)
  const compact = (value: number) => new Intl.NumberFormat(i18n.language, { notation: 'compact', maximumFractionDigits: 1 }).format(value)
  const total = data.totals
  const metricValue = (item: UsageTotals): number | null => {
    switch (metric) {
      case 'tokens': return item.unknown_usage === item.requests + item.external_turns ? null : item.tokens
      case 'requests': return item.requests
      case 'latency': return item.requests ? item.duration_ms / item.requests / 1000 : null
    }
  }
  const days = data.days.map(item => ({ ...item, value: metricValue(item) }))
  const models = data.models.map(item => ({ id: `${item.runtime}:${item.name}`, name: `${item.name || t('statistics.unknownModel')} · ${t(`statistics.runtime.${item.runtime}`)}`, value: metricValue(item) ?? 0 })).sort((a, b) => b.value - a.value).slice(0, 6)
  const hasRequests = total.requests + total.external_turns > 0
  const hasReportedUsage = hasRequests && total.unknown_usage < total.requests + total.external_turns
  const labels = { tokens: t('statistics.tokens'), requests: t('statistics.requests'), latency: t('statistics.latency') }
  const tokenParts = [
    { name: t('statistics.uncached'), value: Math.max(0, total.prompt - total.cached), fill: 'var(--foreground)' },
    { name: t('statistics.cached'), value: total.cached, fill: 'var(--nova-success)' },
    { name: t('statistics.output'), value: total.output, fill: 'var(--chart-2)' },
    { name: t('statistics.otherTokens'), value: Math.max(0, total.tokens - total.prompt - total.output), fill: 'var(--chart-3)' },
  ].filter(item => item.value > 0)
  const partsConfig = Object.fromEntries(tokenParts.map(item => [item.name, { label: item.name, color: item.fill }]))

  return <div className="flex flex-col gap-6">
    <MetricStrip items={[
      { label: labels.tokens, value: hasReportedUsage ? compact(total.tokens) : '—', detail: total.unknown_usage ? t('statistics.unknownUsage', { count: total.unknown_usage }) : undefined },
      { label: labels.requests, value: number(total.requests), detail: total.failed_requests ? t('statistics.failures', { count: total.failed_requests }) : undefined },
      { label: t('statistics.cacheHit'), value: total.prompt ? `${(100 * total.cached / total.prompt).toFixed(1)}%` : '—' },
      { label: labels.latency, value: total.requests ? t('statistics.seconds', { value: (total.duration_ms / total.requests / 1000).toFixed(1) }) : '—', detail: total.ttft_requests ? t('statistics.ttft', { value: (total.ttft_ms / total.ttft_requests / 1000).toFixed(1), count: total.ttft_requests }) : undefined },
    ]} />
    <ChartPanel title={t('statistics.usageTrend')} action={
      <ToggleGroup type="single" size="sm" variant="outline" spacing={0} value={metric} onValueChange={value => { if (value) setMetric(value as Metric) }} aria-label={t('statistics.metric')}>
        {(['tokens', 'requests', 'latency'] as const).map(value => <ToggleGroupItem key={value} value={value}>{labels[value]}</ToggleGroupItem>)}
      </ToggleGroup>
    }>
      {hasRequests && (metric === 'tokens' ? hasReportedUsage : total.requests > 0) ? <ChartContainer config={{ value: { label: labels[metric], color: 'var(--foreground)' } }} className="h-72 w-full aspect-auto">
        <BarChart accessibilityLayer data={days} margin={{ top: 12, right: 8, left: 8 }}>
          <CartesianGrid vertical={false} />
          <XAxis dataKey="date" tickLine={false} axisLine={false} minTickGap={28} tickFormatter={value => value.slice(5).replace('-', '/')} />
          <YAxis tickLine={false} axisLine={false} width={52} tickFormatter={value => metric === 'latency' ? `${Number(value).toFixed(1)}s` : compact(value)} />
          <ChartTooltip content={<ChartTooltipContent formatter={value => <span className="font-mono tabular-nums">{metric === 'latency' ? t('statistics.seconds', { value: Number(value).toFixed(1) }) : number(Number(value))}</span>} />} />
          <Bar isAnimationActive={false} dataKey="value" fill="var(--color-value)" radius={3} maxBarSize={36} />
        </BarChart>
      </ChartContainer> : <ChartEmpty label={t(metric === 'tokens' && hasRequests ? 'statistics.noReportedTokens' : metric !== 'tokens' && hasRequests ? 'statistics.noNativeCalls' : 'statistics.noCalls')} />}
      <p className="pt-3 text-xs text-muted-foreground">{total.external_turns ? t('statistics.externalCoverage', { count: total.external_turns }) : t('statistics.newRecords')}</p>
    </ChartPanel>
    <div className="grid min-w-0 gap-6 lg:grid-cols-2">
      <ChartPanel title={t('statistics.configuredModels')}><RankingChart data={models} allowDecimals={metric === 'latency'} label={labels[metric]} unit={metric === 'latency' ? t('statistics.secondsUnit') : undefined} /></ChartPanel>
      <ChartPanel title={t('statistics.tokenDistribution')}>
        {tokenParts.length ? <ChartContainer config={partsConfig} className="h-60 w-full aspect-auto">
          <PieChart accessibilityLayer><ChartTooltip content={<ChartTooltipContent nameKey="name" />} />
            <Pie isAnimationActive={false} data={tokenParts} dataKey="value" nameKey="name" innerRadius="57%" outerRadius="80%" strokeWidth={0} paddingAngle={2} />
            <ChartLegend content={<ChartLegendContent nameKey="name" />} />
          </PieChart>
        </ChartContainer> : <ChartEmpty label={t(hasReportedUsage ? 'statistics.zeroTokens' : 'statistics.noReportedTokens')} />}
      </ChartPanel>
    </div>
  </div>
}
