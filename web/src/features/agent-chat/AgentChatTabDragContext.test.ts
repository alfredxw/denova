import { describe, expect, it } from 'vitest'
import { resolveWorkbenchTabDropEdge } from '@/components/workbench/WorkbenchTabDrag'
import { agentChatDropBeforeId, agentChatTabSortableId, agentChatTabStripDropId } from './AgentChatTabDragContext'
import type { AgentChatTab } from './types'

function terminalTab(id: string, group: 'primary' | 'secondary' = 'primary'): AgentChatTab {
  return {
    kind: 'terminal',
    id,
    projectId: 'project-one',
    workspace: '/books/one',
    group,
    profileId: 'shell',
    title: id,
  }
}

describe('AgentChat tab drag placement', () => {
  const tabs = [
    terminalTab('p1'),
    terminalTab('p2'),
    terminalTab('p3'),
    terminalTab('s1', 'secondary'),
    terminalTab('s2', 'secondary'),
  ]

  it('uses project-scoped sortable and strip ids', () => {
    expect(agentChatTabSortableId('project-one', 'tab-one')).not.toBe(agentChatTabSortableId('project-two', 'tab-one'))
    expect(agentChatTabStripDropId('project-one', 'primary')).not.toBe(agentChatTabStripDropId('project-one', 'secondary'))
  })

  it('translates same-strip arrayMove placement into the next tab id', () => {
    expect(agentChatDropBeforeId(tabs, 'p1', 'primary', 'p3')).toBeNull()
    expect(agentChatDropBeforeId(tabs, 'p3', 'primary', 'p1')).toBe('p1')
    expect(agentChatDropBeforeId(tabs, 'p2', 'primary', 'p2')).toBe('p2')
  })

  it('chooses either edge of a cross-strip target and supports an empty-strip append', () => {
    expect(agentChatDropBeforeId(tabs, 'p1', 'secondary', 's1')).toBe('s1')
    expect(agentChatDropBeforeId(tabs, 'p1', 'secondary', 's1', true)).toBe('s2')
    expect(agentChatDropBeforeId(tabs, 'p1', 'secondary', null)).toBeNull()
  })

  it('keeps the rendered cross-strip insertion edge aligned with the dragged preview center', () => {
    const target = { left: 400, width: 160 }
    expect(resolveWorkbenchTabDropEdge({ left: 360, width: 160 }, target)).toBe('start')
    expect(resolveWorkbenchTabDropEdge({ left: 490, width: 160 }, target)).toBe('end')
  })
})
