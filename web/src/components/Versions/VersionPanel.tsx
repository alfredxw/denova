import { useCallback, useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Clock3, FileClock, FileText, MoreHorizontal, RefreshCw, ShieldCheck, ShieldAlert } from 'lucide-react'
import { toast } from 'sonner'
import { createVersion, getVersionDiff, getVersions, getVersionStatus, restoreVersion } from '@/lib/api'
import type { VersionChange, VersionEntry, VersionStatus } from '@/lib/api'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { VersionTimeline, type VersionItem } from '@/features/versions/components/version-timeline'
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
  const queryClient = useQueryClient()
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [changesExpanded, setChangesExpanded] = useState(true)
  const [historyExpanded, setHistoryExpanded] = useState(true)
  const [rollbackVersion, setRollbackVersion] = useState<VersionItem | null>(null)
  const [diffVersion, setDiffVersion] = useState<VersionItem | null>(null)
  const [diffPath, setDiffPath] = useState('')
  const [diffText, setDiffText] = useState<{ original: string; modified: string } | null>(null)

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
    setMessage('')
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
    mutationFn: createVersion,
    onSuccess: async (_, versionMessage) => {
      setMessage('')
      setError('')
      toast.success(`已保存版本：${versionMessage}`)
      await invalidateVersionQueries()
    },
    onError: (e) => showOperationError(e, '创建版本失败', setError),
  })

  const restoreMutation = useMutation({
    mutationFn: restoreVersion,
    onSuccess: async () => {
      setRollbackVersion(null)
      setError('')
      toast.success('已恢复版本')
      await invalidateVersionQueries()
    },
    onError: (e) => showOperationError(e, '恢复版本失败', setError),
  })

  const loading = statusQuery.isFetching || historyQuery.isFetching || createMutation.isPending || restoreMutation.isPending
  const changes = status?.changes ?? []
  const canCreate = message.trim().length > 0 && !loading && Boolean(workspace)
  const timelineItems = useMemo(() => versions.map(versionToTimelineItem), [versions])
  const currentVersionItem = useMemo(() => status?.latest ? versionToTimelineItem(status.latest) : null, [status?.latest])

  const createManualVersion = () => {
    const trimmed = message.trim()
    if (!trimmed || loading) return
    createMutation.mutate(trimmed)
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
        toast.info('这个版本与当前工作区没有可对比的文件')
        return
      }
      const diff = await getVersionDiff(version.id, selectedPath)
      if (diff.text) {
        setDiffText({ original: diff.original || '', modified: diff.modified || '' })
      } else {
        setDiffText(null)
        toast.info('该文件不是文本文件，暂不支持内容对比')
      }
    } catch (e) {
      showOperationError(e, '读取版本差异失败', setError)
    }
  }

  return (
    <div className="nova-sidebar flex h-full min-h-0 flex-col text-xs text-[var(--nova-text-muted)]">
      <div className="nova-topbar flex h-9 shrink-0 items-center border-b px-3">
        <span className="font-semibold text-[var(--nova-text)]">版本管理</span>
        <TooltipIconButton label="刷新版本状态" className="ml-auto text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={refresh} disabled={loading}>
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </TooltipIconButton>
        <TooltipIconButton label="关闭版本管理" className="text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={onClose}>
          <MoreHorizontal className="h-3.5 w-3.5" />
        </TooltipIconButton>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="px-3 py-2">
          <VersionHeader workspace={workspace} status={status} changesCount={changes.length} />
          <AutoSummary status={status} />

          <div className="mt-3">
            <div className="mb-1 flex items-center gap-2 text-[11px] font-semibold text-[var(--nova-text-muted)]">
              <FileClock className="h-3.5 w-3.5" />
              <span>手动保存版本</span>
            </div>
            <Textarea
              value={message}
              onChange={(event) => setMessage(event.target.value)}
              placeholder="输入这次版本说明"
              rows={2}
              className="nova-field min-h-0 resize-none px-2 py-1.5 text-xs leading-5 placeholder:text-[var(--nova-text-faint)] focus-visible:ring-0"
              disabled={loading || !workspace}
            />
            <Button type="button" size="sm" className="mt-2 flex w-full items-center justify-center gap-2 border border-[var(--nova-border)] bg-[var(--nova-active)] font-medium text-[var(--nova-text)] hover:bg-[var(--nova-hover)] disabled:opacity-45" onClick={createManualVersion} disabled={!canCreate}>
              <ShieldCheck className="h-3.5 w-3.5" />
              <span>保存当前版本</span>
            </Button>
          </div>

          <SectionHeader title="当前变更" count={changes.length} expanded={changesExpanded} onToggle={() => setChangesExpanded(value => !value)} />
          {changesExpanded && <ChangesList changes={changes} onOpenDiff={(path) => currentVersionItem && openDiff(currentVersionItem, path)} />}

          <SectionHeader title="版本历史" count={timelineItems.length} expanded={historyExpanded} onToggle={() => setHistoryExpanded(value => !value)} />
          {historyExpanded && (
            <VersionTimeline
              versions={timelineItems}
              selectedVersionId={status?.latest?.id}
              loading={loading}
              canRollback={timelineItems.length > 0}
              onOpenDiff={(version) => void openDiff(version)}
              onRollback={setRollbackVersion}
            />
          )}

          {error && (
            <div className="mt-3 rounded border border-red-500/30 bg-red-500/10 px-2 py-1.5 leading-5 text-red-200">
              {error}
            </div>
          )}
        </div>
      </ScrollArea>

      <RollbackDialog
        open={Boolean(rollbackVersion)}
        version={rollbackVersion}
        loading={restoreMutation.isPending}
        onOpenChange={(open) => { if (!open) setRollbackVersion(null) }}
        onRollback={(version) => restoreMutation.mutate(version.id)}
      />

      <VersionDiffDialog
        open={Boolean(diffVersion && diffPath && diffText)}
        title={diffPath ? `版本差异：${diffPath}` : '版本差异'}
        original={diffText?.original || ''}
        modified={diffText?.modified || ''}
        language="markdown"
        onOpenChange={(open) => { if (!open) { setDiffVersion(null); setDiffText(null); setDiffPath('') } }}
      />
    </div>
  )
}

function VersionHeader({ workspace, status, changesCount }: { workspace: string; status: VersionStatus | null; changesCount: number }) {
  const hasVersions = status?.has_versions ?? false
  const clean = status?.clean ?? true
  const Icon = !hasVersions || !clean ? ShieldAlert : ShieldCheck
  const label = !hasVersions ? '尚无版本' : clean ? '已保护' : `${changesCount} 个未保存版本变更`

  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
      <div className="flex items-center gap-2">
        <Icon className={`h-3.5 w-3.5 ${!hasVersions || !clean ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-accent-green)]'}`} />
        <span className="min-w-0 flex-1 truncate font-medium text-[var(--nova-text)]">{workspaceName(workspace) || '未选择书籍'}</span>
        <span className="rounded-full bg-[var(--nova-active)] px-2 py-0.5 text-[11px] text-[var(--nova-text)]">{label}</span>
      </div>
      <div className="mt-2 flex items-center gap-2 text-[11px] text-[var(--nova-text-faint)]">
        {status?.latest ? <span>当前版本：{formatTime(status.latest.created_at)}</span> : <span>保存第一个版本后即可查看历史和恢复。</span>}
      </div>
    </div>
  )
}

function AutoSummary({ status }: { status: VersionStatus | null }) {
  const auto = status?.auto
  return (
    <div className="mt-2 grid grid-cols-2 gap-2">
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5">
        <div className="flex items-center gap-1 text-[var(--nova-text)]"><Clock3 className="h-3 w-3" />定时保存</div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{auto?.timed_enabled ? `有变更每 ${auto.timed_interval_minutes} 分钟` : '已关闭'}</div>
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5">
        <div className="flex items-center gap-1 text-[var(--nova-text)]"><FileText className="h-3 w-3" />Agent 保存</div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{auto?.agent_enabled ? `约 ${auto.agent_char_threshold} 字触发` : '已关闭'}</div>
      </div>
    </div>
  )
}

function SectionHeader({ title, count, expanded, onToggle }: { title: string; count: number; expanded: boolean; onToggle: () => void }) {
  return (
    <button type="button" className="nova-nav-item mt-3 flex w-full items-center gap-1 rounded-[var(--nova-radius)] py-1 text-left font-semibold text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={onToggle}>
      {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
      <span>{title}</span>
      <span className="ml-auto rounded-full bg-[var(--nova-active)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{count}</span>
    </button>
  )
}

function ChangesList({ changes, onOpenDiff }: { changes: VersionChange[]; onOpenDiff: (path: string) => void }) {
  if (changes.length === 0) {
    return <div className="rounded bg-[var(--nova-surface)] px-2 py-2 text-[var(--nova-text-faint)]">暂无变更</div>
  }
  return (
    <div className="space-y-0.5">
      {changes.map(change => (
        <button key={`${change.status}:${change.path}`} type="button" className="group flex w-full items-center gap-2 rounded px-1.5 py-1 text-left hover:bg-[var(--nova-hover)]" title={change.path} onClick={() => onOpenDiff(change.path)}>
          <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[9px] text-[var(--nova-text-muted)]">{statusLabel(change.status)}</span>
          <span className="min-w-0 flex-1 truncate text-[var(--nova-text-muted)]">{fileName(change.path)}</span>
          <span className="truncate text-[10px] text-[var(--nova-text-faint)]">{dirName(change.path)}</span>
          <span className={`shrink-0 text-[11px] ${statusColor(change.status)}`}>{statusText(change.status)}</span>
        </button>
      ))}
    </div>
  )
}

function versionToTimelineItem(version: VersionEntry): VersionItem {
  return {
    id: version.id,
    title: version.message || '(无说明)',
    description: sourceText(version.source),
    createdAt: formatTime(version.created_at),
    author: `${version.file_count} 文件 · ${formatBytes(version.total_bytes)}`,
  }
}

function sourceText(source: VersionEntry['source']) {
  if (source === 'timer') return '定时'
  if (source === 'agent') return 'Agent'
  if (source === 'rollback_backup') return '回滚前备份'
  return '手动'
}

function showOperationError(e: unknown, fallback: string, setError: (message: string) => void) {
  const message = e instanceof Error ? e.message : fallback
  setError(message)
  toast.error(message)
}

function workspaceName(path: string) {
  return path.split('/').filter(Boolean).pop() || path
}

function fileName(path: string) {
  return path.split('/').pop() || path
}

function dirName(path: string) {
  const parts = path.split('/')
  parts.pop()
  return parts.join('/')
}

function statusLabel(status: string) {
  if (status === 'added') return 'A'
  if (status === 'deleted') return 'D'
  return 'M'
}

function statusText(status: string) {
  if (status === 'added') return '新增'
  if (status === 'deleted') return '删除'
  return '修改'
}

function statusColor(status: string) {
  if (status === 'deleted') return 'text-red-300'
  if (status === 'added') return 'text-[var(--nova-accent-green)]'
  return 'text-[var(--nova-accent)]'
}

function formatTime(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
