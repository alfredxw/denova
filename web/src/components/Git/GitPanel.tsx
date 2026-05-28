import { useCallback, useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, ChevronDown, ChevronRight, GitBranch, GitCommit, MoreHorizontal, RefreshCw, Undo2 } from 'lucide-react'
import { toast } from 'sonner'
import { createGitVersion, getGitHistory, getGitStatus, initGitRepository, popGitStash, rollbackGitVersion, stashGitChanges } from '@/lib/api'
import type { GitChange, GitCommit as GitCommitInfo, GitStatus } from '@/lib/api'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { VersionTimeline, type VersionItem } from '@/features/versions/components/version-timeline'
import { RollbackDialog } from '@/features/versions/components/rollback-dialog'

interface GitPanelProps {
  workspace: string
  refreshSignal: number
  visible: boolean
  onClose: () => void
}

const gitKeys = {
  all: ['git'] as const,
  status: (workspace: string) => ['git', 'status', workspace] as const,
  history: (workspace: string) => ['git', 'history', workspace] as const,
}

/** GitPanel 以左侧 Source Control 风格展示当前书籍的版本状态。 */
export function GitPanel({ workspace, refreshSignal, visible, onClose }: GitPanelProps) {
  const queryClient = useQueryClient()
  const [message, setMessage] = useState('')
  const [operationOutput, setOperationOutput] = useState('')
  const [operationSummary, setOperationSummary] = useState('版本操作结果会显示在这里')
  const [error, setError] = useState('')
  const [changesExpanded, setChangesExpanded] = useState(true)
  const [historyExpanded, setHistoryExpanded] = useState(true)
  const [outputExpanded, setOutputExpanded] = useState(false)
  const [rollbackVersion, setRollbackVersion] = useState<VersionItem | null>(null)

  const statusQuery = useQuery({
    queryKey: gitKeys.status(workspace),
    queryFn: async () => normalizeStatus(await getGitStatus()),
    enabled: Boolean(workspace && visible),
  })
  const status = statusQuery.data ?? null
  const initialized = status?.initialized ?? false
  const clean = status?.clean ?? true

  const historyQuery = useQuery({
    queryKey: gitKeys.history(workspace),
    queryFn: () => getGitHistory(30),
    enabled: Boolean(workspace && visible && initialized),
  })
  const history = historyQuery.data ?? []

  const invalidateGitQueries = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: gitKeys.all })
  }, [queryClient])

  const refresh = useCallback(async () => {
    if (!workspace || !visible) return
    await invalidateGitQueries()
  }, [invalidateGitQueries, visible, workspace])

  useEffect(() => {
    setError('')
    setOperationOutput('')
    setOperationSummary('版本操作结果会显示在这里')
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

  const initMutation = useMutation({
    mutationFn: initGitRepository,
    onSuccess: async (result) => {
      setOperationOutput(result.output || '(无输出)')
      setOperationSummary('版本仓库已初始化')
      setError('')
      toast.success('版本仓库已初始化')
      await invalidateGitQueries()
    },
    onError: (e) => showOperationError(e, '初始化 Git 仓库失败', setError),
  })

  const commitMutation = useMutation({
    mutationFn: createGitVersion,
    onSuccess: async (result, commitMessage) => {
      setOperationOutput(result.output || '(无输出)')
      setOperationSummary(`已创建版本：${commitMessage}`)
      setMessage('')
      setError('')
      toast.success('已创建版本')
      await invalidateGitQueries()
    },
    onError: (e) => showOperationError(e, '创建版本失败', setError),
  })

  const rollbackMutation = useMutation({
    mutationFn: rollbackGitVersion,
    onSuccess: async (result, hash) => {
      setOperationOutput(result.output || '(无输出)')
      setOperationSummary(`已回滚到版本：${shortHash(hash)}`)
      setRollbackVersion(null)
      setError('')
      toast.success('回滚成功')
      await invalidateGitQueries()
    },
    onError: (e) => showOperationError(e, '回滚版本失败', setError),
  })

  const stashMutation = useMutation({
    mutationFn: stashGitChanges,
    onSuccess: async (result) => {
      setOperationOutput(result.output || '(无输出)')
      setOperationSummary('已暂存当前未提交内容')
      setError('')
      toast.success('已暂存当前内容')
      await invalidateGitQueries()
    },
    onError: (e) => showOperationError(e, '暂存当前内容失败', setError),
  })

  const popMutation = useMutation({
    mutationFn: popGitStash,
    onSuccess: async (result) => {
      setOperationOutput(result.output || '(无输出)')
      setOperationSummary('已恢复最近一次暂存内容')
      setError('')
      toast.success('已恢复暂存内容')
      await invalidateGitQueries()
    },
    onError: (e) => showOperationError(e, '恢复暂存内容失败', setError),
  })

  const loading = statusQuery.isFetching || historyQuery.isFetching || initMutation.isPending || commitMutation.isPending || rollbackMutation.isPending || stashMutation.isPending || popMutation.isPending
  const changes = status?.changes ?? []
  const canCommit = initialized && !clean && message.trim().length > 0 && !loading
  const versions = history.map(commitToVersionItem)

  const createVersion = () => {
    const trimmed = message.trim()
    if (!trimmed || loading) return
    commitMutation.mutate(trimmed)
  }

  const handleRollback = async (version: VersionItem) => {
    if (!status?.clean) {
      setError('当前工作区有未提交变更，请先创建版本后再回滚')
      return
    }
    await rollbackMutation.mutateAsync(version.id)
  }

  return (
    <div className="nova-sidebar flex h-full min-h-0 flex-col text-xs text-[var(--nova-text-muted)]">
      <div className="nova-topbar flex h-9 shrink-0 items-center border-b px-3">
        <span className="font-semibold text-[var(--nova-text)]">版本管理</span>
        <TooltipIconButton
          label="刷新版本状态"
          className="ml-auto text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={refresh}
          disabled={loading}
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </TooltipIconButton>
        <TooltipIconButton
          label="关闭版本管理"
          className="text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={onClose}
        >
          <MoreHorizontal className="h-3.5 w-3.5" />
        </TooltipIconButton>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="px-3 py-2">
          <RepositoryHeader workspace={workspace} status={status} changesCount={changes.length} />

        {!initialized ? (
          <div className="mt-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
            <div className="text-[var(--nova-text)]">尚未初始化版本仓库</div>
            <div className="mt-1 leading-5 text-[var(--nova-text-faint)]">初始化后可创建版本、查看历史并安全回滚。</div>
            <Button
              type="button"
              size="sm"
              className="mt-3 w-full border border-[var(--nova-border)] bg-[var(--nova-active)] font-medium text-[var(--nova-text)] hover:bg-[var(--nova-hover)]"
              onClick={() => initMutation.mutate()}
              disabled={loading || !workspace}
            >
              初始化版本仓库
            </Button>
          </div>
        ) : (
          <>
            <div className="mt-3">
              <div className="mb-1 flex items-center gap-2 text-[11px] uppercase tracking-wide text-[var(--nova-text-faint)]">
                <GitCommit className="h-3.5 w-3.5" />
                <span>Commit Changes</span>
              </div>
              <Textarea
                value={message}
                onChange={(event) => setMessage(event.target.value)}
                placeholder={clean ? '暂无可提交变更' : '输入本次版本说明'}
                rows={2}
                className="nova-field min-h-0 resize-none px-2 py-1.5 text-xs leading-5 placeholder:text-[var(--nova-text-faint)] focus-visible:ring-0"
                disabled={loading || clean}
              />
              <Button
                type="button"
                size="sm"
                className="mt-2 flex w-full items-center justify-center gap-2 border border-[var(--nova-border)] bg-[var(--nova-active)] font-medium text-[var(--nova-text)] hover:bg-[var(--nova-hover)] disabled:opacity-45"
                onClick={createVersion}
                disabled={!canCommit}
              >
                <span>✓</span>
                <span>Commit</span>
              </Button>
              <div className="mt-2 grid grid-cols-2 gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="nova-nav-item flex items-center justify-center gap-1 border-[var(--nova-border)] bg-transparent text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-40"
                  onClick={() => stashMutation.mutate()}
                  disabled={loading || clean}
                  title={clean ? '当前没有可暂存的未提交变更' : '暂存当前未提交内容'}
                >
                  <Archive className="h-3.5 w-3.5" />
                  Stash
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="nova-nav-item flex items-center justify-center gap-1 border-[var(--nova-border)] bg-transparent text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-40"
                  onClick={() => popMutation.mutate()}
                  disabled={loading}
                  title="恢复最近一次暂存内容"
                >
                  <Undo2 className="h-3.5 w-3.5" />
                  Pop
                </Button>
              </div>
            </div>

            <SectionHeader
              title="Changes"
              count={changes.length}
              expanded={changesExpanded}
              onToggle={() => setChangesExpanded(value => !value)}
            />
            {changesExpanded && <ChangesList changes={changes} />}

            <SectionHeader
              title="Graph"
              count={versions.length}
              expanded={historyExpanded}
              onToggle={() => setHistoryExpanded(value => !value)}
            />
            {historyExpanded && (
              <VersionTimeline
                versions={versions}
                canRollback={Boolean(status?.clean)}
                loading={loading}
                onRollback={setRollbackVersion}
              />
            )}
          </>
        )}

        {error && (
          <div className="mt-3 rounded border border-red-500/30 bg-red-500/10 px-2 py-1.5 leading-5 text-red-200">
            {error}
          </div>
        )}

          <div className="mt-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
            <button
              type="button"
              className="nova-nav-item flex w-full items-center gap-2 px-2 py-1.5 text-left text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]"
              onClick={() => setOutputExpanded(value => !value)}
            >
              {outputExpanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
              <span className="truncate">{operationSummary}</span>
            </button>
            {outputExpanded && (
              <pre className="max-h-40 overflow-auto border-t border-[var(--nova-border)] p-2 font-mono text-[11px] leading-5 text-[var(--nova-text-faint)] whitespace-pre-wrap">
                {operationOutput || '暂无原始输出'}
              </pre>
            )}
          </div>
        </div>
      </ScrollArea>

      <RollbackDialog
        open={Boolean(rollbackVersion)}
        version={rollbackVersion}
        loading={rollbackMutation.isPending}
        onOpenChange={(open) => { if (!open) setRollbackVersion(null) }}
        onRollback={handleRollback}
      />
    </div>
  )
}

function RepositoryHeader({ workspace, status, changesCount }: { workspace: string; status: GitStatus | null; changesCount: number }) {
  const branch = status?.branch || 'master'
  const initialized = status?.initialized ?? false
  const clean = status?.clean ?? true

  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
      <div className="flex items-center gap-2">
        <GitBranch className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="min-w-0 flex-1 truncate font-medium text-[var(--nova-text)]">{workspaceName(workspace) || '未选择书籍'}</span>
        {initialized && <span className="rounded-full bg-[var(--nova-active)] px-2 py-0.5 text-[11px] text-[var(--nova-text)]">{branch}</span>}
      </div>
      <div className="mt-2 flex items-center gap-2 text-[11px] text-[var(--nova-text-faint)]">
        <span className={initialized ? clean ? 'text-[var(--nova-accent-green)]' : 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-faint)]'}>
          {!initialized ? '未初始化' : clean ? '工作区干净' : `${changesCount} 个变更`}
        </span>
        {workspace && <span className="min-w-0 flex-1 truncate" title={workspace}>{workspace}</span>}
      </div>
    </div>
  )
}

function SectionHeader({ title, count, expanded, onToggle }: { title: string; count: number; expanded: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      className="nova-nav-item mt-3 flex w-full items-center gap-1 rounded-[var(--nova-radius)] py-1 text-left font-semibold text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
      onClick={onToggle}
    >
      {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
      <span>{title}</span>
      <span className="ml-auto rounded-full bg-[var(--nova-active)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{count}</span>
    </button>
  )
}

function ChangesList({ changes }: { changes: GitChange[] }) {
  if (changes.length === 0) {
    return <div className="rounded bg-[var(--nova-surface)] px-2 py-2 text-[var(--nova-text-faint)]">暂无变更</div>
  }
  return (
    <div className="space-y-0.5">
      {changes.map(change => (
        <div key={`${change.status}:${change.path}`} className="group flex items-center gap-2 rounded px-1.5 py-1 hover:bg-[var(--nova-hover)]" title={change.path}>
          <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[9px] text-[var(--nova-text-muted)]">M</span>
          <span className="min-w-0 flex-1 truncate text-[var(--nova-text-muted)]">{fileName(change.path)}</span>
          <span className="truncate text-[10px] text-[var(--nova-text-faint)]">{dirName(change.path)}</span>
          <span className={`shrink-0 text-[11px] ${statusColor(change.status)}`}>{change.status.trim() || 'M'}</span>
        </div>
      ))}
    </div>
  )
}

function normalizeStatus(status: GitStatus): GitStatus {
  return {
    ...status,
    changes: status?.changes ?? [],
  }
}

function commitToVersionItem(commit: GitCommitInfo): VersionItem {
  return {
    id: commit.hash,
    title: commit.subject || '(无说明)',
    description: commit.short_hash,
    createdAt: commit.date,
    author: commit.author,
  }
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

function shortHash(hash: string) {
  return hash.slice(0, 7)
}

function statusColor(status: string) {
  if (status.includes('D')) return 'text-red-300'
  if (status.includes('U') || status.includes('?')) return 'text-[var(--nova-accent-green)]'
  if (status.includes('A')) return 'text-[var(--nova-text-muted)]'
  return 'text-[var(--nova-accent)]'
}
