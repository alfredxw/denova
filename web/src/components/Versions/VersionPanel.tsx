import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { RefreshCw, X } from 'lucide-react'
import { toast } from 'sonner'
import {
  createVersion,
  getVersionDiff,
  getVersionRestorePlan,
  getVersions,
  getVersionStatus,
  restoreVersion,
} from '@/lib/api'
import type { VersionDiff, VersionDiffComparison, VersionEntry, VersionRestorePlan } from '@/lib/api'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { RollbackDialog } from '@/features/versions/components/rollback-dialog'
import { CURRENT_WORKSPACE_SELECTION, VersionSidebar } from './VersionSidebar'
import { VersionDiffWorkspace, type VersionDiffMode } from './VersionDiffWorkspace'

interface VersionPanelProps {
  projectId: string
  workspace: string
  refreshSignal?: number
  visible?: boolean
  onClose?: () => void
  onWorkspaceChanged?: (paths: string[]) => void | Promise<void>
}

const INITIAL_HISTORY_LIMIT = 30
const MAX_HISTORY_LIMIT = 200

const versionKeys = {
  all: ['versions'] as const,
  status: (projectId: string) => ['versions', 'status', projectId] as const,
  history: (projectId: string, limit: number) => ['versions', 'history', projectId, limit] as const,
  diff: (projectId: string, versionId: string, comparison: VersionDiffComparison, path = '') =>
    ['versions', 'diff', projectId, versionId, comparison, path] as const,
}

/** Project-scoped version history with a persistent, responsive diff workspace. */
export function VersionPanel({ projectId, workspace, refreshSignal = 0, visible = true, onClose, onWorkspaceChanged }: VersionPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [error, setError] = useState('')
  const [historyLimit, setHistoryLimit] = useState(INITIAL_HISTORY_LIMIT)
  const [search, setSearch] = useState('')
  const [selectionKey, setSelectionKey] = useState('')
  const [selectedPath, setSelectedPath] = useState('')
  const [diffMode, setDiffMode] = useState<VersionDiffMode>('split')
  const [rollbackVersion, setRollbackVersion] = useState<VersionEntry | null>(null)
  const [restorePaths, setRestorePaths] = useState<string[] | undefined>()
  const [restorePlan, setRestorePlan] = useState<VersionRestorePlan | null>(null)
  const [restorePlanLoading, setRestorePlanLoading] = useState(false)
  const restorePlanRequestRef = useRef(0)
  const closeHistoryPaneRef = useRef<() => void>(() => {})

  const statusQuery = useQuery({
    queryKey: versionKeys.status(projectId),
    queryFn: () => getVersionStatus(projectId),
    enabled: Boolean(projectId && visible),
  })
  const status = statusQuery.data ?? null

  const historyQuery = useQuery({
    queryKey: versionKeys.history(projectId, historyLimit),
    queryFn: () => getVersions(projectId, historyLimit),
    enabled: Boolean(projectId && visible),
    placeholderData: previous => previous,
  })
  const versions = historyQuery.data ?? []
  const selectedKey = selectionKey || versions[0]?.id || CURRENT_WORKSPACE_SELECTION
  const currentSelection = selectedKey === CURRENT_WORKSPACE_SELECTION
  const selectedVersion = currentSelection ? null : versions.find(version => version.id === selectedKey) ?? null
  const targetVersion = currentSelection ? status?.latest ?? null : selectedVersion
  const comparison: VersionDiffComparison = currentSelection ? 'workspace' : 'parent'

  const summaryQuery = useQuery({
    queryKey: versionKeys.diff(projectId, targetVersion?.id ?? '', comparison),
    queryFn: () => getVersionDiff(projectId, targetVersion!.id, undefined, comparison),
    enabled: Boolean(projectId && visible && targetVersion && !currentSelection),
  })
  const summary = useMemo<VersionDiff | null>(() => {
    if (!targetVersion) return null
    if (!currentSelection) return summaryQuery.data ?? null
    return {
      version: targetVersion,
      comparison: 'workspace',
      changes: status?.changes ?? [],
      text: false,
      binary: false,
    }
  }, [currentSelection, status?.changes, summaryQuery.data, targetVersion])
  const selectedChanges = summary?.changes ?? []

  useEffect(() => {
    setHistoryLimit(INITIAL_HISTORY_LIMIT)
    setSearch('')
    setSelectionKey('')
    setSelectedPath('')
    setError('')
  }, [projectId])

  useEffect(() => {
    setSelectedPath(current => selectedChanges.some(change => change.path === current) ? current : selectedChanges[0]?.path || '')
  }, [selectedKey, selectedChanges])

  const fileDiffQuery = useQuery({
    queryKey: versionKeys.diff(projectId, targetVersion?.id ?? '', comparison, selectedPath),
    queryFn: () => getVersionDiff(projectId, targetVersion!.id, selectedPath, comparison),
    enabled: Boolean(projectId && visible && targetVersion && selectedPath),
  })

  const invalidateVersionQueries = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: versionKeys.all })
  }, [queryClient])

  const refresh = useCallback(async () => {
    if (!projectId || !visible) return
    await invalidateVersionQueries()
  }, [invalidateVersionQueries, projectId, visible])

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
    mutationFn: () => createVersion(projectId),
    onSuccess: async result => {
      setError('')
      setSelectionKey('')
      toast.success(t('versions.saved', { message: result.version?.message || result.message }))
      await invalidateVersionQueries()
    },
    onError: cause => showOperationError(cause, t('versions.createFailed'), setError),
  })

  const restoreMutation = useMutation({
    mutationFn: ({ id, paths }: { id: string; paths?: string[] }) => restoreVersion(projectId, id, paths),
    onSuccess: async result => {
      closeRestoreDialog()
      setError('')
      toast.success(t('versions.restoreSuccess'))
      if (result.restored_paths.length > 0) await onWorkspaceChanged?.(result.restored_paths)
      await invalidateVersionQueries()
    },
    onError: cause => showOperationError(cause, t('versions.restoreFailed'), setError),
  })

  const closeRestoreDialog = () => {
    restorePlanRequestRef.current += 1
    setRollbackVersion(null)
    setRestorePaths(undefined)
    setRestorePlan(null)
    setRestorePlanLoading(false)
  }

  const openRestoreDialog = async (version: VersionEntry, paths?: string[]) => {
    const requestID = restorePlanRequestRef.current + 1
    restorePlanRequestRef.current = requestID
    setRollbackVersion(version)
    setRestorePaths(paths)
    setRestorePlan(null)
    setRestorePlanLoading(true)
    try {
      const plan = await getVersionRestorePlan(projectId, version.id, paths)
      if (restorePlanRequestRef.current !== requestID) return
      setRestorePlan(plan)
      setError('')
    } catch (cause) {
      if (restorePlanRequestRef.current !== requestID) return
      closeRestoreDialog()
      showOperationError(cause, t('versions.restorePlanFailed'), setError)
    } finally {
      if (restorePlanRequestRef.current === requestID) setRestorePlanLoading(false)
    }
  }

  const queryError = statusQuery.error || historyQuery.error || summaryQuery.error || fileDiffQuery.error
  const visibleError = error || (queryError instanceof Error ? queryError.message : '')
  const operationLoading = createMutation.isPending || restoreMutation.isPending || restorePlanLoading
  const refreshing = statusQuery.isFetching || historyQuery.isFetching
  const canLoadMore = versions.length === historyLimit && historyLimit < MAX_HISTORY_LIMIT

  const sidebar = (
    <VersionSidebar
      workspace={workspace}
      status={status}
      versions={versions}
      changes={selectedChanges}
      selectedKey={selectedKey}
      selectedPath={selectedPath}
      search={search}
      error={visibleError}
      historyLoading={historyQuery.isFetching}
      statusLoading={statusQuery.isLoading}
      operationLoading={operationLoading}
      refreshing={refreshing}
      canLoadMore={canLoadMore}
      onSearchChange={setSearch}
      onSelectCurrent={() => {
        setSelectionKey(CURRENT_WORKSPACE_SELECTION)
        closeHistoryPaneRef.current()
      }}
      onSelectVersion={version => {
        setSelectionKey(version.id)
        closeHistoryPaneRef.current()
      }}
      onSelectPath={path => {
        setSelectedPath(path)
        closeHistoryPaneRef.current()
      }}
      onRefresh={() => void refresh()}
      onCreate={() => createMutation.mutate()}
      onLoadMore={() => setHistoryLimit(limit => Math.min(limit + INITIAL_HISTORY_LIMIT, MAX_HISTORY_LIMIT))}
    />
  )

  return (
    <div className="flex h-full min-h-0 flex-col bg-background text-foreground">
      <div className="flex h-9 shrink-0 items-center border-b px-3">
        <span className="text-xs font-semibold">{t('versions.title')}</span>
        <TooltipIconButton label={t('versions.refresh')} className="ml-auto" onClick={() => void refresh()} disabled={operationLoading || refreshing}>
          <RefreshCw className={refreshing ? 'animate-spin' : ''} />
        </TooltipIconButton>
        {onClose && (
          <TooltipIconButton label={t('versions.close')} onClick={onClose}>
            <X />
          </TooltipIconButton>
        )}
      </div>

      <AdaptiveSurface
        className="min-h-0 flex-1"
        collapseAt={960}
        mobilePaneScope="surface"
        left={{
          id: 'version-history',
          title: t('versions.history'),
          side: 'left',
          content: sidebar,
          desktopClassName: 'border-r bg-background',
          mobileClassName: 'bg-background',
        }}
        leftResize={{
          layoutKey: 'denova:versions:history-layout',
          label: t('versions.resizeHistorySidebar'),
          defaultSize: '340px',
          minSize: '280px',
          maxSize: '46%',
          mainMinSize: '420px',
        }}
      >
        {controls => {
          closeHistoryPaneRef.current = controls.closePane
          return (
            <VersionDiffWorkspace
              currentSelection={currentSelection}
              targetVersion={targetVersion}
              summary={summary}
              diff={fileDiffQuery.data ?? null}
              selectedPath={selectedPath}
              mode={diffMode}
              isMobile={controls.isMobile}
              loading={summaryQuery.isLoading || fileDiffQuery.isLoading}
              restoring={operationLoading}
              onModeChange={setDiffMode}
              onOpenSidebar={controls.openLeft}
              onRestoreVersion={() => targetVersion && void openRestoreDialog(targetVersion)}
              onRestoreFile={() => targetVersion && selectedPath && void openRestoreDialog(targetVersion, [selectedPath])}
            />
          )
        }}
      </AdaptiveSurface>

      <RollbackDialog
        open={Boolean(rollbackVersion)}
        version={rollbackVersion}
        plan={restorePlan}
        loading={restoreMutation.isPending}
        planLoading={restorePlanLoading}
        onOpenChange={open => { if (!open) closeRestoreDialog() }}
        onRollback={version => restoreMutation.mutate({ id: version.id, paths: restorePaths })}
      />
    </div>
  )
}

function showOperationError(cause: unknown, fallback: string, setError: (message: string) => void) {
  const message = cause instanceof Error ? cause.message : fallback
  setError(message)
  toast.error(message)
}
