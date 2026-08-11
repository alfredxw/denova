import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, GitBranch, Network, Pencil, Play, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { formatDateTime } from '@/i18n'
import type { BranchSummary, PlotNode, Snapshot } from '../types'
import { BranchTimeline } from './BranchTimeline'

interface StorylinesViewProps {
  snapshot: Snapshot | null
  branches: BranchSummary[]
  currentBranchId: string
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onContinueBranch: (branchId: string) => void | Promise<void>
  onRenameBranch: (branchId: string, title: string) => void | Promise<void>
  onCreateBranch: (turnId: string, title: string) => void | Promise<void>
  onDeleteBranch: (branchId: string) => void | Promise<void>
  onBackToStory: () => void
  headerControls?: ReactNode
}

interface StorylineListItem {
  branch: BranchSummary
  title: string
  parentName: string
  divergenceTitle: string
  headSummary: string
  turnCount: number
  updatedAt: string
  current: boolean
}

/** 手机端剧情线默认界面：纵向列表 + 分支详情，关系图作为可切换的次级总览。 */
export function StorylinesView({
  snapshot,
  branches,
  currentBranchId,
  onSwitchBranch,
  onContinueBranch,
  onRenameBranch,
  onCreateBranch,
  onDeleteBranch,
  onBackToStory,
  headerControls,
}: StorylinesViewProps) {
  const { t } = useTranslation()
  const [activeBranchId, setActiveBranchId] = useState<string | null>(null)
  const [showGraph, setShowGraph] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<BranchSummary | null>(null)
  const [renameTarget, setRenameTarget] = useState<{ branchId: string; title: string } | null>(null)
  const [renaming, setRenaming] = useState(false)
  const [renameError, setRenameError] = useState('')

  const graphNodes = useMemo(() => snapshot?.graph?.nodes || [], [snapshot])
  const graphBranches = useMemo(() => {
    if (snapshot?.graph?.branches?.length) return snapshot.graph.branches
    return branches
  }, [branches, snapshot])
  const items = useMemo(
    () => buildStorylineList(graphNodes, graphBranches, currentBranchId, t),
    [currentBranchId, graphBranches, graphNodes, t],
  )
  const activeItem = items.find((item) => item.branch.id === activeBranchId) ?? null
  const activeNodes = useMemo(
    () => activeBranchId ? orderTimelineNodes(graphNodes.filter((node) => node.branch_id === activeBranchId)) : [],
    [activeBranchId, graphNodes],
  )

  const switchToBranch = async (branchId: string) => {
    await onSwitchBranch(branchId)
    setActiveBranchId(null)
  }

  const continueBranch = async (branchId: string) => {
    await onContinueBranch(branchId)
    setActiveBranchId(null)
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    try {
      await onDeleteBranch(deleteTarget.id)
      if (activeBranchId === deleteTarget.id) setActiveBranchId(null)
      setDeleteTarget(null)
    } catch (error) {
      console.error('[StorylinesView.tsx] 删除剧情线失败', { branchId: deleteTarget.id, error })
      toast.error(t('storylines.deleteFailed'))
      setDeleteTarget(null)
    }
  }

  const openRenameDialog = () => {
    if (!activeItem) return
    setRenameTarget({ branchId: activeItem.branch.id, title: activeItem.title })
    setRenameError('')
  }

  const submitRename = async () => {
    if (!renameTarget || renaming) return
    const title = renameTarget.title.trim()
    if (!title) {
      setRenameError(t('storylines.renameRequired'))
      return
    }
    setRenaming(true)
    setRenameError('')
    try {
      await onRenameBranch(renameTarget.branchId, title)
      setRenameTarget(null)
    } catch (error) {
      console.error('[StorylinesView.tsx] 重命名剧情线失败', { branchId: renameTarget.branchId, error })
      setRenameError(error instanceof Error ? error.message : t('storylines.renameFailed'))
    } finally {
      setRenaming(false)
    }
  }

  if (showGraph) {
    return (
      <div className="flex h-full min-h-0 flex-col bg-[var(--nova-surface)]">
        <div className="nova-topbar flex min-h-11 shrink-0 items-center gap-2 border-b px-3 py-2">
          <Button variant="ghost" size="xs" className="nova-nav-item gap-1.5" onClick={() => setShowGraph(false)}>
            <ArrowLeft className="h-4 w-4" />
            <span className="hidden sm:inline">{t('storylines.backToList')}</span>
          </Button>
          <span className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--nova-text-faint)]">
            {t('storylines.graphOverview')}
          </span>
        </div>
        <div className="min-h-0 flex-1">
          <BranchTimeline
            snapshot={snapshot}
            branches={branches}
            currentBranchId={currentBranchId}
            onSwitchBranch={onSwitchBranch}
            onCreateBranch={onCreateBranch}
            onDeleteBranch={onDeleteBranch}
            fill
            variant="workspace"
            onBackToStory={onBackToStory}
            headerControls={headerControls}
          />
        </div>
      </div>
    )
  }

  const view = activeItem ? (
      <div className="flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
        <header className="nova-topbar flex min-h-12 shrink-0 items-center gap-2 border-b px-3 py-2">
          <Button
            variant="ghost"
            size="xs"
            className="nova-nav-item gap-1.5"
            onClick={() => setActiveBranchId(null)}
            aria-label={t('storylines.backToList')}
          >
            <ArrowLeft className="h-4 w-4" />
            <span className="hidden sm:inline">{t('storylines.backToList')}</span>
          </Button>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium text-[var(--nova-text)]" title={activeItem.title}>{activeItem.title}</div>
            <div className="truncate text-[11px] text-[var(--nova-text-faint)]" title={activeItem.turnCount > 0 ? t('storylines.turnCount', { count: activeItem.turnCount }) : t('storylines.emptyBranch')}>
              {activeItem.turnCount > 0
                ? t('storylines.turnCount', { count: activeItem.turnCount })
                : t('storylines.emptyBranch')}
            </div>
          </div>
          <Button variant="outline" size="xs" className="nova-nav-item gap-1.5" onClick={() => setShowGraph(true)} aria-label={t('storylines.graphOverview')}>
            <Network className="h-4 w-4" />
            <span className="hidden sm:inline">{t('storylines.graphOverview')}</span>
          </Button>
          <Button
            variant="ghost"
            size="xs"
            className="nova-nav-item gap-1.5 text-[var(--nova-danger)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
            onClick={() => setDeleteTarget(activeItem.branch)}
            aria-label={t('storylines.deleteBranch', { name: activeItem.title })}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="xs"
            className="nova-nav-item gap-1.5"
            onClick={openRenameDialog}
            aria-label={t('storylines.renameBranch', { name: activeItem.title })}
          >
            <Pencil className="h-4 w-4" />
          </Button>
        </header>

        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[var(--nova-border)] px-3 py-2">
          <Button size="xs" className="nova-nav-item gap-1.5" onClick={() => void continueBranch(activeItem.branch.id)}>
            <Play className="h-4 w-4" />
            {t('storylines.continuePlaying')}
          </Button>
          <Button
            variant="outline"
            size="xs"
            className="nova-nav-item gap-1.5"
            onClick={() => void switchToBranch(activeItem.branch.id)}
            disabled={activeItem.current}
          >
            {t('storylines.switchBranch')}
          </Button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-3" data-testid="storylines-detail-timeline">
          {activeNodes.length === 0 ? (
            <div className="py-10 text-center text-xs text-[var(--nova-text-faint)]">{t('storylines.emptyBranch')}</div>
          ) : (
            <ol className="space-y-3">
              {activeNodes.map((node, index) => (
                <li key={node.id} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
                  <div className="flex items-center gap-2">
                    <span className="text-[10px] font-medium text-[var(--nova-text-faint)]">{index + 1}</span>
                    <span className="min-w-0 flex-1 truncate text-sm font-medium text-[var(--nova-text)]" title={node.title}>{node.title}</span>
                    {node.current && <Badge variant="outline" className="border-[var(--nova-accent)]/35 bg-[var(--nova-accent)]/10 text-[var(--nova-accent)]">{t('storylines.current')}</Badge>}
                    {node.head && <Badge variant="outline" className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)]">{t('storylines.head')}</Badge>}
                    {node.terminal && <Badge variant="outline" className="border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]">{t('branchTimeline.terminalBadge')}</Badge>}
                  </div>
                  {node.summary && <p className="mt-1 line-clamp-2 text-xs leading-5 text-[var(--nova-text-muted)]">{node.summary}</p>}
                  {node.ts && <time className="mt-1 block text-[10px] text-[var(--nova-text-faint)]">{formatDateTime(node.ts)}</time>}
                </li>
              ))}
            </ol>
          )}
        </div>
      </div>
  ) : (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      <header className="nova-topbar flex min-h-12 shrink-0 items-center gap-2 border-b px-3 py-2">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <GitBranch className="h-4 w-4 shrink-0 text-[var(--nova-accent-blue)]" />
          <span className="shrink-0 truncate text-sm font-medium text-[var(--nova-text)]" title={t('storylines.title')}>{t('storylines.title')}</span>
          {headerControls}
        </div>
        <Button variant="outline" size="xs" className="nova-nav-item gap-1.5" onClick={() => setShowGraph(true)} aria-label={t('storylines.graphOverview')}>
          <Network className="h-4 w-4" />
          <span className="hidden sm:inline">{t('storylines.graphOverview')}</span>
        </Button>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto p-3" data-testid="storylines-branch-list">
        {items.length === 0 ? (
          <div className="py-10 text-center text-xs text-[var(--nova-text-faint)]">{t('storylines.empty')}</div>
        ) : (
          <ol className="space-y-2">
            {items.map((item) => (
              <li key={item.branch.id}>
                <button
                  type="button"
                  className="nova-nav-item flex w-full min-h-14 flex-col items-stretch gap-1 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2.5 text-left hover:bg-[var(--nova-hover)]"
                  onClick={() => setActiveBranchId(item.branch.id)}
                  aria-label={t('storylines.openBranch', { title: item.title })}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="min-w-0 flex-1 truncate text-sm font-medium text-[var(--nova-text)]" title={item.title}>{item.title}</span>
                    {item.current && <Badge variant="outline" className="shrink-0 border-[var(--nova-accent)]/35 bg-[var(--nova-accent)]/10 text-[var(--nova-accent)]">{t('storylines.current')}</Badge>}
                    <Badge variant="outline" className="shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">{t('storylines.turnCount', { count: item.turnCount })}</Badge>
                  </span>
                  <span className="line-clamp-1 text-xs text-[var(--nova-text-muted)]">
                    {item.headSummary || item.divergenceTitle || t('storylines.noSummary')}
                  </span>
                  <span className="truncate text-[10px] text-[var(--nova-text-faint)]">
                    {[item.divergenceTitle ? t('storylines.divergenceFrom', { title: item.divergenceTitle }) : '', item.parentName ? t('storylines.fromBranch', { name: item.parentName }) : '', item.updatedAt ? t('storylines.updatedAt', { time: formatDateTime(item.updatedAt) }) : '']
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                </button>
              </li>
            ))}
          </ol>
        )}
      </div>
    </div>
  )

  return (
    <>
      {view}
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t('storylines.deleteBranchTitle')}
        description={t('storylines.confirmDelete', { name: deleteTarget ? storylineTitle(deleteTarget, t) : '' })}
        confirmLabel={t('common.delete')}
        tone="danger"
        onConfirm={() => void confirmDelete()}
      />
      <Dialog
        open={Boolean(renameTarget)}
        onOpenChange={(open) => {
          if (!open && !renaming) setRenameTarget(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('storylines.renameDialogTitle')}</DialogTitle>
            <DialogDescription>{renameTarget ? t('storylines.renameDialogDescription', { name: renameTarget.title }) : ''}</DialogDescription>
          </DialogHeader>
          <Input
            className="nova-field text-sm"
            value={renameTarget?.title ?? ''}
            onChange={(event) => setRenameTarget((current) => current ? { ...current, title: event.target.value } : current)}
            placeholder={t('storylines.renamePlaceholder')}
            aria-label={t('storylines.renamePlaceholder')}
          />
          {renameError && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] p-2 text-xs text-[var(--nova-danger)]">{renameError}</div>}
          <DialogFooter>
            <Button variant="ghost" onClick={() => setRenameTarget(null)} disabled={renaming}>{t('common.cancel')}</Button>
            <Button className="gap-1.5 border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]" onClick={() => void submitRename()} disabled={renaming || !renameTarget?.title.trim()}>
              <Pencil className="h-4 w-4" />
              {renaming ? t('common.saving') : t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function buildStorylineList(
  nodes: PlotNode[],
  branches: BranchSummary[],
  currentBranchId: string,
  t: (key: string, options?: Record<string, unknown>) => string,
): StorylineListItem[] {
  let sourceBranches = branches
  if (sourceBranches.length === 0 && nodes.length > 0) {
    const byId = new Map<string, BranchSummary>()
    for (const node of nodes) {
      const existing = byId.get(node.branch_id)
      if (!existing) {
        byId.set(node.branch_id, {
          id: node.branch_id,
          head: node.head ? node.id : '',
          title: node.branch_id === 'main' ? undefined : node.branch_id,
          created_at: node.ts,
          current: node.branch_id === currentBranchId,
        })
      } else if (node.head) {
        existing.head = node.id
      }
    }
    sourceBranches = Array.from(byId.values())
  }

  const items = sourceBranches.map((branch) => {
    const branchNodes = nodes.filter((node) => node.branch_id === branch.id)
    const headNode = branchNodes.find((node) => node.id === branch.head) ?? branchNodes[branchNodes.length - 1]
    const divergenceNode = branch.from_event ? nodes.find((node) => node.id === branch.from_event) : undefined
    const parent = branch.from ? sourceBranches.find((candidate) => candidate.id === branch.from) : undefined
    const updatedAt = branchNodes.reduce((latest, node) => (node.ts > latest ? node.ts : latest), branch.created_at || '')
    return {
      branch,
      title: storylineTitle(branch, t),
      parentName: parent ? storylineTitle(parent, t) : '',
      divergenceTitle: divergenceNode?.title || '',
      headSummary: headNode?.summary || '',
      turnCount: branchNodes.length,
      updatedAt,
      current: branch.id === currentBranchId,
    }
  })

  return items.sort((a, b) => {
    if (a.current !== b.current) return a.current ? -1 : 1
    const timeDelta = b.updatedAt.localeCompare(a.updatedAt)
    if (timeDelta !== 0) return timeDelta
    return a.title.localeCompare(b.title)
  })
}

function orderTimelineNodes(nodes: PlotNode[]): PlotNode[] {
  return [...nodes].sort((a, b) => {
    const timeDelta = a.ts.localeCompare(b.ts)
    if (timeDelta !== 0) return timeDelta
    return a.id.localeCompare(b.id)
  })
}

function storylineTitle(branch: BranchSummary, t: (key: string, options?: Record<string, unknown>) => string): string {
  if (branch.id === 'main') return t('storylines.mainBranch')
  if (branch.title?.trim()) return branch.title.trim()
  return branch.id || t('storylines.unknownBranch')
}
