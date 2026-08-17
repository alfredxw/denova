import { useMemo, useState } from 'react'
import { ArrowRight, GitBranch, Loader2, Map as MapIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { BranchSummary, PlotNode, Snapshot } from '../../types'
import { branchDisplayName } from '../branching/model'

interface BranchPreviewProps {
  branches: BranchSummary[]
  currentBranchId: string
  snapshot: Snapshot | null
  onSwitchBranch: (branchId: string) => void | Promise<void>
  onOpenTimeline: () => void
}

export function BranchPreview({ branches, currentBranchId, snapshot, onSwitchBranch, onOpenTimeline }: BranchPreviewProps) {
  const { t } = useTranslation()
  const [switchingBranchId, setSwitchingBranchId] = useState('')
  const [switchError, setSwitchError] = useState('')
  const resolvedBranches = useMemo(
    () => resolvePreviewBranches(branches, currentBranchId, snapshot),
    [branches, currentBranchId, snapshot],
  )
  const graphNodes = snapshot?.graph?.nodes || []
  const labels = {
    main: t('branchTimeline.mainBranch'),
    unknown: t('branchTimeline.unknownBranch'),
  }

  const switchBranch = async (branch: BranchSummary) => {
    if (branch.id === currentBranchId || switchingBranchId) return
    setSwitchingBranchId(branch.id)
    setSwitchError('')
    try {
      await onSwitchBranch(branch.id)
    } catch (error) {
      console.error('[director-console] switch story branch from preview failed', { branchId: branch.id, error })
      setSwitchError(t('directorPanel.branches.switchFailed'))
    } finally {
      setSwitchingBranchId('')
    }
  }

  return (
    <section data-testid="director-branch-preview" className="flex h-full min-h-0 flex-col">
      <div className="director-console__scroll min-h-0 flex-1 overflow-y-auto px-4 py-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="director-console__display truncate text-sm font-semibold text-[var(--nova-text)]">{t('directorPanel.branches.title')}</h3>
            <p className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">{t('directorPanel.branches.description')}</p>
          </div>
          <span className="shrink-0 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 font-mono text-[10px] text-[var(--nova-text-muted)]">
            {resolvedBranches.length}
          </span>
        </div>

        {resolvedBranches.length > 0 ? (
          <div className="mt-4 space-y-2">
            {resolvedBranches.map((branch, index) => {
              const current = branch.id === currentBranchId
              const name = branchDisplayName(branch, labels)
              const branchNodes = graphNodes.filter((node) => node.branch_id === branch.id)
              const nodeCount = previewNodeCount(branch, branchNodes, snapshot)
              const origin = graphNodes.find((node) => node.id === (branch.from_event || branch.from))
              const head = graphNodes.find((node) => node.id === branch.head)
              const terminal = head?.terminal === true || (current && snapshot?.current_turn?.terminal_outcome?.terminal === true)
              const empty = branch.id !== 'main' && Boolean(branch.from_event) && branch.head === branch.from_event
              const busy = switchingBranchId === branch.id
              return (
                <button
                  key={branch.id}
                  type="button"
                  aria-current={current ? 'true' : undefined}
                  aria-label={current ? t('directorPanel.branches.currentNamed', { name }) : t('directorPanel.branches.switchTo', { name })}
                  disabled={Boolean(switchingBranchId)}
                  onClick={() => void switchBranch(branch)}
                  className={cn(
                    'group relative flex w-full min-w-0 items-stretch gap-3 rounded-[var(--nova-radius)] border px-3 py-2.5 text-left outline-none transition-colors focus-visible:ring-2 focus-visible:ring-[var(--director-brass)]/45 disabled:cursor-wait',
                    current
                      ? 'border-[color-mix(in_srgb,var(--director-brass)_45%,var(--nova-border))] bg-[color-mix(in_srgb,var(--director-brass)_8%,var(--nova-surface))]'
                      : 'border-[var(--nova-border)] bg-[var(--nova-surface)] hover:bg-[var(--nova-hover)]',
                  )}
                >
                  <span aria-hidden="true" className="relative flex w-3 shrink-0 justify-center">
                    {index < resolvedBranches.length - 1 ? <span className="absolute bottom-[-18px] top-3 w-px bg-[var(--nova-border)]" /> : null}
                    <span className={cn('relative mt-1 h-2.5 w-2.5 rounded-full border-2', current ? 'border-[var(--director-brass)] bg-[var(--director-brass)]' : 'border-[var(--nova-text-faint)] bg-[var(--director-canvas)]')} />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--nova-text)]">{name}</span>
                      {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-muted)]" /> : null}
                      {current ? <span className="shrink-0 text-[10px] font-medium text-[var(--director-brass)]">{t('directorPanel.branches.current')}</span> : null}
                    </span>
                    <span className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">
                      <span>{t('directorPanel.branches.turnCount', { count: nodeCount })}</span>
                      {empty ? <span>{t('directorPanel.branches.empty')}</span> : null}
                      {terminal ? <span className="text-[var(--director-ember)]">{t('branchTimeline.terminalBadge')}</span> : null}
                    </span>
                    {origin ? <span className="mt-0.5 block truncate text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.branches.from', { title: origin.title })}</span> : null}
                  </span>
                </button>
              )
            })}
          </div>
        ) : (
          <div className="mt-4 flex min-h-40 flex-col items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 text-center">
            <GitBranch className="h-5 w-5 text-[var(--nova-text-faint)]" />
            <p className="mt-2 text-xs font-medium text-[var(--nova-text-muted)]">{t('directorPanel.branches.none')}</p>
            <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.branches.noneHint')}</p>
          </div>
        )}

        {switchError ? <div role="alert" className="mt-3 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] p-2 text-xs text-[var(--nova-danger)]">{switchError}</div> : null}
      </div>

      <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--director-canvas)] p-3">
        <Button variant="outline" className="w-full justify-between border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]" onClick={onOpenTimeline}>
          <span className="flex min-w-0 items-center gap-2">
            <MapIcon className="h-3.5 w-3.5" />
            <span className="truncate">{t('directorPanel.branches.openTimeline')}</span>
          </span>
          <ArrowRight className="h-3.5 w-3.5" />
        </Button>
      </div>
    </section>
  )
}

function resolvePreviewBranches(branches: BranchSummary[], currentBranchId: string, snapshot: Snapshot | null) {
  const merged = new Map<string, BranchSummary>()
  for (const branch of branches) merged.set(branch.id, branch)
  for (const branch of snapshot?.graph?.branches || []) merged.set(branch.id, branch)

  if (currentBranchId && !merged.has(currentBranchId)) {
    merged.set(currentBranchId, {
      id: currentBranchId,
      head: snapshot?.current_turn?.id || snapshot?.turns.at(-1)?.id || '',
      title: currentBranchId,
      created_at: snapshot?.turns[0]?.ts || '',
      current: true,
    })
  }

  return Array.from(merged.values()).sort((left, right) => {
    if (left.id === 'main') return -1
    if (right.id === 'main') return 1
    return left.created_at.localeCompare(right.created_at) || left.id.localeCompare(right.id)
  })
}

function previewNodeCount(branch: BranchSummary, graphNodes: PlotNode[], snapshot: Snapshot | null) {
  if (graphNodes.length > 0) return graphNodes.length
  if (snapshot?.branch_id !== branch.id) return 0
  return snapshot.turn_count ?? snapshot.turns.length
}
