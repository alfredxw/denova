import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Activity, AlertTriangle, Download, ListTree, RefreshCw, Route, Settings2, Stethoscope } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { downloadAgentRunTrace, exportAgentRunTrace, getAgentRunTrace, getGlobalAgentRunTraces } from '@/lib/api'
import type { AgentRunTrace, GlobalAgentRunTraceIssue, GlobalAgentRunTraceSummary } from '@/lib/api'
import { cn } from '@/lib/utils'
import { HarnessAgentPanel } from './HarnessAgentPanel'
import { HarnessRunPicker } from './HarnessRunPicker'
import { HarnessWorkspace } from './HarnessWorkspace'
import { TrajectoryRunList } from './TrajectoryRunList'
import { TrajectoryTraceWorkspace } from './TrajectoryTraceWorkspace'

interface TrajectoryPageProps {
  onClose?: () => void
}

type DeveloperWorkspaceTab = 'trajectory' | 'harness'

/** Global developer workspace for inspecting Runs and evolving Harness State. */
export function TrajectoryPage({ onClose }: TrajectoryPageProps) {
  const { t } = useTranslation()
  const [workspaceTab, setWorkspaceTab] = useState<DeveloperWorkspaceTab>('trajectory')
  const [runsOpen, setRunsOpen] = useState(true)
  const [agentOpen, setAgentOpen] = useState(true)
  const [stateRefreshToken, setStateRefreshToken] = useState(0)
  const [runs, setRuns] = useState<GlobalAgentRunTraceSummary[]>([])
  const [issues, setIssues] = useState<GlobalAgentRunTraceIssue[]>([])
  const [selectedRunURI, setSelectedRunURI] = useState('')
  const [selectedEvidence, setSelectedEvidence] = useState<Set<string>>(new Set())
  const selectedEvidenceRef = useRef<ReadonlySet<string>>(selectedEvidence)
  const [trace, setTrace] = useState<AgentRunTrace | null>(null)
  const [loadingRuns, setLoadingRuns] = useState(false)
  const [loadingTrace, setLoadingTrace] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const selectedRun = useMemo(
    () => runs.find((run) => run.trajectory_uri === selectedRunURI) ?? null,
    [runs, selectedRunURI],
  )
  const harnessEvidence = useMemo(() => {
    if (workspaceTab === 'trajectory') return selectedRunURI ? [selectedRunURI] : undefined
    const selected = runs
      .filter((run) => selectedEvidence.has(run.trajectory_uri))
      .map((run) => run.trajectory_uri)
    return selected.length > 0 ? selected : undefined
  }, [runs, selectedEvidence, selectedRunURI, workspaceTab])

  useEffect(() => {
    selectedEvidenceRef.current = selectedEvidence
  }, [selectedEvidence])

  const loadRuns = useCallback(async (preferredRunURI?: string) => {
    setLoadingRuns(true)
    setError(null)
    try {
      const catalog = await getGlobalAgentRunTraces(100)
      setRuns(catalog.runs)
      setIssues(catalog.issues ?? [])
      const validURIs = new Set(catalog.runs.map((run) => run.trajectory_uri))
      const retainedEvidence = new Set([...selectedEvidenceRef.current].filter((uri) => validURIs.has(uri)))
      const removedCount = selectedEvidenceRef.current.size - retainedEvidence.size
      selectedEvidenceRef.current = retainedEvidence
      setSelectedEvidence(retainedEvidence)
      if (removedCount > 0) {
        toast.warning(t('continualLearning.evidence.removed', { count: removedCount }))
      }
      const nextRunURI = preferredRunURI && validURIs.has(preferredRunURI)
        ? preferredRunURI
        : catalog.runs[0]?.trajectory_uri ?? ''
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
    void loadRuns()
  }, [loadRuns])

  useEffect(() => {
    if (!selectedRun) {
      setTrace(null)
      return
    }
    let cancelled = false
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

  const toggleEvidence = (trajectoryURI: string) => {
    setSelectedEvidence((current) => {
      const next = new Set(current)
      if (next.has(trajectoryURI)) next.delete(trajectoryURI)
      else next.add(trajectoryURI)
      selectedEvidenceRef.current = next
      return next
    })
  }

  const clearEvidence = () => {
    selectedEvidenceRef.current = new Set()
    setSelectedEvidence(new Set())
  }

  const viewRun = (trajectoryURI: string) => {
    setSelectedRunURI(trajectoryURI)
    setWorkspaceTab('trajectory')
  }

  const handleAgentSettled = useCallback(() => {
    setStateRefreshToken((value) => value + 1)
  }, [])

  return (
    <Tabs value={workspaceTab} onValueChange={(value) => setWorkspaceTab(value as DeveloperWorkspaceTab)} className="h-full min-h-0 gap-0">
      <AdaptiveSurface
        className="h-full min-h-0"
        mainClassName="min-h-0 min-w-0"
        collapseAt={1000}
        mobilePaneScope="surface"
        left={workspaceTab === 'trajectory' ? {
          id: 'trajectory-runs',
          title: t('trajectory.runs.title'),
          side: 'left',
          icon: <ListTree className="size-4" />,
          enabled: true,
          desktopVisible: runsOpen,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)] bg-[var(--nova-surface-2)]',
          mobileClassName: 'w-[min(88vw,360px)] bg-[var(--nova-surface-2)]',
          content: (
            <TrajectoryRunList
              runs={runs}
              selectedRunURI={selectedRunURI}
              onSelect={setSelectedRunURI}
            />
          ),
        } : undefined}
        leftResize={workspaceTab === 'trajectory' ? {
          layoutKey: 'nova-trajectory-runs-layout',
          label: t('layout.resize.left'),
          defaultSize: '250px',
          minSize: '210px',
          maxSize: '40%',
          mainMinSize: '300px',
        } : undefined}
        right={{
          id: 'trajectory-harness-agent',
          title: t('continualLearning.agent.title'),
          side: 'right',
          icon: <Stethoscope className="size-4" />,
          enabled: true,
          desktopVisible: agentOpen,
          desktopClassName: 'min-h-0 border-l border-[var(--nova-border)] bg-[var(--nova-surface)]',
          mobileClassName: 'w-[min(94vw,480px)] bg-[var(--nova-surface)]',
          content: (
            <HarnessAgentPanel
              evidence={harnessEvidence}
              evidenceControl={workspaceTab === 'harness' ? (
                <HarnessRunPicker
                  runs={runs}
                  selected={selectedEvidence}
                  loading={loadingRuns}
                  onToggle={toggleEvidence}
                  onClear={clearEvidence}
                  onView={viewRun}
                />
              ) : undefined}
              onSettled={handleAgentSettled}
            />
          ),
        }}
        rightResize={{
          layoutKey: 'nova-trajectory-harness-agent-layout',
          label: t('layout.resize.right'),
          defaultSize: '440px',
          minSize: '320px',
          maxSize: '65%',
          mainMinSize: '300px',
        }}
      >
        {({ isMobile, openPaneId, togglePane }) => {
          const runsVisible = isMobile ? openPaneId === 'trajectory-runs' : runsOpen
          const agentVisible = isMobile ? openPaneId === 'trajectory-harness-agent' : agentOpen
          return (
            <FeaturePageShell
              icon={Route}
              title={t('trajectory.title')}
              subtitle={t('trajectory.globalSubtitle')}
              className="[&_button]:focus-visible:border-transparent [&_button]:focus-visible:bg-[var(--nova-hover)] [&_button]:focus-visible:outline-none [&_button]:focus-visible:ring-0"
              onClose={onClose}
              error={error}
              leadingContent={(
                <TabsList variant="line" className="h-7 gap-1">
                  <TabsTrigger value="trajectory" className="px-2 text-[11px]"><Route />{t('trajectory.tab.trajectory')}</TabsTrigger>
                  <TabsTrigger value="harness" className="px-2 text-[11px]"><Settings2 />{t('trajectory.tab.harness')}</TabsTrigger>
                </TabsList>
              )}
              actions={(
                <>
                  <Badge variant="outline" className="hidden h-5 px-1.5 text-[9px] uppercase tracking-[0.12em] xl:inline-flex">{t('trajectory.developerBadge')}</Badge>
                  {workspaceTab === 'trajectory' ? (
                    <Button
                      type="button"
                      size="xs"
                      variant={runsVisible ? 'secondary' : 'outline'}
                      className="px-1.5 xl:px-2.5"
                      aria-label={t('trajectory.runs.title')}
                      aria-pressed={runsVisible}
                      onClick={() => {
                        if (isMobile) {
                          togglePane('trajectory-runs')
                          return
                        }
                        setRunsOpen((value) => !value)
                      }}
                    >
                      <ListTree /><span className="hidden xl:inline">{t('trajectory.runs.title')}</span>
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    size="icon-xs"
                    variant="ghost"
                    disabled={loadingRuns || loadingTrace}
                    onClick={() => void loadRuns(selectedRunURI)}
                    aria-label={t('trajectory.refresh')}
                  >
                    <RefreshCw className={cn((loadingRuns || loadingTrace) && 'animate-spin')} />
                  </Button>
                  {workspaceTab === 'trajectory' ? (
                    <Button
                      type="button"
                      size="xs"
                      variant="ghost"
                      className="px-1.5 xl:px-2.5"
                      disabled={!selectedRun}
                      onClick={() => void exportTrace()}
                      aria-label={t('trajectory.export.action')}
                    >
                      <Download /><span className="hidden xl:inline">{t('trajectory.export.action')}</span>
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    size="xs"
                    variant={agentVisible ? 'secondary' : 'outline'}
                    className="px-1.5 xl:px-2.5"
                    aria-label={t('continualLearning.openAgent')}
                    aria-pressed={agentVisible}
                    onClick={() => {
                      if (isMobile) {
                        togglePane('trajectory-harness-agent')
                        return
                      }
                      setAgentOpen((value) => !value)
                    }}
                  >
                    <Stethoscope /><span className="hidden xl:inline">{t('continualLearning.openAgent')}</span>
                  </Button>
                </>
              )}
            >
              <TabsContent value="trajectory" forceMount className="min-h-0 overflow-hidden data-[state=inactive]:hidden">
                <TrajectoryRunWorkspace
                  runs={runs}
                  issues={issues}
                  trace={trace}
                  loadingRuns={loadingRuns}
                  loadingTrace={loadingTrace}
                />
              </TabsContent>
              <TabsContent value="harness" forceMount className="min-h-0 overflow-hidden data-[state=inactive]:hidden">
                <HarnessWorkspace refreshToken={stateRefreshToken} />
              </TabsContent>
            </FeaturePageShell>
          )
        }}
      </AdaptiveSurface>
    </Tabs>
  )
}

function TrajectoryRunWorkspace({
  runs,
  issues,
  trace,
  loadingRuns,
  loadingTrace,
}: {
  runs: GlobalAgentRunTraceSummary[]
  issues: GlobalAgentRunTraceIssue[]
  trace: AgentRunTrace | null
  loadingRuns: boolean
  loadingTrace: boolean
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
        {trace ? (
          <TrajectoryTraceWorkspace trace={trace} />
        ) : (
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
