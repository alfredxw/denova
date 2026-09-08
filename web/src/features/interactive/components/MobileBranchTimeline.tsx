import { useMemo, useState } from 'react'
import { ArrowLeft, ChevronDown, GitBranch, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { CreateBranchDialog } from './branching/CreateBranchDialog'
import { branchCreationSourceFromPlotNode, branchDisplayName, plotNodesFromSnapshot, type BranchCreationSource } from './branching/model'
import type { BranchTimelineProps } from './BranchTimeline'

/** A scrollable route on touch screens, using the same graph and branch commands. */
export function MobileBranchTimeline({ projectId, snapshot, branches, currentBranchId, onSwitchBranch, onCreateBranch, onDeleteBranch, onBackToStory, headerControls }: BranchTimelineProps) {
  const { t } = useTranslation()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [creationSource, setCreationSource] = useState<BranchCreationSource | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const nodes = useMemo(() => plotNodesFromSnapshot(snapshot, t), [snapshot, t])
  const routes = [...new Map([...(snapshot?.graph?.branches ?? []), ...branches].map((branch) => [branch.id, branch])).values()]
  const current = routes.find((branch) => branch.id === currentBranchId)
  const labels = { main: t('branchTimeline.mainBranch'), unknown: t('branchTimeline.unknownBranch') }
  const visibleNodes = useMemo(() => {
    const byId = new Map(nodes.map((node) => [node.id, node]))
    const path = new Set<string>()
    let id = current?.head || current?.from_event
    while (id && !path.has(id)) {
      path.add(id)
      id = byId.get(id)?.parent_id
    }
    return nodes.filter((node) => path.has(node.id) || node.branch_id === currentBranchId)
  }, [current?.head, current?.from_event, nodes, currentBranchId])
  const emptyBranch = current && current.id !== 'main' && current.head === current.from_event && !nodes.some((node) => node.branch_id === current.id)
  return (
    <section className="flex h-full min-h-0 min-w-0 flex-col bg-background" aria-label={t('branchTimeline.title')}>
      <header className="flex shrink-0 flex-col gap-2 border-b p-3">
        <div className="flex items-center gap-2">
          {onBackToStory ? <Button variant="ghost" onClick={onBackToStory}><ArrowLeft />{t('branchTimeline.backToStory')}</Button> : null}
          <h2 className="ml-auto text-sm font-medium">{t('branchTimeline.title')}</h2>
        </div>
        {headerControls}
        <Select value={currentBranchId} onValueChange={(id) => { setSelectedId(null); onSwitchBranch(id) }}>
          <SelectTrigger className="w-full" aria-label={t('branchTimeline.title')}><GitBranch /><SelectValue /></SelectTrigger>
          <SelectContent><SelectGroup>{routes.map((branch) => <SelectItem key={branch.id} value={branch.id}>{branchDisplayName(branch, labels)}</SelectItem>)}</SelectGroup></SelectContent>
        </Select>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-3">
        {emptyBranch ? <div className="mb-3 flex items-center justify-between gap-2 rounded-lg border p-3 text-sm text-muted-foreground"><span>{t('branchTimeline.emptyBranch')}</span><Button variant="ghost" aria-label={t('branchTimeline.deleteEmptyBranch')} onClick={() => setDeleteOpen(true)}><Trash2 /></Button></div> : null}
        {visibleNodes.length ? <ol className="flex flex-col gap-2">{visibleNodes.map((node, index) => (
          <li key={node.id} className="overflow-hidden rounded-xl border">
            <button type="button" className="flex w-full min-w-0 items-center gap-3 p-3 text-left" aria-expanded={selectedId === node.id} onClick={() => setSelectedId(selectedId === node.id ? null : node.id)}>
              <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{index + 1}</span>
              <span className="min-w-0 flex-1"><span className="block break-words text-sm font-medium">{node.title}</span><span className="mt-1 block text-xs text-muted-foreground">{node.id === current?.head ? t('branchTimeline.currentNode') : branchDisplayName(routes.find((branch) => branch.id === node.branch_id), labels)}{node.terminal ? ` · ${t('branchTimeline.terminalBadge')}` : ''}</span></span>
              <ChevronDown className={`size-4 shrink-0 ${selectedId === node.id ? 'rotate-180' : ''}`} />
            </button>
            {selectedId === node.id ? <div className="flex flex-col gap-3 border-t p-3"><p className="whitespace-pre-wrap break-words text-sm leading-relaxed text-muted-foreground">{node.summary}</p><Button variant="outline" className="w-full" onClick={() => setCreationSource(branchCreationSourceFromPlotNode(node))}><Plus />{t('branchTimeline.createBranch')}</Button></div> : null}
          </li>
        ))}</ol> : <p className="p-4 text-center text-sm text-muted-foreground">{t('branchTimeline.noNodes')}</p>}
      </div>
      <CreateBranchDialog projectId={projectId} source={creationSource} onClose={() => setCreationSource(null)} onCreate={(source, title, customAgentId) => onCreateBranch(source.turnId, title, customAgentId)} />
      <ConfirmDialog open={deleteOpen} onOpenChange={setDeleteOpen} title={t('branchTimeline.deleteEmptyBranch')} description={t('branchTimeline.confirmDeleteEmpty', { name: branchDisplayName(current, labels) })} confirmLabel={t('common.delete')} tone="danger" onConfirm={() => { if (emptyBranch) return onDeleteBranch(current.id) }} />
    </section>
  )
}
