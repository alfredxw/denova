import type { AgentMessageView } from '@/lib/agent-message-view'

export interface AgentProcessNode {
  view: AgentMessageView
  children: AgentProcessNode[]
}

/**
 * Rebuilds the durable tool hierarchy without mutating message history.
 * A parent must have appeared earlier in the append-only stream; malformed
 * forward references and cycles stay visible as roots instead of recursing.
 */
export function buildToolCallTree(views: AgentMessageView[]): AgentProcessNode[] {
  const nodes: AgentProcessNode[] = views.map(view => ({ view, children: [] }))
  const toolIndexes = new Map<string, number>()
  nodes.forEach((node, index) => {
    if (node.view.kind === 'tool' && node.view.partId) toolIndexes.set(node.view.partId, index)
  })

  const roots: AgentProcessNode[] = []
  nodes.forEach((node, index) => {
    const parentCallID = node.view.metadata.parent_call_id || ''
    const parentIndex = parentCallID ? toolIndexes.get(parentCallID) : undefined
    if (parentIndex === undefined || parentIndex >= index) {
      roots.push(node)
      return
    }
    nodes[parentIndex].children.push(node)
  })
  return roots
}
