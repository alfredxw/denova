import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { FileClock, MoreHorizontal, RefreshCw, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { createVersion, getVersionDiff, getVersionRestorePlan, getVersions, getVersionStatus, restoreVersion } from '@/lib/api'
import type { VersionRestorePlan } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { VersionTimeline, type VersionItem } from '@/features/versions/components/version-timeline'
import { AutoSummary } from './AutoSummary'
import { ChangesList } from './ChangesList'
import { SectionHeader } from './SectionHeader'
import { VersionHeader } from './VersionHeader'
import { versionToTimelineItem } from './version-panel-utils'
import { RollbackDialog } from '@/features/versions/components/rollback-dialog'
import { VersionDiffDialog } from '@/features/versions/components/version-diff-dialog'

interface VersionPanelProps {
  workspace: string
  refreshSignal: number
  visible: boolean
  onClose: () => void
}

const versionKeys = {
  all: ['versions'] as const,
  status: (workspace: string) => ['versions', 'status', workspace] as const,
  history: (workspace: string) => ['versions', 'history', workspace] as const,
}

/** VersionPanel 展示 Nova 原生快照版本状态、历史和恢复操作。 */
export function VersionPanel({ workspace, refreshSignal, visible, onClose }: VersionPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [error, setError] = useState('')
  const [changesExpanded, setChangesExpanded] = useState(true)
  const [historyExpanded, setHistoryExpanded] = useState(true)
  const [rollbackVersion, setRollbackVersion] = useState<VersionItem | null>(null)
  const [restorePaths, setRestorePaths] = useState<string[] | undefined>()
  const [restorePlan, setRestorePlan] = useState<VersionRestorePlan | null>(null)
  const [restorePlanLoading, setRestorePlanLoading] = useState(false)
  const [diffVersion, setDiffVersion] = useState<VersionItem | null>(null)
  const [diffPath, setDiffPath] = useState('')
  const [diffText, setDiffText] = useState<{ original: string; modified: string } | null>(null)
  const restorePlanRequestRef = useRef(0)

  const statusQuery = useQuery({
    queryKey: versionKeys.status(workspace),
    queryFn: getVersionStatus,
    enabled: Boolean(workspace && visible),
  })
  const status = statusQuery.data ?? null

  const historyQuery = useQuery({
    queryKey: versionKeys.history(workspace),
    queryFn: () => getVersions(30),
    enabled: Boolean(workspace && visible),
  })
  const versions = historyQuery.data ?? []

  const invalidateVersionQueries = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: versionKeys.all })
  }, [queryClient])

  const refresh = useCallback(async () => {
    if (!workspace || !visible) return
    await invalidateVersionQueries()
  }, [invalidateVersionQueries, visible, workspace])

  useEffect(() => {
    setError('')
  }, [workspace])

  useEffect(() => {
    void refresh()
  }, [refresh, refreshSignal])

  useEffect(() => {
    if (!visible) return
    const handleFocus = () => void refresh()
    window.addEventListener('focus', handleFocus)
    return () => window.removeEventListener('focus', handleFocus)
  }, [refresh, visible])

  const createMutation = useMutation({
    mutationFn: () => createVersion(),
    onSuccess: async (result) => {
      setError('')
      toast.success(t('versions.saved', { message: result.version?.message || result.message }))
      await invalidateVersionQueries()
    },
    onError: (e) => showOperationError(e, t('versions.createFailed'), setError),
  })

  const restoreMutation = useMutation({
    mutationFn: ({ id, paths }: { id: string; paths?: string[] }) => restoreVersion(id, paths),
    onSuccess: async () => {
      setRollbackVersion(null)
      setRestorePaths(undefined)
      setRestorePlan(null)
      setError('')
      toast.success(t('versions.restoreSuccess'))
      await invalidateVersionQueries()
    },
    onError: (e) => showOperationError(e, t('versions.restoreFailed'), setError),
  })

  const loading = statusQuery.isFetching || historyQuery.isFetching || createMutation.isPending || restoreMutation.isPending || restorePlanLoading
  const changes = status?.changes ?? []
  const canCreate = !loading && Boolean(workspace)
  const timelineItems = useMemo(() => versions.map((version) => versionToTimelineItem(version, t)), [t, versions])
  const currentVersionItem = useMemo(() => status?.latest ? versionToTimelineItem(status.latest, t) : null, [status?.latest, t])

  const createManualVersion = () => {
    if (loading) return
    createMutation.mutate()
  }

  const openDiff = async (version: VersionItem, path?: string) => {
    try {
      setDiffVersion(version)
      let selectedPath = path || ''
      if (!selectedPath) {
        const summary = await getVersionDiff(version.id)
        selectedPath = summary.changes[0]?.path || ''
      }
      setDiffPath(selectedPath)
      if (!selectedPath) {
        setDiffText(null)
        toast.info(t('versions.noComparableFiles'))
        return
      }
      const diff = await getVersionDiff(version.id, selectedPath)
      if (diff.text) {
        setDiffText({ original: diff.original || '', modified: diff.modified || '' })
      } else {
        setDiffText(null)
        toast.info(t('versions.fileBinary'))
      }
    } catch (e) {
      showOperationError(e, t('versions.diffReadFailed'), setError)
    }
  }

  const openRestoreDialog = async (version: VersionItem, paths?: string[]) => {
    const requestID = restorePlanRequestRef.current + 1
    restorePlanRequestRef.current = requestID
    setRollbackVersion(version)
    setRestorePaths(paths)
    setRestorePlan(null)
    setRestorePlanLoading(true)
    try {
      const plan = await getVersionRestorePlan(version.id, paths)
      if (restorePlanRequestRef.current !== requestID) return
      setRestorePlan(plan)
      setError('')
    } catch (e) {
      if (restorePlanRequestRef.current !== requestID) return
      setRollbackVersion(null)
      setRestorePaths(undefined)
      showOperationError(e, t('versions.restorePlanFailed'), setError)
    } finally {
      if (restorePlanRequestRef.current === requestID) {
        setRestorePlanLoading(false)
      }
    }
  }

  return (
    <div className="nova-sidebar flex h-full min-h-0 flex-col text-xs text-[var(--nova-text-muted)]">
      <div className="nova-topbar flex h-9 shrink-0 items-center border-b px-3">
        <span className="font-semibold text-[var(--nova-text)]">{t('versions.title')}</span>
        <TooltipIconButton label={t('versions.refresh')} className="ml-auto text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={refresh} disabled={loading}>
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </TooltipIconButton>
        <TooltipIconButton label={t('versions.close')} className="text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={onClose}>
          <MoreHorizontal className="h-3.5 w-3.5" />
        </TooltipIconButton>
      </div>

      <ScrollArea className="min-h-0 flex-1 overflow-x-hidden">
        <div className="w-full max-w-full min-w-0 overflow-hidden px-3 py-2">
          <VersionHeader workspace={workspace} status={status} changesCount={changes.length} />
          <AutoSummary status={status} />

          <div className="mt-3">
            <div className="mb-1 flex items-center gap-2 text-[11px] font-semibold text-[var(--nova-text-muted)]">
              <FileClock className="h-3.5 w-3.5" />
              <span>{t('versions.manualSave')}</span>
            </div>
            <Button type="button" size="sm" className="mt-2 flex w-full items-center justify-center gap-2 border border-[var(--nova-border)] bg-[var(--nova-active)] font-medium text-[var(--nova-text)] hover:bg-[var(--nova-hover)] disabled:opacity-45" onClick={createManualVersion} disabled={!canCreate}>
              <ShieldCheck className={`h-3.5 w-3.5 ${createMutation.isPending ? 'animate-pulse' : ''}`} />
              <span>{createMutation.isPending ? t('versions.savingWithSummary') : t('versions.saveCurrent')}</span>
            </Button>
          </div>

          <SectionHeader title={t('versions.currentChanges')} count={changes.length} expanded={changesExpanded} onToggle={() => setChangesExpanded(value => !value)} />
          {changesExpanded && <ChangesList changes={changes} onOpenDiff={(path) => currentVersionItem && openDiff(currentVersionItem, path)} />}

          <SectionHeader title={t('versions.history')} count={timelineItems.length} expanded={historyExpanded} onToggle={() => setHistoryExpanded(value => !value)} />
          {historyExpanded && (
            <VersionTimeline
              versions={timelineItems}
              selectedVersionId={status?.latest?.id}
              loading={loading}
              canRollback={timelineItems.length > 0}
              onOpenDiff={(version) => void openDiff(version)}
              onOpenDiffPath={(version, path) => void openDiff(version, path)}
              onRollback={(version) => void openRestoreDialog(version)}
              onRestorePath={(version, path) => void openRestoreDialog(version, [path])}
            />
          )}

          {error && (
            <InlineErrorNotice className="mt-3" message={error} />
          )}
        </div>
      </ScrollArea>

      <RollbackDialog
        open={Boolean(rollbackVersion)}
        version={rollbackVersion}
        plan={restorePlan}
        loading={restoreMutation.isPending}
        planLoading={restorePlanLoading}
        onOpenChange={(open) => {
          if (!open) {
            restorePlanRequestRef.current += 1
            setRollbackVersion(null)
            setRestorePaths(undefined)
            setRestorePlan(null)
            setRestorePlanLoading(false)
          }
        }}
        onRollback={(version) => restoreMutation.mutate({ id: version.id, paths: restorePaths })}
      />

      <VersionDiffDialog
        open={Boolean(diffVersion && diffPath && diffText)}
        title={diffPath ? t('versions.diffTitleWithPath', { path: diffPath }) : t('versions.diffTitle')}
        original={diffText?.original || ''}
        modified={diffText?.modified || ''}
        language="markdown"
        onOpenChange={(open) => { if (!open) { setDiffVersion(null); setDiffText(null); setDiffPath('') } }}
      />
    </div>
  )
}

function showOperationError(e: unknown, fallback: string, setError: (message: string) => void) {
  const message = e instanceof Error ? e.message : fallback
  setError(message)
  toast.error(message)
}
