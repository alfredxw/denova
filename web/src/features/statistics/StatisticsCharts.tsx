import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, Cell, XAxis, YAxis } from 'recharts'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from '@/components/ui/chart'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'

// Hierarchy: full-width chart area, compact title, controls at the trailing edge.
// Neutral card surfaces and 16px padding match the workbench's quiet depth.
export function ChartPanel({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return <Card><CardHeader className="flex flex-wrap items-center justify-between gap-3"><CardTitle>{title}</CardTitle>{action}</CardHeader><CardContent>{children}</CardContent></Card>
}

export function ChartEmpty({ label }: { label: string }) {
  return <Empty className="min-h-60"><EmptyHeader><EmptyTitle>{label}</EmptyTitle></EmptyHeader></Empty>
}

export function MetricStrip({ items }: { items: Array<{ label: string; value: string; detail?: string }> }) {
  return <dl className="grid grid-cols-2 gap-x-6 gap-y-4 pb-1 sm:grid-cols-4">{items.map(item => (
    <div key={item.label} className="flex min-w-0 flex-col gap-1">
      <dt className="text-sm text-muted-foreground">{item.label}</dt>
      <dd className="text-2xl font-semibold tracking-tight tabular-nums">{item.value}</dd>
      {item.detail && <dd className="text-xs text-muted-foreground">{item.detail}</dd>}
    </div>
  ))}</dl>
}

export function RankingChart({ data, label, unit = '', allowDecimals = false }: { data: Array<{ id: string; name: string; value: number }>; label: string; unit?: string; allowDecimals?: boolean }) {
  const { t, i18n } = useTranslation()
  const config = { value: { label, color: 'var(--chart-2)' } } satisfies ChartConfig
  if (!data.length || !data.some(item => item.value > 0)) return <ChartEmpty label={t('statistics.empty')} />
  return <ChartContainer config={config} className="aspect-auto w-full" style={{ height: Math.max(220, data.length * 44) }}>
    <BarChart accessibilityLayer layout="vertical" data={data} margin={{ left: 0, right: 16 }}>
      <CartesianGrid horizontal={false} />
      <XAxis type="number" allowDecimals={allowDecimals} tickLine={false} axisLine={false} tickFormatter={value => `${new Intl.NumberFormat(i18n.language, { notation: 'compact' }).format(value)}${unit}`} />
      <YAxis type="category" dataKey="name" tickLine={false} axisLine={false} width={125} tick={({ x, y, payload }) => (
        <foreignObject x={Number(x) - 120} y={Number(y) - 10} width={112} height={20}>
          <div className="truncate text-right text-xs leading-5 text-muted-foreground" title={String(payload.value)}>{payload.value}</div>
        </foreignObject>
      )} />
      <ChartTooltip content={<ChartTooltipContent formatter={value => <span className="font-mono tabular-nums">{new Intl.NumberFormat(i18n.language).format(Number(value))}{unit}</span>} />} cursor={false} />
      <Bar isAnimationActive={false} dataKey="value" fill="var(--color-value)" radius={3} maxBarSize={20}>{data.map((item, index) => <Cell key={item.id} fill={index === 0 ? 'var(--foreground)' : 'var(--chart-2)'} />)}</Bar>
    </BarChart>
  </ChartContainer>
}
