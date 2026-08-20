import type { BranchSummary, PlotNode, Snapshot, TurnEvent } from '../../types'

export interface BranchCreationSource {
  turnId: string
  title: string
  summary?: string
}

interface BranchNameLabels {
  main: string
  unknown: string
}

export function branchCreationSourceFromTurn(turn: TurnEvent, fallbackTitle: string): BranchCreationSource {
  return {
    turnId: turn.id,
    title: sourceTitle((turn.user_context_only ? turn.narrative : turn.user) || turn.narrative, fallbackTitle),
    summary: sourceSummary(turn.narrative),
  }
}

export function branchCreationSourceFromPlotNode(node: PlotNode): BranchCreationSource {
  return { turnId: node.id, title: node.title, summary: node.summary }
}

export function branchCreationSourceFromMessage(turnId: string, content: string, fallbackTitle: string): BranchCreationSource {
  return { turnId, title: sourceTitle(content, fallbackTitle), summary: sourceSummary(content) }
}

export function branchDisplayName(branch: BranchSummary | undefined, labels: BranchNameLabels) {
  if (!branch) return labels.unknown
  if (branch.id === 'main') return labels.main
  return branch.title?.trim() || branch.id
}

export function plotNodesFromSnapshot(snapshot: Snapshot | null, translate: (key: string, options?: Record<string, unknown>) => string): PlotNode[] {
  if (snapshot?.graph?.nodes?.length) return snapshot.graph.nodes
  return (snapshot?.turns || []).map((turn, index, turns) => plotNodeFromTurn(turn, index, turns.length, translate))
}

function sourceTitle(value: string, fallback: string) {
  const firstLine = value.trim().split(/\r?\n/).find(Boolean) || fallback
  return firstLine.length > 28 ? `${firstLine.slice(0, 27)}…` : firstLine
}

function sourceSummary(value: string) {
  const text = value.trim().replace(/\s+/g, ' ')
  if (!text) return undefined
  return text.length > 160 ? `${text.slice(0, 159)}…` : text
}

function plotNodeFromTurn(turn: TurnEvent, index: number, total: number, translate: (key: string, options?: Record<string, unknown>) => string): PlotNode {
  const title = firstLine((turn.user_context_only ? turn.narrative : turn.user) || turn.narrative) || `${translate('branchTimeline.nodeFallback')} ${index + 1}`
  return {
    id: turn.id,
    parent_id: turn.parent_id || undefined,
    branch_id: turn.branch_id || 'main',
    title: truncateText(title, 18),
    summary: truncateText(firstLine(turn.narrative) || translate('branchTimeline.nodeFallback'), 28),
    ts: turn.ts,
    current: index === total - 1,
    head: index === total - 1,
    terminal: turn.terminal_outcome?.terminal === true,
    terminal_type: turn.terminal_outcome?.type,
  }
}

function firstLine(value: string) {
  return value.trim().split(/\r?\n/).find(Boolean) || ''
}

function truncateText(value: string, maxLength: number) {
  const text = value.trim()
  return text.length > maxLength ? `${text.slice(0, maxLength - 1)}…` : text
}
