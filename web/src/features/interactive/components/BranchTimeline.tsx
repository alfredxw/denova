import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode, type RefObject } from 'react'
import { ArrowLeft, ChevronDown, ChevronUp, Crosshair, GitBranch, Move, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import type { BranchSummary, PlotNode, Snapshot, TurnEvent } from '../types'

interface BranchTimelineProps {
  snapshot: Snapshot | null
  branches: BranchSummary[]
  currentBranchId: string
  onSwitchBranch: (branchId: string) => void
  onCreateBranch: (turnId: string, title: string) => void | Promise<void>
  onDeleteBranch: (branchId: string) => void
  expanded?: boolean
  fill?: boolean
  onExpandedChange?: (expanded: boolean) => void
  variant?: 'panel' | 'workspace'
  onBackToStory?: () => void
  headerControls?: ReactNode
}

interface TimelineRow {
  branchId: string
  branch?: BranchSummary
  nodes: PlotNode[]
  startColumn: number
  empty: boolean
  color: string
  colorSoft: string
}

interface PositionedNode {
  node: PlotNode
  row: number
  column: number
  x: number
  y: number
  color: string
  colorSoft: string
}

interface EmptyBranchMarker {
  branch: BranchSummary
  row: number
  column: number
  x: number
  y: number
  color: string
  colorSoft: string
  from?: PositionedNode
}

interface GraphLayout {
  rows: TimelineRow[]
  positionedNodes: PositionedNode[]
  nodeById: Map<string, PositionedNode>
  connections: Array<{ from: PositionedNode; to: PositionedNode | EmptyBranchMarker; branchChanged: boolean; color: string; dashed?: boolean }>
  emptyBranches: EmptyBranchMarker[]
  width: number
  height: number
  metrics: GraphMetrics
}

interface GraphMetrics {
  columnWidth: number
  laneHeight: number
  nodeCardWidth: number
  nodeDotX: number
  nodeCenterY: number
  left: number
  top: number
  right: number
  bottom: number
}

const DEFAULT_GRAPH_METRICS: GraphMetrics = {
  columnWidth: 220,
  laneHeight: 74,
  nodeCardWidth: 176,
  nodeDotX: 18,
  nodeCenterY: 26,
  left: 32,
  top: 26,
  right: 72,
  bottom: 24,
}

const BRANCH_COLORS = [
  { color: '#7fa7d9', soft: 'rgba(127,167,217,0.14)' },
  { color: '#d6aa62', soft: 'rgba(214,170,98,0.14)' },
  { color: '#81b38d', soft: 'rgba(129,179,141,0.14)' },
  { color: '#c98c8c', soft: 'rgba(201,140,140,0.14)' },
  { color: '#a795d8', soft: 'rgba(167,149,216,0.14)' },
  { color: '#72b8b7', soft: 'rgba(114,184,183,0.14)' },
]

export function BranchTimeline({
  snapshot,
  branches,
  currentBranchId,
  onSwitchBranch,
  onCreateBranch,
  onDeleteBranch,
  expanded: controlledExpanded,
  fill = false,
  onExpandedChange,
  variant = 'panel',
  onBackToStory,
  headerControls,
}: BranchTimelineProps) {
  const { t } = useTranslation()
  const [internalExpanded, setInternalExpanded] = useState(false)
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [selectedNodeSnapshot, setSelectedNodeSnapshot] = useState<PlotNode | null>(null)
  const [branchSourceNode, setBranchSourceNode] = useState<PlotNode | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [branchTitle, setBranchTitle] = useState('')
  const [creatingBranch, setCreatingBranch] = useState(false)
  const [createError, setCreateError] = useState('')
  const scrollRef = useRef<HTMLDivElement | null>(null)

  const graphNodes = useMemo(() => buildGraphNodes(snapshot, t), [snapshot, t])
  const graphBranches = useMemo(() => buildGraphBranches(snapshot, branches, graphNodes), [branches, graphNodes, snapshot])
  const selectedNode = graphNodes.find((node) => node.id === selectedNodeId) ||
    (selectedNodeSnapshot?.id === selectedNodeId ? selectedNodeSnapshot : null)
  const createSourceNode = branchSourceNode || selectedNode
  const workspaceMode = variant === 'workspace'
  const expanded = workspaceMode ? true : controlledExpanded ?? internalExpanded
  const scrollSize = useElementSize(scrollRef, expanded)
  useDragScroll(scrollRef, expanded)
  const emptyBranchCount = graphBranches.filter((branch) => isEmptyBranch(branch, graphNodes)).length
  const metrics = useMemo(() => buildGraphMetrics(scrollSize.width), [scrollSize.width])
  const layout = useMemo(() => buildGraphLayout(graphNodes, graphBranches, metrics, scrollSize), [graphBranches, graphNodes, metrics, scrollSize])

  const currentPositionedNode = useMemo(() => {
    const branchHead = graphBranches.find((branch) => branch.id === currentBranchId)?.head
    return layout.positionedNodes.find((item) => item.node.id === selectedNodeId) ||
      layout.positionedNodes.find((item) => item.node.id === branchHead) ||
      layout.positionedNodes.find((item) => item.node.current && item.node.branch_id === currentBranchId) ||
      layout.positionedNodes.find((item) => item.node.branch_id === currentBranchId && item.node.head) ||
      null
  }, [currentBranchId, graphBranches, layout.positionedNodes, selectedNodeId])

  useEffect(() => {
    if (!expanded || !currentPositionedNode) return
    const scroller = scrollRef.current
    if (!scroller) return
    window.requestAnimationFrame(() => {
      scrollElementTo(scroller, Math.max(0, currentPositionedNode.x - scroller.clientWidth * 0.35), Math.max(0, currentPositionedNode.y - scroller.clientHeight * 0.45), 'smooth')
    })
  }, [currentPositionedNode, expanded])

  const setExpanded = (nextExpanded: boolean) => {
    if (workspaceMode) return
    if (controlledExpanded === undefined) setInternalExpanded(nextExpanded)
    onExpandedChange?.(nextExpanded)
  }

  const selectNode = useCallback((node: PlotNode) => {
    setSelectedNodeId(node.id)
    setSelectedNodeSnapshot(node)
    if (node.branch_id !== currentBranchId) onSwitchBranch(node.branch_id)
  }, [currentBranchId, onSwitchBranch])

  // Pan back to the current/selected node. On desktop the MiniMap gives an
  // overview, but it is hidden below the sm breakpoint, so on a phone this is
  // the only way back after the user pans around the graph.
  const recenter = useCallback(() => {
    const scroller = scrollRef.current
    if (!scroller || !currentPositionedNode) return
    scrollElementTo(scroller, Math.max(0, currentPositionedNode.x - scroller.clientWidth * 0.35), Math.max(0, currentPositionedNode.y - scroller.clientHeight * 0.45), 'smooth')
  }, [currentPositionedNode])

  const openCreateDialog = () => {
    if (!selectedNode) return
    setBranchSourceNode(selectedNode)
    setBranchTitle(t('branchTimeline.newFromNode', { title: selectedNode.title }))
    setCreateError('')
    setCreateDialogOpen(true)
  }

  const handleCreateDialogOpenChange = (open: boolean) => {
    if (creatingBranch) return
    setCreateDialogOpen(open)
    if (open) return
    setBranchSourceNode(null)
    setBranchTitle('')
    setCreateError('')
  }

  const submitCreateBranch = async () => {
    if (!createSourceNode || creatingBranch) return
    setCreatingBranch(true)
    setCreateError('')
    try {
      await onCreateBranch(createSourceNode.id, branchTitle.trim() || t('branchTimeline.newBranch'))
      setCreateDialogOpen(false)
      setBranchSourceNode(null)
      setBranchTitle('')
    } catch (error) {
      setCreateError(error instanceof Error ? error.message : t('branchTimeline.createFailed'))
    } finally {
      setCreatingBranch(false)
    }
  }

  const deleteBranch = (branch: BranchSummary) => {
    const label = formatBranchName(branch, t)
    if (!window.confirm(t('branchTimeline.confirmDeleteEmpty', { name: label }))) return
    onDeleteBranch(branch.id)
    if (selectedNode?.branch_id === branch.id) setSelectedNodeId(null)
  }

  return (
    <div className={`${workspaceMode ? 'h-full min-h-0 border-0 p-4' : `${fill ? 'h-full min-h-0' : expanded ? 'h-[min(260px,calc(100vh-96px))] min-h-[180px]' : 'h-[48px]'} border-t px-3 py-2 transition-[height] sm:px-4`} flex flex-col border-[var(--nova-border)] bg-[var(--nova-surface)]`}>
      <div className="flex items-center justify-between gap-2 text-xs text-[var(--nova-text-faint)]">
        {workspaceMode ? (
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <div className="flex items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1 font-medium text-[var(--nova-text)]">
              <GitBranch className="h-3.5 w-3.5 text-[var(--nova-accent-blue)]" />
              {t('branchTimeline.title')}
            </div>
            {headerControls}
            {onBackToStory && (
              <Button variant="outline" size="xs" className="nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={onBackToStory}>
                <ArrowLeft className="h-3.5 w-3.5" />
                {t('branchTimeline.backToStory')}
              </Button>
            )}
          </div>
        ) : (
          <button type="button" className="nova-nav-item flex items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1 font-medium text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={() => setExpanded(!expanded)}>
            <GitBranch className="h-3.5 w-3.5 text-[var(--nova-accent-blue)]" />
            {t('branchTimeline.title')}
            {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronUp className="h-3.5 w-3.5" />}
          </button>
        )}
        <div className="flex min-w-0 flex-1 items-center justify-end gap-2 overflow-hidden">
          <span className="truncate text-[var(--nova-text-faint)]">{t('branchTimeline.nodeCount', { count: graphNodes.length || snapshot?.turns?.length || 0 })}</span>
          {emptyBranchCount > 0 && <Badge variant="outline" className="hidden border-[var(--nova-accent)]/35 bg-[var(--nova-accent)]/10 text-[var(--nova-accent)] sm:inline-flex">{t('branchTimeline.emptyBranchCount', { count: emptyBranchCount })}</Badge>}
          {selectedNode && (
            <Button variant="outline" size="xs" className="nova-nav-item hidden gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] sm:inline-flex" onClick={openCreateDialog}>
              <Plus className="h-3.5 w-3.5" />
              {t('branchTimeline.createFromSelected')}
            </Button>
          )}
        </div>
      </div>

      {expanded && (
        <div className="mt-2 flex min-h-0 flex-1 flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] shadow-[var(--nova-shadow)]">
          <div className="nova-topbar flex min-h-10 shrink-0 flex-wrap items-center justify-between gap-2 border-b px-3 py-1.5 sm:px-4">
            <div className="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto">
              {layout.rows.map((row) => (
                <button
                  key={row.branchId}
                  type="button"
                  className={`nova-nav-item flex h-7 max-md:min-h-9 shrink-0 items-center gap-2 rounded-[var(--nova-radius)] border px-2 text-xs transition ${row.branchId === currentBranchId ? 'is-active text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'}`}
                  style={row.branchId === currentBranchId ? { borderColor: row.color, background: row.colorSoft } : undefined}
                  onClick={() => onSwitchBranch(row.branchId)}
                  title={formatBranchName(row.branch, t)}
                >
                  <span className="h-2.5 w-2.5 rounded-full shadow-[0_0_10px_currentColor]" style={{ background: row.color, color: row.color }} />
                  <span className="max-w-32 truncate">{formatBranchName(row.branch, t)}</span>
                  <span className="text-[var(--nova-text-faint)]">{row.nodes.length}</span>
                </button>
              ))}
              {layout.rows.length === 0 && <span className="text-xs text-[var(--nova-text-faint)]">{t('branchTimeline.noRoutes')}</span>}
            </div>
            <div className="flex shrink-0 items-center gap-2 text-[var(--nova-text-faint)]">
              <span className="hidden items-center gap-1.5 text-xs sm:flex">
                <Move className="h-3.5 w-3.5" />
                {t('branchTimeline.dragHint')}
              </span>
              <Button size="xs" variant="outline" className="nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!selectedNode} onClick={openCreateDialog}>
                <Plus className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
                {t('branchTimeline.createBranch')}
              </Button>
            </div>
          </div>

          <div ref={scrollRef} className="min-h-0 flex-1 cursor-grab select-none overflow-auto overscroll-contain bg-[var(--nova-surface-2)] touch-none active:cursor-grabbing" data-testid="branch-graph-scroll">
            <div
              data-testid="branch-graph-canvas"
              data-edge-count={layout.connections.length}
              className="relative min-w-max"
              style={{ width: layout.width, height: layout.height }}
            >
              <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_1px_1px,rgba(148,163,184,0.12)_1px,transparent_0)] [background-size:18px_18px]" />
              <svg className="pointer-events-none absolute inset-0 overflow-visible" width={layout.width} height={layout.height} aria-hidden="true">
                {layout.connections.map((connection) => (
                  <path
                    key={`${connection.from.node.id}-${'node' in connection.to ? connection.to.node.id : connection.to.branch.id}`}
                    d={connectionPath(connection.from, connection.to, layout.metrics)}
                    fill="none"
                    stroke={connection.color}
                    strokeWidth={connection.branchChanged ? 2.6 : 2}
                    strokeDasharray={connection.dashed ? '4 6' : undefined}
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    opacity={0.9}
                  />
                ))}
              </svg>

              {layout.positionedNodes.map(({ node, x, y, color, colorSoft }) => (
                <button
                  key={node.id}
                  type="button"
                  data-no-drag
                  className={`absolute z-10 flex h-[52px] cursor-pointer items-center gap-2 overflow-hidden rounded-[var(--nova-radius)] border px-3 py-1.5 text-left shadow-[0_8px_18px_rgba(0,0,0,0.20)] backdrop-blur transition ${node.id === selectedNodeId ? 'text-[var(--nova-text)] ring-2 ring-white/10' : node.current ? 'text-[var(--nova-text)]' : 'border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:border-[var(--nova-active)] hover:text-[var(--nova-text)]'}`}
                  style={{
                    left: x,
                    top: y,
                    width: layout.metrics.nodeCardWidth,
                    background: node.id === selectedNodeId ? `linear-gradient(180deg, rgba(48,50,56,0.96), ${colorSoft})` : colorSoft,
                    borderColor: node.id === selectedNodeId || node.current ? color : undefined,
                    boxShadow: node.id === selectedNodeId
                      ? `0 10px 24px rgba(0,0,0,0.28), 0 0 0 1px ${color}33`
                      : undefined,
                  }}
                  onClick={() => selectNode(node)}
                  title={`${node.title}\n${node.summary}`}
                >
                  <span className="h-2.5 w-2.5 shrink-0 rounded-full shadow-[0_0_14px_currentColor]" style={{ background: color, color }} />
                  <span className="min-w-0 flex-1 overflow-hidden">
                    <span className="block truncate text-[12px] leading-4 font-medium">{node.title}</span>
                    <span className="mt-0.5 block truncate text-[11px] leading-4 text-[var(--nova-text-faint)]">{node.summary || t('branchTimeline.nodeFallback')}</span>
                  </span>
                  {node.head && <Badge variant="outline" className="h-5 max-w-12 shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 text-[10px] text-[var(--nova-text-muted)]">HEAD</Badge>}
                </button>
              ))}

              {layout.emptyBranches.map((empty) => (
                <div
                  key={empty.branch.id}
                  className="absolute z-10 flex h-[38px] cursor-grab items-center gap-2 rounded-[var(--nova-radius)] border border-dashed px-3 text-xs text-[var(--nova-text-muted)] active:cursor-grabbing"
                  style={{ left: empty.x, top: empty.y + 5, width: layout.metrics.nodeCardWidth, borderColor: empty.color, background: empty.colorSoft }}
                >
                  <span className="h-2.5 w-2.5 rounded-full" style={{ background: empty.color }} />
                  <span className="min-w-0 flex-1 truncate" title={formatBranchName(empty.branch, t)}>{t('branchTimeline.emptyBranch')}</span>
                  <button
                    type="button"
                    data-no-drag
                    className="rounded p-1 text-[var(--nova-danger)] opacity-75 hover:bg-[var(--nova-danger-bg)] hover:opacity-100"
                    onClick={() => deleteBranch(empty.branch)}
                    aria-label={t('branchTimeline.deleteEmptyBranchWithName', { name: formatBranchName(empty.branch, t) })}
                    title={t('branchTimeline.deleteEmptyBranch')}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              ))}

              {layout.rows.length === 0 && <span className="absolute left-6 top-6 text-xs text-[var(--nova-text-faint)]">{t('branchTimeline.noNodes')}</span>}
            </div>
          </div>

          <div className="flex min-h-[48px] shrink-0 items-center justify-between gap-3 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 text-xs text-[var(--nova-text-faint)] sm:px-4">
            {selectedNode ? (
              <div className="min-w-0">
                <span className="text-[var(--nova-text)]">{t('branchTimeline.selectedNode')}</span>
                <span className="truncate">{selectedNode.title}</span>
              </div>
            ) : (
              <span>{t('branchTimeline.selectHint')}</span>
            )}
            <Button size="xs" variant="outline" className="nova-nav-item shrink-0 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] sm:hidden" onClick={recenter} disabled={!currentPositionedNode} aria-label={t('branchTimeline.recenter')} title={t('branchTimeline.recenter')}>
              <Crosshair className="h-3.5 w-3.5" />
            </Button>
            <MiniMap layout={layout} scrollRef={scrollRef} ariaLabel={t('branchTimeline.minimap')} />
            {selectedNode && (
              <Button size="xs" className="shrink-0 gap-1.5 border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]" onClick={openCreateDialog}>
                <Plus className="h-3.5 w-3.5" />
                {t('branchTimeline.createBranch')}
              </Button>
            )}
          </div>
        </div>
      )}

      <Dialog open={createDialogOpen} onOpenChange={handleCreateDialogOpenChange}>
        <DialogContent className="nova-panel border text-[var(--nova-text)]">
          <DialogHeader>
            <DialogTitle>{t('branchTimeline.dialogTitle')}</DialogTitle>
            <DialogDescription className="text-[var(--nova-text-muted)]">
              {createSourceNode ? t('branchTimeline.dialogDescription', { title: createSourceNode.title }) : ''}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Input className="nova-field text-sm" value={branchTitle} onChange={(event) => setBranchTitle(event.target.value)} placeholder={t('branchTimeline.namePlaceholder')} />
            {createSourceNode?.summary && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2 text-xs leading-5 text-[var(--nova-text-muted)]">{createSourceNode.summary}</div>}
            {createError && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] p-2 text-xs text-[var(--nova-danger)]">{createError}</div>}
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => handleCreateDialogOpenChange(false)} disabled={creatingBranch}>{t('common.cancel')}</Button>
            <Button className="gap-1.5 border border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)] hover:bg-[var(--nova-hover)]" onClick={submitCreateBranch} disabled={!createSourceNode || creatingBranch}>
              <Plus className="h-4 w-4" />
              {creatingBranch ? t('common.creating') : t('branchTimeline.createAndSwitch')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function MiniMap({ layout, scrollRef, ariaLabel }: { layout: GraphLayout; scrollRef: RefObject<HTMLDivElement | null>; ariaLabel: string }) {
  const [viewport, setViewport] = useState({ left: 0, top: 0, width: 100, height: 100 })
  const draggingRef = useRef(false)

  const updateViewport = useCallback(() => {
    const scroller = scrollRef.current
    if (!scroller || layout.width <= 0 || layout.height <= 0) return
    setViewport({
      left: (scroller.scrollLeft / layout.width) * 100,
      top: (scroller.scrollTop / layout.height) * 100,
      width: Math.min(100, (scroller.clientWidth / layout.width) * 100),
      height: Math.min(100, (scroller.clientHeight / layout.height) * 100),
    })
  }, [layout.height, layout.width, scrollRef])

  useEffect(() => {
    const scroller = scrollRef.current
    if (!scroller) return
    updateViewport()
    scroller.addEventListener('scroll', updateViewport, { passive: true })
    const observer = new ResizeObserver(updateViewport)
    observer.observe(scroller)
    return () => {
      scroller.removeEventListener('scroll', updateViewport)
      observer.disconnect()
    }
  }, [scrollRef, updateViewport])

  const moveTo = (event: ReactPointerEvent<HTMLDivElement>) => {
    const scroller = scrollRef.current
    if (!scroller) return
    const rect = event.currentTarget.getBoundingClientRect()
    const ratioX = Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width))
    const ratioY = Math.min(1, Math.max(0, (event.clientY - rect.top) / rect.height))
    scrollElementTo(
      scroller,
      Math.max(0, ratioX * layout.width - scroller.clientWidth / 2),
      Math.max(0, ratioY * layout.height - scroller.clientHeight / 2),
      draggingRef.current ? 'auto' : 'smooth',
    )
  }

  return (
    <div
      className="group relative hidden h-10 min-w-[220px] max-w-[380px] flex-1 cursor-crosshair overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] shadow-[inset_0_1px_0_rgba(255,255,255,0.06),0_8px_22px_rgba(0,0,0,0.20)] sm:block"
      onPointerDown={(event) => {
        draggingRef.current = true
        event.currentTarget.setPointerCapture(event.pointerId)
        moveTo(event)
      }}
      onPointerMove={(event) => {
        if (draggingRef.current) moveTo(event)
      }}
      onPointerUp={(event) => {
        draggingRef.current = false
        event.currentTarget.releasePointerCapture(event.pointerId)
      }}
      onPointerCancel={() => {
        draggingRef.current = false
      }}
      aria-label={ariaLabel}
    >
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(180deg,rgba(255,255,255,0.045),rgba(255,255,255,0)_42%),radial-gradient(circle_at_50%_0%,rgba(180,184,192,0.12),transparent_62%)]" />
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-white/10" />
      <div className="pointer-events-none absolute inset-y-0 left-0 w-8 bg-gradient-to-r from-[var(--nova-surface-2)] to-transparent" />
      <div className="pointer-events-none absolute inset-y-0 right-0 w-8 bg-gradient-to-l from-[var(--nova-surface-2)] to-transparent" />
      <svg className="absolute inset-0 h-full w-full px-2 py-1.5" viewBox={`0 0 ${layout.width} ${layout.height}`} preserveAspectRatio="none" aria-hidden="true">
        <defs>
          <filter id="nova-minimap-soft-glow" x="-20%" y="-80%" width="140%" height="260%">
            <feGaussianBlur stdDeviation="1.6" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        {layout.connections.map((connection) => (
          <path
            key={`mini-${connection.from.node.id}-${'node' in connection.to ? connection.to.node.id : connection.to.branch.id}`}
            d={connectionPath(connection.from, connection.to, layout.metrics)}
            fill="none"
            stroke={connection.color}
            strokeWidth={connection.branchChanged ? 5 : 4}
            strokeDasharray={connection.dashed ? '10 12' : undefined}
            strokeLinecap="round"
            strokeLinejoin="round"
            opacity={connection.dashed ? 0.18 : 0.34}
          />
        ))}
        {layout.positionedNodes.map((item) => (
          <circle
            key={`mini-node-${item.node.id}`}
            cx={item.x + layout.metrics.nodeDotX}
            cy={item.y + layout.metrics.nodeCenterY}
            r={item.node.current || item.node.head ? 6 : 4.5}
            fill={item.node.current ? 'var(--nova-text)' : item.color}
            opacity={item.node.current ? 0.95 : 0.62}
            filter={item.node.current ? 'url(#nova-minimap-soft-glow)' : undefined}
          />
        ))}
      </svg>
      <div
        className="absolute rounded-[5px] border border-white/45 bg-white/10 shadow-[0_0_0_1px_rgba(0,0,0,0.35),0_0_18px_rgba(212,215,221,0.16),inset_0_1px_0_rgba(255,255,255,0.22)] transition-all duration-150 group-active:duration-0"
        style={{
          left: `${viewport.left}%`,
          top: `${viewport.top}%`,
          width: `${viewport.width}%`,
          height: `${viewport.height}%`,
        }}
      />
      <div className="pointer-events-none absolute inset-0 rounded-md ring-1 ring-black/30" />
    </div>
  )
}

function buildGraphMetrics(viewportWidth: number): GraphMetrics {
  if (viewportWidth > 0 && viewportWidth < 640) {
    const nodeCardWidth = Math.max(152, Math.min(DEFAULT_GRAPH_METRICS.nodeCardWidth, viewportWidth - 72))
    return {
      columnWidth: Math.max(nodeCardWidth + 26, 190),
      laneHeight: 76,
      nodeCardWidth,
      nodeDotX: 18,
      nodeCenterY: 24,
      left: 24,
      top: 28,
      right: 28,
      bottom: 28,
    }
  }

  if (viewportWidth >= 1180) {
    return { ...DEFAULT_GRAPH_METRICS, columnWidth: 270, right: 112 }
  }

  return DEFAULT_GRAPH_METRICS
}

function buildGraphLayout(nodes: PlotNode[], branches: BranchSummary[], metrics: GraphMetrics = DEFAULT_GRAPH_METRICS, viewport = { width: 0, height: 0 }): GraphLayout {
  const columnById = buildNodeColumns(nodes)
  const rowsByBranch = new Map<string, TimelineRow>()

  for (const [index, branch] of branches.entries()) {
    const palette = BRANCH_COLORS[index % BRANCH_COLORS.length]
    rowsByBranch.set(branch.id, {
      branchId: branch.id,
      branch,
      nodes: [],
      startColumn: Math.max(0, branch.from_event ? (columnById.get(branch.from_event) ?? 0) + 1 : 0),
      empty: isEmptyBranch(branch, nodes),
      color: palette.color,
      colorSoft: palette.soft,
    })
  }

  for (const node of nodes) {
    if (!rowsByBranch.has(node.branch_id)) {
      const palette = BRANCH_COLORS[rowsByBranch.size % BRANCH_COLORS.length]
      rowsByBranch.set(node.branch_id, { branchId: node.branch_id, nodes: [], startColumn: 0, empty: false, color: palette.color, colorSoft: palette.soft })
    }
    rowsByBranch.get(node.branch_id)?.nodes.push(node)
  }

  const rows = Array.from(rowsByBranch.values()).map((row) => ({
    ...row,
    nodes: row.nodes.sort((a, b) => {
      const columnDiff = (columnById.get(a.id) ?? 0) - (columnById.get(b.id) ?? 0)
      return columnDiff || a.id.localeCompare(b.id)
    }),
  }))

  const displayColumnById = new Map<string, number>()
  for (const row of rows) {
    let previousColumn = -1
    for (const node of row.nodes) {
      const column = Math.max(columnById.get(node.id) ?? 0, previousColumn + 1)
      displayColumnById.set(node.id, column)
      previousColumn = column
    }
  }

  let maxColumn = 0
  for (const node of nodes) maxColumn = Math.max(maxColumn, displayColumnById.get(node.id) ?? 0)
  for (const row of rows) maxColumn = Math.max(maxColumn, row.startColumn)

  const positionedNodes: PositionedNode[] = []
  const nodeById = new Map<string, PositionedNode>()
  rows.forEach((row, rowIndex) => {
    for (const node of row.nodes) {
      const column = displayColumnById.get(node.id) ?? 0
      const positioned = {
        node,
        row: rowIndex,
        column,
        x: metrics.left + column * metrics.columnWidth,
        y: metrics.top + rowIndex * metrics.laneHeight,
        color: row.color,
        colorSoft: row.colorSoft,
      }
      positionedNodes.push(positioned)
      nodeById.set(node.id, positioned)
    }
  })

  const connections: GraphLayout['connections'] = []
  const connectionKeys = new Set<string>()
  const addConnection = (from: PositionedNode | undefined, to: PositionedNode | EmptyBranchMarker | undefined, color: string, dashed = false) => {
    if (!from || !to) return
    const toId = 'node' in to ? to.node.id : to.branch.id
    const key = `${from.node.id}->${toId}`
    if (connectionKeys.has(key)) return
    connectionKeys.add(key)
    connections.push({ from, to, branchChanged: 'node' in to ? from.node.branch_id !== to.node.branch_id : true, color, dashed })
  }

  for (const positioned of positionedNodes) {
    if (!positioned.node.parent_id) continue
    addConnection(nodeById.get(positioned.node.parent_id), positioned, positioned.color)
  }
  for (const row of rows) {
    for (let index = 1; index < row.nodes.length; index += 1) {
      addConnection(nodeById.get(row.nodes[index - 1].id), nodeById.get(row.nodes[index].id), row.color)
    }
  }

  const emptyBranches = rows.flatMap((row, rowIndex) => {
    if (!row.branch || !row.empty) return []
    const column = row.startColumn
    return [{
      branch: row.branch,
      row: rowIndex,
      column,
      x: metrics.left + column * metrics.columnWidth,
      y: metrics.top + rowIndex * metrics.laneHeight,
      color: row.color,
      colorSoft: row.colorSoft,
      from: row.branch.from_event ? nodeById.get(row.branch.from_event) : undefined,
    }]
  })

  for (const empty of emptyBranches) {
    addConnection(empty.from, empty, empty.color, true)
  }

  return {
    rows,
    positionedNodes,
    nodeById,
    connections,
    emptyBranches,
    width: Math.max(viewport.width || 0, metrics.left + metrics.right + (maxColumn + 1) * metrics.columnWidth + metrics.nodeCardWidth),
    height: Math.max(viewport.height || 0, 180, metrics.top + metrics.bottom + rows.length * metrics.laneHeight),
    metrics,
  }
}

function connectionPath(from: Pick<PositionedNode, 'x' | 'y'>, to: Pick<PositionedNode | EmptyBranchMarker, 'x' | 'y'>, metrics: GraphMetrics = DEFAULT_GRAPH_METRICS) {
  const startX = from.x + metrics.nodeCardWidth
  const startY = from.y + metrics.nodeCenterY
  const endX = to.x
  const endY = to.y + metrics.nodeCenterY
  const curve = Math.max(52, Math.min(120, Math.abs(endX - startX) * 0.42))
  return `M ${startX} ${startY} C ${startX + curve} ${startY}, ${endX - curve} ${endY}, ${endX} ${endY}`
}

function buildNodeColumns(nodes: PlotNode[]) {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const columnById = new Map<string, number>()

  const getColumn = (nodeId: string, path = new Set<string>()): number => {
    const cached = columnById.get(nodeId)
    if (cached !== undefined) return cached
    if (path.has(nodeId)) return 0
    path.add(nodeId)
    const node = byId.get(nodeId)
    const column = node?.parent_id ? getColumn(node.parent_id, path) + 1 : 0
    path.delete(nodeId)
    columnById.set(nodeId, column)
    return column
  }

  for (const node of nodes) getColumn(node.id)
  return columnById
}

function isEmptyBranch(branch: BranchSummary, nodes: PlotNode[]) {
  return branch.id !== 'main' && branch.head === branch.from_event && !nodes.some((node) => node.branch_id === branch.id)
}

function buildGraphNodes(snapshot: Snapshot | null, t: (key: string, options?: Record<string, unknown>) => string): PlotNode[] {
  if (snapshot?.graph?.nodes?.length) return snapshot.graph.nodes
  return (snapshot?.turns || []).map((turn, index, turns) => turnToPlotNode(turn, index, turns.length, t))
}

function buildGraphBranches(snapshot: Snapshot | null, branches: BranchSummary[], nodes: PlotNode[]): BranchSummary[] {
  if (snapshot?.graph?.branches?.length) return snapshot.graph.branches
  if (branches.length) return branches
  if (!nodes.length) return []

  const summaries = new Map<string, BranchSummary>()
  for (const node of nodes) {
    const current = node.branch_id === (snapshot?.branch_id || 'main')
    const existing = summaries.get(node.branch_id)
    summaries.set(node.branch_id, {
      id: node.branch_id,
      head: node.head || !existing ? node.id : existing.head,
      title: node.branch_id === 'main' ? 'main' : node.branch_id,
      created_at: node.ts,
      current,
    })
  }
  return Array.from(summaries.values())
}

function turnToPlotNode(turn: TurnEvent, index: number, total: number, t: (key: string, options?: Record<string, unknown>) => string): PlotNode {
  const title = firstLine(turn.user || turn.narrative) || `${t('branchTimeline.nodeFallback')} ${index + 1}`
  return {
    id: turn.id,
    parent_id: turn.parent_id || undefined,
    branch_id: turn.branch_id || 'main',
    title: truncateText(title, 18),
    summary: truncateText(firstLine(turn.narrative) || t('branchTimeline.nodeFallback'), 28),
    ts: turn.ts,
    current: index === total - 1,
    head: index === total - 1,
  }
}

function firstLine(value: string) {
  return value.trim().split(/\r?\n/).find(Boolean) || ''
}

function truncateText(value: string, maxLength: number) {
  const text = value.trim()
  return text.length > maxLength ? `${text.slice(0, maxLength - 1)}…` : text
}

function formatBranchName(branch: BranchSummary | undefined, t: (key: string) => string) {
  if (!branch) return t('branchTimeline.unknownBranch')
  if (branch.id === 'main') return t('branchTimeline.mainBranch')
  if (branch.title?.trim()) return branch.title.trim()
  return branch.id
}

function scrollElementTo(element: HTMLElement, left: number, top: number, behavior: ScrollBehavior) {
  if (typeof element.scrollTo === 'function') {
    element.scrollTo({ left, top, behavior })
    return
  }
  element.scrollLeft = left
  element.scrollTop = top
}

function useElementSize(ref: RefObject<HTMLElement | null>, active: boolean) {
  const [size, setSize] = useState({ width: 0, height: 0 })

  useEffect(() => {
    if (!active) return
    const node = ref.current
    if (!node) return

    const updateSize = () => {
      setSize((current) => {
        const next = { width: node.clientWidth, height: node.clientHeight }
        return current.width === next.width && current.height === next.height ? current : next
      })
    }

    updateSize()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(updateSize)
    observer.observe(node)
    return () => observer.disconnect()
  }, [active, ref])

  return size
}

function useDragScroll(ref: RefObject<HTMLElement | null>, active: boolean) {
  const dragRef = useRef<{ x: number; y: number; left: number; top: number; active: boolean; moved: boolean; suppressClick: boolean }>({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    active: false,
    moved: false,
    suppressClick: false,
  })

  useEffect(() => {
    if (!active) return
    const node = ref.current
    if (!node) return
    const previousTouchAction = node.style.touchAction
    node.style.touchAction = 'none'

    const onPointerDown = (event: PointerEvent) => {
      if (event.pointerType === 'mouse' && event.button !== 0) return
      if (shouldIgnoreDragStart(event.target)) return
      dragRef.current = { x: event.clientX, y: event.clientY, left: node.scrollLeft, top: node.scrollTop, active: true, moved: false, suppressClick: false }
      node.setPointerCapture(event.pointerId)
    }
    const onPointerMove = (event: PointerEvent) => {
      if (!dragRef.current.active) return
      const deltaX = event.clientX - dragRef.current.x
      const deltaY = event.clientY - dragRef.current.y
      if (!dragRef.current.moved && Math.hypot(deltaX, deltaY) > 4) {
        dragRef.current.moved = true
        dragRef.current.suppressClick = true
      }
      if (!dragRef.current.moved) return
      event.preventDefault()
      node.scrollLeft = dragRef.current.left - deltaX
      node.scrollTop = dragRef.current.top - deltaY
    }
    const onPointerUp = (event: PointerEvent) => {
      dragRef.current.active = false
      if (node.hasPointerCapture(event.pointerId)) node.releasePointerCapture(event.pointerId)
    }
    const onPointerCancel = () => {
      dragRef.current.active = false
    }

    const onClickCapture = (event: MouseEvent) => {
      if (!dragRef.current.suppressClick) return
      dragRef.current.suppressClick = false
      event.preventDefault()
      event.stopPropagation()
    }

    node.addEventListener('pointerdown', onPointerDown)
    node.addEventListener('pointermove', onPointerMove)
    node.addEventListener('pointerup', onPointerUp)
    node.addEventListener('pointercancel', onPointerCancel)
    node.addEventListener('click', onClickCapture, true)
    return () => {
      node.removeEventListener('pointerdown', onPointerDown)
      node.removeEventListener('pointermove', onPointerMove)
      node.removeEventListener('pointerup', onPointerUp)
      node.removeEventListener('pointercancel', onPointerCancel)
      node.removeEventListener('click', onClickCapture, true)
      node.style.touchAction = previousTouchAction
    }
  }, [active, ref])
}

function shouldIgnoreDragStart(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return true
  return Boolean(target.closest('button,a,input,textarea,select,[role="button"],[data-no-drag]'))
}
