import { useMemo, useRef, useState } from 'react'
import { ChevronDown, ChevronUp, GitBranch, Hand, MousePointer2, Plus, Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import type { BranchSummary, PlotNode, Snapshot } from '../types'

interface BranchTimelineProps {
  snapshot: Snapshot | null
  branches: BranchSummary[]
  currentBranchId: string
  onSwitchBranch: (branchId: string) => void
  onCreateBranch: (turnId: string, title: string) => void
  onDeleteBranch: (branchId: string) => void
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
  from?: PositionedNode
}

interface GraphLayout {
  rows: TimelineRow[]
  positionedNodes: PositionedNode[]
  nodeById: Map<string, PositionedNode>
  connections: Array<{ from: PositionedNode; to: PositionedNode; branchChanged: boolean; color: string }>
  emptyBranches: EmptyBranchMarker[]
  chapters: string[]
  width: number
  height: number
}

const COLUMN_WIDTH = 250
const LANE_HEIGHT = 72
const NODE_CARD_WIDTH = 176
const NODE_DOT_X = 18
const NODE_CENTER_Y = 21
const GRAPH_LEFT = 72
const GRAPH_TOP = 58
const GRAPH_RIGHT = 64
const GRAPH_BOTTOM = 28

const BRANCH_COLORS = [
  { color: '#8b5cf6', soft: 'rgba(139,92,246,0.16)' },
  { color: '#3b82f6', soft: 'rgba(59,130,246,0.16)' },
  { color: '#f05260', soft: 'rgba(240,82,96,0.16)' },
  { color: '#59c178', soft: 'rgba(89,193,120,0.16)' },
  { color: '#f59e3d', soft: 'rgba(245,158,61,0.16)' },
  { color: '#22c7d6', soft: 'rgba(34,199,214,0.16)' },
]

export function BranchTimeline({ snapshot, branches, currentBranchId, onSwitchBranch, onCreateBranch, onDeleteBranch }: BranchTimelineProps) {
  const [expanded, setExpanded] = useState(false)
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [branchTitle, setBranchTitle] = useState('')
  const scrollRef = useDragScroll<HTMLDivElement>()
  const graphNodes = snapshot?.graph?.nodes || []
  const graphBranches = snapshot?.graph?.branches?.length ? snapshot.graph.branches : branches
  const selectedNode = graphNodes.find((node) => node.id === selectedNodeId) || null
  const layout = useMemo(() => buildGraphLayout(graphNodes, graphBranches), [graphBranches, graphNodes])
  const emptyBranchCount = graphBranches.filter((branch) => isEmptyBranch(branch, graphNodes)).length

  const selectNode = (node: PlotNode) => {
    setSelectedNodeId(node.id)
    if (node.branch_id !== currentBranchId) onSwitchBranch(node.branch_id)
  }

  const openCreateDialog = () => {
    if (!selectedNode) return
    setBranchTitle(`基于「${selectedNode.title}」的新剧情线`)
    setCreateDialogOpen(true)
  }

  const submitCreateBranch = () => {
    if (!selectedNode) return
    onCreateBranch(selectedNode.id, branchTitle.trim() || '新剧情线')
    setCreateDialogOpen(false)
    setBranchTitle('')
  }

  const deleteBranch = (branch: BranchSummary) => {
    const label = formatBranchName(branch)
    if (!window.confirm(`删除空剧情线「${label}」？`)) return
    onDeleteBranch(branch.id)
    if (selectedNode?.branch_id === branch.id) setSelectedNodeId(null)
  }

  return (
    <div className={`${expanded ? 'h-[430px]' : 'h-[52px]'} border-t border-[#2f3540] bg-[#14171c] px-4 py-3 transition-[height]`}>
      <div className="flex items-center justify-between gap-3 text-xs text-[#858b96]">
        <button type="button" className="flex items-center gap-1.5 font-medium text-[#c3cad6] hover:text-[#edf2fa]" onClick={() => setExpanded(!expanded)}>
          <GitBranch className="h-3.5 w-3.5 text-[#8b5cf6]" />
          剧情路线图
          {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronUp className="h-3.5 w-3.5" />}
        </button>
        <div className="flex min-w-0 flex-1 items-center justify-end gap-2">
          <span className="truncate text-[#737d8d]">{graphNodes.length || snapshot?.turns?.length || 0} 个剧情节点</span>
          {emptyBranchCount > 0 && <Badge variant="outline" className="border-[#4a3d2f] bg-[#282119] text-[#d5aa72]">{emptyBranchCount} 条空剧情线</Badge>}
          {selectedNode && (
            <Button variant="outline" size="xs" className="gap-1.5 border-[#3a414d] bg-[#20242b] text-[#c3cbd7] hover:bg-[#252c38]" onClick={openCreateDialog}>
              <Plus className="h-3.5 w-3.5" />
              从选中节点创建
            </Button>
          )}
        </div>
      </div>

      {expanded && (
        <div className="mt-3 h-[358px] overflow-hidden rounded-xl border border-[#2d3440] bg-[#11151c] shadow-[0_18px_42px_rgba(0,0,0,0.30),inset_0_1px_0_rgba(255,255,255,0.05)]">
          <div className="flex h-11 items-center justify-between border-b border-[#29313c] bg-[#171c25] px-4">
            <div className="flex items-center gap-2">
              <div className="flex overflow-hidden rounded-md border border-[#2f3a49] bg-[#10141c]">
                <button type="button" className="flex h-7 w-8 items-center justify-center bg-[#1f3157] text-[#6aa8ff]" aria-label="选择节点">
                  <MousePointer2 className="h-3.5 w-3.5" />
                </button>
                <button type="button" className="flex h-7 w-8 items-center justify-center text-[#9aa4b5]" aria-label="拖动画布">
                  <Hand className="h-3.5 w-3.5" />
                </button>
              </div>
              <Button size="xs" variant="outline" className="gap-1.5 border-[#354051] bg-[#1a202b] text-[#c4ccd8] hover:bg-[#242b38]" disabled={!selectedNode} onClick={openCreateDialog}>
                <Plus className="h-3.5 w-3.5 text-[#d8b35f]" />
                创建分支
              </Button>
              <Button size="xs" variant="outline" className="gap-1.5 border-[#3a3036] bg-[#201a20] text-[#d7a1ad] hover:bg-[#302029]" disabled={!layout.rows.some((row) => row.branch && isEmptyBranch(row.branch, graphNodes))}>
                <Trash2 className="h-3.5 w-3.5" />
                删除空分支
              </Button>
            </div>
            <div className="flex items-center gap-2 text-xs text-[#8d96a7]">
              <span>100%</span>
              <Badge variant="outline" className="border-[#384150] bg-[#202633] text-[#aeb8c8]">居中</Badge>
            </div>
          </div>

          <div className="grid h-[247px] grid-cols-[210px_minmax(0,1fr)] border-b border-[#29313c]">
            <aside className="border-r border-[#29313c] bg-[#151a22] p-3">
              <div className="mb-3 flex items-center justify-between text-sm font-semibold text-[#dde3ed]">
                章节
                <Plus className="h-4 w-4 text-[#8c96a6]" />
              </div>
              <div className="space-y-1.5">
                {layout.rows.map((row) => (
                  <div key={row.branchId} className={`flex h-8 items-center gap-2 rounded-md px-2 text-xs ${row.branchId === currentBranchId ? 'bg-[#202838] text-[#e4ebf6]' : 'text-[#9aa4b5]'}`}>
                    <span className="h-2.5 w-2.5 rounded-full" style={{ background: row.color }} />
                    <button type="button" className="min-w-0 flex-1 truncate text-left" onClick={() => onSwitchBranch(row.branchId)} title={formatBranchName(row.branch)}>
                      {formatBranchName(row.branch)}
                    </button>
                    <span className="text-[#7e8898]">{row.nodes.length}</span>
                    {row.branch && isEmptyBranch(row.branch, graphNodes) && (
                      <button
                        type="button"
                        className="rounded p-0.5 text-[#9d6673] hover:bg-[#3a2028] hover:text-[#ff9aaa]"
                        onClick={() => deleteBranch(row.branch!)}
                        aria-label={`删除空剧情线 ${formatBranchName(row.branch)}`}
                        title="删除空剧情线"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                ))}
                {layout.rows.length === 0 && <div className="rounded-md border border-dashed border-[#303846] p-3 text-xs text-[#858f9f]">还没有剧情路线。</div>}
              </div>
            </aside>

            <div ref={scrollRef} className="overflow-auto bg-[#0f131a] cursor-grab active:cursor-grabbing">
              <div
                data-testid="branch-graph-canvas"
                className="relative min-w-max"
                style={{ width: layout.width, height: layout.height }}
              >
                <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_1px_1px,rgba(148,163,184,0.12)_1px,transparent_0)] [background-size:18px_18px]" />
                {layout.chapters.map((chapter, index) => (
                  <div key={chapter} className="absolute top-0 h-full border-l border-dashed border-[#2f3a48]" style={{ left: GRAPH_LEFT + index * COLUMN_WIDTH - 24 }}>
                    <div className="ml-8 mt-5 whitespace-nowrap text-xs text-[#909bad]">{chapter}</div>
                  </div>
                ))}
                <svg className="pointer-events-none absolute inset-0 overflow-visible" width={layout.width} height={layout.height} aria-hidden="true">
                  <defs>
                    <filter id="nova-route-glow" x="-20%" y="-120%" width="140%" height="340%">
                      <feGaussianBlur stdDeviation="2.4" result="blur" />
                      <feMerge>
                        <feMergeNode in="blur" />
                        <feMergeNode in="SourceGraphic" />
                      </feMerge>
                    </filter>
                  </defs>
                  {layout.connections.map((connection) => (
                    <path
                      key={`${connection.from.node.id}-${connection.to.node.id}`}
                      d={connectionPath(connection.from, connection.to)}
                      fill="none"
                      stroke={connection.color}
                      strokeWidth={connection.branchChanged ? 2.6 : 2}
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      opacity={0.9}
                      filter="url(#nova-route-glow)"
                    />
                  ))}
                  {layout.emptyBranches.map((branch) => branch.from ? (
                    <path
                      key={`empty-${branch.branch.id}`}
                      d={connectionPath(branch.from, branch)}
                      fill="none"
                      stroke={branch.color}
                      strokeWidth={2}
                      strokeDasharray="4 6"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      opacity={0.8}
                    />
                  ) : null)}
                </svg>

                {layout.positionedNodes.map(({ node, x, y, color, colorSoft }) => (
                  <button
                    key={node.id}
                    type="button"
                    className={`absolute z-10 flex h-[42px] items-center gap-2 rounded-lg border px-3 text-left shadow-[0_12px_26px_rgba(0,0,0,0.25)] backdrop-blur transition ${node.id === selectedNodeId ? 'border-[#f0cf8b] text-[#fff1ce] ring-2 ring-[#f0cf8b]/25' : node.current ? 'border-[#6aa8ff] text-[#eaf4ff]' : 'border-[#3a4656] text-[#c4ccd8] hover:border-[#74849a]'}`}
                    style={{ left: x, top: y, width: NODE_CARD_WIDTH, background: node.id === selectedNodeId ? 'rgba(64,48,28,0.96)' : colorSoft }}
                    onClick={() => selectNode(node)}
                    title={`${node.title}\n${node.summary}`}
                  >
                    <span className="h-2.5 w-2.5 shrink-0 rounded-full shadow-[0_0_14px_currentColor]" style={{ background: color, color }} />
                    <span className="min-w-0 flex-1 truncate text-[12px] font-medium">{node.title}</span>
                    <span className="text-[#7d8797]">⋮</span>
                  </button>
                ))}

                {layout.emptyBranches.map((empty) => (
                  <div
                    key={empty.branch.id}
                    className="absolute z-10 flex h-[36px] items-center gap-2 rounded-lg border border-dashed px-3 text-xs text-[#b7beca]"
                    style={{ left: empty.x, top: empty.y + 3, width: NODE_CARD_WIDTH, borderColor: empty.color, background: 'rgba(21,25,34,0.88)' }}
                  >
                    <span className="h-2.5 w-2.5 rounded-full" style={{ background: empty.color }} />
                    空剧情线
                  </div>
                ))}

                {layout.rows.length === 0 && <span className="absolute left-6 top-6 text-xs text-[#858b96]">还没有剧情节点，输入第一句话开始。</span>}
              </div>
            </div>
          </div>

          <div className="grid h-[70px] grid-cols-[210px_minmax(0,1fr)] bg-[#161b24]">
            <div className="flex items-center border-r border-[#29313c] px-4 text-xs text-[#818b9b]">
              {selectedNode ? (
                <div className="min-w-0">
                  <div className="text-[#d6dbe5]">已选节点</div>
                  <div className="truncate">{selectedNode.title}</div>
                </div>
              ) : (
                <span>点击节点先选中。</span>
              )}
            </div>
            <div className="flex min-w-0 items-center gap-3 px-4">
              <MiniMap layout={layout} />
              {selectedNode && (
                <Button size="xs" className="shrink-0 gap-1.5 bg-[#6d3fe6] hover:bg-[#7c4bf0]" onClick={openCreateDialog}>
                  <Plus className="h-3.5 w-3.5" />
                  创建剧情线
                </Button>
              )}
            </div>
          </div>
        </div>
      )}

      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent className="border-[#303238] bg-[#202329] text-[#d7dbe2]">
          <DialogHeader>
            <DialogTitle>从选中节点创建剧情线</DialogTitle>
            <DialogDescription className="text-[#9aa4b5]">
              {selectedNode ? `将从「${selectedNode.title}」分叉，创建后故事舞台会切换到新剧情线。` : ''}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Input className="border-[#3a3d45] bg-[#17191d] text-sm" value={branchTitle} onChange={(event) => setBranchTitle(event.target.value)} placeholder="剧情线名称" />
            {selectedNode?.summary && <div className="rounded-md border border-[#303743] bg-[#17191d] p-2 text-xs leading-5 text-[#aab2c0]">{selectedNode.summary}</div>}
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setCreateDialogOpen(false)}>取消</Button>
            <Button className="gap-1.5 bg-[#2d6fb8] hover:bg-[#347dca]" onClick={submitCreateBranch}>
              <Plus className="h-4 w-4" />
              创建并切换
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function MiniMap({ layout }: { layout: GraphLayout }) {
  return (
    <div className="relative h-12 min-w-0 flex-1 overflow-hidden rounded-lg border border-[#2b3340] bg-[#10151d]">
      <svg className="absolute inset-0 h-full w-full" viewBox={`0 0 ${layout.width} ${layout.height}`} preserveAspectRatio="none" aria-hidden="true">
        {layout.connections.map((connection) => (
          <path
            key={`mini-${connection.from.node.id}-${connection.to.node.id}`}
            d={connectionPath(connection.from, connection.to)}
            fill="none"
            stroke={connection.color}
            strokeWidth={6}
            strokeLinecap="round"
            strokeLinejoin="round"
            opacity={0.58}
          />
        ))}
        {layout.positionedNodes.map((item) => (
          <circle key={`mini-node-${item.node.id}`} cx={item.x + NODE_DOT_X} cy={item.y + NODE_CENTER_Y} r={5} fill={item.color} opacity={0.85} />
        ))}
      </svg>
      <div className="absolute inset-y-1 left-[12%] w-[34%] rounded border border-[#a8c7ff] bg-[#89b4ff]/10 shadow-[0_0_18px_rgba(137,180,255,0.25)]" />
    </div>
  )
}

function buildGraphLayout(nodes: PlotNode[], branches: BranchSummary[]): GraphLayout {
  const rowsByBranch = new Map<string, TimelineRow>()
  for (const [index, branch] of branches.entries()) {
    const palette = BRANCH_COLORS[index % BRANCH_COLORS.length]
    rowsByBranch.set(branch.id, {
      branchId: branch.id,
      branch,
      nodes: [],
      startColumn: Math.max(0, branch.from_event ? nodeColumnById(branch.from_event, nodes) + 1 : 0),
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
    nodes: row.nodes.sort((a, b) => nodeColumn(a, nodes) - nodeColumn(b, nodes)),
  }))
  let maxColumn = 0
  for (const node of nodes) maxColumn = Math.max(maxColumn, nodeColumn(node, nodes))
  for (const row of rows) maxColumn = Math.max(maxColumn, row.startColumn)
  const positionedNodes: PositionedNode[] = []
  const nodeById = new Map<string, PositionedNode>()
  rows.forEach((row, rowIndex) => {
    for (const node of row.nodes) {
      const column = nodeColumn(node, nodes)
      const positioned = {
        node,
        row: rowIndex,
        column,
        x: GRAPH_LEFT + column * COLUMN_WIDTH,
        y: GRAPH_TOP + rowIndex * LANE_HEIGHT,
        color: row.color,
        colorSoft: row.colorSoft,
      }
      positionedNodes.push(positioned)
      nodeById.set(node.id, positioned)
    }
  })
  const connections = positionedNodes.flatMap((positioned) => {
    if (!positioned.node.parent_id) return []
    const from = nodeById.get(positioned.node.parent_id)
    if (!from) return []
    return [{ from, to: positioned, branchChanged: from.node.branch_id !== positioned.node.branch_id, color: positioned.color }]
  })
  const emptyBranches = rows.flatMap((row, rowIndex) => {
    if (!row.branch || !row.empty) return []
    const column = row.startColumn
    return [{
      branch: row.branch,
      row: rowIndex,
      column,
      x: GRAPH_LEFT + column * COLUMN_WIDTH,
      y: GRAPH_TOP + rowIndex * LANE_HEIGHT,
      color: row.color,
      from: row.branch.from_event ? nodeById.get(row.branch.from_event) : undefined,
    }]
  })
  const chapters = Array.from({ length: Math.max(1, maxColumn + 1) }, (_, index) => index === 0 ? '序章' : `第${toChineseNumber(index)}章`)
  return {
    rows,
    positionedNodes,
    nodeById,
    connections,
    emptyBranches,
    chapters,
    width: Math.max(900, GRAPH_LEFT + GRAPH_RIGHT + (maxColumn + 1) * COLUMN_WIDTH + NODE_CARD_WIDTH),
    height: Math.max(220, GRAPH_TOP + GRAPH_BOTTOM + rows.length * LANE_HEIGHT),
  }
}

function connectionPath(from: Pick<PositionedNode, 'x' | 'y'>, to: Pick<PositionedNode, 'x' | 'y'>) {
  const startX = from.x + NODE_CARD_WIDTH
  const startY = from.y + NODE_CENTER_Y
  const endX = to.x
  const endY = to.y + NODE_CENTER_Y
  const curve = Math.max(52, Math.min(120, Math.abs(endX - startX) * 0.42))
  return `M ${startX} ${startY} C ${startX + curve} ${startY}, ${endX - curve} ${endY}, ${endX} ${endY}`
}

function nodeColumn(node: PlotNode, nodes: PlotNode[]) {
  return nodeColumnById(node.id, nodes)
}

function nodeColumnById(nodeId: string, nodes: PlotNode[]) {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  let column = 0
  const seen = new Set<string>()
  for (let id = nodeId; id;) {
    if (seen.has(id)) break
    seen.add(id)
    const node = byId.get(id)
    if (!node?.parent_id) break
    column += 1
    id = node.parent_id
  }
  return column
}

function isEmptyBranch(branch: BranchSummary, nodes: PlotNode[]) {
  return branch.id !== 'main' && branch.head === branch.from_event && !nodes.some((node) => node.branch_id === branch.id)
}

function formatBranchName(branch?: BranchSummary) {
  if (!branch) return '未知剧情线'
  if (branch.title?.trim()) return branch.title.trim()
  if (branch.id === 'main') return '主线'
  return branch.id
}

function toChineseNumber(value: number) {
  const digits = ['零', '一', '二', '三', '四', '五', '六', '七', '八', '九']
  if (value <= 10) return value === 10 ? '十' : digits[value]
  if (value < 20) return `十${digits[value - 10]}`
  const tens = Math.floor(value / 10)
  const ones = value % 10
  return `${digits[tens]}十${ones ? digits[ones] : ''}`
}

function useDragScroll<T extends HTMLElement>() {
  const ref = useRef<T | null>(null)
  const dragRef = useRef<{ x: number; y: number; left: number; top: number; active: boolean }>({ x: 0, y: 0, left: 0, top: 0, active: false })

  return (node: T | null) => {
    if (!node || ref.current === node) {
      ref.current = node
      return
    }
    ref.current = node
    node.onpointerdown = (event) => {
      if ((event.target as HTMLElement).closest('button,input')) return
      dragRef.current = { x: event.clientX, y: event.clientY, left: node.scrollLeft, top: node.scrollTop, active: true }
      node.setPointerCapture(event.pointerId)
    }
    node.onpointermove = (event) => {
      if (!dragRef.current.active) return
      node.scrollLeft = dragRef.current.left - (event.clientX - dragRef.current.x)
      node.scrollTop = dragRef.current.top - (event.clientY - dragRef.current.y)
    }
    node.onpointerup = (event) => {
      dragRef.current.active = false
      node.releasePointerCapture(event.pointerId)
    }
    node.onpointercancel = () => {
      dragRef.current.active = false
    }
  }
}
