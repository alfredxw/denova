import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart'
import { ChartEmpty, ChartPanel, MetricStrip, RankingChart } from './StatisticsCharts'
import type { StatisticsReport } from './api'

// Intent: players see when their stories moved forward and which they explored.
// Hierarchy: daily turns lead; a quiet story ranking follows. Palette/surfaces:
// existing neutral Cards, with green reserved for activity. Border depth, 14px
// labels, 24px values and the 4px/24px spacing rhythm match shared Statistics.
export function GameStatistics({ data }: { data: StatisticsReport['game'] }) {
  const { t, i18n } = useTranslation()
  const number = (value: number) => new Intl.NumberFormat(i18n.language).format(value)
  const turns = data.days.reduce((sum, day) => sum + day.turns, 0)
  const activeDays = data.days.filter(day => day.turns > 0).length
  const activeStories = data.stories.filter(story => story.turns > 0)
  const distribution = activeStories.slice(0, 6).map(story => ({ id: `${story.project_id}/${story.id}`, name: story.name, value: story.turns }))
  if (activeStories.length > 6) distribution.push({ id: 'other', name: t('statistics.other'), value: activeStories.slice(6).reduce((sum, story) => sum + story.turns, 0) })

  return <div className="flex flex-col gap-6">
    <MetricStrip items={[
      { label: t('statistics.gameTurns'), value: number(turns) },
      { label: t('statistics.activeDays'), value: number(activeDays) },
      { label: t('statistics.activeStories'), value: number(activeStories.length) },
      { label: t('statistics.newBranches'), value: number(data.new_branches) },
    ]} />
    <ChartPanel title={t('statistics.gameTrend')}>
      {turns ? <ChartContainer config={{ turns: { label: t('statistics.gameTurns'), color: 'var(--nova-success)' } }} className="h-72 w-full aspect-auto">
        <BarChart accessibilityLayer data={data.days} margin={{ top: 12, right: 8, left: 8 }}>
          <CartesianGrid vertical={false} />
          <XAxis dataKey="date" tickLine={false} axisLine={false} minTickGap={28} tickFormatter={value => value.slice(5).replace('-', '/')} />
          <YAxis allowDecimals={false} tickLine={false} axisLine={false} width={40} tickFormatter={value => new Intl.NumberFormat(i18n.language, { notation: 'compact' }).format(value)} />
          <ChartTooltip content={<ChartTooltipContent />} />
          <Bar isAnimationActive={false} dataKey="turns" fill="var(--color-turns)" radius={3} maxBarSize={36} />
        </BarChart>
      </ChartContainer> : <ChartEmpty label={t('statistics.noGameActivity')} />}
      <p className="pt-3 text-xs text-muted-foreground">{t('statistics.gameCoverage')}</p>
    </ChartPanel>
    <ChartPanel title={t('statistics.storyActivity')}>
      <RankingChart data={distribution} label={t('statistics.gameTurns')} />
    </ChartPanel>
  </div>
}
