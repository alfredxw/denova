import { useMemo, useRef, useState } from 'react'
import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { Button } from '@/components/ui/button'
import { useIsMobile } from '@/hooks/useIsMobile'
import type { AgentRunTrace } from '@/lib/api'
import { cn } from '@/lib/utils'
import { TrajectoryContentInspector } from './TrajectoryContentInspector'
import { TrajectoryContentLedger } from './TrajectoryContentLedger'
import { TrajectoryInspector } from './TrajectoryInspector'
import { TrajectoryJSONBlock } from './TrajectoryInspectorParts'
import { TrajectoryLedger } from './TrajectoryLedger'
import { TrajectoryTimeline, type TrajectoryRange } from './TrajectoryTimeline'
import {
  analyzeTrajectory,
  formatTrajectoryDuration,
  projectTimeline,
  timelineRangeSpanIDs,
  type TrajectoryAnalysis,
  type TrajectoryTimelineMode,
} from './trajectory-analysis'
import { analyzeTrajectoryContent, type TrajectoryContentSelection } from './trajectory-content'

type TrajectoryWorkspaceView = 'records' | 'timeline' | 'raw'
export type TrajectoryDetailPlacement = 'inline' | 'side'

interface TrajectoryTraceWorkspaceProps {
  trace: AgentRunTrace
}

/** High-density trace workspace with on-demand timing and reusable inline/side details. */
export function TrajectoryTraceWorkspace({ trace }: TrajectoryTraceWorkspaceProps) {
  const { t } = useTranslation()
  const isCompact = useIsMobile()
  const [workspaceView, setWorkspaceView] = useState<TrajectoryWorkspaceView>('records')
  const [detailPlacement, setDetailPlacement] = useState<TrajectoryDetailPlacement>('inline')
  const [timelineMode, setTimelineMode] = useState<TrajectoryTimelineMode>('actual')
  const [range, setRange] = useState<TrajectoryRange | null>(null)
  const [selectedSpanID, setSelectedSpanID] = useState('')
  const [selectedEventID, setSelectedEventID] = useState('')
  const [contentSelection, setContentSelection] = useState<TrajectoryContentSelection | null>(null)
  const openInspectorRef = useRef<() => void>(() => {})
  const closeInspectorPaneRef = useRef<() => void>(() => {})

  const analysis = useMemo(() => analyzeTrajectory(trace), [trace])
  const content = useMemo(() => analyzeTrajectoryContent(trace, analysis), [analysis, trace])
  const projection = useMemo(() => projectTimeline(analysis, timelineMode), [analysis, timelineMode])
  const rangeSpanIDs = useMemo(() => range
    ? timelineRangeSpanIDs(projection, range.start, range.end)
    : null, [projection, range])
  const inspectableEntries = useMemo(() => content.requests.flatMap((request) => [
    ...request.entries,
    ...request.debugInputEntries,
    ...request.debugOutputEntries,
  ]), [content.requests])
  const selectedSpan = analysis.spans.find((span) => span.id === selectedSpanID) ?? null
  const selectedEvent = analysis.events.find((event) => event.id === selectedEventID) ?? null
  const selectedEntry = contentSelection?.type === 'entry'
    ? inspectableEntries.find((entry) => entry.id === contentSelection.id) ?? null
    : null
  const selectedTool = contentSelection?.type === 'tool'
    ? content.toolCalls.find((exchange) => exchange.id === contentSelection.id) ?? null
    : null
  const contentInspectorOpen = Boolean(selectedEntry || selectedTool)
  const analysisInspectorOpen = Boolean(selectedSpan || selectedEvent)
  const inspectorOpen = workspaceView === 'records'
    ? detailPlacement === 'side' && contentInspectorOpen
    : workspaceView === 'timeline' && analysisInspectorOpen
  const inspectorTitle = selectedTool?.call.name
    || (selectedEntry ? `${t(`trajectory.records.kind.${selectedEntry.kind}`)} · ${t('trajectory.records.request', { index: selectedEntry.requestIndex })}` : '')
    || selectedSpan?.label
    || selectedEvent?.label
    || t('trajectory.inspector.title')

  const closeInspector = () => {
    setContentSelection(null)
    setSelectedSpanID('')
    setSelectedEventID('')
  }

  const changeView = (view: TrajectoryWorkspaceView) => {
    setWorkspaceView(view)
    closeInspector()
    closeInspectorPaneRef.current()
  }

  const changeDetailPlacement = (placement: TrajectoryDetailPlacement) => {
    setDetailPlacement(placement)
    if (placement === 'inline') closeInspectorPaneRef.current()
    else if (contentSelection) openInspectorRef.current()
  }

  const inlineInspector = detailPlacement === 'inline' && contentInspectorOpen ? (
    <TrajectoryContentInspector
      entry={selectedEntry}
      exchange={selectedTool}
      variant="inline"
      onClose={closeInspector}
    />
  ) : null

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col">
      <TraceMetricStrip analysis={analysis} />
      <div className="flex h-8 shrink-0 items-stretch border-b border-[var(--nova-border)] px-2" role="tablist" aria-label={t('trajectory.workspace.tabs')}>
        {(['records', 'timeline', 'raw'] as const).map((view) => (
          <button
            key={view}
            type="button"
            role="tab"
            aria-selected={workspaceView === view}
            onClick={() => changeView(view)}
            className={cn(
              'relative min-w-0 px-2.5 text-[10px] text-[var(--nova-text-faint)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none',
              workspaceView === view && 'bg-[var(--nova-active)] text-[var(--nova-text)] after:absolute after:inset-x-2 after:bottom-0 after:h-px after:bg-[var(--nova-text-muted)]',
            )}
          >
            {t(`trajectory.view.${view}`)}
          </button>
        ))}
        {workspaceView === 'records' && (
          <div className="ml-auto flex items-center gap-0.5 pl-2" role="group" aria-label={t('trajectory.detailPlacement.label')}>
            <span className="hidden px-1 text-[9px] text-[var(--nova-text-faint)] sm:inline">{t('trajectory.detailPlacement.label')}</span>
            {(['inline', 'side'] as const).map((placement) => (
              <Button
                key={placement}
                type="button"
                size="xs"
                variant="ghost"
                className={cn('h-5 px-1.5 text-[9px] focus-visible:ring-0 focus-visible:outline-none', detailPlacement === placement && 'bg-[var(--nova-active)] text-[var(--nova-text)]')}
                aria-pressed={detailPlacement === placement}
                onClick={() => changeDetailPlacement(placement)}
              >
                {t(`trajectory.detailPlacement.${placement}`)}
              </Button>
            ))}
          </div>
        )}
        <span className="ml-2 hidden min-w-0 max-w-52 self-center truncate font-mono text-[8px] text-[var(--nova-text-faint)] xl:block">{trace.summary.id}</span>
      </div>
      {trace.truncated && (
        <div className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-warning-bg)] px-3 py-1 text-[10px] text-[var(--nova-warning)]">
          <AlertTriangle className="size-3" />{t('trajectory.truncated')}
        </div>
      )}
      <div className="relative min-h-0 flex-1">
        <AdaptiveSurface
          className="h-full min-h-0 min-w-0"
          mainClassName="min-h-0 min-w-0"
          mobilePaneScope="surface"
          rightResize={{
            layoutKey: 'trajectory-record-inspector-layout',
            label: t('trajectory.inspector.resize'),
            defaultSize: '440px',
            minSize: '320px',
            maxSize: '58%',
            mainMinSize: '360px',
          }}
          right={{
            id: 'trajectory-record-inspector',
            title: inspectorTitle,
            side: 'right',
            enabled: workspaceView !== 'raw',
            desktopVisible: inspectorOpen,
            desktopClassName: 'bg-[var(--nova-surface-2)]',
            mobileClassName: 'bg-[var(--nova-surface-2)]',
            onClose: closeInspector,
            content: workspaceView === 'records' ? (
              <TrajectoryContentInspector entry={selectedEntry} exchange={selectedTool} showHeader={!isCompact} onClose={closeInspector} />
            ) : (
              <TrajectoryInspector span={selectedSpan} event={selectedEvent} showHeader={!isCompact} onClose={closeInspector} />
            ),
          }}
        >
          {({ openRight, closePane }) => {
            openInspectorRef.current = openRight
            closeInspectorPaneRef.current = closePane

            if (workspaceView === 'raw') {
              return (
                <section className="h-full min-h-0 overflow-auto bg-[var(--nova-surface-2)] p-2" aria-label={t('trajectory.view.raw')}>
                  <TrajectoryJSONBlock value={trace} className="min-h-full" />
                </section>
              )
            }

            if (workspaceView === 'timeline') {
              return (
                <div className="flex h-full min-h-0 flex-col">
                  <TimelineScaleControl
                    mode={timelineMode}
                    onChange={(nextMode) => {
                      setTimelineMode(nextMode)
                      setRange(null)
                    }}
                  />
                  <TrajectoryTimeline
                    projection={projection}
                    selectedSpanID={selectedSpanID}
                    range={range}
                    onRangeChange={setRange}
                    onSpanSelect={(spanID) => {
                      setSelectedSpanID(spanID)
                      setSelectedEventID('')
                      setContentSelection(null)
                      openRight()
                    }}
                  />
                  <div className="min-h-0 flex-1">
                    <TrajectoryLedger
                      analysis={analysis}
                      selectedSpanID={selectedSpanID}
                      selectedEventID={selectedEventID}
                      rangeSpanIDs={rangeSpanIDs}
                      onSpanSelect={(spanID) => {
                        setSelectedSpanID(spanID)
                        setSelectedEventID('')
                        setContentSelection(null)
                        openRight()
                      }}
                      onEventSelect={(eventID) => {
                        setSelectedEventID(eventID)
                        setSelectedSpanID('')
                        setContentSelection(null)
                        openRight()
                      }}
                    />
                  </div>
                </div>
              )
            }

            return (
              <TrajectoryContentLedger
                content={content}
                selection={contentSelection}
                rangeSpanIDs={rangeSpanIDs}
                inlineInspector={inlineInspector}
                onSelect={(selection) => {
                  const sameSelection = contentSelection?.type === selection.type && contentSelection.id === selection.id
                  if (detailPlacement === 'inline' && sameSelection) {
                    setContentSelection(null)
                    return
                  }
                  setContentSelection(selection)
                  setSelectedSpanID('')
                  setSelectedEventID('')
                  if (detailPlacement === 'side') openRight()
                }}
              />
            )
          }}
        </AdaptiveSurface>
      </div>
    </div>
  )
}

function TimelineScaleControl({ mode, onChange }: { mode: TrajectoryTimelineMode; onChange: (mode: TrajectoryTimelineMode) => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex h-8 shrink-0 items-center gap-0.5 border-b border-[var(--nova-border)] px-2">
      <span className="mr-1 text-[9px] text-[var(--nova-text-faint)]">{t('trajectory.timeline.scale')}</span>
      {(['actual', 'duration', 'sequence'] as const).map((value) => (
        <Button
          key={value}
          type="button"
          size="xs"
          variant="ghost"
          className={cn('h-5 px-1.5 text-[9px] focus-visible:ring-0 focus-visible:outline-none', mode === value && 'bg-[var(--nova-active)]')}
          aria-pressed={mode === value}
          onClick={() => onChange(value)}
        >
          {t(`trajectory.timeline.mode.${value}`)}
        </Button>
      ))}
    </div>
  )
}

function TraceMetricStrip({ analysis }: { analysis: TrajectoryAnalysis }) {
  const { t } = useTranslation()
  const metrics = analysis.metrics
  const items = [
    [t('trajectory.metric.total'), formatTrajectoryDuration(metrics.totalMs)],
    [t('trajectory.metric.model'), `${metrics.modelCalls} · ${formatTrajectoryDuration(metrics.modelMs)}`],
    [t('trajectory.metric.tools'), `${metrics.toolCalls} · ${formatTrajectoryDuration(metrics.toolMs)}`],
    [t('trajectory.metric.ttft'), `P50 ${formatTrajectoryDuration(metrics.p50TTFTMs)} · P95 ${formatTrajectoryDuration(metrics.p95TTFTMs)}`],
    [t('trajectory.metric.throughput'), metrics.averageThroughput === null ? '—' : `${metrics.averageThroughput.toFixed(1)} tok/s`],
    [t('trajectory.metric.idle'), formatTrajectoryDuration(metrics.idleMs)],
    [t('trajectory.metric.cache'), metrics.promptTokens > 0 ? `${(metrics.cacheHitRate * 100).toFixed(1)}%` : '—'],
  ]
  return (
    <div className="flex shrink-0 divide-x divide-[var(--nova-border-soft)] overflow-x-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      {items.map(([label, value]) => (
        <div key={label} className="flex h-7 min-w-max items-center gap-1.5 px-2.5">
          <div className="text-[8px] uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">{label}</div>
          <div className="font-mono text-[9px] text-[var(--nova-text)]">{value}</div>
        </div>
      ))}
    </div>
  )
}
