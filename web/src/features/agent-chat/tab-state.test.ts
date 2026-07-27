import { beforeEach, describe, expect, it } from 'vitest'
import {
  appendTab,
  createTabId,
  dedupeTabs,
  MAX_AGENT_CHAT_TABS,
  moveTab,
  nextActiveTabId,
  otherTabIds,
  persistWorkbenchState,
  readStoredWorkbenchState,
  setTabPinned,
  setTabTitle,
  tabIdsAfter,
  tabsInGroup,
} from './tab-state'
import type { AgentChatAgentTab, AgentChatTab } from './types'

function agentTab(id: string, sessionId: string, workspace = '/books/one'): AgentChatAgentTab {
  return { kind: 'agent', id, workspace, sessionId }
}

function terminalTab(id: string, workspace = '/books/one'): AgentChatTab {
  return { kind: 'terminal', id, workspace, profileId: 'shell', title: '' }
}

describe('agent-chat tab state', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('keeps one tab per page and per session but allows many terminals', () => {
    const deduped = dedupeTabs([
      { kind: 'page', id: 'p1', workspace: '/books/one', pageId: 'reader' },
      { kind: 'page', id: 'p2', workspace: '/books/one', pageId: 'reader' },
      agentTab('a1', 's1'),
      agentTab('a2', 's1'),
      terminalTab('t1'),
      terminalTab('t2'),
    ])

    expect(deduped.map((tab) => tab.id)).toEqual(['p1', 'a1', 't1', 't2'])
  })

  it('reuses an existing tab instead of opening a duplicate', () => {
    const tabs = [agentTab('a1', 's1'), terminalTab('t1')]

    expect(appendTab(tabs, agentTab('a2', 's1'))).toEqual({ tabs, activeId: 'a1' })
    expect(appendTab(tabs, { kind: 'page', id: 'p1', workspace: '/books/one', pageId: 'skills' })).toEqual({
      tabs: [...tabs, { kind: 'page', id: 'p1', workspace: '/books/one', pageId: 'skills' }],
      activeId: 'p1',
    })
  })

  it('drops the oldest tab when the limit is exceeded so the new tab stays visible', () => {
    const tabs = Array.from({ length: MAX_AGENT_CHAT_TABS }, (_, index) => terminalTab(`t${index}`))

    const { tabs: next, activeId } = appendTab(tabs, terminalTab('new'))

    expect(next).toHaveLength(MAX_AGENT_CHAT_TABS)
    expect(next[0].id).toBe('t1')
    expect(next.at(-1)?.id).toBe('new')
    expect(activeId).toBe('new')
  })

  it('activates the right neighbour after closing, falling back to the left one', () => {
    const tabs = [terminalTab('t1'), terminalTab('t2'), terminalTab('t3')]

    expect(nextActiveTabId(tabs, 't2', 't2')).toBe('t3')
    expect(nextActiveTabId(tabs, 't3', 't3')).toBe('t2')
    expect(nextActiveTabId(tabs, 't1', 't3')).toBe('t3')
    expect(nextActiveTabId([terminalTab('t1')], 't1', 't1')).toBeNull()
  })

  it('persists a distinct tab group for each project and drops cross-project records', () => {
    persistWorkbenchState({
      activeProjectPath: '/books/two',
      projects: {
        '/books/one': {
          tabs: [agentTab('a1', 's1'), terminalTab('t1')],
          activeTabIds: { primary: 'a1', secondary: null },
          focusedGroup: 'primary',
        },
        '/books/two': {
          tabs: [agentTab('a2', 's2', '/books/two'), terminalTab('t2', '/books/two')],
          activeTabIds: { primary: 'a2', secondary: null },
          focusedGroup: 'secondary',
        },
      },
    })

    const stored = readStoredWorkbenchState()
    expect(stored.activeProjectPath).toBe('/books/two')
    expect(stored.projects['/books/one'].tabs.map((tab) => tab.id)).toEqual(['a1', 't1'])
    expect(stored.projects['/books/two'].tabs.map((tab) => tab.id)).toEqual(['a2', 't2'])
    expect(stored.projects['/books/two'].focusedGroup).toBe('secondary')

    window.localStorage.setItem(
      'nova.agentchat.workbenches.v3',
      JSON.stringify({
        activeProjectPath: '/books/one',
        projects: {
          '/books/one': {
            tabs: [agentTab('foreign', 's9', '/books/two'), { kind: 'agent', id: 'missing-workspace', sessionId: 's1' }],
            activeTabIds: { primary: 'foreign', secondary: null },
            focusedGroup: 'primary',
          },
        },
      }),
    )
    expect(readStoredWorkbenchState().projects['/books/one']).toMatchObject({
      tabs: [],
      activeTabIds: { primary: null, secondary: null },
    })
  })

  it('does not persist blank draft conversations', () => {
    persistWorkbenchState({
      activeProjectPath: '/books/one',
      projects: {
        '/books/one': {
          tabs: [{ ...agentTab('draft', 's-draft'), draft: true }, agentTab('saved', 's-saved')],
          activeTabIds: { primary: 'draft', secondary: null },
          focusedGroup: 'primary',
        },
      },
    })

    expect(readStoredWorkbenchState().projects['/books/one']).toMatchObject({
      tabs: [expect.objectContaining({ id: 'saved' })],
      activeTabIds: { primary: null, secondary: null },
    })
  })

  it('generates unique tab ids carrying the tab kind', () => {
    const ids = [createTabId('agent'), createTabId('agent'), createTabId('terminal')]

    expect(new Set(ids).size).toBe(3)
    expect(ids[0].startsWith('agent-')).toBe(true)
    expect(ids[2].startsWith('terminal-')).toBe(true)
  })

})

describe('agent-chat tab groups', () => {
  const split: AgentChatTab[] = [
    terminalTab('t1'),
    terminalTab('t2'),
    { ...terminalTab('t3'), group: 'secondary' },
  ]

  it('treats a tab without a group as belonging to the left side', () => {
    expect(tabsInGroup(split, 'primary').map((tab) => tab.id)).toEqual(['t1', 't2'])
    expect(tabsInGroup(split, 'secondary').map((tab) => tab.id)).toEqual(['t3'])
  })

  it('reorders inside a strip and appends when dropped past the last tab', () => {
    expect(moveTab(split, 't2', 'primary', 't1').map((tab) => tab.id)).toEqual(['t2', 't1', 't3'])
    expect(moveTab(split, 't1', 'primary', null).map((tab) => tab.id)).toEqual(['t2', 't1', 't3'])
  })

  it('moves a tab across the split', () => {
    const moved = moveTab(split, 't1', 'secondary', 't3')

    expect(tabsInGroup(moved, 'primary').map((tab) => tab.id)).toEqual(['t2'])
    expect(tabsInGroup(moved, 'secondary').map((tab) => tab.id)).toEqual(['t1', 't3'])
  })

  it('holds pinned tabs at the front of their own strip', () => {
    const pinned = setTabPinned(split, 't2', true)

    expect(tabsInGroup(pinned, 'primary').map((tab) => tab.id)).toEqual(['t2', 't1'])
    // A drop cannot push an unpinned tab in front of a pinned one.
    expect(tabsInGroup(moveTab(pinned, 't1', 'primary', 't2'), 'primary').map((tab) => tab.id)).toEqual(['t2', 't1'])
    expect(setTabPinned(pinned, 't2', false).find((tab) => tab.id === 't2')?.pinned).toBeUndefined()
  })

  it('renames a tab and clears the override when the title is blank', () => {
    expect(setTabTitle(split, 't1', ' Draft ').find((tab) => tab.id === 't1')?.customTitle).toBe('Draft')
    expect(setTabTitle(setTabTitle(split, 't1', 'Draft'), 't1', '  ').find((tab) => tab.id === 't1')?.customTitle).toBeUndefined()
  })

  it('bulk closes stay inside one strip and spare pinned tabs', () => {
    const tabs = setTabPinned([...split, terminalTab('t4')], 't1', true)

    expect(otherTabIds(tabs, 't2')).toEqual(['t4'])
    expect(tabIdsAfter(tabs, 't2')).toEqual(['t4'])
    expect(tabIdsAfter(tabs, 't4')).toEqual([])
    // 't3' lives on the other side, so neither list may reach it.
    expect(otherTabIds(tabs, 't3')).toEqual([])
  })
})
