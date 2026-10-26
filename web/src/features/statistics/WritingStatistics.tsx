import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, Cell, Pie, PieChart, ReferenceLine, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { ChartEmpty, ChartPanel, MetricStrip, RankingChart } from './StatisticsCharts'
import type { StatisticsReport } from './api'

// The signed saved-volume chart is the focal point. Green/red are semantic
// changes; current stock and chapter status use neutral surfaces and marks.
export function WritingStatistics({ data }: { data: StatisticsReport['writing'] }) {
  const { t, i18n } = useTranslation()
  const number = (value: number) => new Intl.NumberFormat(i18n.language).format(value)
  const words = data.projects.reduce((sum, item) => sum + item.words, 0)
  const chapters = data.projects.reduce((sum, item) => sum + item.chapters, 0)
  const confirmed = data.projects.reduce((sum, item) => sum + item.confirmed, 0)
  const observed = data.days.filter(item => item.delta !== null)
  const delta = observed.reduce((sum, item) => sum + (item.delta ?? 0), 0)
  const progress = [
    { name: t('statistics.confirmed'), value: confirmed, fill: 'var(--nova-success)' },
    { name: t('statistics.draft'), value: chapters - confirmed, fill: 'var(--chart-2)' },
  ]
  const distribution = data.projects.slice(0, 6).map(item => ({ id: item.id, name: item.name, value: item.words }))
  if (data.projects.length > 6) distribution.push({ id: 'other', name: t('statistics.other'), value: data.projects.slice(6).reduce((sum, item) => sum + item.words, 0) })

  return <div className="flex flex-col gap-6">
    <MetricStrip items={[
      { label: t('statistics.manuscript'), value: number(words) },
      { label: t('statistics.netChange'), value: observed.length ? `${delta > 0 ? '+' : ''}${number(delta)}` : '—' },
      { label: t('statistics.chapters'), value: number(chapters) },
      { label: t('statistics.confirmed'), value: `${number(confirmed)} / ${number(chapters)}` },
    ]} />
    <ChartPanel title={t('statistics.writingTrend')}>
      {observed.length ? <ChartContainer config={{ delta: { label: t('statistics.netChange'), color: 'var(--nova-success)' } }} className="h-72 w-full aspect-auto">
        <BarChart accessibilityLayer data={data.days} margin={{ top: 12, right: 8, left: 8 }}>
          <CartesianGrid vertical={false} />
          <XAxis dataKey="date" tickLine={false} axisLine={false} minTickGap={28} tickFormatter={value => value.slice(5).replace('-', '/')} />
          <YAxis tickLine={false} axisLine={false} width={52} tickFormatter={value => new Intl.NumberFormat(i18n.language, { notation: 'compact' }).format(value)} />
          <ChartTooltip content={<ChartTooltipContent />} />
          <ReferenceLine y={0} stroke="var(--border)" />
          <Bar isAnimationActive={false} dataKey="delta" radius={3} maxBarSize={36}>{data.days.map(day => <Cell key={day.date} fill={(day.delta ?? 0) < 0 ? 'var(--nova-danger)' : 'var(--nova-success)'} />)}</Bar>
        </BarChart>
      </ChartContainer> : <ChartEmpty label={t('statistics.noWritingHistory')} />}
      <p className="pt-3 text-xs text-muted-foreground">{t('statistics.writingCoverage')}</p>
    </ChartPanel>
    <div className="grid min-w-0 gap-6 lg:grid-cols-2">
      <ChartPanel title={t('statistics.manuscriptDistribution')}>
        {distribution.length ? <RankingChart data={distribution} label={t('statistics.manuscript')} /> : <ChartEmpty label={t('statistics.noBooks')} />}
      </ChartPanel>
      <ChartPanel title={t('statistics.chapterProgress')}>
        {chapters ? <ChartContainer config={{ [t('statistics.confirmed')]: { label: t('statistics.confirmed'), color: 'var(--nova-success)' }, [t('statistics.draft')]: { label: t('statistics.draft'), color: 'var(--chart-2)' } }} className="h-60 w-full aspect-auto">
          <PieChart accessibilityLayer><ChartTooltip content={<ChartTooltipContent nameKey="name" />} />
            <Pie isAnimationActive={false} data={progress} dataKey="value" nameKey="name" innerRadius="57%" outerRadius="80%" strokeWidth={0} paddingAngle={2} />
            <ChartLegend content={<ChartLegendContent nameKey="name" />} />
          </PieChart>
        </ChartContainer> : <ChartEmpty label={t('statistics.noChapters')} />}
      </ChartPanel>
    </div>
  </div>
}
