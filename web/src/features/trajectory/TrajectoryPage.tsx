import { useCallback, useEffect, useMemo, useState } from 'react'
import { Activity, AlertTriangle, Download, ListTree, RefreshCw, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { downloadAgentRunTrace, exportAgentRunTrace, getAgentRunTrace, getGlobalAgentRunTraces } from '@/lib/api'
import type { AgentRunTrace, GlobalAgentRunTraceIssue, GlobalAgentRunTraceSummary } from '@/lib/api'
import { cn } from '@/lib/utils'
import { TrajectoryRunList } from './TrajectoryRunList'
import { TrajectoryRunHeader } from './TrajectoryRunHeader'
import { TrajectoryTraceWorkspace } from './TrajectoryTraceWorkspace'
import { useTrajectoryNavigation, type TrajectoryNavigationTarget } from './trajectory-navigation'

interface TrajectoryPageProps {
  onClose?: () => void
}

/** Global read-only evidence workspace. Agent behavior is managed from Agents. */
export function TrajectoryPage({ onClose }: TrajectoryPageProps) {
  const { t } = useTranslation()
  const trajectoryNavigation = useTrajectoryNavigation()
  const [runsOpen, setRunsOpen] = useState(true)
  const [runs, setRuns] = useState<GlobalAgentRunTraceSummary[]>([])
  const [issues, setIssues] = useState<GlobalAgentRunTraceIssue[]>([])
  const [selectedRunURI, setSelectedRunURI] = useState('')
  const [agentFilter, setAgentFilter] = useState('all')
  const [trace, setTrace] = useState<AgentRunTrace | null>(null)
  const [loadingRuns, setLoadingRuns] = useState(false)
  const [loadingTrace, setLoadingTrace] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const agentOptions = useMemo(() => {
    const values = new Set<string>()
    for (const run of runs) {
      const id = run.agent_kind?.trim()
      if (id) values.add(id)
    }
    return [...values].sort((left, right) => left.localeCompare(right))
  }, [runs])
  const visibleRuns = useMemo(
    () => agentFilter === 'all' ? runs : runs.filter((run) => run.agent_kind === agentFilter),
    [agentFilter, runs],
  )
  const selectedRun = useMemo(
    () => runs.find((run) => run.trajectory_uri === selectedRunURI) ?? null,
    [runs, selectedRunURI],
  )

  const loadRuns = useCallback(async (preferred?: { runURI?: string; target?: TrajectoryNavigationTarget }) => {
    setLoadingRuns(true)
    setError(null)
    try {
      const catalog = await getGlobalAgentRunTraces(100, preferred?.target)
      setRuns(catalog.runs)
      setIssues(catalog.issues ?? [])
      const validURIs = new Set(catalog.runs.map((run) => run.trajectory_uri))
      const target = preferred?.target
      const targetRun = target
        ? catalog.runs.find((run) => run.project_id === target.projectId && run.id === target.runId)
        : undefined
      if (target && !targetRun) {
        setSelectedRunURI('')
        setTrace(null)
        toast.warning(t('trajectory.navigation.notFound'))
        return
      }
      const nextRunURI = targetRun?.trajectory_uri
        ?? (preferred?.runURI && validURIs.has(preferred.runURI) ? preferred.runURI : catalog.runs[0]?.trajectory_uri ?? '')
      setSelectedRunURI(nextRunURI)
      if (!nextRunURI) setTrace(null)
    } catch (cause) {
      console.error('[TrajectoryPage.tsx] failed to load the global Agent Run catalog', { cause })
      setError(errorMessage(cause))
      setRuns([])
      setIssues([])
      setSelectedRunURI('')
      setTrace(null)
    } finally {
      setLoadingRuns(false)
    }
  }, [t])

  useEffect(() => {
    void loadRuns({ target: trajectoryNavigation.intent ?? undefined })
  }, [loadRuns, trajectoryNavigation.intent])

  useEffect(() => {
    if (agentFilter !== 'all' && !agentOptions.includes(agentFilter)) setAgentFilter('all')
  }, [agentFilter, agentOptions])

  useEffect(() => {
    if (!visibleRuns.length) {
      setSelectedRunURI('')
      return
    }
    if (!visibleRuns.some((run) => run.trajectory_uri === selectedRunURI)) {
      setSelectedRunURI(visibleRuns[0].trajectory_uri)
    }
  }, [selectedRunURI, visibleRuns])

  useEffect(() => {
    if (!selectedRun) {
      setTrace(null)
      return
    }
    let cancelled = false
    setTrace((current) => current?.summary.id === selectedRun.id ? current : null)
    setLoadingTrace(true)
    setError(null)
    void getAgentRunTrace(selectedRun.project_id, selectedRun.id)
      .then((nextTrace) => {
        if (!cancelled) setTrace(nextTrace)
      })
      .catch((cause) => {
        if (cancelled) return
        console.error('[TrajectoryPage.tsx] failed to load global Agent Run detail', {
          projectID: selectedRun.project_id,
          runID: selectedRun.id,
          cause,
        })
        setError(errorMessage(cause))
        setTrace(null)
      })
      .finally(() => {
        if (!cancelled) setLoadingTrace(false)
      })
    return () => { cancelled = true }
  }, [selectedRun])

  const exportTrace = async () => {
    if (!selectedRun) return
    try {
      const file = await exportAgentRunTrace(selectedRun.project_id, selectedRun.id)
      downloadAgentRunTrace(file)
      toast.success(t('trajectory.export.success', { filename: file.filename }))
    } catch (cause) {
      console.error('[TrajectoryPage.tsx] failed to export Agent Run trace', {
        projectID: selectedRun.project_id,
        runID: selectedRun.id,
        cause,
      })
      toast.error(t('trajectory.export.failed'), { description: errorMessage(cause) })
    }
  }

  return (
    <AdaptiveSurface
      className="h-full min-h-0"
      mainClassName="min-h-0 min-w-0"
      collapseAt={850}
      mobilePaneScope="surface"
      left={{
        id: 'trajectory-runs',
        title: t('trajectory.runs.title'),
        side: 'left',
        icon: <ListTree className="size-4" />,
        desktopVisible: runsOpen,
        desktopClassName: 'min-h-0 border-r border-[var(--nova-border)] bg-[var(--nova-surface-2)]',
        mobileClassName: 'w-[min(88vw,360px)] bg-[var(--nova-surface-2)]',
        content: <TrajectoryRunList runs={visibleRuns} selectedRunURI={selectedRunURI} onSelect={setSelectedRunURI} />,
      }}
      leftResize={{
        layoutKey: 'nova-trajectory-runs-layout',
        label: t('layout.resize.left'),
        defaultSize: '250px',
        minSize: '210px',
        maxSize: '40%',
        mainMinSize: '300px',
      }}
    >
      {({ isMobile, openPaneId, togglePane }) => {
        const runsVisible = isMobile ? openPaneId === 'trajectory-runs' : runsOpen
        return (
          <FeaturePageShell
            icon={Route}
            title={t('trajectory.title')}
            subtitle={t('trajectory.globalSubtitle')}
            className="[&_button]:focus-visible:border-transparent [&_button]:focus-visible:bg-[var(--nova-hover)] [&_button]:focus-visible:outline-none [&_button]:focus-visible:ring-0"
            onClose={onClose}
            error={error}
            actions={(
              <>
                <Badge variant="outline" className="hidden h-5 px-1.5 text-[9px] uppercase tracking-[0.12em] xl:inline-flex">{t('trajectory.developerBadge')}</Badge>
                <Select value={agentFilter} onValueChange={setAgentFilter}>
                  <SelectTrigger size="sm" className="hidden h-7 w-40 text-[10px] sm:flex" aria-label={t('trajectory.agentFilter')}><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t('trajectory.allAgents')}</SelectItem>
                    {agentOptions.map((id) => <SelectItem key={id} value={id}>{id}</SelectItem>)}
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  size="xs"
                  variant={runsVisible ? 'secondary' : 'outline'}
                  className="px-1.5 xl:px-2.5"
                  aria-label={t('trajectory.runs.title')}
                  aria-pressed={runsVisible}
                  onClick={() => {
                    if (isMobile) togglePane('trajectory-runs')
                    else setRunsOpen((value) => !value)
                  }}
                >
                  <ListTree /><span className="hidden xl:inline">{t('trajectory.runs.title')}</span>
                </Button>
                <Button type="button" size="icon-xs" variant="ghost" disabled={loadingRuns || loadingTrace} onClick={() => void loadRuns({ runURI: selectedRunURI })} aria-label={t('trajectory.refresh')}>
                  <RefreshCw className={cn((loadingRuns || loadingTrace) && 'animate-spin')} />
                </Button>
                <Button type="button" size="xs" variant="ghost" className="px-1.5 xl:px-2.5" disabled={!selectedRun} onClick={() => void exportTrace()} aria-label={t('trajectory.export.action')}>
                  <Download /><span className="hidden xl:inline">{t('trajectory.export.action')}</span>
                </Button>
              </>
            )}
          >
            <TrajectoryRunWorkspace runs={visibleRuns} issues={issues} trace={trace} loadingRuns={loadingRuns} loadingTrace={loadingTrace}
              trajectoryURI={selectedRunURI}
              onOpenRun={(runId) => {
                if (!selectedRun) return
                setAgentFilter('all')
                void loadRuns({ target: { projectId: selectedRun.project_id, runId } })
              }}
            />
          </FeaturePageShell>
        )
      }}
    </AdaptiveSurface>
  )
}

function TrajectoryRunWorkspace({ runs, issues, trace, loadingRuns, loadingTrace, trajectoryURI, onOpenRun }: {
  runs: GlobalAgentRunTraceSummary[]
  issues: GlobalAgentRunTraceIssue[]
  trace: AgentRunTrace | null
  loadingRuns: boolean
  loadingTrace: boolean
  trajectoryURI: string
  onOpenRun: (runID: string) => void
}) {
  const { t } = useTranslation()
  if (loadingRuns && runs.length === 0) {
    return <EmptyPage icon={Activity} title={t('common.loading')} description={t('trajectory.loadingGlobalDescription')} />
  }
  if (runs.length === 0) {
    return <EmptyPage icon={Route} title={t('trajectory.empty')} description={t('trajectory.emptyGlobalDescription')} />
  }
  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)]">
      {issues.length > 0 ? (
        <div className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-warning-bg)] px-3 py-1.5 text-[10px] text-[var(--nova-warning)]">
          <AlertTriangle className="size-3.5" />{t('trajectory.partialProjects', { count: issues.length })}
        </div>
      ) : null}
      <section className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface)]">
        {trace ? <>
          <TrajectoryRunHeader key={`header:${trace.summary.id}`} trace={trace} trajectoryURI={trajectoryURI} onOpenRun={onOpenRun} />
          <div className="min-h-0 flex-1"><TrajectoryTraceWorkspace key={trace.summary.id} trace={trace} /></div>
        </> : (
          <EmptyPage icon={Activity} title={loadingTrace ? t('common.loading') : t('trajectory.selectRun')} description={t('trajectory.selectRunDescription')} />
        )}
      </section>
    </div>
  )
}

function EmptyPage({ icon: Icon, title, description }: { icon: typeof Route; title: string; description: string }) {
  return (
    <div className="grid min-h-0 flex-1 place-items-center px-6 text-center">
      <div>
        <Icon className="mx-auto size-6 text-[var(--nova-text-faint)]" />
        <div className="mt-2 text-xs font-medium text-[var(--nova-text)]">{title}</div>
        <p className="mt-1 max-w-md text-[11px] leading-5 text-[var(--nova-text-faint)]">{description}</p>
      </div>
    </div>
  )
}

function errorMessage(cause: unknown) {
  return cause instanceof Error ? cause.message : String(cause || 'Unknown error')
}
